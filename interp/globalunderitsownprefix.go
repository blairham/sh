// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "slices"

// A `-g` declaration standing behind an assignment prefix of **its own** —
// `w=7 declare -g w=3` — which writes the shell's own cell underneath the
// binding that prefix made.
//
// The third of the three mechanisms a `-g` write can meet, and the innermost:
// interp/globalunderacallprefix.go is an *enclosing call's* prefix, the
// scopes are a `local`, and this is the entry the running builtin's own
// command made. Each is a different stack and each had to be lifted where it
// stands.
//
// Measured 2026-09-18 on bash 5.3.20 and zsh 5.9.2, from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME and no
// `set -o posix`, each line read back with `${w-U}` and `declare -p`:
//
//	w=7 declare w=3       bash  U                 zsh  U
//	x=7 declare -g x=3    bash  3                 zsh  U
//	y=7 declare -g y      bash  U, declare -- y   zsh  U
//	v=1; v=7 declare -g v=3    bash  3            zsh  1
//
// The first line is the control and is what makes the second one about the
// letter: without it the prefix's value goes away with the command in both
// columns, which is what this engine already did everywhere. The fourth says
// the write lands on a cell that was already there rather than merely
// outliving the prefix.
//
// # The promotion is not asked where the write goes underneath
//
// One dialect **keeps** a prefix's value where the declaration in front of it
// names the export or readonly attribute over the same name — `a=7 declare -x
// a` leaves `declare -x a="7"` behind, Semantics.DeclarationPromotesThePrefixEntry.
// That question is about the prefix's own binding, and a `-g` declaration is
// not writing it. Measured on the pairs that say so:
//
//	a=7 declare -x a      declare -x a="7"    promoted
//	t=7 declare -gx t     declare -x t        the attribute alone, no value
//	c=7 declare -r c      declare -rx c="7"   promoted, prefix export and all
//	b=7 declare -gr b=3   declare -r b="3"    no export: that was the prefix's
//
// Rows two and four are the letter taking the question away: the value the
// prefix set is gone as it is from any other command, and what is left is
// what the declaration wrote on the cell underneath — which never had the
// export attribute the prefix put on the temporary.

// liftedOwnPrefix is one name's entry in the running command's own assignment
// prefix, taken off for the length of a global declaration.
type liftedOwnPrefix struct {
	name string
	// at is where the pre-prefix state is kept, found again by index because
	// the slice is the one the command's take-back will walk.
	at int
	// live is what the name held with the prefix on it, put back when the
	// declaration is done.
	live savedVar
}

// globalDeclarationRunsUnderItsOwnPrefix lifts the running command's own
// assignment-prefix entry for each name a `-g` declaration is about to write,
// and hands back the function that puts it on again.
//
// What the declaration leaves underneath becomes the state the command's own
// take-back restores, which is the whole of the measurement: the prefix's
// value goes away as it always does, and the write outlives it.
func (r *Runner) globalDeclarationRunsUnderItsOwnPrefix(names []string) func() {
	var lifted []liftedOwnPrefix
	for _, name := range names {
		at := slices.IndexFunc(r.prefixHeldUndo, func(u savedVar) bool { return u.name == name })
		if at < 0 {
			// Not held by this command's prefix, or held and not saved —
			// which is a prefix the dialect keeps, and then there is nothing
			// underneath it to write.
			continue
		}
		if !r.ask(r.sem().DeclareGlobalReachesPastALocal,
			"`declare -g` writing under its own command's assignment prefix") {
			// The column that writes what is visible, which is the prefix's
			// temporary and leaves with it.
			continue
		}
		live := r.saveVar(name)
		// restoreVar rather than a write, for the reason the call-prefix lift
		// gives: what the prefix displaced is a whole state — the value, the
		// array, the export tri-state, the attributes — and all of it is what
		// the declaration is writing over.
		r.restoreVar(r.prefixHeldUndo[at])
		lifted = append(lifted, liftedOwnPrefix{name: name, at: at, live: live})
		r.globalUnderItsOwnPrefix = append(r.globalUnderItsOwnPrefix, name)
	}
	if lifted == nil {
		return func() {}
	}
	return func() {
		for i := len(lifted) - 1; i >= 0; i-- {
			l := lifted[i]
			if l.at < len(r.prefixHeldUndo) && r.prefixHeldUndo[l.at].name == l.name {
				r.prefixHeldUndo[l.at] = r.saveVar(l.name)
			}
			r.restoreVar(l.live)
			if at := slices.Index(r.globalUnderItsOwnPrefix, l.name); at >= 0 {
				r.globalUnderItsOwnPrefix = slices.Delete(r.globalUnderItsOwnPrefix, at, at+1)
			}
		}
	}
}
