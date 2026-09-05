// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// `==` in `test` is not a wording difference dressed up as one. Where the
// answer is no the word is not an operator, so the expression is read a
// different way and reports a different status — 2 for "this is not an
// expression" rather than 1 for "it is false". A test that only checked the
// true case would pass under either answer.
func runTestBuiltin(t *testing.T, a Answer, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.TestAcceptsDoubleEqual = a
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

func TestDoubleEqualIsAnOperatorOrIsNotAWordAtAll(t *testing.T) {
	for _, c := range []struct {
		name   string
		answer Answer
		src    string
		status int
		out    string
	}{
		{"yes, and equal", Yes, `test a == a`, 0, ""},
		{"yes, and unequal", Yes, `test a == b`, 1, ""},
		{"no, so not an expression", No, `test a == a`, 2, "operator"},
		{"no, and the same for unequal operands", No, `test a == b`, 2, "operator"},
		{"unanswered is refused", Unspecified, `test a == a`, 2, "no dialect was chosen"},
		// The single `=` is unanimous and must not move with the answer.
		{"single equals where == is refused", No, `test a = a`, 0, ""},
		{"single equals where == is unanswered", Unspecified, `test a = a`, 0, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runTestBuiltin(t, c.answer, c.src)
			if st != c.status {
				t.Errorf("%s: status %d, want %d (output %q)", c.src, st, c.status, out)
			}
			if c.out == "" {
				if out != "" {
					t.Errorf("%s: said %q, want silence", c.src, out)
				}
			} else if !strings.Contains(out, c.out) {
				t.Errorf("%s: said %q, want it to mention %q", c.src, out, c.out)
			}
		})
	}
}

// The refusal is reported once. Falling through to another reading of the
// same three words would complain about them a second time, and the second
// complaint — a malformed expression — is a consequence of the refusal rather
// than anything the script did.
func TestARefusedOperatorIsNotAlsoAMalformedExpression(t *testing.T) {
	out, _ := runTestBuiltin(t, Unspecified, `test a == b`)
	if n := strings.Count(out, "\n"); n != 1 {
		t.Errorf("said %q (%d lines), want one", out, n)
	}
	if strings.Contains(out, "operator expected") {
		t.Errorf("said %q, want no second complaint about the same words", out)
	}
}

// `[` is `test` under another name, and a diagnostic blames the name that was
// typed. The wordings carry it, so a fixed "test:" in them looks right until
// the same expression is written the other way.
func TestADiagnosticNamesTheWordThatWasTyped(t *testing.T) {
	// Not the substrate's empty wordings, which name nothing at all: this is
	// about a wording that does name the builtin, so it needs one.
	dg := Diagnostics{
		TestBinaryExpected:   "%[2]s: %[1]s: binary operator expected",
		TestTooManyArguments: "%[2]s: too many arguments",
	}
	for _, c := range []struct{ src, want string }{
		{`test a b c`, "test: b: binary operator expected"},
		{`[ a b c ]`, "[: b: binary operator expected"},
		// A wording with no operand in it still has a name in it, and the
		// operand it is handed must not surface as a stray argument.
		{`test a = a = a`, "test: too many arguments"},
		{`[ a = a = a ]`, "[: too many arguments"},
	} {
		f, err := syntax.Parse(c.src, syntax.Core())
		if err != nil {
			t.Fatalf("parse %q: %v", c.src, err)
		}
		var buf bytes.Buffer
		sem := permissive()
		r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run %q: %v", c.src, err)
		}
		got := buf.String()
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: said %q, want it to contain %q", c.src, got, c.want)
		}
		if strings.Contains(got, "%!") {
			t.Errorf("%s: said %q, want no formatting wreckage", c.src, got)
		}
	}
}
