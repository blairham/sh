// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// A here-document whose body runs to the end of the input inside an open
// construct is what this shell names, at its `<<`, rather than the construct.
// Measured 2026-10-15 against ksh93u+ 2012-08-01 from script files; every
// sentence below is that shell's. See
// syntax.Dialect.UnterminatedHeredocOutranksItsConstruct (#6599).
func TestAnUnterminatedHeredocInsideAConstructIsWhatIsNamed(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if true; then\n:\ncat <<EOF\nabc\n", "syntax error at line 3: `<<' unmatched"},
		{"{\n:\ncat <<EOF\nabc\n", "syntax error at line 3: `<<' unmatched"},
		{"(\n:\ncat <<EOF\nabc\n", "syntax error at line 3: `<<' unmatched"},
		{"while true; do\ncat <<EOF\nabc\n", "syntax error at line 2: `<<' unmatched"},
		{"case a in\na) cat <<EOF\nabc\n", "syntax error at line 2: `<<' unmatched"},
		{"f() {\ncat <<EOF\nabc\n", "syntax error at line 2: `<<' unmatched"},
		{"if cat <<EOF\nabc\n", "syntax error at line 1: `<<' unmatched"},
		{"if true; then\ncat <<EOF &&\nabc\n", "syntax error at line 2: `<<' unmatched"},
		{"if true; then\ncat <<EOF |\nabc\n", "syntax error at line 2: `<<' unmatched"},
		// Opened before the construct, and still the document.
		{"cat <<EOF; if true; then\nabc\n", "syntax error at line 1: `<<' unmatched"},
		// An empty body is a body that began.
		{"if true; then\ncat <<EOF\n", "syntax error at line 2: `<<' unmatched"},
		// Of two, the one that ran out, at its own operator.
		{"if true; then\ncat <<EOF\nabc\nEOF\ncat <<E2\nx\n", "syntax error at line 5: `<<' unmatched"},
		{"if true; then\ncat <<EOF <<E2\nabc\nEOF\n", "syntax error at line 2: `<<' unmatched"},
		// The controls: a document that closed, and an operator with no
		// newline after it, leave the construct to be named.
		{"if true; then\ncat <<EOF\nabc\nEOF\n", "syntax error at line 5: `then' unmatched"},
		{"if true; then\ncat <<EOF", "syntax error at line 2: `then' unmatched"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// At the top level nothing is open around the document, and an unfinished
// body is the body: every shell in the panel runs it.
func TestAnUnterminatedHeredocAtTheTopLevelStillParses(t *testing.T) {
	if _, err := syntax.Parse("cat <<EOF\nabc\n", ksh.Dialect()); err != nil {
		t.Errorf("got %v, want it read", err)
	}
}

// A document inside a substitution is read by the substitution's own lexer,
// and it is still the document that is named where the substitution sits in
// an open construct — while at the top level, or after a list operator, the
// substitution is. Measured 2026-10-15 against ksh93u+ 2012-08-01 from script
// files (#6706).
func TestAnUnterminatedHeredocInsideASubstitutionInsideAConstructIsNamed(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if true; then\nx=$(cat <<EOF\nabc\n", "syntax error at line 2: `<<' unmatched"},
		{"{\nx=$(cat <<EOF\nabc\n", "syntax error at line 2: `<<' unmatched"},
		{"while true; do\nx=$(cat <<EOF\nabc\n", "syntax error at line 2: `<<' unmatched"},
		{"case a in a)\nx=$(cat <<EOF\nabc\n", "syntax error at line 2: `<<' unmatched"},
		{"for i in $(cat <<EOF\nabc\n", "syntax error at line 1: `<<' unmatched"},
		{"if true; then\nx=\"$(cat <<EOF\nabc\n", "syntax error at line 2: `<<' unmatched"},
		{"if true; then\ncat <(cat <<EOF\nabc\n", "syntax error at line 2: `<<' unmatched"},
		{"if true; then\nx=${ cat <<EOF\nabc\n", "syntax error at line 2: `<<' unmatched"},
		{"[[ $(cat <<EOF\nabc\n", "syntax error at line 1: `<<' unmatched"},
		// At the document's own line, not the substitution's.
		{"if true; then\nx=$(\n:\ncat <<EOF\nabc\n", "syntax error at line 4: `<<' unmatched"},
		{"if true; then\nx=$(cat <<EOF\n", "syntax error at line 2: `<<' unmatched"},
		// The controls: nothing open around it, a list operator, a body that
		// never began, a document that closed, and no document at all.
		{"x=$(cat <<EOF\nabc\n", "syntax error at line 1: `(' unmatched"},
		{"x=$(if true; then cat <<EOF\nabc\n", "syntax error at line 1: `(' unmatched"},
		{"true &&\nx=$(cat <<EOF\nabc\n", "syntax error at line 2: `(' unmatched"},
		{"if true; then\nx=$(cat <<EOF", "syntax error at line 2: `(' unmatched"},
		{"if true; then\nx=$(cat <<EOF\nabc\nEOF\n", "syntax error at line 2: `(' unmatched"},
		{"if true; then\nx=$(echo\nabc\n", "syntax error at line 2: `(' unmatched"},
		{"[[ $(echo\nabc\n", "syntax error at line 1: `(' unmatched"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
		// And the position is the operator's in this text, not in the
		// substitution's.
		var se *syntax.Error
		if strings.Contains(tc.want, "<<") && errors.As(err, &se) && int(se.Pos.Offset) != strings.Index(tc.src, "<<") {
			t.Errorf("%q: at offset %d, want %d", tc.src, se.Pos.Offset, strings.Index(tc.src, "<<"))
		}
	}
}
