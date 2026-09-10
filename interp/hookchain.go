// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// A hook is a function a shell runs on its own account, at a moment no script
// names it: before a prompt, after a line is read, where the working directory
// changed. This file is the *chain* — what one hook's name expands to and how
// the list is run — and it is here rather than in a front end because the sites
// are on both sides of that line. `precmd` fires in a prompt loop and `chpwd`
// fires inside `cd`, which is a builtin, and a builtin cannot reach up into
// repl.
//
// One implementation, so a rule cannot be carried by one site and not another.
// Every rule below was a bug in some shell before it was a rule, and the four
// prompt-hook rules were already written twice here (#1281, #1301) before they
// were written once.
//
// # What was measured
//
// zsh 5.9.2, `/opt/homebrew/bin/zsh`, 2026-09-07 through a pseudo-terminal for
// the prompt hooks and 2026-09-10 with `-c` for the directory hook, which does
// not need a terminal. With `chpwd` defined and `chpwd_functions=(a b)`:
//
//	$ zsh -c 'chpwd() { print NAMED }; a() { print A }; b() { print B }
//	         chpwd_functions=(a b); cd /tmp'
//	NAMED
//	A
//	B
//
//   - The named function runs **first**, then the members of the list in the
//     order the list holds them.
//   - Neither is deduplicated. `chpwd_functions=(chpwd)` ran the named function
//     twice, and a name appearing twice in the list ran twice.
//   - A name with no function behind it is passed over **in silence**, and so
//     is a name that resolves to a builtin or to a file on `PATH`:
//     `chpwd_functions=(echo true /bin/echo)` ran nothing and said nothing. See
//     [Runner.CallFunction], which is where that rule lives.
//   - An **empty** element is passed over the same way.
//   - A **scalar** by the list's name is not a list: `chpwd_functions=a` ran
//     nothing, and `precmd_functions=a` ran nothing at the prompt either. The
//     list has to be an array.
//   - A hook that **fails** stops nothing. With the named hook returning 3 and
//     the first member returning 9, every later member still ran.
//   - A hook that calls `exit` is the one thing that stops the chain: the rest
//     went unrun and the status it set was kept.
//
// # `$?`
//
// A hook is told the status of the command before it and cannot change what
// the next command reads. After `(exit 5); cd /tmp` every member of the chain
// read `$?` as 5 — including the ones after a member that had returned 9 — so
// the status is put back **before each** call as well as after the last. That
// is two facts and not one: a chain that restored only at the end would show
// the second hook the first one's status, and zsh does not.

// HookChain is the names one hook calls, in order: its own, then its list's.
//
// The list is the hook's name plus [Semantics.HookListSuffix], and both halves
// matter: `add-zsh-hook chpwd f` leaves no function called `chpwd` behind — it
// appends `f` to `chpwd_functions` — so a caller that read only the named
// function would find a correctly registered hook and run nothing, which is
// #1281 exactly.
//
// Names rather than functions, and every name whether or not anything answers
// to it, because "is this a function" is [Runner.CallFunction]'s question and
// that is where it is asked. Answering it here would put the rule in two
// places and would also cost a caller that wants to know whether *anything*
// has been put in a hook's way — see repl's hookIsDefined, which walks these
// names and asks [Runner.HasFunction] about each.
func (r *Runner) HookChain(name string) []string {
	names := []string{name}
	suffix := r.sem().HookListSuffix
	if suffix == "" {
		return names
	}
	// A scalar is not a list, measured above, and arrayElemsOfTheName is the
	// reading that already draws that line — arrayElems answers a plain
	// string as a one-element array, which is right for `${x[0]}` and wrong
	// here. It is also the opposite reading from the one an *evaluated* hook
	// wants: bash's `PROMPT_COMMAND` is command text whether it holds one
	// string or many. Which is why the two chains differ in what they hand to
	// FireChain rather than in FireChain itself.
	list, ok := r.arrayElemsOfTheName(name + suffix)
	if !ok {
		return names
	}
	return append(names, list...)
}

// FireChain runs one chain of hook items and is the whole of what a hook
// mechanism is, apart from what an item *is*.
//
// Both kinds a shell has are a list run in order under the same four rules —
// the status is saved and put back around every item, a failing item stops
// nothing, an item that exited ends the chain, and each item runs through
// whatever the caller wraps it in — and only what one item is differs: a
// function to call, or command text to evaluate. So the rules live here once
// and the difference is the closure.
//
// Written this way rather than as a second loop beside the first because a
// second loop is how a rule comes to be carried by one caller and not the
// other.
func (r *Runner) FireChain(ctx context.Context, items []string, run func(item string)) {
	status := r.ExitStatus()
	for _, item := range items {
		r.SetExitStatus(status)
		run(item)
		if r.Exited() {
			return
		}
	}
	r.SetExitStatus(status)
}

// FireHook runs one *function* hook's whole chain: the function of that name,
// then every function named in its list, in order. An empty name is a shell
// without that hook and runs nothing.
//
// around is what each call is run through, and nil is the call itself. Nothing
// under interp/ recovers from a panic — that is internal/panicguard's opening
// paragraph and the reason the guard lives out in the front ends — so this
// package passes nil and a front end that has a guard passes one. Wrapped per
// item rather than around the whole loop, so one hook with a bug costs its own
// call and not the rest of the chain, which is also what a hook that merely
// *fails* does.
func (r *Runner) FireHook(ctx context.Context, around func(call func()), name string, args ...string) {
	if name == "" {
		return
	}
	if around == nil {
		around = func(call func()) { call() }
	}
	r.FireChain(ctx, r.HookChain(name), func(fn string) {
		around(func() { _, _ = r.CallFunction(ctx, fn, args...) })
	})
}
