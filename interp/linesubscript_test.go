// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The `(f)` letter in a subscript's flag group counts a *string* through its
// lines rather than its characters. Named by the grammar flag and not by a
// shell, as every test in this package is: interp/linesubscript.go holds the
// measurements the rows below are taken from.
const threeLines = "v='a\nb\nc'\n"

// withRanges is the one answer the rows about a pair need and the rest do
// not: a comma in a subscript separates the pair rather than being the
// arithmetic operator. Without it there is no range to write a group in
// front of.
func withRanges(sem *Semantics) { sem.SubscriptCommaIsARange = Yes }

// A plain `(f)` subscript is the line at that position, clamped at both ends.
//
// The clamp is what separates this from an ordinary subscript, which answers
// nothing outside the value, and it is why every row past the last line is
// here rather than left to the single one that would have implied them.
func TestALineSubscriptNamesALineOfAString(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" "${v[(f)1]}"`, `[a]`},
		{`printf "[%s]" "${v[(f)2]}"`, `[b]`},
		{`printf "[%s]" "${v[(f)3]}"`, `[c]`},
		{`printf "[%s]" "${v[(f)4]}"`, `[c]`},
		{`printf "[%s]" "${v[(f)9]}"`, `[c]`},
		{`printf "[%s]" "${v[(f)0]}"`, `[a]`},
		{`printf "[%s]" "${v[(f)-1]}"`, `[c]`},
		{`printf "[%s]" "${v[(f)-2]}"`, `[b]`},
		{`printf "[%s]" "${v[(f)-9]}"`, `[a]`},
		// The whole line comes back, so a length is the line's and not a
		// count of lines.
		{`printf "[%s]" "${#v[(f)2]}"`, `[1]`},
		// The subscript is an expression like any other.
		{`printf "[%s]" "${v[(f)1+1]}"`, `[b]`},
	} {
		out, st := runSub(t, threeLines+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// Which lines a value has: a run of separators is one separator, a run at the
// front makes no empty line, and a run at the end makes exactly one.
//
// Measured rather than derived from a split, and the rows are why: a split on
// every newline would put an empty line at the front of the second value and
// two at the end of the fifth.
func TestWhichLinesAStringHasForALineSubscript(t *testing.T) {
	for _, tc := range []struct{ set, src, want string }{
		{"v='\na\nb'", `printf "[%s]" "${v[(f)1]}" "${v[(f)2]}"`, `[a][b]`},
		{"v='\n\na\nb'", `printf "[%s]" "${v[(f)1]}" "${v[(f)2]}"`, `[a][b]`},
		{"v='a\n\nb'", `printf "[%s]" "${v[(f)1]}" "${v[(f)2]}"`, `[a][b]`},
		{"v='a\nb\n'", `printf "[%s]" "${v[(f)3]}" "${v[(f)-2]}"`, `[][b]`},
		{"v='a\nb\n\n'", `printf "[%s]" "${v[(f)3]}" "${v[(f)-2]}"`, `[][b]`},
		{"v='\n'", `printf "[%s]" "${v[(f)1]}" "${v[(f)2]}"`, `[][]`},
		{"v=abc", `printf "[%s]" "${v[(f)1]}" "${v[(f)9]}"`, `[abc][abc]`},
		{"v=", `printf "[%s]" "${v[(f)1]}"`, `[]`},
	} {
		out, st := runSub(t, tc.set+"\n"+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s; %s = %q (status %d), want %q at 0", tc.set, tc.src, out, st, tc.want)
		}
	}
}

// A `(f)` subscript names a character *position*, and only a subscript
// written without a comma reads the whole line at it.
//
// This is the half that does not follow from the rows above: the first end of
// a pair is where its line begins and the second end is an ordinary character
// index unless it carries a group of its own.
func TestALineSubscriptInARangeNamesACharacterPosition(t *testing.T) {
	const v = "v='aaa\nbbb\nccc'\n"
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" "${v[(f)1,2]}"`, `[aa]`},
		{`printf "[%s]" "${v[(f)2,2]}"`, `[]`},
		{`printf "[%s]" "${v[(f)2,7]}"`, `[bbb]`},
		{`printf "[%s]" "${v[(f)1,3]}"`, `[aaa]`},
		{`printf "[%s]" "${v[(f)5,-1]}"`, `[ccc]`},
		{`printf "[%s]" "${v[(f)0,3]}"`, `[aaa]`},
		{`printf "[%s]" "${v[(f)-2,5]}"`, `[b]`},
		// A group in the second end ends at the line rather than beginning
		// at it, so the two ends are not the same question.
		{`printf "[%s]" "${v[1,(f)1]}"`, `[aaa]`},
		{`printf "[%s]" "${v[(f)2,(f)2]}"`, `[bbb]`},
		{`printf "[%s]" "${v[1,(f)9]}"`, "[aaa\nbbb\nccc]"},
	} {
		out, st := runSub(t, v+tc.src, withRanges)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The trailing empty line never moved the end, so a range ending at it ends
// where the line before it ended. The one place that line's two positions
// differ, and measured rather than derived.
func TestARangeEndingAtATrailingEmptyLine(t *testing.T) {
	const v = "v='aa\nbb\n'\n"
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" "${v[1,(f)2]}"`, "[aa\nbb]"},
		{`printf "[%s]" "${v[1,(f)3]}"`, "[aa\nbb]"},
		{`printf "[%s]" "${v[(f)3,-1]}"`, `[]`},
		{`printf "[%s]" "${v[(f)3,(f)3]}"`, `[]`},
	} {
		out, st := runSub(t, v+tc.src, withRanges)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A search beside `(f)` walks the lines as a search over an array walks its
// elements, and substitutes the character position rather than the number.
func TestASearchOverTheLinesOfAString(t *testing.T) {
	const v = "v='aa\nbb\ncc'\n"
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" "${v[(fr)bb]}"`, `[bb]`},
		{`printf "[%s]" "${v[(fR)*b*]}"`, `[bb]`},
		{`printf "[%s]" "${v[(fr)b*]}"`, `[bb]`},
		{`printf "[%s]" "${v[(fi)aa]}"`, `[1]`},
		{`printf "[%s]" "${v[(fi)bb]}"`, `[4]`},
		{`printf "[%s]" "${v[(fI)*]}"`, `[7]`},
		{`printf "[%s]" "${v[(fi)*]}"`, `[1]`},
		{`printf "[%s]" "${v[(fie)bb]}"`, `[4]`},
		{`printf "[%s]" "${v[(fin:2:)*]}"`, `[4]`},
		{`printf "[%s]" "${v[(fib:2:)*]}"`, `[4]`},
		// The match is the whole line, so a prefix of one is a miss — which
		// is where this parts company with a search over characters.
		{`printf "[%s]" "${v[(fi)b]}"`, `[0]`},
		// A walk that ran and matched nothing has no position at all, where
		// an array's forward search answers one past its last element.
		{`printf "[%s]" "${v[(fi)zz]}"`, `[0]`},
		{`printf "[%s]" "${v[(fI)zz]}"`, `[0]`},
		{`printf "[%s]" "${v[(fr)zz]}"`, `[]`},
		{`printf "[%s]" "${v[(fR)zz]}"`, `[]`},
		// And that position read as a range's start.
		{`printf "[%s]" "${v[(fr)bb,-1]}"`, "[bb\ncc]"},
		{`printf "[%s]" "${v[(fr)zz,-1]}"`, "[aa\nbb\ncc]"},
		{`printf "[%s]" "${v[1,(fr)cc]}"`, "[aa\nbb\ncc]"},
	} {
		out, st := runSub(t, v+tc.src, withRanges)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A walk that never ran keeps the array's own misses, which is the half that
// would have been guessed wrong: only a walk that ran and matched nothing
// answers with no position.
func TestALineSearchThatNeverRan(t *testing.T) {
	const v = "v='aa\nbb\ncc\ndd'\n"
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" "${v[(fib:9:)bb]}"`, `[5]`},
		{`printf "[%s]" "${v[(fIb:9:)bb]}"`, `[0]`},
		{`printf "[%s]" "${v[(fib:-9:)bb]}"`, `[0]`},
		{`printf "[%s]" "${v[(fib:3:)bb]}"`, `[0]`},
		{`printf "[%s]" "${v[(fib:3:)dd]}"`, `[10]`},
		{`e=; printf "[%s]" "${e[(fi)x]}"`, `[0]`},
		{`e=; printf "[%s]" "${e[(fI)x]}"`, `[0]`},
	} {
		out, st := runSub(t, v+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The letter says what a *string* is counted through, so a list of values is
// not this construct: an array counts its elements and answers nothing
// outside them, where a string's clamp would have answered the last line.
func TestALineSubscriptOverAListIsTheOrdinaryReading(t *testing.T) {
	const a = "a=(p q r)\n"
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" "${a[(f)2]}"`, `[q]`},
		{`printf "[%s]" "${a[(f)9]}"`, `[]`},
		{`printf "[%s]" "${a[(f)0]}"`, `[]`},
		{`printf "[%s]" "${a[(f)-1]}"`, `[r]`},
		{`printf "[%s]" "${a[(f)1,2]}"`, `[p q]`},
		{`printf "[%s]" "${a[(fr)q]}"`, `[q]`},
		{`printf "[%s]" "${a[(fi)q]}"`, `[2]`},
		{`printf "[%s]" "${a[(fI)q]}"`, `[2]`},
		// An element carrying a newline is still one element.
		{`b=('x\ny' z); printf "[%s]" "${b[(f)1]}"`, `[x\ny]`},
	} {
		out, st := runSub(t, a+tc.src, withRanges)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A table has no lines either, so the letter says nothing and the key reading
// is left to answer.
func TestALineSubscriptOverATableIsTheKeyReading(t *testing.T) {
	const m = "typeset -A m\nm=(k1 v1 k2 v2)\n"
	for _, tc := range []struct{ src, want string }{
		{`printf "[%s]" "${m[(f)k1]}"`, `[v1]`},
		{`printf "[%s]" "${m[(f)nosuch]}"`, `[]`},
		{`printf "[%s]" "${m[(fr)v1]}"`, `[v1]`},
	} {
		out, st := runAssoc(t, m+tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// Writing through one is refused by name, because the letter names a span of
// characters and that side of the construct answers with one subscript.
//
// The index of the line's first character is what it would be handed, and
// writing there would put the value *inside* the line — a plausible wrong
// string at status 0, which is the one thing a refusal by name exists to
// prevent. See interp/subscriptflags.go for what the write is measured to be.
func TestWritingThroughALineSubscriptIsRefusedByName(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v[(f)2]=ZZ`, `(f)`},
		{`v[(f)2]+=ZZ`, `(f)`},
		{`v[(fr)b]=ZZ`, `(f)`},
	} {
		out, st := runSub(t, threeLines+tc.src+`
printf "[%s]" "$v"`)
		if !strings.Contains(out, tc.want+" subscript flag is not implemented") || st == 0 {
			t.Errorf("%s = %q (status %d), want a refusal naming %s", tc.src, out, st, tc.want)
		}
		if strings.Contains(out, "ZZ") {
			t.Errorf("%s = %q, wrote through a refused subscript", tc.src, out)
		}
	}
}

// And a list is not a string, so the same spelling over one is the ordinary
// write it always was.
func TestWritingThroughALineSubscriptOverAListIsOrdinary(t *testing.T) {
	out, st := runSub(t, "a=(p q r)\na[(f)2]=ZZ\n"+`printf "[%s]" "${a[@]}"`)
	if out != `[p][ZZ][r]` || st != 0 {
		t.Errorf("a[(f)2]=ZZ = %q (status %d), want [p][ZZ][r] at 0", out, st)
	}
}
