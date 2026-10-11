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
// about 3,130 (3,240 under the race detector) (#6354). The bound sits under the
// middle figure, so either copy coming back fails here rather than in a
// benchmark nobody reads.
//
// Not parallel: it reads the process's allocation counters, and a top-level
// test that is not parallel finishes before any parallel one starts.
func TestParseBytesPerLine(t *testing.T) {
	const bound = 3450
	src := bigScript(300)
	lines := uint64(strings.Count(src, "\n"))
	best := uint64(1 << 62)
	for range 3 {
		var m0, m1 runtime.MemStats
		runtime.ReadMemStats(&m0)
		if _, err := Parse(src, Core()); err != nil {
			t.Fatal(err)
		}
		runtime.ReadMemStats(&m1)
		best = min(best, (m1.TotalAlloc-m0.TotalAlloc)/lines)
	}
	if best > bound {
		t.Errorf("a parse allocates %d bytes per line of bigScript, want at most %d", best, bound)
	}
}
