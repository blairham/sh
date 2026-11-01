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

func arithTreeKeyOf(r *Runner, text string, reading arithReading) arithTreeKey {
	return arithTreeKey{
		text:    text,
		d:       r.Dialect,
		prec:    r.arithPrecedence,
		moved:   r.arithPrecedenceMoved,
		reading: reading,
	}
}

func readTreeFresh(t *testing.T, r *Runner, text string, reading arithReading) (syntax.ArithExpr, error) {
	t.Helper()
	defer func(prev bool) { arithTreeCacheDisabled = prev }(arithTreeCacheDisabled)
	arithTreeCacheDisabled = true
	return r.readArithText(text, reading)
}

// A hit is the parse it stands for: the same tree, node for node, as reading
// the text again would give — asked twice, so that the second answer is the
// one the cache held. Each reading is asked separately, and the entry is
// looked for first, so a cache that kept nothing cannot pass by agreeing with
// itself.
func TestACachedArithTreeIsTheParseItStandsFor(t *testing.T) {
	r := cacheTestRunner()
	for _, reading := range []arithReading{arithReadExpanded, arithReadAsWritten, arithReadSubscript} {
		for _, text := range []string{
			"offset + __asize + 1",
			" 3 * (x - 1) ",
			"a[i+1] ? b : c >> 2",
			"n = m++, k += 2",
			"0x1f & ~mask",
			"9223372036854775808 + 1",
			"-(x) + !y || z",
		} {
			if _, err := r.readArithText(text, reading); err != nil {
				t.Fatalf("reading %d, %q: refused (%v), so the row tests nothing", reading, text, err)
			}
			if _, ok := arithTreeCache.Load(arithTreeKeyOf(r, text, reading)); !ok {
				t.Fatalf("reading %d, %q: not kept, so the comparison below tests nothing", reading, text)
			}
			cached, err := r.readArithText(text, reading)
			if err != nil {
				t.Fatalf("reading %d, %q: the hit answered %v", reading, text, err)
			}
			fresh, err := readTreeFresh(t, r, text, reading)
			if err != nil {
				t.Fatalf("reading %d, %q: the fresh read answered %v", reading, text, err)
			}
			if !reflect.DeepEqual(cached, fresh) {
				t.Errorf("reading %d, %q: the cache answered %#v, a fresh read %#v", reading, text, cached, fresh)
			}
		}
	}
}

// The key is the grammar and the reading: another dialect pointer, a moved
// arithmetic order or another of the three readings is a different entry. And
// the three kinds of text the cache does not keep are read every time, the
// refusal included.
func TestTheArithTreeCacheIsKeyedOnWhatReadsTheText(t *testing.T) {
	r := cacheTestRunner()
	const text = "keyed + 1"
	if _, err := r.readArithText(text, arithReadExpanded); err != nil {
		t.Fatal(err)
	}
	if _, ok := arithTreeCache.Load(arithTreeKeyOf(r, text, arithReadExpanded)); !ok {
		t.Fatal("the text was not kept under the runner's own grammar, so nothing below tests anything")
	}
	if _, ok := arithTreeCache.Load(arithTreeKeyOf(r, text, arithReadAsWritten)); ok {
		t.Error("another reading of the same text finds an entry it never made")
	}
	if _, ok := arithTreeCache.Load(arithTreeKeyOf(cacheTestRunner(), text, arithReadExpanded)); ok {
		t.Error("a runner on another dialect pointer finds an entry it never made")
	}
	// The order is asked by reading, not by looking for a key: a text whose
	// two orders are two different trees, kept in the native order first.
	const ordered = "2 + 3 & 1"
	native, err := r.readArithText(ordered, arithReadExpanded)
	if err != nil {
		t.Fatal(err)
	}
	r.SetArithPrecedence(syntax.ArithPrecedenceShiftsAndBitwiseBindTighter)
	moved, err := r.readArithText(ordered, arithReadExpanded)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := readTreeFresh(t, r, ordered, arithReadExpanded)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(native, fresh) {
		t.Fatalf("%q reads the same in both orders, so nothing here tests the order", ordered)
	}
	if !reflect.DeepEqual(moved, fresh) {
		t.Error("a moved arithmetic order was answered with the tree read before it moved")
	}
	for _, c := range []struct {
		text    string
		reading arithReading
	}{
		{"é + 1", arithReadExpanded},
		{"1 << 2", arithReadExpanded},
		{"1 +", arithReadExpanded},
		{"$x + 1", arithReadAsWritten},
	} {
		r.readArithText(c.text, c.reading)
		if _, ok := arithTreeCache.Load(arithTreeKeyOf(r, c.text, c.reading)); ok {
			t.Errorf("%q was kept, and it is not a text the cache keeps", c.text)
		}
	}
	// A refusal is the parser's every time, not the first time only.
	for range 2 {
		if _, err := r.readArithText("1 +", arithReadExpanded); err == nil {
			t.Error("an unreadable text read as a tree")
		}
	}
}

