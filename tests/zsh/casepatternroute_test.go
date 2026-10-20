// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// A pattern this shell will not compile, standing as a `case` arm, leaves
// **nought** behind it. From a command string that is the shell's exit status;
// from a script file the shell overwrites it with 1 — and a boundary that
// *copies* the shell does not, so a subshell or a command substitution hands
// its caller the nought on either route (#4765, #4803).
//
// Measured 2026-09-27 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) run `-f`, both
// streams discarded and `$?` taken immediately. The standard-input route was
// measured with the same file on `-s` and answers as the file does.
//
// The two shapes in the first pair are **different verdicts** — a plain
// unterminated bracket, and a bracket a `[:name:]` left open, which no scan
// for the first can see — and they move together, which is what says the arm
// decides rather than either scan.
func TestARefusedCaseArmPatternLeavesNought(t *testing.T) {
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

		// A boundary that copies the shell hands the nought on, on either
		// route — which is what makes the nought the refusal's own number
		// rather than the command string's. The file column is the row that
		// says it: the same program at the top level is 1 there.
		{"a subshell", `( case '[a' in ([a) :;; esac )`, 0, 0},
		{"a command substitution", `v=$( case '[a' in ([a) :;; esac )`, 0, 0},
		{"a subshell inside a function", `f() { ( case '[a' in ([a) :;; esac ); }; f`, 0, 0},
		{"a subshell inside a subshell", `( ( case '[a' in ([a) :;; esac ) )`, 0, 0},
		{"a function called inside a subshell", `f() { case '[a' in ([a) :;; esac; }; ( f )`, 0, 0},

		// And the controls at the same boundary: every other give-up carries
		// its own status across it, and borrowed text inside one still
		// reports rather than copies. Without these the subshell rows would
		// read the same for a shell that simply lost a status at `( … )`.
		{"a glob in a subshell", `( print -rl -- [a )`, 1, 1},
		{"a condition in a subshell", `( [[ '[a' == [a ]] )`, 2, 2},
		{"a selection pattern in a subshell", `( typeset -m '[a' )`, 1, 1},
		{"a division by zero in a subshell", `( print $((1/0)) )`, 1, 1},
		{"borrowed text in a subshell", `( eval 'case "[a" in ([a) :;; esac' )`, 1, 1},

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

// And the caller runs on, which is the half an exit-status table cannot show:
// the refusal ends the *subshell*, the shell that started it carries on, and
// what it is handed is the nought.
//
// The control beside it is the same boundary around a give-up that is not a
// `case` arm, where the caller is handed 1 and runs on just the same — so the
// difference between the two rows is the number and nothing else.
func TestTheCallerOfASubshellRunsOnFromARefusedCaseArm(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a subshell", `( case '[a' in ([a) :;; esac ) 2>/dev/null; print "after=$?"; print "on"`, "after=0\non\n"},
		{"a command substitution", `v=$( case '[a' in ([a) :;; esac ) 2>/dev/null; print "after=$?"; print "on"`, "after=0\non\n"},
		{"a glob, which is the control", `( print -rl -- [a ) 2>/dev/null; print "after=$?"; print "on"`, "after=1\non\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, route := range []interp.Route{interp.RouteCommandString, interp.RouteScriptFile} {
				out, st := runZshRoute(t, t.TempDir(), c.src, route)
				if out != c.want || st != 0 {
					t.Errorf("%s on %v = %q (status %d), want %q", c.src, route, out, st, c.want)
				}
			}
		})
	}
}
