// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"os/user"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// `$userdirs` is every account in the password database, to its home
// directory — in an interactive shell — and nothing in any other.
//
// Empty in a script is a **measurement** rather than a stub. The documented
// reading is that a `~user` expansion puts the user in the table, so an empty
// answer looks exactly like a parameter nobody wrote. Measured 2026-09-27 on
// zsh 5.9.2 under `-f` from a script file with `zsh/parameter` loaded:
//
//	: ~bhamilton; : ~root; : ~daemon; : ~nobody
//	${#userdirs}      0
//	hash -d zz=/tmp
//	${#nameddirs}     1        ← the control, in the same run
//
// All four of those lookups succeed there — `~root` expands to `/var/root` —
// and the table does not move, while the *neighboring* table fills in the
// same shell on the next line, so the view is live and it is `userdirs` that
// has nothing in it.
//
// **Interactive, the table is full**, and that is what `compadd -k userdirs`
// — the login names `ssh <Tab>` offers — reads (#6156). Measured 2026-10-06
// on the same build, `print ${#userdirs}`:
//
//	zsh -f -c        0        zsh -c          0      a script on stdin   0
//	zsh -f -i -c   133        zsh -i -c     133      and in a completion
//	                                                 function at a prompt 133
//
// with standard input `/dev/null` for the two `-i` rows, so it is the shell
// being interactive and not a terminal. The 133 are the 132 accounts of
// `/etc/passwd` and the person's own, whose account on this platform is in the
// directory service rather than in that file; `root` is `/var/root`.
//
// This shell reads `/etc/passwd` and adds the account it is running as. That
// is the whole database where the file is the database, which is Linux
// without a directory service; on macOS it misses any *other* account the
// directory service holds, which reading the database there would need the C
// library for, and the binaries are built without it.
//
// The `~user` machinery itself is not what is missing, which is the other
// reading to rule out: `~root` expands here too, and did before this.
//
// A **view** and not a stored empty table, for the reason every other
// produced parameter in this dialect is one: a stored table is what a read
// finds first, so a name given one has stopped answering for anything from
// that moment, and the producer is where the answer goes when there is one.
func registerUserDirs(r *interp.Runner) {
	r.SetDynamicAssoc("userdirs", zshUserDirsView)
	// Readonly and hidden, measured with the rest of the module:
	// `${(t)userdirs}` is `association-readonly-hide-hideval-special`, and
	// the freeze is what registerAbsentParameters was already putting on the
	// name — a produced table without it takes an assignment into a stored
	// table and shadows itself.
	r.MarkReadonly("userdirs")
	// Silent to `-p`, as zsh/parameter's frozen tables are; see
	// interp.Runner.SetSilentToPrint.
	r.SetSilentToPrint("userdirs")
	hideModuleParameter(r, "userdirs")
}

// zshUserDirsView is `$userdirs` as it stands. See the file comment.
func zshUserDirsView(r *interp.Runner) interp.AssocArray {
	if !r.Interactive {
		return nil
	}
	out := interp.AssocArray{}
	for name, home := range repl.AccountHomes(repl.AccountFile) {
		out[name] = interp.Scalar(home)
	}
	if u, err := user.Current(); err == nil && u.Username != "" && u.HomeDir != "" {
		if _, ok := out[u.Username]; !ok {
			out[u.Username] = interp.Scalar(u.HomeDir)
		}
	}
	return out
}
