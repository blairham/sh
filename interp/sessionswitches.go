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

// ChecksHashedCommand reports whether the command hash is looked at before it
// is believed.
//
// bash's `shopt -s checkhash` is the only spelling in the panel, and it is
// here rather than in that dialect for the reason every switch in this file
// is: the *behavior* is the core's — lookPath either falls back to a fresh
// search or does not — and three of the four shells do it with nothing set.
// See Semantics.CommandHashIsTrusted, which this can only turn down.
func (r *Runner) ChecksHashedCommand() bool { return r.checksHashedCommand }

// SetChecksHashedCommand moves it.
func (r *Runner) SetChecksHashedCommand(on bool) { r.checksHashedCommand = on }

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

// SearchesPathForSource reports whether `.` looks along $PATH for an operand
// with no slash in it — bash's `sourcepath`, which is on by default.
//
// One shell in the panel names it and the other four have the search with no
// way to turn it off, so the *capability* is core and the switch is bash's.
// Turning it off makes a bare operand a path relative to the shell's own
// directory and nothing else, which is what a shell with neither half of the
// mechanism does — and which is right often enough to be invisible until a
// script relies on either (#3058).
//
// `. -p list file` is the other half and is not this: an explicit list wins
// over the switch, so a `-p` still searches with `sourcepath` off.
func (r *Runner) SearchesPathForSource() bool { return !r.dotSearchesPathOff }

// SetSearchesPathForSource moves it.
func (r *Runner) SetSearchesPathForSource(on bool) { r.dotSearchesPathOff = !on }

// ReportsShiftPastTheEnd reports whether `shift` says anything when its count
// is above `$#` — bash's `shift_verbose`, which is off there by default.
//
// One shell in the panel names the question and the other four simply answer
// it: dash, ksh93 and zsh have a sentence for the count and always write it,
// and this shell is silent because its Diagnostics carries no wording rather
// than because anything withheld one. So the capability is the *withholding*,
// which is why the field behind this stores the deviation and a Runner that
// was never told reports.
//
// It governs the count above `$#` alone. The other end of the range —
// Diagnostics.ShiftNegativeCount — is written whatever this says, measured:
// the shell with the option names a negative count with it off as well as on.
func (r *Runner) ReportsShiftPastTheEnd() bool { return !r.shiftPastEndQuiet }

// SetReportsShiftPastTheEnd moves it.
func (r *Runner) SetReportsShiftPastTheEnd(on bool) { r.shiftPastEndQuiet = !on }

// ReportsLoopControlOutsideALoop reports whether a `break` or `continue` with
// no loop around it says so.
//
// The same shape ReportsShiftPastTheEnd has and for the same reason: two
// columns are silent here because their Diagnostics carries no wording rather
// than because anything withheld one, so the field behind this stores the
// deviation and a Runner that was never told reports. What moves it is POSIX
// mode in the one column whose mode moves it — see
// Semantics.LoopControlOutsideALoopSilentInPosixMode.
//
// It governs the sentence alone. Whether the misuse also ends the script is
// Semantics.LoopControlOutsideALoopIsFatal, which is read whatever this says:
// the column that stops is not the column that withholds.
func (r *Runner) ReportsLoopControlOutsideALoop() bool { return !r.loopControlQuiet }

// SetReportsLoopControlOutsideALoop moves it.
func (r *Runner) SetReportsLoopControlOutsideALoop(on bool) { r.loopControlQuiet = !on }

// ExpandsAnOperandsSubscriptAgain reports whether a subscript that reaches a
// builtin as **text** — `unset -v 'a[$k]'`, `printf -v 'c[$k]'`, `read
// 'b[$k]'`, `test -v 'g[$k]'` — is expanded once more before the element is
// found, which is bash's `assoc_expand_once` read the way round the shell
// behaves rather than the way round the option is named.
//
// One shell in the panel names the question and it names the suppression:
// `shopt -s assoc_expand_once` asks for the round to *stop*, and
// `array_expand_once` is the same switch under a second name. So the table
// inverts and this does not, for the reason CompletesEmptyCommandWord gives —
// there is exactly one place in the program where the sense of the bit is
// decided.
//
// It is a permission and not a behavior: the round happens where a dialect's
// axis says it does and this can only turn that down. The three axes are
// Semantics.UnsetExpandsAFlatSubscript,
// Semantics.OutputOperandExpandsAFlatSubscript and
// Semantics.TestIsSetExpandsAFlatSubscript, and the surfaces they cover are
// the four this option was measured to move. Two neighboring surfaces round
// under axes of their own and are deliberately out of reach here, because
// bash does not move them either: a declaration's operand and `[[ -v ]]`
// find the key `x y` with the option set and unset alike, measured
// 2026-09-19.
func (r *Runner) ExpandsAnOperandsSubscriptAgain() bool { return !r.operandSubscriptExpandedOnce }

