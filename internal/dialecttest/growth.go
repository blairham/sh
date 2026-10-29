// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialecttest

import (
	"context"
	"io"
	"runtime"
	"strconv"
	"testing"
)

// Growth sizes: an operation is run on an array of growthSmall elements and on
// one of growthLarge, and what one operation allocates is compared.
const (
	growthSmall = 2000
	growthLarge = 20000
	// growthOps is how many operations one sample runs, so that one cost
	// amortized over several of them — a slice that doubles under an append —
	// is spread thin rather than landing in one operation's figure.
	growthOps = 500
	// growthSamples is how many samples are taken at each size; the least is
	// kept, because another goroutine's allocation can only add to a figure.
	growthSamples = 3
	// growthBytesPerElement is the bound: an operation may allocate this many
	// more bytes per element the array gained. One that walks the array into
	// a list allocates at least a string header per element, 16 bytes, and
	// one that does not allocates nothing more; an append that reallocates
	// the slice once in growthOps is well under one.
	growthBytesPerElement = 1.0
)

// BytesPerOperation is what one run of op allocates, in bytes, on a runner
// that has already run setup: the least over several samples of growthOps
// runs each.
//
// Allocation rather than time (#6411). A ratio of two wall-clock times, each
// about a millisecond, was the earlier form of every array-cost test, and a
// shared runner's scheduling jitter is larger than that: it failed the macOS
// job three times on diffs that could not reach it. The bytes an operation
// allocates do not depend on the machine's load, and every route by which an
// array operation has grown with the array — reading the whole array into a
// list to take one element, to count it, or to index it (#6099, #6358) —
// allocated that list.
//
// What it cannot see is a walk that allocates nothing: a scan of the store for
// its end, which is how the map before #6099 found where an append lands. That
// cost is the store's own, and interp's TestAppendingToAnIndexedArrayIsLinear
// times it there, in batches of appends short enough that a busy runner cannot
// tell a short array from a long one (#7020). Put back the map and every read
// and count row here fails; the append row is the one left to that test.
//
// The test must not be parallel: the figure is the process's, so an
// allocation by a test running alongside lands in it. Taking the least sample
// is the defense against the rest, since nothing can make it smaller.
func (p Preset) BytesPerOperation(t testing.TB, setup, op string) float64 {
	t.Helper()
	r := p.Runner(Base{Stdout: io.Discard, Stderr: io.Discard, Dir: t.TempDir()})
	ctx := context.Background()
	run := func(src string) {
		t.Helper()
		if err := r.RunPart(ctx, p.ParseThrough(t, r, src)); err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		if r.Exited() || r.ExitStatus() != 0 {
			t.Fatalf("%q: exited %v, status %d", src, r.Exited(), r.ExitStatus())
		}
	}
	run(setup)
	loop := `for ((i=0;i<` + strconv.Itoa(growthOps) + `;i++)); do ` + op + `; done`
	f := p.ParseThrough(t, r, loop)
	best := 0.0
	var before, after runtime.MemStats
	for i := range growthSamples + 1 {
		runtime.ReadMemStats(&before)
		if err := r.RunPart(ctx, f); err != nil {
			t.Fatalf("run %q: %v", loop, err)
		}
		runtime.ReadMemStats(&after)
		if r.Exited() || r.ExitStatus() != 0 {
			t.Fatalf("%q: exited %v, status %d", loop, r.Exited(), r.ExitStatus())
		}
		// The first run is not counted: it fills whatever the runner caches
		// on first sight of a command, which is the same at either size.
		if i == 0 {
			continue
		}
		v := float64(after.TotalAlloc-before.TotalAlloc) / growthOps
		if i == 1 || v < best {
			best = v
		}
	}
	return best
}

// ArrayOperationGrowth is how many more bytes one run of op allocates for
// each element the array holds beyond growthSmall, where build(n) is the
// source that makes an array of n elements. Near zero for an operation that
// does not depend on the array's length, and 16 or more for one that reads
// the array into a list.
func (p Preset) ArrayOperationGrowth(
	t testing.TB,
	build func(n int) string,
	op string,
) (perElement, small, large float64) {
	t.Helper()
	small = p.BytesPerOperation(t, build(growthSmall), op)
	large = p.BytesPerOperation(t, build(growthLarge), op)
	return (large - small) / (growthLarge - growthSmall), small, large
}

// CheckArrayOperationsDoNotGrow fails each op whose allocation grows with the
// array build makes, and fails the whole check if control — an operation that
// does read the whole array — is not seen to grow, because a measurement that
// cannot tell a walk from no walk passes everything.
func (p Preset) CheckArrayOperationsDoNotGrow(t *testing.T, build func(n int) string, control string, ops ...string) {
	t.Helper()
	if per, small, large := p.ArrayOperationGrowth(t, build, control); per < growthBytesPerElement {
		t.Fatalf("the control %s allocated %.0f bytes on %d elements and %.0f on %d, %.2f per element: "+
			"the measurement does not see a walk", control, small, growthSmall, large, growthLarge, per)
	}
	for _, op := range ops {
		t.Run(op, func(t *testing.T) {
			if per, small, large := p.ArrayOperationGrowth(t, build, op); per >= growthBytesPerElement {
				t.Errorf("%s allocated %.0f bytes on %d elements and %.0f on %d: %.2f more per element, want about 0",
					op, small, growthSmall, large, growthLarge, per)
			}
		})
	}
}
