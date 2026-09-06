// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `typeset -T SCALAR array [sep]`, measured against zsh 5.9.2 (2026-09-06).
// One shell's letter: bash refuses `-T` outright and ksh93's `-T` declares a
// *type*, so nothing here is an axis.

// The whole of the tie, in both directions, and it has to be both: a
// one-directional mirror passes half of these.
func TestATieReflectsInBothDirections(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -T SCA sca
SCA=a:b:c
print -r -- "1 arr=[${(j:|:)sca}] n=${#sca}"
sca=(x y z)
print -r -- "2 SCA=[$SCA]"
sca+=(w)
print -r -- "3 SCA=[$SCA]"
SCA=p:q
print -r -- "4 arr=[${(j:|:)sca}]"
sca[1]=Z
print -r -- "5 SCA=[$SCA]"`)
	want := "1 arr=[a|b|c] n=3\n2 SCA=[x:y:z]\n3 SCA=[x:y:z:w]\n4 arr=[p|q]\n5 SCA=[Z:q]\n"
	if out != want || st != 0 {
		t.Errorf("a tie = %q (status %d), want %q", out, st, want)
	}
}

// The separator is the third operand, and it is used both ways.
func TestATieUsesTheSeparatorItWasGiven(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -T S2 s2 '#'
S2=a#b#c
print -r -- "split=[${(j:|:)s2}]"
s2=(1 2)
print -r -- "join=[$S2]"`)
	want := "split=[a|b|c]\njoin=[1#2]\n"
	if out != want || st != 0 {
		t.Errorf("a separator = %q (status %d), want %q", out, st, want)
	}
}

// **`unset` of either half unsets both.** Half a tie is not a state this
// shell has, and the tie itself goes too — a later assignment to the scalar
// is a plain scalar again.
func TestUnsettingEitherHalfUnsetsBoth(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -T U1 u1
U1=a:b
unset U1
print -r -- "1 n=${#u1} set=[${u1+SET}]"
typeset -T U2 u2
U2=a:b
unset u2
print -r -- "2 U2=[${U2-UNSET}] set=[${U2+SET}]"
typeset -T U3 u3
U3=a:b
unset U3
U3=c:d
print -r -- "3 untied n=${#u3} U3=[$U3]"`)
	want := "1 n=0 set=[]\n2 U2=[UNSET] set=[]\n3 untied n=0 U3=[c:d]\n"
	if out != want || st != 0 {
		t.Errorf("unsetting a tie = %q (status %d), want %q", out, st, want)
	}
}

// A value on the declaration, on either half. The array's arrives by the
// ordinary operand-assignment path and the scalar's is part of the word.
func TestATieTakesAValueOnEitherHalf(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -T B1 b1=(one two)
print -r -- "1 B1=[$B1] b1=[${(j:|:)b1}]"
typeset -T B2=x:y b2
print -r -- "2 st=$? b2=[${(j:|:)b2}]"`)
	want := "1 B1=[one:two] b1=[one|two]\n2 st=0 b2=[x|y]\n"
	if out != want || st != 0 {
		t.Errorf("a declared value = %q (status %d), want %q", out, st, want)
	}
}

// A name that already holds a value keeps it and is read back through the tie
// that has just arrived — the same rule the integer and case attributes
// follow, and what makes `PATH=…; typeset -T PATH path` fill `path` rather
// than emptying both.
func TestATieReadsBackAValueTheScalarAlreadyHeld(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `V=one:two:three
typeset -T V v
print -r -- "n=${#v} [${(j:|:)v}] V=[$V]"`)
	want := "n=3 [one|two|three] V=[one:two:three]\n"
	if out != want || st != 0 {
		t.Errorf("a standing value = %q (status %d), want %q", out, st, want)
	}
}

// The five refusals, each in this shell's words — and the fatality is *not*
// the same for all of them, which is measured rather than tidied: tying a
// name to itself and naming something that is not a name both end the
// script, while the other three are reported and run on.
func TestTheTieRefusals(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -T X 2>&1
print -r -- "1 st=$?"
typeset -T S s
typeset -T S other 2>&1
print -r -- "2 st=$?"
typeset -T S s
print -r -- "3 same-again st=$?"
typeset -T Q q=plain 2>&1
print -r -- "4 st=$?"
print -r -- "reached"`)
	want := "zsh:typeset:1: -T requires names of scalar and array\n1 st=1\n" +
		"zsh:typeset:4: can't tie already tied scalar: S\n2 st=1\n" +
		"3 same-again st=0\n" +
		"zsh:typeset:8: second argument of tie must be array: q\n4 st=1\n" +
		"reached\n"
	if out != want || st != 0 {
		t.Errorf("the reported refusals = %q (status %d), want %q", out, st, want)
	}
}

// And the two that end the script, each on its own because neither reaches
// the line after it.
func TestTyingANameToItselfEndsTheScript(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -T A A 2>&1
print -r -- "reached"`)
	want := "zsh:typeset:1: can't tie a variable to itself: A\n"
	if out != want || st != 1 {
		t.Errorf("a self-tie = %q (status %d), want %q with 1", out, st, want)
	}
}

