// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"
	"unsafe"

	"github.com/blairham/sh/syntax"
)

// The two shapes a parsed program is mostly made of, and what each is
// allowed to cost.
//
// A tree is not a thing a shell builds once and drops. A real interactive
// startup reads about 6MB of shell — an rc file, a plugin framework and the
// function files it sources — and keeps what it parsed for as long as the
// session lasts, because a function that is defined might be called. That
// came to 150,000 [syntax.Word]s and 185,000 [syntax.Span]s, so these two
// sizes are multiplied by six figures before anything else in the tree is
// counted, and they are what the resident heap is mostly made of.
//
// Which matters because of where the time goes. The profile of a warm
// startup is `runtime.madvise` and `runtime.memclrNoHeapPointers` — mapping
// and zeroing pages — with nothing of ours reaching 1%. Those are
// proportional to how large the heap has to *be*, which tracks what is kept;
// measured twice on #2073, reducing the *garbage* instead changed nothing
// anybody could detect.
//
// So this is a budget, and the numbers are not arbitrary. 48 and 64 are Go
// size classes: an allocation of 72 bytes — which is what both of these were
// — is served from the 80-byte class and the spare 8 are paid for and never
// addressable. A field added in reading order rather than in width order
// gives the padding straight back, and nothing else in the repository would
// notice.
//
// Raising one of these is a decision, not a fix. It costs about 150KB of
// resident memory per byte on Word and 185KB per byte on Span, and the
// sentence saying why belongs here next to the number.
var nodeBudget = []struct {
	name string
	got  uintptr
	want uintptr
	// perTree is how many of these one real startup was measured to keep,
	// so that a failure can price itself rather than report a number.
	perTree int
}{
	{"Word", unsafe.Sizeof(syntax.Word{}), 48, 149937},
	{"Span", unsafe.Sizeof(syntax.Span{}), 64, 185748},
	{"Pos", unsafe.Sizeof(syntax.Pos{}), 12, 485622},
}

func TestTheTwoShapesATreeIsMadeOfStayWithinTheirBudget(t *testing.T) {
	for _, b := range nodeBudget {
		if b.got != b.want {
			t.Errorf("%s is %d bytes, budgeted %d — one startup keeps %d of them, "+
				"so that is %.1fMB of resident memory (#2073)",
				b.name, b.got, b.want,
				b.perTree, float64(b.perTree)*float64(int(b.got)-int(b.want))/1e6)
		}
	}
}

// TestPositionsCoverAnyInputTheParserCanHold guards the bound the size of
// [syntax.Pos] is bought with.
//
// Its fields are int32, so a position can name 2GiB of input and no more.
// That is not a limit anything reaches — the parser holds the whole of its
// input as one string, and a 2GiB shell script is not a thing — but the
// failure it would produce is a wrapped offset and a diagnostic pointing at
// the wrong place, which is silent. Writing the bound down as a test is what
// makes it a decision rather than an accident of a type.
func TestPositionsCoverAnyInputTheParserCanHold(t *testing.T) {
	const twoGiB = 1<<31 - 1
	p := syntax.Pos{Offset: twoGiB, Line: twoGiB, Col: twoGiB}
	if int64(p.Offset) != twoGiB || int64(p.Line) != twoGiB || int64(p.Col) != twoGiB {
		t.Errorf("a position cannot hold %d, so the parser cannot name the end of its own input", twoGiB)
	}
}
