// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A list assigned to one element is refused here, a keyed one included, and
// the array keeps nothing — where ksh93 makes the element a table of its own
// (#6778). Measured 2026-11-02 against bash 5.3.20 from `-c` under `env -i
// PATH=/usr/bin:/bin`, with the write on a line of its own, byte for byte.
func TestAKeyedLiteralAssignedToAnElementIsRefusedHere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"a table's element",
			"declare -A a\na[k]=([u]=1 [v]=2)\necho \"st=$?\"; declare -p a",
			"bash: line 2: a[k]: cannot assign list to array member\nst=1\ndeclare -A a\n",
		},
		{
			"an indexed array's element",
			"declare -a a\na[1]=([u]=1)\necho \"st=$?\"; declare -p a",
			"bash: line 2: a[1]: cannot assign list to array member\nst=1\ndeclare -a a\n",
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
