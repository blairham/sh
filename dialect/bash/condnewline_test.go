// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// A condition term whose first word has been read and whose shape is not yet
// settled will not take a newline here either, and this shell words that one
// position as a statement about the operator it was still waiting for rather
// than as a token the grammar did not want.
//
// Measured 2026-09-18 on bash 5.3.20 and on bash 3.2.57, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`; both builds agree:
//
//	[[ y      →  unexpected token `newline', conditional binary operator expected
//	]]           syntax error near `y'
//	             `[[ y'
//
// against `[[ -n x` over the same two lines, which runs. The first line is
// what is asserted here; the two after it are the generic echo this shell
// writes for any refused token and are left as they stand (#3627).
func TestATermsFirstWordWillNotEndItsLineHere(t *testing.T) {
	d := bash.Dialect()
	if d.ConditionNewlineMayFollowATermsFirstWord {
		t.Error("ConditionNewlineMayFollowATermsFirstWord is true, want false")
	}
	for _, src := range []string{"[[ y\n]]\n", "[[ y\n&& -n z ]]\n", "[[ ( y\n) ]]\n"} {
		_, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile))
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", src)
			continue
		}
		got := bash.Diagnostics().ForScript().ParseDiagnostic("s.sh", "", err, src)
		if want := "unexpected token `newline', conditional binary operator expected"; !strings.Contains(got, want) {
			t.Errorf("%q:\n got %q\nwant it to carry %q", src, got, want)
		}
	}
	// The control: a finished term takes the newline, so the sentence above
	// belongs to the undecided position and not to conditions with newlines.
	for _, src := range []string{"[[ -n x\n]]\n", "[[ y == z\n]]\n"} {
		if _, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile)); err != nil {
			t.Errorf("parse %q: %v", src, err)
		}
	}
}
