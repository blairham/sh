// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `${(S)x}` through the shell that has the flag, against zsh 5.9.2 on
// 2026-09-12.
//
// The substitution half is what the flag is usually described as — non-greedy
// — and every assertion here is a *pair*, the flagged spelling beside the
// unflagged one, because either alone is a value rather than a comparison.
func TestTheSearchingFlagMakesASubstitutionNonGreedy(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `s=abab
print -r -- "[${s/*b/_}][${(S)s/*b/_}]"
print -r -- "[${s//*b/_}][${(S)s//*b/_}]"
v=abcabc
print -r -- "[${v/#a*b/X}][${(S)v/#a*b/X}]"
print -r -- "[${v/%b*c/X}][${(S)v/%b*c/X}]"
print -r -- "[${v/a*c/X}][${(S)v/a*c/X}]"
print -r -- "[${v//a*c/X}][${(S)v//a*c/X}]"`)
	want := "[_][_ab]\n[_][__]\n[Xc][Xcabc]\n[aX][abcaX]\n[X][Xabc]\n[X][XX]\n"
	if out != want || st != 0 {
		t.Errorf("(S) on a substitution = %q (status %d), want %q", out, st, want)
	}
}

// The half the name does not give away: against a trim the flag is a
// substring search, so the piece taken comes out of the middle.
//
// The `%%` pair is the one that cannot be explained as a length rule. The
// flag answers `aXb`, taking `Xc` out of the end, where the unflagged
// operator answers `a` for having taken the longest suffix `XbXc` — and
// `${(S)str%X*}` takes a single `X` from the *middle*, which is the thing a
// suffix trim cannot do at all.
func TestTheSearchingFlagMakesATrimASubstringSearch(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `str=aXbXc
print -r -- "[${str#X*}][${(S)str#X*}]"
print -r -- "[${str##X*}][${(S)str##X*}]"
print -r -- "[${str%X*}][${(S)str%X*}]"
print -r -- "[${str%%X*}][${(S)str%%X*}]"
w=xxhelloyy
print -r -- "[${w#hello}][${(S)w#hello}][${(S)w%hello}]"
print -r -- "[${(S)str#zz}][${(S)str#}]"`)
	want := "[aXbXc][abXc]\n[aXbXc][a]\n[aXb][aXbc]\n[a][aXb]\n" +
		"[xxhelloyy][xxyy][xxyy]\n[aXbXc][aXbXc]\n"
	if out != want || st != 0 {
		t.Errorf("(S) on a trim = %q (status %d), want %q", out, st, want)
	}
}

// `(S)` chooses which match and `(M)` says which side of it to keep, so the
// two are one span read from both ends rather than two mechanisms.
//
// Every line here is the pair, and the pair is the assertion: an
// implementation that kept a split point instead of a span can answer the
// trim and not the match once the piece comes from the middle.
func TestTheSearchingAndMatchingFlagsCompose(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `str=aXbXc
print -r -- "[${(S)str#X*}][${(SM)str#X*}]"
print -r -- "[${(S)str##X*}][${(SM)str##X*}]"
print -r -- "[${(S)str%X*}][${(SM)str%X*}]"
print -r -- "[${(S)str%%X*}][${(SM)str%%X*}]"
print -r -- "[${(SM)str#zz}][${(SM)str#}]"
a=(xxAyy zzAww)
print -r -- "[${(S@)a#A}][${(SM@)a#A}]"`)
	want := "[abXc][X]\n[a][XbXc]\n[aXbc][X]\n[aXb][Xc]\n[][]\n" +
		"[xxyy zzww][A A]\n"
	if out != want || st != 0 {
		t.Errorf("(SM) = %q (status %d), want %q", out, st, want)
	}
}

// And where the flag does not reach, measured one operator at a time.
//
// A flag that chooses between a longest and a shortest match has nothing to
// say where there is no choice. The exclusion is the row worth having: it is
// a pattern operator, so a reading that gave the flag to "anything with a
// pattern in it" would move it, and it is a whole-value test with no second
// match to prefer.
func TestTheSearchingFlagReachesOnlyThePatternOperators(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `str=aXbXc
print -r -- "[${(S)str}][${(S)str:1}][${(S)str:-alt}]"
print -r -- "[${(S)str:#a*}][${(S)#str}][${(S)str[(i)X]}]"
a=(f1 f22 f333)
print -r -- "[${(S@)a:#f2*}]"`)
	want := "[aXbXc][XbXc][aXbXc]\n[][5][2]\n[f1 f333]\n"
	if out != want || st != 0 {
		t.Errorf("(S) elsewhere = %q (status %d), want %q", out, st, want)
	}
}

