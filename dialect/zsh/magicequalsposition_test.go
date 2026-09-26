// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The two positions MAGIC_EQUAL_SUBST does not reach, and the words behind
// which the next one still names the command.
//
// interp/equalscontextposition.go holds the rule and the core's own tests.
// This is the dialect's half: which words this shell registers, which is data
// rather than behavior and would otherwise be asserted by nothing. A mutant
// dropping `command` from the list leaves every core test green.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, `-f -c`, `HOME=/Users/testhome`, 2026-09-26. `go
// version -m` on that path reports *not a Go executable*.
func TestMagicEqualSubstDoesNotReachTheCommandWordOrAnArrayLiteral(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an array literal's element", `x=(a=~); print -r -- $x`, "a=~"},
		{"the command word", `--opt=~`, "--opt=~"},
		{"behind `command`", `command --opt=~`, "--opt=~"},
		{"behind `noglob`", `noglob --opt=~`, "--opt=~"},
		{"behind `builtin`", `builtin --opt=~`, "--opt=~"},
		// **The control**, and it is what says the option was on for all of
		// the above: the same word one place along is an argument and does
		// expand.
		{"an ordinary argument", `print -r -- --opt=~`, "--opt=/Users/testhome"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := magicEqualRun(t, "setopt magicequalsubst\n"+tc.src)
			if !strings.Contains(got, tc.want) {
				t.Errorf("setopt magicequalsubst; %s = %q, want it to name %q", tc.src, got, tc.want)
			}
			// Nothing above may read as "the tilde was never expandable":
			// a row wanting the tilde kept must not also match the path.
			if tc.want != "--opt=/Users/testhome" && strings.Contains(got, "/Users/testhome") {
				t.Errorf("setopt magicequalsubst; %s = %q, which expanded the tilde", tc.src, got)
			}
		})
	}
}
