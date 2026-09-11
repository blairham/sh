// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// keywordBodyDialect is the core plus whichever of the two body-shape flags a
// case is about. The keyword is core, so nothing else has to be turned on.
func keywordBodyDialect(set func(*syntax.Dialect)) syntax.Dialect {
	d := syntax.Core()
	if set != nil {
		set(&d)
	}
	return d
}

// TestTheKeywordBodyFollowsTheCompoundFlag — the flag that refuses
// `f() echo hi` refuses `function f` with the same body, which it did not
// before: it was read in the parenthesized production and nowhere else, so a
// definition one grammar calls a syntax error was read and defined at status
// 0 (#1833).
func TestTheKeywordBodyFollowsTheCompoundFlag(t *testing.T) {
	strict := keywordBodyDialect(func(d *syntax.Dialect) { d.FuncBodyMustBeCompound = true })
	for _, src := range []string{"function f\necho hi", "function f echo hi", "function f\nx=1"} {
		if _, err := syntax.Parse(src, strict); err == nil {
			t.Errorf("%q: the refusing grammar accepted a simple body", src)
		}
	}
	// A compound body of any shape is still a body to this grammar.
	for _, src := range []string{
		"function f { echo hi; }",
		"function f\n(( 1 ))",
		"function f\n( echo hi )",
		"function f\nfor i in 1; do echo hi; done",
	} {
		if _, err := syntax.Parse(src, strict); err != nil {
			t.Errorf("%q: a compound body refused: %v", src, err)
		}
	}
	// And without the flag the simple command is a one-command body.
	if _, err := syntax.Parse("function f\necho hi", syntax.Core()); err != nil {
		t.Errorf("the accepting grammar refused: %v", err)
	}
}

// TestTheKeywordBodyCanBeHeldToABraceGroup — the narrower rule, which is a
// second flag rather than a stronger reading of the first: the grammar that
// wants braces after the keyword takes a bare simple command after the
// parentheses, so one flag could not say both.
func TestTheKeywordBodyCanBeHeldToABraceGroup(t *testing.T) {
	braces := keywordBodyDialect(func(d *syntax.Dialect) {
		d.FunctionKeywordBodyMustBeBraceGroup = true
	})
	for _, src := range []string{
		"function f\necho hi",
		"function f\n(( 1 ))",
		"function f\n( echo hi )",
		"function f\nfor i in 1; do echo hi; done",
	} {
		if _, err := syntax.Parse(src, braces); err == nil {
			t.Errorf("%q: a body that is not a brace group was accepted", src)
		}
	}
	for _, src := range []string{"function f { echo hi; }", "function f\n{ echo hi; }"} {
		if _, err := syntax.Parse(src, braces); err != nil {
			t.Errorf("%q: a brace group refused: %v", src, err)
		}
	}
	// The parenthesized form is a separate question and this flag is not it.
	if _, err := syntax.Parse("f() echo hi", braces); err != nil {
		t.Errorf("the parenthesized form was held to the keyword's rule: %v", err)
	}
}

// TestARefusedKeywordBodyNamesTheTokenItBeganWith — the same rule the
// parenthesized form follows: the shape is only known once the body has been
// read, so the token named has to be the one saved before reading it.
func TestARefusedKeywordBodyNamesTheTokenItBeganWith(t *testing.T) {
	for _, c := range []struct {
		name  string
		set   func(*syntax.Dialect)
		src   string
		token string
	}{
		{"compound", func(d *syntax.Dialect) { d.FuncBodyMustBeCompound = true }, "function f\necho hi", "echo"},
		{"compound", func(d *syntax.Dialect) { d.FuncBodyMustBeCompound = true }, "function f\nx=1", "x=1"},
		{"braces", func(d *syntax.Dialect) { d.FunctionKeywordBodyMustBeBraceGroup = true }, "function f\necho hi", "echo"},
		{"braces", func(d *syntax.Dialect) { d.FunctionKeywordBodyMustBeBraceGroup = true }, "function f\nfor i in 1; do :; done", "for"},
	} {
		_, err := syntax.Parse(c.src, keywordBodyDialect(c.set))
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Errorf("%s %q: refusal = %v, want a *syntax.Error", c.name, c.src, err)
			continue
		}
		if se.Kind != syntax.ErrUnexpected {
			t.Errorf("%s %q: kind = %v, want ErrUnexpected", c.name, c.src, se.Kind)
		}
		if se.Token != c.token {
			t.Errorf("%s %q: token = %q, want %q", c.name, c.src, se.Token, c.token)
		}
		if !strings.Contains(err.Error(), c.token) {
			t.Errorf("%s %q: %v does not name %q", c.name, c.src, err, c.token)
		}
	}
}
