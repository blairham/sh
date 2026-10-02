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
// The change itself is never attempted. Making it would change the process
// under every Runner in the program, which a library may not do; and as
// anyone but root it is refused, which is what is answered here without
// asking. **As root the assignment is stored and nothing else happens**,
// where the reference becomes that id — a difference recorded rather than
// modeled, and the suite's own row skips itself under root.
func guardIdentityAssignments(r *interp.Runner) {
	for _, id := range []struct {
		name, what string
		current    func() int
	}{
		{"UID", "user ID", os.Getuid},
		{"EUID", "effective user ID", os.Geteuid},
		{"GID", "group ID", os.Getgid},
		{"EGID", "effective group ID", os.Getegid},
	} {
		r.SetAssignmentGuard(id.name, func(r *interp.Runner, value string) (string, bool) {
			if os.Geteuid() == 0 {
				return "", false
			}
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err == nil && n == id.current() {
				return "", false
			}
			return "failed to change " + id.what + ": operation not permitted", true
		})
	}
}
