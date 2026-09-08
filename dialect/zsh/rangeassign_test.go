// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A subscript *range* on the left of an assignment names a span of elements
// and the words replace the whole span, so the array's length changes by the
// words' count less the span's — it grows, shrinks or holds.
//
// Measured 2026-09-07 against zsh 5.9.2, which is the only panel member with
// a range on the left at all. Every row is that run.
//
// The fields are printed with delimiters and never counted alone, because a
// count is a hash of the answer here: an off-by-one at either end of a span
// leaves the length right and the contents wrong, and the bug this closes
// (#1411) was found only because someone read the values. The count is carried
// alongside rather than instead, since the empty-literal rows have no fields
// to print.
//
// The set is chosen to tell the splice apart from the reading it replaced —
// the arithmetic comma operator, whose value is its right operand, which made
// `a[2,3]=(x y)` run as `a[3]=(x y)` and grow the array to four. That reading
// agrees with this one on nothing except a pair whose ends are equal, which is
// why that row is here too: it is the one case where no axis is asked.
func TestASubscriptRangeOnTheLeftReplacesTheSpan(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The report's own row. Three elements in, three out.
		{`a=(1 2 3); a[2,3]=(x y)`, "[1][x][y] n=3"},
		// Fewer words than the span covers shrinks it, more grows it. The
		// splice that never removed the span was wrong in the same direction
		// for all three counts, so one row could not have told them apart.
		{`a=(1 2 3); a[2,3]=(x)`, "[1][x] n=2"},
		{`a=(1 2 3); a[2,3]=(x y z)`, "[1][x][y][z] n=4"},
		{`a=(1 2 3); a[2,3]=(x y z w)`, "[1][x][y][z][w] n=5"},
		// No words at all deletes the span.
		{`a=(1 2 3); a[2,3]=()`, "[1] n=1"},
		// A span in the middle of a longer array, which is where a splice
		// that closed up from the wrong end would show.
		{`a=(1 2 3 4 5); a[2,4]=(x)`, "[1][x][5] n=3"},
		// An end past the last element *is* the last, so this is the span 2
		// through 3 and not a pad out to eleven.
		{`a=(1 2 3); a[2,10]=(x y)`, "[1][x][y] n=3"},
		// Each end takes the negative rule a single subscript takes, counted
		// back from the last element.
		{`a=(1 2 3); a[2,-1]=(x)`, "[1][x] n=2"},
		{`a=(1 2 3); a[-2,-1]=(x y)`, "[1][x][y] n=3"},
		{`a=(1 2 3); a[-1,-1]=(x y)`, "[1][2][x][y] n=4"},
		// A start below the first element is the first element.
		{`a=(1 2 3); a[0,2]=(x y)`, "[x][y][3] n=3"},
		{`a=(1 2 3); a[0,1]=(x)`, "[x][2][3] n=3"},
		{`a=(1 2 3); a[0,3]=(x)`, "[x] n=1"},
		{`a=(1 2 3); a[0,-1]=(x)`, "[x] n=1"},
		// An end before the start is a span with nothing in it, so the words
		// are *inserted* where it would have begun and nothing comes out.
		// The same answer `unset "a[2,1]"` gives, which is what makes the two
		// one reading rather than two.
		{`a=(1 2 3); a[3,2]=(x)`, "[1][2][x][3] n=4"},
		{`a=(1 2 3); a[2,1]=(x y)`, "[1][x][y][2][3] n=5"},
		{`a=(1 2 3); a[3,0]=(x)`, "[1][2][x][3] n=4"},
		{`a=(1 2 3); a[-1,-2]=(x)`, "[1][2][x][3] n=4"},
		// Including at the very front, which the arithmetic reading refused
		// outright: `1,0` is `0` there, and `a[0]=(x)` is below the first
		// element, so a line that inserts ended the script instead.
		{`a=(1 2 3); a[1,0]=(x)`, "[x][1][2][3] n=4"},
		{`a=(1 2 3); a[-5,-4]=(x)`, "[x][1][2][3] n=4"},
		// A negative start reaching past the front on an empty array has
		// nowhere to insert but the front.
		{`a=(); a[-2,-1]=(x)`, "[x] n=1"},
		// A start past the last element has nothing to replace: the gap
		// becomes empty elements and the words follow them, which is the
		// padding a single subscript already does for `a[5]=(x)`.
		{`a=(1 2 3); a[5,6]=(x)`, "[1][2][3][][x] n=5"},
		{`a=(1 2 3); a[5,5]=(x)`, "[1][2][3][][x] n=5"},
		// A name holding nothing becomes an array, and the padding runs from
		// zero elements — one empty, not two.
		{`unset a; a[2,3]=(x y)`, "[][x][y] n=3"},
		{`a=(); a[2,3]=(x y)`, "[][x][y] n=3"},
		// A pair whose ends are equal is that subscript under either reading,
		// so it is answered by the single subscript and no axis is asked.
		{`a=(1 2 3); a[2,2]=(x y)`, "[1][x][y][3] n=4"},
		// Both ends are expressions, the same as a single subscript is.
		{`a=(1 2 3); a[1+1,2+1]=(x y)`, "[1][x][y] n=3"},
		{`a=(1 2 3); i=2; j=3; a[$i,$j]=(x y)`, "[1][x][y] n=3"},
		// The words are the ones the literal would have made on its own,
		// placement and all: `([2]=p)` is two positions, so this splices two.
		{`a=(1 2 3); a[2,3]=([2]=p)`, "[1][][p] n=3"},
		// A value's own spaces survive, the words being words and not fields
		// re-split.
		{`a=(1 2 3); a[2,3]=("x y" z)`, "[1][x y][z] n=3"},
		// A plain value rather than a literal is the one word that replaces
		// the span, so the two spellings differ only in how many words there
		// are — including when the word is empty.
		{`a=(1 2 3); a[2,3]=x`, "[1][x] n=2"},
		{`a=(1 2 3); a[3,2]=x`, "[1][2][x][3] n=4"},
		{`a=(1 2 3); a[2,3]=`, "[1][] n=2"},
		{`unset a; a[2,3]=x`, "[][x] n=2"},
		// The left of an assignment and the inside of an expansion disagree
		// about a second comma: `${a[1,2,3]}` is a bad substitution, while
		// this is the span 1 through the arithmetic `2,3`, which is 3.
		{`a=(1 2 3); a[1,2,3]=(x y)`, "[x][y] n=2"},
		// A declaration's own operand takes the same reading.
		{`a=(1 2 3); typeset a[2,3]=(x y)`, "[1][x][y] n=3"},
		// The attributes a write folds through are applied afterwards, as
		// they are for every other write.
		{`typeset -U a=(1 2 3); a[2,3]=(1 9)`, "[1][9] n=2"},
		// Twice over, because the second splice reads the length the first
		// left rather than the one the script started with — which is the
		// shape the failure surfaced in.
		{`a=(1 2 3); a[2,3]=(x y); a[2,3]=(p q r)`, "[1][p][q][r] n=4"},
	} {
		out, st := answersRun(t, tc.src+`; printf '[%s]' "${a[@]}"; print -r -- " n=$#a"`)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
		if st != 0 {
			t.Errorf("%s: status %d, want 0", tc.src, st)
		}
	}
}

