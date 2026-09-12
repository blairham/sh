// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A definition's name read as the word that was *written*, quotes and all.
//
// The axis is [Dialect.FunctionNameIsSourceText], where the panel is. It is
// the control for the two "any word is a name" flags either side of it: the
// name here is `f`, which no set of names could exclude, so what is refused
// is the quoting rather than the characters.

// sourceTextName is the grammar this needs: the keyword, the hybrid parens,
// the name carried to the definition, and the name read as source text. That
// combination is one shell's and the rows below say which parts do what.
func sourceTextName() Dialect {
	d := Core()
	d.FunctionKeyword = true
	d.FunctionKeywordParens = true
	d.FunctionNamePunctuation = true
	d.FunctionNameCheckedWhenTheDefinitionRuns = true
	d.FunctionNameIsSourceText = true
	// The `name()` half needs the parentheses to be the announcement, which
	// is [Dialect.FuncDefAtParen] and is what the shell with this rule has:
	// nothing about the word says it is a name, so nothing about the word can
	// decide the reading. Without it a quoted word never reaches the name
	// test at all and the `(` after it is refused, which is the row
	// TestWithoutTheFlagTheQuotesComeOff still pins.
	d.FuncDefAtParen = true
	return d
}

func TestAQuotedNameAfterTheKeywordIsTheQuotedWord(t *testing.T) {
	d := sourceTextName()
	for _, tc := range []struct{ name, src, refused string }{
		{"single quotes", `function 'f' { echo p; }`, `'f'`},
		{"double quotes", `function "f" { echo p; }`, `"f"`},
		{"an empty quoted run stuck to it", `function f"" { echo p; }`, `f""`},
		{"a backslash", `function \f { echo p; }`, `\f`},
		{"quotes around a name that is not one either", `function 'a b' { echo p; }`, `'a b'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := decl(t, tc.src, d)
			if fn.RefusedName != tc.refused {
				t.Errorf("refused %q, want %q", fn.RefusedName, tc.refused)
			}
			if fn.Name != "" {
				t.Errorf("name is %q; a refused word declares nothing", fn.Name)
			}
		})
	}
}

func TestAQuotedNameBeforeParensIsTheQuotedWord(t *testing.T) {
	d := sourceTextName()
	// The `name()` spelling of the same rule, which is the half #1566 left
	// as a question: the parentheses announce the definition, and the word
	// in front of them is a name or is refused on its own terms.
	for _, tc := range []struct{ name, src, refused string }{
		{"single quotes", `'q'() { echo p; }`, `'q'`},
		{"an empty name", `''() { echo b; }`, `''`},
		{"a backslash-escaped blank", `a\ b() { echo p; }`, `a\ b`},
		{"double quotes around a blank", `"a b"() { echo p; }`, `"a b"`},
		{"an operator inside quotes", `'a;b'() { echo one; }`, `'a;b'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := decl(t, tc.src, d)
			if fn.RefusedName != tc.refused {
				t.Errorf("refused %q, want %q", fn.RefusedName, tc.refused)
			}
		})
	}
}

func TestANameThatIsOneIsStillDefinedEitherSpelling(t *testing.T) {
	d := sourceTextName()
	// The other side of the control: reading the source text must not refuse
	// the names that are names. The punctuated one is here because this
	// dialect has [Dialect.FunctionNamePunctuation] and the source text is
	// what that test now runs on.
	for _, tc := range []struct{ src, want string }{
		{`function f { echo p; }`, "f"},
		{`function :f { echo p; }`, ":f"},
		{`f() { echo p; }`, "f"},
		{`f-g() { echo p; }`, "f-g"},
		{`function f() { echo p; }`, "f"},
	} {
		fn := decl(t, tc.src, d)
		if fn.Name != tc.want || fn.RefusedName != "" {
			t.Errorf("%s: name %q refused %q, want name %q and no refusal",
				tc.src, fn.Name, fn.RefusedName, tc.want)
		}
	}
}

func TestWithoutTheFlagTheQuotesComeOff(t *testing.T) {
	// The two shells that remove them, which is the same site read the other
	// way: `function 'f'` defines `f` and refuses nothing.
	d := sourceTextName()
	d.FunctionNameIsSourceText = false
	fn := decl(t, `function 'f' { echo p; }`, d)
	if fn.Name != "f" || fn.RefusedName != "" {
		t.Errorf("name %q refused %q, want `f` and no refusal", fn.Name, fn.RefusedName)
	}
	// And the `name()` spelling goes back to refusing the parenthesis, which
	// is what says the two halves are one flag rather than two.
	refuses(t, d, `'q'() { echo p; }`)
}

func TestAnArrayAssignmentIsNotADefinitionOfAQuotedName(t *testing.T) {
	// The reading the widened gate must not swallow: `a=()` is an empty
	// array everywhere, and a parenthesis after a word is what both look
	// like. The `=` has to be bare to make one.
	d := sourceTextName()
	d.ArrayLiteral = true
	f, err := Parse(`a=()`, d)
	if err != nil {
		t.Fatalf("a=(): %v", err)
	}
	if _, isFunc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl); isFunc {
		t.Error("a=() read as a function definition")
	}
}
