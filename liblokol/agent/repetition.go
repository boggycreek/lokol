// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package agent

import "strings"

// IsVerbatimRepetition detects if current and previous responses are identical or near-verbatim repeats across turns (lokol-kih.8).
func IsVerbatimRepetition(current, previous string) bool {
	c := strings.TrimSpace(current)
	p := strings.TrimSpace(previous)
	if c == "" || p == "" {
		return false
	}
	if c == p {
		return true
	}
	normC := strings.ToLower(strings.Join(strings.Fields(c), " "))
	normP := strings.ToLower(strings.Join(strings.Fields(p), " "))
	return normC == normP
}
