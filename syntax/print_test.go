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
