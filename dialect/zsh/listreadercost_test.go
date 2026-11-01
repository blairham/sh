// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"runtime"
	"testing"

	"github.com/blairham/sh/internal/testenv"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// listReaderElems is how long the lists in the rows below are: long enough
// that what a reader pays per element dwarfs what it pays per call.
const listReaderElems = 4000

// listReaderRunner is a zsh runner holding an array `y` of listReaderElems
// short words and a table `h` of half as many pairs, and a parsed prog.
func listReaderRunner(tb testing.TB, prog string) (*interp.Runner, func()) {
	tb.Helper()
	r := optionStateRunner(tb)
	setup, err := syntax.Parse(`y=({1..4000}); typeset -A h; for i in {1..2000}; do h[k$i]=$i; done`, Dialect())
	if err != nil {
		tb.Fatal(err)
	}
	if _, err := r.Run(context.Background(), setup); err != nil {
		tb.Fatal(err)
	}
	f, err := syntax.Parse(prog, Dialect())
	if err != nil {
		tb.Fatal(err)
	}
	return r, func() {
		if _, err := r.Run(context.Background(), f); err != nil {
			tb.Fatal(err)
		}
	}
}

// bytesPerElement is what one run of prog allocates, per element of the list
// it reads, less what a run of base allocates — so a row measures the reader
// and not the statement around it. Bytes rather than time, so the row is
// deterministic.
func bytesPerElement(t *testing.T, prog, base string) float64 {
	t.Helper()
	if testenv.RaceDetector {
		// Measured on the Linux race leg: 121, 102 and 217 bytes an element
		// for rows that measure 103, 84 and 176 without the detector. The
		// bounds are for the build that ships; the leg without -race holds
		// them.
		t.Skip("allocation counts under -race include the detector's own")
	}
	measure := func(p string) float64 {
		_, run := listReaderRunner(t, p)
		run()
		const runs = 20
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		for range runs {
			run()
		}
		runtime.ReadMemStats(&after)
		return float64(after.TotalAlloc-before.TotalAlloc) / runs
	}
	return (measure(prog) - measure(base)) / listReaderElems
}

// An array literal built from a long expansion stored each element and, beside
// it, an entry in a map of the positions it had written — read only when the
// name carries an attribute that folds what is written to it, and built
// whether or not it did — into a store that grew a doubling at a time.
// `x=("${(@kv)h}")` over 30,000 pairs spent a third of its time on the two,
// and was 4.4x zsh 5.9.2 (#6362).
//
// The two came to about 150 bytes an element: these rows measured 284, 293
// and 337 before, and 137, 146 and 190 once the store was sized up front and
// the map kept only for a name that folds (#6368). The fields the expansion
// hands over were then built into slices that regrew a doubling at a time:
// 129, 138 and 182 before those were sized from the list, with other work
// landed in between, and 84, 92 and 136 after. Each bound sits between the
// last two.
func TestAnArrayLiteralPaysOnlyForItsElements(t *testing.T) {
	for _, row := range []struct {
		prog, base string
		most       float64
	}{
		{`x=("${y[@]}")`, `x=()`, 120},
		{`x=("${(@kv)h}")`, `x=()`, 130},
		{`x=(${(s:,:)${(j:,:)y}})`, `x=(); : ${(j:,:)y}`, 170},
	} {
		got := bytesPerElement(t, row.prog, row.base)
		t.Logf("%s: %.0f bytes per element", row.prog, got)
		if got > row.most {
			t.Errorf("%s: %.0f bytes per element; want at most %.0f", row.prog, got, row.most)
		}
		if got < 40 {
			// The control: an element of the store is forty bytes, so a row
			// below that is not measuring the literal at all, and would pass
			// whatever the literal did.
			t.Errorf(
				"%s: %.0f bytes per element — less than the store's own element, so the row is not measuring the literal",
				row.prog,
				got,
			)
		}
	}
}

// An unquoted list reached the command line through slices regrown per
// element, and `print` copied every argument through a builder looking for
// escapes it did not hold. `print -l -- $a` over 50,000 elements was 2.5x zsh
// 5.9.2 (#6362).
//
// Measured 203, 176 and 198 bytes an element before, and 103, 84 and 152
// after; the bounds sit between.
func TestAListReaderPaysOnlyForItsElements(t *testing.T) {
	for _, row := range []struct {
		prog, base string
		most       float64
	}{
		{`print -l -- $y`, `print -l --`, 150},
		{`x=($y)`, `x=()`, 130},
		{`for k v in "${(@kv)h}"; do :; done`, `for k v in; do :; done`, 175},
	} {
		got := bytesPerElement(t, row.prog, row.base)
		t.Logf("%s: %.0f bytes per element", row.prog, got)
		if got > row.most {
			t.Errorf("%s: %.0f bytes per element; want at most %.0f", row.prog, got, row.most)
		}
	}
}

// What the map was for — an attribute that folds what is written — is pinned
// in tests/bash/elementfold_test.go and tests/ksh/elementfold_test.go,
// the two dialects whose literals go through one; zsh's do not.

func BenchmarkKeyValueLiteral(b *testing.B) {
	_, run := listReaderRunner(b, `x=("${(@kv)h}")`)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		run()
	}
}
