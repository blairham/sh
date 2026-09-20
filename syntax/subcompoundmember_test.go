// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A dotted member path written after a subscript's closing bracket —
// `a[1].p=9` and `${a[1].p}`, the two halves of one dialect's compound
// variable held in an array element (#2853).
//
// It is a field of its own rather than part of the name because the name scan
// is long since past by then: `a` ends at the `[`, and the dot behind the `]`
// has nothing left to run on. Before a `]` nothing is needed — `.` is a name
// byte where [Dialect.DottedName] is set and `c.p` is one Name.
func TestAMemberPathAfterASubscript(t *testing.T) {
	t.Parallel()
	d := Dialect{DottedName: true, ArraySubscript: true, AppendAssign: true, ArrayLiteral: true}
	for _, c := range []struct{ src, name, member string }{
		{`a[1].p=9`, "a", ".p"},
		{`a[1].q.r=9`, "a", ".q.r"},
		{`a[1]+=(p=1)`, "a", ""},
		{`a[1]=9`, "a", ""},
		{`a.b=9`, "a.b", ""},
	} {
		a := firstAssignIn(t, c.src, d)
		if a.Name != c.name || a.Member != c.member {
			t.Errorf("%q gave name %q member %q, want %q and %q",
				c.src, a.Name, a.Member, c.name, c.member)
		}
	}
}

// The path stands between the bracket and the operator, which is where it was
// written — so a tree that carries one prints back as source that parses to
// the same tree. Without this the printer dropped it silently and
// `a[1].p=9` came back as `a[1]=9`, a write to a different cell.
func TestAMemberPathSurvivesPrinting(t *testing.T) {
	t.Parallel()
	d := Dialect{
		DottedName: true, ArraySubscript: true,
		AppendAssign: true, ParamIndirection: true,
	}
	for _, src := range []string{
		`a[1].p=9`,
		`a[1].q.r=9`,
		`a[1].p+=9`,
		`a[$i].p=9`,
		`echo "${a[1].p}" ${!a[1].@} "${a[1].q.r}"`,
	} {
		f, err := Parse(src, d)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if got := Print(f); got != src {
			t.Errorf("printing %q gave %q", src, got)
		}
	}
}

// The reading half, on the node the expander reads it off.
func TestAMemberPathOnAnExpansion(t *testing.T) {
	t.Parallel()
	d := Dialect{DottedName: true, ArraySubscript: true, ParamIndirection: true}
	for _, c := range []struct{ src, name, member string }{
		{`echo ${a[1].p}`, "a", ".p"},
		{`echo ${a[1].q.r}`, "a", ".q.r"},
		{`echo ${!a[1].@}`, "a", "."},
		{`echo ${a[1]}`, "a", ""},
		{`echo ${c.p}`, "c.p", ""},
	} {
		e := firstParam(t, c.src, d)
		if e.Name != c.name || e.Member != c.member {
			t.Errorf("%q gave name %q member %q, want %q and %q",
				c.src, e.Name, e.Member, c.name, c.member)
		}
	}
}

// Without the dialect flag there is no member path at all, and the word is
// whatever it was before: `a[1].p=9` is a command, not an assignment.
func TestAMemberPathNeedsTheDialect(t *testing.T) {
	t.Parallel()
	f, err := Parse(`a[1].p=9`, Dialect{ArraySubscript: true})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := assignedNames(f); len(got) != 0 {
		t.Errorf("without the flag: assigned %v, want a command name", got)
	}
}
