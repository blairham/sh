// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// nameExpands is the core with the one flag this file is about.
func nameExpands() Dialect {
	d := Core()
	d.FunctionNameExpands = true
	d.FunctionKeywordParens = true
	return d
}

// A function definition's name is a *word* where the dialect says so, so an
// expansion in one is kept rather than flattened (#1256).
//
// Both spellings, because they are two parsers: the parenthesised form is
// found by looksLikeFuncDef and the keyword form reads its name directly.
func TestAFunctionNameMayHoldAnExpansion(t *testing.T) {
	on, off := nameExpands(), Core()
	for _, src := range []string{
		`_p_${w}() { :; }`,
		`_p_$w() { :; }`,
		`_p_$(echo s)() { :; }`,
		`_p_$((1+1))() { :; }`,
		`${w}() { :; }`,
		`_p_${w}_x() { :; }`,
	} {
		if _, err := Parse(src, on); err != nil {
			t.Errorf("%s: refused where the flag allows: %v", src, err)
		}
		if _, err := Parse(src, off); err == nil {
			t.Errorf("%s: parsed without the flag", src)
		}
	}
	// The keyword form, which parsed *either* way before and was the worse
	// half: without the flag it took the token's literal text, so
	// `_p_${w}` defined `_p_w` — a real function, at status 0, and not the
	// one the script wrote. It is refused now.
	kw := nameExpands()
	kwOff := Core()
	kwOff.FunctionKeyword = true
	for _, src := range []string{
		`function _p_${w} { :; }`,
		`function _p_${w}() { :; }`,
	} {
		if _, err := Parse(src, kw); err != nil {
			t.Errorf("%s: refused where the flag allows: %v", src, err)
		}
		if _, err := Parse(src, kwOff); err == nil {
			t.Errorf("%s: parsed without the flag, where it used to define `_p_w`", src)
		}
	}
}

// The word is kept, not the literal — which is the whole of the fix, and
// what "did it parse" cannot see. `_p_${w}` and `_p_w` both parse and are
// different functions.
func TestAnExpandedFunctionNameKeepsItsWord(t *testing.T) {
	on := nameExpands()
	for _, src := range []string{`_p_${w}() { :; }`, `function _p_${w} { :; }`} {
		f, err := Parse(src, on)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		fn, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl)
		if !ok {
			t.Fatalf("%s: not a function declaration", src)
		}
		if fn.NameWord == nil {
			t.Errorf("%s: NameWord is nil, so the expansion was flattened to %q", src, fn.Name)
		}
	}
	// And an ordinary name does *not* carry one, so nothing downstream has
	// to expand a word that was never written as one.
	f, err := Parse(`plain() { :; }`, on)
	if err != nil {
		t.Fatal(err)
	}
	if fn := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl); fn.NameWord != nil {
		t.Errorf("a plain name carried a NameWord: %v", fn.NameWord)
	}
}

// A dialect that commits to a definition at the paren checks the name once
// the parens close, and a name that is not text yet cannot be checked there.
//
// No preset holds all three flags today, which is why this builds the
// dialect rather than naming a shell: the guard is what stops the two
// answers from colliding the moment one does, and without it every expanded
// name in such a dialect is `Bad function name`.
func TestAnExpandedNameIsNotCheckedAsTextAtTheParen(t *testing.T) {
	d := nameExpands()
	d.FuncDefAtParen = true
	d.FunctionNamePunctuation = false
	// The written name has to be one whose *literal text* is not a name, or
	// the check passes for the wrong reason and the guard is invisible:
	// `_p_${w}` flattens to `_p_w`, which `isName` is happy with. `_p_${w}-x`
	// flattens to `_p_w-x`, which it is not.
	for _, src := range []string{`_p_${w}-x() { :; }`, `_p_${w}.x() { :; }`, `${w}-x() { :; }`} {
		if _, err := Parse(src, d); err != nil {
			t.Errorf("%s: an expanded name where the paren commits: %v", src, err)
		}
	}
	if _, err := Parse(`_p_${w}() { :; }`, d); err != nil {
		t.Errorf("an expanded name where the paren commits: %v", err)
	}
	// And a plain name that is not one is still refused there, which is the
	// control: the guard exempts the word, not the check.
	if _, err := Parse(`f-g() { :; }`, d); err == nil {
		t.Error("`f-g() { :; }` parsed; the name check still applies to text")
	}
}

