// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The prompt theme engine's seam in shell.
//
// docs/spec/prompt-theme.md asks for an *asked* half beside the volunteered
// report: what a setting resolves to and which layer it came from, printed on
// request. A person reaches that by typing a word, and the word this shell
// answers to is `prompt`.
//
// # Why this is not a builtin
//
// Decided by the maintainer, 2026-09-21. A `prompt` **builtin** would be a
// name real bash, zsh, ksh, dash and BusyBox ash do not have, and since #1117
// there is exactly one table deciding what a name reports as — so it would be
// visible to `type`, `type -t`, `type -a`, `command -v`, `command -V`,
// `compgen -A builtin`, zsh's `whence -w` and its `which`, all at once. That
// is an observable divergence from the shell each dialect binary claims to be,
// and the answer is the one this repository already has for "a capability the
// core provides and no builtin should carry": the prelude, which is the third
// extension route in docs/design.md and the one `pushd`, `popd` and `dirs`
// are shipped through.
//
// So the *name* a person types is a function, and this is what that function
// runs. The split matters in both directions: a prelude function is subject to
// the same table, so `prompt` is presented as a builtin exactly as `pushd` is —
// an already-measured divergence of a kind this shell already has, rather than
// a new one — while the Go side of it stays out of every listing because it is
// not in the builtin table at all.
//
// # Reachable only from inside the prelude
//
// [diagnoseCommand] is the precedent and this is the second member of that
// family: a name the lookup answers only while a function the prelude defined
// is on the stack. A script that runs the word gets what the dialect it is
// written for would give it, which is a command that was not found — so
// nothing here widens the surface a script can see, and a person who writes
// their own `prompt` function shadows ours and reaches none of this.
const promptEngineCommand = "promptengine"

// SetPromptEngine installs what the prelude's `prompt` function runs.
//
// Nil in a library, and filled in by the front end — the same shape as
// [Runner.ReplaceProcess] and [Runner.DieBySignal], and for the same reason.
// The engine is the front end's: it draws on a terminal, reads a
// configuration file and holds a session's theme, none of which is this
// package's business. What is this package's business is that a word typed at
// a prompt reaches it, and that no other word does.
//
// A runner with none installed does not answer the name at all, so the
// prelude function reports a command that was not found rather than a shell
// that silently did nothing. A front end that wires the function wires this.
func (r *Runner) SetPromptEngine(fn Builtin) { r.promptEngine = fn }

// HasPromptEngine reports whether one is installed, for a front end composing
// a prelude: the function is worth defining only where there is something
// behind it.
func (r *Runner) HasPromptEngine() bool { return r.promptEngine != nil }

// promptEngineBuiltin is the lookup's answer for [promptEngineCommand], or
// nil where there is nothing to answer with.
//
// The speaker check is [diagnoseCommand]'s and is the whole of the narrowing:
// r.speaker is non-empty only while a function the prelude defined is
// running, so this name exists for the dialect's own text and for nothing
// else.
func (r *Runner) promptEngineBuiltin(name string) (Builtin, bool) {
	if name != promptEngineCommand || r.speaker == "" || r.promptEngine == nil {
		return nil, false
	}
	return r.promptEngine, true
}

// PromptEngineName is the word the prelude's `prompt` function runs, exported
// so that a front end writing that function does not spell it a second time.
//
// One place the name is written down, for the reason `preludePrivatePrefix`
// is one place: two spellings of one name is a pair that can drift, and the
// drift would be a prelude function calling a command that is not there.
const PromptEngineName = promptEngineCommand
