// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The POSIX `name()` form's name is any word (#1743).
//
// The keyword form has read one since #1548; this is the same rule for the
// other spelling, which until now refused a quoted word before any name test
// was reached — so one shell's two ways of writing the same definition
// disagreed inside this parser.
//
// One shell in the panel reads it that way. Measured on zsh 5.9.2, 2026-09-10,
// from a script file under `env -i`: bash 5.3.15 and bash 3.2.57 read the
// definition and refuse the *name* where it runs, that build invoked as `sh`
// does the same and is fatal, ksh93u+ refuses the name and stops, and dash is
// the only column that refuses to parse it.
//
// The rows are picked so that no character class could pass them: a semicolon
// and a pipe end a word when they are bare, so a wider set of *name
// characters* cannot be what this is — the quoting is.

// anyWordPosixName is the core with the flag this file is about.
func anyWordPosixName() Dialect {
	d := Core()
	d.FunctionNameIsAnyWord = true
	return d
}

func TestAnyWordIsAPosixFunctionName(t *testing.T) {
	on, off := anyWordPosixName(), Core()
	for _, tc := range []struct{ src, name string }{
		{`'a b'() { echo b; }`, "a b"},
		{`a\ b() { echo b; }`, "a b"},
		{`"a b"() { echo b; }`, "a b"},
		{`''() { echo b; }`, ""},
		{`""() { echo b; }`, ""},
		{`'a;b'() { echo b; }`, "a;b"},
		{`'a|b'() { echo b; }`, "a|b"},
		{`'a$b'() { echo b; }`, "a$b"},
		{`'a*b'() { echo b; }`, "a*b"},
		{`"a'b"() { echo b; }`, "a'b"},
		{`'a b c'() { echo b; }`, "a b c"},
		{`a\	b() { echo b; }`, "a\tb"},
		// `=` is excluded only where it is bare, which is what says the
		// exclusion is about an assignment and not about the character.
		{`'a=b'() { echo b; }`, "a=b"},
		{`a\=b() { echo b; }`, "a=b"},
		// The control the group needs: a name nobody could object to, in
		// quotes. Without it these rows would read as a wider set of name
		// characters, and no set of characters can contain `f`.
		{`'f'() { echo b; }`, "f"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			f, err := Parse(tc.src, on)
			if err != nil {
				t.Fatalf("refused where the flag allows: %v", err)
			}
			fn, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl)
			if !ok {
				t.Fatalf("read as %T, want a function definition", f.Stmts[0].Expr.(*Pipeline).Cmds[0])
			}
			if fn.Name != tc.name {
				t.Errorf("name is %q, want %q", fn.Name, tc.name)
			}
			if fn.Keyword {
				t.Error("recorded as the keyword form, which is a different declaration")
			}
			if _, err := Parse(tc.src, off); err == nil {
				t.Error("parsed without the flag")
			}
		})
	}
}

// A name whose *bare* text holds a pattern character is still refused.
//
// The shell this models matches such a word against the filesystem —
// `a*b() { :; }` is `no matches found: a*b` there and defines nothing — so a
// function literally called `a*b` would be a plausible wrong answer where a
// refusal is a visible one. It is the same exception
// [Dialect.FunctionKeywordNameIsAnyWord] keeps, and quoting takes it away:
// the quoted rows above include `'a*b'`.
func TestABarePatternIsNotAPosixFunctionName(t *testing.T) {
	d := anyWordPosixName()
	for _, src := range []string{
		`a*b() { echo b; }`,
		`a?b() { echo b; }`,
		`a[b() { echo b; }`,
		`a*'b'() { echo b; }`,
	} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%s: parsed, want a refusal", src)
		}
	}
}

// An array assignment is still an assignment and not a definition of a
// function whose name ends in `=`.
//
// Without this `a=()` reads as a definition, because a parenthesis pair
// follows a word either way — and the flag has taken away the name test that
// used to part them.
func TestAnAssignmentIsNotAPosixFunctionDefinition(t *testing.T) {
	d := anyWordPosixName()
	d.ArrayLiteral = true
	for _, src := range []string{
		`a=()`,
		`a=(x y)`,
	} {
		f, err := Parse(src, d)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if _, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl); ok {
			t.Errorf("%s: read as a function definition", src)
		}
	}
}

// The parentheses are still what announce a definition, so a quoted word with
// no `()` after it is an ordinary command name.
//
// The flag removes the *name* test and not the parentheses, and without this
// row "any word is a name" could be satisfied by a reading that took every
// quoted first word for a definition.
func TestTheParensStillAnnounceThePosixDefinition(t *testing.T) {
	d := anyWordPosixName()
	for _, src := range []string{
		`'a b'`,
		`'a b' x`,
		`'a b' (x)`,
	} {
		f, err := Parse(src, d)
		if err != nil {
			continue
		}
		if _, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl); ok {
			t.Errorf("%s: read as a function definition", src)
		}
	}
}

// The two spellings agree once the flag is on, which is the disagreement that
// made this an issue: `function a\ b { … }` defined a function here while
// `a\ b() { … }` was a parse error.
func TestBothSpellingsTakeTheSameName(t *testing.T) {
	d := anyWordPosixName()
	d.FunctionKeyword = true
	d.FunctionKeywordNameIsAnyWord = true
	for _, tc := range []struct{ keyword, posix string }{
		{`function 'a b' { echo b; }`, `'a b'() { echo b; }`},
		{`function a\ b { echo b; }`, `a\ b() { echo b; }`},
		{`function '' { echo b; }`, `''() { echo b; }`},
	} {
		kf, err := Parse(tc.keyword, d)
		if err != nil {
			t.Fatalf("%s: %v", tc.keyword, err)
		}
		pf, err := Parse(tc.posix, d)
		if err != nil {
			t.Fatalf("%s: %v", tc.posix, err)
		}
		kn := kf.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl).Name
		pn := pf.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl).Name
		if kn != pn {
			t.Errorf("%s names %q and %s names %q", tc.keyword, kn, tc.posix, pn)
		}
	}
}
