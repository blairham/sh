// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// An expansion reads its parameter **once**, and every operator works from
// that one read.
//
// It asks for it more than once by design — a conditional asks whether its
// test fires, the list path asks what the expansion came to, and the scalar
// path then asks for the value behind both — and each of those used to be a
// fresh read. That is invisible for a plain name and wrong wherever a read
// costs something. The cheapest instrument is a subscript, because a
// subscript is **arithmetic** and moves: `a[i++]` names a different element
// every time it is evaluated, so the counter is a count of the reads.
//
// Measured 2026-09-16 against bash 5.3.20 and ksh93u+ 2012-08-01, `env -i`
// with a scratch HOME and no startup files; every row was run against both
// real shells and this one side by side and all three agreed (#3104).
func TestASubscriptIsEvaluatedOncePerExpansion(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The row that was already right, and the reason the others are a
		// finding rather than the shape of the engine: a plain read asks
		// once and always did.
		{"a plain read", `a=(x y z); i=0; echo "[${a[i++]}] i=$i"`, "[x] i=1"},
		// The word forms, which asked **four** times: by the fourth the
		// counter was past the end, the element was unset, and the word was
		// substituted — a wrong value at status 0.
		{"a default word", `a=(x y z); i=0; echo "[${a[i++]-D}] i=$i"`, "[x] i=1"},
		{"and its colon form", `a=(x y z); i=0; echo "[${a[i++]:-D}] i=$i"`, "[x] i=1"},
		// The alternate forms were right, which is what said the second read
		// belonged to the *non-firing* side rather than to the test.
		{"an alternate word", `a=(x y z); i=0; echo "[${a[i++]+S}] i=$i"`, "[S] i=1"},
		{"and its colon form", `a=(x y z); i=0; echo "[${a[i++]:+S}] i=$i"`, "[S] i=1"},
		// The other two conditionals, which asked three times.
		{"an assigning word", `a=(x y z); i=0; echo "[${a[i++]=D}] i=$i"`, "[x] i=1"},
		{"and an erroring one", `a=(x y z); i=0; echo "[${a[i++]?}] i=$i"`, "[x] i=1"},
		// And the operators, which asked twice: the second evaluation moved
		// the subscript, so the operator was applied to the *next* element.
		// `${a[i++]#x}` on `(x y z)` is the empty string in both shells and
		// was `y` here — the trim having missed and the element having moved.
		{"a prefix trim", `a=(x y z); i=0; echo "[${a[i++]#x}] i=$i"`, "[] i=1"},
		{"a replacement", `a=(x y z); i=0; echo "[${a[i++]/x/y}] i=$i"`, "[y] i=1"},
		{"a substring", `a=(x y z); i=0; echo "[${a[i++]:0:1}] i=$i"`, "[x] i=1"},
		{"a length", `a=(x y z); i=0; echo "[${a[i++]}] i=$i"`, "[x] i=1"},
		// A subscript in the *slice* of a whole array is the same question
		// asked of a different operand.
		{"a whole-array slice offset", `a=(x y z); i=0; echo "[${a[@]:i++:1}] i=$i"`, "[x] i=1"},
		// Two nodes in one word, each read once: the hold is per node, so
		// the inner one does not spend the outer one's.
		{
			"a nested expansion keeps its own count",
			`a=(x y z); i=0; j=0; echo "[${a[i++]-${a[j++]}}] i=$i j=$j"`,
			"[x] i=1 j=0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, nil); strings.TrimSpace(out) != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// The hold belongs to **one span**, so a node expanded again gets a fresh
// read: a loop that walks an array with `a[i++]` must advance once per pass
// and not once for the whole loop.
//
// The shape that would fail a hold nothing cleared, and the reason the clear
// is at the top of expandAt rather than at the end of an expansion.
func TestTheHoldDoesNotOutliveItsSpan(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a for loop advances every pass",
			`a=(x y z); i=0; for n in 1 2 3; do printf '%s ' "${a[i++]-D}"; done; echo "i=$i"`,
			"x y z i=3",
		},
		{
			"a while loop too",
			`b=(p q r); k=0; while [ $k -lt 3 ]; do printf '%s ' "${b[k++]-D}"; done; echo "k=$k"`,
			"p q r k=3",
		},
		{
			// Two spellings of the same node in one word are two nodes, so
			// both advance.
			"two expansions in one word",
			`c=(1 2); m=0; echo "[${c[m++]-D}][${c[m++]-D}] m=$m"`,
			"[1][2] m=2",
		},
		{
			// The same node, reached twice through two calls: the AST is
			// one and the reads are two.
			"one node through two calls",
			`d=(u v); p=0; f(){ printf '%s ' "${d[p++]-D}"; }; f; f; echo "p=$p"`,
			"u v p=2",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, nil); strings.TrimSpace(out) != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
			}
		})
	}
}
