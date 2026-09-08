// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The capabilities an interactive session has and a script cannot see, and
// the seams a dialect and a front end reach them through.
//
// They are here rather than in a dialect because none of them is one shell's
// idea. Keeping $COLUMNS abreast of the window, reading a bare directory name
// as a `cd`, offering every command for an empty word and looking at the job
// table before leaving are behaviors; the names `checkwinsize`, `autocd`,
// `no_empty_cmd_completion` and `checkjobs` are bash's spellings of them, and
// `autocd` and `checkjobs` are *also* zsh's spellings of two of them.
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

// ChecksRunningJobsAtExit reports whether a job that is still *running* holds
// this shell's exit back, the way a stopped one does.
//
// Both shells in the panel that hold an exit at all have a name for this and
// they are the same name — bash's `shopt -s checkjobs` and zsh's
// `setopt checkjobs` — which is the third time a capability has arrived under
// one spelling in two option namespaces, so it is the core's and neither
// dialect's. They start it in different places: measured through a
// pseudo-terminal, bash 5.3.15 leaves at once with `sleep 40 &` in the table
// and zsh 5.9.2 says `you have running jobs.` and stays, so bash's default is
// off and zsh's is on. Each dialect starts it where its shell does.
//
// It also decides whether the sentence is followed by the job table, and that
// is not a second switch because it is not a second name: bash's one word
// asks for both halves of "check the jobs", and measured, the listing appears
// under `shopt -s checkjobs` and not under `shopt -u checkjobs` — for stopped
// jobs as much as for running ones. Whether this shell lists at all is
// Semantics.HeldExitListsTheJobs, since zsh never does.
func (r *Runner) ChecksRunningJobsAtExit() bool { return r.checksRunningJobsAtExit }

// SetChecksRunningJobsAtExit moves it.
func (r *Runner) SetChecksRunningJobsAtExit(on bool) { r.checksRunningJobsAtExit = on }

// ChecksStoppedJobsAtExit reports whether a *stopped* job holds the exit —
// the older half of the same question, and the one Semantics answers by
// default.
//
// It is a switch as well as an axis because one shell lets a session turn it
// off and the other does not. Measured through a pseudo-terminal: zsh 5.9.2
// with `unsetopt checkjobs` and a job suspended with ^Z leaves at the first
// `exit` and says nothing, while bash 5.3.15 with `shopt -u checkjobs` still
// says `There are stopped jobs.` and stays. So zsh's `checkjobs` is the
// master switch over both kinds and bash's governs only the running ones.
//
// The field behind it stores the *deviation*, so a Runner nobody has spoken
// to follows Semantics.StoppedJobsHoldTheExit — which is where dash and ksh93,
// who hold for neither kind, are answered.
func (r *Runner) ChecksStoppedJobsAtExit() bool { return !r.stoppedJobExitCheckOff }

// SetChecksStoppedJobsAtExit moves it, in the positive direction the getter
// reads.
func (r *Runner) SetChecksStoppedJobsAtExit(on bool) { r.stoppedJobExitCheckOff = !on }
