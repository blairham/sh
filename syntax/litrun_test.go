// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// A literal run is a substring of the source until it skips something, and a
// copy from then on; either way it must spell what appending each byte to a
// buffer would (#6381). Every way of cutting a short source into a sequence of
// consumed and skipped stretches is held to a strings.Builder fed the same.
func TestLitRunSpellsWhatWasAdded(t *testing.T) {
	t.Parallel()
	const src = "abcdefgh"
	// Each bit pattern says, for each byte, whether it is added (1) or
	// skipped (0); a second pattern says where one add stops and the next
	// begins, so contiguous adds of several bytes are covered too.
	for keep := range 1 << len(src) {
		for cut := range 1 << len(src) {
			r := litRun{src: src}
			var want strings.Builder
			from := -1
			for i := 0; i <= len(src); i++ {
				added := i < len(src) && keep&(1<<i) != 0
				if from >= 0 && (!added || cut&(1<<i) != 0) {
					r.add(from, i)
					want.WriteString(src[from:i])
					from = -1
				}
				if added && from < 0 {
					from = i
				}
			}
			if r.String() != want.String() || r.Len() != want.Len() {
				t.Fatalf("keep %08b cut %08b: run is %q (%d), want %q", keep, cut, r.String(), r.Len(), want.String())
			}
			r.Reset()
			if r.Len() != 0 || r.String() != "" {
				t.Fatalf("keep %08b cut %08b: a reset run is %q", keep, cut, r.String())
			}
		}
	}
}

// The case the copy exists for, end to end: a backslash-newline inside a word
// joins the two halves, outside quotes and inside them, and the value is the
// joined text rather than a stretch of the source.
func TestLitRunJoinsAContinuation(t *testing.T) {
	t.Parallel()
	for src, want := range map[string]string{
		"ab\\\ncd":     "abcd",
		"\"ab\\\ncd\"": "abcd",
		"abcd":         "abcd",
		"\"abcd\"":     "abcd",
	} {
		tok := NewLexer(src, Core()).Next()
		if got := tok.Literal(); got != want {
			t.Errorf("%q reads as %q, want %q", src, got, want)
		}
	}
}
