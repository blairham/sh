// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a `local` of one half of a tie means, from both ends and for both
// kinds of tie. See interp/tielocal.go for where each row was measured; the
// tests here name the seam and the letter and never a shell.
//
// Runner.Tie is the dialect's seam — a tie the *shell* arrives with — and
// `typeset -T` is the script's letter. The two get different answers, so
// every row below says which one it is asking about.

// A local of the scalar half of one of the shell's own ties displaces the
// array half with it, and the caller gets both back. Saving the name written
// and not the pair left a function-local search path in the caller for the
// rest of the script (#1630).
func TestALocalOfAShellsOwnTieShadowsBothHalves(t *testing.T) {
	out, errs, st := declRunTied(t, `S=one:two
f() { local S=zzz; echo "in=[$S][${s[@]}]"; }
f
echo "after=[$S][${s[@]}]"`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "in=[zzz][zzz]\nafter=[one:two][one two]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a local of a shell tie's scalar half = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// And from the array end, which is the direction `compaudit`'s first line
// takes: the local array is a fresh one holding nothing, not the caller's
// elements, and the scalar half is displaced and empty beside it.
func TestALocalOfAShellsOwnTiesArrayHalfIsAFreshEmptyArray(t *testing.T) {
	out, errs, st := declRunTied(t, `S=one:two
f() { local -a s; echo "in=[${#s[@]}][${s[*]}][$S]"; }
f
echo "after=[${#s[@]}][$S]"`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "in=[0][][]\nafter=[2][one:two]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a local of a shell tie's array half = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The array half is an array whatever the declaration says, because the kind
// belongs to the shell's parameter and not to the line: without `-a` the
// answer is the same. A declaration that set the cell to an empty *string*
// instead answers this with the caller's elements still in view, since they
// live in a table of their own.
func TestALocalOfAShellsOwnTiesArrayHalfIsAnArrayWithoutTheLetter(t *testing.T) {
	out, errs, st := declRunTied(t, `S=one:two
f() { local s; echo "in=[${#s[@]}][${s[*]}][$S]"; }
f`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "in=[0][][]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a local of a shell tie's array half without -a = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The two halves are not emptied the same way, and this is the row that says
// so. A valueless local of the *scalar* half sets a string, and the mirror
// splits that string into the one field it has — so the array holds one empty
// element where the row above holds none.
func TestALocalOfAShellsOwnTiesScalarHalfSplitsTheEmptyString(t *testing.T) {
	out, errs, st := declRunTied(t, `S=one:two
f() { local S; echo "in=[$S][${#s[@]}][${s[1]-NONE}]"; }
f`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "in=[][1][]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a valueless local of a shell tie's scalar half = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The control every row above is only meaningful against: nothing here
// suppresses the tie itself. A write with no local in front of it goes on
// mirroring, and so does one *inside* the local — the pair the function is
// holding is still a pair.
func TestATieGoesOnMirroringInsideAndOutsideALocal(t *testing.T) {
	out, errs, st := declRunTied(t, `S=one:two
echo "top=[${s[@]}]"
f() { local S=zzz; s=(p q); echo "in=[$S][${s[@]}]"; }
f
echo "after=[$S][${s[@]}]"`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "top=[one two]\nin=[p:q][p q]\nafter=[one:two][one two]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a tie around a local = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// A tie a *script* made is the other answer: `local` makes a new parameter,
// which is simply not tied, and the other half goes on naming the outer cell.
// A pair-shadow applied to every tie would answer this `[zzz][zzz]`.
func TestALocalOfAScriptsTieIsAnOrdinaryLocal(t *testing.T) {
	out, errs, st := declRun(t, `typeset -T S s
S=one:two
f() { local S=zzz; echo "in=[$S][${s[@]}]"; }
f
echo "after=[$S][${s[@]}]"`, withHidingAndTies, Diagnostics{})
	want := "in=[zzz][one two]\nafter=[one:two][one two]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a local of a script tie's scalar half = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// And a script's tie is untouched by a local of its *array* half in the same
// way: the scalar keeps the outer value rather than being emptied with it.
func TestALocalOfAScriptsTieArrayHalfLeavesTheScalarAlone(t *testing.T) {
	out, errs, st := declRun(t, `typeset -T S s
S=one:two
f() { local -a s; echo "in=[$S]"; }
f
echo "after=[$S][${s[@]}]"`, withHidingAndTies, Diagnostics{})
	want := "in=[one:two]\nafter=[one:two][one two]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a local of a script tie's array half = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// A script's tie is out of effect only in a scope *deeper* than the one it
// was made in. Its own declaration shadows both halves before recording it,
// so a rule that asked "is either half shadowed anywhere" turned every
// function-local tie off the moment it was made.
func TestATieMadeInAFunctionIsNotDetachedByItsOwnDeclaration(t *testing.T) {
	out, errs, st := declRun(t, `f() {
  typeset -T S s
  S=one:two
  echo "own=[${s[@]}]"
  g
  echo "back=[${s[@]}]"
}
g() { local S=zzz; echo "deeper=[${s[@]}]"; }
f`, withHidingAndTies, Diagnostics{})
	want := "own=[one two]\ndeeper=[one two]\nback=[one two]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a function-local tie = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// A function called from inside the local sees the local pair, both halves of
// it, and the caller's pair is back the moment the outer call returns.
func TestAShellsOwnTieShadowedByALocalIsWhatACalleeSees(t *testing.T) {
	out, errs, st := declRunTied(t, `S=one:two
g() { echo "g=[$S][${s[@]}]"; }
f() { local S=zzz; g; }
f
g`, withHidingAndTies, Diagnostics{}, [2]string{"S", "s"})
	want := "g=[zzz][zzz]\ng=[one:two][one two]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a callee under a shadowed tie = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// A write to the *other* half from inside the local reaches the outer cell,
// because a script's tie leaves that half naming it. This is the row that
// makes the answer above more than bookkeeping: shadowing the partner anyway
// — the rule the shell's own pairs follow — takes the write back when the
// function returns, and both rows above pass either way.
//
// The array is what the row asserts, and the scalar is where this engine's
// one variable table shows: the shell being copied mirrors the write into the
// outer `S` as well, and a stack of saved values has no way to name a cell a
// shadow is standing over. It is the same approximation `-h` already carries
// — see the comment on hidesItsTie.
func TestAWriteToTheOtherHalfOfAScriptsTieReachesTheCaller(t *testing.T) {
	out, errs, st := declRun(t, `typeset -T S s
S=one:two
f() { local S=zzz; s=(p q); }
f
echo "after=[${s[@]}]"`, withHidingAndTies, Diagnostics{})
	want := "after=[p q]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a write to the other half of a script tie = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}
