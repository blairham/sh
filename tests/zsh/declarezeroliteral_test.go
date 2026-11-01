// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A declaration's store to `0` sets `$0`, as `0=x` does, for the lifetime of
// the frame it was made in. Measured 2026-10-16 on zsh 5.9.2, `-f`, under
// `env -i PATH=/usr/bin:/bin` (#6809).
func TestADeclarationSetsDollarZero(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`typeset 0=x; echo $? $0`, "0 x\n"},
		{`export 0=x; readonly 0=y; echo $0`, "y\n"},
		{`f(){ local 0=x; echo $0; }; f; echo ${0:t}`, "x\nzsh\n"},
		{`f(){ local 0=x; local 0=z; echo $0; }; f`, "z\n"},
		{`f(){ typeset -g 0=y; echo $0; }; f; echo ${0:t}`, "y\nzsh\n"},
		{`f(){ local 0; echo ${(t)0}; }; f`, "scalar-local-special\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), "0=zsh\n"+tc.src)
		if out != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}

// An array literal with a kind letter over one of the shell's scalar or
// numeric slots: on a line that lands on the slot the value is refused, in
// the sentence the letter picks, and on a line that makes a local the letter
// is. Each ends the shell at 1. Measured 2026-10-16 on zsh 5.9.2 (#6807).
func TestALiteralWithAKindLetterOverAFixedScalar(t *testing.T) {
	const (
		special = "can't assign array value to non-array special"
		plain   = "can't assign array value to non-array"
		kind    = "can't change type of a special parameter"
	)
	for _, tc := range []struct{ src, want string }{
		{`typeset -A HOME=(a b)`, "zsh:typeset:1: HOME: " + special},
		{`typeset -a RANDOM=(1)`, "zsh:typeset:1: RANDOM: " + special},
		{`typeset -i HOME=(1)`, "zsh:typeset:1: HOME: " + plain},
		{`typeset -i RANDOM=(1)`, "zsh:typeset:1: RANDOM: " + plain},
		{`typeset -F SECONDS=(1)`, "zsh:typeset:1: SECONDS: " + plain},
		{`typeset -a 0=(a b)`, "zsh:typeset:1: 0: " + special},
		{`typeset -i 0=(1)`, "zsh:typeset:1: 0: " + plain},
		{`f(){ typeset -g -a HOME=(a); }; f`, "f:typeset: HOME: " + special},
		{`f(){ export -i RANDOM=(1); }; f`, "f:export: RANDOM: " + plain},
		{`f(){ local -i RANDOM=(1); }; f`, "f:local: RANDOM: inconsistent type for assignment"},
		// The controls: the letter's sentence wherever a local is made, or
		// where no literal was written.
		{`f(){ local -a HOME=(a); }; f`, "f:local: HOME: " + kind},
		{`f(){ readonly -i HOME=(1); }; f`, "f:readonly: HOME: " + kind},
		{`typeset -a HOME=x`, "zsh:typeset:1: HOME: " + kind},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src+"\necho after")
		if want := tc.want + "\n"; out != want || st != 1 {
			t.Errorf("%s\n got %q (%d)\nwant %q (1)", tc.src, out, st, want)
		}
	}
}
