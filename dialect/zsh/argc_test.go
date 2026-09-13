// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `$ARGC` is `$#` under this shell's own name, and it is *produced* rather
// than stored: it moves with `shift`, with a function call, and with the
// return from one.
//
// Measured 2026-09-10 against zsh 5.9.2 with `-f`. The shift in the middle is
// what makes this discriminating — a shell that filled the name in once, when
// the function was entered, answers `[3][2][2][3]` here and passes any test
// that reads the count only on the way in.
func TestARGCCountsThePositionalParameters(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `set -- a b c
print -r -- "[$ARGC]"
f() { print -r -- "[$ARGC]"; shift; print -r -- "[$ARGC]" }
f x y
print -r -- "[$ARGC]"`)
	want := "[3]\n[2]\n[1]\n[3]\n"
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
}

// The shape #1682 was filed for, reduced to the four lines that matter.
//
// Every powerlevel10k configuration ends with `p10k reload`, and that
// function's dispatch opens by asking whether it was called with anything at
// all. An unset name is zero in arithmetic and `!0` is one, so a shell without
// `$ARGC` takes *every* call as a call with no arguments — which is thirteen
// lines of help onto a startup, at status 1, with the word it was handed
// sitting in the list it just printed. Both halves are here because only the
// pair is discriminating: a shell that answered `usage` to neither would have
// broken the case the guard exists for.
func TestARGCDecidesADispatch(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f() { if (( !ARGC )); then print -r -- usage; return 1; fi; print -r -- "ran $1" }
f reload
print -r -- "st=$?"
f
print -r -- "st=$?"`)
	want := "ran reload\nst=0\nusage\nst=1\n"
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
}

// Readonly, which is measured rather than defensive: zsh answers `ARGC=9`
// with `read-only variable: ARGC` and stops, so the print below it never runs.
//
// It is also what keeps the producer from being shadowed. A produced scalar
// with no writer takes an assignment into the stored table, and a stored value
// is what a read finds first — so without the mark this would be accepted in
// silence and `$ARGC` would report nine for three parameters from then on,
// which is exactly the reading the dispatch above turns on.
func TestARGCRefusesAssignment(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `set -- a b c
ARGC=9
print -r -- "unreached [$ARGC][$#]"`)
	if !strings.Contains(out, "read-only variable: ARGC") || st != 1 {
		t.Errorf("got %q status %d, want the readonly refusal at 1", out, st)
	}
	if strings.Contains(out, "unreached") {
		t.Errorf("got %q, want the assignment to have ended the script", out)
	}
}

// And zero where there are no positional parameters, which is the value the
// dispatch above is entitled to read as `no arguments`. A shell that left the
// name unset would answer the empty string here and still pass the arithmetic
// half of every case above, because arithmetic reads an unset name as zero.
func TestARGCIsZeroWithNoParameters(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -r -- "[$ARGC]"`)
	if out != "[0]\n" || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, "[0]\n")
	}
}

// And silent to the `-p` listings while the bare ones still write it.
//
// Measured 2026-09-12, zsh 5.9.2, `env -i PATH=/usr/bin:/bin` with a scratch
// HOME. Six forms, one run, one name:
//
//	typeset -p ARGC   nothing, status 0
//	typeset -p        no row
//	readonly -p       no row
//	typeset -r        ARGC=0
//	readonly          ARGC=0
//	typeset           integer 10 readonly ARGC=0
//
// So the silence belongs to the **`-p` word** and not to the name, which is
// why it is asked of the word rather than of the shape that came back: two
// dialects give the bare and the `-p` listing one shape, and gating on the
// shape would have taken the name out of the bare one here.
//
// The readonly mark above is what put the name into any of these: a produced
// parameter is in none of the tables a listing walks and reaches one only
// through an attribute. So this is the second half of that mark rather than a
// rule of its own, and this shell wrote `typeset -r ARGC=0` for all six
// before it (#2518).
//
// The three forms are asserted in one run on purpose. A mark that suppressed
// the name everywhere passes the first three and loses the last three, and a
// name suppressed nowhere is the state this fixes — only both halves together
// say which.
func TestARGCIsSilentToDashPAndPresentToTheBareListings(t *testing.T) {
	t.Run("every -p form writes no row and reports 0", func(t *testing.T) {
		out, st := runZsh(t, t.TempDir(), `typeset -p ARGC
print -r -- "named=$?"
typeset -p
print -r -- "wholep=$?"
readonly -p
print -r -- "readonlyp=$?"`)
		if st != 0 {
			t.Errorf("status %d, want 0", st)
		}
		for _, mark := range []string{"named=0\n", "wholep=0\n", "readonlyp=0\n"} {
			if !strings.Contains(out, mark) {
				t.Errorf("got %q, want %q", out, mark)
			}
		}
		// `ARGC=0` rather than the bare name: the scratch directory this
		// test runs in is named after the test, so `PWD` and `OLDPWD` carry
		// the four letters and a search for them finds the harness.
		if strings.Contains(out, "ARGC=0") {
			t.Errorf("got %q, want the name absent from all three", out)
		}
	})
	// The half a gate on the listing's *shape* would lose. A bare `readonly`
	// and `readonly -p` come back in two different shapes in most dialects
	// and in one shape here, so only the word tells them apart — and this is
	// the form zsh writes the name in.
	for _, word := range []string{"readonly", "typeset -r"} {
		t.Run("the bare `"+word+"` writes it", func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), word)
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if !strings.Contains(out, "ARGC=0\n") {
				t.Errorf("got %q, want `ARGC=0` — the silence is the `-p` "+
					"word's and must not reach a bare listing", out)
			}
		})
	}
}
