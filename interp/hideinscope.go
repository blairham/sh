// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The hide-in-scope attribute: `typeset -h` and `typeset +h`.
//
// A shell that ties `PATH` to `path` — see tiedscalar.go — has to answer what
// a *local* declaration of one of those names means. Two readings are
// available and both are wanted by real scripts: the local is still the
// special parameter, so writing it moves the search path for the duration of
// the call, or the local is an ordinary parameter that merely happens to be
// spelled `PATH` and the tie goes on operating on the outer cell. `-h` is the
// letter that picks the second reading and `+h` picks the first back.
//
// Measured 2026-09-09 against zsh 5.9.2 with `-f` and a two-entry `PATH`,
// which is the shell in the panel with the letter. Six rows, and the last two
// are the ones that make `+h` more than a letter taken and dropped:
//
//	f(){ local    PATH=/x; print ${path[*]} }   → /x            (tied)
//	f(){ local +h PATH=/x; print ${path[*]} }   → /x            (tied)
//	f(){ local -h PATH=/x; print ${path[*]} }   → /bin /usr/bin (detached)
//	typeset -h PATH; PATH=/x; print ${path[*]}  → /x            (tied)
//	typeset -h PATH; f(){ local    PATH=/x; … } → /bin /usr/bin (detached)
//	typeset -h PATH; f(){ local +h PATH=/x; … } → /x            (tied)
//
// Three facts are in those rows.
//
//   - **It is an attribute of a binding, and what it governs is the binding
//     in front of it.** Row four sets it at the top level and rows five and
//     six are a *later* function reading it: a plain `local` with no `h` of
//     its own is detached because the binding it displaced carried the
//     letter. So it lives in a map on the runner beside the other attributes
//     of a name, and not in declareFlags alone.
//
//     The shadow's own binding is a fresh one and carries nothing, which is
//     measured rather than reasoned from and is where this file used to say
//     "an attribute of the name". zsh 5.9.2, 2026-09-13:
//
//	typeset -h v=1;  ${(t)v}                     scalar-hide
//	f(){ local    v=2; print ${(t)v} }; f        scalar-local
//	f(){ local -h v=2; print ${(t)v} }; f        scalar-local-hide
//
//     Row two is the one that says so: the behavior is inherited and the
//     *word* is not, so a shadow that kept the attribute described itself
//     with a letter the shell never writes there. What the scope records at
//     the shadow is therefore the answer and not the attribute — see
//     shadowIsHidden, which every consumer of the letter goes through.
//
//   - **It detaches only where a local stands.** Row four is the control that
//     says so: the attribute is set and the global `PATH` goes on driving
//     `path` regardless. What the letter suppresses is the *specialness of a
//     shadow*, which is why tieDetached asks both questions and not one.
//
//   - **`+h` is not the absence of `-h`.** Rows two and six are the same
//     command and only the second changes an answer, because only there was
//     there an inherited attribute to take off. A `+h` that merely parsed and
//     did nothing would pass row two and fail row six.
//
// **`unset` does not forget it.** This file used to say the opposite —
// "`typeset -h PATH; unset PATH; PATH=/y` leaves a later `local PATH` tied
// again" — and the reading could not have come from the shell: the letter is
// visible only over one of the shell's *own* ties (see tielocal.go), and this
// engine's `unset` of such a name forgot the tie as well, so the name that
// came back was untied whether or not it had kept the letter and both answers
// looked identical. Fixing the tie (#1631) made the row measurable, and zsh
// 5.9.2 keeps the letter: after that very line a `local PATH=/z` still leaves
// `path` holding `/y`, and only `typeset +h PATH` puts the tie back in a
// local's reach. clearAttributes says so, with the four rows.
//
// **It detaches a produced parameter as well as a tie**, and that is the
// second thing it means rather than a second letter. A name the shell
// *produces* — `ARGC`, or any of the thirty parameters a zsh module
// registers, all of which carry `hide` from the module — reads back the
// shell's own value through a plain shadow, and a hidden shadow is an
// ordinary parameter holding whatever the declaration wrote. Measured
// 2026-09-13, zsh 5.9.2, `env -i PATH=/usr/bin:/bin` with a scratch `HOME`,
// `ZDOTDIR` and `HISTFILE`, over a script file with the modules loaded:
//
//	f(){ local    ARGC;   print $ARGC }          0    the produced value
//	f(){ local    ARGC=5; … }                    f: read-only variable: ARGC
//	f(){ local -h ARGC=5; print $ARGC }          5
//	f(){ local -h ARGC=5; print ${(t)ARGC} }     scalar-local-hide
//	f(){ local parameters; print "$parameters" } (an empty line)
//	f(){ local parameters; print ${(t)parameters} }  scalar-local
//	f(){ local EPOCHSECONDS=5; print $EPOCHSECONDS } 5
//	f(){ local +h EPOCHSECONDS=5; … }            f: read-only variable: …
//
// Rows four and six are what says the *producer* is gone and not merely the
// freeze: the kind and the `special` word are both read off the produced
// tables, so a shadow that lifted the freeze alone would still describe as
// `integer-…-special` and still answer the shell's value. Row eight is the
// control on the other side, since `EPOCHSECONDS` carries `hide` from its
// module and `+h` is the only thing asking for the second view back.
//
// This half was described and not wired for as long as the word was right
// (#2552, #2586): `hideInScope` reached a tie and nothing else, so
// `local parameters` read the whole table and `local EPOCHSECONDS=5` read the
// clock. suspendProducer is the wiring, and it is per-scope for the reason
// the freeze is — the name is the shell's own again the moment the call
// returns.
//
// No listing shows it. `typeset -h v=1; typeset -p v` is `typeset v=1` there,
// and `typeset -p PATH` after `typeset -h PATH` still writes the tie — so
// declareprint asks nothing about this and the letter is invisible except
// through the behavior above.
//
// One shell in the panel spells the letter with this meaning, so it is a
// letter rather than an axis, the way `-H`, `-U` and `-T` are: bash 5.3 and
// bash 3.2 answer `+h: invalid option` under `local`, `typeset` and
// `declare` alike, dash has no `typeset` and reads `local +h` as a bad
// variable name, and ksh93's `-h` is a wholly different attribute that does
// not detach anything. The dialect that has it says so in
// Semantics.DeclareOptions, LocalOptions and IntegerOptions; every other
// dialect goes on refusing the letter by name.

