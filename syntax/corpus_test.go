// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"

	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/syntax"
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
			l := syntax.NewLexer(c.Snippet, bash.Dialect())
			toks := l.Tokens()

			if err := l.Err(); err != nil {
				// A snippet the reference shells reject may be rejected at
				// the lexer as readily as at the parser — input that runs out
				// inside a quote is the plain case — and the corpus records
				// both. TestCorpusParses makes the same allowance.
				if c.SyntaxError {
					return
				}
				t.Fatalf("did not lex: %v\n  snippet: %s", err, c.Snippet)
			}
			if l.Incomplete() && !c.Unfinished {
				t.Fatalf("reported incomplete, but this snippet ran to completion in real shells\n  snippet: %s", c.Snippet)
			}
			if len(toks) == 0 || toks[len(toks)-1].Kind != syntax.TokEOF {
				t.Fatalf("token stream did not end at TokEOF\n  snippet: %s", c.Snippet)
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

// Every corpus snippet is a complete program that real shells ran, so each
// must parse. This is the parser's version of TestCorpusLexes and the same
// argument: the corpus checking the implementation rather than the panel.
func TestCorpusParses(t *testing.T) {
	for _, c := range oracle.Corpus {
		t.Run(c.ID, func(t *testing.T) {
			p := syntax.NewParser(c.Snippet, bash.Dialect())
			f := p.Parse()

			// Cases the reference shells reject must be rejected here too.
			// Accepting them would be just as wrong as failing on the rest,
			// and the corpus records both sides.
			if c.SyntaxError {
				if p.Err() == nil {
					t.Fatalf("accepted a snippet the reference shells reject\n  snippet: %s", c.Snippet)
				}
				return
			}
			if err := p.Err(); err != nil {
				t.Fatalf("did not parse: %v\n  snippet: %s", err, c.Snippet)
			}
			if p.Incomplete() && !c.Unfinished {
				t.Fatalf("reported incomplete, but this ran to completion in real shells\n  snippet: %s", c.Snippet)
			}
			if len(f.Stmts) == 0 {
				t.Fatalf("parsed to no statements\n  snippet: %s", c.Snippet)
			}
		})
	}
}