// A half that is not a name at all. Two spellings because this shell has two
// wordings for them, and the second is what the parser's own reordering can
// produce: it lifts an array literal out of the operands and appends its bare
// name, so `typeset -T R r=(a b) ':'` arrives as `R`, `:`, `r` and a
// positional reading would tie `R` to `:`.
func TestATieHalfThatIsNotAName(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -T A ':' 2>&1
print -r -- "reached"`)
	want := "zsh:typeset:1: not valid in this context: :\n"
	if out != want || st != 1 {
		t.Errorf("a non-name half = %q (status %d), want %q with 1", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(), `typeset -T A 1x 2>&1
print -r -- "reached"`)
	want = "zsh:typeset:1: not an identifier: 1x\n"
	if out != want || st != 1 {
		t.Errorf("a numeric half = %q (status %d), want %q with 1", out, st, want)
	}
}

// The other letters go to the halves they belong to: `-U` and `-r` to both,
// and **export to the scalar alone**, because the scalar is what a child can
// be told. The listing is where that shows, and `T` comes last of all the
// letters.
func TestTheTieListsBackWithTheLettersItsHalvesHave(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -TUx A1 a1
a1=(p q p)
typeset -p A1 a1
typeset -Tr B1 b1
typeset -p B1 b1
typeset -T C1 c1 '#'
c1=(1 2)
typeset -p C1 c1
typeset -T D1 d1
typeset -p D1 d1`)
	want := "export -UT A1 a1=( p q )\ntypeset -aUT A1 a1=( p q )\n" +
		"typeset -rT B1 b1=(  )\ntypeset -arT B1 b1=(  )\n" +
		"typeset -T C1 c1=( 1 2 ) '#'\ntypeset -aT C1 c1=( 1 2 ) '#'\n" +
		"typeset -T D1 d1=(  )\ntypeset -aT D1 d1=(  )\n"
	if out != want || st != 0 {
		t.Errorf("the tie listing = %q (status %d), want %q", out, st, want)
	}
}

// `-U` and `-T` together, which is the spelling a plugin manager writes:
// the array dedupes and the scalar is the joined survivors.
func TestAUniqueTieJoinsOnlyTheSurvivors(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -gxTU TU tu
tu=(a b a)
print -r -- "TU=[$TU] tu=[${(j:|:)tu}]"`)
	want := "TU=[a:b] tu=[a|b]\n"
	if out != want || st != 0 {
		t.Errorf("a unique tie = %q (status %d), want %q", out, st, want)
	}
}

// `typeset -T` with nothing to tie lists the ties, both halves of each, as
// plain assignments — not in `typeset -p`'s shape. Filtered to the names the
// case invents, because the rest of a listing is the machine's.
func TestBareDashTListsTheTies(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -T ZA za
za=(1 2)
typeset -T ZB zb '#'
zplain=notatie
typeset -T`)
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	// Whole lines out of a listing the machine's own environment shares —
	// and the upper-case halves come with the lower-case ones, which is what
	// says both halves of a tie are listed.
	wantWholeLines(t, out, "ZA=1:2", "za=( 1 2 )", "ZB=''", "zb=(  )")
	for _, unwanted := range []string{"zplain=notatie", "zplain"} {
		if hasWholeZshLine(out, unwanted) {
			t.Errorf("line %q in %q, want only the ties listed", unwanted, out)
		}
	}
}

func hasWholeZshLine(out, line string) bool {
	for _, got := range strings.Split(out, "\n") {
		if got == line {
			return true
		}
	}
	return false
}

// A tie declared in a function is the function's, and is gone on return —
// the tie as much as the values. The tie is not in the variable tables, so
// the scope's save-and-restore does not carry it and it is undone by hand.
func TestAFunctionLocalTieIsGoneOnReturn(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `fn() { typeset -T L1 l1; L1=a:b; print -r -- "in=[${(j:|:)l1}]"; }
fn
print -r -- "out L1=[${L1-UNSET}] l1=[${l1+SET}]"
L1=p:q
print -r -- "untied n=${#l1}"`)
	want := "in=[a|b]\nout L1=[UNSET] l1=[]\nuntied n=0\n"
	if out != want || st != 0 {
		t.Errorf("a local tie = %q (status %d), want %q", out, st, want)
	}
}

// A readonly tie refuses a write through either half.
// A readonly tie refuses a write through either half, and the refusal is the
// one a readonly name always gets — fatal here, so neither run reaches the
// line after it.
func TestAReadonlyTieRefusesBothHalves(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -Tr R1 r1
R1=x
print -r -- "reached"`)
	want := "zsh:2: read-only variable: R1\n"
	if out != want || st != 1 {
		t.Errorf("through the scalar = %q (status %d), want %q with 1", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(), `typeset -Tr R1 r1
r1=(1)
print -r -- "reached"`)
	want = "zsh:2: read-only variable: r1\n"
	if out != want || st != 1 {
		t.Errorf("through the array = %q (status %d), want %q with 1", out, st, want)
	}
}
