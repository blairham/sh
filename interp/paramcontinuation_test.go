// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// A backslash-newline inside `${ }` is a line continuation on every route into
// one, and the script goes on past it. Measured 2026-09-16 from script files:
// bash 5.3, bash 3.2, zsh 5.9.2, ksh93u+ and dash print every line below
// (#3452). Before, each was a bad substitution, and the two that did not break
// outright read wrong — a trim with the pair in front of its pattern stripped
// nothing.
//
// The here-document rows are the route the parser does not see: an unquoted
// body is read into words when it runs, through the same scanner.
func TestALineContinuationInAParameterExpansionIsRemovedOnEveryRoute(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"in double quotes", "x=5; echo \"[${x\\\n}]\"", "[5]"},
		{"unquoted", "x=5; echo [${x\\\n}]", "[5]"},
		{"inside the name", "xy=5; echo \"[${x\\\ny}]\"", "[5]"},
		{"an assignment's value", "x=5; y=${x\\\n}; echo \"[$y]\"", "[5]"},
		{"a case word", "x=5; case ${x\\\n} in 5) echo '[yes]';; esac", "[yes]"},
		{"a default's operator", "x=; echo \"[${x:\\\n-d}]\"", "[d]"},
		{"a trim's pattern", "x=a.b.c; echo \"[${x#\\\n*.}]\"", "[b.c]"},
		{"a trim's operator", "x=a.b.c; echo \"[${x%\\\n%.*}]\"", "[a]"},
		{"a replacement's pattern", "x=bab; echo \"[${x/\\\na/c}]\"", "[bcb]"},
		{"a length", "x=abc; echo \"[${#\\\nx}]\"", "[3]"},
		{"a nested expansion", "y=7; unset x; echo \"[${x-${y\\\n}}]\"", "[7]"},
		{"inside a command substitution", "x=5; echo \"[$(echo ${x\\\n})]\"", "[5]"},
		{"an unquoted here-document body", "x=5; cat <<E\n[${x\\\n}] [${x:\\\n-d}]\nE", "[5] [5]"},
		// And the controls: a quoted delimiter leaves the body alone, and a
		// quoted here-document inside a command substitution in an operand
		// keeps its own pair.
		{"a quoted here-document body", "x=5; cat <<'E'\n[${x\\\n}]\nE", "[${x\\\n}]"},
		{"a quoted here-document in an operand", "unset x; echo \"[${x-$(cat <<'E'\na\\\nb\nE\n)}]\"", "[a\\\nb]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src+"\necho after", nil)
			if want := tc.want + "\nafter"; strings.TrimSpace(out) != want {
				t.Errorf("%q printed %q, want %q", tc.src, strings.TrimSpace(out), want)
			}
		})
	}
}
