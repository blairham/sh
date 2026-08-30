// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/internal/syntax"
)

// The oracle corpus is real shell that real shells ran. Long before an
// interpreter exists, every one of those snippets has to lex — cleanly, with
// no leftover error and no claim of being unfinished, because each was written
// as a complete program and executed as one.
//
// This is the earliest conformance signal available: it is the corpus checking
// the implementation rather than the panel.
func TestCorpusLexes(t *testing.T) {
	for _, c := range oracle.Corpus {
		t.Run(c.ID, func(t *testing.T) {
			l := syntax.NewLexer(c.Snippet, syntax.Bash())
			toks := l.Tokens()

			if err := l.Err(); err != nil {
				t.Fatalf("did not lex: %v\n  snippet: %s", err, c.Snippet)
			}
			if l.Incomplete() {
				t.Fatalf("reported incomplete, but this snippet ran to completion in real shells\n  snippet: %s", c.Snippet)
			}
			if len(toks) == 0 || toks[len(toks)-1].Kind != syntax.EOF {
				t.Fatalf("token stream did not end at EOF\n  snippet: %s", c.Snippet)
			}

			// Every byte of the input has to be inside some token or between
			// tokens; positions never going backwards is the weakest form of
			// that and catches a scanner that loses its place.
			var last syntax.Pos
			for _, tk := range toks {
				if tk.Pos.Offset < last.Offset {
					t.Fatalf("positions went backwards at %v\n  snippet: %s", tk.Pos, c.Snippet)
				}
				last = tk.Pos
			}
		})
	}
}
