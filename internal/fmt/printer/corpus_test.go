// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package printer_test

import (
	"sort"
	"testing"

	"github.com/blairham/sh/internal/fmt/comments"
	"github.com/blairham/sh/internal/fmt/printer"
	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/syntax"
)

// The corpus as the formatter's gate, which is what #1401 asked for by name.
//
// It is free evidence: some two thousand cases written to pin *behavior*, by
// people not thinking about layout, so nothing in it was chosen to please a
// printer. Every case is held to the three promises at once — the output is
// the same program, formatting it again changes nothing, and no comment is
// lost — under two styles, because a formatter that keeps its promises under
// one arrangement need not keep them under another.
func TestFormattingTheCorpusKeepsItsPromises(t *testing.T) {
	styles := []struct {
		name  string
		style syntax.Style
	}{
		{"core", syntax.CoreStyle()},
		// The preserving answer, run over a corpus that has no brace short
		// form in it. That is the point: the field must not change anything
		// where the construct is absent, and a corpus-wide run is what says
		// so rather than an assertion that it does not.
		{"preserving", preserving()},
	}
	d := oracle.Dialect()
	for _, st := range styles {
		t.Run(st.name, func(t *testing.T) {
			var formatted int
			for _, c := range oracle.Corpus {
				if c.SyntaxError {
					// Nothing to lay out: these are in the corpus because
					// the reference shells reject them.
					continue
				}
				in, err := syntax.Parse(c.Snippet, d)
				if err != nil {
					t.Errorf("%s: did not parse: %v\n  snippet: %s", c.ID, err, c.Snippet)
					continue
				}
				cs := comments.Recover(c.Snippet, in)
				out := printer.Format(c.Snippet, in, cs, st.style)
				after, err := syntax.Parse(out, d)
				if err != nil {
					t.Errorf("%s: output does not parse: %v\n  snippet: %s\n  output: %s",
						c.ID, err, c.Snippet, out)
					continue
				}
				if why, ok := syntax.SameProgram(in, after); !ok {
					t.Errorf("%s: the program changed at %s\n  snippet: %s\n  output: %s",
						c.ID, why, c.Snippet, out)
					continue
				}
				outCs := comments.Recover(out, after)
				if got, want := texts(outCs), texts(cs); !sameStrings(got, want) {
					t.Errorf("%s: comments changed\n  before: %q\n  after:  %q", c.ID, want, got)
					continue
				}
				if again := printer.Format(out, after, outCs, st.style); again != out {
					t.Errorf("%s: not a fixed point\n  once:  %q\n  twice: %q", c.ID, out, again)
					continue
				}
				formatted++
			}
			// A gate that silently formatted nothing would pass. This is the
			// same guard the round-trip test puts on its own count.
			if formatted < 1500 {
				t.Errorf("only %d cases were formatted; the corpus should supply far more", formatted)
			}
			t.Logf("%d corpus cases formatted and verified", formatted)
		})
	}
}

func preserving() syntax.Style {
	s := syntax.CoreStyle()
	s.BraceShortForm = syntax.PreserveShortForm
	return s
}

func texts(cs []comments.Comment) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Text
	}
	sort.Strings(out)
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
