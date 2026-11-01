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
// Then 24 after it, on main at ee80b4336, and 11 now (#6373): an operand
// with a value copied its word twice over to expand a name that was a plain
// identifier, the list of operands that were names grew an append at a time,
// and the unwind made a list of the names to put back. A scope also keeps one
// record per name now rather than a table per kind of record — see
// interp/scoperecords.go — which is time rather than allocations.
//
// And 4 after that, on main at 7fd1b8456 (#6373): an operand written
// `name=$1` or `name=$v` is read as the one parameter it is rather than
// built into a word of its own and taken down the full road, the list of
// operands that are names is the front of the line's own until one is
// left out.
//
// Allocations rather than time, so the row is deterministic. The bound has
// room above what a call costs now and is well below what it cost before.
func TestTypesetCostsFewAllocations(t *testing.T) {
	if got := localCostAllocs(t, localCostDefs, "declaring", "plain"); got > 6 {
		t.Errorf("three declarations allocate %v times a call more than plain assignments; want at most 6", got)
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
// names the body declares; the baseline for three on main at 26b573707 was
// 25 a declaration. "five" is the shape a real-config start makes most
// (#6373): valueless names, five to a line.
func BenchmarkTypeset(b *testing.B) {
	defs := localCostDefs + `function five { typeset a b c d e; }
`
	for _, c := range []struct {
		name, call string
		names      float64
	}{
		{"three", "declaring", 3},
		{"five", "five", 5},
	} {
		b.Run(c.name, func(b *testing.B) {
			call := localCostRunner(b, defs, c.call+" x y")
			plain := localCostRunner(b, defs, "plain x y")
			extra := testing.AllocsPerRun(100, call) - testing.AllocsPerRun(100, plain)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				call()
			}
			b.ReportMetric(extra/c.names, "allocs/decl")
		})
	}
}
