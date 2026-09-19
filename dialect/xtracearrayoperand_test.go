// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// What `set -x` writes for a declaration utility's **array literal** operand,
// in the three dialects that have the construct — #3567, where every column
// wrote the command word and left the assignment it carried out of the trace
// entirely.
//
// Measured 2026-09-19, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> x.sh` over
// a script file, stdin from `/dev/null`. bash 3.2.57 answers as 5.3.20 does.
//
// **dash 0.5.12 and BusyBox ash 1.37.0 have no column here**, and that is the
// measurement rather than an omission: neither has an array literal at all, so
// `typeset b=(1 2)` is a syntax error in both and there is no operand to place.
//
// The probe puts a *scalar* operand on the same command as the array one,
// which is the row that says these are two fields: bash takes the literal off
// the command line and leaves `y=1` on it, where ksh93 takes both off.
//
// An empty literal is the next two rows, because the three columns write three
// different things for it — a pair, nothing at all, and a spaced pair — and
// because it arrives by **two** routes that only one of them tells apart. With
// no letters beside it, `()` is an empty compound body in the column that has
// compound variables and the parser says so; with `-A` on the same command the
// letters have taken the array reading and the parser cannot, so a run holding
// only the first row passes with the second one wrong.
const arrayOperandProbe = "set -x\ntypeset y=1 b=(1 2)\ntypeset w=()\ntypeset -A s=()\n"

// The subscripted literal, which only two columns can be asked about: zsh 5.9.2
// reads `[k]` inside a literal as a glob qualifier and writes the mangled word
// it made of it, so there is no answer there to grade against.
const arrayOperandSubscriptProbe = "set -x\ntypeset -A m=([k]=v)\n"

func arrayOperandPresets() []struct {
	dialecttest.Preset
	where   interp.TraceDeclarationOperand
	quoted  bool
	want    string
	wantSub string
} {
	return []struct {
		dialecttest.Preset
		where   interp.TraceDeclarationOperand
		quoted  bool
		want    string
		wantSub string
	}{
		{
			dialecttest.Preset{
				Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
				Diagnostics: bash.Diagnostics, Apply: bash.Apply,
			},
			interp.TraceOperandSplitBefore, true,
			"+ b=('1' '2')\n+ typeset y=1 b\n+ w=()\n+ typeset w\n+ s=()\n+ typeset -A s\n",
			"+ m=(['k']='v')\n+ typeset -A m\n",
		},
		{
			dialecttest.Preset{
				Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
				Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
			},
			interp.TraceOperandSplitBefore, false,
			"+ y=1\n+ b=( 1 2 )\n+ typeset y b\n+ typeset w\n+ typeset -A s\n",
			"+ m[k]=v\n+ typeset -A m\n",
		},
		{
			dialecttest.Preset{
				Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
				Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
			},
			interp.TraceOperandOnTheCommandLine, false,
			"+zsh:2> typeset y=1 b=( 1 2 )\n+zsh:3> typeset w=( )\n+zsh:4> typeset -A s=( )\n",
			"",
		},
	}
}

func TestEachDialectAnswersWhereAnArrayOperandIsTraced(t *testing.T) {
	for _, p := range arrayOperandPresets() {
		t.Run(p.Name, func(t *testing.T) {
			d := p.Diagnostics()
			if got := d.TraceDeclarationArrayOperand; got != p.where {
				t.Errorf("TraceDeclarationArrayOperand = %v, want %v", got, p.where)
			}
			if got := d.TraceArrayOperandQuotesEveryElement; got != p.quoted {
				t.Errorf("TraceArrayOperandQuotesEveryElement = %v, want %v", got, p.quoted)
			}
		})
	}
}

// The trace itself, which is what the fields are for: a preset holding the
// right values and writing the wrong bytes would pass the test above.
func TestAnArrayOperandIsTracedInEveryDialect(t *testing.T) {
	for _, p := range arrayOperandPresets() {
		t.Run(p.Name, func(t *testing.T) {
			out, _, err := p.Combined(t, dialecttest.Base{}, arrayOperandProbe)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if out != p.want {
				t.Errorf("traced\n%s\nwant\n%s", out, p.want)
			}
			if p.wantSub == "" {
				return
			}
			out, _, err = p.Combined(t, dialecttest.Base{}, arrayOperandSubscriptProbe)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if out != p.wantSub {
				t.Errorf("subscripted\n%s\nwant\n%s", out, p.wantSub)
			}
		})
	}
}

// Where an **appending** literal's `+` goes, which is a third answer and
// follows the scalar field rather than the array one. Measured 2026-09-19 over
// `typeset u+=(3 4)`: ksh93u+ writes `+ u+=( 3 4 )` and then `+ typeset u+`,
// and bash 5.3.20 writes `+ u+=('3' '4')` and then `+ typeset u`.
//
// Only the traced lines are compared. ksh93 goes on to refuse the `u+` it left
// on the command line — `typeset: u+: invalid variable name` — which is a
// question about what the utility is handed rather than about what was
// written, and this shell does not hand it one.
func TestAnAppendingArrayOperandKeepsItsMarkerWhereTheColumnDoes(t *testing.T) {
	for _, c := range []struct {
		dialecttest.Preset
		want string
	}{
		{
			dialecttest.Preset{
				Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
				Diagnostics: bash.Diagnostics, Apply: bash.Apply,
			},
			"+ u+=('3' '4')\n+ typeset u\n",
		},
		{
			dialecttest.Preset{
				Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
				Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
			},
			"+ u+=( 3 4 )\n+ typeset u+\n",
		},
	} {
		t.Run(c.Name, func(t *testing.T) {
			out, _, err := c.Combined(t, dialecttest.Base{}, "set -x\ntypeset u+=(3 4)\n")
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			var traced strings.Builder
			for _, line := range strings.SplitAfter(out, "\n") {
				if strings.HasPrefix(line, "+ ") {
					traced.WriteString(line)
				}
			}
			if got := traced.String(); got != c.want {
				t.Errorf("traced\n%s\nwant\n%s", got, c.want)
			}
		})
	}
}
