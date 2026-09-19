// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// What `set -x` writes for a declaration utility's `name=value` operand, in
// all five dialects — the split #3567 was filed from, where every column wrote
// the operand on the command line and nothing else.
//
// Measured 2026-09-19, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> x.sh` over
// a script file holding `set -x` and then the two commands below:
//
//	shell                     	export ev=1 ew=2                 	readonly rv=5
//	bash 5.3.20, 3.2.57       	the line / `+ ev=1` / `+ ew=2`   	the line / `+ rv=5`
//	ksh93u+ 2012-08-01        	`+ ev=1` `+ ew=2` `+ export ev ew`	`+ rv=5` `+ readonly rv`
//	zsh 5.9.2                 	one line                         	one line
//	dash 0.5.12               	one line                         	one line
//	BusyBox ash 1.37.0        	one line, operands quoted as words	one line
//
// The ksh93 row was measured twice — 93u+ 2012-08-01 on macOS and the same
// version on Debian bullseye for linux/arm64 — because the whole of that
// column is one build otherwise, and the two agree line for line.
//
// Two operands on one command rather than one, which is what says the trailing
// and leading lines are per *operand*: a shape that repeated the command would
// write one line for `export ev=1 ew=2` in the column that writes two.
const declarationOperandProbe = "set -x\nexport ev=1 ew=2\nreadonly rv=5\n"

func xtraceDeclarationPresets() []struct {
	dialecttest.Preset
	where  interp.TraceDeclarationOperand
	repeat []string
	want   string
} {
	return []struct {
		dialecttest.Preset
		where  interp.TraceDeclarationOperand
		repeat []string
		want   string
	}{
		{
			dialecttest.Preset{
				Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
				Diagnostics: bash.Diagnostics, Apply: bash.Apply,
			},
			interp.TraceOperandOnTheCommandLine,
			[]string{"export", "readonly"},
			"+ export ev=1 ew=2\n+ ev=1\n+ ew=2\n+ readonly rv=5\n+ rv=5\n",
		},
		{
			dialecttest.Preset{
				Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
				Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
			},
			interp.TraceOperandSplitBefore, nil,
			"+ ev=1\n+ ew=2\n+ export ev ew\n+ rv=5\n+ readonly rv\n",
		},
		{
			dialecttest.Preset{
				Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
				Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
			},
			interp.TraceOperandOnTheCommandLine, nil,
			"+zsh:2> export ev=1 ew=2\n+zsh:3> readonly rv=5\n",
		},
		{
			dialecttest.Preset{
				Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
				Diagnostics: dash.Diagnostics, Apply: dash.Apply,
			},
			interp.TraceOperandOnTheCommandLine, nil,
			"+ export ev=1 ew=2\n+ readonly rv=5\n",
		},
		{
			dialecttest.Preset{
				Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
				Diagnostics: ash.Diagnostics, Apply: ash.Apply,
			},
			interp.TraceOperandOnTheCommandLine, nil,
			"+ export 'ev=1' 'ew=2'\n+ readonly 'rv=5'\n",
		},
	}
}

func TestEachDialectAnswersWhereADeclarationOperandIsTraced(t *testing.T) {
	for _, p := range xtraceDeclarationPresets() {
		t.Run(p.Name, func(t *testing.T) {
			d := p.Diagnostics()
			if got := d.TraceDeclarationOperand; got != p.where {
				t.Errorf("TraceDeclarationOperand = %v, want %v", got, p.where)
			}
			got := d.TraceRepeatsAScalarOperandAfter
			if len(got) != len(p.repeat) {
				t.Fatalf("TraceRepeatsAScalarOperandAfter = %q, want %q", got, p.repeat)
			}
			for i, name := range p.repeat {
				if got[i] != name {
					t.Errorf("TraceRepeatsAScalarOperandAfter[%d] = %q, want %q", i, got[i], name)
				}
			}
		})
	}
}

// The trace itself, which is what the fields are for: a preset holding the
// right values and writing the wrong bytes would pass the test above.
func TestADeclarationOperandIsTracedInEveryDialect(t *testing.T) {
	for _, p := range xtraceDeclarationPresets() {
		t.Run(p.Name, func(t *testing.T) {
			out, _, err := p.Combined(t, dialecttest.Base{}, declarationOperandProbe)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if out != p.want {
				t.Errorf("traced\n%s\nwant\n%s", out, p.want)
			}
		})
	}
}

// The subscript goes with the assignment written in front and the word left on
// the command line is the bare name — measured on ksh93u+ 2012-08-01, which is
// the only column that splits, so it is asserted on that dialect alone.
func TestASplitOperandLeavesTheNameWithoutItsSubscript(t *testing.T) {
	p := dialecttest.Preset{
		Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
		Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
	}
	out, _, err := p.Combined(t, dialecttest.Base{},
		"typeset -a a\nset -x\ntypeset a[1]=v\n")
	if err != nil {
		t.Fatalf("err %v: %s", err, out)
	}
	const want = "+ a[1]=v\n+ typeset a\n"
	if out != want {
		t.Errorf("traced\n%s\nwant\n%s", out, want)
	}
}
