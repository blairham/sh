// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// A definition's name decided by how the word was **written** rather than by
// what it says.
//
// The flag is [Dialect.FunctionNameIsAnyBareWord], where the panel is. It is
// the third answer to the question [Dialect.FunctionKeywordNameIsAnyWord] and
// [Dialect.FunctionNameIsAnyWord] give the other two, and the row that tells
// it from both is `a*b()`: a name matched against the filesystem there, an
// ordinary definition here.

// bareWordName is the grammar this needs. The keyword and its hybrid parens,
// punctuation in a bare name, the parentheses as the whole announcement, and
// the word carried to the definition rather than refused while reading — that
// combination is one shell's, and the rows below say which parts do what.
func bareWordName() Dialect {
	d := Core()
	d.FunctionKeyword = true
	d.FunctionKeywordParens = true
	d.FunctionNamePunctuation = true
	d.FuncDefAtParen = true
	d.FunctionNameCheckedWhenTheDefinitionRuns = true
	d.FunctionNameIsAnyBareWord = true
	return d
}

// A word written bare is a name, whatever characters are in it — including
// the three a pattern is made of, which is the whole of what separates this
// flag from [Dialect.FunctionNameIsAnyWord].
func TestABareWordIsANameWhateverIsInIt(t *testing.T) {
	d := bareWordName()
	for _, tc := range []struct{ src, name string }{
		{`f() { :; }`, "f"},
		{`a.b() { :; }`, "a.b"},
		{`a*b() { :; }`, "a*b"},
		{`a?b() { :; }`, "a?b"},
		{`a[b() { :; }`, "a[b"},
		{`a{b() { :; }`, "a{b"},
		{`a}b() { :; }`, "a}b"},
		{`a~b() { :; }`, "a~b"},
		{`function f { :; }`, "f"},
		{`function a.b { :; }`, "a.b"},
		{`function a*b { :; }`, "a*b"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			fn := decl(t, tc.src, d)
			if fn.Name != tc.name {
				t.Errorf("name is %q, want %q", fn.Name, tc.name)
			}
			if fn.RefusedName != "" {
				t.Errorf("refused %q; a bare word is a name here", fn.RefusedName)
			}
		})
	}
}

// And a word written any other way is carried to the definition as the source
// text, declaring nothing. The name `f` is the control: no set of names could
// exclude it, so what is refused is the spelling.
func TestAWordNotWrittenBareDeclaresNothing(t *testing.T) {
	d := bareWordName()
	for _, tc := range []struct{ src, refused string }{
		{`'f'() { :; }`, `'f'`},
		{`"f"() { :; }`, `"f"`},
		{`\f() { :; }`, `\f`},
		{`a"b"() { :; }`, `a"b"`},
		{`f""() { :; }`, `f""`},
		{`''() { :; }`, `''`},
		{`a\ b() { :; }`, `a\ b`},
		{`_p_${w}() { :; }`, `_p_${w}`},
		{`$(echo n)() { :; }`, `$(echo n)`},
		{`function 'f' { :; }`, `'f'`},
		{`function '' { :; }`, `''`},
		{`function \f { :; }`, `\f`},
		{`function a\*b { :; }`, `a\*b`},
		{`function _p_${w} { :; }`, `_p_${w}`},
		{`function $(echo n) { :; }`, `$(echo n)`},
	} {
		t.Run(tc.src, func(t *testing.T) {
			fn := decl(t, tc.src, d)
			if fn.RefusedName != tc.refused {
				t.Errorf("refused %q, want %q", fn.RefusedName, tc.refused)
			}
			if fn.Name != "" {
				t.Errorf("name is %q; a word that was not written bare declares nothing", fn.Name)
			}
			if fn.NameWord != nil {
				t.Errorf("a word was kept for expansion; this flag never expands a name")
			}
		})
	}
}

// The body is read whole, which is what makes this a definition the grammar
// takes rather than a line it skips: a broken body is still a syntax error,
// and it is the reason the answer belongs at the definition's run.
func TestABodyUnderARefusedNameIsStillParsed(t *testing.T) {
	d := bareWordName()
	if _, err := Parse(`'h'() { if; }`, d); err == nil {
		t.Error("a broken body parsed; the declaration is read whole here")
	}
	fn := decl(t, `'h'() { echo one; echo two; }`, d)
	if fn.Body == nil {
		t.Fatal("no body was kept under a refused name")
	}
}

// An assignment is still an assignment, and the reading is lexical: only a
// **bare** `=` makes one, so a quoted one is a name that declares nothing.
func TestABareEqualsIsAnAssignmentAndAQuotedOneIsNot(t *testing.T) {
	d := bareWordName()
	if _, err := Parse(`a=() { :; }`, d); err == nil {
		t.Error("`a=()` parsed as a definition; a bare `=` is an assignment")
	}
	fn := decl(t, `'a=b'() { :; }`, d)
	if fn.RefusedName != `'a=b'` {
		t.Errorf("refused %q, want `'a=b'`", fn.RefusedName)
	}
}

// The word goes back as it was written, which is the only thing it is kept
// for here — this shell prints no complaint, so nothing quotes it. A print
// that dropped the quotes would hand back a program that *defines* `f` where
// the input defined nothing.
func TestARefusedBareWordNamePrintsBackAsItWasWritten(t *testing.T) {
	d := bareWordName()
	for _, src := range []string{`'f'() { :; }`, `''() { :; }`, `function 'f' { :; }`} {
		t.Run(src, func(t *testing.T) {
			f, err := Parse(src, d)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := Print(f)
			if !strings.Contains(got, "'f'") && !strings.Contains(got, "''") {
				t.Errorf("printed %q, want the quoting kept", got)
			}
			again, err := Parse(got, d)
			if err != nil {
				t.Fatalf("reparse %q: %v", got, err)
			}
			fn, ok := again.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl)
			if !ok || fn.RefusedName == "" || fn.Name != "" {
				t.Errorf("reparsed %q into a definition of %q; the refusal did not survive the print", got, fn.Name)
			}
		})
	}
}

// Without the flag the readings either side of it stand unchanged, which is
// what says this is a third answer and not a widening of the other two.
func TestWithoutTheBareWordFlagTheOlderReadingsStand(t *testing.T) {
	d := bareWordName()
	d.FunctionNameIsAnyBareWord = false
	// The quotes come off and an ordinary name is declared, which is the
	// reading every dialect without a name flag has.
	if fn := decl(t, `function 'f' { :; }`, d); fn.Name != "f" || fn.RefusedName != "" {
		t.Errorf("name %q refused %q, want `f` and no refusal", fn.Name, fn.RefusedName)
	}
	// And a name whose text is not one is refused where it stands, rather
	// than being carried anywhere.
	if _, err := Parse(`''() { :; }`, d); err == nil {
		t.Error("an empty quoted name parsed without the flag")
	}
}