// setHideInScope records or forgets the attribute for a name. It is a
// separate step from applyAttributes because it has to run *after* the
// declaration takes its shadow: the scope saves the outer state at that
// moment, and applying the letter first would leave the scope saving the
// attribute this very declaration had just added and putting it back forever.
//
// And it runs *before* the value, which is what makes the `+h` row below a
// refusal rather than an assignment — see the caller in declarebuiltin.go.
func (r *Runner) setHideInScope(name string, f declareFlags) {
	if !f.hideNamed {
		// Nothing written, so the name keeps whatever it carries — which is
		// what makes the inheritance in rows five and six of the table above
		// happen by itself rather than by copying anything.
		return
	}
	if !f.hide {
		delete(r.hideInScope, name)
		r.shadowStopsHiding(name)
		return
	}
	if r.hideInScope == nil {
		r.hideInScope = map[string]bool{}
	}
	r.hideInScope[name] = true
	r.shadowStartsHiding(name)
}

// shadowStartsHiding makes the shadow standing over a name a hidden one:
// an ordinary parameter that merely happens to be spelled like one of the
// shell's own.
//
// Two things come off, and they are the two halves of what the letter means.
//
//   - **The freeze**, where the shadow kept a produced parameter's. Measured:
//     `f(){ local -h ARGC=5; print $ARGC }` is `5` in zsh 5.9.2, against
//     `read-only variable: ARGC` for the same line without the letter.
//
//   - **The producer**, which is the half this used to leave standing. With
//     it in place the cell the declaration wrote was never read: the value
//     came from the shell, so `local EPOCHSECONDS=5` read the clock and
//     `local parameters` read the whole table where zsh reads `5` and an
//     empty line (#2586). Suspending it for the scope is what makes the
//     shadow an ordinary parameter rather than a second view — and it is why
//     `${(t)}` inside the shadow is `scalar-local` there and not
//     `association-local-hide-special`, since the kind and the `special`
//     word are both read off the produced tables.
//
// Only where a shadow was taken, which is the control on the other side:
// `typeset -h ARGC; ARGC=5` at the top level is `read-only variable: ARGC` in
// the same shell and `${(t)ARGC}` is still `integer-readonly-hide-special`,
// so the letter alone thaws nothing and suspends nothing. The scope has
// already recorded what to put back — see shadow, which writes savedReadonly
// before this runs.
func (r *Runner) shadowStartsHiding(name string) {
	if !r.localInTheInnermostScope(name) {
		return
	}
	sc := r.scopes[len(r.scopes)-1]
	if sc.hiddenShadow == nil {
		sc.hiddenShadow = map[string]bool{}
	}
	sc.hiddenShadow[name] = true
	delete(r.readonly, name)
	r.suspendProducer(sc, name)
}

