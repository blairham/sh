// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The two answers a shell gives about what its history *keeps*: what one
// entry is when a command took more than one line to type, and what becomes
// of the file when the session ends.
//
// Here rather than in the front end for the reason HistoryExpansionVerifies
// is here: a script moves both of them by name — `shopt -s lithist`, `shopt
// -u histappend` — and a bit only the front end held would be unreachable
// from the line that asks for it. What each one *does* is the front end's,
// because only a front end has a session, a list and a file.
//
// Both are named for the state this core is in with nothing said, which is
// the state every shell that has no such option is in: the entry keeps the
// lines it was typed on, and the file is appended to. A preset that wants
// bash's defaults turns both on — see dialect/bash's Configure — and the two
// option names are the negatives of these, which is the shape
// `no_empty_cmd_completion` and `globskipdots` already have.

// HistoryJoinsATypedCommand reports whether a command typed over several
// physical lines becomes **one joined entry**, with a separator where each
// newline was, rather than an entry holding those newlines.
//
// bash's name for the other side of this is `lithist`, off by default, and
// the two states were measured 2026-09-22 against bash 5.3.20 by reading its
// own history file back after a session that typed
//
//	for q in ZZ / do / echo MARK$q / done
//
//	(default)              for q in ZZ; do echo MARK$q; done
//	shopt -s lithist       the four lines, as they were typed
//
// So the joined form is what bash records with nothing said, and `lithist`
// asks for the literal one. This shell recorded the literal one either way,
// which is what put the name in the refusing table: `shopt -u lithist` asked
// for a joining nothing did.
//
// The join itself is not written twice. internal/histjoin already holds the
// rule — a `;` unless the text so far ends in something that cannot take one,
// a newline across a quote or a here-document — because a script's own
// history gate and `fc`'s editor road both reached the same question first.
// This makes a prompt the third reader of it rather than the author of a
// second copy.
func (r *Runner) HistoryJoinsATypedCommand() bool { return r.histJoinLines }

// SetHistoryJoinsATypedCommand moves it.
func (r *Runner) SetHistoryJoinsATypedCommand(on bool) { r.histJoinLines = on }

// HistoryKeepsATypedCommandWhole reports whether a command typed over several
// physical lines is **one entry** at all, which is a different question from
// what goes between its lines.
//
// bash's name for the other side is `cmdhist`, **on** by default, and the two
// bits compose in one direction only. Measured 2026-09-23 on bash 5.3.15
// through a pty, typing `for i in 1 2` / `do` / `  echo $i` / `done` and then
// reading `history`:
//
//	cmdhist on,  lithist off   one entry, `for i in 1 2; do   echo $i; done`
//	cmdhist on,  lithist on    one entry, holding the newlines
//	cmdhist off, lithist off   four entries
//	cmdhist off, lithist on    four entries
//
// So this bit decides the **count** and HistoryJoinsATypedCommand decides only
// the separator inside one entry — which makes `lithist` moot once `cmdhist`
// is off, and is why two bits are needed rather than three states of one.
//
// **The history file cannot measure this.** Three of those four rows write
// four lines to `$HISTFILE`, because one entry holding newlines and four
// separate entries are the same bytes there. `history`'s own numbering is what
// separates them: one entry numbered 3 against four numbered 3 to 6. The first
// instrument read the file and reported the option doing nothing (#4149).
//
// On with nothing said, which is both this core's state — it records the
// construct it was given — and bash's default, so a preset has nothing to say
// unless a script moves it.
func (r *Runner) HistoryKeepsATypedCommandWhole() bool { return !r.histSplitLines }

// SetHistoryKeepsATypedCommandWhole moves it.
func (r *Runner) SetHistoryKeepsATypedCommandWhole(on bool) { r.histSplitLines = !on }

// RewritesTheHistoryFile reports whether a session that ends writes its list
// over the history file rather than appending what it added to whatever the
// file now holds.
//
// bash's name for the other side of this is `histappend`, off by default —
// and what the option actually decides is narrower than "append or not",
// which is the whole reason this bit is worth having rather than assuming.
// Measured 2026-09-22 against bash 5.3.20 through a pseudo-terminal, eight
// lines already in HISTFILE and a session typing four:
//
//	HISTSIZE=20 HISTFILESIZE=100, default   the eight, then the session's
//	HISTSIZE=2  HISTFILESIZE=100, default   two lines — the eight are gone
//	HISTSIZE=2  HISTFILESIZE=100, -s        the eight, then two
//
// So bash appends in the ordinary case **with the option off**, and rewrites
// only where the list no longer holds every line the session added — which is
// what HISTSIZE trimming it below that count does. `shopt -s histappend` is
// what makes it append there too. Four further probes say the same from the
// other side: with the option off, a line another process appended to
// HISTFILE mid-session survives, a HISTFILE truncated mid-session is left
// truncated, and a HISTFILE the filesystem marks append-only is written
// without complaint — none of which a rewrite could do.
//
// That measurement is why the name could not simply be recorded at bash's
// default: this shell appends unconditionally, so it sits on the option's
// **on** side in the one case that tells them apart, and reporting `off`
// without the rewrite would be the listing saying what the file does not do.
func (r *Runner) RewritesTheHistoryFile() bool { return r.histRewrite }

// SetRewritesTheHistoryFile moves it.
func (r *Runner) SetRewritesTheHistoryFile(on bool) { r.histRewrite = on }
