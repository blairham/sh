// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What this shell runs between one command and the next, and what it says
// about the ones it does not run.

// The two hooks and the suffix their lists are spelled with.
//
// The suffix is the load-bearing one: `add-zsh-hook precmd f` defines no
// function called `precmd`, it appends `f` to `precmd_functions`, so a session
// reading only the named function finds a correctly registered hook and runs
// nothing — which is #1281.
func TestTheHookNames(t *testing.T) {
	h := zsh.HookStyle()
	if h.BeforePrompt != "precmd" {
		t.Errorf("the prompt hook is %q, want precmd", h.BeforePrompt)
	}
	if h.BeforeCommand != "preexec" {
		t.Errorf("the command hook is %q, want preexec", h.BeforeCommand)
	}
	if h.ListSuffix != "_functions" {
		t.Errorf("the list suffix is %q, want _functions", h.ListSuffix)
	}
}

// The hooks this shell has and this one does not fire are named rather than
// ignored, because a hook that is registered and never called quietly is the
// bug being fixed wearing a different name.
//
// Each is on the same *calling convention* — the named function, then the
// `_functions` array, measured — and on a firing site this loop does not
// reach: `chpwd` where the directory changed, `periodic` on a timer,
// `zshaddhistory` where a line is saved, `zshexit` on the way out.
func TestTheHooksThisShellDoesNotFireAreNamed(t *testing.T) {
	want := map[string]bool{"chpwd": true, "periodic": true, "zshaddhistory": true, "zshexit": true}
	got := map[string]bool{}
	for _, name := range zsh.HookStyle().Unfired {
		got[name] = true
	}
	for name := range want {
		if !got[name] {
			t.Errorf("%s is not named as unfired", name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("%s is named as unfired and is not one of this shell's hooks", name)
		}
	}
}

// The tripwire for `cd -q`, which is the *whole* of what that letter does.
//
// Measured 2026-09-08 in zsh 5.9.2: a `chpwd` function and a name in
// `chpwd_functions` both ran on a plain `cd` and neither ran on `cd -q`, and
// nothing else changed — `cd -q -` still printed the directory at a prompt
// and a CDPATH move stayed silent either way. So `-q` is hook suppression and
// nothing besides.
//
// This shell fires no `chpwd`, which is why interp's cdOptions can grant the
// letter and be done: what `-q` asks for is already true here. That reasoning
// is only sound while the two halves stay in step, and this is what keeps
// them there. If `chpwd` leaves this list — because it gained a firing site
// inside `cd` — the suppression stops being free and cdOptions has to carry
// the flag to that site. Failing here rather than silently is the difference
// between a letter that is honored and one that is swallowed. #1558.
func TestChpwdIsUnfiredWhichIsWhatMakesCdQuietHonest(t *testing.T) {
	if zsh.Semantics().CdHasQuietOption != interp.Yes {
		t.Fatalf("this shell has `cd -q`; the preset says %v", zsh.Semantics().CdHasQuietOption)
	}
	for _, name := range zsh.HookStyle().Unfired {
		if name == "chpwd" {
			return
		}
	}
	t.Error("chpwd now has a firing site, so `cd -q` has something to suppress: " +
		"interp/builtin.go's cdOptions grants the letter and does nothing with it, " +
		"and that is only correct while this is true. Carry the flag to the new site.")
}

// The command hook's third argument is the line about to run, laid out the way
// this shell lays a function out.
//
// Measured on 2026-09-07 through a pseudo-terminal, zsh 5.9.2, typing
// `for i in 1 2; do echo $i; done` at a prompt with `preexec` defined: `$3`
// came back as the four lines below — `do` on a line of its own, one tab of
// indent, no terminators. That is FunctionLayout, which is why the hook table
// names it rather than spelling a second arrangement that could drift from it.
func TestTheCommandHooksManyLineFormIsTheFunctionLayout(t *testing.T) {
	f, err := syntax.Parse("for i in 1 2; do echo $i; done\n", zsh.Dialect())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	const want = "for i in 1 2\ndo\n\techo $i\ndone"
	if got := syntax.PrintFileWith(f, zsh.HookStyle().CommandLayout); got != want {
		t.Errorf("the third argument would be %q, want %q", got, want)
	}
}
