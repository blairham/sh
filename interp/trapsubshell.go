// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "context"

// How traps cross into a subshell.
//
// POSIX is plain about the working state: entering a subshell puts every
// handled condition back to its default action, and an ignored signal stays
// ignored. All four shells do exactly that — measured, a handled USR1 no
// longer fires inside `( … )` anywhere, and `trap '' INT; (kill -INT $$;
// echo alive)` prints alive everywhere. None of that is an axis.
//
// What `trap` *lists* in the child is where the panel splits, and it splits
// by the kind of boundary rather than once. bash and ksh93 keep the parent's
// listing visible in `( … )` and `$( … )` — the save=$(trap) idiom POSIX
// carves out — and bash alone keeps it in a pipeline element and a
// background job, while zsh keeps it only in a pipeline element. Every shell
// that keeps it drops it the moment the child modifies a trap. dash lists
// only what survived the entry: the ignored signals. zsh lists least of all:
// an inherited ignore is working but unlisted, though one the child sets
// itself is shown. Measured, each cell, against all four.

// savedTrap is one entry of the listing a subshell inherited: the action and
// the canonical condition — "EXIT", a signal's name, or a pseudo-condition —
// rendered in the dialect's own spelling only when printed.
type savedTrap struct {
	action string
	cond   string
}

// trapContext is the kind of boundary a cloned runner was made for, so the
// axis that decides whether the parent's listing survives that boundary is
// asked when something lists — a clone must not be the thing that refuses a
// script over a question nothing asked.
type trapContext uint8

const (
	// trapContextSubshell is `( … )`, `$( … )`, and any boundary without a
	// kind of its own.
	trapContextSubshell trapContext = iota
	trapContextPipeline
	trapContextBackground
)

// inheritTraps gives a fresh clone the trap state a subshell starts with:
// its own table holding only the parent's ignored signals, no EXIT trap, the
// pseudo-traps marked as arrived rather than set, and the parent's listing
// as a snapshot for the dialects whose `trap` still shows it.
func (c *Runner) inheritTraps(r *Runner) {
	// The process machinery — arrivals, and a death a subshell caused — is
	// shared on purpose, and shared *now*: sigs creates it lazily, and a
	// clone taken before anything trapped would otherwise build a private
	// one, where a `kill -INT $$` aimed at the parent would be recorded for
	// nobody.
	c.signals = r.sigs()
	c.traps = map[string]string{}
	c.inheritedIgnored = map[string]bool{}
	// Nothing this subshell raised on itself yet, and nothing of the
	// parent's either: clone copies the runner by value, so a parent holding
	// an undelivered arrival of its own would otherwise hand a copy of it to
	// every child it started.
	c.selfPending = nil
	for name, action := range r.trapTable() {
		if action == "" {
			c.traps[name] = ""
			c.inheritedIgnored[name] = true
		}
	}
	c.exitTrap = nil
	c.errTrapInherited = c.errTrap != nil
	c.debugTrapInherited = c.debugTrap != nil
	c.returnTrapInherited = c.returnTrap != nil
	if r.trapSnapshot != nil {
		// The parent is itself a subshell still holding its parent's
		// listing. Whether that listing survived the *outer* boundary has
		// not been asked yet, so the chain grows rather than resolving: a
		// `(trap)` inside a pipeline element shows the original traps only
		// where both axes say keep, which is what ksh93's empty
		// `(trap) | cat` against its own populated `(trap)` pins down.
		c.trapSnapshot = r.trapSnapshot
		c.trapContexts = append(append([]trapContext{}, r.trapContexts...), trapContextSubshell)
		return
	}
	c.trapSnapshot = r.entryTrapListing()
	c.trapContexts = nil
	if c.trapSnapshot != nil {
		c.trapContexts = []trapContext{trapContextSubshell}
	}
}

