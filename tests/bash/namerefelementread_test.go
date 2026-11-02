// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A subscript read through a reference aimed at an element finds nothing here,
// where ksh93 reads the nested element the write built — the other column of
// interp.Semantics.ElementWriteThroughAnElementReferenceNests, which decides
// the read as well as the write (#7366). Measured 2026-11-02 against bash
// 5.3.20 from `-c` under `env -i PATH=/usr/bin:/bin`, byte for byte.
func TestAnElementReadThroughAnElementReferenceFindsNothingHere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the control",
			`declare -A a; a[x]=s; declare -n e='a[x]'; echo "${e[0]} ${#e[@]}"`,
			" 0\n",
		},
		{
			"every reading",
			`declare -A a; a[x]=s; declare -n e='a[x]'; echo "<$e> <${e[0]}> <${#e[@]}> <${e[@]}> <${!e[@]}>"`,
			"<s> <> <0> <> <>\n",
		},
		{
			"an indexed array's element",
			`declare -a a; a[1]=s; declare -n e='a[1]'; echo "<$e> <${e[0]}> <${#e[@]}> <${e[@]}> <${!e[@]}>"`,
			"<s> <> <0> <> <>\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runBash(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
