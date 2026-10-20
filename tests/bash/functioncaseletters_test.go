// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The case letters are attributes of a function here, and a function line
// that writes `f` or `F` under both signs is settled by where the signs are,
// not by the last one. Measured 2026-10-03 on bash 5.3.20 with `f(){ echo
// body; }` defined; see Runner.functionLineUnderBothSigns and
// Semantics.FunctionAttributeLetters.
func TestAFunctionLineCarriesCaseLettersAndBothSigns(t *testing.T) {
	const def = "f(){ echo body; }; "
	for _, tc := range []struct{ src, want string }{
		{"typeset -fu f; f; typeset -F", "body\ndeclare -fu f\n"},
		{"typeset -fu f; typeset -fl f; typeset -F", "declare -fl f\n"},
		{"typeset -fx f; typeset -fc f; typeset -ft f; typeset -F", "declare -ftxc f\n"},
		{"typeset -fu f; typeset -f +u f; typeset -F", "declare -f f\n"},
		{"typeset +f -f f; echo st=$?", "st=0\n"},
		{"typeset -f +f f; echo st=$?", "st=0\n"},
		{"typeset -f +f", "f () \n{ \n    echo body\n}\n"},
		{"typeset -F +f", "declare -f f\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if out, _ := answersRun(t, def+tc.src); out != tc.want {
				t.Errorf("= %q, want %q", out, tc.want)
			}
		})
	}
}
