// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A declaration utility's **appending** operand is read as an assignment, so
// its value is neither split into fields nor matched against the filesystem —
// exactly as `name=value` already was.
//
// It was neither until #3772. Runner.assignShaped wants a plain name in front
// of the `=` and `x+` is not one, so `typeset x+=$v` took the ordinary word
// route: field splitting and then pathname expansion. Where the declarations
// take the operator that is a **silent wrong value** — `v="a b"` left `a`
// behind at status 0 and nothing downstream could tell — and where they refuse
// the name it was a pathname refusal standing in for the name refusal.
//
// Measured 2026-09-19, `set -x; export x+=q*` in a directory holding a file
// named `x+=q1`, a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with
// standard input on the null device:
//
//	bash 5.3.20 	`+ export 'x+=q*'`  	read as an assignment
//	bash 3.2.57 	`+ export 'x+=q*'`  	read as an assignment
//	zsh 5.9.2   	`+ export x+='q*'`  	read as an assignment
//	ksh93u+     	`+ x+='q*'`         	read as an assignment
//	dash 0.5.12 	`+ export x+=q1`    	an ordinary word, matched
//	BusyBox ash 	`+ export 'x+=q1'`  	an ordinary word, matched
//
// The same split for field splitting: `v="a b"; export x+=$v` reaches BusyBox
// ash's trace as two words, `'x+=a' b`, and the other four as one. So the
// reading is the grammar's `name+=value` in operand position rather than a
// rule of its own, and syntax.Dialect.AppendAssign is the gate — which is why
// these are named for the flag and not for a shell.
//
// Whether the utility then *accepts* the name is the later and separate
// question Semantics.DeclarationTakesAnAppendOperand answers; it was answering
// about a word that had already arrived split.

// appendWordRun runs src with the appending operand taken, under a dialect
// that has the append operator or one that does not.
func appendWordRun(t *testing.T, src string, appendAssign bool, files ...string) (string, string, int) {
	t.Helper()
	set := func(sem *Semantics) {
		withAppendOperand(Yes)(sem)
		// A valueless declaration leaves the empty string, so that a second
		// name reaching the utility is visible at all: with the quiet answer
		// `typeset b` leaves `b` unset and a `${b-unset}` reads the same
		// whether the word split or not, which is a probe that would pass on
		// the bug.
		sem.DeclaredNameWithoutValueIsEmpty = Yes
	}
	return declRunWith(t, src, set, Diagnostics{}, nil, func(r *Runner) {
		d := syntax.Core()
		d.AppendAssign = appendAssign
		r.Dialect = &d
		for _, name := range files {
			if err := os.WriteFile(filepath.Join(r.Dir, name), nil, 0o600); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
	})
}

// TestAnAppendOperandIsNotSplit: the value keeps the blanks the expansion put
// in it, where an ordinary word would have become two fields.
//
// The second row is the discriminator that says the *word* moved rather than
// the builtin having become lenient: a second name on the same command shows
// the trailing field was never handed over as an operand of its own. Without
// it, `typeset x+=$v` alone leaves `x` holding `a` either way — the `b` is
// simply declared as a valueless second name and says nothing.
func TestAnAppendOperandIsNotSplit(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the value keeps its blank", `v="a b"; typeset x+=$v; echo "[$x]"`, "[a b]\n"},
		{
			"no second name is declared",
			`v="a b"; typeset x+=$v; echo "[$x][${b-unset}]"`,
			"[a b][unset]\n",
		},
		{"a quoted value is unchanged", `v="a b"; typeset x+="$v"; echo "[$x]"`, "[a b]\n"},
		{"it joins what the name holds", `x=p; v="a b"; typeset x+=$v; echo "[$x]"`, "[pa b]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := appendWordRun(t, tc.src, true)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q at 0", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// TestAnAppendOperandIsNotAPattern: the word is never matched against the
// filesystem, so the pattern survives into the value.
//
// The files are named for the **whole word** and not for the value half,
// because the whole word is what the ordinary route matched: `x+=a*` globbed
// to `x+=aone x+=atwo` and left `aoneatwo` in the name. A directory holding
// `aone` and `atwo` alone cannot tell the two readings apart — nothing matches
// `x+=a*` either way — which is the probe that would have passed on the bug.
func TestAnAppendOperandIsNotAPattern(t *testing.T) {
	out, errs, st := appendWordRun(t, `typeset x+=a*; echo "[$x]"`, true, "x+=aone", "x+=atwo")
	if want := "[a*]\n"; out != want || errs != "" || st != 0 {
		t.Errorf("typeset x+=a* = %q (stderr %q, status %d), want %q at 0", out, errs, st, want)
	}
}

// TestAnAppendOperandsValueTakesTheAssignmentsTildes: the value half is
// expanded as an assignment's, which is more than "not split and not matched"
// — a leading tilde and a tilde after each colon are expanded there and in no
// ordinary word.
//
// So the word is cut at its `=` with the `+` left on the name, rather than
// handed to the value expansion whole.
func TestAnAppendOperandsValueTakesTheAssignmentsTildes(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a leading tilde", `HOME=/hh; typeset x+=~/a; echo "[$x]"`, "[/hh/a]\n"},
		{"a tilde after a colon", `HOME=/hh; typeset x+=q:~/a; echo "[$x]"`, "[q:/hh/a]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := appendWordRun(t, tc.src, true)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q at 0", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// TestAnAppendOperandIsAnOrdinaryWordWithoutTheAppendGrammar is the other side
// of the flag, and it is what makes the three above statements about the gate
// rather than about the engine: with the operator absent from the grammar the
// same words split and match, which is what the two columns that have no `+=`
// do.
func TestAnAppendOperandIsAnOrdinaryWordWithoutTheAppendGrammar(t *testing.T) {
	out, errs, st := appendWordRun(t, `v="a b"; typeset x+=$v; echo "[$x][${b-unset}]"`, false)
	if want := "[a][]\n"; out != want || errs != "" || st != 0 {
		t.Errorf("split reading = %q (stderr %q, status %d), want %q at 0", out, errs, st, want)
	}
	out, errs, st = appendWordRun(t, `typeset x+=a*; echo "[$x]"`, false, "x+=aone", "x+=atwo")
	if want := "[aoneatwo]\n"; out != want || errs != "" || st != 0 {
		t.Errorf("matched reading = %q (stderr %q, status %d), want %q at 0", out, errs, st, want)
	}
}

// TestOnlyADeclarationUtilitysAppendOperandIsReadAsOne: the reading belongs to
// the utility and not to the shape, so the identical word in front of any
// other command splits and matches like the word it is.
func TestOnlyADeclarationUtilitysAppendOperandIsReadAsOne(t *testing.T) {
	out, errs, st := appendWordRun(t, `v="a b"; echo x+=$v`, true)
	if want := "x+=a b\n"; out != want || errs != "" || st != 0 {
		t.Errorf("echo x+=$v = %q (stderr %q, status %d), want %q at 0", out, errs, st, want)
	}
	out, errs, st = appendWordRun(t, `printf "[%s]" x+=a*; echo`, true, "x+=aone", "x+=atwo")
	if want := "[x+=aone][x+=atwo]\n"; out != want || errs != "" || st != 0 {
		t.Errorf("printf x+=a* = %q (stderr %q, status %d), want %q at 0", out, errs, st, want)
	}
}
