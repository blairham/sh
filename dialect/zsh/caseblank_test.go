// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A `case` pattern may hold a blank when the arm opens with `(` (#1744).
//
// This shell matches such a pattern; the other five call the line a syntax
// error in four wordings at three statuses. Every row is a measurement on
// zsh 5.9.2 taken 2026-09-10 from a script file under `env -i`.
//
// The rows are matches rather than parses, because parsing is the cheap half:
// a reading that admitted the line and then dropped or collapsed the blanks
// would parse every row and match half of them. `(a b)` against `a  b` and
// `(a  b)` against `a  b` are the pair that says so.
func TestABlankInAParenthesizedCasePattern(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"the pattern matches its subject", `case 'a b' in (a b) printf hit;; (*) printf no;; esac`, "hit"},
		{"and misses a longer run of blanks", `case 'a  b' in (a b) printf hit;; (*) printf no;; esac`, "no"},
		{"which its own spelling matches", `case 'a  b' in (a  b) printf hit;; (*) printf no;; esac`, "hit"},
		{"and misses the blank removed", `case ab in (a b) printf hit;; (*) printf no;; esac`, "no"},
		{"a tab is not a space", "case 'a b' in (a\tb) printf hit;; (*) printf no;; esac", "no"},
		{"and matches a tab", "case $'a\tb' in (a\tb) printf hit;; (*) printf no;; esac", "hit"},
		{"a glob still globs across it", `case 'ax b' in (a* b) printf hit;; (*) printf no;; esac`, "hit"},

		// Blanks beside a separator are still separating, which is what
		// keeps `(a | b)` two alternatives here as it is everywhere.
		{"the separator still separates", `case 'a b' in (a | b) printf hit;; (*) printf no;; esac`, "no"},
		{"and each side of it is a whole pattern", `case a in (a | b) printf hit;; (*) printf no;; esac`, "hit"},
		{"a blank before the closing paren is dropped", `case 'a ' in (a ) printf hit;; (*) printf no;; esac`, "no"},
		{"as is one after the opening paren", `case 'a b' in ( a b ) printf hit;; (*) printf no;; esac`, "hit"},
		{"and that pattern misses the blanks kept", `case ' a b ' in ( a b ) printf hit;; (*) printf no;; esac`, "no"},

		// An alternation survives it on either side.
		{"an alternative may hold one", `case 'a b' in (a b|z) printf hit;; (*) printf no;; esac`, "hit"},
		{"and its neighbor need not", `case z in (a b|z) printf hit;; (*) printf no;; esac`, "hit"},
		{"a blank before the separator is dropped too", `case 'a b' in (a b |z) printf hit;; (*) printf no;; esac`, "hit"},

		// The failing line's own shape: a group, a blank, more pattern.
		{"a group and then more pattern", `case 'x y' in ((x) y) printf hit;; (*) printf no;; esac`, "hit"},
		{"the group still alternates inside it", `case 'q y' in ((x|q) y) printf hit;; (*) printf no;; esac`, "hit"},

		// The terminators are unaffected: the arm is an ordinary arm once
		// its pattern is read.
		{"the fallthrough terminator", `case 'a b' in (a b) printf hit;& (*) printf two;; esac`, "hittwo"},
		{"and the retry terminator", `case 'a b' in (a b) printf hit;| (*) printf two;; esac`, "hittwo"},

		// And the body is ordinary commands again, where a blank separates
		// words rather than joining them.
		{"the body gets its blanks back", `case 'a b' in (a b) printf '[%s]' one two;; esac`, "[one][two]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The paren is what licenses it and the position is not.
//
// `case 'a b' in a b) …` is “parse error near `b'“ in this shell too, so
// this is a rule about the parenthesized form rather than "a word may follow
// a pattern". Without this the flag would read as a much larger claim.
func TestOnlyAParenthesizedCaseListTakesTheBlank(t *testing.T) {
	if _, err := parseZsh(`case 'a b' in a b) echo hit;; esac`); err == nil {
		t.Error("an arm with no paren parsed, want the parse error that shell gives")
	}
}

// And an operator after the blanks is still an operator: `(a >b)` is
// “parse error near `>'“ there rather than a pattern with a space on it.
func TestAnOperatorStillEndsACasePattern(t *testing.T) {
	for _, src := range []string{
		`case x in (a >b) echo hit;; esac`,
		`case x in (a &b) echo hit;; esac`,
	} {
		if _, err := parseZsh(src); err == nil {
			t.Errorf("%s: parsed, want a parse error", src)
		}
	}
}
