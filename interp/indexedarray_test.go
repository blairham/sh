// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"maps"
	"math"
	"math/rand/v2"
	"runtime"
	"slices"
	"strconv"
	"testing"
	"time"
	"unsafe"
)

// arrayModel is what the store promised while it was a map, and what every
// method is checked against: the store's representation may change, and its
// answers may not.
type arrayModel map[int]string

func checkAgainstModel(t *testing.T, step string, a Array, m arrayModel) {
	t.Helper()
	if a.Len() != len(m) {
		t.Fatalf("%s: Len = %d, want %d", step, a.Len(), len(m))
	}
	want := slices.Sorted(maps.Keys(m))
	if got := a.subscripts(); !slices.Equal(got, want) {
		t.Fatalf("%s: subscripts = %v, want %v", step, got, want)
	}
	var walked []int
	for k, e := range a.All() {
		walked = append(walked, k)
		if e.Str != m[k] {
			t.Fatalf("%s: All yields %d=%q, want %q", step, k, e.Str, m[k])
		}
	}
	if !slices.Equal(walked, want) {
		t.Fatalf("%s: All walks %v, want %v in order", step, walked, want)
	}
	lo, hi, any := a.bounds()
	if any != (len(want) > 0) || any && (lo != want[0] || hi != want[len(want)-1]) {
		t.Fatalf("%s: bounds = %d %d %v, want the ends of %v", step, lo, hi, any, want)
	}
	for _, k := range []int{-3, -1, 0, 1, 2, 5, 63, 64, 65, 200, 1 << 20} {
		e, ok := a.Lookup(k)
		v, held := m[k]
		if ok != held || e.Str != v || a.Get(k).Str != v || a.Has(k) != held {
			t.Fatalf("%s: Lookup(%d) = %q %v, want %q %v", step, k, e.Str, ok, v, held)
		}
	}
}

// The shapes a script can make, each against the model: gaps, an unset in
// the middle and at the end, writes below zero and far past the end, and a
// gap filled back in.
func TestIndexedArrayAnswersAsTheMapDid(t *testing.T) {
	t.Parallel()
	type op struct {
		del bool
		pos int
	}
	cases := map[string][]op{
		"append":                 {{pos: 0}, {pos: 1}, {pos: 2}},
		"a gap, as a[5]=y":       {{pos: 0}, {pos: 5}},
		"the gap filled back in": {{pos: 0}, {pos: 3}, {pos: 1}, {pos: 2}},
		"unset in the middle":    {{pos: 0}, {pos: 1}, {pos: 2}, {del: true, pos: 1}},
		"unset the last":         {{pos: 0}, {pos: 1}, {pos: 2}, {del: true, pos: 2}},
		"unset the last over a hole": {
			{pos: 0},
			{pos: 1},
			{pos: 2},
			{pos: 3},
			{del: true, pos: 2},
			{del: true, pos: 1},
			{del: true, pos: 3},
		},
		"unset everything":          {{pos: 0}, {pos: 1}, {del: true, pos: 0}, {del: true, pos: 1}},
		"unset the first":           {{pos: 0}, {pos: 1}, {pos: 2}, {del: true, pos: 0}},
		"append after an unset end": {{pos: 0}, {pos: 1}, {del: true, pos: 1}, {pos: 1}},
		"a word boundary":           {{pos: 0}, {pos: 63}, {pos: 64}, {del: true, pos: 64}, {pos: 65}},
		"far past the end":          {{pos: 0}, {pos: 1 << 20}, {pos: 1}},
		"below zero":                {{pos: 0}, {pos: -1}, {pos: 1}, {del: true, pos: -1}},
		"first write far out":       {{pos: 200}, {pos: 0}},
		"unset what is not there":   {{pos: 0}, {del: true, pos: 7}, {del: true, pos: -2}},
	}
	for name, ops := range cases {
		t.Run(name, func(t *testing.T) {
			a, m := NewArray(0), arrayModel{}
			for i, o := range ops {
				if o.del {
					a.Delete(o.pos)
					delete(m, o.pos)
				} else {
					v := "v" + strconv.Itoa(i)
					a.Set(o.pos, Scalar(v))
					m[o.pos] = v
				}
				checkAgainstModel(t, name+" step "+strconv.Itoa(i), a, m)
				checkAgainstModel(t, name+" clone "+strconv.Itoa(i), a.clone(), m)
				checkAgainstModel(t, name+" shallow "+strconv.Itoa(i), a.shallowClone(), m)
			}
		})
	}
}