// Building a dialect for a parse records that a text was read while the order
// stood away from the dialect's own (Runner.arithReadInAnotherOrder), and a
// hit builds none. So a runner whose first read of a text is a hit has to
// record it all the same, or every tree it holds is trusted in an order it
// was not read in.
func TestAnArithTreeHitRecordsTheOrderItWasReadIn(t *testing.T) {
	first := cacheTestRunner()
	second := cacheTestRunner()
	second.Dialect = first.Dialect
	if first.nativeArithPrecedence() == syntax.ArithPrecedenceShiftsAndBitwiseBindTighter {
		t.Fatal("the core order is the one this moves to, so nothing below moves anything")
	}
	first.SetArithPrecedence(syntax.ArithPrecedenceShiftsAndBitwiseBindTighter)
	second.SetArithPrecedence(syntax.ArithPrecedenceShiftsAndBitwiseBindTighter)
	const text = "1 + 2 & 3 | 4"
	first.readArithText(text, arithReadExpanded)
	if _, ok := arithTreeCache.Load(arithTreeKeyOf(first, text, arithReadExpanded)); !ok {
		t.Fatal("the text was not kept, so the second read below is not a hit")
	}
	if !first.arithReadInAnotherOrder {
		t.Fatal("the parse itself did not record the order, so the hit below has nothing to match")
	}
	if second.arithReadInAnotherOrder {
		t.Fatal("the second runner recorded an order before reading anything")
	}
	second.readArithText(text, arithReadExpanded)
	if !second.arithReadInAnotherOrder {
		t.Error("a hit in a moved order did not record it, where the parse it stands for does")
	}
}

// A full cache starts again rather than stopping: the texts read once it
// filled are kept, and the ones that filled it are let go.
func TestAFullArithTreeCacheStartsAgain(t *testing.T) {
	arithTreeCache.Clear()
	arithTreeCacheSize.Store(0)
	r := cacheTestRunner()
	text := func(i int) string { return fmt.Sprintf("full_%d + 1", i) }
	for i := range arithTreeCacheLimit {
		r.readArithText(text(i), arithReadExpanded)
	}
	if _, ok := arithTreeCache.Load(arithTreeKeyOf(r, text(0), arithReadExpanded)); !ok {
		t.Fatal("the cache filled without keeping its first text, so nothing below tests anything")
	}
	last := text(arithTreeCacheLimit)
	r.readArithText(last, arithReadExpanded)
	if _, ok := arithTreeCache.Load(arithTreeKeyOf(r, last, arithReadExpanded)); !ok {
		t.Error("a text read once the cache was full was not kept")
	}
	if _, ok := arithTreeCache.Load(arithTreeKeyOf(r, text(0), arithReadExpanded)); ok {
		t.Error("a full cache kept what filled it")
	}
	if n := arithTreeCacheSize.Load(); n >= arithTreeCacheLimit {
		t.Errorf("the count is %d after starting again", n)
	}
}

// Shared by every runner in the process, so it is read and filled from
// several goroutines at once; -race is the judge.
func TestTheArithTreeCacheIsSharedSafely(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := cacheTestRunner()
			for i := range 50 {
				text := fmt.Sprintf("v%d + w[%d] * 2", i%5, g%3)
				if tree, err := r.readArithText(text, arithReadExpanded); err != nil || tree == nil {
					t.Errorf("%q read as %v, %v", text, tree, err)
				}
			}
		}()
	}
	wg.Wait()
}
