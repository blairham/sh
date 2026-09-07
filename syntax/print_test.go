// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// The corpus round trip lives in printroundtrip_test.go, where it compares
// the trees rather than only reading the printed source back — the version
// that stood here could not tell a pattern group from three escaped
// characters (#1221).

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

// A translatable string prints as a plain double-quoted one.
//
// The `$` of `$"..."` contributes nothing to the tree — the spans are exactly
// what a bare `"` produces — so the printed form drops it rather than record a
// byte the tree holds no question for. The assertion is the round trip: the
// printed source must parse to the same tree in the dialect that read the
// original, and it does trivially, because it *is* the plain-quote spelling of
// that tree. It also now parses identically in a dialect without the flag,
// which the original did not.
func TestPrintingATranslatableString(t *testing.T) {
	d := syntax.Core()
	d.DollarDoubleQuote = true
	for _, tc := range []struct{ name, src, want string }{
		{"the dollar is dropped", `echo $"a b"`, `echo "a b"`},
		{"expansions inside survive", `echo $"hi $x"`, `echo "hi $x"`},
		{"mid-word, like any quoting", `echo a$"b c"d`, `echo a"b c"d`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, d)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			printed := strings.TrimRight(syntax.Print(f), "\n")
			if printed != tc.want {
				t.Errorf("printing %q gave %q, want %q", tc.src, printed, tc.want)
			}
			// The round trip, in both dialects: the printed form must mean
			// the same thing wherever it lands.
			for _, rd := range []syntax.Dialect{d, syntax.Core()} {
				second, err := syntax.Parse(printed, rd)
				if err != nil {
					t.Fatalf("printed form %q does not reparse: %v", printed, err)
				}
				if again := strings.TrimRight(syntax.Print(second), "\n"); again != printed {
					t.Errorf("printing is not settled: once %q, twice %q", printed, again)
				}
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
