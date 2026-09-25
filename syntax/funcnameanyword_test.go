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
	t.Parallel()
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
// the quoted rows above include `'a*b'`. The dialect that generates the name
// from that match keeps the word rather than refusing the line — see
// [Dialect.FunctionNameIsFilenameGenerated], which this one has not got.
func TestABarePatternIsNotAPosixFunctionName(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// Dialect.FunctionNameIsFilenameGenerated, both answers, over both spellings.
//
// The flag decides whether a name holding a bare pattern character is a word
// the shell generates when the definition runs or a word the grammar refuses.
// Every row is written twice for that reason: the point is not what the name
// comes to — this package never matches anything — but whether there is a
// definition here at all.
//
// The word is kept on the declaration rather than flattened, which is the
// half a "does it parse" check cannot see: [FuncDecl.Name] would take the
// literal spelling and define a function called `a*b`, which is the wrong
// answer the refusal existed to avoid. So every row asserts NameWord.
func TestAFunctionNameMayBeGeneratedWhenTheDefinitionRuns(t *testing.T) {
	t.Parallel()
	gen := Core()
	gen.FunctionNameIsAnyWord = true
	gen.FunctionKeywordNameIsAnyWord = true
	gen.FunctionKeyword = true
	gen.FunctionNameIsFilenameGenerated = true
	off := gen
	off.FunctionNameIsFilenameGenerated = false
	for _, tc := range []struct{ name, src, written string }{
		{"a question mark, the name() spelling", `?x() { :; }`, "?x"},
		{"a question mark, the keyword spelling", `function ?x { :; }`, "?x"},
		{"a star", `a*b() { :; }`, "a*b"},
		{"a bracket", `a[b]c() { :; }`, "a[b]c"},
		{"a star after the keyword", `function a*b { :; }`, "a*b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.src, gen)
			if err != nil {
				t.Fatalf("with the flag on, parse %q: %v", tc.src, err)
			}
			fn := funcDeclOf(t, f)
			if fn.NameWord == nil {
				t.Fatalf("%q kept no name word — the literal spelling would "+
					"define a function called %q, which is the wrong answer", tc.src, fn.Name)
			}
			if got := PrintWord(fn.NameWord); got != tc.written {
				t.Errorf("the name word is %q, want %q", got, tc.written)
			}
			// And with the flag off the same text is not a definition at
			// all, which is what this parser did for every dialect before.
			if _, err := Parse(tc.src, off); err == nil {
				t.Errorf("with the flag off, %q parsed — the pattern character "+
					"takes the definition reading away there", tc.src)
			}
		})
	}
}

// The rows the flag does **not** reach: quoting is what decides whether a
// character is a pattern, and it is read per span rather than over the name's
// text — so a name that comes to `a*b` through quotes is an ordinary name and
// keeps no word.
func TestAQuotedPatternIsAnOrdinaryFunctionName(t *testing.T) {
	t.Parallel()
	d := Core()
	d.FunctionNameIsAnyWord = true
	d.FunctionKeywordNameIsAnyWord = true
	d.FunctionKeyword = true
	d.FunctionNameIsFilenameGenerated = true
	for _, tc := range []struct{ name, src, want string }{
		{"a quoted pattern", `'a*b'() { :; }`, "a*b"},
		{"an escaped pattern", `a\*b() { :; }`, "a*b"},
		{"an escaped pattern after the keyword", `function a\*b { :; }`, "a*b"},
		{"a name with no pattern in it at all", `ab() { :; }`, "ab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.src, d)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			fn := funcDeclOf(t, f)
			if fn.NameWord != nil {
				t.Errorf("%q kept a name word — nothing in it is a live pattern", tc.src)
			}
			if fn.Name != tc.want {
				t.Errorf("the name is %q, want %q", fn.Name, tc.want)
			}
		})
	}
}

// funcDeclOf digs the declaration out of a one-statement file.
func funcDeclOf(t *testing.T, f *File) *FuncDecl {
	t.Helper()
	if len(f.Stmts) != 1 {
		t.Fatalf("%d statements, want 1", len(f.Stmts))
	}
	fn, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl)
	if !ok {
		t.Fatalf("did not parse to a function declaration")
	}
	return fn
}
