// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// refusing builds a dialect that keeps two names for itself, at the stage the
// caller asks for.
//
// The names are a shell's special builtins in the two presets that have such
// a set, and nothing here says so: what the parser is given is a set of
// words, and `export` is a word like any other to it.
func refusingNames(atRun bool) syntax.Dialect {
	d := syntax.Core()
	d.FunctionNamesRefused = map[string]bool{"export": true, "set": true}
	d.FunctionNameCheckedWhenTheDefinitionRuns = atRun
	return d
}

// A name in the set is refused while the definition is read, where the
// dialect does not defer the check.
//
// The whole input is refused and not only the definition, which is the half
// that matters: a reading refusal reaches a definition in a branch nothing
// takes, and a running one cannot.
func TestARefusedNameIsASyntaxErrorWhileReading(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"export() { :; }\n",
		"set() { :; }\n",
		"if false; then export() { :; }; fi\n",
		"printf a; export() { :; }; printf b\n",
	} {
		if _, err := syntax.Parse(src, refusingNames(false)); err == nil {
			t.Errorf("%q parsed, where the dialect keeps the name", src)
		}
	}
}

// And where the dialect defers, the same definition parses whole and carries
// the word for the interpreter to complain about.
func TestARefusedNameParsesAndIsCarried(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"export() { :; }\n",
		"function export { :; }\n",
	} {
		f, err := syntax.Parse(src, refusingNames(true))
		if err != nil {
			t.Fatalf("%q refused while parsing: %v", src, err)
		}
		fn, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.FuncDecl)
		if !ok {
			t.Fatalf("%q is a %T, want a *FuncDecl", src, f.Stmts[0].Expr)
		}
		if fn.RefusedName != "export" {
			t.Errorf("%q: RefusedName = %q, want %q", src, fn.RefusedName, "export")
		}
	}
}

// A name the set does not hold is a name at either stage, which is the
// control: without it a test that refuses everything would pass both rows
// above.
func TestANameOutsideTheSetIsUntouched(t *testing.T) {
	t.Parallel()
	for _, atRun := range []bool{false, true} {
		for _, src := range []string{
			"exports() { :; }\n",
			"unset() { :; }\n",
			"read() { :; }\n",
		} {
			f, err := syntax.Parse(src, refusingNames(atRun))
			if err != nil {
				t.Fatalf("%q refused while parsing (atRun=%v): %v", src, atRun, err)
			}
			fn, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.FuncDecl)
			if !ok {
				t.Fatalf("%q is a %T, want a *FuncDecl", src, f.Stmts[0].Expr)
			}
			if fn.RefusedName != "" {
				t.Errorf("%q: RefusedName = %q, want none", src, fn.RefusedName)
			}
		}
	}
}

// An empty set refuses nothing, which is what the four presets without one
// hold and what a hand-built dialect has.
func TestNoSetRefusesNothing(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	if _, err := syntax.Parse("export() { :; }\n", d); err != nil {
		t.Errorf("a dialect with no set refused a definition: %v", err)
	}
}
