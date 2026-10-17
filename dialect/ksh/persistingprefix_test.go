// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A prefix that persists makes a name carrying a numeric or a width letter a
// plain scalar, export and all; a `function`-form call is handed a fresh
// exported cell with no letters. Measured 2026-10-16 on ksh93u+ under `-c` —
// see Semantics.PersistingPrefixMakesATypedNameAPlainScalar (#6750).
func TestAPersistingPrefixMakesATypedNameAPlainScalar(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -i v=1; v=2+2 :; typeset -p v`, "v=2+2\n"},
		{`typeset -i v=1; f(){ :; }; v=2+2 f; typeset -p v`, "v=2+2\n"},
		{`typeset -i v=1; v=2+2 eval "typeset -p v"`, "v=2+2\n"},
		{`typeset -xi v=1; v=5 :; typeset -p v`, "v=5\n"},
		{`typeset -R5 v=x; v=ab :; typeset -p v`, "v=ab\n"},
		{`typeset -F 2 v; v=1/3 :; echo $v`, "1/3\n"},
		{`typeset -i v=1; v=1+ :; echo "st=$? [$v]"`, "st=0 [1+]\n"},
		{`typeset -i v=1; v=2+2 export v; typeset -p v`, "typeset -x v=2+2\n"},
		{`typeset -i v=1; function g { typeset -p v; }; v=2+2 g`, "typeset -x v=2+2\n"},
		{`typeset -u v=x; function g { typeset -p v; }; v=abc g`, "typeset -x v=abc\n"},
		// The controls: a case letter alone keeps the store going through
		// it, and a regular builtin's prefix is given back.
		{`typeset -u v=x; v=abc :; typeset -p v`, "typeset -u v=ABC\n"},
		{`typeset -i v=1; v=2+2 true; typeset -p v`, "typeset -i v=1\n"},
		{`typeset -i x=2; x+=5 :; typeset -p x`, "typeset -i x=7\n"},
	} {
		out, _ := runKsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