// The same promise over random sequences, which reach orderings no list of
// cases thought of: a hole opened and closed across a word, a trim that
// takes several holes with it, a switch to the map in the middle.
func TestIndexedArrayRandomOperationsAnswerAsTheMapDid(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(6099, 1))
	for round := range 300 {
		a, m := NewArray(0), arrayModel{}
		far := round%5 == 0
		for i := range 200 {
			pos := rng.IntN(150)
			if far && rng.IntN(40) == 0 {
				pos = rng.IntN(1<<16) - 10
			}
			if rng.IntN(3) == 0 {
				a.Delete(pos)
				delete(m, pos)
			} else {
				v := strconv.Itoa(i)
				a.Set(pos, Scalar(v))
				m[pos] = v
			}
		}
		checkAgainstModel(t, "round "+strconv.Itoa(round), a, m)
	}
}

// A nil array reads as one with nothing in it, the way a nil map did.
func TestANilIndexedArrayReadsAsEmpty(t *testing.T) {
	t.Parallel()
	var a Array
	if a.Len() != 0 || a.Has(0) || len(a.subscripts()) != 0 {
		t.Fatal("a nil array holds something")
	}
	for range a.All() {
		t.Fatal("a nil array walked an element")
	}
	if _, _, any := a.bounds(); any {
		t.Fatal("a nil array has bounds")
	}
	if a.clone() != nil || a.shallowClone() != nil {
		t.Fatal("a nil array cloned to an array")
	}
	a.Delete(3)
}

// Deleting during a walk is allowed and the deleted position is not visited,
// in both forms of the store.
func TestIndexedArrayWalkHonorsADeleteMadeDuringIt(t *testing.T) {
	t.Parallel()
	for _, far := range []bool{false, true} {
		a := ArrayOf(Scalar("a"), Scalar("b"), Scalar("c"), Scalar("d"))
		if far {
			a.Set(1<<20, Scalar("z"))
		}
		var seen []int
		for k := range a.All() {
			seen = append(seen, k)
			if k == 0 {
				a.Delete(2)
			}
		}
		want := []int{0, 1, 3}
		if far {
			want = append(want, 1<<20)
		}
		if !slices.Equal(seen, want) {
			t.Fatalf("far=%v: walked %v, want %v", far, seen, want)
		}
	}
}

// A write during a walk that moves the store into the map does not lose the
// positions the walk had not reached yet.
func TestIndexedArrayWalkSurvivesTheSwitchToTheMap(t *testing.T) {
	t.Parallel()
	a := ArrayOf(Scalar("a"), Scalar("b"), Scalar("c"))
	var seen []int
	for k := range a.All() {
		seen = append(seen, k)
		if k == 0 {
			a.Set(-5, Scalar("neg"))
		}
	}
	if !slices.Equal(seen, []int{0, 1, 2}) {
		t.Fatalf("walked %v, want 0 1 2", seen)
	}
}

// appendN appends n elements the way `a+=(x)` does: at the position past the
// end, which is asked of the array every time.
func appendN(n int) Array {
	a := NewArray(0)
	for range n {
		a.Set(a.pastTheEnd(), Scalar("x"))
	}
	return a
}

// Appending N elements is O(N) (#6099), which is two promises: an append does
// the same work whatever the array's length, and the store grows by a factor,
// so what it copies as it grows comes to a constant per element. The map before
// #6099 broke the first, since every append searched it for its end; a store
// that copied itself on every append, or grew by a fixed step, would break the
// second. Each half is checked by the instrument that can see it.
func TestAppendingToAnIndexedArrayIsLinear(t *testing.T) {
	t.Run("an append costs the same at any length", checkAnAppendCostsTheSameAtAnyLength)
	t.Run("the store grows by a factor", checkTheStoreGrowsByAFactor)
}

