// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "slices"

// The line editor walks the shell's own history list, and a command that
// changes that list changes what Up and a search reach.
//
// There are two lists here and there have to be: the editor keeps one, which
// can hold a line the shell's own does not (a line a pattern rule kept
// recallable, a line refused for holding a credential), and the dialect
// keeps the one `fc`, `history` and every `!` reference read. Typing a line
// puts it in both. Everything else that moves the shell's list used to move
// that one alone, so the editor never saw it (#5903).
//
// Measured 2026-10-04 through a pty against zsh 5.9.2, with `GLOBAL_RCS` off
// and a seed file of three `echo RAN$((40+2))<word>` lines — the marker is
// arithmetic so the recalled line's own echo cannot satisfy it:
//
//	route                                       Up   ran
//	.zshrc `fc -R seed`                         ×3   alpha
//	.zshrc `fc -R seed`, HISTFILE two lines     ×4   bravo — the file's after the seed
//	.zshrc `print -s 'echo …prints'`            ×1   prints
//	typed `fc -R seed`                          ×1   charlie
//	typed `fc -R seed`                          ×3   alpha
//	typed `echo …before`, `fc -p seed`          ×1   charlie
//	                                            ×2   bravo
//	then `fc -P`                                ×1   the `fc -p` line again
//	                                            ×2   before
//
// This shell reached none of them: Up walked only lines typed in the session
// and the file it started from. So the rule is the one the table says — after
// any command, the list Up walks is the list the shell holds — and it is
// applied the only way that does not ask each builtin to know about an
// editor: the list is compared with how the session last left it, and a
// difference is a command's change and is taken whole. A line typed at the
// prompt is the session's own change and moves the snapshot with it, so the
// editor's extra lines survive everything except a command that actually
// rewrote the list.
//
// `fc -A` and `fc -W` write a file and leave the list alone, so they move
// nothing here — measured, Up after either is the line before it.

// listSeen is the shell's list as the session last left it; nil is a session
// that has not started following.
type listSeen struct{ lines []string }

// startFollowing takes the list the session starts from and answers what the
// editor should walk.
//
// The file's lines are what it was handed, and that is all a session whose
// startup files read nothing has. One whose startup files did — `fc -R`,
// `print -s` — has a longer list in the shell than the file, ahead of the
// file's lines once they are seeded, and that list is the one to walk.
func (s Shell) startFollowing(earlier []string) []string {
	if s.Runner == nil || s.listSeen == nil {
		return earlier
	}
	held := s.Runner.HistoryEntries()
	s.listSeen.lines = slices.Clone(held)
	if len(held) <= len(earlier) {
		return earlier
	}
	return slices.Clone(held)
}

// followTheList hands the editor the shell's list when a command changed it.
func (s Shell) followTheList(recall recalls) {
	if s.Runner == nil || s.listSeen == nil {
		return
	}
	held := s.Runner.HistoryEntries()
	if slices.Equal(held, s.listSeen.lines) {
		return
	}
	s.listSeen.lines = slices.Clone(held)
	recall.replace(slices.Clone(held))
}

// sawTheList moves the snapshot to the list as it stands, for a change the
// session made itself.
func (s Shell) sawTheList() {
	if s.Runner == nil || s.listSeen == nil {
		return
	}
	s.listSeen.lines = slices.Clone(s.Runner.HistoryEntries())
}
