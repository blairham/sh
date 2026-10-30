// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A declaration's array-literal operand is traced as its elements are
// expanded, which is ahead of the assignment prefix's line: this column
// expands the operand before the prefix. Measured 2026-10-30 against GNU bash
// 5.3.20 under `env -i PATH=/usr/bin:/bin`, from `-c` and from a script file
// alike. The literal's line was written with the command's, behind the
// prefix's (#7006).
//
// The division row is the same point from the other side: the literal's line
// is out before a later operand fails. The last two rows are controls this
// shell already answered, a scalar operand staying on the command's line. See
// interp/arrayoperandorder.go.
func TestAnArrayOperandIsTracedAsItIsExpanded(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"typeset", `set -x; v=new typeset -a a=(1)`, "+ a=('1')\n+ v=new\n+ typeset -a a\n"},
		{"declare with a scalar behind", `set -x; v=new declare -a a=(1) y=2`, "+ a=('1')\n+ v=new\n+ declare -a a y=2\n"},
		{
			"a literal reading the name the prefix sets",
			`set -x; v=old; v=new typeset -a a=($v)`,
			"+ v=old\n+ a=('old')\n+ v=new\n+ typeset -a a\n",
		},
		{"two literals", `set -x; v=new typeset -a a=(1) b=(2)`, "+ a=('1')\n+ b=('2')\n+ v=new\n+ typeset -a a b\n"},
		{"a table", `set -x; v=1 w=2 declare -A m=([k]=v)`, "+ m=(['k']='v')\n+ v=1\n+ w=2\n+ declare -A m\n"},
		{"an empty literal", `set -x; v=1 typeset -a w=()`, "+ w=()\n+ v=1\n+ typeset -a w\n"},
		{"an appending prefix", `set -x; v+=x typeset -a a=(1)`, "+ a=('1')\n+ v=x\n+ typeset -a a\n"},
		{"local", `set -x; f() { v=1 local -a a=(1); }; f`, "+ f\n+ a=('1')\n+ v=1\n+ local -a a\n"},
		{
			"substitutions in both",
			`set -x; v=$(echo P >&2) typeset -a a=($(echo O >&2))`,
			"++ echo O\nO\n+ a=()\n++ echo P\nP\n+ v=\n+ typeset -a a\n",
		},
		{
			"a later operand that fails",
			`set -x; typeset -a a=(1) b=($((1/0))); echo st=$?`,
			"+ a=('1')\nbash: line 1: 1/0: division by 0 (error token is \"0\")\n",
		},
		// Controls.
		{"no prefix", `set -x; typeset -a a=(1)`, "+ a=('1')\n+ typeset -a a\n"},
		{"scalar operands", `set -x; v=old; v=new typeset y=$v z=$v`, "+ v=old\n+ v=new\n+ typeset y=old z=old\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
