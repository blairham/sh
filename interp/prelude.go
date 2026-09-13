// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// The prelude's diagnostics seam.
//
// A dialect written as shell — the third extension route — could say
// everything a builtin says except *where*. A function has no way to reach
// the location this package puts in front of every diagnostic, so `pushd
// /nope` reported `cd`'s complaint from a line inside the prelude where every
// shell in the panel that has the builtin reports `pushd`'s from the caller's
// line, and five corpus rows had to be graded on status alone because their
// text could not be pinned (#603).
//
// Two rules close it, and they are one fact seen from two sides:
//
//	A function the prelude defined is the *shell* speaking rather than the
//	script, so while one runs, a diagnostic is located where the script
//	called it and is named after it — whatever raised it.
//
//	Such a function may raise one of its own with [diagnoseCommand], which is
//	the one command in this package that exists for a prelude rather than for
//	a script, and is therefore reachable only from inside one of them.
//
// The name is the one the *script* named. A prelude helper another prelude
// function calls does not take it over: `pushd +9` reports `pushd` and not
// the `__dirs_rotate` that found the index out of range, which is what the
// shells with the builtin say.
//
// What it deliberately does not do is make such a function a builtin in any
// other respect. `type pushd` still answers `function`, because it is one —
// this is about whose diagnostic it is, which is the question the location
// already asks.

// diagnoseCommand is the seam's name in shell.
//
// Kept out of the builtin table on purpose: a name in there is a name a
// script can run, and a bash script calling `diagnose` must get bash's
// answer — `command not found` — rather than ours. The lookup answers it
// only while a prelude function is on the stack, so it exists for the
// dialect's text and for nothing else.
const diagnoseCommand = "diagnose"

// SourcingPrelude marks what the runner reads next as the dialect's own text.
//
// The front end sets it around the prelude and takes it off afterwards; see
// driver.Shell.source, which is the one place a prelude is installed.
//
// Only a function defined while it is on speaks for the shell, and the
// declaration is remembered rather than the name, so redefining one takes the
// voice away: `pushd() { cd /nope; }` in an rc file then reports `cd` from
// the function's own line, which is what bash does for a function shadowing a
// builtin.
func (r *Runner) SourcingPrelude(on bool) { r.sourcingPrelude = on }

// preludeDefined records a function as the dialect's own, and is called for
// every definition the runner makes.
func (r *Runner) preludeDefined(name string, decl *syntax.FuncDecl) {
	if !r.sourcingPrelude {
		return
	}
	if r.preludeFuncs == nil {
		r.preludeFuncs = map[string]*syntax.FuncDecl{}
	}
	r.preludeFuncs[name] = decl
}

// speaksForTheShell reports whether calling this declaration makes the shell
// itself the author of what follows.
//
// The *declaration* is what is compared, and that one word is the whole of the
// "a redefinition takes the voice away" rule: whatever route a later definition
// arrives by — a script, an rc file, a function imported out of the
// environment, and this package does not see those from one place — the runner
// is holding a different declaration afterwards and this stops matching.
// Deleting the old entry when a script defines the name says the same thing a
// second way for the one route that happens to pass through funcDecl, and a
// rule with two mechanisms is a rule that can disagree with itself. It was
// written that way and taken out: a mutant that dropped the delete changed
// nothing any test could see.
func (r *Runner) speaksForTheShell(fn *syntax.FuncDecl) bool {
	return fn != nil && r.preludeFuncs[fn.Name] == fn
}

