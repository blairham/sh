// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package startupcost_test

import (
	"bytes"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/startupcost"
)

// BenchmarkConstruct is #6353's table: each construct's loop under both of our
// dialects and both of the shells they claim to be, so `make startup` prints
// the comparison rather than a number to remember a reference for.
//
// The figure reported is the child's CPU, not its wall time — the table was
// read in CPU because wall on a shared machine is mostly the other tenants —
// and a run whose output is not the row's answer fails the benchmark rather
// than posting the best time in it.
func BenchmarkConstruct(b *testing.B) {
	subjects := all(b)
	for _, c := range startupcost.Constructs() {
		b.Run(c.Name, func(b *testing.B) {
			for _, s := range subjects {
				b.Run(s.Name, func(b *testing.B) {
					var cpu time.Duration
					n := 0
					for b.Loop() {
						took, out, err := startupcost.RunProgram(s, c.Program)
						if err != nil {
							b.Fatalf("%s: %v", s.Name, err)
						}
						if got := strings.TrimSpace(out); got != c.Answer {
							b.Fatalf("%s printed %q, want %q", s.Name, got, c.Answer)
						}
						cpu, n = cpu+took.CPU, n+1
					}
					if n > 0 {
						b.ReportMetric(float64(cpu.Microseconds())/1000/float64(n), "cpu-ms/op")
					}
				})
			}
		})
	}
}

func bashShell(out io.Writer) driver.Shell {
	return driver.Shell{
		Name:                   "bash",
		SystemStartupDirectory: "/etc",
		Dialect:                bash.Dialect(),
		Semantics:              bash.Semantics(),
		Diagnostics:            bash.Diagnostics(),
		Prelude:                bash.Prelude(),
		Register:               bash.Apply,
		PromptStyle:            bash.PromptStyle(),
		EditorStyle:            bash.EditorStyle(),
		HistoryStyle:           bash.HistoryStyle(),
		Stdout:                 out,
		Stderr:                 out,
	}
}

// BenchmarkConstructInProcess is the same table with no process in the way,
// which is what makes a row profilable and its allocations countable: a spawn
// is milliseconds of runtime and loader before any of this repository's code
// runs.
func BenchmarkConstructInProcess(b *testing.B) {
	for _, c := range startupcost.Constructs() {
		b.Run(c.Name, func(b *testing.B) {
			var out bytes.Buffer
			sh := bashShell(&out)
			b.ReportAllocs()
			for b.Loop() {
				out.Reset()
				if status := driver.RunCommand(sh, c.Program, nil); status != 0 {
					b.Fatalf("exited %d: %s", status, out.String())
				}
				if got := strings.TrimSpace(out.String()); got != c.Answer {
					b.Fatalf("printed %q, want %q", got, c.Answer)
				}
			}
		})
	}
}

// constructAllocBounds is what one iteration of a row may allocate, in objects
// and in bytes, beyond which the row has crept.
//
// A ceiling on allocations rather than on time, because allocations are a fact
// about the code and not about the machine: the same tree allocates the same
// on a laptop at load 20 and on an idle runner, so this can gate where a time
// cannot. Each is a little above what the row measured when #6353 brought it
// down, and the old figures are beside them — every one of those fails here.
var constructAllocBounds = map[string]struct{ objects, bytes float64 }{
	// `[ $i -lt N ]`: 27.5 objects an iteration before #6353.
	"test": {14, 400},
	// `[[ $i -lt N ]]`: 27.5 objects and 5.4 KB an iteration before, most
	// of it a parser built to read each operand.
	"dbracket": {8, 200},
	// `: $i`: 19.5 objects an iteration before.
	"builtin": {11, 250},
	// `x=$i`: 13.5 objects an iteration before.
	"assign": {7, 150},
}

// TestConstructAllocations holds the rows that #6353 brought down to what they
// allocate now, so the next creep fails a test rather than waiting for
// somebody to time it.
func TestConstructAllocations(t *testing.T) {
	if testing.Short() {
		t.Skip("runs each row's loop to completion")
	}
	checked := 0
	for _, c := range startupcost.Constructs() {
		bound, ok := constructAllocBounds[c.Name]
		if !ok {
			continue
		}
		checked++
		t.Run(c.Name, func(t *testing.T) {
			var out bytes.Buffer
			sh := bashShell(&out)
			run := func() {
				out.Reset()
				if status := driver.RunCommand(sh, c.Program, nil); status != 0 {
					t.Fatalf("exited %d: %s", status, out.String())
				}
				if got := strings.TrimSpace(out.String()); got != c.Answer {
					t.Fatalf("printed %q, want %q", got, c.Answer)
				}
			}
			run() // warm whatever is built once
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			run()
			runtime.ReadMemStats(&after)
			objects := float64(after.Mallocs-before.Mallocs) / startupcost.ConstructIterations
			byteCount := float64(after.TotalAlloc-before.TotalAlloc) / startupcost.ConstructIterations
			t.Logf("%.1f objects and %.0f bytes an iteration", objects, byteCount)
			if objects > bound.objects || byteCount > bound.bytes {
				t.Errorf("an iteration allocates %.1f objects and %.0f bytes, over the %.0f and %.0f this row was brought down to (#6353)",
					objects, byteCount, bound.objects, bound.bytes)
			}
		})
	}
	if checked != len(constructAllocBounds) {
		t.Fatalf("checked %d rows of %d bounded: a bound names a row that is not in the table", checked, len(constructAllocBounds))
	}
}
