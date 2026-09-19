// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// A condition term whose first word has been read and whose shape is not yet
// settled will not take a newline here, and the refusal names the **newline**
// rather than whatever stands behind it.
//
// `[[ ]]` reaches that position because this shell reads a `]]` standing
// where a term begins as an ordinary word — see
// syntax.Dialect.ConditionCloserIsAWordWhereATermBegins — so the two shapes
// are one question, which is what #3627 was filed to say about #2964's
// reading.
//
// Measured 2026-09-18 against ksh93u+ 2012-08-01, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin on /dev/null:
//
//	[[ ]] then `echo after`          line 2: `newline' unexpected
//	[[ ]] then three blanks, echo    line 5: `newline' unexpected
//	[[ ]] with a final newline       line 2: `newline' unexpected
//	[[ x then a blank line           line 3: `newline' unexpected
//	[[ y then `]]` on the next line   line 2: `newline' unexpected
//
// Line 5 and line 3 are what say the complaint is located at the **last** of
// the blank lines rather than the first.
func TestATermsFirstWordWillNotEndItsLineHere(t *testing.T) {
	d := ksh.Dialect()
	if d.ConditionNewlineMayFollowATermsFirstWord {
		t.Error("ConditionNewlineMayFollowATermsFirstWord is true, want false")
	}
	for _, c := range []struct{ src, want string }{
		{"[[ ]]\necho after\n", "syntax error at line 2: `newline' unexpected"},
		{"[[ ]]\n\n\n\necho after\n", "syntax error at line 5: `newline' unexpected"},
		{"[[ ]]\n", "syntax error at line 2: `newline' unexpected"},
		{"[[ x\n\n", "syntax error at line 3: `newline' unexpected"},
		{"[[ y\n]]\necho ok\n", "syntax error at line 2: `newline' unexpected"},
		{"[[ y\n&& -n z ]]\n", "syntax error at line 2: `newline' unexpected"},
		{"[[ ( y\n) ]]\n", "syntax error at line 2: `newline' unexpected"},
	} {
		_, err := syntax.Parse(c.src, d.On(syntax.RouteFromScriptFile))
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", c.src)
			continue
		}
		if got := ksh.Diagnostics().ForScript().ParseFailure(err); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// The two controls that say it is this position and not newlines in a
// condition: a term that is finished takes one, and the same text with no
// final newline is the unmatched `[[` both columns write.
func TestAFinishedTermAndAMissingNewlineAreUnmovedHere(t *testing.T) {
	d := ksh.Dialect()
	for _, src := range []string{"[[ -n x\n]]\n", "[[ y == z\n]]\n", "[[ ( -n x )\n]]\n"} {
		if _, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile)); err != nil {
			t.Errorf("parse %q: %v", src, err)
		}
	}
	_, err := syntax.Parse("[[ ]]", d.On(syntax.RouteFromScriptFile))
	if err == nil {
		t.Fatal("`[[ ]]` with no final newline parsed, want a refusal")
	}
	if got, want := ksh.Diagnostics().ForScript().ParseFailure(err),
		"syntax error at line 1: `[[' unmatched"; got != want {
		t.Errorf("no final newline:\n got %q\nwant %q", got, want)
	}
}
