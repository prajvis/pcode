// SPDX-License-Identifier: Apache-2.0

package render

import "errors"

var (
	ErrAgent      = errors.New("agent reported an error")
	ErrIncomplete = errors.New("agent run incomplete")
)