// shadowStopsHiding is `+h` reaching a shadow that was already a hidden one,
// and it is not the absence of the line above: it puts the producer back and
// the freeze with it.
//
// Measured 2026-09-12, zsh 5.9.2, `env -i PATH=/usr/bin:/bin` with a scratch
// `HOME`, `ZDOTDIR` and `HISTFILE`, over a script file with `zsh/datetime`
// loaded — `EPOCHSECONDS` carries `hide` from its module, so a plain shadow
// of it is ordinary and `+h` is what asks for the second view back:
//
//	f(){ local    EPOCHSECONDS=5; print $EPOCHSECONDS }   5
//	f(){ local +h EPOCHSECONDS=5; print $EPOCHSECONDS }   f: read-only
//	                                                      variable: …
//	f(){ local +h parameters; print "$parameters" }       the whole table
//
// The second row is the one that says the freeze comes back: without it the
// declaration would take its value and print it, which is what this engine
// did. The third says the producer does — a `+h` that only re-froze would
// leave the name empty.
func (r *Runner) shadowStopsHiding(name string) {
	if !r.localInTheInnermostScope(name) {
		return
	}
	sc := r.scopes[len(r.scopes)-1]
	if sc.hiddenShadow == nil {
		sc.hiddenShadow = map[string]bool{}
	}
	sc.hiddenShadow[name] = false
	r.resumeProducer(sc, name)
	// And the freeze the shadow displaced, on the terms freezeSurvivesAShadow
	// states: a produced parameter's survives its shadow and a script's does
	// not, so only the first comes back here.
	if sc.savedReadonly[name] && r.DynamicParameter(name) {
		if r.readonly == nil {
			r.readonly = map[string]bool{}
		}
		r.readonly[name] = true
	}
}

// shadowIsHidden reports whether the declaration standing over a name took a
// hidden shadow, which is the question every consumer of the letter actually
// asks.
//
// Not `r.hideInScope[name]`, and the difference is the whole of why this
// exists: the attribute describes a *binding*, and the shadow's binding is a
// fresh one that carries the letter only if the declaration wrote it.
// Measured on zsh 5.9.2 — `typeset -h v=1` is `scalar-hide` and a plain
// `local v=2` inside a function is `scalar-local`, with no `hide` in the
// word, while `local -h v=2` is `scalar-local-hide`. So what governs the
// shadow is the attribute the *outer* binding carried, which the scope
// records at the shadow and this reads back.
//
// The innermost scope that shadowed the name answers, which is what lets a
// `+h` deeper in take the hiding off a name an outer scope hid; a name no
// scope has shadowed falls back to the binding's own attribute, so nothing
// changes at the top level.
func (r *Runner) shadowIsHidden(name string) bool {
	for i := len(r.scopes) - 1; i >= 0; i-- {
		if hidden, ok := r.scopes[i].hiddenShadow[name]; ok {
			return hidden
		}
	}
	return r.hideInScope[name]
}

