// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
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

// What the corpus could not reach.
//
// Found by printing every shell script installed on this machine and reading
// it back — 163 of them parse, and nine did not survive the round trip. The
// corpus has 490 snippets and none of them does either of these, because
// nobody writes a case to exercise a printer.
func TestPrintingWhatTheCorpusDoesNotReach(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{
			// A here-document body ends the line, so what closes the block
			// is at the start of the next one and `; }` there is a `;` with
			// nothing before it. Four scripts on this machine close a block
			// straight after a here-document.
			"a block closed after a here-document",
			"f() { cat <<EOF\nbody\nEOF\n}\n",
		},
		{
			"a conditional closed after one",
			"if true; then cat <<EOF\nbody\nEOF\nfi\n",
		},
		{
			"an arm closed after one",
			"case x in a) cat <<EOF\nbody\nEOF\n;; esac\n",
		},
		{
			// Backticks and `$( )` are one node, so every substitution is
			// written the second way — and a backtick one has no space to
			// inherit. `$((` is arithmetic, so a substitution whose first
			// command is a subshell needs one put in: the same trap as a
			// redirection target beginning with `<`, and the reason
			// /opt/homebrew/bin/gettext.sh could not be reprinted.
			"a backtick substitution beginning with a subshell",
			"x=`(echo hi) 2>/dev/null`\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			printed := syntax.Print(f)
			if _, err := syntax.Parse(printed, syntax.Core()); err != nil {
				t.Fatalf("printed source does not parse: %v\n  from: %q\n  gave: %q", err, tc.src, printed)
			}
		})
	}
}

// A dup redirection, which is spelled unlike every other one.
//
// Two rules, and the exact spelling is the whole of what is being tested, so
// these assert the text rather than that it parses. The operator goes tight
// against its target where every other redirection takes a space; and where
// the target is one the reader can resolve — a bare number, or the `-` that
// closes a descriptor — the descriptor being redirected is written out, which
// nothing in the source had to say.
//
// The oracle is `type`: the only place any shell in the panel says a function
// body back, and so the only place a printer can be checked against something
// other than its own opinion.
func TestPrintingADupRedirection(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the descriptor read from is filled in", "echo hi >&2", "echo hi 1>&2"},
		{"and the one read into", "echo hi <&3", "echo hi 0<&3"},
		{"one already written stays as it is", "echo hi 2>&1", "echo hi 2>&1"},
		{"and is not rewritten to the operator's own", "echo hi 3<&0", "echo hi 3<&0"},
		{"a close names its descriptor too", "echo hi >&-", "echo hi 1>&-"},
		{
			// The operator changes and the descriptor does not: `<&-` closes
			// standard input, and is written as a close of it rather than as
			// a close of standard output.
			"a close is written one way whichever asked for it",
			"echo hi <&-", "echo hi 0>&-",
		},
		{"a close that named one keeps it", "echo hi 2>&-", "echo hi 2>&-"},
		{
			// Nothing here can say which descriptor this is: the word is
			// settled when the command runs. Writing one in would be
			// inventing it.
			"a target that is not settled yet gets nothing filled in",
			"echo hi >&$fd", "echo hi >&$fd",
		},
		{
			// Quoting is what tells a descriptor from a file of that name,
			// so a quoted number is not a descriptor to fill in from.
			"a quoted number is not a descriptor",
			"echo hi >&\"1\"", "echo hi >&\"1\"",
		},
		{"nor is a word", "echo hi >&x", "echo hi >&x"},
		{"but the tightness is not conditional on any of that", "echo hi >&/dev/null", "echo hi >&/dev/null"},
		{
			// The operator that redirects both streams ends in the same
			// character and is not a dup: it takes a file, and so takes the
			// space and has no descriptor to fill in.
			"the both-streams operator is not one of these",
			"echo hi &>out", "echo hi &> out",
		},
		{"nor is its appending form", "echo hi &>>out", "echo hi &>> out"},
		{"where a file target keeps the space", "echo hi >out", "echo hi > out"},
		{"and keeps only the descriptor that was written", "echo hi 2>out", "echo hi 2> out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := strings.TrimRight(syntax.Print(f), "\n"); got != tc.want {
				t.Errorf("printing %q gave %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestABackgroundStatementBeforeAClosingWord — the printer wrote the `&` and
// then the `;` separator as well, so `f() { true & }` printed as
// `f() { true &; }`, which parses nowhere. The one file of 1093 installed
// scripts that failed the parse→print→reparse sweep reduced to this.
func TestABackgroundStatementBeforeAClosingWord(t *testing.T) {
	for _, src := range []string{
		"f() { true & }",
		"{ true & }",
		"f() { if true; then true & fi; }",
		"while true; do true & done",
		"( true & )",
		"f() { true & true; }",
		"{ true & true & }",
	} {
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		printed := syntax.Print(f)
		if strings.Contains(printed, "&;") {
			t.Errorf("%q printed as %q: `&;` parses nowhere", src, printed)
		}
		if _, err := syntax.Parse(printed, syntax.Core()); err != nil {
			t.Errorf("%q printed as %q, which does not reparse: %v", src, printed, err)
		}
	}
}
