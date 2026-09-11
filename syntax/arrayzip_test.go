// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// zipping is the core plus the array-zip operators. The flag is named here
// and the shell that sets it is not, which is what keeps this package free of
// dialect names.
func zipping() Dialect {
	d := Core()
	d.ParamArrayZip = true
	return d
}

// Both operators parse, and the doubled one is not the single one followed by
// a `^` in its operand — which is the whole of the disambiguation, since the
// longer spelling has to be tried first.
func TestTheArrayZipOperatorsParse(t *testing.T) {
	for _, tc := range []struct {
		src  string
		op   ParamOp
		arg  string
		name string
	}{
		{`echo ${a:^b}`, ParamZip, "b", "the zip"},
		{`echo ${a:^^b}`, ParamZipCycle, "b", "the cycling zip"},
		{`echo ${a:^}`, ParamZip, "", "a zip with no operand"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := firstParam(t, tc.src, zipping())
			if e.Op != tc.op {
				t.Fatalf("op = %v (%q), want %v (%q)", e.Op, e.Op, tc.op, tc.op)
			}
			got := ""
			if e.Arg != nil {
				got = e.Arg.Literal()
			}
			if got != tc.arg {
				t.Errorf("operand = %q, want %q", got, tc.arg)
			}
		})
	}
}

// Without the flag the colon is the substring it has always been, and the
// `^` goes to the arithmetic that reads the offset. A grammar flag that
// widened the core would take `${v:^…}` away from every dialect that does not
// have the operator, which is a change to shapes the whole panel shares.
func TestWithoutTheFlagAZipIsStillASubstring(t *testing.T) {
	for _, src := range []string{`echo ${a:^b}`, `echo ${a:^^b}`} {
		t.Run(src, func(t *testing.T) {
			if e := firstParam(t, src, Core()); e.Op != ParamSubstring {
				t.Errorf("op = %v (%q), want %v (%q)", e.Op, e.Op, ParamSubstring, ParamSubstring)
			}
		})
	}
}

// The flag moves only its own two spellings: everything else after a colon is
// what it was, including the element selection that shares the position when
// a dialect has both.
func TestAColonBeforeAnythingElseIsUnchangedByTheZip(t *testing.T) {
	both := zipping()
	both.ParamElementSelection = true
	for _, tc := range []struct {
		src string
		op  ParamOp
	}{
		{`echo ${v:2}`, ParamSubstring},
		{`echo ${v:2:2}`, ParamSubstring},
		{`echo ${v: -2}`, ParamSubstring},
		{`echo ${v:+set}`, ParamAlternate},
		{`echo ${v:-alt}`, ParamDefault},
		{`echo ${v:=set}`, ParamAssign},
		{`echo ${v:?msg}`, ParamError},
		{`echo ${v:}`, ParamSubstring},
		{`echo ${a:#p}`, ParamExclude},
		{`echo ${a:|b}`, ParamSetDifference},
		{`echo ${a:*b}`, ParamSetIntersection},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if e := firstParam(t, tc.src, both); e.Op != tc.op {
				t.Errorf("op = %v (%q), want %v (%q)", e.Op, e.Op, tc.op, tc.op)
			}
		})
	}
}

// The printer writes back what it read, for both spellings. A formatter that
// renders `:^^` as `:^` changes the program, and the operator's String is the
// only thing standing between the two.
func TestTheZipOperatorsPrintBack(t *testing.T) {
	for _, src := range []string{`echo ${a:^b}`, `echo ${a:^^b}`, `echo ${a:^b} ${c:^^d}`} {
		t.Run(src, func(t *testing.T) {
			f, err := Parse(src, zipping())
			if err != nil {
				t.Fatalf("parse %q: %v", src, err)
			}
			if got := strings.TrimSpace(Print(f)); got != src {
				t.Errorf("printed %q, want %q", got, src)
			}
		})
	}
}
