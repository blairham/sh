// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A name the *shell* holds in a slot of a fixed kind, so that a declaration
// asking to leave it some other kind is refused.
//
// The neighbor of interp/parameterscopefixed.go and deliberately not the
// same table. That one is `private` and the **scope** of a binding; this is a
// **kind** letter on any of the ordinary declaration words, it refuses at the
// top level as readily as inside a function, and the two name sets are
// different — `RANDOM`, `SECONDS`, `LINENO`, `UID`, `status` and `COLUMNS`
// all refuse `private` and all take `typeset -i`, so reading that table here
// would refuse six names the shell being modeled takes (#4834).
//
// ## The shape is a rule and the set behind it is a list
//
// Swept 2026-09-27 against `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)` — `go version -m` says *not a Go executable*
// for it and `github.com/blairham/sh/cmd/zsh` for ours, so these are two
// programs — one run per cell, `-f` from a script file under `env -i
// PATH=/usr/bin:/bin` with a scratch `HOME`, over 155 candidate names and the
// five kind letters under **both** signs: 1550 cells, of which the instrument
// produced 421 takes and 359 refusals on the minus half alone, so it could
// have answered either way for any row.
//
// The names fall into exactly four groups and nothing else:
//
//	-a take, -i -A -F -E refuse, +a refuse      an array slot
//	-i take, -a -A -F -E refuse, +i refuse      an integer slot
//	every kind letter refuses, every plus taken a scalar slot
//	-i -F -E take, -a -A refuse, every plus taken  SECONDS alone
//
// So the rule is: the slot allows a set of kinds, and a declaration is
// refused when what it would leave the name with is not in that set. A minus
// letter asks for its own kind; a plus letter takes that kind away, which is
// a refusal exactly when the set has nothing else in it. Both halves are
// measured and neither is derivable from the other — `+a path` refuses where
// `+A path` is taken, and `+i SECONDS` is taken where `+i RANDOM` refuses.
//
// **SECONDS is the counterexample that kills the obvious reading.** "The kind
// the name already has" would have `-F SECONDS` refused, since that name is
// an integer until something says otherwise; it is taken, and
// `${(t)SECONDS}` afterwards is `float-local-special`. A float `SECONDS` is a
// real thing in that shell, so the slot allows two kinds and the rule is
// about the slot rather than about the value standing in it.
//
// ## Where it does not apply
//
// **Under a hidden shadow**, which is the one exemption and is the same
// "only where a shadow stands" the freeze and the tie halves of the `-h`
// letter already record. `f(){ typeset -h -i path }` is 0 — the declaration
// makes an ordinary parameter that merely happens to be spelled like the
// shell's — and so is a second declaration behind one: `f(){ typeset -h
// path; typeset -i path }` is 0 too. At the **top level** the letter has no
// shadow to detach and `typeset -h -i path` is refused like any other.
//
// `unset` does not help, and that is the same answer interp/parameterscope-
// fixed.go records for its own table: `unset path; typeset -i path` still
// refuses, because the slot is there whether or not a value is.

// parameterKinds is the set of kinds one shell-held slot will take.
type parameterKinds uint8

func kindBit(k ParameterKind) parameterKinds { return 1 << uint(k) }

func (s parameterKinds) has(k ParameterKind) bool { return s&kindBit(k) != 0 }

// onlyHolds reports whether k is the single kind in the set, which is what
// makes a plus of that letter a refusal: taking it away leaves the slot with
// a kind it does not have.
func (s parameterKinds) onlyHolds(k ParameterKind) bool { return s == kindBit(k) }

// MarkParameterKindFixed records that the shell holds this name in a slot
// that will take only the kinds named, so a declaration leaving it any other
// kind is refused.
//
// One name per call and written out by the dialect that measured them, for
// [Runner.MarkParameterScopeFixed]'s reason: there is no hook this could ride
// on that would get the set right. `PWD` and `OLDPWD` are stored through the
// same seam as `PATH` and take every letter, and `TERM` has no value at all
// in the shell that refuses one.
//
// A call with no kinds is a slot that takes nothing but the plain word, which
// is the scalar group: it is spelled [ScalarParameter] rather than left empty
// so that the table reads as a measurement and not as an omission.
func (r *Runner) MarkParameterKindFixed(name string, kinds ...ParameterKind) {
	if r.kindFixed == nil {
		r.kindFixed = map[string]parameterKinds{}
	}
	var set parameterKinds
	for _, k := range kinds {
		set |= kindBit(k)
	}
	r.kindFixed[name] = set
}

