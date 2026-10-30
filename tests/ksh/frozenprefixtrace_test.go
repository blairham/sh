// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A frozen prefix entry's refusal stands where the per-entry trace has got to:
// the entries ahead of it are written, and nothing behind it is. Measured
// 2026-10-30 against ksh93u+ 2012-08-01 (`/bin/ksh`), from `-c` and from
// script files under `env -i PATH=/usr/bin:/bin`. In front of a special
// builtin or a function this shell refused ahead of every line, and in front
// of a program it wrote every entry and refused after them (#6987).
func TestAFrozenPrefixEntrysRefusalStandsInTheTrace(t *testing.T) {
	const ro = `readonly r=1; `
	for _, c := range []struct{ name, src, want string }{
		{"a special builtin", ro + `set -x; x=1 r=2 y=3 :`, "+ x=1\nksh: r: is read only\n"},
		{"eval", ro + `set -x; x=1 r=2 y=3 eval :`, "+ x=1\nksh: r: is read only\n"},
		{"a function", ro + `f() { :; }; set -x; x=1 r=2 y=3 f`, "+ x=1\nksh: r: is read only\n"},
		{"through command", ro + `set -x; x=1 r=2 y=3 command :`, "+ x=1\nksh: r: is read only\n"},
		{
			"a table's empty key ahead",
			ro + `typeset -A a; set -x; a[""]=1 r=2 a[q]=3 :`,
			"ksh: warning: adding empty subscript\n+ a['']=1\nksh: r: is read only\n",
		},
		{"an element ahead", ro + `b=(1 2); set -x; b[1+1]=w r=2 :`, "+ b[2]=w\nksh: r: is read only\n"},
		{
			"a substitution ahead",
			ro + `set -x; x=$(echo s >&2) r=2 y=3 :`,
			"+ echo s\n+ 1>& 2\ns\n+ x=''\nksh: r: is read only\n",
		},
		{
			"a program",
			ro + `set -x; x=1 r=2 y=3 /usr/bin/true; echo st=$?`,
			"+ /usr/bin/true\n+ x=1\nksh: r: is read only\n+ echo st=1\nst=1\n",
		},
		{
			"a program with a substitution behind",
			ro + `set -x; x=1 r=2 y=$(echo s >&2) /usr/bin/true; echo st=$?`,
			"+ /usr/bin/true\n+ x=1\nksh: r: is read only\n+ echo st=1\nst=1\n",
		},
		// Controls.
		{"nothing ahead", ro + `set -x; r=2 y=3 :`, "ksh: r: is read only\n"},
		{
			"a regular builtin, which does not refuse",
			ro + `set -x; x=1 r=2 y=3 true; echo st=$?`,
			"+ true\n+ x=1\n+ r=2\n+ y=3\n+ echo st=0\nst=0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
