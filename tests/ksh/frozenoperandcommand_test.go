// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A declaration's value refused by a freeze stays fatal through `command`,
// where the declaration's own refusal of a bad name is softened by it. Inside
// a `function`-form call the store is made by the builtin the word named, so
// it is the word that has to decline it. Measured 2026-10-30 on ksh93u+
// 2012-08-01 under `env -i PATH=/usr/bin:/bin`, `-c`, every row byte for byte
// with the shell's own path taken out (#6962). See
// interp.Semantics.FrozenOperandRefusalOutlivesTheCommandWord.
func TestAFrozenOperandsRefusalEndsTheFunctionThroughCommand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"readonly",
			`readonly x=0; function f { command readonly x=5; echo in; }; f; echo "st=$? after"`,
			"ksh: f: line 1: x: is read only\nst=1 after\n",
		},
		{
			"export",
			`readonly x=0; function f { command export x=5; echo in; }; f; echo "st=$? after"`,
			"ksh: f: line 1: x: is read only\nst=1 after\n",
		},
		{
			"an append operand",
			`readonly x=0; function f { command readonly x+=5; echo in; }; f; echo "st=$? after"`,
			"ksh: f: line 1: x: is read only\nst=1 after\n",
		},
		{
			"the operand in front has landed",
			`readonly x=0; function f { command readonly z=1 x=5; echo in; }; f; echo "st=$? after z=$z"`,
			"ksh: f: line 1: x: is read only\nst=1 after z=1\n",
		},
		{
			"a second word does not reach it either",
			`readonly x=0; function f { command command readonly x=5; echo in; }; f; echo "st=$? after"`,
			"ksh: f: line 1: x: is read only\nst=1 after\n",
		},
		{
			"the function is over, not the command",
			`readonly x=0; function f { command readonly x=5 || echo or; echo in; }; f; echo "st=$? after"`,
			"ksh: f: line 1: x: is read only\nst=1 after\n",
		},
		{
			"only the call it was raised in",
			`readonly x=0; function g { command readonly x=5; echo in; }; function f { g; echo inf; }; f; echo "st=$? after"`,
			"ksh: f[1]: g: line 1: x: is read only\ninf\nst=0 after\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runKshValues(t, tc.src); out != tc.want {
				t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
			}
		})
	}
}

// And what the word still takes, which is what keeps the rule about the kind
// of refusal rather than about `command` in a function: the builtin's own
// refusal of a name, an ordinary special-builtin failure after a store
// refusal was caught by the call it ended, and a store refusal at the top
// level or in a POSIX-style function, where the value is refused before the
// builtin runs at all. Measured with the rows above.
func TestCommandStillSoftensWhatIsTheBuiltinsOwn(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a bad name is the builtin's",
			`readonly x=0; function f { command readonly 1y; echo in; }; f; echo "st=$? after"`,
			"ksh: f[1]: readonly: 1y: invalid variable name\nin\nst=0 after\n",
		},
		{
			"a failure after the caught refusal is softened as before",
			`readonly x=0; function g { command readonly x=5; }; g || command set -Z; echo "st=$? after"`,
			"ksh: g: line 1: x: is read only\n" +
				"ksh: set: -Z: unknown option\n" +
				"Usage: set [-sabefhkmnprtuvxBCGH] [-A name] [-o[option]] [arg ...]\n" +
				"st=2 after\n",
		},
		{
			"and inside an eval the word ran",
			`readonly x=0; function g { command readonly x=5; }; command eval 'g; set -Z; echo in2'; echo "st=$? after"`,
			"ksh: eval[1]: g: line 1: x: is read only\n" +
				"ksh: eval[1]: set: -Z: unknown option\n" +
				"Usage: set [-sabefhkmnprtuvxBCGH] [-A name] [-o[option]] [arg ...]\n" +
				"st=2 after\n",
		},
		{
			"the top level ends the script",
			`readonly x=0; command readonly x=5; echo "st=$? after"`,
			"ksh: x: is read only\n",
		},
		{
			"a POSIX-style function ends the script",
			`readonly x=0; f() { command readonly x=5; echo in; }; f; echo "st=$? after"`,
			"ksh: x: is read only\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runKshValues(t, tc.src); out != tc.want {
				t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
			}
		})
	}
}
