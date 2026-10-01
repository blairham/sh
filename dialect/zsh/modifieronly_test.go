// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// modifierOnlyRows are lines whose words are only precommand modifiers, each
// measured 2026-10-01 on zsh 5.9.2 under `-f -c` byte for byte (#5137). The
// last two are the controls: a bare assignment empties `$_`, and a word
// behind the modifier is a command that sets it.
var modifierOnlyRows = []struct{ name, src, want string }{
	{"builtin keeps the prefix and leaves $_", `: MARK; v=1 builtin; print -r -- "_=[$_] v=[$v]"`, "_=[MARK] v=[1]\n"},
	{"exec", `: MARK; v=1 exec; print -r -- "_=[$_] v=[$v]"`, "_=[MARK] v=[1]\n"},
	{"noglob", `: MARK; v=1 noglob; print -r -- "_=[$_] v=[$v]"`, "_=[MARK] v=[1]\n"},
	{"the dash", `: MARK; v=1 -; print -r -- "_=[$_] v=[$v]"`, "_=[MARK] v=[1]\n"},
	{"nocorrect", `: MARK; v=1 nocorrect; print -r -- "_=[$_] v=[$v]"`, "_=[MARK] v=[1]\n"},
	{"builtin builtin", `: MARK; v=1 builtin builtin; print -r -- "_=[$_] v=[$v]"`, "_=[MARK] v=[1]\n"},
	{"command drops the line", `: MARK; v=1 command; print -r -- "_=[$_] v=[$v]"`, "_=[MARK] v=[]\n"},
	{"command and its own options", `: MARK; v=1 noglob command; v=1 command -p; v=1 command --; print -r -- "_=[$_] v=[$v]"`, "_=[MARK] v=[]\n"},
	{"no prefix", `: MARK; builtin; command; exec; noglob builtin; print -r -- "_=[$_]"`, "_=[MARK]\n"},
	{
		"the prefix's status, or none behind command",
		`false; v=$(exit 3) builtin; echo $?; false; v=$(exit 3) command; echo $?`, "3\n0\n",
	},
	{"command never expands the value", `v=$(echo side >&2) command; echo "[$v]"`, "[]\n"},
	{"control: a bare assignment", `: MARK; v=1; print -r -- "_=[$_] v=[$v]"`, "_=[] v=[1]\n"},
	{"control: command -v runs", `: MARK; v=1 command -v; print -r -- "_=[$_] v=[$v]"`, "_=[-v] v=[]\n"},
}

// **A line of modifiers with nothing behind them runs nothing and leaves `$_`
// where it was** (#5137). See the no-redirection half of the commandless rule
// in interp.Runner.simple.
func TestAModifierAloneRunsNothing(t *testing.T) {
	for _, c := range modifierOnlyRows {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}

// **And the prefix persists without the roster's help**: with
// BuiltinsKeepingAnAssignmentPrefix emptied, which is what `posixbuiltins`
// is to do to it (#5136), every row stands as it was — a line where nothing
// ran has nothing to take the value back.
func TestAModifierAloneKeepsItsPrefixWithoutTheRoster(t *testing.T) {
	bare := preset
	bare.Semantics = func() interp.Semantics {
		s := zsh.Semantics()
		s.BuiltinsKeepingAnAssignmentPrefix = ""
		return s
	}
	for _, c := range modifierOnlyRows {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := bare.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
			}
		})
	}
	// The control that the roster is silenced at all: `v=1 alias` keeps the
	// value through the roster and through nothing else.
	out, _, err := bare.Combined(t, dialecttest.Base{Dir: t.TempDir()}, `v=1 alias >/dev/null; print -r -- "v=[$v]"`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "v=[]\n" {
		t.Errorf("the roster is not silenced: got %q", out)
	}
}
