// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A subscript written `lo..hi`, where the grammar reads that as a range. The
// flag is named here and the shell that sets it is not.

func subDots() Dialect {
	d := Core()
	d.ArraySubscript = true
	d.SubscriptDotRange = true
	return d
}

// spanKinds renders a word's spans as kind/quoting/value, which tells a split
// between a substitution and a literal from a split inside one.
func spanKinds(w *Word) string {
	out := ""
	for _, s := range w.Spans {
		switch s.Kind {
		case Literal:
			out += "L"
		case ArithSubst:
			out += "A"
		case ParamExp:
			out += "P"
		default:
			out += "?"
		}
		if s.Quoting != Unquoted {
			out += "q"
		}
		out += "(" + s.Value + ")"
	}
	return out
}

// The first `..` written in a literal is the separator, quoted or not, and
// the ends are the words on either side of it.
func TestADotRangeIsSplitAtTheFirstWrittenDots(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src    string
		lo, hi string
	}{
		{`echo ${a[1..3]}`, "L(1)", "L(3)"},
		{`echo ${a[..2]}`, "", "L(2)"},
		{`echo ${a[2..]}`, "L(2)", ""},
		{`echo ${a[..]}`, "", ""},
		{`echo ${a[i..i+2]}`, "L(i)", "L(i+2)"},
		{`echo ${a["1..3"]}`, "Lq(1)", "Lq(3)"},
		{`echo ${a[1"..3"]}`, "L(1)", "Lq(3)"},
		{`echo ${a[$((1))..$((3))]}`, "A(1)", "A(3)"},
		{`echo ${a[1..3..]}`, "L(1)", ""},
		{`echo ${a[0..9..3]}`, "L(0)", "L(3)"},
		{`echo ${a[1..1+..3]}`, "L(1)", "L(3)"},
		{`echo ${a[1..3"..2"]}`, "L(1)", "Lq(2)"},
		{`echo ${a[1...3]}`, "L(1)", "L(.3)"},
		{`echo ${a[1....3]}`, "L(1)", "L(3)"},
		{`echo ${a[(1..3)]}`, "L(()L(1)", "L(3)L())"},
	} {
		e := firstParam(t, tc.src, subDots())
		if e.IndexDots == nil {
			t.Errorf("%s: no range read", tc.src)
			continue
		}
		lo, hi := spanKinds(e.IndexDots.Lo.Text), spanKinds(e.IndexDots.Hi.Text)
		if lo != tc.lo || hi != tc.hi {
			t.Errorf("%s: ends %q and %q, want %q and %q", tc.src, lo, hi, tc.lo, tc.hi)
		}
		if e.Index == nil || e.Op != 0 || e.Bad {
			t.Errorf("%s: the subscript itself was not kept whole", tc.src)
		}
	}
}

// A `..` that was not written as one in a literal is no separator: escaped,
// produced by a substitution, or inside a substitution's own operand. And a
// grammar without the flag never reads one.
func TestDotsNotWrittenInALiteralAreNoRange(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`echo ${a[1\..3]}`,
		`echo ${a[$x]}`,
		`echo ${a[1${x}3]}`,
		`echo ${a[${x:-1..2}]}`,
		`echo ${a[1.3]}`,
		`echo ${a[@]}`,
	} {
		if e := firstParam(t, src, subDots()); e.IndexDots != nil {
			t.Errorf("%s: read as a range", src)
		}
	}
	d := subDots()
	d.SubscriptDotRange = false
	if e := firstParam(t, `echo ${a[1..3]}`, d); e.IndexDots != nil {
		t.Errorf("a grammar without the flag read a range")
	}
}
