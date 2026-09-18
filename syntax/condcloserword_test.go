// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// Two readings of a `]]` standing where a condition **term** belongs, named
// by the flags and never by a shell — see
// [Dialect.ConditionCloserIsAWordWhereATermBegins] and
// [Dialect.ConditionTermMissingBlamesTheTokenAfterTheCloser] for where the
// rows were measured.

// TestTheCloserIsAWordWhereATermBeginsWhereTheDialectSaysSo. With the flag on
// the text parses: the two characters are an ordinary word, and the condition
// is whatever the grammar makes of it. With it off the token is refused.
func TestTheCloserIsAWordWhereATermBeginsWhereTheDialectSaysSo(t *testing.T) {
	t.Parallel()
	word := Core()
	word.ConditionCloserIsAWordWhereATermBegins = true
	for _, tc := range []struct {
		src string
		why string
	}{
		// The five places a term may begin.
		{`[[ ]] ]]`, "straight after the opener"},
		{`[[ ! ]] ]]`, "after a negation"},
		{`[[ x && ]] ]]`, "after a conjunction"},
		{`[[ x || ]] ]]`, "after a disjunction"},
		{`[[ ( ]] ) ]]`, "inside a group"},
		// And the word standing in a condition of its own shape.
		{`[[ ]] == x ]]`, "as a comparison's left operand"},
		{`[[ ]] && x ]]`, "joined by a connective"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if _, err := Parse(tc.src, word); err != nil {
				t.Errorf("%s: %v — the closer is a word %s", tc.src, err, tc.why)
			}
			if _, err := Parse(tc.src, Core()); err == nil {
				t.Errorf("%s: parsed with the flag off, where the token is refused", tc.src)
			}
		})
	}
	// An **operand**'s position is a separate question and the flag does not
	// reach it: every column takes the closer as the closer behind an
	// operator, so these stay refused either way.
	for _, src := range []string{`[[ -n ]]`, `[[ x == ]]`, `[[ x -eq ]]`} {
		if _, err := Parse(src, word); err == nil {
			t.Errorf("%s: parsed, where the closer behind an operator is still the closer", src)
		}
	}
}

// TestACondTermMissingMayBlameTheTokenAfterTheCloser. The closer is consumed
// and the complaint falls on whatever stands behind it — the *token*, which
// is what this level is responsible for. Where and how it is then worded is
// the dialect's, and dialect/zsh asserts the sentences over every route.
func TestACondTermMissingMayBlameTheTokenAfterTheCloser(t *testing.T) {
	t.Parallel()
	after := Core()
	after.ConditionTermMissingBlamesTheTokenAfterTheCloser = true
	for _, tc := range []struct {
		src, want string
	}{
		{`[[ ]] == x ]]`, "=="},
		{`[[ ]]; echo after`, ";"},
		{`[[ ]] echo after`, "echo"},
		// Blank lines are passed over, so the complaint lands on the next
		// real token rather than on the first newline.
		{"[[ ]]\necho after", "echo"},
		{"[[ ]]\n\n\necho after", "echo"},
		// A newline the input ends with is kept, since there is nothing else
		// to name and the closer is two tokens back by then.
		{"[[ ]]\n", "newline"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			var e *Error
			_, err := Parse(tc.src, after)
			if !errors.As(err, &e) {
				t.Fatalf("%s: %v is not a parse error", tc.src, err)
			}
			if e.Token != tc.want {
				t.Errorf("%s: named %q, want %q", tc.src, e.Token, tc.want)
			}
			// And with the flag off the **closer** is named on every one of
			// them, which is the other column's answer.
			_, err = Parse(tc.src, Core())
			if !errors.As(err, &e) {
				t.Fatalf("%s: with the flag off, %v is not a parse error", tc.src, err)
			}
			if e.Token != "]]" {
				t.Errorf("%s: with the flag off, named %q, want %q", tc.src, e.Token, "]]")
			}
		})
	}
	// Nothing behind the closer is the one shape the two readings cannot be
	// told apart by the token: the parse that consumed it is at the end of
	// the input with nothing to name, and the parse that did not names the
	// `]]`. Both come out as a complaint about the closer, since a wording
	// with no token falls back to the one that was expected — which is why
	// the four routes of `[[ ]]` alone split two ways rather than four, and
	// why the route that splits them is a file with a final newline.
	var e *Error
	if _, err := Parse(`[[ ]]`, after); !errors.As(err, &e) || e.Token != "" || e.Expected != "]]" {
		t.Errorf("at the end of the input: token %q expected %q, want no token and the closer expected",
			e.Token, e.Expected)
	}
	if _, err := Parse(`[[ ]]`, Core()); !errors.As(err, &e) || e.Token != "]]" {
		t.Errorf("at the end of the input with the flag off: named %q, want the closer", e.Token)
	}
}
