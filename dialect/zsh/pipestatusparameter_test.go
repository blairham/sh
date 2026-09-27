// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `$pipestatus` is a parameter this shell has and could not describe.
//
// `${+pipestatus}` answered 1 and `${(t)pipestatus}` answered nothing, in one
// shell in one line (#4865). The record reaches none of the three producer
// tables — it is a field on the runner, because the record is the core's and
// only the name is this dialect's — so every union that answers "what does
// this shell have" walked past it. Measured 2026-09-27 on zsh 5.9.2 under
// `-f` from a script file; interp/pipestatus.go has the seam.
func TestThePipelineStatusDescribesItself(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the two readings agree now",
			"true\n" + `print -r -- "[${(t)pipestatus}] [${+pipestatus}] [$pipestatus]"`,
			"[array-special] [1] [0]\n",
		},
		{"the table renders the same word", "true\n" + `print -r -- $parameters[pipestatus]`, "array-special\n"},
		{"a listing writes the row", "true\n" + `typeset -p pipestatus`, "typeset -a pipestatus=( 0 )\n"},
		{
			"and the bare listing has it once",
			"true\n" +
				"lines=( ${(f)\"$(typeset -p)\"} )\n" +
				"m=( ${(M)lines:#typeset -a pipestatus=*} )\n" +
				`print -r -- "count=$#m first=[$m[1]]"`,
			"count=1 first=[typeset -a pipestatus=( 0 )]\n",
		},
		{"the elements are still the record", `false | true | false; print -r -- "$pipestatus"`, "1 0 1\n"},
		{
			"and unset takes the name away, which is this shell's answer",
			"true\n" + `unset pipestatus; print -r -- "[${(t)pipestatus}] [${+pipestatus}]"`,
			"[] [0]\n",
		},

		// The controls, in the same shell. Three produced parameters that
		// were already right is what says this was one union's gap rather
		// than a description that cannot see a producer at all.
		{"a produced scalar", `print -r -- ${(t)RANDOM}`, "integer-special\n"},
		{"a produced array", `print -r -- ${(t)funcstack}`, "array-readonly-hide-hideval-special\n"},
		{"a produced association", `print -r -- ${(t)parameters}`, "association-readonly-hide-hideval-special\n"},
		{"and a name nothing produces", `print -r -- "[${(t)nosuchname}][${+nosuchname}]"`, "[][0]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
