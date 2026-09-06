// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/repl"
)

// -highlight is read as this binary's own flag and reaches the shell, and
// without it nothing colors anything.
//
// Both halves, because a flag that is read and dropped looks exactly like a
// flag that works: the coloring happens only at a terminal, which no test of
// the flag reaching main can have.
func TestHighlightIsReadAndReachesTheShell(t *testing.T) {
	own, rest, err := readOwnFlags([]string{"-highlight", "-c", "echo hi"})
	if err != nil {
		t.Fatal(err)
	}
	if !own.highlight {
		t.Error("-highlight was not read")
	}
	if len(rest) != 2 || rest[0] != "-c" {
		t.Errorf("what is left for the shell is %q, want the -c and its operand", rest)
	}

	if own, _, err = readOwnFlags([]string{"-c", "echo hi"}); err != nil {
		t.Fatal(err)
	}
	if own.highlight {
		t.Error("-highlight was read from a line that does not have it")
	}
}

// The flag reaches the shell, and its absence leaves the line plain.
func TestWithHighlightingWiresWhatWasAskedFor(t *testing.T) {
	if got := withHighlighting(driver.Shell{}, ownFlags{}).Highlighter; got != nil {
		t.Errorf("a shell with no -highlight carries %#v, want no highlighter", got)
	}
	got := withHighlighting(driver.Shell{}, ownFlags{highlight: true}).Highlighter
	if got == nil {
		t.Fatal("-highlight did not reach the shell")
	}
	runs := got.Highlight(`echo "open`)
	if len(runs) != 1 || runs[0].Style != unclosedQuoteStyle {
		t.Errorf("the highlighter that arrived answers %v, want one run styled %q", runs, unclosedQuoteStyle)
	}
}

// What the flag turns on: the unclosed-quote highlighter, in red.
//
// The style asserted as the bytes it is, because a color read back as a name
// says nothing about what reaches a terminal.
func TestTheFlagsHighlighterColorsAnUnclosedQuotation(t *testing.T) {
	h := repl.UnclosedQuote{Style: unclosedQuoteStyle}
	got := h.Highlight(`echo "one two`)
	if len(got) != 1 {
		t.Fatalf("an unclosed quotation produced %d runs, want 1", len(got))
	}
	if got[0].Style != "\x1b[31m" {
		t.Errorf("the run is styled %q, want red", got[0].Style)
	}
	if got[0].Start != 5 || got[0].End != len(`echo "one two`) {
		t.Errorf("the run is [%d,%d), want [5,%d)", got[0].Start, got[0].End, len(`echo "one two`))
	}
	if runs := h.Highlight(`echo "one two"`); runs != nil {
		t.Errorf("a closed quotation produced %v, want nothing", runs)
	}
}

// The usage message names it, because a flag nobody can find is a flag nobody
// turns on — which for a flag whose only job is reaching a seam means the seam
// is unreachable in practice.
func TestUsageNamesTheHighlightFlag(t *testing.T) {
	out, _, code := helped(t, "-h")
	if code != 0 {
		t.Fatalf("status %d, want 0", code)
	}
	if !strings.Contains(out, "-highlight") {
		t.Errorf("the usage message does not name -highlight:\n%s", out)
	}
}
