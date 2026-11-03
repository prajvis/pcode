// SPDX-License-Identifier: Apache-2.0

package execx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

var ErrIdleTimeout = errors.New("idle timeout waiting for output")

func BashQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func CopyIdle(ctx context.Context, r io.Reader, w io.Writer, idle time.Duration) error {
	return CopyIdleBusy(ctx, r, w, idle, nil)
}

func CopyIdleBusy(ctx context.Context, r io.Reader, w io.Writer, idle time.Duration, busy func() bool) error {
	if idle <= 0 {
		_, err := io.Copy(w, r)
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	type chunk struct {
		data []byte
		err  error
	}
	ch := make(chan chunk)
	done := make(chan struct{})
	defer close(done)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			data := append([]byte(nil), buf[:n]...)
			select {
			case ch <- chunk{data, err}:
				if err != nil {
					return
				}
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	timer := time.NewTimer(idle)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			if busy != nil && busy() {
				timer.Reset(idle)
				continue
			}
			return fmt.Errorf("%w after %s", ErrIdleTimeout, idle)
		case c := <-ch:
			if len(c.data) > 0 {
				if _, err := w.Write(c.data); err != nil {
					return err
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(idle)
			}
			if c.err == io.EOF {
				return nil
			}
			if c.err != nil {
				return c.err
			}
		}
	}
}
