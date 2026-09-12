// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// unsetSem is the permissive base with one letter set answered, which is the
// only axis these cases turn on.
func unsetSem(letters string) Semantics {
	s := permissive()
	s.UnsetOptions = letters
	// `unset` is a special builtin and a bad option to one ends the script
	// in two of the four. These cases are about which letters are *there*,
	// so the shell has to still be running to say what the value is.
	s.BadOptionToSpecialBuiltinFatal = No
	// `-n` on a name that is not a reference is its own axis (#932) and not
	// this one. Answered Yes here so a case about the *letter* still sees the
	// name go; what the letter does to such a name is
	// TestUnsetReferenceLetterOnANameThatIsNotOne's question.
	s.UnsetReferenceLetterRemovesANonReference = Yes
	return s
}

// TestUnsetMatchesNamesAgainstAPattern — `unset -m` reads its operands as
// patterns and removes every parameter whose *name* one matches. One shell's
// alone, and a prompt theme clears its whole namespace with it.
func TestUnsetMatchesNamesAgainstAPattern(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Anchored at both ends of the name, like every other pattern here.
		{`x=1 xy=2 z=3; unset -m "x*"; echo "[${x-gone}][${xy-gone}][${z-gone}]"`, "[gone][gone][3]\n"},
		{`x=1 xy=2; unset -m "x"; echo "[${x-gone}][${xy-gone}]"`, "[gone][2]\n"},
		// Several patterns in one command, and one that matches nothing,
		// which is not an error.
		{`a=1 b=2 c=3; unset -m "a" "b" "zz*"; echo "st=$? [${a-gone}][${b-gone}][${c-gone}]"`, "st=0 [gone][gone][3]\n"},
		// Arrays are parameters too.
		{`a=(p q); unset -m "a"; echo "[${a[@]-gone}]"`, "[gone]\n"},
		// A pattern is not a name, and is not held to a name's rules: this
		// would be refused as an operand without the letter.
		{`ab=1; unset -m "a[b]"; echo "[${ab-gone}]"`, "[gone]\n"},
	} {
		out, st := run(t, tc.src, withSem(unsetSem("vfm")))
		if out != tc.want || st != 0 {
			t.Errorf("%s: out %q status %d, want %q", tc.src, out, st, tc.want)
		}
	}
	// A name that arrived in the environment is a parameter too, and the
	// tables a lookup reads are not the one table: `unset` already had to
	// learn that for a name, and a pattern has to walk the same three.
	out, st := run(t, `unset -m "PAT*"; echo "[${PATH-gone}]"`, withSem(unsetSem("vfm")))
	if !strings.Contains(out, "[gone]") || st != 0 {
		t.Errorf("out %q status %d, want an environment name matched too", out, st)
	}
	// Nothing matching at all is 1 rather than 0 — measured on zsh 5.9.2,
	// which is the only shell with the letter: a removal that removed
	// nothing has not done what it was asked. This reported success, so a
	// script clearing a namespace could not tell an empty one from a typo.
	out, st = run(t, `unset -m "zz*"; echo "st=$?"`, withSem(unsetSem("vfm")))
	if out != "st=1\n" {
		t.Errorf("out %q status %d, want 1 with nothing matched", out, st)
	}
	// With nothing to match, the complaint is the dialect's and not this
	// package's: a dialect that says nothing when `unset` has nothing to
	// unset is three of the panel, and one that complains is zsh. Both
	// halves are asserted, because a wording read from a permissive base
	// would pass either way (#1386).
	out, st = run(t, `unset -m; echo "st=$?"`, withSem(unsetSem("vfm")))
	if out != "st=0\n" {
		t.Errorf("out %q status %d, want silence at 0 where the dialect words nothing", out, st)
	}
	speaks := func(r *Runner) {
		withSem(unsetSem("vfm"))(r)
		r.Diagnostics = &Diagnostics{UnsetNoOperands: "%[1]s: not enough arguments"}
	}
	for _, src := range []string{`unset -m`, `unset`, `unset -v`, `unset -f`} {
		out, st = run(t, src+`; echo "st=$?"`, speaks)
		if !strings.Contains(out, "unset: not enough arguments\n") || !strings.Contains(out, "st=1\n") {
			t.Errorf("%s: out %q status %d, want the one complaint for every spelling", src, out, st)
		}
	}
}

