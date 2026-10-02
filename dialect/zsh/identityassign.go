// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"os"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// Assigning one of the four identity parameters asks the system to become
// that id, and a shell that is not root is refused — so the assignment is.
// Measured 2026-10-02 on zsh 5.9.2 under `-f`, uid 501, B02typeset's `when
// cannot change UID, the command isn't run` (#5142):
//
//	UID=$((UID+1)) /bin/echo ran     failed to change user ID: operation not
//	                                 permitted, status 1, nothing run, the
//	                                 script goes on
//	UID=$((UID+1))                   the same sentence, and the shell ends at 1
//	UID=$((UID+1)) :, … f            the same: a builtin and a function run in
//	                                 the shell
//	UID=$UID                         fine: the id it already is
//	EUID, GID, EGID                  effective user ID, group ID, effective
//	                                 group ID in the sentence
//
// The change is made where the front end allows it — interp.Runner.
// ChangeIdentity, which a binary that is a shell supplies and a library does
// not — and the kernel's answer is the answer. As root the assignment really
// does change the process: measured 2026-10-02 on zsh 5.9.2 in the suite's
// image, as root, `EUID=1; echo $EUID; id -u` writes `1` twice, and a second
// `EUID=10` is then refused with the sentence above, because the process is
// no longer root (#5157). E03posix's `EUID is not a special variable` is that
// row, and an assignment that stored the value and changed nothing passed it
// where the reference fails it.
//
// Where nothing allows it — a library, or a Runner standing in for a
// subshell — the change is not attempted. As anyone but root it would be
// refused, so that is answered here without asking; as root the value is
// stored and nothing else happens.
func guardIdentityAssignments(r *interp.Runner) {
	for _, id := range []struct {
		name, what string
		which      interp.Identity
		current    func() int
	}{
		{"UID", "user ID", interp.IdentityUser, os.Getuid},
		{"EUID", "effective user ID", interp.IdentityEffectiveUser, os.Geteuid},
		{"GID", "group ID", interp.IdentityGroup, os.Getgid},
		{"EGID", "effective group ID", interp.IdentityEffectiveGroup, os.Getegid},
	} {
		r.SetAssignmentGuard(id.name, func(r *interp.Runner, value string) (string, bool) {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err == nil && n == id.current() {
				return "", false
			}
			if err == nil {
				if handled, cerr := r.ChangeProcessIdentity(id.which, n); handled {
					if cerr != nil {
						return "failed to change " + id.what + ": " + cerr.Error(), true
					}
					return "", false
				}
			}
			if os.Geteuid() == 0 {
				return "", false
			}
			return "failed to change " + id.what + ": operation not permitted", true
		})
	}
}
