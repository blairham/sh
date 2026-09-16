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

// What `set -x` writes for an assignment standing in front of a command, in
// all five dialects — the split #3133 was filed from, where this shell wrote
// nothing at all in any of them.
//
// Measured 2026-09-16, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> x.sh` over
// a script file holding `f() { :; }`, `set -x`, and then the three commands
// below. Rendered a word at a time so that a column joining its words could
// not read as agreement with one that writes them on separate lines:
//
//	shell                     	A=3 f zz              	B=4 /usr/bin/env true    	D=6 E=7 :
//	bash 5.3.20, as-sh, 3.2.57	`+ A=3` `+ f zz`      	`+ B=4` `+ /usr/bin/env…`	`+ D=6` `+ E=7` `+ :`
//	ksh93u+ 2012-08-01        	`+ A=3` `+ f zz`      	`+ /usr/bin/env…` `+ B=4`	`+ D=6` `+ E=7` `+ :`
//	zsh 5.9.2                 	`+p> A=3 +p> f zz`    	`+p> B=4 /usr/bin/env…`  	`+p> D=6 E=7 +p> :`
//	dash 0.5.12               	`+ A=3 f zz`          	`+ B=4 /usr/bin/env true`	`+ D=6 E=7 :`
//	BusyBox ash 1.37.0        	`+ A=3 f zz`          	`+ B=4 /usr/bin/env true`	`+ D=6 E=7 :`
//
// Three shapes and four answers. The two columns that look alike in the first
// cell are not alike — ksh93 writes the *command* first and the assignment
// once its value has expanded, which the middle cell shows — and the two that
// share the third cell are not alike either, since zsh writes its trace prefix
// a second time between the assignments and the words.
//
// The probe here uses `true` in place of an external and `:` in place of a
// run of two, which asks ksh93's split the same way without a binary: the
// prefix goes behind a regular builtin and stays in front of a special one.
func xtracePrefixPresets() []struct {
	dialecttest.Preset
	style interp.TracePrefixAssignment
	want  string
} {
	return []struct {
		dialecttest.Preset
		style interp.TracePrefixAssignment
		want  string
	}{
		{dialecttest.Preset{
			Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
			Diagnostics: bash.Diagnostics, Apply: bash.Apply,
		}, interp.TracePrefixOwnLineBefore, "+ A=3\n+ f zz\n+ :\n+ B=4\n+ true\n+ C=5\n+ :\n"},
		{dialecttest.Preset{
			Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
			Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
		}, interp.TracePrefixOwnLineAfter, "+ A=3\n+ f zz\n+ :\n+ true\n+ B=4\n+ C=5\n+ :\n"},
		{
			dialecttest.Preset{
				Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
				Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
			},
			interp.TracePrefixOnTheCommandLineRepeatingThePrefix,
			"+zsh:3> A=3 +zsh:3> f zz\n+f:0> :\n+zsh:4> B=4 +zsh:4> true\n+zsh:5> C=5 +zsh:5> :\n",
		},
		{dialecttest.Preset{
			Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
			Diagnostics: dash.Diagnostics, Apply: dash.Apply,
		}, interp.TracePrefixOnTheCommandLine, "+ A=3 f zz\n+ :\n+ B=4 true\n+ C=5 :\n"},
		{dialecttest.Preset{
			Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
			Diagnostics: ash.Diagnostics, Apply: ash.Apply,
		}, interp.TracePrefixOnTheCommandLine, "+ A=3 f zz\n+ :\n+ B=4 true\n+ C=5 :\n"},
	}
}

func TestEachDialectAnswersThePrefixTraceShape(t *testing.T) {
	for _, p := range xtracePrefixPresets() {
		t.Run(p.Name, func(t *testing.T) {
			if got := p.Diagnostics().TracePrefixAssignment; got != p.style {
				t.Errorf("TracePrefixAssignment = %v, want %v", got, p.style)
			}
		})
	}
}

// The trace itself, which is what the field is for: a preset holding the
// right value and writing the wrong bytes would pass the test above.
func TestThePrefixIsTracedInEveryDialect(t *testing.T) {
	for _, p := range xtracePrefixPresets() {
		t.Run(p.Name, func(t *testing.T) {
			out, _, err := p.Combined(t, dialecttest.Base{},
				"f() { :; }\nset -x\nA=3 f zz\nB=4 true\nC=5 :\n")
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if out != p.want {
				t.Errorf("traced\n%s\nwant\n%s", out, p.want)
			}
		})
	}
}
