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
	// `[ $i -lt N ]`: 27.5 objects an iteration before #6353, and 5.5
	// before an unquoted `$i` stopped taking the full expansion road.
	"test": {4.5, 180},
	// `[[ $i -lt N ]]`: 27.5 objects and 5.4 KB an iteration before, most
	// of it a parser built to read each operand, then 4.5.
	"dbracket": {4, 120},
	// `i=$((i+1))`: 4.5 objects and 330 bytes an iteration while every
	// arithmetic expansion copied the stack of substitution levels and
	// built a closure to put it back.
	"arith-dollar": {3, 160},
	// `x=$((i*2+1))`: 6.5 objects and 338 bytes, for the same reason.
	"arith-expand": {5, 180},
	// `: $i`: 19.5 objects an iteration before, then 5.5.
	"builtin": {4.5, 120},
	// `x=$i`: 13.5 objects an iteration before, then 3.5.
	"assign": {3, 80},
	// `f` with `f() { :; }`: 21.5 objects an iteration before, then 8.5
	// while every call registered a closure to put back a getopts cursor
	// the dialect shares, then 5.4 objects and 319 bytes while every call
	// allocated its scope (#6373).
	"function": {5, 150},
	// `x=${s#*/}`: 6.3 objects and 88 bytes an iteration while every trim
	// bound a method value, let its refusal flag escape and copied a
	// one-span word through a strings.Builder twice, then 2.3 objects — 48
	// bytes on macOS and 72 on the Linux runner.
	"trim": {3, 80},
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
				t.Errorf("an iteration allocates %.1f objects and %.0f bytes, over the %.1f and %.0f this row was brought down to (#6353)",
					objects, byteCount, bound.objects, bound.bytes)
			}
		})
	}
	if checked != len(constructAllocBounds) {
		t.Fatalf("checked %d rows of %d bounded: a bound names a row that is not in the table", checked, len(constructAllocBounds))
	}
}
