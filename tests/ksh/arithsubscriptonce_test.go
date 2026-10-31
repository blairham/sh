// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// TestASubscriptOnAnUnsetNameIsEvaluatedOnce: the subscript is arithmetic,
// and a name holding nothing runs it once. Measured 2026-10-31 on ksh93u+
// under `-c`, each row is `1`; it was 2 here, the subscript evaluated once for
// its refusal and again for the reach past the first element (#7189).
func TestASubscriptOnAnUnsetNameIsEvaluatedOnce(t *testing.T) {
	for _, src := range []string{
		`i=0; : ${x[i++]}; echo $i`,
		`i=0; : ${x[i++]:-d}; echo $i`,
		`i=0; : ${!x[i++]}; echo $i`,
	} {
		if out, _ := runKsh(t, t.TempDir(), src); out != "1\n" {
			t.Errorf("%s\n got %q, want %q", src, out, "1\n")
		}
	}
}

// TestAFailedExpansionInAnArithmeticCommandIsTheExpansions: the shell ends at
// 1 over the division, and the construct whose text did not expand is never
// traced — where its own failure, `(( b = 1/0 ))`, is traced before the shell
// ends. Measured 2026-10-31 on ksh93u+ under `-c`. See
// interp.Semantics.FailedExpansionEscapesAnArithmeticCommand (#7189).
func TestAFailedExpansionInAnArithmeticCommandIsTheExpansions(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"(( b = $((1/0)) + 1 )); echo \"same=$?\"\necho next", "ksh: 1/0: divide by zero\n"},
		{"set -x; (( b = $((1/0)) + 1 )); echo same\necho next", "ksh: 1/0: divide by zero\n"},
		{"readonly a; (( b = 15 + ${a[1]:=x}0 )); echo same\necho next", "ksh: a: is read only\n"},
		// The control: the construct's own failure is traced first.
		{"set -x; (( b = 1/0 )); echo same\necho next", "+ (( b = 1/0 ))\nksh:  b = 1/0 : divide by zero\n"},
	} {
		out, st := runKsh(t, t.TempDir(), c.src)
		if out != c.want || st != 1 {
			t.Errorf("%s\n got %q (status %d)\nwant %q (status 1)", c.src, out, st, c.want)
		}
		if strings.Contains(c.src, "$((1/0))") && strings.Contains(out, "+ ((") {
			t.Errorf("%s\n got %q, traced a construct whose text did not expand", c.src, out)
		}
	}
}