// SetExpandsAnOperandsSubscriptAgain moves it, in the positive direction the
// getter reads.
func (r *Runner) SetExpandsAnOperandsSubscriptAgain(on bool) { r.operandSubscriptExpandedOnce = !on }

// EchoExpandsEscapes reports whether `echo` interprets its backslash escapes
// with no `-e` in front of them — bash's `xpg_echo`.
//
// Semantics.EchoInterpretsEscapes is where a dialect stands on this, and this
// is a script asking to stand on the other side of it while the shell runs.
// One shell in the panel has a name for that: the two columns that expand by
// default have no switch, and the one that agrees with this shell's default
// has no `shopt` at all.
//
// It is an override in one direction only, which is measured: the option
// turns expansion on and `echo -E` still turns it off for the one call. So a
// dialect that already expands is unaffected by it, and `-E` is answered
// ahead of it.
func (r *Runner) EchoExpandsEscapes() bool { return r.echoExpandsEscapes }

// SetEchoExpandsEscapes moves it.
func (r *Runner) SetEchoExpandsEscapes(on bool) { r.echoExpandsEscapes = on }

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

// CorrectsCompletionSpelling reports whether completion retries a directory
// that is not there against the closest name that is — bash's `dirspell`,
// which is `cdspell`'s correction asked for by the Tab key instead of by `cd`.
//
// Two switches for one corrector rather than one switch for two names,
// because bash has two names and a session may set either alone: measured,
// `shopt -s cdspell` corrects a typed `cd` and leaves Tab alone, and
// `shopt -s dirspell` the other way about.
func (r *Runner) CorrectsCompletionSpelling() bool { return r.completionCorrectsSpelling }

// SetCorrectsCompletionSpelling moves it.
func (r *Runner) SetCorrectsCompletionSpelling(on bool) { r.completionCorrectsSpelling = on }

// ExpandsCompletedDirectory reports whether a completion writes the directory
// it read back into the line, rather than keeping the directory as it was
// typed — bash's `direxpand`.
//
// For the front end, which is the only half of a shell that has a line to
// write into. The state is here rather than there for the reason
// CompletesEmptyCommandWord's is: it is a `shopt` name a person types between
// two keystrokes, so a completer that read it once at startup would answer
// with a setting the session had already changed.
//
// The two names are a pair, and that is measured rather than assumed — see
// Runner.CorrectedDirectory, which carries the table.
func (r *Runner) ExpandsCompletedDirectory() bool { return r.completionExpandsDirectory }

// SetExpandsCompletedDirectory moves it.
func (r *Runner) SetExpandsCompletedDirectory(on bool) { r.completionExpandsDirectory = on }

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

// KeepsLastPipelineElement reports whether this session has asked for the last
// element of a pipeline to run in the shell itself, so that `echo x | read v`
// leaves `v` set.
//
// A switch over Semantics.LastPipelineElementInCurrentShell rather than a
// second way of spelling it, and the pair is what the panel needs. Measured
// 2026-09-12 with `echo hi | read x; echo "[$x]"`:
//
//	zsh 5.9.2, ksh93   `[hi]` with nothing said, and no option to say it with
//	dash, bash 3.2.57  `[]`, and no option either
//	bash 5.3.15        `[]` until `shopt -s lastpipe`, then `[hi]`
//
// So the axis is where a shell stands and this is whether a script moved it.
// Storing it as a plain bool rather than as a deviation from the axis is the
// measurement too: the only shell with the name starts it off and the axis
// there is already "a subshell", so on is the only direction it travels.
//
// **It is honored only while the monitor is off**, which is not a detail and
// is the half an implementation is most likely to skip. Measured in bash
// 5.3.15 on the same line: `shopt -s lastpipe; set -m; echo hi | read x`
// answers `[]`, and `set -m; set +m` in front of the pipeline answers `[hi]`
// again. That is why the option reads as a no-op in an interactive session,
// where the monitor is on with nothing said, and why the repro in #2361 spells
// `set +m` out. The condition is enforced in runPipeline rather than in the
// setter, because `set -m` may arrive after `shopt -s lastpipe` and has to
// take effect without the option being written again.
//
// Not an axis of its own, and deliberately: an axis records a disagreement
// between shells, and there is nobody to disagree with. One shell in the panel
// has the option at all.
func (r *Runner) KeepsLastPipelineElement() bool { return r.keepsLastPipelineElement }

// SetKeepsLastPipelineElement moves it, for a dialect naming the capability —
// `shopt -s lastpipe` is the only name the panel has for it.
func (r *Runner) SetKeepsLastPipelineElement(on bool) { r.keepsLastPipelineElement = on }