// The timed half's sizes and bound. appendBound sits between about 1, an append
// that does a fixed amount of work, and the 200 and more an append that walks
// the array comes to at appendLong.
const (
	appendShort          = 16
	appendLong           = 1 << 14
	appendBatch          = 64
	appendSamples        = 32
	appendControlSamples = 8
	appendBound          = 8
)

// What an append costs on an array of appendShort elements and on one of
// appendLong: the least of appendSamples batches of appendBatch appends to each,
// the two sizes taking turns, with each store given room for a batch beforehand
// so that no timed batch grows it. An append that does a fixed amount of work
// takes as long on either; one that walks the array, the way the map's search
// for its end did, takes over 200 times as long on the longer.
//
// This used to time the whole build instead, 100,000 appends against 400,000,
// the best of five, under a bound of 10 on the ratio, and the macOS runner failed
// it at 10.8 and 12.8 with nothing wrong (#6175, #7020). Four things close the
// routes by which load reached that ratio:
//
//   - A batch is microseconds, far shorter than a scheduler's time slice. On a
//     contended machine a 5 ms run is often cut by one and a 1 ms run rarely,
//     so it was the larger run that paid. Here both sizes are timed over equally
//     short batches, equally likely to be interrupted.
//   - The two sizes take turns, so a stretch of load (another package's tests
//     starting, a clock slowing down) lands on both. Timed one size after the
//     other, it landed on whichever was being timed.
//   - A batch writes only memory the untimed batch has already written, so no
//     page is touched for the first time while the clock runs. The old runs
//     built their stores from nothing, and the larger built four times as much.
//   - The least batch is kept on each side, and load only ever adds time. To
//     fail, every batch on the longer array has to be slowed past the bound
//     while one batch on the shorter is not.
//
// Measured on an 18-core M-series Mac with 36 busy loops running and the test
// held to the efficiency cores (GOMAXPROCS=2, taskpolicy -b), the two taking
// turns 50 runs at a time: the old form failed 21 of 200, at ratios from 10.2 to
// 215, and this one none of 200, its highest ratio 1.2 and its control's lowest
// 239. Without taskpolicy neither failed in 200.
//
// **The control is an append that walks the array first**, the shape #6099
// removed, and the check fails unless the measurement sees it grow: one that
// cannot tell a walk from no walk passes everything. What the measurement cannot
// see is the cost of growing the store, which no timed batch pays. That is the
// other half, and it is counted in bytes.
func checkAnAppendCostsTheSameAtAnyLength(t *testing.T) {
	filled := func(n int) Array {
		return ArrayOf(slices.Repeat([]Element{Scalar("x")}, n)...)
	}
	short, long := filled(appendShort), filled(appendLong)
	// batch times appendBatch appends to a by add, then deletes them from the
	// end, so that a is its old length again with the room they took kept.
	batch := func(a Array, add func(Array)) time.Duration {
		n := a.Len()
		start := time.Now()
		for range appendBatch {
			add(a)
		}
		took := time.Since(start)
		for i := n + appendBatch - 1; i >= n; i-- {
			a.Delete(i)
		}
		if a.Len() != n || a.pastTheEnd() != n {
			t.Fatalf("after a batch on %d elements: %d held, past the end %d", n, a.Len(), a.pastTheEnd())
		}
		return took
	}
	// least is the least of samples batches on each array, the two taking turns.
	least := func(samples int, add func(Array)) (onShort, onLong time.Duration, ratio float64) {
		onShort, onLong = math.MaxInt64, math.MaxInt64
		for range samples {
			onShort = min(onShort, batch(short, add))
			onLong = min(onLong, batch(long, add))
		}
		return onShort, onLong, float64(onLong) / float64(max(onShort, 1))
	}
	appendOne := func(a Array) { a.Set(a.pastTheEnd(), Scalar("x")) }
	walkThenAppend := func(a Array) {
		end := 0
		for k := range a.All() {
			end = max(end, k+1)
		}
		a.Set(end, Scalar("x"))
	}
	batch(short, appendOne) // untimed: the room for a batch
	batch(long, appendOne)
	runtime.GC()
	s, l, ratio := least(appendControlSamples, walkThenAppend)
	t.Logf("control, walking first: %v on %d elements, %v on %d, ratio %.2f", s, appendShort, l, appendLong, ratio)
	if ratio <= appendBound {
		t.Fatalf("the control, %d appends that each walk the array first, took %v on %d elements and %v on %d: "+
			"ratio %.2f, so the measurement cannot see a walk", appendBatch, s, appendShort, l, appendLong, ratio)
	}
	s, l, ratio = least(appendSamples, appendOne)
	t.Logf("%d appends: %v on %d elements, %v on %d, ratio %.2f", appendBatch, s, appendShort, l, appendLong, ratio)
	if ratio > appendBound {
		t.Fatalf("%d appends took %v on an array of %d elements and %v on one of %d: ratio %.2f, "+
			"want about 1 (an append does not walk the array)", appendBatch, s, appendShort, l, appendLong, ratio)
	}
}

