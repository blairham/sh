// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A declaration's array-literal operand written behind another literal is
// placed in turn too: the operand walk reached only the literals in front of a
// scalar word, so a literal behind a literal read the old value, a failure
// kept nothing, and the trace wrote every expansion ahead of every line
// (#6798). Measured 2026-10-30 on ksh93u+ 2012-08-01 from script files, byte
// for byte apart from the location's file name.
func TestALiteralOperandBehindALiteralIsPlacedInTurn(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a later literal that fails keeps the one in front of it",
			"a=(1 2); b=(3 4)\ntrap 'typeset -p a b' EXIT\ntypeset -a a=(5) b=(6 $((1/0)))\n",
			"sh: line 3: 1/0: divide by zero\ntypeset -a a=(5)\ntypeset -a b\n",
		},
		{
			"a literal reads the one in front of it",
			"a=(0); typeset -a a=(1) d=(\"${a[0]}\"); echo \"${d[0]}\"\n",
			"1\n",
		},
		{
			"an empty literal is held too",
			"a=(1); typeset -a a=() b=(${#a[@]}); echo \"${b[0]}\"\n",
			"0\n",
		},
		{
			"the trace interleaves the expansions and the lines",
			"set -x\ntypeset a=($(echo x)) b=($(echo y))\n",
			"+ echo x\n+ a=( x )\n+ echo y\n+ b=( y )\n+ typeset a b\n",
		},
		{
			// A POSIX-form body declares no locals here, so what it placed
			// stays when the failure ends the script.
			"a POSIX-form function keeps what it placed",
			"a=(1 2)\nh() { typeset -a a=(7) b=(6 $((1/0))); }\ntrap 'typeset -p a' EXIT\nh\n",
			"sh: line 2: 1/0: divide by zero\ntypeset -a a=(7)\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, _ := answersRun(t, c.src); out != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}

// A declaration stopped inside a `function`-form body was declaring locals of
// that call, and the stopped call takes them with it: what the walk had held
// for the operands in front of the stop is put back rather than left in the
// global cell. Measured 2026-10-29 and 2026-10-30 on ksh93u+ 2012-08-01, from
// script files (#7011, #6798): each failing row writes its complaint and then
// the line shown. The fourth row is the control: nothing stops, and the local
// is gone at the return either way.
func TestAStoppedDeclarationInAKeywordFunctionLeavesNoGlobal(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a value that will not expand",
			"function f { typeset x=1 y=${q?boom}; }; f; echo \"out x=${x-unset}\"\n",
			"out x=unset\n",
		},
		{
			"read by an exit trap",
			"trap 'echo \"trap x=${x-unset}\"' EXIT; function f { typeset x=1 y=${q?boom}; }; f\n",
			"trap x=unset\n",
		},
		{
			"a frozen local",
			"function f { typeset -r r=0; typeset x=1 r=2; echo in; }; f; echo \"out x=${x-unset}\"\n",
			"out x=unset\n",
		},
		{
			"a frozen global the call shadows",
			"readonly r=0; function f { typeset x=1 r=2; echo \"in x=$x\"; }; f; echo \"out x=${x-unset}\"\n",
			"in x=1\nout x=unset\n",
		},
		{
			"a literal behind a literal",
			"a=(1 2)\nfunction g { typeset -a a=(5) b=(6 $((1/0))); }\ng\necho \"after g a=${a[*]}\"\n",
			"after g a=1 2\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			// What the call left is the question; the complaint in front of
			// it is the reference's own and is not what this pins.
			if out, _ := answersRun(t, c.src); !strings.HasSuffix(out, c.want) {
				t.Errorf("%s\ngot  %q\nwant it to end %q", c.src, out, c.want)
			}
		})
	}
}
