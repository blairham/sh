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
	if arrive := r.onParameterArrival[name]; arrive != nil {
		arrive(r)
	}
}

// SetParameterArrival says what else a dialect wants done the first time a
// deferred name is referred to.
//
// One user and it is what the seam was added for: in the shell being modeled,
// a reference to either half of `$WATCH`/`$watch` **loads `zsh/watch`**, and
// the module brings two more parameters with it. So the arrival of one name is
// the arrival of a module, which is a fact about that dialect and not
// something the core could derive.
//
// Behind the removal from the roster above rather than in front of it, so that
// an arrival which refers to a name — the module's own registration does — is
// not the same arrival over again.
func (r *Runner) SetParameterArrival(name string, arrive func(*Runner)) {
	if r.onParameterArrival == nil {
		r.onParameterArrival = map[string]func(*Runner){}
	}
	r.onParameterArrival[name] = arrive
}

// deferredParameterListingWord is what a bare declaration listing writes in
// place of the attribute words for a parameter nothing has referred to.
//
// A word of the same vocabulary attributeWordHead spells `association`,
// `array` and `readonly` in, and it stands **alone**: measured 2026-09-27 on
// zsh 5.9.2 under `-f` from a script file, a bare `typeset` in a shell that
// has read nothing writes `undefined funcstack` — one word and the name, with
// no kind, no freeze and no value — and `: ${#funcstack}` on the line before
// turns the same row into `array readonly funcstack`.
//
// Not in attributeWordHead, because it is not an attribute: no declaration
// can carry it, no letter sets it, and it is the *absence* of the parameter
// rather than a property of one. That is also why it does not belong to the
// guard that makes both attribute vocabularies spell every attribute — the
// other vocabulary belongs to a dialect with no deferred parameters to spell.
const deferredParameterListingWord = "undefined"

// deferredParameterRow is that word in front of the attribute words a
// *declaration* has put on the name, and never a value.
//
// `undefined` sits where the **kind** word sits and says there is no kind
// yet, which is the reading the rest of the row is evidence for. A
// declaration can reach one of these names without bringing it into being,
// and what it leaves is written behind the word in the ordinary order.
// Measured 2026-09-27 on zsh 5.9.2 under `-f`, one shell per line, each read
// back out of a bare `typeset`:
//
//	(nothing)              undefined funcstack
//	readonly funcstack     undefined readonly funcstack
//	typeset -U funcstack   undefined unique funcstack
//	typeset -u funcstack   undefined uppercase funcstack
//	typeset -H funcstack   undefined funcstack
//	: ${#funcstack}        array readonly funcstack
//
// So the word is not "this name has no attributes" — three of those six lines
// would falsify that, and the first draft of this made exactly that claim.
// It is the kind slot, and the registration's *own* readonly goes with it:
// the parameter this shell registered is not there yet, so neither is the
// freeze it was registered with. Only a freeze the **script** asked for is
// written, which is what readonlyByDeclaration records.
//
// `export funcstack` and `typeset -x funcstack` are the pair that does not
// reach here at all: both **materialize** the parameter in the reference —
// `array readonly exported funcstack`, `${(t)}` carrying `export` — so the
// row they produce is the ordinary one and not this.
func (r *Runner) deferredParameterRow(d declaration, isLocal bool) string {
	// No kind: that is the whole of what the word says.
	d.isArr, d.isAssoc, d.integer, d.float = false, false, false, false
	// And no freeze unless the script asked for one. `d.readonly` on these
	// names is the dialect's registration — every module table is marked
	// readonly the moment it is installed — and the reference writes no such
	// word until the parameter exists.
	d.readonly = r.readonlyByDeclaration[d.name]
	return deferredParameterListingWord + " " + r.attributeWordHead(d, isLocal) + d.name
}

// deferredNameListsAsAnOperand reports whether a valueless declaration naming
// a deferred parameter writes the bare name back.
//
// The **second** shape the state has in a listing, beside the `undefined` row
// above, and it is asked of a name the line named rather than of the walk:
// measured 2026-09-27 on zsh 5.9.2, in a shell that has referred to nothing,
//
//	typeset funcstack       funcstack     at 0
//	typeset + funcstack     funcstack     at 0
//	typeset - funcstack     funcstack     at 0
//	local funcstack         funcstack     at 0   at the top level
//	local funcstack         nothing       at 0   inside a function
//	typeset -r funcstack    nothing       at 0
//	typeset -g funcstack    nothing       at 0
//	readonly funcstack      nothing       at 0
//	export funcstack        nothing       at 0
//	typeset -p funcstack    nothing       at 0
//	typeset +               no row for it
//
// so it is two of the three conditions
// Semantics.ValuelessDeclarationOfAHeldNameListsIt is asked under and not the
// third. No attribute letter anywhere on the line, a bare sign not being a
// letter; and the line is **not making the binding it writes**, which is the
// pair of rows `local` gives — at the top level it writes the name and inside
// a function, where the declaration really does take a shadow, it writes
// nothing, in the reference and here.
//
// The condition this name cannot meet is the third one: it *holds* nothing. A
// deferred name has no cell in any table until something refers to it, so
// declaredNameHolds is false for every one of them and the whole form went
// silent (#4924).
//
// **The listing is not a reference and the declaration is not one either.**
// The row is written and the name is left exactly as it was: after `typeset
// funcstack`, a bare `typeset` still writes `undefined funcstack` and
// `typeset -p funcstack` is still nothing, in the reference and here. So this
// returns having written a name and the operand goes no further — a
// `declareEmpty` behind it would have put a stored cell under the name and
// made the *next* listing of it an ordinary row, which is what this shell did
// and is how the divergence read as order-dependent.
func (r *Runner) deferredNameListsAsAnOperand(name string, noLetters, redeclared bool) bool {
	return noLetters && redeclared && r.DeferredParameter(name)
}
