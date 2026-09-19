// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package printer_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/fmt/comments"
	"github.com/blairham/sh/internal/fmt/printer"
	"github.com/blairham/sh/syntax"
)

// A redirection written in **front** of a compound command stays in front of
// it. Two dialects take one there, and this pass owns only the space between
// tokens — moving it to the end would be a change beyond the layout, which is
// the one promise this formatter makes.
//
// The canonical printer does move it, as it already does for a simple
// command's leading redirections; that printer promises the tree rather than
// the spelling. See syntax.Dialect.RedirectionBeforeACompound (#3560).
func TestARedirectionInFrontOfACompoundStaysThere(t *testing.T) {
	d, st := zsh.Dialect(), zsh.Style()
	for _, src := range []string{
		">/dev/null { echo hi; }\n",
		">/dev/null if true; then echo a; fi\n",
		"2>&1 3>&2 { echo x; }\n",
		">a 2>b { echo hi; } >c\n",
		">/dev/null for i in a; do echo hi; done\n",
		"{ echo hi; } >/dev/null\n",
	} {
		f, err := syntax.Parse(src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if got := printer.Format(src, f, comments.Recover(src, f), st); got != src {
			t.Errorf("format %q = %q, want it unmoved", src, got)
		}
	}
}

// And the one place a redirection standing before a command does *not* belong
// in front of it: a definition's, written between the names and the
// parentheses. It is the body's and is written after the body, which is where
// this formatter already put it.
func TestADefinitionsHeaderRedirectionIsStillTheBodys(t *testing.T) {
	d, st := zsh.Dialect(), zsh.Style()
	const src = "a b >out () { echo \"[$0]\"; }\n"
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := printer.Format(src, f, comments.Recover(src, f), st)
	if want := "a b () { echo \"[$0]\"; } >out\n"; got != want {
		t.Errorf("format = %q, want %q", got, want)
	}
	if _, err := syntax.Parse(got, d); err != nil {
		t.Errorf("the output does not parse: %v", err)
	}
}
