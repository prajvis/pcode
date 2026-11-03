// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"

	"github.com/prajvis/pcode/pcode/internal/execx"
	"github.com/prajvis/pcode/pcode/internal/render"
)

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	if errors.Is(err, execx.ErrIdleTimeout) {
		return 4
	}
	if errors.Is(err, render.ErrIncomplete) {
		return 3
	}
	if errors.Is(err, render.ErrAgent) {
		return 2
	}
	return 1
}
