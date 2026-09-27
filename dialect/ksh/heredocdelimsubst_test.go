// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// A `$( )` written in a here-document's **delimiter** is refused here however
// it is written, and nothing else in a delimiter is (#4691).
//
// Measured 2026-09-27 against ksh93u+ 2012-08-01 under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, `-c`, the program being `: <<$D` / `body` /
// `END` / `echo alive`; `go version -m` on the binary: *not a Go executable*.
// The quoted row is what parts this rule from dash's, which takes a quoted
// substitution and refuses an unquoted one — so the two refusing columns
// needed two answers rather than one flag.
func TestACommandSubstitutionInAHeredocDelimiterIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name, delim string
		want        bool
	}{
		{"an unquoted command substitution", "$(echo E)", false},
		{"a command substitution inside a word", "a$(echo E)b", false},
		{"a quoted command substitution, which parts this from dash", `"$(echo E)"`, false},
		{"an arithmetic expansion is not one", "$((1))", true},
		{"a backquoted substitution is not one either", "`a b`", true},
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

// The sentence is this shell's here-document one — the same it says about a
// here-document a substitution could not feed — and the status is 3.
//
// **The token it quotes is approximated**, and deliberately. The reference
// quotes `<<` and a string taken from inside the substitution at an offset
// that moves with the text: `$(echo E)` comes back as “ `<<E' “,
// `$(export q)` as “ `<<rt' “ — a suffix of `export` — and `$(a=1 b c)` as
// “ `<<=1' “, which is not a word of the program at all. A quotation
// landing in the middle of a word is an artifact of that parser's own buffer
// rather than a fact about the language, and reproducing it would mean
// deciding what it had in hand. So what is written is `<<` and the delimiter,
// which is the convention every other here-document refusal here follows.
func TestARefusedDelimiterSubstitutionIsTheHeredocSentence(t *testing.T) {
	src := ": <<$(echo E)\nbody\nEND\necho alive\n"
	_, err := syntax.Parse(src, ksh.Dialect())
	if err == nil {
		t.Fatalf("%q parsed, want a refusal", src)
	}
	d := ksh.Diagnostics()
	got := d.ParseDiagnostic("f.sh", "", err, src)
	for _, want := range []string{
		"syntax error at line 1:",
		"here-document not contained within command substitution",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("said %q, want it to hold %q", got, want)
		}
	}
	if got := d.ParseFailureLine(err); got != 1 {
		t.Errorf("line = %d, want 1 — the operator's own", got)
	}
}

// And the value itself, so a preset mutant dies here even where a row above
// could be satisfied by some other change.
func TestHeredocDelimiterSubstitutionsAnswer(t *testing.T) {
	got := ksh.Dialect().HeredocDelimiterSubstitutions
	if want := syntax.HeredocDelimiterRefusesACommandSubstitution; got != want {
		t.Errorf("HeredocDelimiterSubstitutions = %v, want %v", got, want)
	}
}
