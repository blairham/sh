// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"

	"github.com/blairham/sh/interp"
)

// The `logout` builtin: `exit`, for a login shell, and a refusal for every
// other shell.
//
// Measured 2026-09-16 against bash 5.3.20 and 3.2.57 alike, `env -i` with no
// startup files:
//
//	bash ./s.sh, `logout; echo st=$?`         not login shell: use `exit'  st=1
//	the same with `logout 3`, and `(logout)`  the same refusal, st=1
//	bash -l ./s.sh, `logout 4; echo after`    the script ends at 4
//	bash -l -c '(logout 4); echo sub=$?'      the refusal, sub=1
//
// So the subshell of a login shell is not one, and the refusal carries on.
// Before this the word was `command not found` at 127. One login-shell row is
// recorded and not modeled: `bash -l -c 'logout abc; echo after'` writes
// `exit`'s numeric-argument complaint and then runs `after`, where `exit abc`
// ends the shell; this delegates to `exit` and ends it too.
//
// zsh has the builtin with another refusal — `not login shell`, and the
// script ends — and ksh93 and dash have none.
func registerLogout(r *interp.Runner) { r.Register("logout", logoutBuiltin) }

func logoutBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	if r.LoginShell && !r.InSubshell() {
		if exit, ok := r.Builtin("exit"); ok {
			return exit(r, ctx, args)
		}
	}
	r.Diagnosef("logout: not login shell: use `exit'\n")
	return 1
}
