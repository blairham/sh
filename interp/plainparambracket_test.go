// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// TestAValueWithABracketIsStillMatched: a parameter whose value holds a
// bracket expression is a pattern when it is expanded unquoted, so the plain
// `$name` shortcut must leave it to the word road, which matches it.
func TestAValueWithABracketIsStillMatched(t *testing.T) {
	out, _ := run(t, `: > a; v='[ab]'; echo $v; w='x]'; echo $w`, nil)
	if want := "a\nx]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
