// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// More than one subscript on a braced expansion — `${m[k][2]}` — where the
// grammar has the chain. The flag is named here and the shell that sets it is
// not.

// subChained is the core plus subscripts plus the chain.
func subChained() Dialect {
	d := Core()
	d.ArraySubscript = true
	d.ArraySubscriptFlags = true
	d.ChainedSubscript = true
	return d
}

// subscriptTexts is every subscript of an expansion in written order, which is
// Leading followed by Index.
func subscriptTexts(e *ParamExpr) []string {
	out := make([]string, 0, len(e.Leading)+1)
	for _, lead := range e.Leading {
		out = append(out, operandText(lead.Index))
	}
	if e.Index != nil {
		out = append(out, operandText(e.Index))
	}
	return out
}

// Every subscript is read, in written order, with the *last* one in Index.
func TestAChainOfSubscriptsIsRead(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []string
	}{
		{`echo ${a[1]}`, []string{"1"}},
		{`echo ${a[1][2]}`, []string{"1", "2"}},
		{`echo ${m[k][2]}`, []string{"k", "2"}},
		{`echo ${a[1,3][2]}`, []string{"1,3", "2"}},
		{`echo ${a[1,3][1,2][2]}`, []string{"1,3", "1,2", "2"}},
		{`echo ${a[@][2]}`, []string{"@", "2"}},
		{`echo ${a[(r)y][1]}`, []string{"(r)y", "1"}},
	} {
		e := firstParam(t, tc.src, subChained())
		got := subscriptTexts(e)
		if len(got) != len(tc.want) {
			t.Errorf("%s: subscripts %q, want %q", tc.src, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: subscript %d is %q, want %q", tc.src, i, got[i], tc.want[i])
			}
		}
		if e.Op != 0 || e.Bad {
			t.Errorf("%s: read as op=%v bad=%v, want the whole of it consumed as subscripts", tc.src, e.Op, e.Bad)
		}
	}
}

// A flag group belongs to the subscript it was written on and travels with it
// into Leading, rather than staying on whichever subscript happened to be read
// last.
func TestAFlagGroupStaysWithItsOwnSubscript(t *testing.T) {
	e := firstParam(t, `echo ${a[(r)y][1]}`, subChained())
	if len(e.Leading) != 1 {
		t.Fatalf("leading subscripts %d, want 1", len(e.Leading))
	}
	g := e.Leading[0].Flags
	if g == nil {
		t.Fatalf("the first subscript lost its flag group")
	}
	if g.Flags != "r" || operandText(g.Arg) != "y" {
		t.Errorf("group flags=%q operand=%q, want %q %q", g.Flags, operandText(g.Arg), "r", "y")
	}
	if e.IndexFlags != nil {
		t.Errorf("the last subscript has a group %q, and none was written on it", e.IndexFlags.Src)
	}
}

// Without the flag the second bracket is not a subscript, and what is left
// makes the expansion unreadable — which is how the grammars without the
// chain answer `${a[1][2]}`.
func TestWithoutTheFlagOnlyTheFirstSubscriptIsRead(t *testing.T) {
	d := Core()
	d.ArraySubscript = true
	e := firstParam(t, `echo ${a[1][2]}`, d)
	if len(e.Leading) != 0 {
		t.Errorf("leading subscripts %d, want none read at all", len(e.Leading))
	}
	if got := operandText(e.Index); got != "1" {
		t.Errorf("subscript %q, want %q — the first and only the first", got, "1")
	}
	if !e.Bad {
		t.Errorf("read as op=%v, want the leftover `[2]` to make the expansion unreadable", e.Op)
	}
}

// The bare spelling takes one subscript however many are written, which is
// measured: the rest is ordinary text there rather than a second link.
func TestTheBareSpellingTakesOneSubscript(t *testing.T) {
	d := subChained()
	d.BareSubscript = true
	e := firstParam(t, `echo "$a[1][2]"`, d)
	if len(e.Leading) != 0 {
		t.Errorf("leading subscripts %d, want none: the bare spelling reads one bracket", len(e.Leading))
	}
	if got := operandText(e.Index); got != "1" {
		t.Errorf("subscript %q, want %q", got, "1")
	}
}
