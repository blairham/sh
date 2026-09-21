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
// What it deliberately does not decide is whether such a function is a
// *builtin*. That is the next question over, and [Runner.presentedBuiltin]
// is where it is answered: a name the dialect's prelude presents is one this
// shell has, and every surface that reports on a name says so (#1117).

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
// Turning it **off** also forgets what line the prelude's own text reached,
// which matters for exactly one construct: a `case` subject in the dialect
// that expands it before the line advances reads the line the command in
// front of it was on, and with the prelude's twenty-odd lines still recorded
// that was a line of the dialect's plumbing rather than of the script. See
// Runner.prevLine and Semantics.CaseSubjectKeepsThePreviousLine. Every other
// route sets the line from the node it is about before it says anything, so
// nothing else could see the difference.
//
// Runner.enteredLine is forgotten here for the same reason and is the second
// construct that can see it: the count is read as it stands rather than being
// set from a node, so a prelude of twenty-odd lines left a script whose very
// first statement raises the one diagnostic that reads it blaming line 22 of
// text the script never saw.
func (r *Runner) SourcingPrelude(on bool) {
	r.sourcingPrelude = on
	if !on {
		r.line, r.prevLine, r.enteredLine = 0, 0, 0
	}
}

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
// A name *asked for* is answered the way this shell answers for a builtin,
// which since #1117 is not the same thing as answering for a function:
// `declare -f pushd` writes nothing at status 1, because there is no function
// called `pushd` here any more than there is one in real bash. The lookup
// that says so is [Runner.reportedFunc], and the table it reads is
// [Runner.presentedBuiltin] — one table, read by `type` and by
// `compgen -A builtin` alike, which is #1035's rule kept rather than broken.
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

// presentedPreludeName reports whether name is one the prelude *presents* as
// a command of this shell's own, rather than machinery it runs.
//
// This is the table #1117 asked for, and the point is that it is not a new
// one: the prelude's declarations are already recorded, and
// [preludePrivatePrefix] already splits the ones a shell has from the ones it
// is built out of. Nothing is written down twice, so nothing can drift.
//
// The two halves of the split answer a name differently and neither answers
// it as a function. A private helper is nothing at all — real bash says
// `type: __dirs_rotate: not found` (#2464). A presented name is a builtin —
// real bash says `pushd is a shell builtin`, and measured 2026-09-20 on bash
// 5.3.20 it refuses `declare -f pushd` with 1, so the implementation being
// unreachable by name is conformance here rather than a cost (#1117).
//
// **It asks the prelude's own record and not the live function table**,
// which is the difference between a builtin and a function and is measured:
// `pushd() { echo mine; }` in real bash leaves `compgen -A builtin pushd`
// answering `pushd`, `builtin pushd /tmp` pushing, and `type -a pushd`
// writing the function *and* the builtin, because a function shadows a
// builtin rather than replacing it. A dialect written as shell has one table
// where those shells have two, so the boundary is reconstructed from the
// record the runner already keeps — the same record removeFunctionQuietly
// puts the declaration back from when the shadowing function goes. Which of
// the two a *report* names is still the declaration comparison's answer, and
// that is [Runner.reportedFunc].
func (r *Runner) presentedPreludeName(name string) bool {
	return r.preludeFuncs[name] != nil &&
		!strings.HasPrefix(name, preludePrivatePrefix)
}

// presentedBuiltin is presentedPreludeName narrowed to what running the word
// would actually find, which is the same narrowing [Runner.lookupBuiltin]
// makes for a builtin proper.
//
// Switched off is switched off whichever table the name is in. Measured
// 2026-09-20: `disable pushd; pushd /tmp` on zsh 5.9.2 is
// `command not found: pushd` at 127 and `whence -w pushd` is `pushd: none`,
// and bash 5.3.20 answers `type: pushd: not found` and
// `pushd: command not found` after `enable -n pushd`. A presented name that
// accepted the switch and went on running would be the silent wrong answer
// this project exists to avoid.
func (r *Runner) presentedBuiltin(name string) bool {
	return r.presentedPreludeName(name) &&
		!r.disabledBuiltins[name] && !r.withdrawnBuiltins[name]
}

