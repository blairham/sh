// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"reflect"
	"testing"

	"github.com/blairham/sh/syntax"
)

// Every `$"…"` a file was written with, in the order it was written.
//
// Named for the mark rather than for a shell: what is under test is that the
// tree records it and that the walk finds every one, which is the question an
// option that lists them asks. Nothing here translates anything.
func markDialect() syntax.Dialect {
	d := syntax.Core()
	d.DollarDoubleQuote = true
	return d
}

func TestEveryMarkedStringIsFound(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src string
		want      []syntax.TranslatedString
	}{
		{
			"one on its own", "echo $\"one\"",
			[]syntax.TranslatedString{{Text: "one", Line: 1}},
		},
		{
			// The control this walk needs most: an unmarked string must not
			// be listed, or the option would report every quoted word in the
			// program and read as working.
			"an unmarked string is not one", "echo \"plain\"; echo $\"marked\"",
			[]syntax.TranslatedString{{Text: "marked", Line: 1}},
		},
		{
			"in source order, with their lines", "echo $\"a\" $\"b\"\n\necho $\"c\"",
			[]syntax.TranslatedString{{Text: "a", Line: 1}, {Text: "b", Line: 1}, {Text: "c", Line: 3}},
		},
		{
			"mid-word", "echo pre$\"mid\"post",
			[]syntax.TranslatedString{{Text: "mid", Line: 1}},
		},
		{
			"an empty one", "echo $\"\"",
			[]syntax.TranslatedString{{Text: "", Line: 1}},
		},
		{
			// The places a word can stand that are not a command's argument.
			// Each is a different node, and the walk is the printer's, so a
			// node nobody thought of is still reached.
			"in a case pattern", "case $x in $\"pat\") echo a;; esac",
			[]syntax.TranslatedString{{Text: "pat", Line: 1}},
		},
		{
			"in a conditional operand", "[[ $x == $\"operand\" ]]",
			[]syntax.TranslatedString{{Text: "operand", Line: 1}},
		},
		{
			"in an assignment", "x=$\"assigned\"",
			[]syntax.TranslatedString{{Text: "assigned", Line: 1}},
		},
		{
			"in a redirection target", "echo hi > $\"target\"",
			[]syntax.TranslatedString{{Text: "target", Line: 1}},
		},
		{
			"in a function body", "f() { echo $\"inside\"; }",
			[]syntax.TranslatedString{{Text: "inside", Line: 1}},
		},
		{
			"in a loop's word list", "for i in $\"w\"; do echo $i; done",
			[]syntax.TranslatedString{{Text: "w", Line: 1}},
		},
		{
			// The text is what a reader of the script sees: escapes as
			// written, expansions as written, nothing performed.
			"the text is as written", "echo $\"with \\\"quotes\\\" and $var\"",
			[]syntax.TranslatedString{{Text: "with \\\"quotes\\\" and $var", Line: 1}},
		},
		{
			"a newline in the text", "echo $\"multi\nline\"",
			[]syntax.TranslatedString{{Text: "multi\nline", Line: 1}},
		},
		{
			// A here-document body is not a quoted word, so a `$"…"` written
			// in one is body text and no mark at all.
			"not a here-document body", "cat <<EOT\n$\"body\"\nEOT",
			nil,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, markDialect())
			if err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			got := syntax.TranslatedStrings(f)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("\n got %#v\nwant %#v", got, c.want)
			}
		})
	}
}

// The mark is the dialect's to have at all, so a grammar without it records
// none — and the `$` is then an ordinary character, which is what the shells
// that lack the form do with it.
func TestAGrammarWithoutTheMarkRecordsNone(t *testing.T) {
	t.Parallel()
	f, err := syntax.Parse(`echo $"one"`, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := syntax.TranslatedStrings(f); got != nil {
		t.Errorf("got %#v, want none", got)
	}
}

// What the walk does not reach, asserted rather than left to be discovered: a
// substitution's interior is unparsed text here, so a mark written inside one
// is not found. The shell being modeled does find it.
//
// A test rather than a comment because the day the printer learns to reprint
// inside a `$( )` this changes, and the change should be a failing assertion
// rather than a silent improvement nobody checked.
func TestAMarkInsideASubstitutionIsNotReached(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"echo $(echo $\"inside\")",
		"echo ${x-$\"inside\"}",
	} {
		f, err := syntax.Parse(src, markDialect())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if got := syntax.TranslatedStrings(f); got != nil {
			t.Errorf("%q: got %#v — the limit has moved and wants writing down", src, got)
		}
	}
}