// ExecFailureLeavesTheShellRunning reports whether an `exec` that could not
// happen is an ordinary failed command rather than the end of the script.
//
// Off with nothing said, because ending the script is what every shell in the
// panel does: `exec nosuchcmd; echo REACHED` prints nothing in any of them.
// One shell lets a script ask for the other answer and calls it
// `shopt -s execfail`.
//
// Measured 2026-09-23 on bash 5.3.15, `bash --norc -c` with the option set:
//
//	exec nosuchcmd42; echo "st=$?"    the complaint, then st=127
//	exec ./notexec;   echo "st=$?"    the complaint, then st=126
//	exec ./adir;      echo "st=$?"    the complaint, then st=126
//
// So the *status* is unchanged — it is the status the shell would have exited
// with — and only the ending goes away. The wording is unchanged too, which
// is why this is read where the script stops rather than where it speaks.
//
// The EXIT trap is untouched here, and that is the half an implementation is
// most likely to get wrong. Two axes decide whether a failed `exec` runs the
// trap on its way out; with this on there is no way out, so the trap is
// neither run nor dropped and fires later at the shell's own end. Measured on
// the same binary: `trap "echo TRAP" EXIT; exec nosuchcmd42; echo "after=$?"`
// prints the complaint, `after=127`, then `TRAP`.
//
// The redirection-only form of the builtin never reaches this — `exec 3>f` is
// silent at 0 with the option either way — because that form is not an exec.
//
// Not an axis, for the reason KeepsLastPipelineElement is not: an axis records
// a disagreement between shells, and here there is nobody to disagree with.
// One shell in the panel has the option at all (#4149).
func (r *Runner) ExecFailureLeavesTheShellRunning() bool { return r.execFailureIsSurvivable }

// SetExecFailureLeavesTheShellRunning moves it, for a dialect naming the
// capability — `shopt -s execfail` is the only name the panel has for it.
func (r *Runner) SetExecFailureLeavesTheShellRunning(on bool) { r.execFailureIsSurvivable = on }

// ErrExitEntersACommandSubstitution reports whether the shell a `$(…)` body
// runs in holds `set -e` — `shopt inherit_errexit` under its bash name.
//
// A read of Semantics.ErrExitEntersACommandSubstitution rather than a second
// piece of state, and that is the measurement rather than a convenience. The
// option and the mode are one thing in bash: `set -o posix` leaves `shopt
// inherit_errexit` reporting `on`, and `set +o posix` leaves it on, so a
// separate bit would have had to be kept in step with the mode in both
// directions and would have drifted in the second.
//
// Unanswered reads as off, which is the reading the getter's own name asks
// for — a shell that has never been told is not a shell that inherits — and
// it is unreachable from the one dialect that has the name anyway.
func (r *Runner) ErrExitEntersACommandSubstitution() bool {
	return r.sem().ErrExitEntersACommandSubstitution == Yes
}

// SetErrExitEntersACommandSubstitution moves it, for a dialect naming the
// switch — `shopt -s inherit_errexit` is the only name the panel has for it,
// and it travels both ways: `shopt -u inherit_errexit` in a bash invoked as
// `sh` puts the shell back where plain bash starts.
func (r *Runner) SetErrExitEntersACommandSubstitution(on bool) {
	a := No
	if on {
		a = Yes
	}
	r.swapSemantics(func(s *Semantics) {
		s.ErrExitEntersACommandSubstitution = a
	})
}

// FdVariableDescriptorOutlivesTheCommand reports whether the descriptor a
// `{name}>file` redirection picked is still open once the command carrying it
// has ended — bash's `varredir_close` read the way round the shell behaves
// rather than the way round the option is named.
//
// One shell in the panel names the question and it names the *closing*:
// `shopt -s varredir_close` asks for the descriptor to be taken back. So the
// table inverts and this does not, for the reason CompletesEmptyCommandWord
// gives — there is exactly one place in the program where the sense of the
// bit is decided.
//
// It is a permission and not a behavior: whether such a descriptor outlives
// its command at all is Semantics.FdVariableOutlivesTheCommand, and this can
// only turn that down. A dialect whose axis already closes the descriptor is
// unaffected by the name being set.
func (r *Runner) FdVariableDescriptorOutlivesTheCommand() bool {
	return !r.fdVarClosedWithTheCommand
}

// SetFdVariableDescriptorOutlivesTheCommand moves it, in the positive
// direction the getter reads.
func (r *Runner) SetFdVariableDescriptorOutlivesTheCommand(on bool) {
	r.fdVarClosedWithTheCommand = !on
}