// presentedButSwitchedOff is the state the dispatcher has to know about: the
// word would reach the shell's own implementation of a presented name, and
// the name has been switched off, so it must not. The shells with the
// builtin have two tables and get this for free.
//
// The declaration comparison is the first half on purpose. A script that
// writes its own `pushd` after switching the name off has written an
// ordinary function and it runs — measured 2026-09-20, `enable -n pushd;
// pushd() { echo mine; }; pushd` prints `mine` in bash 5.3.20, because the
// switch was over the builtin and the function is not one.
func (r *Runner) presentedButSwitchedOff(name string) bool {
	return r.speaksForTheShell(r.funcs[name]) &&
		r.presentedPreludeName(name) && !r.presentedBuiltin(name)
}

// presentsAsBuiltin is the question every report about a name asks: would
// this word run a command of the shell's own?
//
// One predicate for the two tables, so `type`, `command -v`, `command -V`,
// zsh's `whence -w` and its `which` cannot answer it one way while
// `compgen -A builtin` answers it the other — which is exactly the split
// #1117 reports and #1035's rule forbids.
func (r *Runner) presentsAsBuiltin(name string) bool {
	if _, ok := r.lookupBuiltin(name); ok {
		return true
	}
	return r.presentedBuiltin(name)
}

// presentedBuiltinNames is every name the prelude presents and has not had
// switched off, for the listings that name what this shell has.
func (r *Runner) presentedBuiltinNames() []string {
	var names []string
	for name := range r.preludeFuncs {
		if r.presentedBuiltin(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// presentedPreludeDecl is the declaration behind a presented name, for the
// two words that ask for the shell's own command and get a function here
// because a dialect written as shell has nowhere else to put one: `builtin
// pushd /tmp` and, in the dialect whose `command` reaches a builtin at all,
// `command pushd /tmp`. Both push in real bash and both were refused here.
// The prelude's own declaration, not whatever the function table holds: a
// script that wraps `pushd` and calls `builtin pushd` from inside the
// wrapper must reach the shell's implementation and not itself, which is the
// entire reason the spelling exists.
func (r *Runner) presentedPreludeDecl(name string) (*syntax.FuncDecl, bool) {
	if !r.presentedBuiltin(name) {
		return nil, false
	}
	fn := r.preludeFuncs[name]
	return fn, fn != nil
}

// reportedFunc is the function table as a question *about a name* sees it:
// nothing the dialect's prelude defined is in it.
//
// Every builtin that answers "what is this name" goes through it — `type`,
// `command -v`, `whence`, `which`, and a named `declare -f` — so the shell
// gives one answer to whether a name exists rather than one per builtin,
// which is the split #2464 reports: `compgen -A function __dirs_rotate` was
// already right while `type __dirs_rotate` was not, from two lookups of one
// table.
//
// **Every prelude declaration and not only the private ones**, which is
// #1117's change and the point where #603's call was overturned. A prelude
// function is how a dialect written as shell spells a builtin, and neither
// half of the prelude is a function to a shell that is asked: a presented
// name is reported as a builtin by [Runner.presentsAsBuiltin], and a private
// helper is reported as nothing. Real bash refuses `declare -f pushd` with 1
// for precisely this reason — there is a builtin of that name and no
// function — so the body this used to write back was itself the divergence.
//
// Calling is deliberately untouched, and it is what makes the name worth
// having: `pushd +9` still reaches `__dirs_rotate`, and `pushd /tmp` still
// runs the prelude's implementation.
func (r *Runner) reportedFunc(name string) (*syntax.FuncDecl, bool) {
	fn, ok := r.funcs[name]
	if !ok || r.speaksForTheShell(fn) {
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