// retagTrapBoundary names the kind of boundary this clone was actually made
// for. clone assumes a subshell; the pipeline, background and process
// substitution paths correct it, because the dialects keep the listing
// across different kinds.
func (r *Runner) retagTrapBoundary(ctx trapContext) {
	if len(r.trapContexts) != 0 {
		r.trapContexts[len(r.trapContexts)-1] = ctx
	}
}

// trapTable is the signal traps this runner answers for: its own when it
// stands for a subshell, the process's — which are the top-level shell's —
// otherwise.
func (r *Runner) trapTable() map[string]string {
	if r.traps != nil {
		return r.traps
	}
	return r.sigs().traps
}

// entryTrapListing is what this runner's `trap` would list right now, in
// listing order, recorded on the way into a subshell for the dialects that
// keep showing it there. Nil when there is nothing to list, so a script with
// no traps never asks anybody anything.
func (r *Runner) entryTrapListing() []savedTrap {
	var entries []savedTrap
	if r.exitTrap != nil {
		entries = append(entries, savedTrap{action: *r.exitTrap, cond: "EXIT"})
	}
	t := r.trapTable()
	for _, name := range sortedKeys(t) {
		entries = append(entries, savedTrap{action: t[name], cond: name})
	}
	for _, name := range []string{"DEBUG", "ERR", "RETURN"} {
		if action := *r.pseudoTrapSlot(name); action != nil {
			entries = append(entries, savedTrap{action: *action, cond: name})
		}
	}
	return entries
}

// keptTrapListing is the parent's listing when this dialect keeps it across
// every boundary this runner was cloned through, nil otherwise. refused
// reports that an axis had no answer, which the caller returns as the
// refusal it already is.
func (r *Runner) keptTrapListing() (entries []savedTrap, refused bool) {
	if r.trapSnapshot == nil {
		return nil, false
	}
	for _, ctx := range r.trapContexts {
		var keep Answer
		var what string
		switch ctx {
		case trapContextPipeline:
			keep = r.sem().PipelineElementKeepsTrapListing
			what = "`trap` in a pipeline element listing the parent's traps"
		case trapContextBackground:
			keep = r.sem().BackgroundJobKeepsTrapListing
			what = "`trap` in a background job listing the parent's traps"
		default:
			keep = r.sem().SubshellKeepsTrapListing
			what = "`trap` in a subshell listing the parent's traps"
		}
		if !r.ask(keep, what) {
			return nil, r.unspecified
		}
	}
	return r.trapSnapshot, false
}

// keptTrapAction is what a kept listing holds for one condition, or nil —
// which is also the answer for an EXIT trap the dialect keeps the signals
// without.
func (r *Runner) keptTrapAction(entries []savedTrap, cond string) (*string, bool) {
	for _, e := range entries {
		if e.cond != cond {
			continue
		}
		if e.cond == "EXIT" &&
			!r.ask(r.sem().KeptTrapListingIncludesExit, "a kept trap listing including the EXIT trap") {
			return nil, r.unspecified
		}
		action := e.action
		return &action, false
	}
	return nil, false
}

// trapsModified marks the trap state as this runner's own: the listing a
// subshell inherited is dropped the moment any trap is set, reset or
// ignored — measured in bash and ksh93, where `(trap ” USR2; trap)` lists
// USR2 and nothing the parent had, while an inherited ignore, being a live
// trap rather than a leftover picture, stays listed through the change.
func (r *Runner) trapsModified() {
	r.trapSnapshot = nil
	r.trapContexts = nil
}

// clearPseudoInherited records that a pseudo-condition was set or reset on
// this side of the subshell boundary, so it is this runner's own from here
// on: it fires and lists like any local trap.
func (r *Runner) clearPseudoInherited(name string) {
	switch name {
	case "ERR":
		r.errTrapInherited = false
	case "DEBUG":
		r.debugTrapInherited = false
	case "RETURN":
		r.returnTrapInherited = false
	}
}