// The flag composes with the arm-order reading a longest trim already has,
// and it widens where that reading applies: a longest *suffix* trim takes the
// longest match in either written order without the flag, and takes the
// written arm with it.
//
// The last pair is the boundary, and it says which of the two questions is
// asked first: with the flag, `abcbc` gives up its final `bc` whichever order
// the arms were written in, because the match that starts closest to the end
// wins over the arm that was written first.
func TestTheSearchingFlagAndTheArmsOfAnAlternation(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `w=abc
print -r -- "[${(S)w##(a|ab)}][${(S)w##(ab|a)}]"
print -r -- "[${w%%(b|bc)}][${(S)w%%(b|bc)}][${(S)w%%(bc|b)}]"
print -r -- "[${(S)w#(a|ab)}][${(S)w#(ab|a)}]"
v=abcbc
print -r -- "[${v%%(bc|cbc)}][${v%%(cbc|bc)}]"
print -r -- "[${(S)v%%(bc|cbc)}][${(S)v%%(cbc|bc)}]"`)
	want := "[bc][c]\n[a][ac][a]\n[bc][bc]\n[ab][ab]\n[abc][abc]\n"
	if out != want || st != 0 {
		t.Errorf("(S) with an alternation = %q (status %d), want %q", out, st, want)
	}
}

// Where a global substitution stops, for a pattern that can match nothing.
//
// The end of the value is a position a match may start at — `(#e)` matches
// only there and fires — but it is not one after the step an empty match
// takes to make progress. So it is a rule about the step and not about the
// position, and the two patterns in the fourth and fifth rows reach the same
// place by the two different routes and answer differently.
//
// This was wrong here before `(S)` needed it: `${v//x#/-}` left `-a-b-c-`,
// one replacement more than the shell makes.
func TestAnEmptyMatchAtTheEndOfASubstitution(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt extendedglob
v=abc
print -r -- "[${v//x#/-}][${v//c#/-}][${v//(#e)/-}]"
print -r -- "[${v//(x#|(#e))/-}][${v//((#s)|(#e))/-}]"
print -r -- "[${(S)v//x#/-}][${(S)v//(#e)/-}]"
e=
print -r -- "[${e//x#/-}][${e//(#e)/-}]"
str=aXbXc
print -r -- "[${str//X#/-}][${(S)str//X#/-}]"`)
	want := "[-a-b-c][-a-b-][abc-]\n[-a-b-c][-abc-]\n[-a-b-c][abc-]\n" +
		"[-][-]\n[-a--b--c][-a-X-b-X-c]\n"
	if out != want || st != 0 {
		t.Errorf("an empty match at the end = %q (status %d), want %q", out, st, want)
	}
}

// What `(#b)` reports through a searching trim: the match a search settled on
// rather than the one the operator would have taken on its own.
//
// One measured binding is deliberately absent. `${(S)w%(#b)(X*)}` fills
// `$match[1]` with `X` in zsh 5.9.2 and sets `$mbegin[1]` and `$mend[1]` to
// **6** for a five-character value — an index past the end, disagreeing with
// the match it reports beside it. That is not a reading this implementation
// can hold consistently, so the row is measured and named here rather than
// reproduced; every other spelling agrees and is asserted.
func TestASearchingTrimReportsTheMatchItTook(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt extendedglob
probe() {
  w=aXbXc
  unset match mbegin mend
  eval ": \"\${$1}\""
  print -r -- "[${match[1]}:${mbegin[1]}:${mend[1]}]"
}
probe '(S)w#(#b)(X*)'
probe '(S)w##(#b)(X*)'
probe '(S)w%%(#b)(X*)'
probe 'w%(#b)(X*)'
probe '(S)w//(#b)(X)/-'
probe '(S)w/(#b)(X*)/-'`)
	want := "[X:2:2]\n[XbXc:2:5]\n[Xc:4:5]\n[Xc:4:5]\n[X:4:4]\n[X:2:2]\n"
	if out != want || st != 0 {
		t.Errorf("(#b) under (S) = %q (status %d), want %q", out, st, want)
	}
}
