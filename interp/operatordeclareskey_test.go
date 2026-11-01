// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// AssignOrErrorOperatorDeclaresTheKey: Yes has `${m[k]=x}` and `${m[k]?e}`
// make a key the table has not got, declared and holding nothing, so the
// colon-less test finds it set and neither assigns nor refuses; No leaves the
// key unset for both operators to act on (#7187).
func TestAnAssignOrErrorOperatorAsksWhetherItMakesTheKey(t *testing.T) {
	t.Parallel()
	const src = `typeset -A m; m[j]=1; echo "<${m[k]=x}>"; echo "[${m[k]-u}][${#m[@]}]"; echo "<${m[q]?e}>" "${#m[@]}"`
	for _, tc := range []struct {
		a      Answer
		want   string
		refuse bool
	}{
		{Yes, "<>\n[][2]\n<> 3\n", false},
		{No, "<x>\n[x][2]\n", true},
	} {
		s := testSemantics()
		s.AssignOrErrorOperatorDeclaresTheKey = tc.a
		out, st := run(t, src, withSem(s))
		if !strings.HasPrefix(out, tc.want) || (st != 0) != tc.refuse {
			t.Errorf("answered %v: got %q status %d, want %q, refused %v", tc.a, out, st, tc.want, tc.refuse)
		}
		if tc.refuse && !strings.Contains(out, "m[q]: e") {
			t.Errorf("answered %v: got %q, want the missing key refused", tc.a, out)
		}
	}
}

// The question is put only for `=` and `?` on a key that is missing from a
// stored table: a key that is there, `:=`, `-` and `+` never raise it, so a
// vector that leaves it open still runs them.
func TestOnlyAMissingKeyUnderTheOperatorAsksWhetherItIsMade(t *testing.T) {
	t.Parallel()
	s := testSemantics()
	s.AssignOrErrorOperatorDeclaresTheKey = Unspecified
	const src = `typeset -A m; m[k]=1; : ${m[k]=x} ${m[k]?e} ${m[q]:=v} ${m[z]-d} ${m[y]+p}; echo "${m[k]} ${m[q]} ${#m[@]}"`
	if out, st := run(t, src, withSem(s)); out != "1 v 2\n" || st != 0 {
		t.Errorf("got %q status %d, want the expansions made and nothing asked", out, st)
	}
}
