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

// `$_` across an `eval`, in all five dialects —
// Semantics.UnderscoreHoldsTheCallAcrossEvalAndSource.
//
// The same question the function-call frame beside this answers, for the two
// builtins that are a function call in everything but name. It splits the
// panel differently, which is why it is an axis of its own: the *caller* after
// a function call is unanimous, and the caller after an `eval` is not.
//
// Measured 2026-09-23, `env -i PATH=/usr/bin:/bin <shell> x.sh` over a script
// file, with `: SEED` on the line above and `$_` read inside the text and on
// the line below:
//
//	                           	inside	after
//	bash 5.3.20                	`first`	`: first; …`
//	ksh93u+ 2012-08-01         	`SEED` 	`: first; …`
//	zsh 5.9                    	`first`	`first`
//	dash 0.5.12, BusyBox ash   	empty — no such parameter
//
// bash and ksh93 land on the same "after" by two different routes, and only
// one of them is this axis: ksh93's `$_` moves only between the commands the
// shell *reads*, so nothing inside the text ever wrote — which its "inside"
// cell is the evidence for, and which is why the axis is not asked there at
// all. See Runner.underscoreAcrossABuiltinsOwnCommands.
//
// zsh alone lets the text write through, and it is the reading this shell had
// in every dialect before the axis existed.
func underscoreEvalPresets() []struct {
	dialecttest.Preset
	holds interp.Answer
	want  string
} {
	return []struct {
		dialecttest.Preset
		holds interp.Answer
		want  string
	}{
		{
			dialecttest.Preset{
				Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
				Diagnostics: bash.Diagnostics, Apply: bash.Apply,
			},
			interp.Yes,
			"inside [first] after [: first; printf 'inside [%s] ' \"$_\"]\n",
		},
		{
			dialecttest.Preset{
				Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
				Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
			},
			interp.Yes,
			"inside [SEED] after [: first; printf 'inside [%s] ' \"$_\"]\n",
		},
		{
			dialecttest.Preset{
				Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
				Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
			},
			interp.No,
			"inside [first] after [first]\n",
		},
		{
			dialecttest.Preset{
				Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
				Diagnostics: dash.Diagnostics, Apply: dash.Apply,
			},
			interp.Yes,
			"inside [] after []\n",
		},
		{
			dialecttest.Preset{
				Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
				Diagnostics: ash.Diagnostics, Apply: ash.Apply,
			},
			interp.Yes,
			"inside [] after []\n",
		},
	}
}

// The probe: an `eval` whose text moves `$_` twice over, read once inside and
// once after. The text is quoted with `'` so the reading shell expands nothing
// of it, and the `printf` inside it is what makes the "after" cell
// discriminating — a text whose last command took no argument would leave the
// same value either way.
const underscoreEvalProbe = ": SEED\n" +
	"eval ': first; printf '\\''inside [%s] '\\'' \"$_\"'\n" +
	"printf 'after [%s]\\n' \"$_\"\n"

func TestEachDialectAnswersTheUnderscoreEvalAxis(t *testing.T) {
	for _, p := range underscoreEvalPresets() {
		t.Run(p.Name, func(t *testing.T) {
			if got := p.Semantics().UnderscoreHoldsTheCallAcrossEvalAndSource; got != p.holds {
				t.Errorf("UnderscoreHoldsTheCallAcrossEvalAndSource = %v, want %v", got, p.holds)
			}
		})
	}
}

// The bytes, which is what the field is for: dash and ash hold Yes and write
// the empty cells anyway, because they keep no `$_` at all and the question is
// never reached there — a preset holding the right value and writing the wrong
// bytes would pass the test above.
func TestTheUnderscoreEvalFrameInEveryDialect(t *testing.T) {
	for _, p := range underscoreEvalPresets() {
		t.Run(p.Name, func(t *testing.T) {
			out, _, err := p.Combined(t, dialecttest.Base{}, underscoreEvalProbe)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if out != p.want {
				t.Errorf("wrote %q, want %q", out, p.want)
			}
		})
	}
}

// And a precommand word in front reaches the same answer, which is measured
// rather than assumed: `command eval`, `builtin eval`, `command -p eval` and
// `command command eval` each leave the call's own last argument in bash
// 5.3.20, and each left the text's last command here before this. The value is
// the same either way round — a precommand word's arguments are the inner call
// written out — so one bracket at the outermost call covers all four.
//
// bash alone, because the other columns cannot be asked the question in this
// shape and that is measured too, not assumed: zsh's `command` searches for an
// external and answers `command not found: eval` (zsh 5.9 and this shell, in
// the same words), and ksh93 refuses `builtin eval` outright. dash and ash
// keep no `$_`. A table row for any of them would be a row about `command`,
// not about this axis.
func TestThePrecommandWordDoesNotLoseTheUnderscoreFrame(t *testing.T) {
	p := underscoreEvalPresets()[0]
	if p.Name != "bash" {
		t.Fatalf("the first preset is %q, want bash", p.Name)
	}
	for _, front := range []string{"command ", "builtin ", "command -p ", "command command "} {
		out, _, err := p.Combined(t, dialecttest.Base{},
			": SEED\n"+front+"eval ': first'\nprintf '[%s]' \"$_\"\n")
		if err != nil {
			t.Fatalf("%q: err %v: %s", front, err, out)
		}
		if want := "[: first]"; out != want {
			t.Errorf("%q wrote %q, want %q", front, out, want)
		}
	}
}
