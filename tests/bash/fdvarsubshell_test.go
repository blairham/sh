// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAnFdVariableOnASubshellIsTheSubshells — a `{name}` redirection on
// parentheses is made in the subshell here, so the shell keeps neither the
// name nor the descriptor. The Yes side of
// interp.Semantics.FdVariableOnASubshellIsTheSubshells (#6520).
//
// Measured 2026-10-15 with `-c` on bash 5.3.20:
//
//	( : ) {v}</dev/null; echo "[$v]"; exec {x}</dev/null; echo "x=$x"
//	[]
//	x=10
//
// Ours printed `[10]` and `x=11`: the number was published out here and its
// descriptor stayed open, so the next allocation skipped it.
func TestAnFdVariableOnASubshellIsTheSubshells(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"numbers", "( echo \"in[$v]\" ) {v}</dev/null; echo \"[$v]\"; exec {x}</dev/null; echo \"x=$x\"\n", "in[10]\n[]\nx=10\n"},
		{"a value", "v=keep; ( : ) {v}</dev/null; echo \"[$v]\"\n", "[keep]\n"},
		{"attributes", "declare -i v=3; ( : ) {v}</dev/null; declare -p v\n", "declare -i v=\"3\"\n"},
		{"a group is the shell's", "{ :; } {v}</dev/null; echo \"[$v]\"\n", "[10]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs := runBashSplit(t, c.src)
			if out != c.want || errs != "" {
				t.Errorf("out %q err %q, want %q", out, errs, c.want)
			}
		})
	}
}

// The refusal for a frozen name is unchanged by whose the store is: two
// sentences, the parentheses do not run, and the status is 1.
func TestAFrozenFdVariableOnASubshellIsStillRefused(t *testing.T) {
	out, errs := runBashSplit(t, "readonly v=1\n( echo in ) {v}</dev/null\necho \"st=$? [$v]\"\n")
	if out != "st=1 [1]\n" {
		t.Errorf("out %q, want the body not run and 1", out)
	}
	if want := "bash: line 2: v: readonly variable\nbash: line 2: v: cannot assign fd to variable\n"; errs != want {
		t.Errorf("said %q, want %q", errs, want)
	}
}