// inheritedPseudoListed answers whether an inherited pseudo-trap still
// belongs in this subshell's listing, which follows whether it still
// *fires* here: zsh lists the ZERR and DEBUG traps a subshell inherits
// because they still run there, and bash lists an inherited DEBUG only
// while the kept snapshot is showing it — once a modification drops the
// snapshot, `(trap ” USR2; trap)` lists USR2 alone. RETURN fires in no
// subshell, so it is never listed as inherited.
func (r *Runner) inheritedPseudoListed(name string) (listed, refused bool) {
	switch name {
	case "ERR":
		listed = r.ask(r.sem().ErrTrapRunsInSubshells, "the ERR trap inside a subshell")
	case "DEBUG":
		listed = r.ask(r.sem().DebugTrapRunsInSubshells, "the DEBUG trap inside a subshell")
	default:
		return false, false
	}
	return listed, r.unspecified
}

// pseudoTrapInherited says the named pseudo-trap crossed the subshell
// boundary rather than being set inside it.
func (r *Runner) pseudoTrapInherited(name string) bool {
	switch name {
	case "ERR":
		return r.errTrapInherited
	case "DEBUG":
		return r.debugTrapInherited
	case "RETURN":
		return r.returnTrapInherited
	}
	return false
}

// endSubshell ends a cloned runner's shell at a boundary that is not a whole
// Run call.
//
// `$( … )` and the process substitutions run their bodies through Run, which
// ends at Finish and so already reaches the EXIT trap. `( … )`, a pipeline
// element, a background job and a coprocess each run a body directly, so the
// end of the subshell is a return in Go with nothing standing for the process
// exit a real shell has. This is that moment — the boundary reconstructed by
// hand, which is what AGENTS.md says a goroutine-for-a-fork costs.
//
// One function rather than the same two lines in four files, for the reason
// substRunner is one helper: a second copy of a boundary's ending is where the
// next fix goes missing.
//
// Called *inside* whatever the boundary set up around the body — the
// subshell's redirections, the element's end of the pipe — because that is
// where the handler's output goes. Measured 2026-09-12 across dash, bash 5.3,
// bash 3.2, ksh93 and zsh: `( trap 'echo T >&2' EXIT; true ) 2>file` puts the
// line in the file everywhere, and `( trap 'echo T' EXIT; exit 5 ) | cat`
// sends it down the pipe.
// The dialect's exit hook fires here too, and by a **narrower** rule than the
// trap's: only when the subshell left through `exit`. Measured 2026-09-12
// against zsh 5.9.2, the one shell in the panel that has a hook at all —
// `( exit 2 )` and `{ exit 2; } | cat` each ran `zshexit` inside the subshell
// and again at the end of the shell, `( true )` and `( false )` ran it only at
// the end, and `x=$(exit 3)` ran it only at the end as well. The status is not
// what decides it: `( exit 0 )` fires. `$?` in the hook is the status the
// subshell is leaving with, which the last of those makes visible — with the
// hook printing `$?`, `( exit 2 )` writes `HOOK=2` from in there and `HOOK=0`
// at the end.
//
// A trap that exits counts as the subshell exiting:
// `( trap 'exit 5' EXIT; true )` fires it and `( trap 'echo T' EXIT; true )`
// does not, which is the pair runExitTrap's return value exists for. One shape
// measured and deliberately not modeled: `( trap 'return 5' EXIT; true )`
// fires the hook and reports 5, so a bare `return` from an EXIT trap ends that
// subshell the way `exit` does — a fact about `return` in a trap body rather
// than about the hook, and nothing here reaches it.
func (r *Runner) endSubshell(ctx context.Context) {
	if r.canceledChunk {
		// The caller stopped this shell rather than the script ending, and
		// the EXIT trap is more of the script. Run excludes it on the same
		// grounds and in the same words.
		return
	}
	// Read before the trap runs, because runExitTrap forces the flag to
	// controlExit on its way out and there would be nothing left to read.
	exited := r.ctl == controlExit
	if r.runExitTrap(ctx) {
		exited = true
	}
	if exited {
		r.fireExitHook(ctx)
	}
}
