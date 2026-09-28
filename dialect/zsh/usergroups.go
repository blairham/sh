// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"os"
	"os/user"
	"strconv"

	"github.com/blairham/sh/interp"
)

// `$usergroups` is the groups the *process* is in, by name, to the numeric id
// of each.
//
// One of the two names in #4909's roster that needed a source outside the
// shell, and the source is the process's own group list rather than the group
// database: `os.Getgroups` is what `id -G` reads and it is the set that
// decides what this shell may open, where a walk of every group on the
// machine would be a different and much longer answer. The name is then
// looked up per id, which is the only part that consults the database.
//
// Measured 2026-09-27 on zsh 5.9.2 under `-f` from a script file with
// `zsh/parameter` loaded: sixteen entries on this machine, `staff -> 20`,
// `everyone -> 12`, `admin -> 80` and the rest, which is exactly what
// `os.Getgroups` returns here.
//
// **A group with no name is written under its number**, which is measured
// rather than chosen — a container with a sparse `/etc/group` is the ordinary
// way to reach one. zsh writes the id as the key there, so an entry is never
// dropped: the count of this table and the length of `$GROUPS` are the same
// fact and a script comparing them must not find them disagreeing because a
// lookup failed.
//
// A **view** rather than a table filled once, for the reason every other
// produced parameter here is one: `setgid` is not a thing a script does, but
// two Runners in one program share no state and a snapshot taken at
// registration would be handed to both.
func zshUserGroupsView(*interp.Runner) interp.AssocArray {
	ids, err := os.Getgroups()
	if err != nil {
		// No group list to report — Windows, and any platform whose
		// process has none. Empty rather than absent: the parameter is
		// there and the answer is that this process is in no group, which
		// is the same shape `$GROUPS` takes in the same place.
		return interp.AssocArray{}
	}
	out := make(interp.AssocArray, len(ids))
	for _, id := range ids {
		number := strconv.Itoa(id)
		name := number
		if g, err := user.LookupGroupId(number); err == nil && g.Name != "" {
			name = g.Name
		}
		out[name] = interp.Scalar(number)
	}
	return out
}

// registerUserGroups installs `$usergroups`.
//
// Readonly and hidden, the pair measured for every module table here:
// `${(t)usergroups}` is `association-readonly-hide-hideval-special`.
func registerUserGroups(r *interp.Runner) {
	r.SetDynamicAssoc("usergroups", zshUserGroupsView)
	r.MarkReadonly("usergroups")
	hideModuleParameter(r, "usergroups")
}
