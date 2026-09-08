// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// anyWordNamed is the core with the keyword and the flag this file is about.
func anyWordNamed() Dialect {
	d := Core()
	d.FunctionKeyword = true
	d.FunctionKeywordParens = true
	d.FunctionKeywordNameIsAnyWord = true
	return d
}

// keywordOnly is the same grammar with the flag off: the shape four of the
// six panel shells have, where the keyword is a keyword and a name is checked
// as text.
func keywordOnly() Dialect {
	d := anyWordNamed()
	d.FunctionKeywordNameIsAnyWord = false
	return d
}

// After the keyword, the word is the name whatever is in it (#1548, #1560).
//
// One shell in the panel reads it that way and no other. The rows are picked
// so that no character class could pass them: a semicolon and a pipe end a
// word when they are bare, so a wider set of *name characters* cannot be what
// this is — the quoting is.
func TestAnyWordIsAKeywordFunctionName(t *testing.T) {
	on, off := anyWordNamed(), keywordOnly()
	for _, src := range []string{
		`function '' { echo b; }`,
		`function "" { echo b; }`,
		`function '' () { echo b; }`,
		`function 'a b' { echo b; }`,
		`function a\ b { echo b; }`,
		`function 'a;b' { echo b; }`,
		`function 'a|b' { echo b; }`,
		`function 'a$b' { echo b; }`,
		`function 'a*b' { echo b; }`,
		`function 'f}' { echo b; }`,
		`function "a'b" { echo b; }`,
		`function 'a b c' { echo b; }`,
	} {
		if _, err := Parse(src, on); err != nil {
			t.Errorf("%s: refused where the flag allows: %v", src, err)
		}
		if _, err := Parse(src, off); err == nil {
			t.Errorf("%s: parsed without the flag", src)
		}
	}
	// The control: a name of nothing but punctuation the *other* flag already
	// carries. It parses either way, which is what says the rows above
	// measure the quoting rather than the characters.
	for _, d := range []Dialect{on, off} {
		d.FunctionNamePunctuation = true
		if _, err := Parse(`function '@#%' { echo b; }`, d); err != nil {
			t.Errorf("`function '@#%%'` refused with the punctuation flag: %v", err)
		}
	}
}

// The names the flag does not carry: the ones the shell matches against the
// filesystem.
//
// Measured on zsh 5.9.2 — `function a*b { :; }` is `no matches found: a*b`,
// `a?b` the same, `a[b` is `bad pattern: a[b` — so nothing is defined there
// either, and taking the word as a literal name here would put `a*b` in the
// table at status 0. Quoting is what decides it and is read per span, which
// only the last two rows can say: `a*'b'` still has a bare `*`.
func TestABarePatternIsNotAKeywordFunctionName(t *testing.T) {
	on := anyWordNamed()
	for _, src := range []string{
		`function a*b { :; }`,
		`function a?b { :; }`,
		`function a[b { :; }`,
		`function *  { :; }`,
		`function a*'b' { :; }`,
	} {
		if _, err := Parse(src, on); err == nil {
			t.Errorf("%s: parsed, where the shell matches the name against the filesystem", src)
		}
	}
	for _, src := range []string{
		`function 'a*b' { :; }`,
		`function 'a?b' { :; }`,
		`function 'a[b' { :; }`,
		`function "a*b" { :; }`,
	} {
		if _, err := Parse(src, on); err != nil {
			t.Errorf("%s: refused, where the pattern characters are quoted text: %v", src, err)
		}
	}
}

// The name the tree carries is the word's text and not the source spelling.
//
// "Did it parse" cannot see this, and it is what the interpreter reads: a
// definition whose Name came back as `'a b'` would be a five-character name
// that `'a b'` never calls.
func TestAKeywordFunctionNameIsTheWordsText(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`function '' { echo b; }`, ""},
		{`function 'a b' { echo b; }`, "a b"},
		{`function a\ b { echo b; }`, "a b"},
		{`function 'a$b' { echo b; }`, "a$b"},
		{`function "a'b" { echo b; }`, "a'b"},
	} {
		f, err := Parse(tc.src, anyWordNamed())
		if err != nil {
			t.Fatalf("%s: refused: %v", tc.src, err)
		}
		fn, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl)
		if !ok {
			t.Fatalf("%s: not a function declaration", tc.src)
		}
		if fn.Name != tc.want {
			t.Errorf("%s: Name is %q, want %q", tc.src, fn.Name, tc.want)
		}
		if fn.NameWord != nil {
			t.Errorf("%s: NameWord is %v, want nil: a quoted literal is not an expansion", tc.src, fn.NameWord)
		}
	}
}

