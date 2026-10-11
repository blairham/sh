// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// bigScript is the text #6354 measured: n records of an assignment, an `if [ ]`
// test holding an arithmetic expansion, and a command of two parameter
// expansions — three lines a record, nothing that forks.
func bigScript(n int) string {
	var b strings.Builder
	b.WriteString("n=0\n")
	for i := range n {
		fmt.Fprintf(&b, "a%d=\"value %d\"\nif [ \"$a%d\" = \"value %d\" ]; then n=$((n+1)); fi\n: \"${a%d#value}\" \"${a%d%%[0-9]*}\"\n", i, i, i, i, i, i)
	}
	b.WriteString("echo $n\n")
	return b.String()
}

// BenchmarkParseBig parses a fixed large script and reports what parsing costs
// per source line — time, bytes allocated and allocations — so a change to the
// lexer or the parser has a number to move (#6354).
func BenchmarkParseBig(b *testing.B) {
	src := bigScript(1000)
	lines := float64(strings.Count(src, "\n"))
	for _, d := range []struct {
		name string
		d    Dialect
	}{{"core", Core()}, {"posix", POSIX()}} {
		b.Run(d.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(src)))
			var m0, m1 runtime.MemStats
			runtime.ReadMemStats(&m0)
			for b.Loop() {
				if _, err := Parse(src, d.d); err != nil {
					b.Fatal(err)
				}
			}
			runtime.ReadMemStats(&m1)
			n := float64(b.N)
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/n/lines, "ns/line")
			b.ReportMetric(float64(m1.TotalAlloc-m0.TotalAlloc)/n/lines, "B/line")
			b.ReportMetric(float64(m1.Mallocs-m0.Mallocs)/n/lines, "allocs/line")
		})
	}
}

// What a parse may allocate per line of bigScript, with room to spare above
// what it does. Every unquoted operand used to cost a fresh Lexer — a kilobyte
// to allocate and clear — and a pattern operand two, one of them only to ask
// whether a quote in it was left open. That put this script at about 4,360
// bytes a line; dropping the second brought it to about 3,600 and the first to
// about 3,130 (3,240 under the race detector) (#6354). An operand's lexer
// then still carried a copy of the whole dialect; sharing it brought the
// script to about 2,940 (3,050 under the race detector) (#6381). The bound
// sits under the figure before each of those, so any of the copies coming back
// fails here rather than in a benchmark nobody reads.
//
// Not parallel: it reads the process's allocation counters, and a top-level
// test that is not parallel finishes before any parallel one starts.
func TestParseBytesPerLine(t *testing.T) {
	const bound = 3100
	bytes, _ := parseCostPerLine(t)
	t.Logf("%d bytes per line", bytes)
	if bytes > bound {
		t.Errorf("a parse allocates %d bytes per line of bigScript, want at most %d", bytes, bound)
	}
}

// How many allocations a parse may make per line of bigScript, with room to
// spare above what it does. Every reserved-word question the parser asked of
// a word of several spans — `n=$((n+1))` is one — used to join the spans into
// a fresh string first, once per question, and so did the check of a
// command's first word against the declaration utilities. That came to about
// 9.7 of the script's 47.9 allocations a line; comparing in place brought it
// to about 38.1. A run of literal text was then still copied out of the
// source through a growing buffer, two allocations a run where a substring
// of the source is none; taking the substring brought it to about 33.6
// (#6381). The bound sits under the figure before each of those.
//
// Not parallel, for the reason TestParseBytesPerLine gives.
func TestParseAllocsPerLine(t *testing.T) {
	const bound = 36
	_, allocs := parseCostPerLine(t)
	t.Logf("%.2f allocations per line", allocs)
	if allocs > bound {
		t.Errorf("a parse makes %.2f allocations per line of bigScript, want at most %d", allocs, bound)
	}
}

// parseCostPerLine parses bigScript three times and returns the fewest bytes
// and allocations any one parse made per line.
func parseCostPerLine(t *testing.T) (bytes uint64, allocs float64) {
	t.Helper()
	src := bigScript(300)
	lines := uint64(strings.Count(src, "\n"))
	bytes, allocs = uint64(1<<62), 1<<62
	for range 3 {
		var m0, m1 runtime.MemStats
		runtime.ReadMemStats(&m0)
		if _, err := Parse(src, Core()); err != nil {
			t.Fatal(err)
		}
		runtime.ReadMemStats(&m1)
		bytes = min(bytes, (m1.TotalAlloc-m0.TotalAlloc)/lines)
		allocs = min(allocs, float64(m1.Mallocs-m0.Mallocs)/float64(lines))
	}
	return bytes, allocs
}
