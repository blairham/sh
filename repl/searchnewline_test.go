// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// A newline ending a reverse incremental search is the *search's* in two of the
// three columns that have the mode, and accepts the line in the third.
//
// A carriage return accepts everywhere, so this is only ever reached by a `C-j`
// or by input that is a file or a pipe — which is the shape a test suite feeds
// an interactive shell, and where it was found. See
// HistoryStyle.SearchNewlineAcceptsTheLine for the measurement.
//
// Both answers on the same keys, because the one that accepts is a dialect's
// answer and not the absence of behavior: a test that asserted only the
// majority would pass against an editor that had never been told there were
// two.
func TestWhetherTheNewlineEndingASearchAcceptsTheLine(t *testing.T) {
	// `C-r one`, a newline, then `XX` and a carriage return. Where the newline
	// is the search's, the `XX` is typed into the line it found — at the match,
	// which is where this editor leaves the cursor — and the return accepts
	// that. Where the newline accepts, `XX` is a line of its own.
	const keys = "\x12one\nXX\r"
	for _, c := range []struct {
		name    string
		accepts bool
		want    []string
	}{
		{"the search's, which is bash's answer and ksh93's", false, []string{"echo XXone"}},
		{"and the line's, which is zsh's", true, []string{"echo one", "XX"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := &editor{
				in: typing(keys), out: &strings.Builder{},
				history:              []string{"echo one"},
				searchNewlineAccepts: c.accepts,
				width:                func() int { return 80 },
			}
			var got []string
			for range c.want {
				line, err := e.readLine(drawPrompt("$ "))
				if err != nil {
					t.Fatalf("read %d: %v", len(got), err)
				}
				got = append(got, line)
				e.remember(line)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Fatalf("read %v, want %v", got, c.want)
				}
			}
		})
	}
}

// And a carriage return accepts under either answer, which is what keeps a
// person pressing Return out of this entirely.
func TestACarriageReturnEndingASearchAlwaysAccepts(t *testing.T) {
	for _, accepts := range []bool{false, true} {
		e := &editor{
			in: typing("\x12one\r"), out: &strings.Builder{},
			history:              []string{"echo one"},
			searchNewlineAccepts: accepts,
			width:                func() int { return 80 },
		}
		line, err := e.readLine(drawPrompt("$ "))
		if err != nil {
			t.Fatalf("accepts=%v: %v", accepts, err)
		}
		if line != "echo one" {
			t.Errorf("accepts=%v gave %q, want the entry the search found", accepts, line)
		}
	}
}
