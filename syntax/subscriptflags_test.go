// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// subFlagged is the core plus subscripts plus a subscript's own flag group.
// The flag is named here and the shell that sets it is not.
//
// ArraySubscript travels with it because a group has nowhere to be without a
// subscript to open.
func subFlagged() Dialect {
	d := Core()
	d.ArraySubscript = true
	d.ArraySubscriptFlags = true
	return d
}

// operandText is the operand behind the group, joined from its spans, which is
// what an expansion-free case can assert exactly.
func operandText(w *Word) string {
	out := ""
	for _, s := range w.Spans {
		out += s.Value
	}
	return out
}

// The group is read off the front of the subscript and the operand is what is
// left, both exactly.
func TestASubscriptFlagGroupIsReadAndTheOperandIsWhatIsLeft(t *testing.T) {
	for _, tc := range []struct {
		src     string
		flags   string
		groupIs string
		operand string
		nth     string
		begin   string
		sep     string
	}{
		{`echo ${a[(r)beta]}`, "r", "(r)", "beta", "", "", ""},
		{`echo ${a[(re)beta]}`, "re", "(re)", "beta", "", "", ""},
		{`echo ${a[(R)*a]}`, "R", "(R)", "*a", "", "", ""},
		{`echo ${a[(I)x]}`, "I", "(I)", "x", "", "", ""},
		{`echo ${a[(rn:2:)x]}`, "rn", "(rn:2:)", "x", "2", "", ""},
		{`echo ${a[(rb:-1:)x]}`, "rb", "(rb:-1:)", "x", "", "-1", ""},
		{`echo ${a[(rn:1+1:b:3:)x]}`, "rnb", "(rn:1+1:b:3:)", "x", "1+1", "3", ""},
		{`echo ${a[(ws:,:)2]}`, "ws", "(ws:,:)", "2", "", "", ","},
		// The delimiter is whatever character arrives, and the four matched
		// pairs close with their partners.
		{`echo ${a[(rn/2/)x]}`, "rn", "(rn/2/)", "x", "2", "", ""},
		{`echo ${a[(rn[2])x]}`, "rn", "(rn[2])", "x", "2", "", ""},
		// An empty group is still a group, and the operand is the whole
		// subscript.
		{`echo ${a[()2]}`, "", "()", "2", "", "", ""},
		// The operand may be empty too.
		{`echo ${a[(r)]}`, "r", "(r)", "", "", "", ""},
	} {
		e := firstParam(t, tc.src, subFlagged())
		if e.IndexFlags == nil {
			t.Errorf("%s: no flag group read", tc.src)
			continue
		}
		g := e.IndexFlags
		if g.Flags != tc.flags || g.Src != tc.groupIs {
			t.Errorf("%s: flags=%q src=%q, want %q %q", tc.src, g.Flags, g.Src, tc.flags, tc.groupIs)
		}
		if got := operandText(g.Arg); got != tc.operand {
			t.Errorf("%s: operand=%q, want %q", tc.src, got, tc.operand)
		}
		if g.Nth != tc.nth || g.Begin != tc.begin || g.Sep != tc.sep {
			t.Errorf("%s: nth=%q begin=%q sep=%q, want %q %q %q",
				tc.src, g.Nth, g.Begin, g.Sep, tc.nth, tc.begin, tc.sep)
		}
		// The subscript as written survives beside the operand, because a
		// diagnostic names what the script wrote.
		if got := operandText(e.Index); got == tc.operand && tc.groupIs != "" {
			t.Errorf("%s: Index lost the group, holding only %q", tc.src, got)
		}
	}
}

