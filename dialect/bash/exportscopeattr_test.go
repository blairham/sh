// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The export attribute a declaration writes inside a function belongs to the
// call, exactly as every other attribute a declaration writes does.
//
// Measured 2026-09-15 on bash 5.3.20, on bash 3.2.57 and on ksh93u+ 2012, all
// three of which answer the same thing through their own spellings. The
// letter used to write a fact about the *global* record, which the scope had
// no entry for and so never put back:
//
//	f(){ declare -x A=1; }; f; declare -p A     ours: `declare -x A`, 0
//	                                            bash: `A: not found`, 1
//	A=outer; f(){ declare -x A=1; }; f; env     ours: A=outer reaches a child
//	                                            bash: nothing does
//	export B=o; f(){ declare +x B; }; f; env    ours: nothing reaches a child
//	                                            bash: B=o does
//
// The last two are the half that matters away from `declare -p`: a function's
// own declaration decided what the *caller's* variable put in a child's
// environment, in both directions, for the rest of the run.
//
// The dialect that reads `-x` as `-g` never arrives at the shadow at all —
// the letter takes no shadow there, which is what the letter means — so this
// is inert for it and its own answer is unchanged. See
// Semantics.ExportLetterDeclaresAGlobal.
func TestTheExportLetterInsideAFunctionDoesNotOutliveTheCall(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want string
	}{{
		"a declaration of a name that did not exist leaves none behind",
		`f(){ declare -x A=1; }; f; declare -p A; echo "st=$?"`,
		"st=1\n",
	}, {
		"and the same under local",
		`f(){ local -x A=1; }; f; declare -p A; echo "st=$?"`,
		"st=1\n",
	}, {
		"and the same under typeset",
		`f(){ typeset -x A=1; }; f; declare -p A; echo "st=$?"`,
		"st=1\n",
	}, {
		"a caller's unexported name is not exported by the callee's letter",
		`A=outer; f(){ declare -x A=inner; }; f; declare -p A`,
		"declare -- A=\"outer\"\n",
	}, {
		"and a caller's exported name is not unexported by the callee's plus",
		`export B=outer; f(){ declare +x B; }; f; declare -p B`,
		"declare -x B=\"outer\"\n",
	}, {
		"a caller's exported name comes back exported after a shadow",
		`export C=outer; f(){ declare -x C=inner; }; f; declare -p C`,
		"declare -x C=\"outer\"\n",
	}, {
		// The other side of the same coin, and the reason this is a scope
		// question rather than a letter question: `export` is not a
		// declaration into the call and writes the global record in bash.
		"export itself still reaches past the call",
		`f(){ export D=1; }; f; declare -p D`,
		"declare -x D=\"1\"\n",
	}, {
		// And so does the letter that says so.
		"and so does the global letter",
		`f(){ declare -gx E=1; }; f; declare -p E`,
		"declare -x E=\"1\"\n",
	}} {
		t.Run(c.name, func(t *testing.T) {
			out, errs := bashRun(t, c.src)
			if out != c.want {
				t.Errorf("got %q (stderr %q), want %q", out, errs, c.want)
			}
		})
	}
}

// bashRun runs one snippet under the bash dialect and answers what it wrote,
// with the shell's own diagnostics kept apart: a refusal this file is about
// names the variable, which is the sentence under test rather than noise.
func bashRun(t *testing.T, src string) (string, string) {
	t.Helper()
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var out, errs strings.Builder
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Stdout: &out, Stderr: &errs,
		Semantics: &sem, Diagnostics: &diag, Name: "bash",
		Dialect: presetDialect(),
	}
	bash.Apply(r)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatal(rerr)
	}
	return out.String(), errs.String()
}
