// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// An array literal written through a subscript replaces *that element* with
// the words, so the array's length changes by the literal's count less one.
//
// Measured 2026-09-07 against zsh 5.9.2. Every row below is that run, and the
// set was chosen to tell the splice apart from the two readings that agree
// with it on the obvious case: replacing the whole array agrees with a
// one-element array and a subscript of 1, and "insert after the element"
// agrees with everything except an empty literal.
func TestAnArrayLiteralThroughASubscriptSplices(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The three rows the report was filed with.
		{`i=1; a=(x y); a[$i]=(p q); print -r -- "[${a[@]}] n=$#a"`, "[p q y] n=3"},
		{`i=1; a=(x y z); a[$i]=(); print -r -- "n=$#a"`, "n=2"},
		{`i=1; a=(x y); a[$i]+=(p); print -r -- "[${a[@]}]"`, "[x p y]"},
		// In the middle, which is where "replaces the whole array" and
		// "splices" part company visibly.
		{`a=(x y z); a[2]=(p q); print -r -- "[${a[@]}]"`, "[x p q z]"},
		{`a=(x y z); a[2]+=(p q); print -r -- "[${a[@]}]"`, "[x y p q z]"},
		// Past the last element: the subscript has to become an element
		// before it can be replaced, and what it becomes is empty.
		{`a=(x y); a[5]=(p q); print -r -- "n=$#a [${a[3]}][${a[5]}]"`, "n=6 [][p]"},
		{`a=(x y); a[5]+=(p); print -r -- "n=$#a [${a[6]}]"`, "n=6 [p]"},
		// A name holding nothing becomes an array, with the padding in front.
		{`unset a; a[2]=(p q); print -r -- "n=$#a [${a[1]}]"`, "n=3 []"},
		// A negative subscript counts back from the end and asks nothing
		// about the base; past the start it lands in front of everything.
		{`a=(x y z); a[-1]=(p q); print -r -- "[${a[@]}]"`, "[x y p q]"},
		{`a=(x y z); a[-5]=(p); print -r -- "[${a[@]}]"`, "[p x y z]"},
		// The subscript is an expression, and one that will not evaluate ends
		// the script exactly as it does for a scalar element.
		{`a=(x y z); a[x+1]=(p q); print -r -- "[${a[@]}]"`, "[p q y z]"},
		// The words are the ones that same literal would have made on its
		// own, placement and all — `a=([3]=p)` is three positions, so this
		// splices three.
		{`a=(x y z); a[2]=([3]=p); print -r -- "n=$#a [${a[4]}]"`, "n=5 [p]"},
		// The attribute a write folds through is applied to the whole array
		// afterwards, as it is for every other write.
		{`typeset -U a=(1 2 3); a[2]=(1 9); print -r -- "[${a[@]}]"`, "[1 9 3]"},
		// A declared array holding nothing takes the splice: the letter is
		// what says the name is an array rather than a scalar.
		{`typeset -a a; a[2]=(p q); print -r -- "n=$#a [${a[2]}]"`, "n=3 [p]"},
		// And the declaration's own operand reaches it, which needs the one
		// thing the route makes hard: the utility is handed the name bare
		// and brings it into being before the literal lands, so what the
		// name holds by then is this line's own declaration.
		{`typeset a[2]=(p q); print -r -- "n=$#a [${a[3]}]"`, "n=3 [q]"},
		{`typeset b[1]=(p q); print -r -- "n=$#b"`, "n=2"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// The two names a splice has nowhere to land in, worded apart from each other
// and both without the subscript. Measured in the same run.
func TestAnArrayLiteralThroughASubscriptNeedsAnArray(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`s=abc; s[2]=(p q); echo after`, "s: attempt to assign array value to non-array"},
		{`s=""; s[1]=(p q); echo after`, "s: attempt to assign array value to non-array"},
		{`s=abc; s[1]+=(p); echo after`, "s: attempt to assign array value to non-array"},
		{`typeset -A h; h=(k v); h[k]=(p q); echo after`, "h: attempt to set slice of associative array"},
		{`typeset a; a[2]=(p q); echo after`, "a: attempt to assign array value to non-array"},
		// A declaration whose name already held a string is refused too:
		// what tells it from the row above is whether the declaration had a
		// value of its own to leave alone.
		{`s=abc; typeset s[1]=(p q); echo after`, "s: attempt to assign array value to non-array"},
		{`typeset -A h; typeset h[k]=(p q); echo after`, "h: attempt to set slice of associative array"},
		// Below the first element is the boundary the scalar spelling
		// already refuses, and it is refused here in the same words.
		{`a=(x y); a[0]=(p q); echo after`, "a: assignment to invalid subscript range"},
	} {
		out, st := answersRun(t, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s:\n  said %q\n  want it to contain %q", tc.src, out, tc.want)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s: the rest of the line ran; the refusal should end the script", tc.src)
		}
		if st != 1 {
			t.Errorf("%s: status %d, want 1", tc.src, st)
		}
	}
}