// TestUnsetsLettersAreTheDialects — the panel splits three ways here and no
// two members have the same set, so the letters cannot be one list in the
// substrate. Measured 2026-09-05: `-v` and `-f` are unanimous, `-n` is bash
// 5.3's and ksh93's, and `-m` is zsh's alone.
func TestUnsetsLettersAreTheDialects(t *testing.T) {
	for _, tc := range []struct {
		letters, src, want string
		gone               bool
	}{
		// The letter the dialect has: accepted, and it does its work.
		{"vfm", `x=1; unset -m "x*"; echo "[${x-gone}]"`, "[gone]", true},
		// The same letter where the dialect does not: refused, and the
		// value survives, which is the half a silently-ignored option would
		// get wrong in the other direction.
		{"vfn", `x=1; unset -m "x*"; echo "[${x-gone}]"`, "[1]", false},
		{"vf", `x=1; unset -n x; echo "[${x-gone}]"`, "[1]", false},
		{"vfn", `x=1; unset -n x; echo "[${x-gone}]"`, "[gone]", true},
	} {
		out, _ := run(t, tc.src, withSem(unsetSem(tc.letters)))
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s with letters %q: out %q, want %q in it", tc.src, tc.letters, out, tc.want)
		}
		if refused := strings.Contains(out, "invalid option"); refused == tc.gone {
			t.Errorf("%s with letters %q: out %q, refusal and effect disagree", tc.src, tc.letters, out)
		}
	}
	// Empty means the two POSIX letters and no more, which is the answer a
	// dialect that has said nothing gets.
	out, _ := run(t, `x=1; unset -n x; echo "[${x-gone}]"`, withSem(unsetSem("")))
	if !strings.Contains(out, "[1]") {
		t.Errorf("out %q, want `-n` refused where the dialect named no letters", out)
	}
	// And the *specification's* answer, read from the preset rather than
	// written here: POSIX gives `unset` `-v` and `-f` and nothing more, so
	// `-n` is refused and `-f` is not. Two assertions rather than one,
	// because a preset that named too many letters and one that named none
	// both leave the first alone.
	base := permissive()
	base.BadOptionToSpecialBuiltinFatal = No
	base.UnsetReferenceLetterRemovesANonReference = Yes
	out, _ = run(t, `x=1; unset -n x; echo "[${x-gone}]"`, withSem(base))
	if !strings.Contains(out, "[1]") {
		t.Errorf("out %q, want `-n` refused by the POSIX letter set", out)
	}
	out, _ = run(t, `f() { :; }; unset -f f; f; echo "st=$?"`, withSem(base))
	if strings.Contains(out, "invalid option") {
		t.Errorf("out %q, want `-f` among the POSIX letters", out)
	}
}

// TestAtFunctionReturnRunsWhenTheCallUnwinds — the seam a dialect needs for
// state the substrate does not hold, which is what `emulate -L` restores
// through.
//
// Asserted on the *order* as well as the fact, because the closures unwind
// like a defer stack and a caller that registered two of them is relying on
// it; and at the top level, where there is no scope and the answer is false
// rather than an error.
func TestAtFunctionReturnRunsWhenTheCallUnwinds(t *testing.T) {
	var marks []string
	var insideOK, outsideOK bool
	out, st := run(t, "f() { mark one; mark two; echo in; }\nmark\nf\necho after", func(r *Runner) {
		r.Register("mark", func(r *Runner, _ context.Context, args []string) int {
			if len(args) == 0 {
				outsideOK = !r.AtFunctionReturn(func() { marks = append(marks, "top") })
				return 0
			}
			name := args[0]
			insideOK = r.AtFunctionReturn(func() { marks = append(marks, name) })
			return 0
		})
	})
	if st != 0 || out != "in\nafter\n" {
		t.Fatalf("out %q status %d, want the function and the line after it", out, st)
	}
	if !outsideOK {
		t.Error("at the top level AtFunctionReturn should report false, not register anything")
	}
	if !insideOK {
		t.Error("inside a function AtFunctionReturn should report true")
	}
	if got := strings.Join(marks, ","); got != "two,one" {
		t.Errorf("marks = %q, want %q — the last registered runs first", got, "two,one")
	}
}
