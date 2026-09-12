// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A redirection standing between a definition's name list and its
// parentheses, and the `()` named as one token from outside the definition.
//
// Both are [Dialect.FunctionMultipleNames] and
// [Dialect.EmptyParensAreOneToken] read further than they were: the first at
// a `(` no word announces, the second at a refusal the definition path never
// reaches.

// namesWithRedirs is the grammar these rows need — the name list before `()`,
// and the joined pair a refusal names.
func namesWithRedirs() Dialect {
	d := Core()
	d.FunctionMultipleNames = true
	d.EmptyParensAreOneToken = true
	d.FunctionNameIsAnyWord = true
	return d
}

// bodyRedirs is the redirection operators on a declaration's body, as written.
func bodyRedirs(t *testing.T, fn *FuncDecl) []string {
	t.Helper()
	g, ok := fn.Body.(*Group)
	if !ok {
		t.Fatalf("body is %T, want a brace group", fn.Body)
	}
	out := make([]string, 0, len(g.Redirs))
	for _, r := range g.Redirs {
		out = append(out, r.Op.String()+PrintWord(r.Word))
	}
	return out
}

func TestARedirectionBetweenTheNamesAndTheParens(t *testing.T) {
	d := namesWithRedirs()
	for _, tc := range []struct {
		name, src string
		names     []string
		redirs    []string
	}{
		{
			// The issue's own shape: both names share one body and the
			// redirection is that body's, so neither call reaches the
			// terminal.
			"between the names and the parentheses",
			`a b >out () { echo "[$0]"; }`,
			[]string{"a", "b"},
			[]string{">out"},
		},
		{
			"in front of the names",
			`>out a b () { echo "[$0]"; }`,
			[]string{"a", "b"},
			[]string{">out"},
		},
		{
			"in the middle of them",
			`a >out b () { echo "[$0]"; }`,
			[]string{"a", "b"},
			[]string{">out"},
		},
		{
			"two of them",
			`a b >o1 >o2 () { echo hi; }`,
			[]string{"a", "b"},
			[]string{">o1", ">o2"},
		},
		{
			"one name and a redirection",
			`a >out () { echo hi; }`,
			[]string{"a"},
			[]string{">out"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := decl(t, tc.src, d)
			got := append([]string{fn.Name}, "")[:1]
			for _, n := range fn.AlsoNamed {
				got = append(got, n.Name)
			}
			if len(got) != len(tc.names) {
				t.Fatalf("names %v, want %v", got, tc.names)
			}
			for i := range got {
				if got[i] != tc.names[i] {
					t.Fatalf("names %v, want %v", got, tc.names)
				}
			}
			if rs := bodyRedirs(t, fn); len(rs) != len(tc.redirs) {
				t.Errorf("body redirections %v, want %v", rs, tc.redirs)
			} else {
				for i := range rs {
					if rs[i] != tc.redirs[i] {
						t.Errorf("body redirections %v, want %v", rs, tc.redirs)
						break
					}
				}
			}
		})
	}
}

func TestAnAssignmentStillEndsTheNameListReading(t *testing.T) {
	d := namesWithRedirs()
	// The bound that was already measured and still holds: an assignment in
	// front of the names is not a definition, redirection or no redirection.
	refuses(t, d, `x=1 a b () { echo X; }`)
	refuses(t, d, `x=1 a b >out () { echo X; }`)
}

func TestWithoutTheNameListTheRedirectedParenIsRefused(t *testing.T) {
	d := namesWithRedirs()
	d.FunctionMultipleNames = false
	// The other five shells' reading, which is also this parser's before the
	// route existed: the `(` after a redirection's target opens nothing.
	refuses(t, d, `a b >out () { echo X; }`)
}

func TestTheEmptyParensAreNamedFromOutsideTheDefinition(t *testing.T) {
	d := namesWithRedirs()
	for _, src := range []string{
		`x=1 f () { echo X; }`,
		`x=1 a b () { echo X; }`,
		`x=1 a b >out () { echo X; }`,
	} {
		_, err := Parse(src, d)
		if err == nil {
			t.Errorf("%s: parsed, want a refusal", src)
			continue
		}
		if got := err.(*Error).Token; got != "()" {
			t.Errorf("%s: named %q, want `()`", src, got)
		}
	}
}

func TestABlankInsideTheParensLeavesThemTwoTokens(t *testing.T) {
	d := namesWithRedirs()
	// Adjacency is the rule: with a blank between them the two characters are
	// a subshell, so the refusal falls further along and names something
	// else. A join that skipped blanks would name `()` here.
	_, err := Parse(`x=1 f ( ) { echo X; }`, d)
	if err == nil {
		t.Fatal("parsed, want a refusal")
	}
	if got := err.(*Error).Token; got == "()" {
		t.Error("named `()` where the parentheses hold a blank")
	}
}

func TestWithoutTheJoinedPairTheOpenParenIsNamedAlone(t *testing.T) {
	d := namesWithRedirs()
	d.EmptyParensAreOneToken = false
	_, err := Parse(`x=1 f () { echo X; }`, d)
	if err == nil {
		t.Fatal("parsed, want a refusal")
	}
	if got := err.(*Error).Token; got != "(" {
		t.Errorf("named %q, want `(`", got)
	}
}
