// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import (
	"context"
	"io"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// localCostRunner is a runner of this dialect with defs run, and a parsed
// call to make against it.
func localCostRunner(tb testing.TB, defs, call string) func() {
	tb.Helper()
	sem, diag, dl := Semantics(), Diagnostics(), Dialect()
	r := &interp.Runner{
		Stdout: io.Discard, Stderr: io.Discard,
		Semantics: &sem, Diagnostics: &diag, Dialect: &dl,
		Dir: tb.TempDir(), Name: "ksh",
	}
	Apply(r)
	prog, err := syntax.Parse(defs, Dialect())
	if err != nil {
		tb.Fatal(err)
	}
	if _, err := r.Run(context.Background(), prog); err != nil {
		tb.Fatal(err)
	}
	c, err := syntax.Parse(call, Dialect())
	if err != nil {
		tb.Fatal(err)
	}
	return func() {
		if _, err := r.Run(context.Background(), c); err != nil {
			tb.Fatal(err)
		}
	}
}

// localCostAllocs is what one call to name allocates beyond one to base: the
// same body with its declarations written as plain assignments, so the
// difference is what declaring costs — the scope's records, the declarations
// and their unwind — and not what a call or an assignment costs.
func localCostAllocs(t *testing.T, defs, name, base string) float64 {
	t.Helper()
	call, plain := localCostRunner(t, defs, name+" x y"), localCostRunner(t, defs, base+" x y")
	call()
	plain()
	return testing.AllocsPerRun(200, call) - testing.AllocsPerRun(200, plain)
}

const localCostDefs = `function plain { c=$1$2; }
function declaring { typeset a=$1 b=$2 c; c=$a$b; }
function empty { :; }
posix() { :; }
`

// Three declarations in a `function`-word body cost 75 allocations a call
// over the same body with plain assignments, measured on main at 26b573707: a
// scope kept what each declaration displaced in a table per kind of record,
// a dozen maps a name, each made on the spot, and the unwind built a set of
// the names to put back (#6361). The tables are kept for the next call now —
// see interp/scopespares.go — so a declaration costs the writes it does.
//
// Allocations rather than time, so the row is deterministic. The bound has
// room above what a call costs now and is well below what it cost before.
func TestTypesetCostsFewAllocations(t *testing.T) {
	if got := localCostAllocs(t, localCostDefs, "declaring", "plain"); got > 50 {
		t.Errorf("three declarations allocate %v times a call more than plain assignments; want at most 50", got)
	}
}

// A `function`-word call takes the whole option table and puts it back at the
// return, so every such call paid for the listing: the names gathered and
// sorted, each resolved, three closures for each negated spelling, and a map
// to hold the states — 27 allocations more than a call of the other form
// on main at 26b573707. The listing is resolved once now and a call keeps a
// state per name (see interp/localsetoptions.go).
func TestAKeywordCallSavesTheOptionsCheaply(t *testing.T) {
	call, posix := localCostRunner(t, localCostDefs, "empty"), localCostRunner(t, localCostDefs, "posix")
	call()
	posix()
	if got := testing.AllocsPerRun(200, call) - testing.AllocsPerRun(200, posix); got > 8 {
		t.Errorf("a `function`-word call allocates %v times more than a POSIX-style one; want at most 8", got)
	}
}

// BenchmarkTypeset reports what one declaration allocates, averaged over the
// three in the body; the baseline on main at 26b573707 was 25 a declaration.
func BenchmarkTypeset(b *testing.B) {
	call := localCostRunner(b, localCostDefs, "declaring x y")
	plain := localCostRunner(b, localCostDefs, "plain x y")
	extra := testing.AllocsPerRun(100, call) - testing.AllocsPerRun(100, plain)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		call()
	}
	b.ReportMetric(extra/3, "allocs/decl")
}