// Printed source has to mean the same thing, and this is exactly where it
// would not: printing the literal names a different function.
func TestAnExpandedFunctionNamePrintsBackAsAWord(t *testing.T) {
	on := nameExpands()
	for _, tc := range []struct{ src, want string }{
		// `${w}` prints as `$w`: the printer's existing normalization of a
		// parameter expansion, and the two are the same expansion. What
		// matters here is that it is still an *expansion* — printing the
		// literal `_p_w` would name a different function.
		{`_p_${w}() { :; }`, "_p_$w() { :; }"},
		{`_p_$w() { :; }`, "_p_$w() { :; }"},
		{`function _p_${w} { :; }`, "_p_$w() { :; }"},
		{`plain() { :; }`, "plain() { :; }"},
	} {
		f, err := Parse(tc.src, on)
		if err != nil {
			t.Errorf("%s: %v", tc.src, err)
			continue
		}
		if got := Print(f); got != tc.want {
			t.Errorf("%s: printed %q, want %q", tc.src, got, tc.want)
		}
		if _, err := Parse(Print(f), on); err != nil {
			t.Errorf("%s: printed form does not parse: %v", tc.src, err)
		}
	}
}

// The flag does not turn every word before a parenthesis into a definition.
// An array assignment is a parenthesis after a word too, and quoting is not
// an expansion.
func TestTheFlagDoesNotWidenWhatADefinitionIs(t *testing.T) {
	on := nameExpands()
	on.ArrayLiteral = true
	on.ArraySubscript = true
	for _, tc := range []struct{ src, why string }{
		{`a=()`, "an empty array assignment"},
		{`a=(1 2)`, "an array assignment"},
		// With an expansion in the subscript, which is where a loop puts
		// one and is the shape that made this a regression rather than a
		// hypothetical: `a[$i]=()` holds an expansion and is followed by an
		// empty parenthesis pair, so the expansion branch claimed it and a
		// real plugin stopped parsing.
		{`a[$i]=()`, "an element assigned an empty array"},
		{`a[$i]=(1 2)`, "an element assigned an array"},
		{`a[$i]+=()`, "an element appended an empty array"},
	} {
		f, err := Parse(tc.src, on)
		if err != nil {
			t.Errorf("%s (%s): %v", tc.src, tc.why, err)
			continue
		}
		if _, isFunc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl); isFunc {
			t.Errorf("%s (%s): read as a function definition", tc.src, tc.why)
		}
	}
	// The parentheses are the whole announcement where the name is a word,
	// because "is this text a name" cannot be asked of a word before it is
	// expanded. Without them the commonest line in any script — a command
	// held in a variable — became a definition.
	for _, src := range []string{`$c HI`, `${c} HI`, `$(echo echo) HI`, `${c}ho HI`} {
		f, err := Parse(src, on)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		if _, isFunc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl); isFunc {
			t.Errorf("%s: read as a function definition", src)
		}
	}
	// A quoted name is not an expansion and stays refused, which is the
	// behavior the flag was not asked to change.
	if _, err := Parse(`'q'() { :; }`, on); err == nil {
		t.Error("`'q'() { :; }` parsed; quoting is not an expansion")
	}
	// The other side of the subscript cases above, and what says the rule is
	// lexical rather than "no `=` anywhere": an `=` that follows an
	// *expansion* is not an assignment's, so the word is still a name — the
	// function it defines is called `_p_foo=`.
	f, err := Parse(`_p_${w}=() { :; }`, on)
	if err != nil {
		t.Fatalf("`_p_${w}=() { :; }`: %v", err)
	}
	if _, isFunc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl); !isFunc {
		t.Error("`_p_${w}=() { :; }`: not read as a function definition")
	}
}
