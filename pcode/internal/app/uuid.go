// SPDX-License-Identifier: Apache-2.0

package app

import (
	"regexp"
	"strings"
)

var uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

func ExtractUUID(text string) (string, bool) {
	match := uuidRe.FindString(strings.TrimSpace(text))
	if match == "" {
		return "", false
	}
	return match, true
}
