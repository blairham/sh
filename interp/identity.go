// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Identity names one of the four ids a process runs as.
type Identity int

const (
	// IdentityUser is the real user id.
	IdentityUser Identity = iota
	// IdentityEffectiveUser is the effective user id.
	IdentityEffectiveUser
	// IdentityGroup is the real group id.
	IdentityGroup
	// IdentityEffectiveGroup is the effective group id.
	IdentityEffectiveGroup
)

// ChangeProcessIdentity asks the process to become id, through the hook a
// front end that is a shell supplies. handled is false where there is no hook
// to ask, or where this Runner stands in for a subshell. A subshell is a
// separate process in a real shell, so its change of id ends with it. Here it
// would be the whole program's, which nothing could put back.
func (r *Runner) ChangeProcessIdentity(which Identity, id int) (handled bool, err error) {
	if r.ChangeIdentity == nil || r.inSubshell {
		return false, nil
	}
	return true, r.ChangeIdentity(which, id)
}
