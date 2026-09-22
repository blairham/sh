// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import "github.com/blairham/sh/interp"

// `BASH_ALIASES`: the alias table presented as an association.
//
// bash alone. Measured 2026-09-12 against 5.3.15, and against zsh 5.9.2 and
// ksh93, where `${BASH_ALIASES[q]}` is empty with `q` aliased:
//
//	alias q=echo; declare -p BASH_ALIASES   declare -A BASH_ALIASES=([q]="echo" )
//	declare -p BASH_ALIASES                 declare -A BASH_ALIASES=()
//	alias q=echo; echo "${BASH_ALIASES[q]}" echo
//	alias q=echo; echo "${!BASH_ALIASES[@]}" q
//	BASH_ALIASES[w]=date; alias             alias w='date'
//	alias q=echo; unset "BASH_ALIASES[q]"; alias   alias q='echo'
//
// The last two are the pair worth keeping. An assignment **defines an alias**,
// so this is a view of the table in both directions rather than a readout of
// it — and an `unset` of one element does **not** remove the alias, which is
// bash's answer and not an omission here.
//
// A view and not a snapshot, which is the whole reason
// [interp.Runner.SetDynamicAssoc] takes a function: a table filled once would
// be right until the next `alias` line and then quietly wrong, and quietly is
// the word — the caller reads an association and is never told it stopped
// tracking.
//
// The regular aliases and not every kind. bash has no global or suffix alias
// to leave out, so `AliasRegularKind` and "all of them" are the same set here;
// naming the kind is what says the choice was made rather than defaulted.
func registerBashAliases(r *interp.Runner) {
	// The listing, which a produced parameter has to be told about: without
	// it the name is in no table the operand-less walk collects from, so
	// `declare -A` and the bare `declare -p` wrote nothing for a parameter
	// whose own `declare -p BASH_ALIASES` this shell already answered.
	//
	// Measured 2026-09-20 on bash 5.3.20, script files under `env -i
	// PATH=/usr/bin:/bin LC_ALL=C`: a bare `declare -A` writes `declare -A
	// BASH_ALIASES=()` and `declare -A BASH_CMDS=()` in a shell with no
	// aliases, and `declare -A BASH_ALIASES=([q]="echo" )` after `alias
	// q=echo` — so the elements are written here as well as under a name,
	// which is what ListsItsElements says.
	r.SetDynamicDeclaration("BASH_ALIASES", interp.ProducedDeclaration{Array: true, ListsItsElements: true})

	r.SetDynamicAssoc("BASH_ALIASES", func(rr *interp.Runner) interp.AssocArray {
		names := rr.AliasNames(interp.AliasRegularKind)
		table := make(interp.AssocArray, len(names))
		for _, name := range names {
			value, _ := rr.LookupAlias(name)
			table[name] = interp.Scalar(value)
		}
		return table
	})
	// One key without building the table. The alias table is a map the
	// runner already holds, so the whole-table producer is cheap and this is
	// not about cost — it is that the two readings must agree, and a lookup
	// through the same table is the shortest way to be sure they do.
	r.SetDynamicAssocElement("BASH_ALIASES", func(rr *interp.Runner, key string) (string, bool) {
		return rr.LookupAlias(key)
	})
	// And the write, which is a definition. `set` false is the unset, and it
	// does nothing: measured, `unset "BASH_ALIASES[q]"` leaves `q` aliased.
	// Registered rather than left out, because without a writer the
	// assignment would land in the stored table and the view would silently
	// become a snapshot of the moment somebody wrote to it — see
	// [interp.Runner.SetDynamicAssocWriter].
	r.SetDynamicAssocWriter("BASH_ALIASES",
		func(rr *interp.Runner, key, value string, set bool) {
			if !set {
				return
			}
			if rr.AliasNameRefused(key) {
				// The table is the alias table, so a name an alias may not
				// carry is refused by this road as squarely as by the
				// builtin — with the builtin's name left off, since no
				// builtin was called. Measured 2026-09-22 on bash 5.3.20
				// from a script file: `BASH_ALIASES['\$']=xx` writes
				// ``s.sh: line 1: \`\$': invalid alias name``, leaves the
				// status at 0, and defines nothing, and `BASH_ALIASES['a
				// b']=x` answers the same for the blank.
				rr.Diagnosef("`%s': invalid alias name\n", key)
				return
			}
			rr.DefineAlias(key, value, interp.AliasRegularKind)
		})
}