// hidesItsTie reports whether either half of a tie stands under a hidden
// shadow.
//
// Either half answers for both, and that is a choice worth naming. The shell
// with the letter keeps the *other* half tied to the outer cell — inside
// `local -h PATH=/x`, writing `path=(/q)` leaves the local `PATH` at `/x` and
// moves the caller's. This engine has one variable table and a stack of saved
// values, so "the outer cell" is not a thing an assignment can name; what it
// can do is decline to mirror at all, which agrees with that measurement on
// what the function sees and differs on what the caller is left holding. The
// narrower rule — suspending only the hidden half — would have disagreed on
// both, since `path=(/q)` would then have written the local `PATH` the
// declaration had just gone out of its way to detach.
//
// The *shadow* is the other half of the question and is asked in tielocal.go,
// where the scope a tie belongs to is worked out: the attribute alone
// detaches nothing, which is the second fact in the comment above.
func (r *Runner) hidesItsTie(t tie) bool {
	return r.shadowIsHidden(t.scalar) || r.shadowIsHidden(t.array)
}

// suspendedProducer is everything a hidden shadow took out of the produced
// tables, kept whole so that the scope's exit needs nothing to have been
// remembered elsewhere.
//
// The same shape [withdrawnParameter] keeps and for the same reason, and
// deliberately not the same type: a withdrawal is a *module selection*, it is
// per-runner and takes the readonly and hidden marks with it, where this is
// per-scope and leaves both to the scope's own records. Sharing one struct
// would have tied a scope's exit to what a `zmodload -F` had done.
type suspendedProducer struct {
	scalar      func(*Runner) string
	array       func(*Runner) []string
	assoc       func(*Runner) AssocArray
	element     func(*Runner, string) (string, bool)
	writeScalar func(*Runner, string)
	writeArray  func(*Runner, []string)
	writeAssoc  func(*Runner, string, string, bool)

	declaration ProducedDeclaration
	declared    bool
}

// suspendProducer takes a name out of the produced tables for as long as one
// scope's hidden shadow stands over it.
//
// Nothing happens for a name the shell does not produce, which is most of
// them: `typeset -h v=1` is an ordinary scalar with a letter on it, and the
// letter is visible there only through a tie.
func (r *Runner) suspendProducer(sc *scope, name string) {
	if _, already := sc.suspendedProducers[name]; already {
		return
	}
	if !r.DynamicParameter(name) {
		return
	}
	p := suspendedProducer{
		scalar:      r.Dynamic[name],
		array:       r.DynamicArrays[name],
		assoc:       r.DynamicAssocs[name],
		element:     r.dynamicAssocElements[name],
		writeScalar: r.dynamicWriters[name],
		writeArray:  r.dynamicArrayWriters[name],
		writeAssoc:  r.dynamicAssocWriters[name],
	}
	p.declaration, p.declared = r.dynamicDeclarations[name]
	delete(r.Dynamic, name)
	delete(r.DynamicArrays, name)
	delete(r.DynamicAssocs, name)
	delete(r.dynamicAssocElements, name)
	delete(r.dynamicWriters, name)
	delete(r.dynamicArrayWriters, name)
	delete(r.dynamicAssocWriters, name)
	delete(r.dynamicDeclarations, name)
	if sc.suspendedProducers == nil {
		sc.suspendedProducers = map[string]suspendedProducer{}
	}
	sc.suspendedProducers[name] = p
}

// resumeProducer puts a suspended name back into every table it came out of.
//
// The nil checks are the caller's half of the trap [putBack] records: a
// function read out of a map that has no such key is a *typed* nil, and
// registering one is a producer nothing can call.
func (r *Runner) resumeProducer(sc *scope, name string) {
	p, ok := sc.suspendedProducers[name]
	if !ok {
		return
	}
	if p.scalar != nil {
		putBack(&r.Dynamic, name, p.scalar)
	}
	if p.array != nil {
		putBack(&r.DynamicArrays, name, p.array)
	}
	if p.assoc != nil {
		putBack(&r.DynamicAssocs, name, p.assoc)
	}
	if p.element != nil {
		putBack(&r.dynamicAssocElements, name, p.element)
	}
	if p.writeScalar != nil {
		putBack(&r.dynamicWriters, name, p.writeScalar)
	}
	if p.writeArray != nil {
		putBack(&r.dynamicArrayWriters, name, p.writeArray)
	}
	if p.writeAssoc != nil {
		putBack(&r.dynamicAssocWriters, name, p.writeAssoc)
	}
	if p.declared {
		putBack(&r.dynamicDeclarations, name, p.declaration)
	}
	delete(sc.suspendedProducers, name)
}

