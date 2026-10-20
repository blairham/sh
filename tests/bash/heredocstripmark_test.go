// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `declare -f` **keeps** the `<<-`, which is the other side of
// syntax.Layout.HereDocumentStripMarkOmitted and the reason it is a flag
// rather than a fix.
//
// The body is written back at column 0 here exactly as it is in zsh — the
// tabs are gone — and the mark is written anyway. So the two columns agree
// about the body and disagree about the operator, which is what the flag
// carries.
//
// Measured 2026-09-29 on bash 5.3.20, the fixture written with real tabs.
func TestDeclareKeepsTheHereDocumentStripMark(t *testing.T) {
	out, errs := runBashSplit(t, "f() {\n\tcat <<-x\n\tfoo\n\tx\n}\ndeclare -f f\n")
	if errs != "" {
		t.Fatalf("stderr %q", errs)
	}
	if !strings.Contains(out, "<<-x") {
		t.Errorf("listing %q, want the mark kept", out)
	}
	// And the body is stripped all the same, which is what makes the mark
	// the only thing the two columns differ about.
	if !strings.Contains(out, "\nfoo\n") {
		t.Errorf("listing %q, want the body at column 0", out)
	}
}
