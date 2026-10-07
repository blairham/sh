// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A semantics vector the runner may write in place.
//
// The vector is swapped copy-on-write: an option change copies it, so that a
// saved pointer — a function's LOCAL_OPTIONS snapshot, a subshell's copy of the
// runner — keeps meaning what it meant when it was taken. That is about 3.3 KB
// per changed option, and in zsh it is paid on every `emulate -L zsh`, which
// moves `localtraps` at least: measured on the maintainer's real startup
// (#6306), ~16,000 copies and 54 MB a start, nearly all of them a vector made
// by one function call and thrown away at its return.
//
// So a vector the runner made for itself, and has handed to nobody, is
// written in place, and one a function's return lets go of is kept to be
// filled the next time instead of allocating:
//
//   - EditSemantics is the write. The first one copies the vector into the
//     spare, or a new one, and every later one writes that copy until
//     somebody takes it.
//   - KeepSemantics is the take, for a caller that holds the pointer past
//     the next option change: it stops the runner writing that vector in
//     place, so the holder's copy cannot change under it.
//   - RestoreSemantics puts a kept vector back, and keeps the one it
//     replaces as the spare when the runner was its only holder.
//
// A clone points at the same vector as its parent, so a clone takes it from
// both of them (see ownTables). A caller that reads the pointer and puts it
// back before anything else can change an option — `let`'s and the split
// flag's own swap — needs nothing: the swap is a new vector, so the one they
// hold is not the one this file writes.

// EditSemantics returns the runner's vector for a write in place, first
// copying it into one of the runner's own if the one it holds has been
// handed to anybody else.
func (r *Runner) EditSemantics() *Semantics {
	if r.semOwned != nil && r.semOwned == r.Semantics {
		return r.semOwned
	}
	s := r.semSpare
	r.semSpare = nil
	if s == nil {
		s = new(Semantics)
	}
	*s = *r.sem()
	r.Semantics, r.semOwned = s, s
	return s
}

// KeepSemantics returns the runner's vector for a caller that will hold it,
// after which the runner copies it before writing it again.
func (r *Runner) KeepSemantics() *Semantics {
	if r.semOwned == r.Semantics {
		r.semOwned = nil
	}
	return r.Semantics
}

// RestoreSemantics puts back a vector taken with KeepSemantics. The one it
// replaces becomes the spare EditSemantics fills next, when nothing but this
// runner ever held it.
func (r *Runner) RestoreSemantics(s *Semantics) {
	if r.semOwned != nil && r.semOwned == r.Semantics && r.Semantics != s {
		r.semSpare = r.semOwned
	}
	r.semOwned = nil
	r.Semantics = s
}