// MarkHideInScope gives a name the hide-in-scope attribute from outside the
// package, which is how a dialect says that one of its *module* parameters is
// spelled the way the shell it models spells them.
//
// Distinct from [Runner.MarkHidden], and the pair is measured rather than
// assumed — see [ParameterAttributes.Hidden]. A module parameter in the shell
// this models carries both: `${(t)mapfile}` is
// `association-hide-hideval-special`, where `hideval` keeps the value out of a
// listing and `hide` is what makes `local mapfile` inside a function an
// ordinary parameter rather than a second view of the filesystem. A name given
// only one of them describes with only that word, so a dialect that set one
// and meant both would be telling a script switching on `${(t)…}` about the
// wrong letter (#2042).
//
// A dialect calls this while registering, which is the top level and has no
// scope — but it goes through the same seam the letter does rather than
// setting the map and stopping, because a second helper that does nearly the
// same thing is where the next fix lands in only one of them.
func (r *Runner) MarkHideInScope(name string) {
	if r.hideInScope == nil {
		r.hideInScope = map[string]bool{}
	}
	r.hideInScope[name] = true
	r.shadowStartsHiding(name)
}

// freezeSurvivesAShadow reports whether a declaration standing in front of a
// frozen name leaves it frozen.
//
// Ordinarily it does not: a local is a fresh binding, the freeze is displaced
// with the value, and `readonly z=1; f(){ local z=5; print $z }` is `5` in
// the shell that allows the shadow at all. That is the answer for a name a
// *script* froze, and it is the wrong answer for one the shell produces.
//
// Measured 2026-09-12, zsh 5.9.2, `env -i PATH=/usr/bin:/bin` with a scratch
// `HOME`, `ZDOTDIR` and `HISTFILE`, over a script file, with `ARGC` — a
// produced parameter this engine has marked readonly since the name was
// added:
//
//	f(){ local ARGC=5; print $ARGC }     f: read-only variable: ARGC, fatal
//	f(){ typeset ARGC=5; … }             the same, and `declare` too
//	f(){ local ARGC=""; … }              the same: an empty value is a value
//	f(){ local ARGC; print $ARGC }       0, and the outer value is intact
//	f(){ local ARGC; ARGC=5 }            0, then read-only variable: ARGC
//	f(){ local -i ARGC; print $ARGC }    0 — a letter is not a value either
//	readonly z=1; f(){ local z=5; … }    5 — the control, and it goes the
//	                                     other way
//
// Row five is the one that decides the shape. A refusal aimed at the
// *declaration* would pass rows one to four and fail it: there the
// declaration is taken, the shadow happens, and the assignment on the next
// line is what the shell refuses. So the freeze is not displaced — the local
// cell is still the special parameter — and every refusal above is the
// ordinary one an assignment to a frozen name already makes.
//
// Restricted to a *produced* parameter, which is what the control row asks
// for: an ordinary readonly is thawed by the shadow in the same shell in the
// same run. Not an axis, because no other column in the panel has such a name
// to ask about — a produced parameter is marked readonly only by the dialect
// modeling the shell above, and bash's `SECONDS` and `EPOCHSECONDS` carry no
// mark by measurement (see dialect/bash/epoch.go).
//
// The hide-in-scope letter is the exemption, and it is measured rather than
// reasoned from: `f(){ local -h ARGC=5; print $ARGC }` is `5` there, and so
// is a local of a module parameter, which carries the letter from
// registration — `f(){ local parameters=1; print $parameters }` is `1`. That
// is the whole of what `-h` means, applied to the freeze instead of to a tie:
// the shadow is an ordinary parameter that merely happens to be spelled like
// the shell's own. A letter written on *this* declaration is applied after
// the shadow, so the freeze is lifted there rather than here — see
// setHideInScope.
func (r *Runner) freezeSurvivesAShadow(name string) bool {
	return r.readonly[name] && r.DynamicParameter(name) && !r.hideInScope[name]
}
