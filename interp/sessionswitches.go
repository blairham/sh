// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Three capabilities an interactive session has and a script cannot see, and
// the seams a dialect and a front end reach them through.
//
// They are here rather than in a dialect because none of them is one shell's
// idea. Keeping $COLUMNS abreast of the window, reading a bare directory name
// as a `cd` and offering every command for an empty word are behaviors; the
// names `checkwinsize`, `autocd` and `no_empty_cmd_completion` are bash's
// spellings of them, and `autocd` is *also* zsh's spelling of the second one.
// A capability written into whichever dialect asked for it first is a
// capability the next dialect writes again — this repository has paid for that
// five times — so the behavior is the core's and each dialect only names it.
//
// Each pair is a getter and a setter rather than an exported field, for the
// reason every other switch here is: one of the three stores its state
// inverted, and a caller reaching past the accessor would be storing the
// option's bit rather than the shell's state. See
// Runner.emptyCommandWordOffersNothing.

// TracksWindowSize reports whether this shell keeps $LINES and $COLUMNS
// abreast of the terminal.
//
// For the front end, which is the only half of a shell holding a terminal to
// ask. interp cannot answer the window's size and does not try: it holds the
// permission, and repl holds the ioctl.
func (r *Runner) TracksWindowSize() bool { return r.tracksWindowSize }

// SetTracksWindowSize moves it, for a dialect naming the capability —
// `shopt -s checkwinsize` — or setting the default its shell starts with.
func (r *Runner) SetTracksWindowSize(on bool) { r.tracksWindowSize = on }

// AutoCd reports whether a bare directory name is read as a `cd` here.
//
// Exported for the two dialects that name it, both of which call it `autocd`.
// The behavior is in this package — see Runner.autoCdInstead — because a
// dialect that implemented it would be implementing it for the other one too,
// or, far more likely, instead of it.
func (r *Runner) AutoCd() bool { return r.autoCd }

// SetAutoCd moves it.
func (r *Runner) SetAutoCd(on bool) { r.autoCd = on }

// CompletesEmptyCommandWord reports whether completing an empty command word
// offers everything that could run — every builtin, function, reserved word
// and executable on PATH.
//
// True is what this shell does with nothing said, which is why the field
// behind it stores the *deviation*: the zero value of a Runner is the shell
// this has always been.
//
// For the front end, which does the completing. The state is here because a
// dialect's option builtin is what moves it and a script may move it between
// two keystrokes, so a front end that read it once at the start of a session
// would be answering with a setting the session had already changed.
func (r *Runner) CompletesEmptyCommandWord() bool { return !r.emptyCommandWordOffersNothing }

// SetCompletesEmptyCommandWord moves it, in the positive direction the getter
// reads.
//
// A dialect whose name for this is a negative — bash's
// `no_empty_cmd_completion` — inverts at its own table and not here, so that
// there is exactly one place in the program where the sense of the bit is
// decided.
func (r *Runner) SetCompletesEmptyCommandWord(on bool) { r.emptyCommandWordOffersNothing = !on }

// CorrectsCdSpelling reports whether `cd` corrects a misspelled operand
// instead of refusing it.
//
// One shell in the panel names this (`cdspell`) and one names the same
// correction asked for by the completer (`dirspell`); both names are that
// dialect's and the correction is here, in spellcorrect.go. See
// Runner.correctPath.
func (r *Runner) CorrectsCdSpelling() bool { return r.cdCorrectsSpelling }

// SetCorrectsCdSpelling moves it.
func (r *Runner) SetCorrectsCdSpelling(on bool) { r.cdCorrectsSpelling = on }
