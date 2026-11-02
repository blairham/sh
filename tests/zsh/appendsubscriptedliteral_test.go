// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An appending literal that holds any `[k]=v` element is a run of element
// stores into the array the name holds: the bare elements in front of the
// first subscripted one start at the first position and overwrite, where an
// ordinary append starts after the last element. Measured 2026-11-01 on zsh
// 5.9.2 from a file (`-f`), this script, byte for byte. The bare elements used
// to go after the last element here, so `b+=(x y [5]=z)` lost the `y` under
// the `z` (#7079).
//
// The fourth row and the last two are the controls: a literal that opens with
// a subscript, one that holds nothing but a subscript, and one with no
// subscript at all answer the same under both readings. The fifth row is a
// produced array, which is stored into the same way.
func TestAnAppendingLiteralWithASubscriptStoresFromTheFirstPosition(t *testing.T) {
	t.Parallel()
	src := "b=(7); b+=(9 [3]=y); typeset -p b\n" +
		"b=(7 8 9); b+=(x [5]=y); typeset -p b\n" +
		"b=(7 8 9); b+=(x y [5]=z); typeset -p b\n" +
		"b=(7 8 9); b+=([5]=z x); typeset -p b\n" +
		"set -- 7 8 9; argv+=(x [5]=y); print -r -- \"$# $*\"\n" +
		"b=(7 8 9 10); b+=(x [2]=y z); typeset -p b\n" +
		"b=(7 8); b+=([1]=y); typeset -p b\n" +
		"b=(7); b+=(9); typeset -p b\n"
	want := "typeset -a b=( 9 '' y )\n" +
		"typeset -a b=( x 8 9 '' y )\n" +
		"typeset -a b=( x y 9 '' z )\n" +
		"typeset -a b=( 7 8 9 '' z x )\n" +
		"5 x 8 9  y\n" +
		"typeset -a b=( x y z 10 )\n" +
		"typeset -a b=( y 8 )\n" +
		"typeset -a b=( 7 9 )\n"
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if out != want || st != 0 {
		t.Errorf("status %d, got\n%s\nwant\n%s", st, out, want)
	}
}

// It is the name already holding an array that starts the run over. Over a
// scalar the value stays first and the bare elements follow it, and an unset
// name starts at the first position whichever reading it is given. Measured
// in the same run.
func TestAnAppendingLiteralWithASubscriptOverAScalarKeepsTheValueFirst(t *testing.T) {
	t.Parallel()
	src := "s=abc; s+=(x [3]=y); typeset -p s\n" +
		"unset u; u+=(x [3]=y); typeset -p u\n"
	want := "typeset -a s=( abc x y )\n" +
		"typeset -a u=( x '' y )\n"
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if out != want || st != 0 {
		t.Errorf("status %d, got\n%s\nwant\n%s", st, out, want)
	}
}

// A bare element that expands to no word at all still takes a position in a
// literal holding any `[k]=v` element, `=` and `+=` alike: it stores an empty
// element and moves the position on. It used to be removed as in any other
// literal, so the next bare element landed one position early and the element
// the empty one should have overwritten survived (#7321). Measured 2026-11-02
// on zsh 5.9.2 from a file (`-f`), this script, byte for byte.
//
// The first row is the issue's. The rows after the quoted one are the shapes
// that come to nothing — an empty array, `"$@"` with no parameters, a
// substitution that prints nothing — and the last three are the controls: a
// literal with no subscripted element removes the empty word as it always
// did, and so does `nullglob`, whose removal is the pathname stage's and comes
// after the element has its word.
func TestAnEmptyBareElementOfASubscriptedLiteralTakesAPosition(t *testing.T) {
	t.Parallel()
	src := "e=\n" +
		"b=(7 8 9); b+=(x $e [6]=y); typeset -p b\n" +
		"b=(x $e z [5]=y); typeset -p b\n" +
		"b=([2]=q x $e y); typeset -p b\n" +
		"b=(x [3]=y $e); typeset -p b\n" +
		"typeset -a b=(x $e z [5]=y); typeset -p b\n" +
		"b=(7 8 9); b+=($e [3]=y); typeset -p b\n" +
		"b=(x \"$e\" z [5]=y); typeset -p b\n" +
		"n=(); b=(x $n z [5]=y); typeset -p b\n" +
		"set --; b=(x \"$@\" z [5]=y); typeset -p b\n" +
		"b=(x $(true) z [5]=y); typeset -p b\n" +
		"b=(x $e z); typeset -p b\n" +
		"b=(7 8 9); b+=(x $e z); typeset -p b\n" +
		"setopt nullglob; b=(x nomatch* z [5]=y); typeset -p b\n"
	want := "typeset -a b=( x '' 9 '' '' y )\n" +
		"typeset -a b=( x '' z '' y )\n" +
		"typeset -a b=( '' q x '' y )\n" +
		"typeset -a b=( x '' y '' )\n" +
		"typeset -a b=( x '' z '' y )\n" +
		"typeset -a b=( '' 8 y )\n" +
		"typeset -a b=( x '' z '' y )\n" +
		"typeset -a b=( x '' z '' y )\n" +
		"typeset -a b=( x '' z '' y )\n" +
		"typeset -a b=( x '' z '' y )\n" +
		"typeset -a b=( x z )\n" +
		"typeset -a b=( 7 8 9 x z )\n" +
		"typeset -a b=( x z '' '' y )\n"
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if out != want || st != 0 {
		t.Errorf("status %d, got\n%s\nwant\n%s", st, out, want)
	}
}

// A negative subscript inside a literal is refused as `[0]=v` is, where the
// same subscript on its own counts back from the end: `bad subscript for
// direct array assignment`, quoting the subscript as written, and the script
// ends at 1 with the name as it was. It used to count back from the end and
// carry on at 0 (#7322). Measured 2026-11-02 on zsh 5.9.2 from a file (`-f`),
// byte for byte but for the file's name where this has `zsh`, the exit trap
// reading the name back. Under `-c` the refusals are the same and the exit
// trap does not run, here as there.
//
// The first row is the issue's. The last is the control: `b[-1]=y` is not a
// literal and still counts from the end.
func TestANegativeSubscriptInALiteralIsRefused(t *testing.T) {
	t.Parallel()
	const bad = "zsh:1: bad subscript for direct array assignment: "
	for _, c := range []struct{ src, want string }{
		{"b=(7 8 9); b+=(x [-1]=y); echo st=$?; typeset -p b", bad + "-1\n"},
		{"b=(7 8 9); b+=([-1]=y); echo st=$?", bad + "-1\n"},
		{
			"b=(7 8 9); trap 'typeset -p b' EXIT; b=(x [-1]=y); echo after",
			bad + "-1\ntypeset -a b=( 7 8 9 )\n",
		},
		{
			"b=(7 8 9); trap 'typeset -p b' EXIT; b+=([2]=q [-1]=y); echo after",
			bad + "-1\ntypeset -a b=( 7 8 9 )\n",
		},
		{"b=(7 8 9); typeset -a b=([1]=z [2]=w [-1]=x); echo after", bad + "-1\n"},
		{"b=(7 8 9); i=-1; b+=([i]=y); echo after", bad + "i\n"},
		{"b=(7 8 9); b[-1]=y; typeset -p b", "typeset -a b=( 7 8 y )\n"},
	} {
		out, st := answersRun(t, c.src)
		wantSt := 1
		if !strings.HasPrefix(c.want, bad) {
			wantSt = 0
		}
		if out != c.want || st != wantSt {
			t.Errorf("%s\n got %q, status %d\nwant %q, status %d", c.src, out, st, c.want, wantSt)
		}
	}
}
