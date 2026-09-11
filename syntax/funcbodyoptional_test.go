// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// `function a b` with no body, and the `;` that may stand between a name list
// and a body it does have. The grammar half; what the names are then bound to
// is dialect/zsh/funcbodyoptional_test.go.

// optionalFuncBody is manyFuncNames with the flag, which is the combination
// one shell in the panel has — the two arrived together and the separator is
// what keeps a bodyless declaration from swallowing the command after it.
func optionalFuncBody() Dialect {
	d := manyFuncNames()
	d.FunctionKeywordBodyIsOptional = true
	d.EmptyCompoundBody = true
	return d
}

// A declaration that ends without a body is a declaration and not a failure,
// and the body it gets is an empty one.
func TestTheKeywordFormMayEndWithoutABody(t *testing.T) {
	d := optionalFuncBody()
	for _, tc := range []struct {
		src   string
		names []string
	}{
		{`function a`, []string{"a"}},
		{`function a b`, []string{"a", "b"}},
		{`function a b c`, []string{"a", "b", "c"}},
	} {
		fn := decl(t, tc.src, d)
		got := append([]string{fn.Name}, nil...)
		for _, n := range fn.AlsoNamed {
			got = append(got, n.Name)
		}
		if len(got) != len(tc.names) {
			t.Errorf("%s: names %v, want %v", tc.src, got, tc.names)
			continue
		}
		for i := range got {
			if got[i] != tc.names[i] {
				t.Errorf("%s: names %v, want %v", tc.src, got, tc.names)
				break
			}
		}
		// An empty body rather than a nil one: a nil body is what a *failed*
		// parse leaves behind, so saying "no body on purpose" that way would
		// be saying it in the one spelling everything downstream already
		// reads as a refusal.
		g, ok := fn.Body.(*Group)
		if !ok {
			t.Errorf("%s: body is %T, want an empty group", tc.src, fn.Body)
			continue
		}
		if len(g.List) != 0 {
			t.Errorf("%s: body has %d statements, want none", tc.src, len(g.List))
		}
	}
	// Without the flag the same text is refused, which is what says this is
	// additive grammar rather than a lenience.
	for _, src := range []string{`function a`, `function a b`} {
		mustFail(t, src, manyFuncNames(), "a bodyless declaration without the flag")
	}
}

// The body is absent exactly when no command follows, which is every way a
// list of words can end rather than only the end of the input.
func TestABodylessDeclarationEndsWhereACommandWouldHaveBegun(t *testing.T) {
	d := optionalFuncBody()
	for _, src := range []string{
		`function a b`,
		`if true; then function a b; fi`,
		`{ function a b; }`,
		`while false; do function a b; done`,
		`function a b && echo and`,
		`function a b | cat`,
		`( function a b )`,
	} {
		mustParse(t, src, d, "a name list that ran out of words")
	}
}

// A `;` between the names and the body is read, and the command after it is
// the **body** rather than the next statement — which is the whole reason the
// separator and the absent body are one flag. Read the other way, `function
// a; echo B` would define an empty `a` and print `B` where it stands, which
// is a different program at status 0.
func TestASeparatorMayStandBetweenTheNamesAndTheBody(t *testing.T) {
	d := optionalFuncBody()
	for _, tc := range []struct {
		src  string
		body string
	}{
		{"function a; echo B", "echo B"},
		{"function a b; echo B", "echo B"},
		{"function a;\necho B", "echo B"},
		{"function a; ; echo B", "echo B"},
		{"function a() ; echo B", "echo B"},
		// A newline already stood there in every dialect, and the `;` joins
		// it rather than replacing it.
		{"function a\necho B", "echo B"},
	} {
		fn := decl(t, tc.src, d)
		simple, ok := fn.Body.(*SimpleCmd)
		if !ok {
			t.Errorf("%s: body is %T, want a simple command", tc.src, fn.Body)
			continue
		}
		if len(simple.Args) == 0 || simple.Args[0].Literal() != "echo" {
			t.Errorf("%s: body is not the command after the separator", tc.src)
		}
	}
	// And the statement after the *body* is still a statement, so the
	// separator is read once rather than being skipped until a body appears.
	f, err := Parse("function a; echo B; echo C", d)
	if err != nil {
		t.Fatalf("function a; echo B; echo C: %v", err)
	}
	if len(f.Stmts) != 2 {
		t.Errorf("function a; echo B; echo C: %d statements, want 2", len(f.Stmts))
	}
	// Without the flag the separator is where the definition stops being
	// readable at all, which is the control.
	mustFail(t, "function a; echo B", manyFuncNames(), "a separator before the body without the flag")
}

// A declaration with no body prints back as one with an empty body, which is
// the only spelling that reads back to the same program: written bare, the
// statement after it would be swallowed as the body the source did not have.
func TestABodylessDeclarationPrintsBackWithAnEmptyBody(t *testing.T) {
	d := optionalFuncBody()
	for _, tc := range []struct{ src, want string }{
		{`function a b`, "function a b { }"},
		{`function a`, "function a { }"},
		{"if true; then function a b; fi", "if true; then function a b { }; fi"},
		// A body that is not a brace group takes a newline in front of it,
		// for the same reason: written on one line, its words are read back
		// as more names.
		{"function a; echo B", "function a\necho B"},
		{"function a b; echo B", "function a b\necho B"},
	} {
		f, err := Parse(tc.src, d)
		if err != nil {
			t.Errorf("%s: %v", tc.src, err)
			continue
		}
		got := Print(f)
		if got != tc.want {
			t.Errorf("%s: printed %q, want %q", tc.src, got, tc.want)
			continue
		}
		back, err := Parse(got, d)
		if err != nil {
			t.Errorf("%s: printed form does not parse: %v", tc.src, err)
			continue
		}
		if why, ok := SameProgram(f, back); !ok {
			t.Errorf("%s: printed form is a different program: %s", tc.src, why)
		}
	}
}
