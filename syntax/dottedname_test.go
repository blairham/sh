// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `.` is a name character in one grammar and in no other.
//
// `${.sh.version}` and a compound variable's `${c.a}` are the same lexical
// rule reached twice, which is the question #2620 asked. The probes that say
// so are on [Dialect.DottedName]; these pin the grammar half of it, and
// especially that the flag does not leak: the other four dialects have to go
// on refusing a dotted name exactly as they did, because a `${.sh.version}`
// that parsed in `cmd/bash` would accept what real bash calls a bad
// substitution at the run.
func TestADottedNameIsOneDialectsGrammar(t *testing.T) {
	t.Parallel()
	dotted := Dialect{DottedName: true, BadSubstitutionAtParseTime: true}
	plain := Dialect{BadSubstitutionAtParseTime: true}
	for _, src := range []string{
		`echo ${.sh.version}`,
		`echo "${.sh.version}"`,
		`echo ${.}`,
		`echo ${.foo}`,
		`echo ${c.a}`,
		`echo ${#.sh.version}`,
		`echo ${.sh.version#Version }`,
	} {
		mustParse(t, src, dotted, "a dot is a name character here")
		mustFail(t, src, plain, "a dot is not a name character here")
	}
}

// The same rule, in the positions that are not an expansion. An assignment's
// left side is the one that changes what the word *is*: `.foo=1` is a command
// name without the flag and an assignment with it.
func TestADottedNameIsAnAssignment(t *testing.T) {
	t.Parallel()
	dotted := Dialect{DottedName: true}
	plain := Dialect{}
	for _, c := range []struct{ src, name string }{
		{`.foo=1`, ".foo"},
		{`a.b=2`, "a.b"},
		{`.sh.version=1`, ".sh.version"},
	} {
		f, err := Parse(c.src, dotted)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		got := assignedNames(f)
		if len(got) != 1 || got[0] != c.name {
			t.Errorf("%q with the flag: assigned %v, want [%s]", c.src, got, c.name)
		}
		f, err = Parse(c.src, plain)
		if err != nil {
			t.Fatalf("%q without the flag: %v", c.src, err)
		}
		if got := assignedNames(f); len(got) != 0 {
			t.Errorf("%q without the flag: assigned %v, want a command name", c.src, got)
		}
	}
}

// A `for` variable is the third position, and it is the parser that decides:
// the name is checked while reading in four of the five dialects, so a dotted
// one has to survive that check to reach a loop at all.
func TestADottedForNameReads(t *testing.T) {
	t.Parallel()
	mustParse(t, "for .x in 1 2; do :; done", Dialect{DottedName: true}, "a dotted `for` variable")
	mustFail(t, "for .x in 1 2; do :; done", Dialect{}, "a dotted `for` variable")
}

// assignedNames is the names the first command of f assigns, which is what
// tells an assignment from a command word whose text happens to hold an `=`.
func assignedNames(f *File) []string {
	var names []string
	for _, st := range f.Stmts {
		pipe, ok := st.Expr.(*Pipeline)
		if !ok {
			continue
		}
		cmd, ok := pipe.Cmds[0].(*SimpleCmd)
		if !ok {
			continue
		}
		for _, a := range cmd.Assigns {
			names = append(names, a.Name)
		}
	}
	return names
}

// A subscript on a name that begins with a dot, which is the only way to read
// `.sh.match` past its whole match and the only way to count `.sh.pipestatus`
// (#2916). A dot *inside* a name was already subscriptable; the leading one was
// taken for a special parameter and the bracket left for the word to refuse.
func TestALeadingDotNameTakesASubscript(t *testing.T) {
	t.Parallel()
	dotted := Dialect{DottedName: true, ArraySubscript: true, BadSubstitutionAtParseTime: true}
	for _, src := range []string{
		`echo "${.sh.match[1]}"`,
		`echo "${#.sh.pipestatus[@]}"`,
		`echo "${.sh.pipestatus[0]:-d}"`,
		`echo "${.[0]}"`,
		`echo "${c.a[0]}"`,
	} {
		mustParse(t, src, dotted, "a subscript on a dotted name")
	}
}
