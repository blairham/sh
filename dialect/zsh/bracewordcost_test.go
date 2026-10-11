// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"io"
	"runtime"
	"strconv"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// braceWordCost runs `for i in {1..n}` with a probe as the loop's body and
// reports two figures per word the range made: the bytes allocated over the
// whole run, and the bytes still live when the body first runs — the list the
// loop walks.
//
// The allocation is what #6359 is about. `{1..200000}` peaked at 80 MB at
// GOGC=100, about 350 bytes a six-character word, because every element was
// built as a span in a word and then expanded through the whole field
// pipeline, all before the first was used. That was 530 bytes allocated a
// word, and the words were held until the last was built; the list itself,
// measured in the body once the rest is garbage, was always about 30.
func braceWordCost(tb testing.TB, n int) (live, allocated float64) {
	tb.Helper()
	sem, diag, dl := zsh.Semantics(), zsh.Diagnostics(), zsh.Dialect()
	r := &interp.Runner{
		Stdout: io.Discard, Stderr: io.Discard,
		Semantics: &sem, Diagnostics: &diag, Dialect: &dl,
		Dir: tb.TempDir(), Name: "zsh",
	}
	zsh.Apply(r)
	var inBody uint64
	r.Register("probe", func(*interp.Runner, context.Context, []string) int {
		var m runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m)
		inBody = m.HeapAlloc
		return 0
	})
	f, err := syntax.Parse("for i in {1.."+strconv.Itoa(n)+"}; do probe; break; done", dl)
	if err != nil {
		tb.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if _, err := r.Run(context.Background(), f); err != nil {
		tb.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if inBody == 0 {
		tb.Fatal("the loop's body never ran")
	}
	live = (float64(inBody) - float64(before.HeapAlloc)) / float64(n)
	allocated = float64(after.TotalAlloc-before.TotalAlloc) / float64(n)
	return live, allocated
}

// A word a range made in plain text is built as the string it is and
// nothing else: the element, the list that holds it and the loop's copy of
// that list come to about 110 bytes a word. The bound leaves room above that
// and is far below the 530 the word-and-span road cost.
func TestABraceRangeIsBuiltAsStrings(t *testing.T) {
	if _, allocated := braceWordCost(t, 100000); allocated > 250 {
		t.Errorf("for i in {1..100000}: %.0f bytes allocated per word; want at most 250", allocated)
	}
}

func BenchmarkBraceRangeForWords(b *testing.B) {
	const n = 100000
	var live, allocated float64
	for range b.N {
		live, allocated = braceWordCost(b, n)
	}
	b.ReportMetric(live, "live-B/word")
	b.ReportMetric(allocated, "alloc-B/word")
}
