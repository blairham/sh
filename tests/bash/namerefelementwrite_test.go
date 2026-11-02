// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// An element written through a reference that is itself aimed at an element
// is refused here, naming the element as a name that is not one, and the
// array keeps nothing — the other column of
// interp.Semantics.ElementWriteThroughAnElementReferenceNests, where ksh93
// nests the element (#7113). Measured 2026-11-02 against bash 5.3.20 from `-c`
// under `env -i PATH=/usr/bin:/bin`, with the write on a line of its own,
// byte for byte.
func TestAnElementWriteThroughAnElementReferenceIsRefusedHere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"a table's element",
			"declare -A a; declare -n e='a[x]'\ne[k]=1\ndeclare -p a e",
			"bash: line 2: `a[x]': not a valid identifier\ndeclare -A a\ndeclare -n e=\"a[x]\"\n",
		},
		{
			"an indexed array's element",
			"declare -a a; declare -n e='a[1]'\ne[2]=z\ndeclare -p a e",
			"bash: line 2: `a[1]': not a valid identifier\ndeclare -a a\ndeclare -n e=\"a[1]\"\n",
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