// Anything the group cannot carry means there was no group, and the subscript
// stands as written — which is the whole of the error handling and the whole
// of why the flag is additive.
func TestASubscriptFlagGroupThatCannotBeReadIsNotAGroup(t *testing.T) {
	for _, src := range []string{
		// A letter outside the set.
		`echo ${a[(z)2]}`,
		`echo ${a[(U)2]}`,
		`echo ${a[(rz)2]}`,
		// An argument-taking flag without one.
		`echo ${a[(n)2]}`,
		`echo ${a[(b)2]}`,
		`echo ${a[(rn)x]}`,
		// An argument whose delimiter never closes.
		`echo ${a[(n:2)x]}`,
		// An argument-taking flag whose "argument" is the parenthesis that
		// would have closed the group. The delimiter search would find a
		// later `)` and read the group past it, so the shape needs its own
		// clause — and the shell that has groups calls this one `invalid
		// subscript` rather than reading it.
		"echo ${a[(n))r)beta]}",
		// No closing parenthesis at all.
		`echo ${a[(r2]}`,
		// A blank is not a flag character.
		`echo ${a[( r )beta]}`,
		// The group has to be at the front.
		`echo ${a[1(r)beta]}`,
		// A digit is not a flag character, so an ordinary parenthesized
		// expression stays arithmetic.
		`echo ${a[(1+1)]}`,
	} {
		e := firstParam(t, src, subFlagged())
		if e.IndexFlags != nil {
			t.Errorf("%s: read a flag group %q, want none", src, e.IndexFlags.Src)
		}
		if e.Index == nil {
			t.Errorf("%s: lost the subscript", src)
		}
	}
}

// Off, the same text is a subscript with no group in it, whatever it holds.
func TestASubscriptFlagGroupOffLeavesTheSubscriptWhole(t *testing.T) {
	d := Core()
	d.ArraySubscript = true
	for _, src := range []string{`echo ${a[(r)beta]}`, `echo ${a[(re)x]}`, `echo ${a[()2]}`} {
		e := firstParam(t, src, d)
		if e.IndexFlags != nil {
			t.Errorf("%s: read a flag group with the dialect flag off", src)
		}
	}
}

// Subscript is the accessor every reading uses, so it answers for both shapes.
func TestSubscriptIsTheOperandBehindAGroupAndTheWholeSubscriptWithout(t *testing.T) {
	e := firstParam(t, `echo ${a[(re)beta]}`, subFlagged())
	if got := operandText(e.Subscript()); got != "beta" {
		t.Errorf("Subscript() with a group = %q, want %q", got, "beta")
	}
	e = firstParam(t, `echo ${a[1,3]}`, subFlagged())
	if got := operandText(e.Subscript()); got != "1,3" {
		t.Errorf("Subscript() without a group = %q, want %q", got, "1,3")
	}
}

// The brace-less spelling carries a group too, which is a lexer question:
// without it the `(` ends the word and the file does not parse at all.
func TestABareSubscriptCarriesAFlagGroup(t *testing.T) {
	d := subFlagged()
	d.BareSubscript = true
	for _, tc := range []struct{ src, flags, operand string }{
		{`echo $a[(r)beta]`, "r", "beta"},
		{`echo $a[(re)be*]`, "re", "be*"},
		{`echo $a[(rn:2:)x]`, "rn", "x"},
	} {
		e := firstParam(t, tc.src, d)
		if e.IndexFlags == nil {
			t.Fatalf("%s: no flag group read", tc.src)
		}
		if e.IndexFlags.Flags != tc.flags || operandText(e.IndexFlags.Arg) != tc.operand {
			t.Errorf("%s: flags=%q operand=%q, want %q %q",
				tc.src, e.IndexFlags.Flags, operandText(e.IndexFlags.Arg), tc.flags, tc.operand)
		}
		if e.Name != "a" {
			t.Errorf("%s: name=%q, want a", tc.src, e.Name)
		}
	}
	// And a `(` that does not open a readable group still ends the word, so
	// nothing else about the bare form moved.
	if _, err := Parse(`echo $a[(z)beta]`, d); err == nil {
		t.Error("$a[(z)beta] parsed; want the parenthesis to end the word as before")
	}
}
