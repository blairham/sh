// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A literal whose elements name where their values go is traced here as the
// writes it performs, one line each — not as the literal it was written as.
// It is the same shape a compound literal already has, and it is a line
// *count* rather than a rendering, which is why it is an axis of its own
// beside the one that decides whether the elements are shown expanded.
//
// Measured 2026-09-17 against ksh93u+ 2012-08-01, a script file, `env -i`
// with LC_ALL=C. Ours wrote one line holding the literal as written (#2866).
func TestASubscriptedLiteralIsTracedAsTheWritesItPerforms(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"a=([2]=c [0]=a)", "+ a[2]=c\n+ a[0]=a\n"},
		{"a=([2]=c)", "+ a[2]=c\n"},
		{"a=([2]='p q')", "+ a[2]='p q'\n"},
		{"a=([k]=)", "+ a[k]=''\n"},
		// The literal's own append is not written: what it performs is the
		// element write, and that is what the line says. An append on the
		// element is kept for the same reason.
		{"a+=([5]=z)", "+ a[5]=z\n"},
		{"a=([2]+=c)", "+ a[2]+=c\n"},
		// The subscript is the value it came to, which falls out of the
		// element list being expanded once by whoever stores it.
		{"x=2\na=([$((x++))]=c)", "+ a[2]=c\n"},
		// The controls. A literal of plain words is one line in every
		// column, and a mixed literal is one line here too — the bracketed
		// word is not read as a subscript at all.
		{"b=(1 2)", "+ b=( 1 2 )\n"},
		{"a=(p [2]=c)", "+ a=( p '[2]=c' )\n"},
	} {
		out, st := answersRun(t, "set -x\n"+tc.src+"\nset +x\n")
		if st != 0 {
			t.Errorf("%s: status %d, want 0: %q", tc.src, st, out)
			continue
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: got %q, want it to contain %q", tc.src, out, tc.want)
		}
	}
}

// And an empty literal writes nothing at all, which is what having no element
// to write comes to.
func TestAnEmptySubscriptedLiteralWritesNoLine(t *testing.T) {
	out, st := answersRun(t, "set -x\na=()\nset +x\n")
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if strings.Contains(out, "a=") || strings.Contains(out, "a[") {
		t.Errorf("got %q, want no line naming the assignment", out)
	}
}
