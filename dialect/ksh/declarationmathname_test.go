// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A declaration whose value will not evaluate is the *evaluator's* sentence
// raised through a builtin, and this shell names the builtin in front of it
// and quotes the text back — the same shape it gives `let`.
//
// Measured 2026-09-18 against ksh93u+ 2012-08-01, one-line script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`:
//
//	typeset -i a=1+   s.sh[1]: typeset: 1+: more tokens expected
//	typeset -i a=1/0  s.sh[1]: typeset: 1/0: divide by zero
//	typeset -F a=1+   s.sh[1]: typeset: 1+: more tokens expected
//	float a=1/0       s.sh[1]: typeset: 1/0: divide by zero
//	integer a=1+      s.sh[1]: typeset: 1+: more tokens expected
//	let 1+            s.sh[1]: let: 1+: more tokens expected
//
// The float letters are what this is a test for: they reached a writer of
// their own that named no builtin and quoted no text, so `typeset -F a=1/0`
// was a bare `divide by zero` here (#3342). `integer` and `float` are this
// shell's own spellings of the same builtin and report under its name, which
// is the control saying the name comes from the builtin that ran rather than
// from the word written — they are measured above and asked of the shipped
// binary rather than here, since this package's runner has no prelude.
func TestADeclarationsBadValueIsNamedAndQuotedLikeLets(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"typeset -i a=1+", "typeset: 1+: more tokens expected"},
		{"typeset -i a=1/0", "typeset: 1/0: divide by zero"},
		{"typeset -F a=1+", "typeset: 1+: more tokens expected"},
		{"typeset -F a=1/0", "typeset: 1/0: divide by zero"},
		{"typeset -E a=1/0", "typeset: 1/0: divide by zero"},
		{"let 1+", "let: 1+: more tokens expected"},
	} {
		out, st := runKshWithPrelude(t, c.src)
		if !strings.Contains(out, c.want) {
			t.Errorf("%q: got %q, want it to carry %q", c.src, out, c.want)
		}
		if st == 0 {
			t.Errorf("%q: status 0, want a failure", c.src)
		}
	}
}

// And nobody is named where no builtin ran: the same evaluator reached from an
// assignment to a name already carrying the attribute writes the sentence
// alone, measured as `s.sh: line 1: 1+: more tokens expected`.
func TestAnIntegerAssignmentOutsideADeclarationNamesNoBuiltinHere(t *testing.T) {
	out, _ := runKshWithPrelude(t, "typeset -i a\na=1+\n")
	if !strings.Contains(out, "1+: more tokens expected") {
		t.Errorf("got %q, want the evaluator's sentence", out)
	}
	if strings.Contains(out, "typeset:") {
		t.Errorf("got %q, want no builtin named", out)
	}
}