// The flag is about the *name* and not about the keyword's other refusals.
//
// parseFuncKeyword fails in three places, and the flag reaches one of them.
// A fix written at the wrong one, or written before the token kind is
// checked, opens the other two — and each of those is a construct the shell
// with the flag reads as something else entirely.
func TestTheKeywordsOtherRefusalsStand(t *testing.T) {
	on := anyWordNamed()
	// The first: what follows the keyword is not a word at all.
	for _, src := range []string{
		"function\n",
		`function > f { :; }`,
		`function | x`,
		`function ; :`,
	} {
		if _, err := Parse(src, on); err == nil {
			t.Errorf("%s: parsed, where the keyword has no name to read", src)
		}
	}
	// The second: a name that is an expansion, where the dialect does not
	// expand one. The flag must not have let a word whose text is not known
	// yet through the test meant for text.
	expands := on
	expands.FunctionNameExpands = false
	for _, src := range []string{
		`function ${w} { :; }`,
		`function _p_${w} { :; }`,
		`function "$w" { :; }`,
	} {
		if _, err := Parse(src, expands); err == nil {
			t.Errorf("%s: parsed, where the dialect does not expand a name", src)
		}
	}
	// And the POSIX form's name is still decided by the punctuation flag, so
	// the widening did not leak across the two productions.
	punct := on
	punct.FunctionNamePunctuation = false
	if _, err := Parse(`a-b() { :; }`, punct); err == nil {
		t.Error("`a-b()` parsed without the punctuation flag")
	}
}

// The empty name is not the empty *word list*, which is a different
// construct and stays a different one (#1295).
//
// `function a b { … }` is the multi-name form and is refused here, at the
// brace rather than at the name — the shell with the flag defines both
// functions and we do not. A fix that let the name check pass on anything
// would land here instead, and the failure would read as an unbalanced brace.
func TestTheWidenedNameIsNotTheMultiNameForm(t *testing.T) {
	on := anyWordNamed()
	for _, src := range []string{
		`function a b { :; }`,
		`function '' a { :; }`,
		`function a '' { :; }`,
		`function 'a b' c { :; }`,
	} {
		if _, err := Parse(src, on); err == nil {
			t.Errorf("%s: parsed, where the multi-name form is not read", src)
		}
	}
	// And the anonymous form, which is what the keyword with *no* name word
	// means, is its own flag and is not reached by this one.
	anon := on
	anon.AnonymousFunction = false
	if _, err := Parse(`function { echo a; }`, anon); err == nil {
		t.Error("`function { … }` parsed without the anonymous-function flag")
	}
}

// The POSIX form does not gain anything from this flag.
//
// looksLikeFuncDef refuses a quoted word before the name test is reached, so
// `”() { … }` is a parse error with the flag on — which is what this
// implementation did before the flag and still does. The shell with the flag
// takes that line too, and so does ksh93; that is a second gap, at a
// different site, and pinning it here is what keeps the two from being
// confused for one (#1561).
func TestThePosixFormStillRefusesAQuotedName(t *testing.T) {
	on := anyWordNamed()
	for _, src := range []string{
		`''() { :; }`,
		`'q'() { :; }`,
		`'a b'() { :; }`,
	} {
		if _, err := Parse(src, on); err == nil {
			t.Errorf("%s: parsed, where a quoted name is not a definition here", src)
		}
	}
	// The control, without which the three above would pass in a dialect
	// that reads no POSIX definition at all.
	if _, err := Parse(`q() { :; }`, on); err != nil {
		t.Fatalf("`q() { :; }` refused, so the rows above measure nothing: %v", err)
	}
}

// A name that cannot be written bare is printed quoted, because printing it
// bare is a different program that parses.
func TestAKeywordFunctionNamePrintsBackAsItself(t *testing.T) {
	for _, src := range []string{
		`function '' { echo b; }`,
		`function 'a b' { echo b; }`,
		`function 'a;b' { echo b; }`,
		`function 'a|b' { echo b; }`,
		`function 'a$b' { echo b; }`,
		`function 'a*b' { echo b; }`,
		`function "a'b" { echo b; }`,
		`function a-b { echo b; }`,
	} {
		d := anyWordNamed()
		d.FunctionNamePunctuation = true
		f, err := Parse(src, d)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		printed := Print(f)
		back, err := Parse(printed, d)
		if err != nil {
			t.Errorf("%s: printed as %q, which does not parse: %v", src, printed, err)
			continue
		}
		got := back.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl).Name
		want := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl).Name
		if got != want {
			t.Errorf("%s: printed as %q, which names %q rather than %q", src, printed, got, want)
		}
	}
}