// kindLetterWritten is the kind this declaration's letters ask for, the sign
// it was written with, and whether any of them was written at all.
//
// Last letter wins where two were written, which is [declareFlags.lastSign]'s
// rule and is the one the rest of this file already follows. The float
// spellings are one kind: `-F` and `-E` differ in how a value is *formatted*
// and not in what the name is, and the shell being modeled takes both of them
// over `SECONDS` and refuses both over `path`.
func (f declareFlags) kindLetterWritten() (kind ParameterKind, plus, written bool) {
	for i, c := range f.letters {
		var k ParameterKind
		switch c {
		case 'a':
			k = ArrayParameter
		case 'A':
			k = AssocParameter
		case 'i':
			k = IntegerParameter
		case 'F', 'E':
			k = FloatParameter
		default:
			continue
		}
		if i >= len(f.letterSigns) {
			continue
		}
		kind, plus, written = k, f.letterSigns[i] == '+', true
	}
	return kind, plus, written
}

// kindLetterOverAShellParameterRefused reports whether a kind letter aimed at
// one of the shell's own parameters is refused, having said so and ended the
// script.
//
// One gate for `typeset`, `local`, `declare`, `readonly`, `export` and the
// two numeric words, because the shell that refuses refuses all of them in
// the same sentence with the written word in the location — measured, `local
// -A path`, `declare -F path`, `export -i path`, `readonly -i path`, `integer
// path` and `float path` each say `path: can't change type of a special
// parameter` and end the shell at 1.
//
// Ahead of `private`'s own refusal where both could fire, which is measured
// rather than chosen: `f(){ private -A path }` is the *type* sentence and is
// fatal, where `f(){ private -i RANDOM }` — a letter that slot takes — is the
// scope sentence at 1 with the script carrying on.
func (r *Runner) kindLetterOverAShellParameterRefused(name string, f declareFlags) bool {
	allowed, fixed := r.kindFixed[name]
	if !fixed {
		return false
	}
	kind, plus, written := f.kindLetterWritten()
	if !written {
		return false
	}
	if f.hideNamed && f.hide {
		// The letter on *this* line asks for an ordinary parameter that
		// merely happens to be spelled like the shell's, so there is no slot
		// left to change the kind of. Only inside a function: at the top
		// level the letter detaches no shadow and the refusal stands.
		if len(r.scopes) > 0 {
			return false
		}
	}
	if r.localInTheInnermostScope(name) && r.shadowIsHidden(name) {
		// And a hidden shadow an *earlier* declaration in this call left
		// standing, which is the same exemption reached the second way.
		return false
	}
	if plus {
		if !allowed.onlyHolds(kind) {
			return false
		}
	} else if allowed.has(kind) {
		return false
	}
	r.fatal("%s\n", Wording(r.diag().SpecialParameterKind,
		"%s: can't change type of a special parameter", name))
	return true
}

// localSlotKind is the kind a fresh local of one of the shell's own
// parameters takes from the slot it shadows, where the declaration wrote no
// kind letter of its own: an array slot's local is an array, so a scalar
// value is the array of that one value, and an integer slot's local is an
// integer, so its value is evaluated. ScalarParameter means nothing is taken.
// Measured 2026-10-03 on zsh 5.9.2 under `env -i PATH=/usr/bin:/bin`, `-f`
// (#5598, #5576):
//
//	f(){ local path=/somewhere; print ${#path} }       1 — ours was 10
//	f(){ local fpath=/a/b; print ${(t)fpath} }         array-local-tied-special
//	f(){ typeset path=/q; print ${(t)path} }           array-local-tied-special
//	f(){ local SHLVL=4; print ${(t)SHLVL} }            integer-local-special
//	f(){ local COLUMNS=1+1; print $COLUMNS }           2
//	f(){ local SHLVL=abc; print $SHLVL }               0
//	f(){ local HISTSIZE; print ${(t)HISTSIZE} }        integer-local-special
//
// and the controls already agreed: `local path=(/a /b)`, `local path;
// path=/x`, `local PATH=/x`, `local RANDOM=3`, and `local -h path=/x`, which
// asks for an ordinary parameter. A kind letter on the line is not
// overridden — `local -a path=/x` is the *refusal* `inconsistent type for
// assignment` there, which is the existing gate's answer.
//
// Only a slot that holds one kind, so that the kind given is never a choice.
// `SECONDS` is the one slot of two kinds and its local is an integer either
// way — it is produced, and `f(){ local SECONDS=1.5 }` is `typeset -i10
// SECONDS=1` in the reference and here — so nothing yet separates the two
// readings, and the narrower one is kept.
func (r *Runner) localSlotKind(name string, f declareFlags) ParameterKind {
	allowed, fixed := r.kindFixed[name]
	if !fixed || (f.hideNamed && f.hide) {
		return ScalarParameter
	}
	if _, _, written := f.kindLetterWritten(); written || f.array || f.assoc || f.integer || f.float {
		return ScalarParameter
	}
	switch {
	case allowed.onlyHolds(ArrayParameter):
		return ArrayParameter
	case allowed.onlyHolds(IntegerParameter):
		return IntegerParameter
	}
	return ScalarParameter
}
