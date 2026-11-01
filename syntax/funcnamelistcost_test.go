// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"math"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// longNameCommand is one command of n plain words — `autoload -Uz _w0 … ` —
// the shape a completion dump's last line has, every word of which could be a
// definition's name.
func longNameCommand(n int) string {
	var b strings.Builder
	b.WriteString("autoload -Uz")
	for i := range n {
		b.WriteString(" _w")
		b.WriteString(strings.Repeat("x", i%7))
		b.WriteByte(byte('a' + i%26))
	}
	b.WriteByte('\n')
	return b.String()
}

// The sizes and the bound. wordListBound sits between the ratio a linear read
// comes to, 6 to 8 (a parse has a fixed cost, which the shorter command feels
// more), and the 50 of a read that asks every word again at each word.
const (
	wordListShort          = 128
	wordListLong           = 8 * wordListShort
	wordListSamples        = 32
	wordListControlSamples = 8
	wordListBound          = 16
)

// Reading a command is linear in its words where the dialect takes a name list
// in front of `()`. Whether the words so far could be that list is asked at
// every word, and it was answered by reading all of them again each time: a
// completion dump's `autoload -Uz` line of about 1,500 names was a million
// questions and about 25 ms of every interactive start on a real
// configuration (#6363).
//
// A ratio of two sizes rather than a number, so the machine's speed cancels:
// the least of wordListSamples reads of a command of wordListShort words and of
// one of wordListLong, the two taking turns, with the collector held off while
// a read is timed. It used to be the best of five reads of 3,000 words and then
// the best of five of 12,000, which is the shape that made interp's
// TestAppendingToAnIndexedArrayIsLinear fail a busy runner with nothing wrong
// (#7020, #7022). Two routes by which load reached that ratio are closed here
// the way that test closes them:
//
//   - Both reads are well under a scheduler's time slice, about 35 µs and
//     220 µs. The 2.7 ms read of 12,000 words was often cut by one on a
//     contended machine and the 0.6 ms read rarely, so it was the larger that
//     paid.
//   - The sizes take turns, so a stretch of load lands on both. Timed one size
//     after the other, it landed on whichever was being timed, and taking the
//     best of five could not undo it, because it slowed all five.
//
// Load only ever adds time, so to fail now every read of the longer command
// has to be slowed past the bound while one of the shorter is not.
//
// **The control is a read followed by the question asked the old way**, every
// word again at each word, and the test fails unless the measurement sees it
// grow past the bound: one that cannot tell the two apart passes everything.
func TestAWordListIsReadInLinearTime(t *testing.T) {
	if testing.Short() {
		t.Skip("times parses")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	d := manyFuncNames()
	asker := NewParser("", d)
	short, long := longNameCommand(wordListShort), longNameCommand(wordListLong)
	// read times one parse of src, and then of then, which is handed the
	// command's words.
	read := func(src string, n int, then func([]*Word)) time.Duration {
		runtime.GC()
		start := time.Now()
		f, err := Parse(src, d)
		var args []*Word
		if err == nil {
			args = f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd).Args
			then(args)
		}
		took := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		if len(args) != n+2 {
			t.Fatalf("read %d words, want %d", len(args), n+2)
		}
		return took
	}
	// least is the least of samples reads at each size, the two taking turns.
	least := func(samples int, then func([]*Word)) (onShort, onLong time.Duration, ratio float64) {
		onShort, onLong = math.MaxInt64, math.MaxInt64
		for range samples {
			onShort = min(onShort, read(short, wordListShort, then))
			onLong = min(onLong, read(long, wordListLong, then))
		}
		return onShort, onLong, float64(onLong) / float64(max(onShort, 1))
	}
	nothing := func([]*Word) {}
	names := 0
	askEveryWordAgain := func(args []*Word) {
		for i := range args {
			if asker.argsCanBeFuncNames(args[:i+1]) {
				names++
			}
		}
	}
	read(short, wordListShort, nothing) // untimed: the first of each
	read(long, wordListLong, nothing)
	s, l, ratio := least(wordListControlSamples, askEveryWordAgain)
	t.Logf("control, asking every word again: %v at %d words, %v at %d, ratio %.2f (%d names)",
		s, wordListShort, l, wordListLong, ratio, names)
	if ratio <= wordListBound {
		t.Fatalf("the control, a read that then asks every word again at each word, took %v at %d words and %v "+
			"at %d: ratio %.2f, so the measurement cannot see that walk", s, wordListShort, l, wordListLong, ratio)
	}
	s, l, ratio = least(wordListSamples, nothing)
	t.Logf("a command of %d words took %v and one of %d took %v: ratio %.2f", wordListShort, s, wordListLong, l, ratio)
	if ratio > wordListBound {
		t.Fatalf("a command of %d words took %v and one of %d took %v: ratio %.2f, want about %d (linear)",
			wordListShort, s, wordListLong, l, ratio, wordListLong/wordListShort)
	}
}
