// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// moveCloseIsForKeeps reports whether the close half of a move outlives the
// command that made it — whether `N<&M-` leaves M closed once the command
// has ended.
//
// FdMoveRelocates never does: the move is one redirection and the command
// takes all of it back. FdMoveDuplicatesThenCloses does, and the measuring
// that said so held one thing fixed that turns out to decide it: the
// destination had been closed. bash gives the source back wherever the
// destination was open as the move was made, wherever the destination is a
// `{name}`'s fresh number, and wherever the command is not made in this
// shell. Measured 2026-10-15 on bash 5.3.20, `-c`, after `exec 9</dev/null`,
// then `true <&9`:
//
//	true 4<&9-                         9 closed
//	true 3<&9-   true 11<&9-           9 closed
//	exec 2>&-; true 2>&9-              9 closed   (2 was closed)
//	exec 4</dev/null; true 4<&9-       9 open
//	true 0<&9-   true 1>&9-   f 0<&9-  9 open
//	true 4</dev/null 4<&9-             9 open     (open by the move's turn)
//	true 5<&- 5<&8-  (5 open before)   8 closed   (closed by the move's turn)
//	true {v}<&9-   { :; } {v}<&9-      9 open, and v's 10 stays open
//	/usr/bin/true 4<&9-   ( : ) 4<&9-  9 open
//	exec 4<&9-   exec {v}<&9-          9 closed   (exec keeps every half)
//
// So the openness is read at the move's own turn, not at the command's
// start. The program and the parentheses are a process of their own in a
// real shell, so nothing they close was ever this shell's; the `{name}` row
// is the issue this was found by (#6626), where ours took the persisting
// destination as reason to keep the close.
func (r *Runner) moveCloseIsForKeeps(fdVar string, destOpen bool) bool {
	return r.sem().FdMove == FdMoveDuplicatesThenCloses &&
		fdVar == "" && !destOpen && r.redirOwner == redirOwnerThisShell
}
