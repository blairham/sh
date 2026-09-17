// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A reference aimed at an array *element* reaches builtins that name a
// variable. Measured 2026-09-17 on bash 5.3.20, script files under `env -i`.
//
// `mapfile` refuses it — a whole array cannot go into one element — and the
// refusal is the identifier one at 1. Here the reference was not followed at
// all, so a parameter whose *name* was `A[0]` came into being and the array
// the script meant was never written. `unset` goes the other way and takes
// the element away, where this shell left it standing.
func TestAReferenceAimedAtAnElementReachesTheBuiltins(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"mapfile refuses it",
			"declare -n r=A[0]\nmapfile -t r <<<m\necho \"st=$?\"\necho \"[$(declare -p A 2>&1)]\"\n",
			"sh: line 2: mapfile: `A[0]': not a valid identifier\nst=1\n[sh: line 4: declare: A: not found]\n",
		},
		{
			"readarray refuses it in the same words",
			"declare -n r=A[0]\nreadarray -t r <<<m\necho \"st=$?\"\n",
			"sh: line 2: readarray: `A[0]': not a valid identifier\nst=1\n",
		},
		{
			"unset takes the element away",
			"B=(b0 b1)\ndeclare -n u=B[1]\nunset -v u\necho \"[$(declare -p B)]\"\n",
			"[declare -a B=([0]=\"b0\")]\n",
		},
		{
			"and does so from inside a call",
			"D=(d0)\nf() { local -n y=D[0]; unset -v y; }\nf\necho \"[$(declare -p D)]\"\n",
			"[declare -a D=()]\n",
		},
		{
			"a reference to a plain name still unsets the name",
			"v=1\ndeclare -n r=v\nunset -v r\necho \"[${v-gone}]\"\n",
			"[gone]\n",
		},
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
