// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `$userdirs` is the users whose home directory this shell has looked up, to
// the directory it found.
//
// It is empty, and that is a **measurement** rather than a stub, which is the
// one thing about this name worth arguing. The documented reading is that a
// `~user` expansion puts the user in the table, so an empty answer looks
// exactly like a parameter nobody wrote. Measured 2026-09-27 on zsh 5.9.2
// under `-f` from a script file with `zsh/parameter` loaded:
//
//	: ~bhamilton; : ~root; : ~daemon; : ~nobody
//	${#userdirs}      0
//	hash -d zz=/tmp
//	${#nameddirs}     1        ← the control, in the same run
//
// All four of those lookups succeed there — `~root` expands to `/var/root` —
// and the table does not move. The `nameddirs` row is what makes that a
// reading rather than a dead probe: the *neighboring* table fills in the same
// shell on the next line, so the view is live and it is `userdirs` that has
// nothing in it. On this platform the home directory comes from the directory
// service rather than from the password file zsh fills this table out of.
//
// So an empty view is the answer here, and the thing that would make it wrong
// is the reference's own table filling. The test that grades this runs the
// same four lookups and the same control, so the day one of them lands a row
// there, the row is missing here and the test says so at this name.
//
// The `~user` machinery itself is not what is missing, which is the other
// reading to rule out: `~root` expands here too, and did before this.
//
// A **view** and not a stored empty table, for the reason every other
// produced parameter in this dialect is one: a stored table is what a read
// finds first, so a name given one has stopped answering for anything from
// that moment, and the producer is where the answer goes when there is one.
func registerUserDirs(r *interp.Runner) {
	r.SetDynamicAssoc("userdirs", func(*interp.Runner) interp.AssocArray { return nil })
	// Readonly and hidden, measured with the rest of the module:
	// `${(t)userdirs}` is `association-readonly-hide-hideval-special`, and
	// the freeze is what registerAbsentParameters was already putting on the
	// name — a produced table without it takes an assignment into a stored
	// table and shadows itself.
	r.MarkReadonly("userdirs")
	hideModuleParameter(r, "userdirs")
}
