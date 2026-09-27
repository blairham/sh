// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A **deferred parameter** is one a dialect has registered and that the shell
// brings into being on the script's first reference to it.
//
// The third state a name can be in, beside "this shell has it" and "this
// shell has not got it" — see absentparam.go for the second, whose roster
// this sits beside. The name is registered: a read of it answers, a write to
// it reaches the producer, and `${+name}` is 1. What it is *not*, until
// something refers to it, is a parameter a listing walks or an `unset` may
// refuse to remove.
//
// It is not an axis. One dialect in this tree loads parameters with modules
// and the rest have none to load, so this is a registration a dialect makes
// name by name — exactly as [Runner.SetAbsentParameter] is — rather than a
// question every preset has to answer.
//
// # What a reference is, measured rather than reasoned
//
// A **script's** read or assignment brings it in; the **shell's own** writes
// to the same table do not. Measured 2026-09-27 on zsh 5.9.2 from a script
// file under `env -i PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME`,
// each cell in a shell of its own so that no row primes the next, reading the
// state through `typeset -p name` — which is itself no reference, proved by
// asking twice in one shell and getting nothing both times:
//
//	first line              then `typeset -p aliases`
//	(nothing)               nothing
//	alias zz=1              nothing     ← the shell filling the table
//	f(){ :; } (functions)   nothing
//	setopt noclobber (opts) nothing
//	cd /usr (dirstack)      nothing
//	${#aliases}             typeset -A aliases
//	${(t)aliases}           typeset -A aliases
//	${+aliases}             typeset -A aliases
//	${functions[f]}         typeset -A functions   ← an element read counts
//	aliases=(zz 1)          typeset -A aliases
//
// So the set test counts, an expansion flag counts, a subscripted read counts
// and a whole-table assignment counts — which is why the two sites that bring
// a parameter in are the two word paths, where refuseAbsentParameter is
// already asked for this same population, and the one place a script's
// assignment is performed. Nothing else in this engine reaches a parameter
// on a script's behalf.
//
// A second, independent route says the same thing without touching the
// parameter at all: `zmodload -e zsh/parameter` is 1 in a fresh shell and 0
// after `: ${#funcstack}` alone. Two routes that do not share an apparatus
// agreeing is what makes the reading the mechanism's rather than the probe's.
//
// # Why it is worth modeling, and it is two bugs rather than one
//
// The state is invisible to a script that reads the name before asking
// anything about it, which is nearly every script that has a reason to — and
// it is a **fatal** difference in both directions for the ones that do not:
//
//	unset funcstack      as the first line of a script: 0 in the reference,
//	                     and `read-only variable: funcstack` at 1 here, which
//	                     ends the script (#4895)
//	${+modules}          and then `unset modules`: `read-only variable:
//	                     modules` at 1 in the reference, and a silent 0 here
//
// The second is the same probe artifact seen from the other side, and it is
// why absentparam.go's recorded reason for exempting an absent name from the
// freeze is no longer what this exemption rests on: that measurement was
// taken in a shell where nothing had referred to the name, so what it
// recorded was this state rather than a rule about `unset`. With the
// deferral modeled, the exemption belongs to the deferral and an absent
// parameter that has been referred to is frozen like any other.

// SetDeferredParameter records that a registered name is one the shell brings
// into being on the script's first reference to it.
//
// For a name the dialect has already registered by some other route — a
// producer, a refusal, or a value the shell maintains. This says nothing
// about what the name holds; it says only that nothing has asked yet.
func (r *Runner) SetDeferredParameter(name string) {
	if r.deferredParams == nil {
		r.deferredParams = map[string]bool{}
	}
	r.deferredParams[name] = true
}

// DeferredParameter reports whether a name is registered and still waiting
// for its first reference.
//
// A name an `unset` has taken away is **not** one: the removal is the end of
// the parameter rather than a return to the state before it, which is
// measured — after `unset funcstack` in a shell that never read it,
// `${+funcstack}` is 0 there and a function that reads `$funcstack` sees
// nothing, where a deferred name would have arrived on that read.
func (r *Runner) DeferredParameter(name string) bool {
	return r.deferredParams[name] && !r.removed[name]
}

// ReferToParameter brings a deferred parameter in from outside the expansion
// machinery, for a dialect that has a second route to the same arrival.
//
// One caller and it is measured: an explicit module load. `zmodload
// zsh/parameter` in a shell that has read none of the module's names makes
// `unset funcstack` on the next line `read-only variable: funcstack` at 1 in
// zsh 5.9.2, where without it the `unset` is a silent 0 — so the load is a
// reference, though nothing in the script named the parameter.
//
// A method rather than the unexported one below because the fact is the
// dialect's: the core knows what an arrival *does* and has no way of knowing
// that loading a module is one.
func (r *Runner) ReferToParameter(name string) { r.referredToParameter(name) }

// referredToParameter is the arrival: the script has named this parameter, so
// whatever was waiting on a first reference is waiting no longer.
//
// Called from the two word paths and from the one place an assignment is
// performed, and deliberately from nowhere else — a listing, an `unset` and
// every write the *shell* makes to one of these tables must leave the state
// alone, which is what the grid at the top of this file measures. Putting it
// anywhere a value is fetched would mean the listing brought the name in by
// asking about it, and then the mechanism would be the thing that defeated
// every probe of it.
func (r *Runner) referredToParameter(name string) {
	if name == "" || !r.deferredParams[name] {
		return
	}
	delete(r.deferredParams, name)
}
