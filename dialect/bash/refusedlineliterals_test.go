// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// A declaration whose letter is refused still declares and stores its array
// literals here, with the letters that decide what the value is.
//
// Measured 2026-10-03 on bash 5.3.20, `-c`, every refusal printed with its
// usage line at status 2:
//
//	typeset -U a=(1 1 2)          declare -a a=([0]="1" [1]="1" [2]="2")
//	declare -U a=(1) b=2 c=(3)    a and c arrays, b not found
//	declare -Ui m=(1+1)           declare -ai m=([0]="2")
//	declare -Ux m=(1)             declare -a m — the export letter is not kept
//	f(){ typeset -U x=(1); }; f   x is the call's local and gone after it
//	f(){ export -U z=(1); }; f    z is the global
//	declare -ail q; f(){ local -Ui q=(1+2); declare -p q; }; f
//	                              the local, -ai holding 3
//
// zsh's answer is the other one, and it is pinned in that dialect's package.
func TestARefusedLetterKeepsTheArrayLiteralsBehindIt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "the literal is stored",
			src:  `typeset -U a=(1 1 2); echo "st=$?"; declare -p a`,
			want: "st=2\ndeclare -a a=([0]=\"1\" [1]=\"1\" [2]=\"2\")\n",
		},
		{
			name: "a plain operand beside it is not",
			src:  `declare -U a=(1) b=2 c=(3) 2>/dev/null; declare -p a c; echo "[${b-unset}]"`,
			want: "declare -a a=([0]=\"1\")\ndeclare -a c=([0]=\"3\")\n[unset]\n",
		},
		{
			name: "a value letter is kept",
			src:  `declare -Ui m=(1+1) 2>/dev/null; declare -p m`,
			want: "declare -ai m=([0]=\"2\")\n",
		},
		{
			name: "the export letter is not",
			src:  `declare -Ux m=(1) 2>/dev/null; declare -p m`,
			want: "declare -a m=([0]=\"1\")\n",
		},
		{
			name: "a function's declaration is its local",
			src:  `f(){ typeset -U x=(1) 2>/dev/null; echo "${x[0]}"; }; f; echo "[${x-unset}]"`,
			want: "1\n[unset]\n",
		},
		{
			name: "an export lands on the global",
			src:  `f(){ export -U z=(1) 2>/dev/null; }; f; declare -p z`,
			want: "declare -a z=([0]=\"1\")\n",
		},
		{
			name: "a local shadows the global",
			src:  `declare -l q; f(){ local -Ui q=(1+2) 2>/dev/null; declare -p q; }; f; declare -p q`,
			want: "declare -ai q=([0]=\"3\")\ndeclare -l q\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.HasSuffix(out, tc.want) {
				t.Errorf("= %q, want it to end %q", out, tc.want)
			}
		})
	}
}

func TestARefusedLetterKeepingTheLiteralsIsThisDialectsAnswer(t *testing.T) {
	if got := bash.Semantics().RefusedDeclarationKeepsItsArrayLiterals; got != interp.Yes {
		t.Errorf("RefusedDeclarationKeepsItsArrayLiterals = %v, want Yes", got)
	}
}
