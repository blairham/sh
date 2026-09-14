// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// Whether a value's trailing blank goes on working when the blank is inside a
// quote the body opened, per dialect (#2685).
//
// The seam itself is unanimous — a quote the body opens reaches the rest of
// the line in all seven columns — and this is the one question inside it that
// is not. Measured 2026-09-13 from a script file, since zsh expands no alias
// under `-c`:
//
//	alias c='CEE'
//	alias q='echo "x '
//	q b" c
//
//	bash 5.3, bash as sh, bash 3.2   x  b CEE
//	dash, ksh93, zsh, BusyBox ash    x  b c
//
// bash offers the next *word* of the resulting line, which is the one after
// the quote closes. The other four offer the text immediately after the
// value, and that is inside the quote and is no word at all.
//
// Asked of each preset's own `syntax.Dialect`, because the table is the
// parser's and the front end is what hands it over: a dialect that answered
// this from a value nobody installed would pass a test written any other way.
func TestEachDialectDecidesWhetherTheBlankReachesPastTheQuote(t *testing.T) {
	const src = `q b" c`
	aliases := func(name string) (string, bool) {
		switch name {
		case "q":
			return `echo "x `, true
		case "c":
			return "CEE", true
		}
		return "", false
	}
	for _, c := range []struct{ name, want string }{
		{"bash", "CEE"},
		{"dash", "c"},
		{"ksh", "c"},
		{"zsh", "c"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser(src, presets[c.name].Dialect())
			p.Aliases = aliases
			f := p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("did not parse: %v", err)
			}
			got := strings.TrimSpace(syntax.Print(f))
			if want := `echo "x  b" ` + c.want; got != want {
				t.Errorf("came to %q, want %q", got, want)
			}
		})
	}
}

// The half of the same rule that does not split, and the control on the row
// above: the word the quote *swallows* is never offered to the table, in any
// column. `q b"` alone is `x  b` in all seven and never the expansion of `b`.
func TestTheWordInsideTheQuoteIsNeverOfferedToTheTable(t *testing.T) {
	aliases := func(name string) (string, bool) {
		switch name {
		case "q":
			return `echo "x `, true
		case "b":
			return "BEE", true
		}
		return "", false
	}
	for _, name := range []string{"bash", "dash", "ksh", "zsh"} {
		t.Run(name, func(t *testing.T) {
			p := syntax.NewParser(`q b"`, presets[name].Dialect())
			p.Aliases = aliases
			f := p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("did not parse: %v", err)
			}
			if got, want := strings.TrimSpace(syntax.Print(f)), `echo "x  b"`; got != want {
				t.Errorf("came to %q, want %q", got, want)
			}
		})
	}
}