// scriptFuncNames is every function a listing of "what functions exist" is
// asking about: the script's own, sorted, with the prelude's left out.
//
// A listing is how a caller captures a shell's state and sources it back
// later, so the prelude's functions being in it is not a cosmetic surplus —
// it records the shell's own implementation as the person's, hands
// `__dirs_rotate` to every later shell as though somebody had written it, and
// redefines `pushd` on top of the prelude's on every command (#1035).
//
// It asks [Runner.speaksForTheShell] and nothing else. There is deliberately
// no second record of prelude-ness for a listing to consult: the one already
// here compares the *declaration*, so a script that redefines `pushd` is in
// the listing from that moment — its function is its own, by the same rule
// that moves the diagnostic's voice back to it. A parallel flag set at
// definition time would have to be cleared on every route a redefinition can
// arrive by, and this package does not see those from one place (#603).
//
// A name *asked for* is still answered: `declare -f pushd` says the prelude's
// function back, because there is one and `type pushd` already says so. That
// is the same call #603 made — a prelude function is the shell speaking, and
// not a builtin in any other respect — rather than a second answer to whether
// the name is a function. Real bash has the three as builtins and refuses
// `declare -f pushd` with 1; the divergence is recorded in
// docs/spec/semantics.md, where the reachability it buys is written down.
func (r *Runner) scriptFuncNames() []string {
	names := make([]string, 0, len(r.funcs))
	for name, fn := range r.funcs {
		if r.speaksForTheShell(fn) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// preludePrivatePrefix is how the dialect's own text says a function is
// machinery rather than a name the shell has.
//
// A prelude is sourced, so everything in it is a function — including the
// helpers the presented ones are built out of. `pushd` is a name real bash
// has and `__dirs_rotate` is not, so a script asking about the second is
// asking about nothing, and answering `__dirs_rotate is a function` reports a
// name no shell in the panel has ever had (#2464).
//
// The mark is the name itself rather than a second table of prelude-ness, and
// that is the same call [Runner.scriptFuncNames] makes for the same reason
// (#1035, #603): a parallel record would have to be cleared on every route a
// redefinition can arrive by, and this package does not see those from one
// place. A name carries its own mark wherever it goes, so there is nothing to
// keep in step.
//
// It reaches the *prelude's* declarations only. A script's own `__helper` is
// the script's, is visible, and shadowing a private helper by writing one
// makes the name the script's by the rule that already moves the diagnostic's
// voice — [Runner.speaksForTheShell] is what both ask.
const preludePrivatePrefix = "__"

// hiddenPreludeFunc reports whether name is one the prelude uses rather than
// one it presents.
func (r *Runner) hiddenPreludeFunc(name string, fn *syntax.FuncDecl) bool {
	return r.speaksForTheShell(fn) && strings.HasPrefix(name, preludePrivatePrefix)
}

// reportedFunc is the function table as a question *about a name* sees it:
// the private prelude helpers are not there.
//
// Every builtin that answers "what is this name" goes through it — `type`,
// `command -v`, `whence`, and a named `declare -f` — so the shell gives one
// answer to whether a name exists rather than one per builtin, which is the
// split #2464 reports: `compgen -A function __dirs_rotate` was already right
// while `type __dirs_rotate` was not, from two lookups of one table.
//
// Calling is deliberately untouched. A private helper is machinery the
// presented functions run, and hiding it from a *report* is not the same as
// taking it away — `pushd +9` still reaches `__dirs_rotate`, which is what
// makes the name worth having at all.
func (r *Runner) reportedFunc(name string) (*syntax.FuncDecl, bool) {
	fn, ok := r.funcs[name]
	if !ok || r.hiddenPreludeFunc(name, fn) {
		return nil, false
	}
	return fn, true
}

// speaking is the name a diagnostic belongs to: the prelude function the
// script called where there is one, and the builtin that is running
// otherwise.
func (r *Runner) speaking() string {
	if r.speaker != "" {
		return r.speaker
	}
	return r.inBuiltin
}

// biDiagnose writes a diagnostic for the prelude function that is speaking.
//
// Located and named by the same code every builtin's complaint goes through,
// which is the whole point: one line in the prelude renders as `sh: line 3:
// popd: directory stack empty` under one dialect and `sh:popd:3: directory
// stack empty` under another, with the prelude saying neither.
//
// It reports and does not decide. The status is 0 because a nonzero one would
// end a script under `set -e` at the report rather than at the refusal the
// report is about — the function says `return 1` on the line after, and that
// is the failure.
//
// Operands are joined with a space, as `echo` joins them. With none there is
// nothing to report and nothing is written.
func biDiagnose(r *Runner, _ context.Context, args []string) int {
	if len(args) == 0 {
		return 0
	}
	r.diagf("%s: %s\n", r.speaking(), strings.Join(args, " "))
	return 0
}
