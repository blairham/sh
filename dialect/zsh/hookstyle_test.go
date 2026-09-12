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
	if suffix := zsh.Semantics().HookListSuffix; suffix != "_functions" {
		t.Errorf("the list suffix is %q, want _functions", suffix)
	}
}

// The hook whose site is a builtin rather than the prompt loop.
//
// It is on the semantics vector and not in the table above because `cd` is
// where it fires, and `cd` is a builtin: measured, a `cd` inside a function
// and a `cd` in a `zsh -c` script with no prompt in sight both ran it, and a
// prompt-loop hook would have caught neither. #1775.
func TestTheDirectoryChangeHookIsNamed(t *testing.T) {
	if name := zsh.Semantics().DirectoryChangeHook; name != "chpwd" {
		t.Errorf("the directory-change hook is %q, want chpwd", name)
	}
}

// The hooks this shell has and this one does not fire are named rather than
// ignored, because a hook that is registered and never called quietly is the
// bug being fixed wearing a different name.
//
// Each is on the same *calling convention* — the named function, then the
// `_functions` array, measured — and on a firing site this loop does not
// reach: `periodic` on a timer, `zshaddhistory` where a line is saved.
//
// `chpwd` was the fourth until #1775 gave it its site inside `cd`, and
// `zshexit` the third until #2111 gave it its site at the end of a session. A
// hook that fires must not also be announced as one that does not, which is
// what the second loop below holds and what the exit hook's own test holds.
func TestTheHooksThisShellDoesNotFireAreNamed(t *testing.T) {
	want := map[string]bool{"periodic": true, "zshaddhistory": true}
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

// The other hook whose site is not the prompt loop: the end of the session.
//
// On the semantics vector for the reason `chpwd` is — every route out of a
// shell passes through interp's Finish and a script run with no prompt in
// sight fires it too, so a prompt loop firing it would miss every session that
// never had one. Announced as unfired until #2111, which is what a plugin
// registering a cleanup hook found on every start.
func TestTheExitHookIsNamedAndNoLongerAnnouncedAsUnfired(t *testing.T) {
	if name := zsh.Semantics().ExitHook; name != "zshexit" {
		t.Errorf("the exit hook is %q, want zshexit", name)
	}
	for _, name := range zsh.HookStyle().Unfired {
		if name == "zshexit" {
			t.Error("zshexit fires at the end of a session now, so a session must not " +
				"also announce it as a hook it will not run")
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
// The letter was free while this shell fired no `chpwd`: what `-q` asks for
// was already true, so cdOptions could grant it and be done. That stopped
// being sound the day the hook gained a site (#1775), and the tripwire that
// stood here — "chpwd is still unfired" — is now the assertion below: the
// hook exists, and the letter suppresses it.
//
// Two halves and both of them named, because a letter that is accepted and
// does nothing is exactly what #1558 was: the shell has a directory-change
// hook, and it has the letter that turns it off.
func TestCdQuietSuppressesTheDirectoryChangeHook(t *testing.T) {
	if zsh.Semantics().CdHasQuietOption != interp.Yes {
		t.Fatalf("this shell has `cd -q`; the preset says %v", zsh.Semantics().CdHasQuietOption)
	}
	if zsh.Semantics().DirectoryChangeHook == "" {
		t.Fatal("`cd -q` is hook suppression and this shell names no hook to suppress")
	}
	for _, name := range zsh.HookStyle().Unfired {
		if name == "chpwd" {
			t.Error("chpwd fires from inside `cd` now, so a session must not also " +
				"announce it as a hook it will not run")
		}
	}
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
