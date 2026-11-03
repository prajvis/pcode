// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/prajvis/pcode/pcode/internal/execx"
	"github.com/prajvis/pcode/pcode/internal/render"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{nil, 0},
		{fmt.Errorf("usage: pcode web exec <command>"), 1},
		{render.ErrAgent, 2},
		{fmt.Errorf("%w: cursor agent error", render.ErrAgent), 2},
		{render.ErrIncomplete, 3},
		{fmt.Errorf("%w: stream ended", render.ErrIncomplete), 3},
		{execx.ErrIdleTimeout, 4},
		{fmt.Errorf("%w after 50ms", execx.ErrIdleTimeout), 4},
		{context.Canceled, 130},
		{fmt.Errorf("wrap: %w", context.Canceled), 130},
	}
	for _, tc := range cases {
		if got := ExitCode(tc.err); got != tc.code {
			t.Errorf("ExitCode(%v)=%d want %d", tc.err, got, tc.code)
		}
	}
}
