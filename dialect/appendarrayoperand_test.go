// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// What a declaration utility is *handed* for an appending array-literal
// operand, in the three dialects that have the construct — #3805, where every
// column handed over the bare name and the operator was lost between the
// parser and the builtin, so a declaration two of the three refuse came back
// at status 0.
//
// Measured 2026-09-19, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> x.sh` over
// a script file, stdin from `/dev/null`, over
//
//	typeset u+=(3 4)
//	echo "tail st=$? u=[${u[*]}]"
//
// bash 5.3.20 takes the operand and says nothing; ksh93u+ 2012-08-01 and
// zsh 5.9.2 each refuse the name `u+` the operator left behind, in their own
// words, and neither reaches the second line. **dash 0.5.12 and BusyBox ash
// 1.37.0 have no column here**: neither grammar has an array literal, so the
// first line is a syntax error in both and there is no operand to hand over.
//
// The sentences below are this harness's location form — it runs a snippet
// rather than a script file, so there is no `x.sh[1]` in front of them. The
// panel's own bytes over a script file are `x.sh[1]: typeset: u+: invalid
// variable name` and `x.sh:typeset:1: not valid in this context: u+`, which
// the binaries this branch builds reproduce exactly.
//
// bash 3.2.57 is the row that made this a field of its own rather than
// Semantics.DeclarationTakesAnAppendOperand asked again. It *takes* the
// operator on a scalar operand — `u=1; typeset u+=3` is `13` there, silently —
// and refuses it on an array literal, so one shell holds both answers at once.
// No preset claims that column, which is why the split is recorded in the
// axis's own documentation rather than asserted here.

// appendArrayOperandProbe is the declaration and then the two things that
// separate the answers: the status it left and what the name came to hold.
//
// Both halves are needed. A column that refuses writes neither, so asserting
// only the complaint would pass for a shell that printed it and stored the
// array anyway — which is what bash 3.2.57 really does and what neither of
// these two columns does.
const appendArrayOperandProbe = "typeset u+=(3 4)\n" + `echo "tail st=$? u=[${u[*]}]"` + "\n"

// appendArrayOperandPlainProbe is the control: the same literal with no
// operator on it, which every column takes. Without it a dialect that had
// stopped declaring array operands altogether would pass every row below.
const appendArrayOperandPlainProbe = "typeset a=(3 4)\n" + `echo "tail st=$? a=[${a[*]}]"` + "\n"

func appendArrayOperandCases() []struct {
	dialecttest.Preset
	takes     interp.Answer
	want      string
	wantPlain string
} {
	return []struct {
		dialecttest.Preset
		takes     interp.Answer
		want      string
		wantPlain string
	}{
		{
			dialecttest.Preset{
				Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
				Diagnostics: bash.Diagnostics, Apply: bash.Apply,
			},
			interp.Yes,
			"tail st=0 u=[3 4]\n",
			"tail st=0 a=[3 4]\n",
		},
		{
			dialecttest.Preset{
				Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
				Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
			},
			interp.No,
			"ksh: typeset: u+: invalid variable name\n",
			"tail st=0 a=[3 4]\n",
		},
		{
			dialecttest.Preset{
				Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
				Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
			},
			interp.No,
			"zsh:typeset:1: not valid in this context: u+\n",
			"tail st=0 a=[3 4]\n",
		},
	}
}

// TestEachDialectAnswersWhetherAnAppendingArrayOperandIsTaken pins the value
// each preset holds.
func TestEachDialectAnswersWhetherAnAppendingArrayOperandIsTaken(t *testing.T) {
	for _, c := range appendArrayOperandCases() {
		t.Run(c.Name, func(t *testing.T) {
			if got := c.Semantics().DeclarationTakesAnAppendingArrayOperand; got != c.takes {
				t.Errorf("DeclarationTakesAnAppendingArrayOperand = %v, want %v", got, c.takes)
			}
		})
	}
}

// TestAnAppendingArrayOperandIsHandedOverAsEachColumnHandsIt is what the field
// is for: a preset holding the right value and writing the wrong bytes would
// pass the test above.
func TestAnAppendingArrayOperandIsHandedOverAsEachColumnHandsIt(t *testing.T) {
	for _, c := range appendArrayOperandCases() {
		t.Run(c.Name, func(t *testing.T) {
			out, _, err := c.Combined(t, dialecttest.Base{}, appendArrayOperandProbe)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if out != c.want {
				t.Errorf("appending\n%q\nwant\n%q", out, c.want)
			}
			out, _, err = c.Combined(t, dialecttest.Base{}, appendArrayOperandPlainProbe)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if out != c.wantPlain {
				t.Errorf("plain\n%q\nwant\n%q", out, c.wantPlain)
			}
		})
	}
}
