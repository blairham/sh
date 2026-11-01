// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/blairham/sh/syntax"
)

// cacheTestRunner is a bare runner with a dialect of its own, which is all
// the span reader asks of one.
func cacheTestRunner() *Runner {
	d := syntax.Core()
	sem := PosixSemantics()
	// testrunner:bare — the reader is the subject; nothing here runs a
	// command, opens a file or reaches a directory.
	return &Runner{Semantics: &sem, Dialect: &d}
}

func readSpansBothWays(t *testing.T, r *Runner, text string) (cached, fresh []syntax.Span) {
	t.Helper()
	cached, _, ok := r.arithSpans(text)
	if !ok {
		t.Fatalf("%q: the cached read refused", text)
	}
	defer func(prev bool) { arithSpanCacheDisabled = prev }(arithSpanCacheDisabled)
	arithSpanCacheDisabled = true
	fresh, _, ok = r.arithSpans(text)
	if !ok {
		t.Fatalf("%q: the fresh read refused", text)
	}
	return cached, fresh
}

// A hit is the read it stands for: the same spans, and the same parameter
// expansions in them, as reading the text again would give — asked twice,
// so that the second answer is the one the cache held.
func TestACachedSpanReadIsTheReadItStandsFor(t *testing.T) {
	r := cacheTestRunner()
	for _, text := range []string{
		" ${+seen[$line]} ",
		" ${#x} + $y * ${z:-3} ",
		" a[$i] = ${b[$((j+1))]} ",
		" $(echo 1) + `echo 2` ",
		" x ",
	} {
		r.arithSpans(text)
		cached, fresh := readSpansBothWays(t, r, text)
		if !reflect.DeepEqual(cached, fresh) {
			t.Errorf("%q: the cache answered %#v, a fresh read %#v", text, cached, fresh)
		}
	}
}

// A hit hands back a copy: what a caller does to the slice it was given is
// not what the next caller is given.
func TestACachedSpanReadIsACopy(t *testing.T) {
	t.Parallel()
	r := cacheTestRunner()
	const text = " $a + $b "
	r.arithSpans(text)
	first, _, _ := r.arithSpans(text)
	first[0] = syntax.Span{Kind: syntax.Literal, Value: "scribbled"}
	second, _, _ := r.arithSpans(text)
	if second[0].Value == "scribbled" {
		t.Error("a caller's write to the spans it was handed reached the next caller")
	}
}

// The key is the grammar: a runner on another dialect pointer, or with the
// arithmetic order moved, is a different entry, and the two texts the cache
// does not keep are read every time.
func TestTheSpanCacheIsKeyedOnWhatReadsTheText(t *testing.T) {
	r := cacheTestRunner()
	const text = " $keyed + 1 "
	r.arithSpans(text)
	key := arithSpanKey{text: text, d: r.Dialect, prec: r.arithPrecedence, moved: r.arithPrecedenceMoved}
	if _, ok := arithSpanCache.Load(key); !ok {
		t.Fatal("the text was not kept under the runner's own grammar, so nothing below tests anything")
	}
	other := cacheTestRunner()
	if _, ok := arithSpanCache.Load(arithSpanKey{text: text, d: other.Dialect}); ok {
		t.Error("a runner on another dialect pointer finds an entry it never made")
	}
	r.SetArithPrecedence(syntax.ArithPrecedenceShiftsAndBitwiseBindTighter)
	moved := arithSpanKey{text: text, d: r.Dialect, prec: r.arithPrecedence, moved: true}
	if _, ok := arithSpanCache.Load(moved); ok {
		t.Error("a moved arithmetic order finds the entry made before it moved")
	}
	for _, text := range []string{" $é + 1 ", " $(cat <<x\n1\nx\n) "} {
		r.arithSpans(text)
		k := arithSpanKey{text: text, d: r.Dialect, prec: r.arithPrecedence, moved: r.arithPrecedenceMoved}
		if _, ok := arithSpanCache.Load(k); ok {
			t.Errorf("%q was kept, and the cache does not read what decides it", text)
		}
	}
}

// Shared by every runner in the process, so it is read and filled from
// several goroutines at once; -race is the judge.
func TestTheSpanCacheIsSharedSafely(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := cacheTestRunner()
			for i := range 50 {
				text := fmt.Sprintf(" $v%d + ${w[%d]} ", i%5, g%3)
				if spans, _, ok := r.arithSpans(text); !ok || len(spans) == 0 {
					t.Errorf("%q read as %v, %v", text, spans, ok)
				}
			}
		}()
	}
	wg.Wait()
}

// The same move from the same dialect is the same pointer, and the pointer it
// hands back holds what a fresh copy would: the moved field, and every other
// field as the dialect it came from had it.
func TestADialectMoveIsKeptAndIsTheCopyItStandsFor(t *testing.T) {
	t.Parallel()
	r := cacheTestRunner()
	base := r.Dialect
	r.SetDoubledQuoteInSingleQuotes(true)
	on := r.Dialect
	if on == base || !on.DoubledQuoteInSingleQuotesIsALiteralQuote {
		t.Fatal("the move did not move the dialect, so nothing below tests anything")
	}
	if on.ExtendedPattern != base.ExtendedPattern || on.ShortFormBody != base.ShortFormBody {
		t.Error("the moved copy changed a field the move does not name")
	}
	r.Dialect = base
	r.SetDoubledQuoteInSingleQuotes(true)
	if r.Dialect != on {
		t.Error("the same move from the same dialect made a second copy")
	}
	r.Dialect = base
	r.SetShortRepeatBody(!base.ShortRepeatBody)
	if r.Dialect == on {
		t.Error("a different move was answered with another move's copy")
	}
	// A clone does not share the table: the copies it holds were built by
	// this runner's readers.
	if c := r.clone(); c.dialectMoves != nil {
		t.Error("a clone shares the runner's kept dialect moves")
	}
}
