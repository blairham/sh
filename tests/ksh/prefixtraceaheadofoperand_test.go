// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A declaration's assignment prefix is traced where it is worked through,
// ahead of the operands, and not where the route that stores it would write
// it. Measured 2026-10-30 against ksh93u+ 2012-08-01 (`/bin/ksh`), script
// files under `env -i PATH=/usr/bin:/bin`. The scalar operands, which this
// shell stores and traces one at a time, wrote their lines in front of the
// prefix's (#7001).
//
// The literal rows say the lines are the walk's rather than the operand's: a
// literal operand is not stored in turn, and the substitution inside it is
// still written behind the prefix. The last two rows were already answered,
// and the second of them is not a free control: a literal's line is written as
// it is expanded (#7006), so it is the prefix's line written ahead that keeps
// it behind. See interp/prefixoperandorder.go.
func TestADeclarationsPrefixIsTracedAheadOfItsOperands(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"typeset", `set -x; v=new typeset y=1`, "+ v=new\n+ y=1\n+ typeset y\n"},
		{
			"export with two entries and two operands",
			`set -x; v=new w=2 export y=1 z=2`,
			"+ v=new\n+ w=2\n+ y=1\n+ z=2\n+ export y z\n",
		},
		{
			"operands that read the prefix",
			`set -x; v=old; v=new typeset y=$v z=$v`,
			"+ v=old\n+ v=new\n+ y=new\n+ z=new\n+ typeset y z\n",
		},
		{"readonly", `set -x; v=1 readonly y=2`, "+ v=1\n+ y=2\n+ readonly y\n"},
		{"an entry reading the one before", `set -x; v=new w=$v typeset y=$w`, "+ v=new\n+ w=new\n+ y=new\n+ typeset y\n"},
		{
			"a substitution in the operand",
			`set -x; v=1 typeset y=$(echo s >&2)`,
			"+ v=1\n+ echo s\n+ 1>& 2\ns\n+ y=''\n+ typeset y\n",
		},
		{
			"a substitution in the prefix",
			`set -x; v=$(echo P >&2) typeset y=1`,
			"+ echo P\n+ 1>& 2\nP\n+ v=''\n+ y=1\n+ typeset y\n",
		},
		{"a literal and a scalar", `set -x; v=new typeset -a a=($v) y=1`, "+ v=new\n+ a=( new )\n+ y=1\n+ typeset -a a y\n"},
		{
			"a substitution in a literal",
			`set -x; v=1 typeset -a a=($(echo s >&2))`,
			"+ v=1\n+ echo s\n+ 1>& 2\ns\n+ a=( )\n+ typeset -a a\n",
		},
		{
			"a substitution in both",
			`set -x; v=$(echo P >&2) typeset -a a=($(echo O >&2))`,
			"+ echo P\n+ 1>& 2\nP\n+ v=''\n+ echo O\n+ 1>& 2\nO\n+ a=( )\n+ typeset -a a\n",
		},
		{"an operand reached ahead of a refused one", `readonly r=0; set -x; v=1 typeset y=1 r=2`, "+ v=1\n+ y=1\nksh: r: is read only\n"},
		// Already answered.
		{"no operand to write", `set -x; v=new typeset y`, "+ v=new\n+ typeset y\n"},
		{"a literal alone", `set -x; v=old; v=new typeset -a a=($v)`, "+ v=old\n+ v=new\n+ a=( new )\n+ typeset -a a\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
