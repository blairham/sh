// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/syntax"
)

// The whole corpus, printed and read back.
//
// Two properties, and neither is "it looks like the input". The tree is what
// is promised, so the checks are about the tree: what comes back has to parse
// at all, and printing it again has to give the same text. The second is what
// catches a printer that loses something — a lost quote parses fine and comes
// back different the next time round.
//
// The corpus is the right input for this because nobody wrote it for a
// printer: 483 snippets someone wrote to pin down a *behavior*, which is a
// better adversary than anything written to exercise this code.
func TestPrintingRoundTripsTheCorpus(t *testing.T) {
	for _, c := range oracle.Corpus {
		if c.SyntaxError {
			// Nothing to print: these are here because the shells reject
			// them.
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			first, err := syntax.Parse(c.Snippet, syntax.Core())
			if err != nil {
				t.Skipf("not core syntax: %v", err)
			}
			printed := syntax.Print(first)

			second, err := syntax.Parse(printed, syntax.Core())
			if err != nil {
				t.Fatalf("printed source does not parse: %v\n  from: %s\n  gave: %s", err, c.Snippet, printed)
			}
			if again := syntax.Print(second); again != printed {
				t.Errorf("printing is not settled:\n  from:  %s\n  once:  %s\n  twice: %s", c.Snippet, printed, again)
			}
		})
	}
}

// What the corpus could not reach.
//
// Found by printing every shell script installed on this machine and reading
// it back — 163 of them parse, and nine did not survive the round trip. The
// corpus has 490 snippets and none of them does either of these, because
// nobody writes a case to exercise a printer.
func TestPrintingWhatTheCorpusDoesNotReach(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{
			// A here-document body ends the line, so what closes the block
			// is at the start of the next one and `; }` there is a `;` with
			// nothing before it. Four scripts on this machine close a block
			// straight after a here-document.
			"a block closed after a here-document",
			"f() { cat <<EOF\nbody\nEOF\n}\n",
		},
		{
			"a conditional closed after one",
			"if true; then cat <<EOF\nbody\nEOF\nfi\n",
		},
		{
			"an arm closed after one",
			"case x in a) cat <<EOF\nbody\nEOF\n;; esac\n",
		},
		{
			// `$((` is arithmetic, so a command substitution whose first
			// command is a subshell needs the space — the same trap as a
			// redirection target beginning with `<`.
			"a substitution beginning with a subshell",
			"x=$( (echo hi) 2>/dev/null)\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			printed := syntax.Print(f)
			if _, err := syntax.Parse(printed, syntax.Core()); err != nil {
				t.Fatalf("printed source does not parse: %v\n  from: %q\n  gave: %q", err, tc.src, printed)
			}
		})
	}
}
