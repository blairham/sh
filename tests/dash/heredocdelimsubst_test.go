// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// What this shell accepts in a here-document's **delimiter**, and what its
// refusals say (#4691).
//
// Nothing in a delimiter expands here or anywhere, so what is in question is
// whether the parser takes the shapes an expansion is written in. This shell
// reads the unquoted part of a delimiter as plain text, which shows up twice:
// a `$(` leaves its `(` standing where no word may have one, and a backquote
// does not protect a blank.
//
// Measured 2026-09-27 against dash 0.5.12 under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`, `-c`, the program being `: <<$D` / `body` / `END` / `echo alive`;
// `go version -m` on the binary: *not a Go executable*. BusyBox ash answers
// every row identically, measured the same day in the pinned image — see
// dialect/ash.
func TestAHeredocDelimiterTakesNoUnquotedSubstitution(t *testing.T) {
	for _, tc := range []struct {
		name, delim string
		want        bool
	}{
		{"an unquoted command substitution", "$(echo E)", false},
		{"the two characters and not the expression", "$((1))", false},
		{"a command substitution inside a word", "a$(echo E)b", false},
		{"a quoted command substitution is taken", `"$(echo E)"`, true},
		{"a backquoted word holding a blank", "`a b`", false},
		{"a backquoted word with no blank in it", "`a`", true},
		{"a quoted backquoted word", "\"`a b`\"", true},
		{"a braced parameter", "${v}", true},
		{"a bare parameter", "$v", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := ": <<" + tc.delim + "\nbody\nEND\necho alive\n"
			if got := parses(t, src); got != tc.want {
				t.Errorf("parses(%q) = %v, want %v", src, got, tc.want)
			}
		})
	}
}

// And what each of the two refusals says, which are different sentences
// because they are different failures: one is a token where no word may have
// one, the other is input that ran out with a construct still open.
func TestADelimiterSubstitutionIsRefusedInThisShellsWords(t *testing.T) {
	for _, tc := range []struct {
		name, delim, want string
		line              int
	}{
		{
			"an unquoted command substitution names the parenthesis",
			"$(echo E)", `Syntax error: "(" unexpected`, 1,
		},
		{
			// The word ended at the blank and left the backquote open, so
			// the rest of the input went into it. Reported where the input
			// ran out rather than where the delimiter is — line 5 for this
			// four-line program because it ends in a newline, and the
			// reference says 5 for the same bytes and 4 without it.
			"a backquote the word ended in front of runs to the end",
			"`a b`", "Syntax error: EOF in backquote substitution", 5,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := ": <<" + tc.delim + "\nbody\nEND\necho alive\n"
			_, err := syntax.Parse(src, dash.Dialect())
			if err == nil {
				t.Fatalf("%q parsed, want a refusal", src)
			}
			d := dash.Diagnostics()
			if got := d.ParseDiagnostic("f.sh", "", err, src); !strings.Contains(got, tc.want) {
				t.Errorf("%q said %q, want it to hold %q", src, got, tc.want)
			}
			if got := d.ParseFailureLine(err); got != tc.line {
				t.Errorf("%q is on line %d, want %d", src, got, tc.line)
			}
		})
	}
}

// And the value itself, so a preset mutant dies here even where a row above
// could be satisfied by some other change.
func TestHeredocDelimiterSubstitutionsAnswer(t *testing.T) {
	got := dash.Dialect().HeredocDelimiterSubstitutions
	if want := syntax.HeredocDelimiterScansNoUnquotedSubstitution; got != want {
		t.Errorf("HeredocDelimiterSubstitutions = %v, want %v", got, want)
	}
}
