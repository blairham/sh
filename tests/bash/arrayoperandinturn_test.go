// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// runBashFileCombined runs src as a script file, both streams on one buffer,
// with the file's path written as `f.sh` so a diagnostic's location reads the
// same on every machine.
func runBashFileCombined(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sh := bashShell(&buf, &buf)
	sh.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	_ = driver.MainArgs(sh, []string{"bash", path})
	return strings.ReplaceAll(buf.String(), path, "f.sh")
}

// A declaration's array-literal operand is expanded, traced and placed before
// the word behind it is expanded, so a later operand sees it and an operand
// that fails leaves the literals in front of it in place. A scalar operand is
// not placed in turn. Measured 2026-10-30 against GNU bash 5.3.20 from script
// files under `env -i PATH=/usr/bin:/bin`, byte for byte apart from the file's
// path. Every literal was expanded after the command's words and placed after
// the utility had run, so the failure kept nothing and the reads saw the old
// values (#6798).
//
// The function rows are where the literal lands: a declaration that makes
// locals takes them with the call the failure ends, and one that writes the
// global keeps it.
func TestAnArrayOperandIsPlacedBeforeTheNextOperandExpands(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a later literal that fails keeps the one in front of it",
			"a=(1 2); b=(3 4)\ntrap 'declare -p a b' EXIT\ntypeset -a a=(5) b=(6 $((1/0)))\n",
			"f.sh: line 3: 1/0: division by 0 (error token is \"0\")\n" +
				"declare -a a=([0]=\"5\")\ndeclare -a b=([0]=\"3\" [1]=\"4\")\n",
		},
		{
			"an earlier literal that fails keeps nothing",
			"a=(1 2); b=(3 4)\ntrap 'declare -p a b' EXIT\ntypeset -a b=(6 $((1/0))) a=(5)\n",
			"f.sh: line 3: 1/0: division by 0 (error token is \"0\")\n" +
				"declare -a a=([0]=\"1\" [1]=\"2\")\ndeclare -a b=([0]=\"3\" [1]=\"4\")\n",
		},
		{
			"a later scalar that fails keeps the literal",
			"a=(0)\ntrap 'declare -p a' EXIT\ndeclare -a a=(5) s=$((1/0))\n",
			"f.sh: line 3: 1/0: division by 0 (error token is \"0\")\ndeclare -a a=([0]=\"5\")\n",
		},
		{
			"a literal reads the one in front of it",
			"a=(0); typeset -a a=(1) b=(\"${a[0]}\"); echo \"${b[0]}\"\n",
			"1\n",
		},
		{
			"a scalar reads the literal in front of it",
			"a=(0); typeset a=(1) c=${a[0]}; echo \"$c\"\n",
			"1\n",
		},
		{
			"a literal does not read the scalar in front of it",
			"x=0; typeset x=1 c=($x); echo \"${c[0]} $x\"\n",
			"0 1\n",
		},
		{
			"an empty literal is placed too",
			"a=(1); declare -a a=() b=(${#a[@]}); echo \"${b[0]}\"\n",
			"0\n",
		},
		{
			"a subscript is read once",
			"i=0; declare m=([i++]=x [i++]=y) w=$i; echo \"i=$i w=$w ${!m[*]}\"\n",
			"i=2 w=2 0 1\n",
		},
		{
			"through a reference the literal lands on the name it names",
			"declare -n ref=tgt; declare -a ref=(1 2) z=(${tgt[1]}); declare -p z\n",
			"declare -a z=([0]=\"2\")\n",
		},
		{
			"a frozen name refused at its turn keeps the literal in front of it",
			"readonly q=1; a=(0)\nset -x\ndeclare -a a=(5) q=(b)\nset +x\necho \"a=${a[*]}\"\n",
			"+ a=('5')\nf.sh: line 3: q: readonly variable\n+ set +x\na=5\n",
		},
		{
			"a declaration of locals takes them with the call it ends",
			"a=(0)\nf(){ declare -a a=(5) b=(6 $((1/0))); }\nf\necho \"a=${a[*]}\"\n",
			"f.sh: line 2: 1/0: division by 0 (error token is \"0\")\na=0\n",
		},
		{
			"a global declaration keeps what it placed",
			"a=(0)\nf(){ declare -ga a=(8) b=(6 $((1/0))); }\nf\necho \"a=${a[*]}\"\n",
			"f.sh: line 2: 1/0: division by 0 (error token is \"0\")\na=8\n",
		},
		{
			"export keeps what it placed",
			"a=(0)\nf(){ export a=(5) b=(6 $((1/0))); }\nf\necho \"a=${a[*]}\"\n",
			"f.sh: line 2: 1/0: division by 0 (error token is \"0\")\na=5\n",
		},
		{
			"the trace interleaves the expansions and the lines",
			"set -x\ntypeset a=($(echo x)) b=($(echo y))\n",
			"++ echo x\n+ a=('x')\n++ echo y\n+ b=('y')\n+ typeset a b\n",
		},
		{
			"a scalar's expansion comes after the literal in front of it",
			"set -x\ndeclare a=(1) s=$(echo x) b=($(echo y))\n",
			"+ a=('1')\n++ echo x\n++ echo y\n+ b=('y')\n+ declare a s=x b\n",
		},
		{
			// #7075's trace row: the literal's line, then the scalar's
			// substitution, then the utility's.
			"a literal's line comes before the scalar behind it expands",
			"set -x\ntypeset -a a=(1) y=$(echo s >&2)\n",
			"+ a=('1')\n++ echo s\ns\n+ typeset -a a y=\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runBashFileCombined(t, c.src); got != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}

// A literal operand placed ahead of the utility costs the utility's own trace
// line when its placement is refused, and keeps what it placed before the
// refusal. Measured 2026-10-30 against GNU bash 5.3.20 from script files. The
// utility's line was written and the literal kept nothing (#6852).
func TestARefusedOperandPlacementCostsTheDeclarationsLine(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"an empty subscript",
			"set -x\ndeclare -a q=([$(true)]=x)\n",
			"++ true\n+ q=(['']='x')\nf.sh: line 2: []=x: bad array subscript\n",
		},
		{
			"an empty key in a table",
			"set -x\ndeclare -A t=([k]=v [\"\"]=y)\ndeclare -p t\n",
			"+ t=(['k']='v' ['']='y')\nf.sh: line 2: ['']='y': bad array subscript\n" +
				"+ declare -p t\ndeclare -A t=([k]=\"v\" )\n",
		},
		{
			"a position before the first, with a literal behind it",
			"set -x\ndeclare -a q=(1 [-5]=x) r=(2)\nset +x\ndeclare -p q; echo \"r=${r-unset}\"\n",
			"+ q=('1' ['-5']='x')\nf.sh: line 2: [-5]=x: bad array subscript\n+ set +x\n" +
				"declare -a q=([0]=\"1\")\nr=unset\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runBashFileCombined(t, c.src); got != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}
