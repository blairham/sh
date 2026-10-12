// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `${+parameters}` is answered without building the table of every
// parameter. It is set whatever the table holds, and F-Sy-H asks it for every
// word of the command line at every keystroke, where building the table cost
// about 1.5 ms with three thousand parameters (#6363). The bound is a small
// fraction of what one build of that table allocates.
func TestTheParametersTableIsNotBuiltToSayItIsSet(t *testing.T) {
	call := localCostRunner(t, `for i in {1..3000}; do typeset -g v$i=x; done`, `(( ${+parameters} )) || exit 3`)
	call()
	const n = 20
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for range n {
		call()
	}
	runtime.ReadMemStats(&after)
	if got := (after.TotalAlloc - before.TotalAlloc) / n; got > 16<<10 {
		t.Errorf("${+parameters} allocates %d bytes a read with 3,000 parameters; want under 16 KB — the table is being built to answer", got)
	}
}

// The answers are what zsh 5.9.2 gives, under `-f`, and a table whose
// building is visible is still built: `: ${+commands}` fills the command hash
// there — `hash | wc -l` goes from 0 to the number of commands on the path —
// so it is not on the list that may skip it.
func TestASetTestOnAProducedTableAnswersAsBefore(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print ${+parameters} ${+parameters[PATH]} ${+parameters[nope]} ${+nosuch}`, "1 1 0 0\n"},
		{`f() { local parameters=x; print ${+parameters} $parameters; }; f`, "1 x\n"},
		{`PATH=/bin:/usr/bin; [[ -z $(hash) ]] && : ${+commands} && [[ -n $(hash) ]] && print hashed`, "hashed\n"},
	} {
		var out strings.Builder
		sem, diag, dl := Semantics(), Diagnostics(), Dialect()
		r := &interp.Runner{
			Stdout: &out, Stderr: &out,
			Semantics: &sem, Diagnostics: &diag, Dialect: &dl,
			Dir: t.TempDir(), Name: "zsh",
		}
		Apply(r)
		prog, err := syntax.Parse(tc.src, Dialect())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Run(context.Background(), prog); err != nil {
			t.Fatal(err)
		}
		if out.String() != tc.want {
			t.Errorf("%s said %q, want %q", tc.src, out.String(), tc.want)
		}
	}
}
