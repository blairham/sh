// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A plain word declared over a name that is really holding an array is
// `inconsistent type for assignment`, whichever declaration word writes it
// (#5074).
//
// `typeset`, `declare`, `readonly` and `export` all refused it. `local` did
// not, and took the declaration silently at 0, leaving a scalar where the
// script had asked for one over an array.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device.
//
// **The rule is keyed on the word doing the second declaration**, and the
// grid says so by holding the first fixed and moving the second — a grid over
// the first word alone agrees with a rule about `typeset` and is silent about
// the one that was broken.
func TestAPlainWordOverAnArrayIsRefusedWhicheverWordWritesIt(t *testing.T) {
	dir := t.TempDir()
	for _, second := range []string{"typeset v=x", "declare v=x", "local v=x", "readonly v=x", "export v=x"} {
		for _, first := range []string{"typeset -a v", "declare -a v", "local -a v"} {
			src := "(){ " + first + "; " + second + "; }\nprint -r -- after\n"
			t.Run(first+" then "+second, func(t *testing.T) {
				out, st := runZsh(t, dir, src)
				want := "(anon):" + declarationWord(second) + ": v: inconsistent type for assignment\n"
				if out != want || st != 1 {
					t.Errorf("out %q status %d, want %q at 1", out, st, want)
				}
			})
		}
	}
}

// declarationWord is the word a row's second declaration begins with, which
// is the word the refusal names.
func declarationWord(s string) string {
	for i := range len(s) {
		if s[i] == ' ' {
			return s[:i]
		}
	}
	return s
}

// The controls, and they are what bound the rule to a **kind change**.
//
// Re-declaring is not itself refused, the other direction is not refused, and
// a declaration with no value to assign is not refused. Each of these was
// already right before #5074 and has to stay right: a gate written to the
// failing rows alone would refuse all four.
func TestReDeclaringTheSameKindIsTakenAndSoIsTheOtherDirection(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"the same kind twice", `(){ local -a v; local -a v; }`, "st=0 after"},
		{"a scalar twice", `(){ local v=1; local v=2; }`, "st=0 after"},
		{"a compound over a scalar", `(){ local v=1; local -a v; }`, "st=0 after"},
		// A type letter names a numeric kind, which the gate exempts for
		// the reason its own doc gives.
		{"an integer letter over an array", `(){ local -a v; local -i v=3; }`, "st=0 after"},
		{"a float letter over an array", `(){ local -a v; local -F v=3; }`, "st=0 after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src + "\n" + `print -r -- "st=$? after"` + "\n"
			if out, st := runZsh(t, dir, src); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}

// And the scope, which is the half the existing gate's exemption is written
// for: a declaration that really does make a **new** binding writes a cell
// holding nothing, so there is no array under it to be inconsistent with.
//
// These rows are why the exemption stays as it is. The name is an array in an
// enclosing scope or in the shell, and the inner `local` is taken in each —
// which is what a gate that simply asked "is this name an array" would break.
func TestANewBindingOverAnOuterArrayIsTaken(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src string }{
		{"a nameless function inside one", `(){ local -a v; (){ local v=x; } }`},
		{"a named function called from one", `(){ local -a v; f(){ local v=x; }; f }`},
		{"an array the shell already held", "v=(1 2)\n(){ local v=x; }"},
		{"and one a `typeset` made at the top level", "typeset -a v\n(){ local v=x; }"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src + "\n" + `print -r -- "st=$? after"` + "\n"
			if out, st := runZsh(t, dir, src); out != "st=0 after\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, "st=0 after\n")
			}
		})
	}
}

// The refusal ends the script, and the frame it names is the one that made it.
//
// Both halves are measured rather than assumed: `f; print st=$?` writes
// nothing after the sentence, and a call made inside a subshell, an `eval` or
// a substitution ends only that — which is the ordinary reach of this shell's
// fatal errors and is what `typeset` already had.
func TestTheRefusalEndsTheScriptAndNamesTheFrame(t *testing.T) {
	const def = "f(){ local -a v; local v=x; }\n"
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a plain call ends the script", def + "f\nprint -r -- after\n", "f:local: v: inconsistent type for assignment\n"},
		{
			"a subshell contains it",
			def + "( f )\nprint -r -- after\n",
			"f:local: v: inconsistent type for assignment\nafter\n",
		},
		{
			"and so does an eval",
			def + "eval f\nprint -r -- after\n",
			"f:local: v: inconsistent type for assignment\nafter\n",
		},
		// A nameless function names the name it reports, which is the frame
		// that made the declaration and not the script.
		{
			"a nameless frame names itself",
			"(){ local -a v; local v=x; }\nprint -r -- after\n",
			"(anon):local: v: inconsistent type for assignment\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			wantStatus := 1
			if tc.want != "f:local: v: inconsistent type for assignment\n" &&
				tc.want != "(anon):local: v: inconsistent type for assignment\n" {
				wantStatus = 0
			}
			if out != tc.want || st != wantStatus {
				t.Errorf("out %q status %d, want %q at %d", out, st, tc.want, wantStatus)
			}
		})
	}
}
