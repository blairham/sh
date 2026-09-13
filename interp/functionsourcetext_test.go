// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A listing that says the definition back as it was **written** — see
// Diagnostics.FunctionListingIsSourceText and syntax.FuncDecl.SourceText.
//
// It is two flags and not one, because the text has to be *kept* by a parser
// and *written* by a vector, and a tree parsed by one dialect may be run by
// another's. The pair of controls below is the whole reason to say so: each
// flag alone leaves the ordinary layout in place, and only both together
// change the listing (#2610).

// sourceTextRun parses under a dialect that keeps a definition's source and
// runs it under the Diagnostics handed in. `keep` is what makes the parser
// half switchable, which is what the controls need.
func sourceTextRun(t *testing.T, src string, keep bool, dg Diagnostics, before func(*Runner)) (string, int) {
	t.Helper()
	d := syntax.Core()
	d.FunctionDefinitionIsSourceText = keep
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.DeclareOptions = "f"
	var out bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &dg,
		Dialect: &d, Dir: t.TempDir(), Name: "testsh",
	})
	r.SetFunctionLayout(syntax.Layout{Indent: "  ", Lines: true, BraceOpenSuffix: " "},
		syntax.Layout{Indent: "  ", Lines: true, BraceOpenSuffix: " "})
	if before != nil {
		before(r)
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), st
}

// The headline. The blanks, the comment and the terminator are all things the
// tree has already forgotten, so none of these can pass through the layout.
func TestAListingCanBeTheDefinitionsOwnSourceText(t *testing.T) {
	dg := Diagnostics{FunctionListingHeader: "%[1]s()%[2]s", FunctionListingIsSourceText: true}
	for _, tc := range []struct{ name, src, want string }{
		{"the blanks as written", "f(){   :;   }; typeset -f f", "f(){   :;   };"},
		{"a comment inside the body", "f() { # note\n :; }; typeset -f f", "f() { # note\n :; };"},
		{
			// Including whatever stood between the body and the terminator.
			"the blanks before the terminator too",
			"f() { :; }   ; typeset -f f", "f() { :; }   ;",
		},
		{
			// A body written over several lines, whose terminator is the
			// newline after the `}` — the row that says the span is taken
			// raw rather than trimmed at its ends.
			"a body written over several lines",
			"f() {\n  echo a\n  echo b\n}\ntypeset -f f",
			"f() {\n  echo a\n  echo b\n}\n",
		},
		{
			"the terminator, and nothing added after it",
			"f() { :; }; typeset -f f; echo AFTER", "f() { :; };AFTER\n",
		},
		{
			"nothing terminated it, so nothing terminates the listing",
			`eval "f() { :; }"; typeset -f f`, "f() { :; }",
		},
		{
			"each definition carries its own terminator",
			"f() { :; }; g() { :; }; typeset -f", "f() { :; };g() { :; };",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := sourceTextRun(t, tc.src, true, dg, nil)
			if out != tc.want || st != 0 {
				t.Errorf("%q = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The two controls, and they are the point of the pair. The same source under
// the same layout, with one half of the pair missing each time: a vector that
// asks for the text but was handed a tree that kept none, and a tree that
// kept the text run by a vector that does not write it. Both lay the body out
// and both end with a newline of their own.
func TestOneHalfOfThePairAloneLeavesTheLayoutInPlace(t *testing.T) {
	const src = "f(){   :;   }; typeset -f f"
	const want = "f(){   :\n}\n"
	out, st := sourceTextRun(t, src, false,
		Diagnostics{FunctionListingHeader: "%[1]s()%[2]s", FunctionListingIsSourceText: true}, nil)
	if out != want || st != 0 {
		t.Errorf("nothing kept the text: %q (status %d), want %q", out, st, want)
	}
	out, st = sourceTextRun(t, src, true,
		Diagnostics{FunctionListingHeader: "%[1]s()%[2]s"}, nil)
	if out != want || st != 0 {
		t.Errorf("nothing writes the text: %q (status %d), want %q", out, st, want)
	}
}

// A function whose body has not been read yet is asked about **first**.
//
// The name here is defined from source and only then marked, so its
// declaration carries the text of the body it used to have. A listing that
// reached for the source before asking would write that body back where the
// shell's own sentence about the name belongs — which is the whole of what
// SetUndefinedFunctions is for.
func TestAFunctionStillToBeReadIsNotListedFromItsOldSource(t *testing.T) {
	dg := Diagnostics{FunctionListingHeader: "%[1]s()%[2]s", FunctionListingIsSourceText: true}
	out, st := sourceTextRun(t, "f() { echo body; }; typeset -f f", true, dg,
		func(r *Runner) {
			r.SetUndefinedFunctions(func(name string) (string, bool) {
				return "{ # undefined\n}", name == "f"
			})
		})
	if want := "f(){ # undefined\n}\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q — the old body came back", out, st, want)
	}
}
