// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"reflect"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// This shell's third arrangement: a whole script written back.
//
// Measured 2026-09-19 on /opt/homebrew/bin/bash 5.3.20, every case run as
// `env -i PATH=/usr/bin:/bin LC_ALL=C bash --pretty-print f.sh </dev/null` and
// compared byte for byte. Here rather than beside the printer for the reason
// the other two arrangements are: an arrangement is one shell's taste.
//
// The finding the whole thing rests on is that this is not a formatter's
// output but the *listing* layout applied to a file — one header spelling
// apart, plus what a file has that a function body does not. The rows below
// are the ones that part the two.
func TestAScriptIsWrittenBackTheListingWay(t *testing.T) {
	l := bash.ScriptListingLayout()
	// The decoder the runner supplies, stubbed to the one escape a row uses:
	// what `$'…'` comes to is this shell's answer and the layout carries a
	// function rather than a flag for exactly that reason.
	l.AnsiCQuotedWordIsItsValue = func(text string) string {
		if text == `a\tb` {
			return "a\tb"
		}
		return text
	}

	for _, c := range []struct {
		name, src, want string
	}{
		{
			"a group at file scope keeps its line",
			"{ echo a; echo b; }\n",
			"{ echo a; echo b; }\n\n",
		},
		{
			"the same group inside a declaration is opened out",
			"f() { { echo a; echo b; }; }\n",
			"f () \n{ \n    { \n        echo a;\n        echo b\n    }\n}\n\n",
		},
		{
			"a nested declaration loses the keyword a listing gives it",
			"f() { inner() { echo a; }; }\n",
			"f () \n{ \n    inner () \n    { \n        echo a\n    }\n}\n\n",
		},
		{
			"a one-line if is opened out and its body joined",
			"if true; then echo a; echo b; fi\n",
			"if true; then\n    echo a; echo b;\nfi\n\n",
		},
		{
			"a nested if joins what follows it",
			"if true; then if false; then echo x; echo y; fi; echo z; fi\n",
			"if true; then\n    if false; then\n        echo x; echo y;\n    fi; echo z;\nfi\n\n",
		},
		{
			"the do of a loop over words takes a line",
			"for i in 1 2; do echo $i; done\n",
			"for i in 1 2;\ndo\n    echo $i;\ndone\n\n",
		},
		{
			"every case arm is opened out",
			"case $x in a) echo A;; *) echo B;; esac\n",
			"case $x in \n    a)\n        echo A\n    ;;\n    *)\n        echo B\n    ;;\nesac\n\n",
		},
		{
			"two declarations written on one line stay on one",
			"f() { echo a; } ; g() { echo b; }\n",
			"f () \n{ \n    echo a\n}; g () \n{ \n    echo b\n}\n\n",
		},
		{
			"a comment block leaves a blank line behind",
			"#!/bin/bash\n# a comment\necho a # trailing\n",
			"\necho a\n\n",
		},
		{
			"a gap of any size is one blank line",
			"echo a\n\n\necho b\n",
			"echo a\n\necho b\n\n",
		},
		{
			"a body that is not a group is put in one",
			"f() ( echo a; echo b )\n",
			"f () \n{ \n    ( echo a;\n    echo b )\n}\n\n",
		},
		{
			"a here-document body is part of its unit",
			"cat <<EOT\nhi\nEOT\necho after\n",
			"cat <<EOT\nhi\nEOT\n\necho after\n\n",
		},
		{
			"a dollar-single word is written as what it stands for",
			"echo $'a\\tb'\n",
			"echo 'a\tb'\n\n",
		},
		{
			"an empty file is a single newline",
			"",
			"\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser(c.src, bash.Dialect())
			f := p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("parsing %q: %v", c.src, err)
			}
			if got := syntax.PrintFileWith(f, l); got != c.want {
				t.Errorf("\n got %q\nwant %q", got, c.want)
			}
		})
	}
}

// The listing arrangement and the script arrangement are the same answers one
// field apart, which is the claim the change rests on and is worth asserting
// rather than leaving to the rows above.
//
// A declaration nested inside a *listed body* is spelled with the keyword and
// the parentheses; the same declaration written back as part of a script is
// spelled with the parentheses alone. Everything else that differs is a field
// a function body never reaches — a top level.
func TestTheScriptArrangementIsTheListingOneFourFieldsApart(t *testing.T) {
	listing, script := bash.FunctionLayout(), bash.ScriptListingLayout()
	if listing.FunctionHeader != syntax.FunctionHeaderKeywordAndParens {
		t.Errorf("listing header %v", listing.FunctionHeader)
	}
	if script.FunctionHeader != syntax.FunctionHeaderParens {
		t.Errorf("script header %v", script.FunctionHeader)
	}
	if !script.StatementsShareALineOutsideADeclaration ||
		!script.FileFollowsTheSourceUnits || !script.TrailingBlankLine {
		t.Errorf("a top-level answer is missing: %+v", script)
	}
	// And the two are otherwise one value: put the four back and nothing is
	// left over. A field added to one arrangement and forgotten in the other
	// is exactly what this catches.
	script.FunctionHeader = listing.FunctionHeader
	script.StatementsShareALineOutsideADeclaration = false
	script.FileFollowsTheSourceUnits = false
	script.TrailingBlankLine = false
	if !reflect.DeepEqual(script, listing) {
		t.Errorf("the two arrangements differ beyond the four fields:\n %+v\n %+v", script, listing)
	}
}
