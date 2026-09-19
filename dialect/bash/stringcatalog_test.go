// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// This shell has the `$"…"` mark and the options that list it; the other four
// have neither.
//
// Measured 2026-09-19 on /opt/homebrew/bin/bash 5.3.20 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on /dev/null.
func TestTheStringCatalogOptionIsThisShellsAlone(t *testing.T) {
	s := bash.Semantics()
	if got, want := s.StringCatalogOption.Spellings, "-D --dump-strings"; got != want {
		t.Errorf("spellings %q, want %q", got, want)
	}
	if got, want := s.StringCatalogOption.PortableObjectSpellings, "--dump-po-strings"; got != want {
		t.Errorf("portable spellings %q, want %q", got, want)
	}
	if !bash.Dialect().DollarDoubleQuote {
		t.Errorf("the grammar has no mark for the option to list")
	}
}

// And a listing drops the mark, which is measured rather than assumed: a body
// written with `$"hi"` comes back with plain quotes from `declare -f`, from
// `type` and through `export -f` alike, so the mark does not survive into any
// listing this shell makes.
func TestAListingDropsTheTranslationMark(t *testing.T) {
	src := `f() { echo $"hi"; }`
	p := syntax.NewParser(src, bash.Dialect())
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("parse: %v", err)
	}
	decl := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.FuncDecl)
	for _, c := range []struct {
		name   string
		layout syntax.Layout
		want   string
	}{
		{"shown to a person", bash.FunctionLayout(), "{ \n    echo \"hi\"\n}"},
		{"written for a child", bash.ExportedFunctionLayout(), "{  echo \"hi\"\n}"},
		{
			// The control: the mark is in the tree, so an arrangement that
			// did not ask to drop it writes it back. Without this row a
			// printer that had simply forgotten the `$` would pass.
			"kept where nothing asked to drop it", syntax.Layout{}, `{ echo $"hi"; }`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := syntax.PrintWith(decl.Body, c.layout); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// Nothing is translated. Recording the mark is a fact about the text and not a
// promise about a message catalog, and the vector says so by holding no
// answer for one.
func TestTheMarkPromisesNoTranslation(t *testing.T) {
	var zero interp.StringCatalogOption
	if bash.Semantics().StringCatalogOption == zero {
		t.Fatalf("this shell has the option")
	}
	// Whatever is added here later, a test that reads TEXTDOMAIN is the tell
	// that the promise has grown; there is nothing to read today.
}
