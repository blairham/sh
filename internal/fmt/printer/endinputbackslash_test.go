// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package printer_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/fmt/comments"
	"github.com/blairham/sh/internal/fmt/printer"
	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/syntax"
)

// A program that ends in a backslash the input ended after gets no final
// newline, because the newline would change what it says (#2680).
//
// `printf x\` is the two words `printf` and `x\`. Put a newline behind that
// backslash and it is an ordinary line continuation instead, joining the word
// to nothing and taking the backslash with it — two words where there were
// three. So the one text this formatter cannot end with a newline is this
// one, alongside the here-document body that runs to end of file.
//
// The alternative was to respell the word as `x\\`, which is the same tree
// and does take a newline. It is what [syntax.Print] does, and it is not
// available here: this package emits every token by its source extent and
// never respells a word. The newline is the part the printer owns, so the
// newline is what gives way.
//
// These cases were unreachable until #2680: a trailing backslash was a
// refusal, so no such program ever reached the formatter. Three corpus rows
// found it the hour it stopped being one.
func TestAProgramEndingInABackslashGetsNoFinalNewline(t *testing.T) {
	for _, c := range []struct {
		name, src string
		newline   bool
	}{
		{"a backslash inside a word", `printf '[%s]' x\`, false},
		{"a backslash that is the whole word", `printf '[%s]' a \`, false},
		{
			// An escaped backslash is a pair, so nothing is left unpaired
			// and the newline is safe. Same last byte, opposite answer.
			"an escaped backslash", `printf '[%s]' x\\`, true,
		},
		{
			// Three: the pair, then one the input ends after.
			"an escaped backslash and one more", `printf '[%s]' x\\\`, false,
		},
		{
			// The byte is the quote, not the backslash, so there is nothing
			// at the end for a newline to turn into a continuation.
			"a backslash inside single quotes", `printf '[%s]' 'x\'`, true,
		},
		{"nothing unusual", `printf '[%s]' a b`, true},
	} {
		d := oracle.Dialect()
		in, err := syntax.Parse(c.src, d)
		if err != nil {
			t.Errorf("%s: did not parse: %v", c.name, err)
			continue
		}
		out := printer.Format(c.src, in, comments.Recover(c.src, in), syntax.CoreStyle())
		if got := strings.HasSuffix(out, "\n"); got != c.newline {
			t.Errorf("%s: %q ends in a newline: %v, want %v", c.name, out, got, c.newline)
		}
		// The promise the newline was in the way of: whatever comes out is
		// the same program. A test on the newline alone would pass for a
		// formatter that dropped it and respelled the word into something
		// else entirely.
		after, err := syntax.Parse(out, d)
		if err != nil {
			t.Errorf("%s: output %q does not parse: %v", c.name, out, err)
			continue
		}
		if why, ok := syntax.SameProgram(in, after); !ok {
			t.Errorf("%s: %q is a different program: %s", c.name, out, why)
		}
	}
}

// A line continuation that really is one keeps the newline, which is what
// says the rule above reads the text and not the last byte.
//
// `printf a \` with a newline after it is a continuation to nothing: the
// backslash and the newline are both gone by the time there is a tree, so
// nothing is left at the end to protect and the formatter ends the file the
// way it ends every other one.
func TestARealLineContinuationStillEndsInANewline(t *testing.T) {
	d := oracle.Dialect()
	for _, src := range []string{"printf '[%s]' a \\\n", "echo a \\\n  b"} {
		in, err := syntax.Parse(src, d)
		if err != nil {
			t.Errorf("%q: did not parse: %v", src, err)
			continue
		}
		out := printer.Format(src, in, comments.Recover(src, in), syntax.CoreStyle())
		if !strings.HasSuffix(out, "\n") {
			t.Errorf("%q: formatted to %q, which does not end in a newline", src, out)
		}
	}
}
