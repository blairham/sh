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
//   - **It is an attribute of the name, not of the declaration.** Row four
//     sets it at the top level and rows five and six are a *later* function
//     reading it: a plain `local` with no `h` of its own inherits whatever
//     the name it shadows carries. So it lives in a map on the runner beside
//     the other attributes of a name, and not in declareFlags alone.
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
func (r *Runner) setHideInScope(name string, f declareFlags) {
	if !f.hideNamed {
		// Nothing written, so the name keeps whatever it carries — which is
		// what makes the inheritance in rows five and six of the table above
		// happen by itself rather than by copying anything.
		return
	}
	if !f.hide {
		delete(r.hideInScope, name)
		return
	}
	if r.hideInScope == nil {
		r.hideInScope = map[string]bool{}
	}
	r.hideInScope[name] = true
	// And where the shadow kept a produced parameter's freeze, the letter is
	// what lifts it — this declaration's shadow is an ordinary parameter now
	// and takes an ordinary value. Measured: `f(){ local -h ARGC=5; print
	// $ARGC }` is `5` in zsh 5.9.2, against `read-only variable: ARGC` for
	// the same line without the letter.
	//
	// Only where a shadow was taken, which is the control on the other side:
	// `typeset -h ARGC; ARGC=5` at the top level is `read-only variable:
	// ARGC` in the same shell, so the letter alone thaws nothing. The scope
	// has already recorded what to put back — see shadow, which writes
	// savedReadonly before this runs.
	//
	// The freeze is the half of the letter this reaches. The other half —
	// that the shadow is an ordinary parameter rather than a second view of
	// the producer — is not wired for a produced parameter here, so the
	// value read back inside that function is still the producer's `0` and
	// not the `5` the declaration wrote. Unchanged by this line, which only
	// turns a refusal into the answer the name already gave; it is the same
	// gap `local parameters` has, recorded in dialect/zsh's
	// hideModuleParameter (#2552).
	if r.localInTheInnermostScope(name) {
		delete(r.readonly, name)
	}
}

// hidesItsTie reports whether either half of a tie carries the attribute.
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
	return r.hideInScope[t.scalar] || r.hideInScope[t.array]
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
func (r *Runner) MarkHideInScope(name string) {
	if r.hideInScope == nil {
		r.hideInScope = map[string]bool{}
	}
	r.hideInScope[name] = true
	// And where the shadow kept a produced parameter's freeze, the letter is
	// what lifts it — this declaration's shadow is an ordinary parameter now
	// and takes an ordinary value. Measured: `f(){ local -h ARGC=5; print
	// $ARGC }` is `5` in zsh 5.9.2, against `read-only variable: ARGC` for
	// the same line without the letter.
	//
	// Only where a shadow was taken, which is the control on the other side:
	// `typeset -h ARGC; ARGC=5` at the top level is `read-only variable:
	// ARGC` in the same shell, so the letter alone thaws nothing. The scope
	// has already recorded what to put back — see shadow, which writes
	// savedReadonly before this runs.
	//
	// The freeze is the half of the letter this reaches. The other half —
	// that the shadow is an ordinary parameter rather than a second view of
	// the producer — is not wired for a produced parameter here, so the
	// value read back inside that function is still the producer's `0` and
	// not the `5` the declaration wrote. Unchanged by this line, which only
	// turns a refusal into the answer the name already gave; it is the same
	// gap `local parameters` has, recorded in dialect/zsh's
	// hideModuleParameter (#2552).
	if r.localInTheInnermostScope(name) {
		delete(r.readonly, name)
	}
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
