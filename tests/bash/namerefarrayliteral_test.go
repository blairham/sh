// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// An array literal assigned through a name reference goes where the reference
// points, in both spellings and from inside a call. Measured 2026-09-17 on
// bash 5.3.20 and ksh93u+ (`nameref` there), script files under `env -i`: every
// row below is the target's own array in both, and every one of them wrote
// nowhere at all here.
func TestAnArrayLiteralThroughAReferenceReachesTheTarget(t *testing.T) {
	t.Parallel()
	const decl = "typeset -n "
	for _, c := range []struct{ name, src, want string }{
		{"the whole array", `a=(Z Y); ` + decl + `v=a; v=(n1 n2); echo "[${a[*]}]"`, "[n1 n2]\n"},
		{"appended", `a=(Z Y); ` + decl + `v=a; v+=(app); echo "[${a[*]}]"`, "[Z Y app]\n"},
		{"from inside a call", `a=(Z Y); f() { ` + decl + `p=$1; p+=(gx); }; f a; echo "[${a[*]}]"`, "[Z Y gx]\n"},
		{"a keyed literal replaces the target", `typeset -A m=([x]=1); ` + decl + `q=m; q=([y]=2); echo "[${m[x]-gone}][${m[y]}]"`, "[gone][2]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := answersRun(t, c.src)
			if out != c.want || status != 0 {
				t.Errorf("wrote %q at %d, want %q at 0", out, status, c.want)
			}
		})
	}
}

// A reference with nothing to point at gives the attribute up and takes the
// array itself; one aimed at an *element* refuses, reports 1 and writes
// nothing. Both measured on bash 5.3.20.
func TestAnArrayLiteralThroughAnUnaimedOrElementReference(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"unaimed takes the array", "typeset -n u; u=(a b); echo \"[${u[*]}]\"", "sh: line 1: warning: u: removing nameref attribute\n[a b]\n"},
		{"aimed at an element writes nothing", "typeset -n e=q[0]; e=(a b); echo \"after $? [${q[0]-none}]\"", "sh: line 1: `q[0]': not a valid identifier\nafter 1 [none]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := answersRun(t, c.src)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}
