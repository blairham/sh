// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// **The stage is a question of its own, and it is a flag.** `function
// _p_${w} { … }` is refused by four of the six — that is the predicate, and
// [syntax.Dialect.FunctionNameExpands] is the one column that takes it — and
// two of those four *parse* it and complain only when the definition is
// reached.
//
// Measured 2026-09-10 through `-c`, over `w=foo; function _p_${w} { echo HI;
// }; echo st=$?; echo after`:
//
//	shell        what a run prints                         script status
//	bash 5.3.15  the complaint, then st=1 and after        0
//	bash 3.2.57  the same two lines                        0
//	bash-as-sh   the complaint, and stops                  2
//	ksh93u+      the complaint, and stops                  1
//	zsh 5.9.2    st=0 and after, `_p_foo` defined          0
//	dash         no keyword, and the `}` is where it stops 2
//
// A syntax check is what a CI job runs, so refusing the parse reported a
// script those two shells run as broken — and the refusal named no word,
// where all four of them quote the one they disliked. What happens when the
// definition *is* reached is interp.Semantics.FunctionNameWhenTheDefinitionRuns
// and not this flag (#1296).

// stagedFuncName is the core with the keyword and the stage flag, which is
// bash and ksh93.
func stagedFuncName() syntax.Dialect {
	d := syntax.Core()
	d.FunctionNameCheckedWhenTheDefinitionRuns = true
	return d
}

// The parse succeeds and the word is carried on the declaration, as the
// source text: what the shells quote is what was written.
func TestARefusedFunctionNameParsesUnderTheFlag(t *testing.T) {
	d := stagedFuncName()
	for _, tc := range []struct{ src, want string }{
		{`function _p_${w} { echo HI; }`, "_p_${w}"},
		{`function _p_$w { echo HI; }`, "_p_$w"},
		{`function _p_$@ { echo HI; }`, "_p_$@"},
		{"function _p_$(echo n) { echo HI; }", "_p_$(echo n)"},
		{`function ${w} { echo HI; }`, "${w}"},
	} {
		f, err := syntax.Parse(tc.src+"\n", d)
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		fn := funcDeclOf(t, f, tc.src)
		if fn.RefusedName != tc.want {
			t.Errorf("%q: RefusedName = %q, want %q", tc.src, fn.RefusedName, tc.want)
		}
		// And no name to bind, which is the half a second field would have
		// let drift: a declaration carrying one is a declaration the
		// interpreter could define. `_p_${w}` flattens to `_p_w`, a
		// perfectly good name for a perfectly wrong function.
		if fn.Name != "" || fn.NameWord != nil {
			t.Errorf("%q: Name = %q, NameWord = %v, want neither", tc.src, fn.Name, fn.NameWord)
		}
		// The body is read, which is the point of parsing at all: the script
		// after the definition is what carries on.
		if fn.Body == nil {
			t.Errorf("%q: the body was not read", tc.src)
		}
	}
}

// Without the flag the refusal stays where it was, which is the other half of
// the same claim: the predicate did not move, only the stage did.
func TestARefusedFunctionNameIsStillRefusedWithoutTheFlag(t *testing.T) {
	for _, src := range []string{
		`function _p_${w} { echo HI; }`,
		`function _p_$@ { echo HI; }`,
	} {
		if _, err := syntax.Parse(src+"\n", syntax.Core()); err == nil {
			t.Errorf("%q parsed without the flag, want the refusal kept", src)
		}
	}
}

// A usable name is unaffected by the flag, in either direction — the point
// being that nothing about an ordinary definition reads it.
func TestTheStageFlagDoesNotTouchAUsableFunctionName(t *testing.T) {
	for _, d := range []syntax.Dialect{syntax.Core(), stagedFuncName()} {
		f, err := syntax.Parse("function f { echo HI; }\n", d)
		if err != nil {
			t.Fatalf("%v", err)
		}
		fn := funcDeclOf(t, f, "function f")
		if fn.Name != "f" || fn.RefusedName != "" {
			t.Errorf("Name = %q, RefusedName = %q, want `f` and none", fn.Name, fn.RefusedName)
		}
	}
}

// **A word only.** `function ; { :; }` stays an ordinary unexpected-token
// failure under the flag, because what those two shells want in that position
// is a word and the *name* is what they check later.
//
// So the two questions stay apart: whether a word may stand there is the
// grammar's, and whether the word is a name is the definition's. Without this
// the flag would have carried a `;` to the interpreter as a refused name.
func TestANonWordAfterTheKeywordIsStillASyntaxError(t *testing.T) {
	d := stagedFuncName()
	for _, src := range []string{
		`function ; { :; }`,
		`function | { :; }`,
		`function`,
	} {
		if _, err := syntax.Parse(src+"\n", d); err == nil {
			t.Errorf("%q parsed, want an unexpected-token refusal", src)
		}
	}
}

// The dialect that *expands* such a name is untouched by the stage flag: the
// word is a name there and never reaches the refusal at all.
func TestAnExpandingDialectKeepsTheWordAsAName(t *testing.T) {
	d := stagedFuncName()
	d.FunctionNameExpands = true
	f, err := syntax.Parse("function _p_${w} { echo HI; }\n", d)
	if err != nil {
		t.Fatalf("%v", err)
	}
	fn := funcDeclOf(t, f, "function _p_${w}")
	if fn.NameWord == nil || fn.RefusedName != "" {
		t.Errorf("NameWord = %v, RefusedName = %q, want the word kept and no refusal",
			fn.NameWord, fn.RefusedName)
	}
}

// The printer writes a refused name back as it stood, so a declaration that
// parses and will fail when it runs fails the same way — with the same word
// quoted — after a round trip. Quoting it would hand back a program the shell
// refuses for a different reason, or accepts.
func TestARefusedFunctionNamePrintsBack(t *testing.T) {
	d := stagedFuncName()
	for _, tc := range []struct{ src, want string }{
		{`function _p_${w} { echo HI; }`, "function _p_${w} { echo HI; }"},
		{`function _p_$@ { echo HI; }`, "function _p_$@ { echo HI; }"},
	} {
		f, err := syntax.Parse(tc.src+"\n", d)
		if err != nil {
			t.Fatalf("%q: %v", tc.src, err)
		}
		got := syntax.Print(f)
		if got != tc.want {
			t.Errorf("%q: printed %q, want %q", tc.src, got, tc.want)
			continue
		}
		again, err := syntax.Parse(got, d)
		if err != nil {
			t.Errorf("%q printed as %q, which does not parse: %v", tc.src, got, err)
			continue
		}
		if why, ok := syntax.SameProgram(f, again); !ok {
			t.Errorf("%q: printed form is a different program: %s", tc.src, why)
		}
	}
}

// funcDeclOf digs the declaration out of a file, failing rather than
// returning an error: every row above has already established that the source
// parses.
func funcDeclOf(t *testing.T, f *syntax.File, src string) *syntax.FuncDecl {
	t.Helper()
	c, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.FuncDecl)
	if !ok {
		t.Fatalf("%q: not a function declaration", src)
	}
	return c
}
