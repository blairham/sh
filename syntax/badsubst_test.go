// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"strings"
	"testing"
)

// An unrecognized operator inside ${ } is refused at parse time by exactly
// one grammar and deferred to the run by the rest, so both directions of the
// flag are pinned — and the deferred node keeps the raw text, because the
// runtime report needs the construct as written.

func TestABadOperatorIsDeferredByDefault(t *testing.T) {
	f, err := Parse(`echo "${foo ~}"`, Core())
	if err != nil {
		t.Fatalf("the default grammar should defer, got %v", err)
	}
	var found *ParamExpr
	walkParams(f, func(e *ParamExpr) {
		if e.Bad {
			found = e
		}
	})
	if found == nil {
		t.Fatal("no Bad node in the tree")
	}
	if found.Src != "foo ~" {
		t.Errorf("Src = %q, want the raw inside of the braces", found.Src)
	}
}

func TestABadOperatorIsAParseErrorWhenAsked(t *testing.T) {
	d := Core()
	d.BadSubstitutionAtParseTime = true
	_, err := Parse(`echo "${foo ~}"`, d)
	if err == nil {
		t.Fatal("the refusing grammar accepted it")
	}
	var pe *Error
	if !errors.As(err, &pe) || pe.Kind != ErrBadSubstitution {
		t.Errorf("err = %v, want ErrBadSubstitution", err)
	}
}

// The grammar that refuses while reading names the character standing where
// the parameter belonged, and with nothing between the braces that character
// is the brace that closed them.
//
// Measured 2026-09-08: `${}` in ksh93 is `syntax error at line 1: `}'
// unexpected` and `${%x}` there is `` `%' unexpected ``. The empty case is
// the one worth a test, because the token is the only part of the report that
// carries it and an empty one reads as `` `' `` — a diagnostic that names
// nothing at all, which is what this used to print.
func TestTheEmptyBracesRefusalNamesTheClosingBrace(t *testing.T) {
	d := Core()
	d.BadSubstitutionAtParseTime = true
	for _, tc := range []struct{ src, token string }{
		{`echo "${}"`, "}"},
		{`echo "${%x}"`, "%"},
		{`echo "${ }"`, " "},
	} {
		_, err := Parse(tc.src, d)
		var pe *Error
		if !errors.As(err, &pe) {
			t.Errorf("%s: err = %v, want a refusal", tc.src, err)
			continue
		}
		if pe.Token != tc.token {
			t.Errorf("%s: token = %q, want %q", tc.src, pe.Token, tc.token)
		}
	}
}

// walkParams visits every ParamExpr span in the file's simple commands, which
// is all these tests need.
func walkParams(f *File, visit func(*ParamExpr)) {
	for _, st := range f.Stmts {
		pipe, ok := st.Expr.(*Pipeline)
		if !ok {
			continue
		}
		cmd, ok := pipe.Cmds[0].(*SimpleCmd)
		if !ok {
			continue
		}
		for _, w := range cmd.Args {
			for i := range w.Spans {
				if w.Spans[i].Param != nil {
					visit(w.Spans[i].Param)
				}
			}
		}
	}
}

// The deferred node still prints back to source that parses to the same tree.
func TestABadNodeRoundTripsThroughThePrinter(t *testing.T) {
	src := `echo "${foo ~}"`
	f, err := Parse(src, Core())
	if err != nil {
		t.Fatal(err)
	}
	printed := Print(f)
	if !strings.Contains(printed, "${foo ~}") {
		t.Errorf("printed = %q, want the construct kept as written", printed)
	}
	if _, err := Parse(printed, Core()); err != nil {
		t.Errorf("printed form does not reparse: %v", err)
	}
}
