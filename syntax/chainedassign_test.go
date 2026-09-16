// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// More than one subscript on the *left of an assignment* — `a[1][2]=v` —
// where the grammar has the chain. The flag is named here and the shell that
// sets it is not.
//
// It is a different flag from ChainedSubscript, which is a braced expansion's
// and belongs to another dialect entirely: the two constructs are spelled
// alike and mean different things in different shells, so a grammar that
// enabled one from the other would give each shell the other's reading.

// assignChained is the core plus subscripts plus the assignment chain.
func assignChained() Dialect {
	d := Core()
	d.ArraySubscript = true
	d.ArrayLiteral = true
	d.AppendAssign = true
	d.ChainedAssignSubscript = true
	return d
}

// assignSubscriptTexts is every subscript of an assignment in written order,
// which is Leading followed by Index.
func assignSubscriptTexts(a *Assign) []string {
	out := make([]string, 0, len(a.Leading)+1)
	for _, lead := range a.Leading {
		out = append(out, lead.Text)
	}
	if a.Index != nil || a.IndexText != "" {
		out = append(out, a.IndexText)
	}
	return out
}

func firstAssignIn(t *testing.T, src string, d Dialect) *Assign {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if len(sc.Assigns) == 0 {
		t.Fatalf("no assignment in %q", src)
	}
	return sc.Assigns[0]
}

// Every subscript is read, in written order, with the *last* one in Index —
// the same way round a chained expansion keeps them, and for the same reason:
// the final subscript is the one the value lands under.
func TestAChainOfAssignmentSubscriptsIsRead(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src  string
		want []string
	}{
		{`a[1]=v`, []string{"1"}},
		{`a[1][2]=v`, []string{"1", "2"}},
		{`a[1][2][3]=v`, []string{"1", "2", "3"}},
		{`m[k][2]=v`, []string{"k", "2"}},
		{`a[$i][$j]=v`, []string{"$i", "$j"}},
		{`a[1][2]+=v`, []string{"1", "2"}},
	} {
		a := firstAssignIn(t, tc.src, assignChained())
		got := assignSubscriptTexts(a)
		if len(got) != len(tc.want) {
			t.Errorf("%s: subscripts %q, want %q", tc.src, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: subscript %d is %q, want %q", tc.src, i, got[i], tc.want[i])
			}
		}
		if a.Name != "a" && a.Name != "m" {
			t.Errorf("%s: name %q, want the text in front of the first bracket", tc.src, a.Name)
		}
	}
}

// And the value is what stands after the last `]=`, which is the half a
// reader that stopped at the first `]` would get wrong.
func TestAChainedAssignmentKeepsItsValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`a[1][2]=v`, "v"},
		{`a[1][2]=x=y`, "x=y"},
		{`a[1][2]=`, ""},
	} {
		a := firstAssignIn(t, tc.src, assignChained())
		got := ""
		if a.Value != nil {
			got = operandText(a.Value)
		}
		if got != tc.want {
			t.Errorf("%s: value %q, want %q", tc.src, got, tc.want)
		}
	}
	if a := firstAssignIn(t, `a[1][2]+=v`, assignChained()); !a.Append {
		t.Errorf("`a[1][2]+=v` did not read as an append")
	}
}

// Without the flag no `]` is read as a link, so the text between the first
// bracket and the last one is a single subscript — which is the reading every
// other dialect keeps, and is what makes `a[1][2]=v` an operand they refuse.
func TestWithoutTheFlagAChainIsOneSubscript(t *testing.T) {
	t.Parallel()
	d := assignChained()
	d.ChainedAssignSubscript = false
	a := firstAssignIn(t, `a[1][2]=v`, d)
	if len(a.Leading) != 0 {
		t.Errorf("Leading = %q, want none without the flag", assignSubscriptTexts(a))
	}
	if a.IndexText != "1][2" {
		t.Errorf("IndexText = %q, want the whole run between the outer brackets", a.IndexText)
	}
}

// A nested bracket inside one subscript is not a link. `a[i[0]][2]=v` reads
// another element to index this one, which is a legal expression, and only
// the bracket that *closes* the run and is followed by another `[` opens a
// link — the same count subscriptBracketsBalance makes.
func TestANestedBracketIsNotALink(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src  string
		want []string
	}{
		{`a[i[0]]=v`, []string{"i[0]"}},
		{`a[i[0]][2]=v`, []string{"i[0]", "2"}},
		{`a[1][i[0]]=v`, []string{"1", "i[0]"}},
	} {
		a := firstAssignIn(t, tc.src, assignChained())
		got := assignSubscriptTexts(a)
		if len(got) != len(tc.want) {
			t.Errorf("%s: subscripts %q, want %q", tc.src, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: subscript %d is %q, want %q", tc.src, i, got[i], tc.want[i])
			}
		}
	}
}
