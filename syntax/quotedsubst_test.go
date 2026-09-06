// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// nested is the core with the grammar this file is about.
func nested() Dialect {
	d := Core()
	d.NestedParamExpansion = true
	// The flag group is the other half of the idiom: `${(@f)"$(cmd)"}` needs
	// both, and a dialect with one and not the other cannot write it.
	d.ParamExpansionFlags = true
	return d
}

// innerOf parses `echo ${…}` and hands back the outer expansion's inner word.
func innerOf(t *testing.T, src string) *ParamExpr {
	t.Helper()
	f, err := Parse("echo "+src, nested())
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	cmd := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	for _, sp := range cmd.Args[1].Spans {
		if sp.Kind == ParamExp {
			return sp.Param
		}
	}
	t.Fatalf("%s: no parameter expansion in the word", src)
	return nil
}

// TestAQuotedSubstitutionStandsWhereANameBelongs — `${(@f)"$(cmd)"}` is the
// idiom for splitting a command's output into an array by line, and the
// quotes are load-bearing rather than decoration: quoted, the inner comes to
// one field for the flags to split.
//
// Asserted on the span's *quoting* as well as its kind, because a parser that
// stripped the quotes would build a tree that parses and means the other
// program.
func TestAQuotedSubstitutionStandsWhereANameBelongs(t *testing.T) {
	for _, tc := range []struct {
		src     string
		kind    SpanKind
		quoting Quoting
	}{
		{`${(@f)"$(cmd)"}`, CommandSubst, DoubleQuoted},
		{`${(f)"$(cmd)"}`, CommandSubst, DoubleQuoted},
		{`${"${v}"}`, ParamExp, DoubleQuoted},
		// And the unquoted spellings still parse, unchanged.
		{`${(@f)$(cmd)}`, CommandSubst, Unquoted},
		{`${${v}}`, ParamExp, Unquoted},
	} {
		e := innerOf(t, tc.src)
		if e.Inner == nil || len(e.Inner.Spans) != 1 {
			t.Errorf("%s: inner = %v, want one span", tc.src, e.Inner)
			continue
		}
		got := e.Inner.Spans[0]
		if got.Kind != tc.kind || got.Quoting != tc.quoting {
			t.Errorf("%s: inner span is %v/%v, want %v/%v",
				tc.src, got.Kind, got.Quoting, tc.kind, tc.quoting)
		}
	}
}

// TestTheQuotesDoNotWidenWhatMayStandThere — what the name position holds is
// a *substitution*, and quoting a string does not make it one. Every one of
// these is a bad substitution in the shell with the grammar, measured, and
// the last two are the pair that says the rule inside the quotes is the same
// rule as outside them.
func TestTheQuotesDoNotWidenWhatMayStandThere(t *testing.T) {
	for _, src := range []string{
		`${"abc"}`,  // a quoted string is not a substitution
		`${'$(c)'}`, // single quotes are literal text
		"${`c`}",    // the backquoted spelling is not one of the two
		`${"$v"}`,   // unbraced, quoted — refused, as `${$v}` is
		`${$v}`,     // unbraced, unquoted — the same refusal
	} {
		e := innerOf(t, src)
		if e.Inner != nil {
			t.Errorf("%s: read %v as a substitution in the name position", src, e.Inner)
		}
	}
	// An unterminated quote is not one either, and it fails as an
	// unterminated *quote* rather than as anything about the name position:
	// the shape is decided by what the lexer read, so a `"` that never
	// closed never gets that far.
	if _, err := Parse(`echo ${"$(c)`, nested()); err == nil {
		t.Error(`${"$(c) parsed, want an unterminated quote`)
	}
}
