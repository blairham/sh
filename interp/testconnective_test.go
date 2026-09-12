// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"
)

// `[ "$a" -a "$b" ]` — the both-set guard, and the shape `test` is most often
// written in outside a one-operator check. It is three words, and three words
// went to the comparison rule, found no comparison among them and reported
// `binary operator expected` at status 2.
//
// The grammar past four arguments already had both connectives with `-a`
// binding tighter than `-o`, so this was never a missing feature: it was the
// three-word form falling through a rule that did not mention it. Which is
// the worst way for it to be wrong — a guard meant to answer yes or no
// answered "this is not an expression", and a script branching on `$?` took
// neither arm, because 2 is neither 0 nor 1.
//
// Unanimous across the panel: bash 5.3, bash-as-`sh`, bash 3.2, ksh93,
// zsh 5.9.2, dash and BusyBox ash all answer these the same way, so they are
// the core's and not a dialect's.

// TestThreeWordConnectivesJoinTwoStrings. Each side is true when it is
// non-empty, which is the one-argument rule applied to each operand — there
// is no room for an operator on either side of a three-word expression.
func TestThreeWordConnectivesJoinTwoStrings(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`test x -a y`, 0},
		{`test x -a ""`, 1},
		{`test "" -a x`, 1},
		{`test "" -a ""`, 1},
		{`test x -o y`, 0},
		{`test x -o ""`, 0},
		{`test "" -o x`, 0},
		{`test "" -o ""`, 1},
		// `[` is the same builtin under another name, so it answers alike.
		{`[ x -a "" ]`, 1},
		{`[ "" -o x ]`, 0},
		// A word that looks like an operator is still only a string here:
		// with three words there is nothing for it to operate on.
		{`test -n -a x`, 0},
		{`test -a -a -a`, 0},
	} {
		if got := testStatus(t, dir, tc.src); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.src, got, tc.want)
		}
	}
}

// TestAComparisonStillWinsOverTheConnectives, because the middle word decides
// and a comparison is looked for first. Nothing here is ambiguous today —
// neither connective is also a comparison — and it is asserted so that adding
// one cannot quietly change which rule a three-word expression takes.
func TestAComparisonStillWinsOverTheConnectives(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`test -a = -a`, 0},
		{`test -a = -o`, 1},
		{`test -o != -o`, 1},
	} {
		if got := testStatus(t, dir, tc.src); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.src, got, tc.want)
		}
	}
}

// TestTheConnectivesKeepTheirPrecedencePastThreeWords is why the three-word
// rule is its own arm rather than an entry in the comparison table: the
// grammar reads `-a` as binding tighter than `-o`, and a comparison that
// swallowed `a -o b` whole would associate `[ a -o b -a c ]` the other way
// and answer the opposite.
func TestTheConnectivesKeepTheirPrecedencePastThreeWords(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want int
	}{
		// "" || (x && "") is false. Left-to-right it would be ("" || x) && ""
		// — also false — so the discriminating pair is below.
		{`test "" -o x -a ""`, 1},
		{`test x -a "" -o x`, 0},
		// x || ("" && "") is true; ((x || "") && "") is false.
		{`test x -o "" -a ""`, 0},
		// A negation in front of three words negates all three. Six of the
		// seven columns agree; ksh93 alone answers 1 to the second of
		// these, which is recorded in the oracle rather than followed.
		{`test ! x -a y`, 1},
		{`test ! x -a ""`, 0},
	} {
		if got := testStatus(t, dir, tc.src); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.src, got, tc.want)
		}
	}
}

// TestOwnershipOperatorsAskAboutTheEffectiveIdentity — `-O` and `-G`, which
// were not operators here at all: `test -O f` was `unary operator expected`
// at 2 and `[[ -O f ]]` was a syntax error that abandoned the clause.
//
// The file is one this process just made, so it is owned by the effective
// user and group by construction and the true answers do not depend on the
// machine. The false half is a path that does not exist, which is false for
// every file operator and so needs no identity of its own.
func TestOwnershipOperatorsAskAboutTheEffectiveIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mine"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`test -O mine`, 0},
		{`test -G mine`, 0},
		{`[ -O mine ]`, 0},
		{`[ -G mine ]`, 0},
		{`test -O nosuchfile`, 1},
		{`test -G nosuchfile`, 1},
		// An empty operand is not a path, which is the answer every other
		// file operator gives it.
		{`test -O ""`, 1},
		{`test -G ""`, 1},
	} {
		if got := testStatus(t, dir, tc.src); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.src, got, tc.want)
		}
	}
}
