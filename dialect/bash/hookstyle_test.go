// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
)

// What this shell runs between one command and the next.

// The one name, and which of the two mechanisms it is.
//
// Both halves are asserted because the field it is *not* in is the failure
// this table was written for: `PROMPT_COMMAND` holds command text, so a shell
// that put the name in BeforePrompt would look for a *function* called
// `PROMPT_COMMAND`, find none, and run nothing — which is #1458 again with the
// variable set and the loop reaching it.
func TestTheHookIsAVariableAndNotAFunction(t *testing.T) {
	h := bash.HookStyle()
	if h.BeforePromptVariable != "PROMPT_COMMAND" {
		t.Errorf("the evaluated prompt hook is %q, want PROMPT_COMMAND", h.BeforePromptVariable)
	}
	if h.BeforePrompt != "" {
		t.Errorf("this shell names a prompt *function* %q, and has none", h.BeforePrompt)
	}
}

// This shell has no hook after a line is read and before it runs.
//
// zsh's `preexec` has no counterpart here — measured, and it is why the two
// mechanisms are two fields rather than one: a shell with `PROMPT_COMMAND` has
// only the prompt half. A name here would be a function this shell would call
// and bash never calls.
func TestThisShellHasNoCommandHook(t *testing.T) {
	h := bash.HookStyle()
	if h.BeforeCommand != "" {
		t.Errorf("a command hook %q is named, and this shell has none", h.BeforeCommand)
	}
	if suffix := bash.Semantics().HookListSuffix; suffix != "" {
		t.Errorf("a list suffix %q is named, and this shell's hook is not a list of function names", suffix)
	}
}

// No hook fires where the working directory changed either.
//
// Measured 2026-09-10: a `chpwd` function defined in bash 5.3.15, in 3.2.57
// and under an argv[0] of `sh` ran on none of their `cd`s and none of them
// said anything. dash and ksh93 answer the same, and zsh alone does not — see
// interp.Semantics.DirectoryChangeHook. #1775.
func TestThisShellHasNoDirectoryChangeHook(t *testing.T) {
	if name := bash.Semantics().DirectoryChangeHook; name != "" {
		t.Errorf("a directory-change hook %q is named, and this shell has none", name)
	}
}

// Nothing is named as unfired, because every hook this shell has is the one it
// fires.
//
// The assertion is worth making rather than obvious: a name here would make
// the session complain, once, that a hook it *does* run is not implemented —
// the honest-refusal machinery pointed at something that works.
func TestThisShellHasNoUnfiredHooks(t *testing.T) {
	if got := bash.HookStyle().Unfired; len(got) != 0 {
		t.Errorf("this shell names %v as unfired, and fires every hook it has", got)
	}
}
