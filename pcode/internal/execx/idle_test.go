// SPDX-License-Identifier: Apache-2.0

package execx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestBashQuote(t *testing.T) {
	if got := BashQuote("pwd"); got != "'pwd'" {
		t.Fatalf("got %q", got)
	}
	if got := BashQuote("it's"); got != `'it'"'"'s'` {
		t.Fatalf("got %q", got)
	}
}

func TestCopyIdleEmpty(t *testing.T) {
	err := CopyIdle(context.Background(), strings.NewReader(""), io.Discard, time.Second)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCopyIdleTimeout(t *testing.T) {
	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()
	err := CopyIdle(context.Background(), pr, io.Discard, 50*time.Millisecond)
	if !errors.Is(err, ErrIdleTimeout) {
		t.Fatalf("err=%v", err)
	}
}

func TestCopyIdlePassesData(t *testing.T) {
	var buf bytes.Buffer
	err := CopyIdle(context.Background(), strings.NewReader("hello"), &buf, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello" {
		t.Fatalf("got %q", buf.String())
	}
}