// `+=` reads no range at all: it takes the arithmetic-comma value of the whole
// subscript, exactly as it did before ranges were read on the left.
//
// Measured in the same run, and the first row is the only one that can tell
// the two hypotheses apart. A span clamped to the last element would append at
// four elements; the arithmetic comma's `10` pads out to eleven, and eleven is
// what real zsh gives. Every shorter subscript — `a[2,3]+=(x)`, `a[1,2]+=(x)`,
// `a[3,2]+=(x)` — happens to agree under both readings, so a suite built from
// those alone would have graded a range-reading `+=` as correct.
func TestAppendingThroughASubscriptRangeReadsNoRange(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The discriminating row.
		{`a=(1 2 3); a[2,10]+=(x)`, "[1][2][3][][][][][][][][x] n=11"},
		// And the rows that agree under both, kept so the whole construct is
		// pinned rather than only its sharpest corner.
		{`a=(1 2 3); a[2,3]+=(x)`, "[1][2][3][x] n=4"},
		{`a=(1 2 3); a[1,2]+=(x)`, "[1][2][x][3] n=4"},
		{`a=(1 2 3); a[3,2]+=(x)`, "[1][2][x][3] n=4"},
		{`a=(1 2 3); a[0,2]+=(x)`, "[1][2][x][3] n=4"},
		{`a=(1 2 3); a[2,-1]+=(x)`, "[1][2][3][x] n=4"},
		{`a=(1 2 3); a[1,2,3]+=(x)`, "[1][2][3][x] n=4"},
		// A plain value on the same route joins element *3*, the operand the
		// arithmetic comma names, rather than replacing a span.
		{`a=(1 2 3); a[2,3]+=x`, "[1][2][3x] n=3"},
	} {
		out, st := answersRun(t, tc.src+`; printf '[%s]' "${a[@]}"; print -r -- " n=$#a"`)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
		if st != 0 {
			t.Errorf("%s: status %d, want 0", tc.src, st)
		}
	}
}

