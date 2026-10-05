// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"maps"
	"math/rand/v2"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"testing"
	"time"
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
	type op struct {
		del bool
		pos int
	}
	cases := map[string][]op{
		"append":                     {{pos: 0}, {pos: 1}, {pos: 2}},
		"a gap, as a[5]=y":           {{pos: 0}, {pos: 5}},
		"the gap filled back in":     {{pos: 0}, {pos: 3}, {pos: 1}, {pos: 2}},
		"unset in the middle":        {{pos: 0}, {pos: 1}, {pos: 2}, {del: true, pos: 1}},
		"unset the last":             {{pos: 0}, {pos: 1}, {pos: 2}, {del: true, pos: 2}},
		"unset the last over a hole": {{pos: 0}, {pos: 1}, {pos: 2}, {pos: 3}, {del: true, pos: 2}, {del: true, pos: 1}, {del: true, pos: 3}},
		"unset everything":           {{pos: 0}, {pos: 1}, {del: true, pos: 0}, {del: true, pos: 1}},
		"unset the first":            {{pos: 0}, {pos: 1}, {pos: 2}, {del: true, pos: 0}},
		"append after an unset end":  {{pos: 0}, {pos: 1}, {del: true, pos: 1}, {pos: 1}},
		"a word boundary":            {{pos: 0}, {pos: 63}, {pos: 64}, {del: true, pos: 64}, {pos: 65}},
		"far past the end":           {{pos: 0}, {pos: 1 << 20}, {pos: 1}},
		"below zero":                 {{pos: 0}, {pos: -1}, {pos: 1}, {del: true, pos: -1}},
		"first write far out":        {{pos: 200}, {pos: 0}},
		"unset what is not there":    {{pos: 0}, {del: true, pos: 7}, {del: true, pos: -2}},
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

// Appending is O(1) amortized, so appending N elements is O(N): twice the
// elements may cost about twice the time, and not four times, which is what
// the map cost when each append searched it for its end (#6099).
//
// A ratio of two sizes rather than a number, so the machine's speed and load
// cancel; the best of several runs, so one descheduled run does not decide
// it. Four times the elements rather than twice: linear is a ratio near 4 and
// quadratic one near 16, and the bound at 10 sits between them with room a busy
// runner cannot close — at twice the elements linear is 2 and quadratic 4, and
// cache and allocator noise alone took the macOS runner past 3 (#6175).
//
// **With the collector held off while a run is timed.** The larger run builds
// twice the live heap, and a collection during it scans all of it, so on a
// loaded runner the collector's share alone pushed the ratio past 3 — 3.42,
// 3.39 and 5.26 on the macOS runner in one afternoon, each passing on rerun
// (#6175). What the test is about is the appends, so they are what is timed.
func TestAppendingToAnIndexedArrayIsLinear(t *testing.T) {
	if testing.Short() {
		t.Skip("times two runs")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	const n = 100_000
	best := func(n int) time.Duration {
		b := time.Duration(1<<63 - 1)
		for range 5 {
			runtime.GC()
			start := time.Now()
			if a := appendN(n); a.Len() != n {
				t.Fatalf("appended %d, have %d", n, a.Len())
			}
			b = min(b, time.Since(start))
		}
		return b
	}
	small, large := best(n), best(4*n)
	if ratio := float64(large) / float64(small); ratio > 10 {
		t.Fatalf("appending %d took %v and %d took %v: ratio %.2f, want about 4 (linear)",
			n, small, 4*n, large, ratio)
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
