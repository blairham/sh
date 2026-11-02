// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A declaration's array literal placed ahead of the utility stays placed
// whatever the utility then does, and inside a function a frozen literal is
// refused at its own turn while the walk goes on. Measured 2026-11-02 against
// GNU bash 5.3.20 from script files under `env -i PATH=/usr/bin:/bin`, byte for
// byte apart from the file's path. This shell held each literal, put it back
// before the utility ran and stored it only after a utility that succeeded, so
// every row came out empty, and the frozen literal's line was written and its
// refusal made after the utility's line (#7103).
func TestAPlacedLiteralOperandOutlivesTheUtilitysRefusal(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"local outside a function keeps the literal as a global",
			"local -a q=(1 2)\ndeclare -p q\n",
			"f.sh: line 1: local: can only be used in a function\n" +
				"declare -a q=([0]=\"1\" [1]=\"2\")\n",
		},
		{
			"with the letters that shape its value and no others",
			"local -A m=([k]=v)\nlocal -i n=(1+1)\nlocal -x x=(1)\ndeclare -p m n x\n",
			"f.sh: line 1: local: can only be used in a function\n" +
				"f.sh: line 2: local: can only be used in a function\n" +
				"f.sh: line 3: local: can only be used in a function\n" +
				"declare -A m=([k]=\"v\" )\ndeclare -ai n=([0]=\"2\")\ndeclare -a x=([0]=\"1\")\n",
		},
		{
			"a frozen literal in a function is refused at its turn",
			"readonly r=1\n" +
				"f(){ declare -a c=(5) r=(b) d=(6); echo \"in st=$? c=${c[*]} d=${d[*]}\"; }\n" +
				"set -x\nf\nset +x\necho \"out c=${c[*]-unset} d=${d[*]-unset}\"\n",
			"+ f\n+ c=('5')\nf.sh: line 2: f: r: readonly variable\n+ d=('6')\n" +
				"+ declare -a c r d\nf.sh: line 2: declare: r: readonly variable\n" +
				"+ echo 'in st=1 c=5 d=6'\nin st=1 c=5 d=6\n+ set +x\nout c=unset d=unset\n",
		},
		{
			"after its words, and under no name past the call's first command",
			"readonly r=1\nf(){ :; local -a c=(5) r=($(echo X >&2)); echo \"st=$? c=${c[*]}\"; }\nf\n",
			"X\nf.sh: line 2: r: readonly variable\nf.sh: line 2: local: r: readonly variable\nst=1 c=5\n",
		},
		{
			"a frozen operand the utility refuses at the top level",
			"readonly r=1\ndeclare -a c=(5) r d=(6)\necho \"st=$?\"\ndeclare -p c d\n",
			"f.sh: line 2: declare: r: readonly variable\nst=1\n" +
				"declare -a c=([0]=\"5\")\ndeclare -a d=([0]=\"6\")\n",
		},
		{
			"a name the utility refuses keeps the call's locals",
			"f(){ declare -a c=(5) 'x-y' d=(6); echo \"in st=$? c=${c[*]} d=${d[*]}\"; }\n" +
				"f\necho \"out c=${c[*]-unset}\"\n",
			"f.sh: line 1: declare: `x-y': not a valid identifier\nin st=1 c=5 d=6\nout c=unset\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := runBashFileCombined(t, c.src); got != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}
