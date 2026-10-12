// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
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

// Reading a command is linear in its words where the dialect takes a name list
// in front of `()`. Whether the words so far could be that list is asked at
// every word, and it was answered by reading all of them again each time: a
// completion dump's `autoload -Uz` line of about 1,500 names was a million
// questions and about 25 ms of every interactive start on a real
// configuration (#6363).
//
// A ratio of two sizes rather than a number, so the machine's speed and load
// cancel, and the best of five with the collector held off, for the reasons
// interp/indexedarray_test.go gives. Linear is a ratio near 4 and quadratic
// one near 16; the bound sits between them.
func TestAWordListIsReadInLinearTime(t *testing.T) {
	if testing.Short() {
		t.Skip("times two parses")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	d := manyFuncNames()
	const n = 3000
	best := func(n int) time.Duration {
		src := longNameCommand(n)
		b := time.Duration(1<<63 - 1)
		for range 5 {
			runtime.GC()
			start := time.Now()
			f, err := Parse(src, d)
			el := time.Since(start)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd).Args); got != n+2 {
				t.Fatalf("read %d words, want %d", got, n+2)
			}
			b = min(b, el)
		}
		return b
	}
	small, large := best(n), best(4*n)
	if ratio := float64(large) / float64(small); ratio > 9 {
		t.Fatalf("a command of %d words took %v and one of %d took %v: ratio %.2f, want about 4 (linear)",
			n, small, 4*n, large, ratio)
	}
}
