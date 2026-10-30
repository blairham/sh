// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// An assignment through a name reference is traced under the variable it lands
// in, where the store reaches the reference's variable at all. Measured
// 2026-10-30 against ksh93u+ 2012-08-01 (`/bin/ksh`), from `-c` and script
// files under `env -i PATH=/usr/bin:/bin`. This shell wrote the word as the
// script spelled it (#6991).
//
// A regular builtin's prefix and a `function`-form call's are cells of the
// command's own, which no reference reaches, and keep the word: those are
// the controls. See interp/xtracenameref.go.
func TestAnAssignmentThroughAReferenceIsTracedUnderItsVariable(t *testing.T) {
	const refs = `typeset -A a; x=0; typeset -n n=a m=x; `
	for _, c := range []struct{ name, src, want string }{
		{"a prefix", refs + `set -x; m=1 :`, "+ x=1\n+ :\n"},
		{"a prefix element", refs + `set -x; n[k]=1 :`, "+ a[k]=1\n+ :\n"},
		{"a bare assignment", refs + `set -x; m=1; m+=2`, "+ x=1\n+ x+=2\n"},
		{"a bare element", refs + `set -x; n[k]=1`, "+ a[k]=1\n"},
		{"a literal", refs + `set -x; m=(1 2)`, "+ x=( 1 2 )\n"},
		{"two references", refs + `typeset -n l=m; set -x; l=5`, "+ x=5\n"},
		{"a reference to an element", `typeset -A a; typeset -n e='a[x y]'; set -x; e=1`, "+ a['x y']=1\n"},
		{"an indexed element", `typeset -a b=(1 2); typeset -n e=b; set -x; e[1+1]=w`, "+ b[2]=w\n"},
		{"a POSIX-form function", refs + `f() { :; }; set -x; m=1 f`, "+ x=1\n+ f\n+ :\n"},
		{"a program", refs + `set -x; m=1 /usr/bin/true`, "+ /usr/bin/true\n+ x=1\n"},
		{"a program through command", refs + `set -x; m=1 command /usr/bin/true`, "+ command /usr/bin/true\n+ x=1\n"},
		{"a substitution", refs + `set -x; m=$(echo v) :`, "+ echo v\n+ x=v\n+ :\n"},
		// Controls.
		{"a regular builtin", refs + `set -x; m=2 true`, "+ true\n+ m=2\n"},
		{"a regular builtin through command", refs + `set -x; m=1 command echo hi`, "+ command echo hi\n+ m=1\nhi\n"},
		{"a function-form call", refs + `function g { :; }; set -x; m=1 g`, "+ g\n+ m=1\n"},
		{"no reference", `x=0; set -x; x=1 :`, "+ x=1\n+ :\n"},
		// A reference aimed at nothing is aimed by its first store, and the
		// line names the reference as it stood before that store.
		{"a reference aimed at nothing", `typeset -n u; set -x; u=y; u=z`, "+ u=y\n+ y=z\n"},
		{"a prefix through a reference aimed at nothing", `typeset -n u; set -x; u=y :`, "+ u=y\n+ :\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
