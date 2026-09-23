// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// The container letter on `export` takes effect only where the operand carries a
// value. Valueless it records no container, converts none and refuses none.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh` over a
// script file with standard input on the null device, against bash 5.3.15 in the
// digest-pinned image the suite is graded in and bash 5.3.20 on macOS — the two
// giving the same answer to every row.
//
// This shell recorded the letter whatever the operand carried, so a valueless
// `export -A n` declared a table, and a valueless one over a name already holding
// the other kind reached the conversion refusal — `a=(x 1); export -A a` was
// `cannot convert indexed to associative array` where the reference exports the
// array and says nothing (#4089, #4179).
func TestTheContainerLetterOnExportNeedsAValue(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// With a value the letter is read, and all three shapes of value count.
		{"an array literal", `export -a q=(1 2); declare -p q`, `declare -ax q=([0]="1" [1]="2")`},
		{"a table literal", `export -A m=([k]=v); declare -p m`, `declare -Ax m=([k]="v" )`},
		{"a plain value under the table letter", `export -A n=5; declare -p n`, `declare -Ax n=([0]="5" )`},
		// Valueless it records nothing.
		{"valueless under the array letter", `export -a b; declare -p b`, `declare -x b`},
		{"valueless under the table letter", `export -A n2; declare -p n2`, `declare -x n2`},
		// And refuses nothing over a name already holding the other kind, which
		// is the row the suite reaches.
		{
			"over a standing array",
			"a=(x 1 y 2)\nexport -A a\ndeclare -p a",
			`declare -ax a=([0]="x" [1]="1" [2]="y" [3]="2")`,
		},
		{
			"over a standing table",
			"declare -A d=([k]=1)\nexport -a d\ndeclare -p d",
			`declare -Ax d=([k]="1" )`,
		},
		// The element after a valueless letter is the half that is observable
		// beyond a listing: with no table attribute recorded, `y[k]` is
		// arithmetic on an indexed array rather than a key.
		{
			"the element after a valueless table letter",
			"export -A y\ny[k]=q\ndeclare -p y",
			`declare -ax y=([0]="q")`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if strings.TrimSpace(out) != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// Per operand and not per line, which is what keeps one `export` from answering
// its own operands two ways: `export -A a b=1` records a table for `b` and
// nothing for `a`.
func TestTheContainerLetterOnExportIsPerOperand(t *testing.T) {
	out, st := answersRun(t, "export -A a b=1\ndeclare -p a\ndeclare -p b")
	want := "declare -x a\ndeclare -Ax b=([0]=\"1\" )\n"
	if out != want || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, want)
	}
}

// The axis, pinned so that no preset here drifts off the column it was measured
// from. Every other dialect's `export` takes no container letter at all — which
// Semantics.ExportOptions is the statement of — so none of them can be asked.
func TestTheExportContainerLetterIsThisDialectsAnswer(t *testing.T) {
	if got := bash.Semantics().ExportContainerLetterNeedsAValue; got != interp.Yes {
		t.Errorf("ExportContainerLetterNeedsAValue = %v, want Yes", got)
	}
}
