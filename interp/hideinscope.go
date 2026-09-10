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
//     shadow*, which is why hiddenLocal asks both questions and not one.
//
//   - **`+h` is not the absence of `-h`.** Rows two and six are the same
//     command and only the second changes an answer, because only there was
//     there an inherited attribute to take off. A `+h` that merely parsed and
//     did nothing would pass row two and fail row six.
//
// `unset` forgets it, measured: `typeset -h PATH; unset PATH; PATH=/y` leaves
// a later `local PATH` tied again. That is where the other attributes of a
// name are forgotten too, in unsetName.
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
}

// hiddenLocal reports whether a name is, right now, a hidden shadow: it
// carries the attribute *and* some scope on the stack has displaced the outer
// cell. Both halves are required — see the second fact in the file comment.
func (r *Runner) hiddenLocal(name string) bool {
	if !r.hideInScope[name] {
		return false
	}
	for _, sc := range r.scopes {
		if _, saved := sc.saved[name]; saved {
			return true
		}
	}
	return false
}

// tieDetached reports whether a tie is out of effect because one of its two
// names is a hidden shadow.
//
// Either half suspends it, and that is a choice worth naming. The shell with
// the letter keeps the *other* half tied to the outer cell — inside
// `local -h PATH=/x`, writing `path=(/q)` leaves the local `PATH` at `/x` and
// moves the caller's. This engine has one variable table and a stack of saved
// values, so "the outer cell" is not a thing an assignment can name; what it
// can do is decline to mirror at all, which agrees with that measurement on
// what the function sees and differs on what the caller is left holding. The
// narrower rule — suspending only the hidden half — would have disagreed on
// both, since `path=(/q)` would then have written the local `PATH` the
// declaration had just gone out of its way to detach.
func (r *Runner) tieDetached(t tie) bool {
	return r.hiddenLocal(t.scalar) || r.hiddenLocal(t.array)
}
