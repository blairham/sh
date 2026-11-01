// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestTheEqualsTildeTableIsAskedAboutEachCommand: the runner remembers the
// table's answer for the last command name it asked about, so a command that
// is in the table must not be answered with the one before it that is not,
// nor the other way round.
func TestTheEqualsTildeTableIsAskedAboutEachCommand(t *testing.T) {
	t.Parallel()
	out, _ := runGrammar(t, `HOME=/h
printf '%s\n' a=~
echo a=~
printf '%s\n' a=~
echo a=~
`, nil, func(r *Runner) { r.MarkBuiltinWhoseEqualsOpensATildeContext("echo") })
	if want := "a=~\na=/h\na=~\na=/h\n"; out != want {
		t.Errorf(
			"got %q, want %q: the builtin in the table opens a tilde context after `=` and the other does not",
			out,
			want,
		)
	}
}
