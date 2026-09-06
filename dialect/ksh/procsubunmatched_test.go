// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// This shell is the reason an unmatched process substitution is a wording of
// its own rather than an argument the command-substitution one takes: it
// reaches a different *diagnosis*, not a different sentence. `$(` is an
// unmatched parenthesis named at the line the opener is on; `<(` is the end
// of the file named at the line the file ended on. Measured against ksh93u+
// over a script — `cat <(echo hi` answers the second and `v=$(echo hi` the
// first, and no wording of one can be the other.
//
// Both are here together for that reason. A test of `<(` alone would pass a
// change that made `$(` say the same thing.
func TestAnUnmatchedProcessSubstitutionNamesTheEndOfTheFile(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"cat <(echo hi\necho after\n", "syntax error at line 3: `end of file' unexpected"},
		{"cat >(echo hi\necho after\n", "syntax error at line 3: `end of file' unexpected"},
		{"v=$(echo hi\necho after\n", "syntax error at line 1: `(' unmatched"},
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
