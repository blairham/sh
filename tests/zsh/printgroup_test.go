// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A printed pattern group still matches where the group is the *word's* and
// not a condition's operand (#1221).
//
// The behavioral half, and the half that says what the bug cost: the printer
// escaped the group, so the printed source parsed, ran, exited 0 and answered
// the opposite question. `TestAPrintedPatternOperandStillMatches` beside this
// is the same check for the condition route, which was the one route that was
// already right — so these are the rows it could not have caught.
//
// Measured on zsh 5.9.2, 2026-09-07, each probe in a script file of its own
// under `env -i` with a scratch HOME, in a directory holding `v5` and `v6`,
// every file checked with `od -c` because the whole question is backslashes:
//
//	echo (v5|v6)          v5 v6      the escaped form it printed:  (v5|v6)
//	case … ((a|b)) …      hit        the escaped form it printed:  miss
//
// So the shell agreed that what was printed meant something else, in both
// rows, which is what made this the printer's bug rather than the parser's.
func TestAPrintedPatternGroupStillMatchesOutsideACondition(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// An argument, where the group is matched against filenames.
			// The files are made by the snippet, which is what lets each
			// run have a directory of its own — the row carries its whole
			// world with it rather than depending on what ran before.
			"an argument that globs",
			`: > w5; : > w6; : > w7; echo (w5|w6)`,
			"w5 w6",
		},
		{
			// And where it matches nothing, the group's text is what the
			// refusal names — which is the row that says the printed form
			// is *read* as a pattern and not as a filename with
			// parentheses in it.
			"an argument matching nothing is refused as a pattern",
			`echo (zz1|zz2)`,
			"zsh:1: no matches found: (zz1|zz2)",
		},
		// A `case` arm, with the arm's own paren in front of the group and
		// without it: two readings of the same characters and one printed
		// form.
		{"an arm behind the arm's paren", `k=a; case $k in ((a|b)) echo hit;; *) echo miss;; esac`, "hit"},
		{"an arm whose paren is the group's", `k=b; case $k in (a|b)) echo hit;; *) echo miss;; esac`, "hit"},
		{"and the arm still misses what it should", `k=c; case $k in ((a|b)) echo hit;; *) echo miss;; esac`, "miss"},
		// A group behind pattern text, which the issue's title did not
		// cover and which took the same escape.
		{"a group behind pattern text", `k=x.zip; case $k in *.(zip|tgz)) echo hit;; *) echo miss;; esac`, "hit"},
		{
			"a group in a loop's item list",
			`: > ac1; : > bc1; for x in (a|b)c1; do echo "[$x]"; done`,
			"[ac1]\n[bc1]",
		},
		// And the other direction, which a fix that simply stopped escaping
		// parentheses would break: a parenthesis the script protected is a
		// parenthesis, and printing has to keep it one.
		{"a protected parenthesis stays literal", `case '(a|b)' in \(a\|b\)) echo hit;; *) echo miss;; esac`, "hit"},
		{"and does not become a group", `case a in \(a\|b\)) echo hit;; *) echo miss;; esac`, "miss"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, zsh.Dialect())
			if err != nil {
				t.Fatalf("%s: %v", tc.src, err)
			}
			printed := syntax.Print(f)
			// Both are run, because a row proves nothing unless the
			// original answers what the table says it does: a snippet that
			// says `miss` for the wrong reason would pass a check on the
			// printed form alone.
			before, _ := answersRun(t, tc.src)
			after, _ := answersRun(t, printed)
			if got := strings.TrimSpace(before); got != tc.want {
				t.Fatalf("%s said %q, want %q — the row is wrong, not the printer", tc.src, got, tc.want)
			}
			if got := strings.TrimSpace(after); got != tc.want {
				t.Errorf("%s printed as %q, which said %q, want %q", tc.src, printed, got, tc.want)
			}
		})
	}
}
