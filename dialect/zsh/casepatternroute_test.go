// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// A pattern this shell will not compile, standing as a `case` arm, ends the
// shell at **nought** from a command string and at 1 from a script file.
//
// Measured 2026-09-27 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) run `-f`, both
// streams discarded and `$?` taken immediately. The standard-input route was
// measured with the same file on `-s` and answers as the file does (#4765).
//
// The two shapes in the first pair are **different verdicts** — a plain
// unterminated bracket, and a bracket a `[:name:]` left open, which no scan
// for the first can see — and they move together, which is what says the
// route decides rather than either scan.
func TestARefusedCaseArmPatternEndsACommandStringAtNought(t *testing.T) {
	for _, c := range []struct {
		name, src string
		wantC     int
		wantFile  int
	}{
		{"an unterminated bracket", `case '[a' in ([a) print ONE;; (*) print STAR;; esac`, 0, 1},
		{"a bracket a class left open", `case zzz in (x[[:alpha:]) print ONE;; (*) print STAR;; esac`, 0, 1},
		{"inside a function, which is the same shell", `f() { case '[a' in ([a) :;; esac; }; f`, 0, 1},

		// The controls. A condition's operand has a status of its own and
		// keeps it on both routes, which is what says the refusal is
		// reaching the right number where it is asked for one; and every
		// other pattern surface is the ordinary fatal status on both, which
		// is what says this is the `case` arm's row.
		{"a condition holding the first pattern", `[[ '[a' == [a ]]`, 2, 2},
		{"a condition holding the second", `[[ zzz == x[[:alpha:] ]]`, 2, 2},
		{"a glob", `print -r -- "[a"*`, 1, 1},
		{"a substitution's pattern", `v='[a'; print -r -- X${v#[a}Y`, 1, 1},
		{"an element filter", `v=(one two); print -r -- "${v:#[a}"`, 1, 1},
		{"a selection pattern", `typeset -m '[a'`, 1, 1},
		{"a pattern written as a word", `print -rl -- [a`, 1, 1},
		{"borrowed text, which is a boundary of its own", `eval 'case "[a" in ([a) :;; esac'`, 1, 1},

		// And an arm that matches before the bad one is never compiled, so
		// there is no refusal to have a status — the row that says the
		// refusal happens at the arm as it is tried.
		{"an earlier arm matches", `case '[a' in (*) print STAR;; ([a) print ONE;; esac`, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, st := runZshRoute(t, t.TempDir(), c.src, interp.RouteCommandString); st != c.wantC {
				t.Errorf("%s from a command string = %d, want %d", c.src, st, c.wantC)
			}
			if _, st := runZshRoute(t, t.TempDir(), c.src, interp.RouteScriptFile); st != c.wantFile {
				t.Errorf("%s from a script file = %d, want %d", c.src, st, c.wantFile)
			}
		})
	}
}