// What a range on the left refuses, and the refusal ends the script.
//
// The whole diagnostic is compared rather than searched for, because a
// substring test cannot see a prefix somebody added — and the array is read
// back afterwards in the rows that leave one, so a refusal that also wrote is
// not mistaken for a clean one.
func TestASubscriptRangeOnTheLeftRefuses(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// A span wholly below the first element, which is the rule
		// `unset "a[0,0]"` follows: `a[0,1]` begins out of reach and ends
		// inside and is accepted, and this lies entirely out of reach.
		{`a=(1 2 3); a[0,0]=(x); echo after`, "zsh:1: a: assignment to invalid subscript range\n"},
		{`a=(1 2 3); a[0,0]=(); echo after`, "zsh:1: a: assignment to invalid subscript range\n"},
		{`a=(1 2 3); a[0,0]=x; echo after`, "zsh:1: a: assignment to invalid subscript range\n"},
		// A string has no span of elements to replace, and the sentence is
		// the one the single-subscript spelling already says.
		{`a=hello; a[2,3]=(x y); echo after`, "zsh:1: a: attempt to assign array value to non-array\n"},
		// A table has keys rather than positions, so a literal's words have
		// no span to land in — worded apart from the string's refusal.
		{`typeset -A h; h=(k v); h[a,b]=(x y); echo after`, "zsh:1: h: attempt to set slice of associative array\n"},
		// An end that will not evaluate is blamed as arithmetic and ends the
		// script, the same complaint the identical text inside `$(( ))`
		// makes — the range reading must not swallow it and splice anyway.
		{`a=(1 2 3); a[2,3/0]=(x y); echo after`, "zsh:1: division by zero\n"},
		{`a=(1 2 3); a[3/0,2]=(x y); echo after`, "zsh:1: division by zero\n"},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, out, tc.want)
		}
		if st != 1 {
			t.Errorf("%s: status %d, want 1", tc.src, st)
		}
	}
}

// A table's subscript is the one place a comma means nothing at all: with a
// plain value the characters are part of the *key*, so `h[a,b]=x` stores under
// the three characters and leaves every other key alone.
//
// The row a range fix is likeliest to break, because it reaches for the comma
// before the attribute has been asked about. `assoc/the-subscript-is-not-
// arithmetic` is the same claim at `1+1`.
func TestATableSubscriptWithACommaIsAKey(t *testing.T) {
	const src = `typeset -A h; h=(k v); h[a,b]=x; ` +
		`print -r -- "n=$#h [${h[a,b]}] [${h[k]}] [${h[a]-none}]"`
	out, st := answersRun(t, src)
	const want = "n=2 [x] [v] [none]\n"
	if out != want {
		t.Errorf("%s:\n  said %q\n  want %q", src, out, want)
	}
	if st != 0 {
		t.Errorf("%s: status %d, want 0", src, st)
	}
}