// What the store allocates while 4096 elements are appended to an empty array,
// counted in elements: a store that grows by a factor allocates a few elements'
// worth for each one appended, however long the array gets (Go's growth makes
// it under six), where one that copied itself on every append allocates half
// the array's length for each, 2048 here, and one that grew by a fixed step of
// s allocates 2048/s. Bytes rather than time, because the figure is the same on
// every run on every machine and there is nothing in it for a busy runner to
// move. The least of three is kept because another goroutine's allocation can
// add to the figure and nothing can take from it.
func checkTheStoreGrowsByAFactor(t *testing.T) {
	const n, bound = 4096, 16
	least := uint64(math.MaxUint64)
	var before, after runtime.MemStats
	for range 3 {
		runtime.ReadMemStats(&before)
		a := appendN(n)
		runtime.ReadMemStats(&after)
		if a.Len() != n {
			t.Fatalf("appended %d, have %d", n, a.Len())
		}
		least = min(least, after.TotalAlloc-before.TotalAlloc)
	}
	perElement := float64(least) / n / float64(unsafe.Sizeof(Element{}))
	t.Logf("appending %d allocated %d bytes, %.2f elements' worth per element", n, least, perElement)
	if perElement > bound {
		t.Fatalf("appending %d elements allocated %d bytes, %.1f elements' worth for each one appended: "+
			"want a few at most (the store grows by a factor)", n, least, perElement)
	}
}

func BenchmarkIndexedArrayAppend(b *testing.B) {
	for b.Loop() {
		appendN(10_000)
	}
}

func BenchmarkIndexedArrayRead(b *testing.B) {
	a := appendN(3000)
	var e Element
	for b.Loop() {
		e = a.Get(5)
	}
	_ = e
}

func BenchmarkIndexedArrayWalk(b *testing.B) {
	a := appendN(3000)
	n := 0
	for b.Loop() {
		for range a.All() {
			n++
		}
	}
}

// A short gap stays in the slice, so `a[10]=x` on a small zsh array does not
// cost the map; a write below zero or far out does move it there.
func TestAShortGapStaysInTheSlice(t *testing.T) {
	t.Parallel()
	a := ArrayOf(Scalar("x"))
	a.Set(10, Scalar("y"))
	if a.sparse != nil {
		t.Fatal("a gap of nine moved the array into the map")
	}
	a.Set(-1, Scalar("z"))
	if a.sparse == nil {
		t.Fatal("a write below zero stayed in the slice")
	}
	b := ArrayOf(Scalar("x"))
	b.Set(1<<20, Scalar("y"))
	if b.sparse == nil {
		t.Fatal("a write a million past the end stayed in the slice")
	}
}

// A clone reaches a nested array too, in the slice's fast path as in the
// map's: a write through the copy's nested element must not reach the
// original's.
func TestACloneCopiesNestedElementsInBothForms(t *testing.T) {
	t.Parallel()
	for _, sparse := range []bool{false, true} {
		a := ArrayOf(Scalar("x"), Element{Nested: ArrayOf(Scalar("p"), Scalar("q"))})
		if sparse {
			a.Set(-1, Scalar("neg"))
		}
		b := a.clone()
		b.Get(1).Nested.Set(0, Scalar("Z"))
		if got := a.Get(1).Nested.Get(0).Str; got != "p" {
			t.Fatalf("sparse=%v: the original's nested element became %q", sparse, got)
		}
	}
}
