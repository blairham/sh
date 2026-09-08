// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// A declaration printed and run again declares the same locals it did.
//
// This is the shell where the question exists. `typeset` in a `function f {
// …; }` body declares a local here and in an `f() { …; }` body assigns the
// global — Semantics.TypesetLocalNeedsKeywordFunction — so the two spellings
// are two programs, and a print that wrote back the second for the first
// handed over a program whose locals leak. Measured 2026-09-08 on ksh93u+
// 2012-08-01: the keyword body leaves the outer value alone and the bare one
// overwrites it, and `eval "$(typeset -f f)"` keeps the locality because
// ksh93's own listing writes the word back.
//
// The whole point of asserting it here rather than only on the printed text
// is that the text is not the claim: the claim is that the program still
// does what it did, and only running it says so. A printer test that
// compared strings would pass against a word written where this grammar
// reads something else.
func TestAPrintedKeywordFunctionStillDeclaresLocals(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the keyword form keeps its local",
			"v=g\nfunction f { typeset v=in; }\nf\necho \"out=$v\"\n",
			"out=g",
		},
		{
			// The control, and the one that says the test can tell the two
			// apart at all: the same body without the word leaks, before
			// and after the trip alike.
			"the bare form still leaks",
			"v=g\nf() { typeset v=in; }\nf\necho \"out=$v\"\n",
			"out=in",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			direct, st := runKshSource(t, tc.src)
			if st != 0 || !strings.Contains(direct, tc.want) {
				t.Fatalf("as written = %q (status %d), want %q", direct, st, tc.want)
			}
			f, err := syntax.Parse(tc.src, ksh.Dialect())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			printed := syntax.Print(f)
			again, st := runKshSource(t, printed)
			if st != 0 || !strings.Contains(again, tc.want) {
				t.Errorf("printed as %q, which answers %q (status %d), want %q — "+
					"the print changed which variables are local",
					printed, again, st, tc.want)
			}
		})
	}
}

// runKshSource runs one source string under this dialect.
func runKshSource(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}
