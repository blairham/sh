// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
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
