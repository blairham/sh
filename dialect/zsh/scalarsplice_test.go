// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// Assigning to a subscript of a name that holds a string (#1746).
//
// Measured on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f -c`) on 2026-09-10. Every
// probe uses `v=abc` and a subscript other than the first, because that is the
// smallest shape where the two readings give two different strings: `aXc` is
// the character at 2 replaced, and a one-element array or a subscript of 1
// answers `X` under both and proves nothing.
//
// `typeset -p` is asserted beside the value throughout. The bug was not that
// the value was wrong — for some rows it was not — but that the *name* came
// back an array, and only its declaration says so.

func spliced(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st := runZsh(t, t.TempDir(), src)
	return strings.TrimSpace(out), st
}

func TestASubscriptOnAStringSplicesItsCharacters(t *testing.T) {
	for _, tc := range []struct{ write, want string }{
		// zsh 5.9.2, `v=abc` throughout:
		{`v[2]=X`, "[aXc] typeset v=aXc"},
		{`v[2]=XY`, "[aXYc] typeset v=aXYc"},
		{`v[2]=`, "[ac] typeset v=ac"},
		{`v[2,3]=XY`, "[aXY] typeset v=aXY"},
		{`v[2,3]=X`, "[aX] typeset v=aX"},
		{`v[2,3]=XYZW`, "[aXYZW] typeset v=aXYZW"},
		{`v[2,-1]=X`, "[aX] typeset v=aX"},
		{`v[1,-1]=X`, "[X] typeset v=X"},
		// An index past the end appends, and does not pad the gap — which is
		// what makes `v[$#v+1]=x` the way a script appends to a string here.
		{`v[4]=X`, "[abcX] typeset v=abcX"},
		{`v[10]=X`, "[abcX] typeset v=abcX"},
		{`v[$#v+1]=Q`, "[abcQ] typeset v=abcQ"},
		// Negative counts back from the last character; past the first it
		// lands in front of everything and takes nothing out.
		{`v[-1]=X`, "[abX] typeset v=abX"},
		{`v[-2]=X`, "[aXc] typeset v=aXc"},
		{`v[-4]=X`, "[Xabc] typeset v=Xabc"},
		{`v[-5]=X`, "[Xabc] typeset v=Xabc"},
		// A reversed range is an empty span at the start: the value goes in
		// and nothing comes out.
		{`v[3,2]=X`, "[abXc] typeset v=abXc"},
		{`v[1,0]=X`, "[Xabc] typeset v=Xabc"},
		{`v[0,1]=X`, "[Xbc] typeset v=Xbc"},
		// `+=` joins the span it names, back where the span was. It reads no
		// range, so the comma in the third row is the arithmetic operator and
		// the character joined is the one at 3.
		{`v[2]+=X`, "[abXc] typeset v=abXc"},
		{`v[2,3]+=X`, "[abcX] typeset v=abcX"},
		{`v[4]+=X`, "[abcX] typeset v=abcX"},
		// Reached through every spelling that stores an element, not only
		// through the assignment statement.
		{`typeset "v[2]"=X`, "[aXc] typeset v=aXc"},
		{`(( v[2] = 5 ))`, "[a5c] typeset v=a5c"},
		{`x="v[2]"; : ${(P)x::=Z}`, "[aZc] typeset v=aZc"},
	} {
		out, st := spliced(t, "v=abc\n"+tc.write+"\nprint -r -- \"[$v]\" \"$(typeset -p v)\"")
		if out != tc.want || st != 0 {
			t.Errorf("%s: got %q status %d, want %q", tc.write, out, st, tc.want)
		}
	}
}

// A name nobody set is not a string, and the subscript builds an array with
// the gap in front of it — which is the boundary, and the half that must not
// move: `typeset -a u=( '' X )` is what zsh leaves.
func TestASubscriptOnANameThatHoldsNothingBuildsAnArray(t *testing.T) {
	for _, tc := range []struct{ start, want string }{
		{"", "typeset -a u=( '' X )"},
		{"u=abc\nunset u\n", "typeset -a u=( '' X )"},
		// But a declaration is holding the empty string, so it splices — and
		// with nothing to splice into the value stands alone.
		{"typeset u\n", "typeset u=X"},
		{"u=\n", "typeset u=X"},
	} {
		out, st := spliced(t, tc.start+"u[2]=X\ntypeset -p u")
		if out != tc.want || st != 0 {
			t.Errorf("%q: got %q status %d, want %q", tc.start, out, st, tc.want)
		}
	}
}

