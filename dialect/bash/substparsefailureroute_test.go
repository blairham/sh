// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	bash "github.com/blairham/sh/dialect/bash"
)

// A `$( … )` body that will not parse costs **127** when the program came
// from a `-c` string, where the identical text from a file or from standard
// input costs the refusal's own 2.
//
// Three numbers for one failure, and this shell is the only column with more
// than one. Measured 2026-09-26 under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with stdout and stderr discarded and `$?` taken immediately, against
// /opt/homebrew/bin/bash 5.3.20 (`not a Go executable` by `go version -m`)
// and `cmd/bash` built under its own name:
//
//	-c  echo $(echo hi; for)            127
//	-c  ( echo $(for) )                 127
//	-c  eval "echo $(for)"              127
//	-c  if — a plain syntax error         2
//	-c  echo $(cat 10 — input ran out     2
//	file / stdin, the same three lines    2
//
// See interp.Diagnostics.SubstitutionParseFailureStatusFromCommandString for
// the whole grid and for what each control rules out (#4697).
func TestARefusedSubstitutionBodyCosts127FromACommandStringHere(t *testing.T) {
	t.Parallel()
	if got := bash.Diagnostics().SubstitutionParseFailureStatusFromCommandString; got != 127 {
		t.Errorf("= %d, want 127", got)
	}
	// And the number is not the syntax status wearing another hat: the two
	// are different here, which is what makes the row worth a field.
	if got := bash.Diagnostics().SyntaxStatus(); got == 127 {
		t.Errorf("the syntax status is %d, so the route's answer cannot be told from it", got)
	}
}
