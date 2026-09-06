// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `typeset -H`, measured against zsh 5.9.2 (2026-09-06).
//
// The letter hides a name's *value* from every listing that would write one.
// Nothing else moves: the name is declared, holds what it holds, and reads
// back through `$h` and `${m[k]}` exactly as it would without the letter — so
// the listings below are the only place the attribute can be observed at all.
//
// It is this shell's letter alone with that meaning. bash refuses `-H` under
// both `declare` and `typeset`, and ksh93 has an `-H` that is a different
// attribute entirely and does *not* hide: `typeset -H h=hid` lists back as
// `typeset -H h=hid` there, value and flag both. That refusal is kept — see
// Diagnostics.UnimplementedOptionLetters under dialect/ksh.

func TestTypesetHHidesTheValueAndNothingElse(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -H h=hid
echo "read=[$h]"
typeset -p h`)
	want := "read=[hid]\ntypeset h\n"
	if out != want || st != 0 {
		t.Errorf("typeset -H = %q (status %d), want %q", out, st, want)
	}
}

// `+H` puts the value back — and it is the round trip that shows the value was
// never touched, only withheld.
func TestTypesetPlusHShowsTheValueAgain(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -iH n=5
typeset -p n
typeset +H n
typeset -p n`)
	want := "typeset -i n\ntypeset -i n=5\n"
	if out != want || st != 0 {
		t.Errorf("the -H round trip = %q (status %d), want %q", out, st, want)
	}
}

// The bundled spellings scripts actually write. The attributes still speak in
// the listing; only the value is missing, a table's and an array's alike.
func TestTypesetHOnCompoundNames(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -gAH m
m[k]=v
echo "elem=[${m[k]}]"
typeset -p m
typeset -gaH a
a=(x y)
echo "elem=[${a[1]}]"
typeset -p a`)
	want := "elem=[v]\ntypeset -A m\nelem=[x]\ntypeset -a a\n"
	if out != want || st != 0 {
		t.Errorf("typeset -gAH/-gaH = %q (status %d), want %q", out, st, want)
	}
}

// The other listings the attribute reaches. `export -p` names the builtin,
// `readonly -p` writes the `typeset -r` shape, and the bare forms write the
// assignment alone — each of them short of the value, where an ordinary name
// on the next line still carries one.
func TestTypesetHReachesTheOtherListings(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -xH ex=E
typeset -rH ro=R
typeset -x ex2=E2
export -p
readonly -p
export`)
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	// Whole lines, matched as lines: the rest of these listings is the
	// environment the test was started with, and a `Contains` would take
	// `export ex` out of `export ex2=E2` and call the value hidden.
	for _, want := range []string{
		"export ex",     // the hidden one, `export -p`
		"export ex2=E2", // an ordinary one on the same listing, with its value
		"typeset -r ro", // the hidden one, `readonly -p`
		"ex",            // the hidden one, bare `export`
		"ex2=E2",
	} {
		if !hasLine(out, want) {
			t.Errorf("listing: no line %q in %q", want, out)
		}
	}
	for _, unwanted := range []string{"export ex=E", "typeset -r ro=R", "ex=E"} {
		if hasLine(out, unwanted) {
			t.Errorf("listing: line %q in %q, want the value withheld", unwanted, out)
		}
	}
}

// And a bare `set`, which is a listing of its own rather than a declaration
// one: the hidden name is written with no `=` at all. Filtered to two names
// the script invents, because the rest of the listing is the machine's.
func TestBareSetWritesAHiddenNameWithoutItsValue(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -H zzh=hid
zzv=plain
set`)
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	if !hasLine(out, "zzh") {
		t.Errorf("bare set: no line %q in %q", "zzh", out)
	}
	if !hasLine(out, "zzv=plain") {
		t.Errorf("bare set: no line %q in %q, want the listing to have run at all", "zzv=plain", out)
	}
	if hasLine(out, "zzh=hid") {
		t.Errorf("bare set: line %q in %q, want the value withheld", "zzh=hid", out)
	}
}

// `local` reads the letter too — the same set under both names here.
func TestLocalReadsTheHidingLetter(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f() { local -H l=L; typeset -p l; echo "read=[$l]"; }
f`)
	want := "typeset l\nread=[L]\n"
	if out != want || st != 0 {
		t.Errorf("local -H = %q (status %d), want %q", out, st, want)
	}
}

// A declaration that only adds an attribute must not take the value away. It
// did: `typeset -H h` on a name already holding `hid` emptied it, so the
// letter meant to hide a value destroyed it instead — and the same line with
// `-x`, `-i`, `-l` or `-u` in place of `-H` did the same thing.
//
// What is kept is read back through the attribute that has just arrived,
// which is not the same as leaving it alone: `5+2` becomes 7 under `-i` and
// `MiXeD` becomes `MIXED` under `-u`.
func TestAnAttributeAddedToANameKeepsAndRefoldsItsValue(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `h=hid
typeset -H h
typeset +H h
typeset -p h
a=5+2
typeset -i a
echo "a=[$a]"
d=MiXeD
typeset -u d
echo "d=[$d]"
x=xyz
typeset -x x
echo "x=[$x]"`)
	want := "typeset h=hid\na=[7]\nd=[MIXED]\nx=[xyz]\n"
	if out != want || st != 0 {
		t.Errorf("an attribute on a name with a value = %q (status %d), want %q", out, st, want)
	}
}

// The value a *new* declaration gets is still the empty one, which is this
// shell's answer where the other three leave the name unset — the rule is
// about a name the declaration creates, and the local a function's shadow
// makes is created however much the caller held.
func TestAValuelessDeclarationOfANewNameIsStillEmpty(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -x fresh
echo "fresh=[${fresh-UNSET}]"
outer=5
function f { typeset -x outer; echo "in=[${outer-UNSET}]"; }
f
echo "after=[$outer]"`)
	want := "fresh=[]\nin=[]\nafter=[5]\n"
	if out != want || st != 0 {
		t.Errorf("a valueless declaration = %q (status %d), want %q", out, st, want)
	}
}