// `ksharrays` moves the first character with the first element: it is the base
// and nothing else that decides which character a subscript names, so the
// option that made `${a[1]}` the second element makes `v[1]=X` the second
// character. The negative reading does not move, which is the same half of the
// option that did not move for arrays (#1726).
func TestKshArraysMovesWhichCharacterASubscriptNames(t *testing.T) {
	for _, tc := range []struct{ write, want string }{
		{`v[0]=X`, "[Xbc]"},
		{`v[1]=X`, "[aXc]"},
		{`v[2]=X`, "[abX]"},
		{`v[3]=X`, "[abcX]"},
		{`v[1,2]=XY`, "[aXY]"},
		{`v[-1]=X`, "[abX]"},
	} {
		out, st := spliced(t,
			"setopt ksharrays\nv=abc\n"+tc.write+"\nprint -r -- \"[$v]\"")
		if out != tc.want || st != 0 {
			t.Errorf("ksharrays %s: got %q status %d, want %q", tc.write, out, st, tc.want)
		}
	}
}

// A non-negative subscript below the first character is the array's refusal
// reached through a string: the same sentence, and the script ends at 1.
func TestASubscriptBelowTheFirstCharacterIsRefusedByName(t *testing.T) {
	for _, write := range []string{`v[0]=X`, `v[0,0]=X`} {
		out, st := spliced(t, "v=abc\n"+write+"\nprint -r -- reached")
		const want = "zsh:2: v: assignment to invalid subscript range"
		if out != want || st != 1 {
			t.Errorf("%s: got %q status %d, want %q status 1", write, out, st, want)
		}
	}
}

// A name carrying an arithmetic attribute has no characters for a subscript to
// reach. Measured: the subscript is ignored and the value lands whole, and
// `+=` at one is refused outright.
func TestASubscriptOnANumericNameTakesTheValueWhole(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{"typeset -i n=123\nn[2]=9\ntypeset -p n", "typeset -i n=9", 0},
		{"typeset -i n=123\nn[5]=9\ntypeset -p n", "typeset -i n=9", 0},
		{"typeset -i n=123\nn[2,3]=99\ntypeset -p n", "typeset -i n=99", 0},
		{"typeset -F f=1.5\nf[2]=9\ntypeset -p f", "typeset -F f=9.0000000000", 0},
		{"typeset -i n=12\nn[1]+=9", "zsh:2: attempt to add to slice of a numeric variable", 1},
		{"typeset -i n=12\nn[1,2]+=9", "zsh:2: attempt to add to slice of a numeric variable", 1},
		{"typeset -F f=1.5\nf[1]+=9", "zsh:2: attempt to add to slice of a numeric variable", 1},
	} {
		out, st := spliced(t, tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%q: got %q status %d, want %q status %d",
				tc.src, out, st, tc.want, tc.status)
		}
	}
}

// `read` into a subscripted operand. It resolved the name with setVar and
// never looked at the brackets, so the line below created a parameter *called*
// `v[3]` and left `v` alone — which is the shape a prompt theme's worker reads
// its responses in, one `sysread` at a time into `buf[$#buf+1]`.
func TestReadFillsASubscriptedOperand(t *testing.T) {
	for _, tc := range []struct{ start, operand, want string }{
		{"v=xy\n", `v[$#v+1]`, "[xyQ] typeset v=xyQ"},
		{"v=xy\n", `v[2]`, "[xQ] typeset v=xQ"},
		{"v=xy\n", `v[2,3]`, "[xQ] typeset v=xQ"},
	} {
		out, st := spliced(t, tc.start+"read '"+tc.operand+"' <<IN\nQ\nIN\n"+
			"print -r -- \"[$v]\" \"$(typeset -p v)\"")
		if out != tc.want || st != 0 {
			t.Errorf("read %s: got %q status %d, want %q", tc.operand, out, st, tc.want)
		}
	}
	// An array's element, which is what the whole panel does and what this
	// shell did not.
	out, st := spliced(t, "a=(x y z)\nread 'a[2]' <<IN\nQ\nIN\ntypeset -p a")
	if want := "typeset -a a=( x Q z )"; out != want || st != 0 {
		t.Errorf("read a[2]: got %q status %d, want %q", out, st, want)
	}
}
