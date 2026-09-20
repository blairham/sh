// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import "github.com/blairham/sh/interp"

// `BASH_CMDS`: the command hash presented as an association.
//
// bash alone, and the same shape `BASH_ALIASES` has — a view of a table the
// runner already keeps, in both directions. Measured 2026-09-13 against
// 5.3.15:
//
//	ls >/dev/null; echo "${BASH_CMDS[ls]}"    /bin/ls
//	ls >/dev/null; echo "${!BASH_CMDS[@]}"    ls
//	hash -p /bin/ls zz; declare -p BASH_CMDS  declare -A BASH_CMDS=([zz]="/bin/ls" )
//	BASH_CMDS[w]=/bin/date; hash              hits/command, 0 /bin/date
//	BASH_CMDS[w]=/bin/date; hash -t w         /bin/date
//	hash -p /bin/ls zz; unset "BASH_CMDS[zz]"; hash   still there
//
// So an assignment **hashes a command**, exactly as `hash -p` does and with
// the same zero hit count, and an `unset` of one element does *not* take the
// entry away. Both of those are bash's answers rather than omissions here,
// and the second is the one worth writing down: the pair of them is why this
// is a view and not a copy.
//
// A view and not a snapshot, which is what [interp.Runner.SetDynamicAssoc]
// taking a function is for: a table filled once would be right until the next
// command ran and then quietly wrong.
func registerBashCmds(r *interp.Runner) {
	// The listing, for the reason the alias table's is — see
	// registerBashAliases, where the measurement is. Both tables are in the
	// operand-less `declare -A` in bash 5.3.20 and were in neither listing
	// here.
	r.SetDynamicDeclaration("BASH_CMDS", interp.ProducedDeclaration{Array: true, ListsItsElements: true})
	r.SetDynamicAssoc("BASH_CMDS", func(rr *interp.Runner) interp.AssocArray {
		names := rr.HashedCommandNames()
		table := make(interp.AssocArray, len(names))
		for _, name := range names {
			path, _ := rr.HashedCommandPath(name)
			table[name] = interp.Scalar(path)
		}
		return table
	})
	// One key without building the table, for the reason BASH_ALIASES has
	// the same pair: the two readings must agree, and reading through the
	// same table is the shortest way to be sure they do.
	r.SetDynamicAssocElement("BASH_CMDS", func(rr *interp.Runner, key string) (string, bool) {
		return rr.HashedCommandPath(key)
	})
	// And the write, which is a hashing. `set` false is the unset, and it
	// does nothing — measured above. Registered rather than left out,
	// because without a writer the assignment would land in the stored table
	// and the view would silently become a snapshot of the moment somebody
	// wrote to it; see [interp.Runner.SetDynamicAssocWriter].
	r.SetDynamicAssocWriter("BASH_CMDS",
		func(rr *interp.Runner, key, value string, set bool) {
			if !set {
				return
			}
			rr.HashCommand(key, value)
		})
}
