// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"github.com/blairham/sh/interp"
)

// Semantics is what zsh means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()

	// This shell reads a `#!` line itself rather than leaving it to the
	// kernel, which is two answers and not one.
	//
	// A line naming nothing is refused: measured 2026-09-25, `printf
	// '#!\necho hi\n' > empty; chmod +x empty; empty` is
	// `zsh:1: exec format error: empty` at 126 here and `hi` at 0 in the
	// other six columns — the same for a `#!` followed only by blanks. See
	// TestAnEmptyInterpreterLineIsRefused.
	// `${!name}` — the one construct this shell's grammar has not and a
	// **mode** gives it. `emulate ksh` turns on syntax.Dialect.ParamIndirection
	// (see emulateindirection.go) and what the sigil then means is zsh's own
	// `(k)`: the subscript, not the thing the value names. Answered here
	// rather than in the emulation table because it cannot be reached in any
	// other state — the shell's own mode and `emulate sh` refuse the
	// construct at the parse, so there is no mode for the answer to be wrong
	// in and nothing for an `emulate zsh` to restore. See
	// interp.Semantics.IndirectionIsTheSubscriptFlag, where the twenty rows
	// are.
	s.IndirectionIsTheSubscriptFlag = interp.Yes
	// See Semantics.UnterminatedHeredocLoneBackslash (#6286).
	s.UnterminatedHeredocLoneBackslash = interp.LoneBackslashSpaced

	s.EmptyInterpreterLineIsNotAScript = interp.Yes
	// And a line naming a word with no slash in it is looked up on PATH:
	// `printf '#!cat\necho hi\n' > tstcmd; chmod +x tstcmd; tstcmd` prints
	// the file's own two lines at 0 here, where the three bash columns say
	// `cat: bad interpreter` at 126 and dash, ksh93 and BusyBox ash say
	// `not found` at 127. See TestASlashlessInterpreterIsFoundOnPath.
	s.SlashlessInterpreterIsPathSearched = interp.Yes
	s.CommandNotFoundStatusIsNotFound = interp.No
	// A pathname operand is written back as typed, through `command -v`,
	// `type` and `whence -p` alike: `./bb/tool` is `./bb/tool`. Measured
	// 2026-09-14 against 5.9.2.
	s.APathnameOperandIsReportedAbsolute = interp.No
	// The command that named a `>(cmd)` waits for its body here, which is
	// what makes `printf x | tee >(sleep 3) >/dev/null` take three seconds
	// in this shell and none in bash and ksh93 (#2197).
	s.WritingSubstitutionIsWaitedForAtTheCommand = interp.Yes
	// `<>` onto a process substitution this command made: `print a <> >(cat)` writes into the substitution, and `cat <> <(echo in)` reads from it (#5514).
	s.ReadWriteRedirectionTakesTheSubstitutionsEnd = interp.Yes
	// And it names the path after /proc/self/fd where that directory is
	// there: measured 2026-09-21, `echo <(true)` is `/proc/self/fd/11` in
	// the pinned Linux image and `/dev/fd/11` on the panel machine, while
	// bash and ksh93 write `/dev/fd/N` in both. One image, one /dev/fd
	// symlink, and this shell alone writes the target (#3986).
	s.SubstitutionPathPrefersProcSelfFd = interp.Yes
	s.SetFTurnsOffGlobbing = interp.No
	// A numeric signal goes to `kill(2)` unchecked here, so `kill -99 $$` is
	// `kill <pid> failed: invalid argument` at 1 — the errno, printed —
	// rather than a word refused. Measured 2026-09-16; `kill -s 99` is still
	// `unknown signal: SIG99`, since `-s` takes a name.
	// `kill -n signum` is an option here, and `-s` with nothing after it is
	// an option missing its argument rather than a signal named `s`.
	s.KillReadsTheNumberOption = interp.Yes
	s.KillOptionWithNoArgumentIsASignalName = interp.No
	s.KillSendsASignalNumberItCannotName = interp.Yes
	// And `-s` really does take a name here: a word of digits after it is
	// the name it is not. Measured 2026-09-17 — `kill -s 9 999999` is
	// `unknown signal: SIG9` and the listing hint at 1, where bash, ksh93,
	// dash and BusyBox ash all reach a real send. The number *out of* range
	// is the axis above and this is the same position with one in it, which
	// is where the three that send part company with this column (#3544).
	s.KillNameOptionReadsANumber = interp.No
	// One name older than the table's own: `IOT` is signal 6 here, as it is
	// in ksh93 and as it is not in bash. Measured 2026-09-17 — `kill -l
	// IOT`, `kill -s IOT`, `kill -IOT` and `trap 'x' IOT` are all the ABRT
	// the table carries, and `kill -l 6` is `ABRT`, so this shell reads the
	// older spelling and never writes it (#3536).
	s.SignalNamesTheShellAlsoReads = "IOT=ABRT"
	// Neither editing mode is selected on its own. Measured 2026-09-11 in a
	// session at a real terminal: `[[ -o emacs ]]` and `[[ -o vi ]]` both
	// answer 1 there, which is why this shell's own default for `emacs` is
	// off and its listing says nothing about the name until a script moves
	// one.
	s.InteractiveSelectsEmacs = interp.No
	// And `-B` is not brace expansion here either: measured 2026-09-11,
	// `set -B` turns the terminal bell off and leaves `{a,b}` expanding,
	// while `set +o braceexpand` — the borrowed spelling of `ignorebraces`
	// — is what stops it. The letter stays among the ones this dialect
	// refuses, since the bell is not implemented (#1856).
	s.SetBTurnsOffBraceExpansion = interp.No
	// `set -t` is a letter this shell has and will not move — the same answer
	// it gives the `onecmd` name it borrowed for `singlecommand` — so it is
	// refused rather than acted on. See Diagnostics.UnimplementedOptionLetters,
	// which is where the letter is refused.
	s.SetHasTheTLetter = interp.No
	// What a running script may not move, the command line that started the
	// shell may. Measured 2026-09-10 on zsh 5.9.2 against a three-line
	// script: `zsh -t plain.sh`, `zsh -o singlecommand plain.sh` and
	// `zsh -o onecmd plain.sh` each run the first line and stop, where
	// `set -t` and `setopt singlecommand` inside such a script are
	// `can't change option` at 1 and fatal. So this shell's five "fixed"
	// names are five a script may not change rather than five states it
	// cannot reach (#1730). See setopt.go's singleCommandOption for the one
	// of them that has something to apply.
	s.ImmovableOptionsSetAtInvocation = interp.Yes
	// And the route the option does *not* stop, which is worth declaring now
	// that this shell can have it on: measured, `zsh -t -c $'echo A\necho B'`
	// writes both lines, so a command string is read to its end.
	s.OneCommandStopsACommandString = interp.No
	// `-c` and `-s` together: `-s` names the operands here, so `sh -sc CMD
	// name a` keeps the shell in `$0` and makes both operands parameters.
	// bash and dash let the command string name them instead.
	s.StdinOptionNamesTheOperands = interp.Yes
	// noclobber reaches `>>` here as well as `>`: appending to a file that is
	// not there is `no such file or directory` at 1 rather than a new file,
	// where POSIX puts the option on `>` alone and the other five columns
	// comply. Measured 2026-09-07 with an append to an *existing* file as
	// the control, which all six allow. It is what `>>|` and `>>!` are for
	// (#1247).
	s.NoclobberBlocksAppendCreate = interp.Yes
	// The reading side of the same option: zsh's short spelling of noglob
	// is `-F`, and that capital is what its `$-` reports.
	s.NoglobLetterIsF = interp.No
	// And what this shell does spend `-f` on: the startup files, whose name
	// in its own option namespace is `norcs`. The letter and the name are
	// one state, which is measured from both ends — `set -f` puts `norcs`
	// in a bare `setopt` listing and `f` in `$-`, `set -o norcs` puts the
	// letter there without the letter having been written, and `set +f` in a
	// `zsh -f` shell takes both away again (#1542). See setopt.go's `rcs`
	// entry, which is the same state read the other way up.
	s.SetFLetterOption = zshFLetterOption
	// `set -h` is histignoredups here — a history option — not the command
	// tracking the letter abbreviates in bash and ksh93.
	s.SetHLetterTracksCommands = interp.No
	// And so nothing at this shell's startup spells command tracking, which
	// leaves its default to be said outright: it is **on**. Measured on zsh
	// 5.9.2, 2026-09-25, in `zsh -f -c` — `[[ -o hashcmds ]]` is 0,
	// `${options[hashcmds]}` and `${options[hashall]}` are both `on`, a bare
	// `unsetopt` names the row `nohashcmds`, and `set -o` writes
	// `nohashcmds            off`, which is the spelling and the state a
	// listing gives an option that is on by default.
	//
	// **The shell's own default is the noun, not what it does.** Both shells
	// hash a command they ran, in every state of the option, so a probe that
	// only ran `ls` and read `hash` back agrees under either reading — which
	// is why this was reported off for as long as it was. The pair that
	// discriminates holds the *hashing* fixed and moves the report:
	// `ls >/dev/null; hash` lists `ls` here and in zsh alike while
	// `[[ -o hashcmds ]]` parted, so the disagreement was never the hashing
	// (#4533). The option is not inert either — `unsetopt hashcmds; ls
	// >/dev/null; hash` is empty in both — so what was wrong was the state a
	// fresh shell starts in and nothing else.
	s.CommandTrackingStartsOn = interp.Yes
	// Job control wants the terminal: with none, `set -m` is refused —
	// `can't change option: -m`, at 1, fatally like every `set` failure
	// here. Measured; bash and ksh93 grant the same request silently.
	s.MonitorNeedsATerminal = interp.Yes
	// zsh leaves it off with no terminal, silently. With one it puts `m` in
	// `$-` and announces its jobs while its own `set -o` still lists
	// `monitor off` — zsh disagreeing with itself rather than an answer to
	// this.
	s.InteractiveMonitorNeedsATerminal = interp.Yes
	// The same, and zsh is worth stating rather than assuming: it forks for
	// a `( )` that has a command after it, which is where this was measured.
	s.SubshellRunsOnAfterSignalingTheShell = interp.Yes
	// And with the signal trapped: `trap 'echo T' USR1; (kill -USR1 $$; echo
	// in); echo "after $?"` writes `in`, `T` and `after 0`. Measured
	// 2026-11-01 on zsh 5.9.2 (#7168).
	s.SubshellEndsWhenItSendsTheShellATrappedSignal = interp.No
	// And it announces both ends of a job on that route: measured on
	// `-i script.sh` through a pseudo-terminal, `[1] <pid>` as the job
	// starts and `[1]  + done       sleep 0.3` as it ends.
	s.InteractiveScriptAnnouncesJobs = interp.Yes
	// And on the other route too: measured 2026-09-12 on `-i -c` through a
	// pseudo-terminal, `[1] <pid>` as the job starts and `[1]  + done` after the
	// `wait` that reaps it.
	s.InteractiveCommandStringAnnouncesJobs = interp.Yes
	// Written where the job ended rather than where a prompt is drawn, which
	// the same run says: `-i -c` never draws one and zsh writes the row.
	s.FinishedJobNoticeNeedsAPrompt = interp.No
	// And the moment the job ends rather than before the next prompt, which
	// is `NOTIFY` — on out of the box here and off in the rest of the panel.
	// Measured 2026-09-25 on a pseudo-terminal: `sleep 0.4 &` writes `[1]
	// <pid>` and then `[1]  + done       sleep 0.4` about four tenths of a
	// second later, with nothing typed in between.
	//
	// The one axis here a running script can move, through `unsetopt NOTIFY`
	// — so the entry in setopt.go writes this rather than remembering the
	// request. Until #4524 it was remembered and ignored, and this shell
	// behaved as `nonotify` in every session (#4524).
	s.FinishedJobNoticeArrivesAtOnce = interp.Yes
	// A job started with `&` reads the shell's own standard input here, where
	// the other five hand it an empty one — measured 2026-09-07,
	// `sh -c '/bin/cat & wait; echo ---; /bin/cat' < f` writes the file's line
	// *before* the marker in zsh 5.9.2 and after it in the other five. POSIX
	// XCU 2.9.3 specifies the majority; this is the divergence, and it is
	// this shell's to keep.
	s.BackgroundJobInput = interp.BackgroundJobInputIsTheShells
	// unanswered BackgroundJobInputIsOnlyTheShellsOwn: this shell substitutes
	// nothing at all for a background job's input — the line above is the
	// statement of that — so which streams a substitution reaches has no
	// answer to observe here. Measured 2026-09-23 on 5.9.2: the job reads the
	// shell's own input, an `exec`-installed one, an enclosing loop's
	// redirection and a pipeline alike (#4153).
	// `$!` before any background command is `0` here and nothing in the other
	// five columns — a number nothing ever had. Measured,
	// `sh -c 'echo "[$!]"'` writes `[0]`, and that zero is *set*: `${!-unset}`
	// is `0` and `${!+set}` is `set`, so `set -u; echo "[$!]"; echo "st=$?"`
	// writes `[0]` then `st=0`.
	//
	// It is the quiet side of the `set -u` split with ksh93 and for a
	// different reason — ksh93 has nothing there and does not mind, zsh has a
	// value — and it is worth writing down beside this shell refusing an
	// unset `$1`, which it does.
	s.LastBackgroundPid = interp.LastBackgroundPidZero
	s.ProcessSubstitutionIsTheLastBackgroundJob = interp.No
	// Measured: `echo $-` reports `569X` under -c, a script file and
	// standard input alike — letters from zsh's own single-letter option
	// namespace, which shares almost nothing with the other shells'. The
	// `s` of the standard-input route is added on top of it and comes from
	// Runner.Route; under `-c` zsh shows neither route letter, where bash
	// and ksh93 show `c`.
	s.DefaultOptionLetters = zshStartupLetters
	// And `569XZi` when interactive, so the interactive set is the same four
	// plus the line-editor letter. Measured 2026-09-05 under `-i script.sh`
	// and at a pseudo-terminal alike; `i` comes from the runner.
	s.InteractiveOptionLetters = zshInteractiveStartupLetters
	// The option namespace's own way of saying `-i`, and the shell prompts for
	// it: measured 2026-09-16 with a program on a pipe so nothing else could
	// make it interactive, `zsh -f -o interactive` draws a prompt, runs the
	// line and draws another, and reads `~/.zshrc` exactly as `-i` does.
	// `+o interactive` and `-o nointeractive` are the other direction and draw
	// none, on a pipe and — measured through a pseudo-terminal — on a terminal
	// too, where the shell would otherwise have prompted (#3195).
	s.InteractiveOptionName = "interactive"
	// `-o pathscript` looks the script operand up along PATH, stopping at
	// the first entry of the name that is not a directory. Measured
	// 2026-10-02 on 5.9.2 (#5138).
	// `exec` with options and nothing to run is refused before its
	// redirections. See Semantics.ExecOptionsRequireACommand (#5380).
	s.ExecOptionsRequireACommand = interp.Yes
	// And `builtin`'s name is looked up before anything behind it is
	// matched. See Semantics.BuiltinNameIsLookedUpBeforeGlobbing (#5377).
	s.BuiltinNameIsLookedUpBeforeGlobbing = interp.Yes
	// An exported name past ASCII reaches no child. See
	// Semantics.NameBeyondASCIIStaysOutOfTheEnvironment (#5364).
	s.NameBeyondASCIIStaysOutOfTheEnvironment = interp.Yes
	// A bare `trap` inside the running EXIT trap lists it (it does here,
	// measured 2026-10-02). See Semantics.ExitTrapListedWhileItRuns.
	s.ExitTrapListedWhileItRuns = interp.Yes
	// `ZSH_NAME` is the name the shell was invoked as. See
	// Semantics.InvocationNameParameter (#5336).
	s.InvocationNameParameter = "ZSH_NAME"
	// A trap's action is listed from its parse. See
	// Semantics.TrapActionListedFromItsParse (#5409).
	s.TrapActionListedFromItsParse = interp.Yes
	s.ScriptSearchOptionName = "pathscript"
	// And sh and ksh emulation start with it on (#5383).
	s.ScriptSearchUnderEmulation = "sh ksh"
	s.ScriptSearchTakesTheFirstFile = true
	s.OptionNamesFoldCaseAndUnderscores = true
	s.NonInteractiveOptionName = "nointeractive"
	// zsh has a history expander and starts a prompt with it on — measured
	// 2026-09-15 through a pseudo-terminal with a two-row prompt, where
	// `echo one two three` then `echo !!` echoes the expanded line and runs
	// it, with nothing configured.
	//
	// The letter is not `H` here and must not be: zsh's `set -H` is
	// `rmstarsilent`, measured by diffing `setopt` across it, and this
	// dialect's own letter table answers that before the core's `-H` is
	// reached. The spellings that reach the expander are `setopt banghist`
	// and the borrowed `set -o histexpand`.
	s.HistoryExpansion = interp.Yes
	// An event's words are the shell's own, a `${ }` included. See
	// Semantics.HistoryWords.
	s.HistoryWords = interp.HistoryWordsShellBraces
	// And a `:q` quotes where it is written. See
	// Semantics.HistoryQuoteModifierInPlace.
	s.HistoryQuoteModifierInPlace = interp.Yes
	// A backslash in a substitution's replacement escapes what follows it,
	// and it happens **twice**: `!!:s/o/\\\\X/` written with four
	// backslashes comes back with one. bash copies all four through. See
	// Semantics.HistorySubstitutionUnescapesTheReplacement for the panel and
	// for the mixed-content row that pins it to characters rather than to a
	// division.
	s.HistorySubstitutionUnescapesTheReplacement = interp.Yes
	// And a `:h` or `:t` takes a count of components to keep: `:h2` on
	// `/my/path/for/testing` is `/my` where bash leaves `/my/path/for2`.
	// See Semantics.HistoryHeadAndTailTakeACount.
	s.HistoryHeadAndTailTakeACount = interp.Yes
	// And `:P` makes the word an absolute, resolved path, which bash has no
	// modifier for. See Semantics.HistoryAbsolutePathModifier.
	s.HistoryAbsolutePathModifier = interp.Yes
	// A quote or a backquote against the event character is ordinary text
	// here and part of the event's name in bash and ksh93, a second event
	// character closes the name it is in, and `!{…}` is the braced form
	// neither of the others has. Measured 2026-09-18 at a prompt: `echo
	// "T!'x E"` prints its own text, `X!ab!cdY` is `event not found: ab!`,
	// and `!{x}` is the event `x`. See
	// Semantics.HistoryQuoteEndsAnEventReference,
	// Semantics.HistoryEventCharClosesAnEventName and
	// Semantics.HistoryBracedEventReference.
	s.HistoryQuoteEndsAnEventReference = interp.Yes
	s.HistoryEventCharClosesAnEventName = interp.Yes
	s.HistoryBracedEventReference = interp.Yes
	// And a `$` does not end a word designator: `!!:$-3` is a range this
	// shell cannot make rather than the last word with `-3` written after
	// it, which is `no such word in event`. Measured the same day over
	// `echo a b c d e`. See Semantics.HistoryLastWordEndsTheDesignator.
	s.HistoryLastWordEndsTheDesignator = interp.No
	// No keyword option. The letter is not the option here: zsh spells
	// `interactivecomments` with `-k` and answers `no such option` to `set
	// -o keyword`, both measured 2026-09-16, so a `name=value` word after
	// the command name stays a positional and that is zsh's answer rather
	// than a gap. See Semantics.KeywordAssignments.
	s.KeywordAssignments = interp.No
	// The container letter `export` takes here is the indexed one alone — `-A`
	// is `bad option: -A` — and it takes effect without a value: `export -a b`
	// is an empty exported array, `typeset -ax b=(  )`, and over a standing
	// scalar it converts it to the same. Measured 2026-10-15 on zsh 5.9.2
	// (#6452), which corrects the 2026-09-23 note here that said `export`
	// took no container letter at all. See
	// interp.Semantics.ExportContainerLetterNeedsAValue.
	s.ExportContainerLetterNeedsAValue = interp.No
	// Restricted mode, `setopt restricted` or `set -r`. Measured 2026-10-02
	// on zsh 5.9.2 under `-f`, from `-c` and from script files (#5155):
	// `set +r` and `unsetopt restricted` are refused, so the mode is never
	// left; the frozen names are not readonly names — `readonly -p` lists
	// nothing and the refusal is `PATH: restricted`; `exec echo hi` ends the
	// script at 1; `. ./x` reads the file; `command -p echo hi` is `echo:
	// restricted` at 1 and the line after it runs.
	s.RestrictedModeIsLeftByTheLetter = interp.No
	s.RestrictedFreezeIsAReadonly = interp.No
	// And any declaration of a frozen name that would make a binding or name
	// a letter is refused as well, with no value behind it: measured
	// 2026-10-02, `f(){ local PATH }; f` is `f:local: PATH: restricted` and
	// `export PATH` is `zsh:export:1: PATH: restricted`, both fatal, where
	// `typeset -g PATH` inside a function and a bare `typeset PATH` listing
	// are taken (#5485).
	s.RestrictedFreezeRefusesAValuelessDeclaration = interp.Yes
	// unanswered RestrictedFreezeRefusesALetterThatChangesTheName: every letter
	// is refused by the axis above, so the narrower question is never asked.
	// And a writing redirection whose target is one `>(cmd)` is taken in the
	// mode: `print a > >(cat)` prints `a` at 0 where `>> >(cat)` and a target
	// that only holds the path are refused (#5485).
	s.RestrictedRedirectTakesAProcessSubstitution = interp.Yes
	// And the script that refusal ends exits with the builtin's own status,
	// which the refusal does not fail: `export PATH` is 0 and `x=1; export
	// PATH x` is 1. Measured 2026-10-02 (#5515).
	s.RestrictedRefusalEndsWithTheBuiltinsStatus = interp.Yes
	s.RestrictedBuiltinRefusalIsFatal = interp.Yes
	s.RestrictedDotRefusesAPath = interp.No
	s.RestrictedCommandOptionRefusalIsFatal = interp.No
	// unanswered KeywordPromotesADeclarationsOperand: there is no keyword
	// option here to reach a declaration with. `-k` is this shell's
	// `interactivecomments` and `set -o keyword` is `no such option`, both
	// measured 2026-09-16, so the question cannot be put to it.

	// The seven declaration commands are reserved words here, and the
	// assignment rule for their operands belongs to the word as written:
	// under `setopt shwordsplit`, `$cmd a=$b`, `\typeset a=$b`, `'typeset'
	// a=$b`, `"typeset" a=$b`, `type'set' a=$b`, `noglob typeset a=$b` and
	// `builtin typeset a=$b` all split the value, where `typeset a=$b`, an
	// alias for it and `nocorrect typeset a=$b` keep it whole. Measured
	// 2026-09-16 on zsh 5.9.2, for typeset, declare, export, local, readonly,
	// float and integer. See #3315.
	s.DeclarationCommandWord = interp.DeclarationByUnquotedLiteralWord
	// And `command` in front of one does not keep the rule. The question can
	// only be put to this shell under `setopt posixbuiltins`, since `command
	// typeset` is `command not found` without it: `setopt shwordsplit
	// posixbuiltins; command typeset a=$b` is `[x]` there, and so is
	// `command -p typeset a=$b`. Measured 2026-09-16 on zsh 5.9.2. See #3341.
	s.CommandPrefixKeepsADeclaration = interp.No

	s.HistoryExpansionAtAPrompt = interp.Yes
	// The startup files, and zsh has more of them than the rest of the panel
	// put together. Measured 2026-09-05 through a pseudo-terminal with a
	// scratch home directory: `zsh -l -i` reads `.zshenv`, `.zprofile`,
	// `.zshrc` and `.zlogin`, in that order, and `zsh -c cmd` reads
	// `.zshenv` alone — the only startup file any shell in the panel reads
	// for a plain command string.
	s.StartupDirectoryVariable = "ZDOTDIR"
	s.UnconditionalStartupFile = ".zshenv"
	s.LoginStartupFiles = ".zprofile"
	// After the run-commands file rather than before it, so a person's
	// `.zlogin` sees what their `.zshrc` did. Read for a non-interactive
	// login shell too, which zsh is one of the three to do at all.
	s.LateLoginStartupFile = ".zlogin"
	s.InteractiveStartupFile = ".zshrc"
	// And the system-wide file in front of each of those, which this shell
	// is the one in the panel with enough slots to pin the *position* of.
	// Measured 2026-09-12 with `setopt sourcetrace`, which names each file
	// as it is read: `zsh -o sourcetrace -l -i` writes `~/.zshenv`,
	// `/etc/zprofile`, `~/.zprofile`, `/etc/zshrc`, `~/.zshrc`, `~/.zlogin`.
	// So each system file comes first in its own slot rather than all of
	// them coming before all of the person's — a shell that hoisted them
	// would read `/etc/zshrc` before `~/.zprofile`, and `~/.zprofile` is
	// where a person sets `$PATH`.
	//
	// `zshenv` and `zlogin` are the manual's answer rather than a measured
	// one, and the difference is worth stating: neither `/etc/zshenv` nor
	// `/etc/zlogin` exists on the machine this was measured on, so no probe
	// can see them read. The slot each occupies *is* measured, because the
	// two files that do exist each land first in theirs.
	//
	// Both have since been *seen* read, on the other platform, which is the
	// half of the blind spot the sentence above could not close: a Debian
	// zsh keeps all four in `/etc/zsh` and traces `/etc/zsh/zshenv` on every
	// invocation. Names and not paths, so this is unchanged by that — see
	// [SystemStartupDirectory], which is where the directory differs (#3987).
	s.SystemStartupFiles = interp.SystemStartupFiles{
		Unconditional: "zshenv",
		Login:         "zprofile",
		Interactive:   "zshrc",
		LateLogin:     "zlogin",
		Logout:        "zlogout",
	}
	// The logout file, read when an interactive login shell ends however it
	// ends: measured, `zsh -l -i` reads `~/.zlogout` after a typed `exit`
	// and at the end of its input, and `zsh -l -c exit` reads nothing — see
	// the two fields for the grid (#5996).
	s.LogoutFile = ".zlogout"
	s.LogoutFileReadWhen = interp.LogoutWhenAnInteractiveLoginEnds
	// And the two options asked before each file rather than once at the
	// start: `unsetopt rcs` or `unsetopt globalrcs` in `~/.zshenv` stops the
	// files after it, and `setopt globalrcs` there undoes a `-d` (#5905).
	s.StartupFilesOptionName = "rcs"
	s.SystemStartupFilesOptionName = "globalrcs"
	// And zsh reads it for a login shell as well as a plain one, where bash
	// reads only its profile.
	s.InteractiveStartupFileWhenLogin = interp.Yes
	// One escape hatch, spelled two ways, and it means *every* file: `zsh -f
	// -l -i` reads no `.zshenv`, no `.zprofile`, no `.zshrc` and no
	// `.zlogin`. It is not the POSIX `-f` — measured, `zsh -f -c 'echo
	// /etc/pas*'` still expands the pattern, so the letter is spent on this
	// here and on globbing everywhere else.
	s.StartupFileOptions = interp.StartupFileOptions{
		// The two emulations that re-point this shell's option letters at
		// sh's, where `-f` is globbing and `-d` is no letter at all. The
		// long spellings are unaffected and `-l` is login in both sets — see
		// the field, which carries the measured table (#4518).
		LettersBorrowedUnderEmulation: "sh ksh",
		// Both spellings of login-ness, which zsh has like the other three.
		Login:       "-l --login",
		SuppressAll: "-f --no-rcs",
		// And one that drops root's files and keeps the person's, which no
		// other shell in the panel has. Measured 2026-09-12: `zsh -d -l -c`
		// still reads `.zshenv`, `.zprofile` and `.zlogin`, and every one of
		// them reports the inherited `${PATH%%:*}` rather than
		// `path_helper`'s, so `/etc/zprofile` did not run.
		SuppressSystem: "-d --no-globalrcs",
	}
	// What `zsh --version` writes, on standard output at status 0 — measured
	// 2026-09-11, one line and no more.
	s.VersionOption = interp.VersionOption{Spellings: "--version", Text: versionLine()}
	// And `--help`, which this shell answers with a usage block on standard
	// output at 0 — measured 2026-09-18 on zsh 5.9.2, and `no such option:
	// help` at 1 here until now, since the front end hands any unmatched
	// `--word` to the option table and this is not an option name (#3156).
	//
	// The whole answer is the **trailer**, which is unusual and is forced by
	// the measurement: Diagnostics.InvocationUsage is the block a *refused*
	// option gets as well as the one `--help` gets, and this shell prints no
	// block at all under a refusal — `zsh -o badname -c :` and `zsh --badname
	// -c :` are each one line, `no such option: badname`, with nothing after
	// them. So a block hung there would start appearing where the reference
	// prints none. The trailer is the one piece of the answer that is this
	// option's alone, and it is the piece that reads the shell's own name.
	//
	// Generated from this dialect's three option tables rather than pasted —
	// see helpBlock, where the measurement that settles that choice is. The
	// generated text is committed as helpBlockText, so a start does not build
	// it from the tables: that was 14 page faults and ~0.04 ms of every start
	// on macOS, for a block only `--help` reads (#6491).
	s.HelpOption = interp.HelpOption{Spellings: "--help", Trailer: helpBlockText}
	// And `--emulate MODE`, which is the `emulate` builtin run before a line
	// is read rather than a spelling of an option name. zsh alone on the
	// panel; measured 2026-09-18 on 5.9.2 under `env -i PATH=/usr/bin:/bin
	// LC_ALL=C`, and the rows are in interp.EmulationOption.
	//
	// Both refusals are the shell's own words at status 1, where a word this
	// front end cannot place exits 2 — `--emulate` with nothing after it is
	// `--emulate: argument required`, and `-x --emulate sh` is `--emulate:
	// must precede other options`. The mode itself is never judged here: the
	// builtin reads the word by its first letter, the same way it reads
	// `emulate fish` at a prompt (#5254).
	//
	// And the *name*, which asks the same question without an option word:
	// the first letter of argv[0]'s basename picks the mode this shell starts
	// in. Measured 2026-09-26 on 5.9.2 by copying the binary under each name
	// — `s`, `sx`, `shell`, `shx`, `b`, `bash` and `bsh` all report `sh`, `k`
	// and `ksh` report `ksh`, `c` and `csh` report `csh`, and `xsh`, `mysh`,
	// `ash`, `dash`, `fish` and `SH` report `zsh`. A leading dash is stripped
	// first, so the login spelling `-sh` is the name too, and then one
	// leading `r`: `rsh` is sh emulation, `rzsh` and `rr` are not. The rows
	// are in interp.EmulationOption.NameInitials.
	s.EmulationOption = interp.EmulationOption{
		Spellings:        "--emulate",
		Builtin:          "emulate",
		MissingArgument:  "%s: argument required",
		OutOfOrder:       "%s: must precede other options",
		Status:           1,
		NameInitials:     "s=sh b=sh k=ksh c=csh",
		NameDropsInitial: "r",
	}
	// `-b` ends the option reading at the end of the word it is written in,
	// so every word after that one is an operand — which is why `zsh -b -c
	// cmd` is a failure to open a file called `-c` rather than a command
	// that ran. The panel's only such letter; five of the other six spend
	// `b` on the option that reports a finished background job at once, and
	// this shell refuses it at `set` while reading it at invocation, which
	// is what says it is the invocation's alone. Measured 2026-09-19; the
	// rows are in interp.Semantics.EndOfOptionsInvocationLetter.
	s.EndOfOptionsInvocationLetter = "b"
	// The parameter an `autoload`d name is looked up on, and the panel's only
	// one: `autoload -Uz is-at-least` finds its file on `$FPATH` and nothing
	// else does. Measured under `-f`, so it is the shell's own value and not a
	// startup file's — which is also what says the value survives the escape
	// hatch above. Named here and filled in by the front end, because which
	// directories is a fact about where a shell was installed; see
	// interp.Semantics.FunctionSearchVariable.
	s.FunctionSearchVariable = "FPATH"
	// `+i` takes the prompt back, the letter's half of the name above.
	// Measured 2026-09-16 on zsh 5.9.2 with the program on a pipe: `zsh -i +i
	// -c 'echo $-'` reports `569X`, which is what a shell started with no
	// letter at all reports, and `zsh -i -c` reports it with `i` in front.
	s.PlusSignedInteractiveLetterStillPrompts = interp.No
	s.CommandStringShowsCInDollarDash = interp.No
	s.LoginShowsLInDollarDash = interp.Yes
	s.CommandStringShowsSInDollarDash = interp.No
	// And the order, which is the simplest in the panel and the only sorted
	// one: the whole string by byte, so the digits lead, the capitals follow
	// and the lowercase letters come last. Measured 2026-09-12 on zsh 5.9.2,
	// the interactive row through a pseudo-terminal:
	//
	//	set -f; set -u; set -e   569Xefu
	//	set -C                   569CX
	//	set -C -e                569CXe
	//	set -o noglob            569FX
	//	set -e -C, on stdin      569CXes
	//	-l -c                    569Xl
	//	-i -c, at a terminal     569XZim
	//
	// The `-C` rows are what make it a sort rather than an append: the
	// letter lands in front of a startup letter this shell already held.
	//
	// The string spells the sort out for **every** letter this shell can
	// show rather than only the handful the rows above name, and that is a
	// correction rather than padding: the thirty letters #2578 added were
	// refused when it was written, so a string naming only the letters that
	// could then be produced left each new one to "keeps its produced place
	// and follows the ones it does" — which put `D` after `X` where zsh puts
	// it before. Measured on the same binary, one run rather than thirty:
	//
	//	set -T -Q -D -u -e -w -y -h -p -G -M   569DGMQTXehpuwy
	//	set -Y -B -a -C -F                     569BCFXYa
	//
	// Both are the whole string by byte with nothing else to explain, which
	// is why writing the byte order out in full is the same claim the rows
	// make and not a wider one.
	//
	// **The digits were `569` and are now all ten** (#5057). The same
	// correction as the paragraph above, and found the same way: the ten
	// digit letters were refused when this was written, so a string naming
	// only the three this shell starts with left the other seven to keep
	// their produced place — and they came out *after* the letters, where
	// zsh puts every digit in front. Measured 2026-09-28 on one binary:
	//
	//	set -8 -0 -3 -T -y -1            0135689TXfy
	//	set -9 +9 -7 -2 -B -w -4         24567BXfw
	//	set -0 -1 -2 -3 -4 -7 -8         0123456789Xf
	//
	// The last row is the one that settles it: all ten digits, in order,
	// ahead of `X` and `f`. It is plain ASCII — digits, then upper, then
	// lower — and the second row also shows a letter taken back out (`+9`)
	// leaving no gap behind it.
	s.DollarDashLetterOrder = "0123456789" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz"
	// The panel's holdout: `echo hi >&-` is status 0 here and 1 in the other
	// three — the text is quietly lost and nothing is said about a simple
	// command's own closed stream. What zsh prints when the stream was
	// closed by `exec >&-` instead — a `write error` with the status still 0
	// — is measured in docs/spec/semantics.md and not reproduced.
	//
	// EBADF is the odd errno here and not EFBIG: a write past `ulimit -f`
	// with XFSZ ignored or handled, or onto a full disk, fails `echo`,
	// `print` and `printf` at 1 and says `zsh:printf:1: write error: file
	// too large`, as the rest of the panel does. The axis is asked for EBADF
	// alone; see it (#7172).
	s.BuiltinWriteErrorFailsTheCommand = interp.No
	// And whichever errno fails the command, only the three builtins whose
	// output is the point of running them fail: `set`, `typeset -p`, `alias`
	// and `export` past `ulimit -f` leave 0 and say nothing. Measured
	// 2026-11-01, zsh 5.9.2; see Semantics.BuiltinWriteErrorFailsOnlyThese.
	s.BuiltinWriteErrorFailsOnlyThese = "echo print printf"
	// `set -a` marks an array or a table as surely as a scalar, whoever
	// stored it: `set -a; A=(1); typeset -p A` is `typeset -ax A=( 1 )`.
	// Measured 2026-10-03; see the axis.
	s.AllexportMarksACompound = interp.Yes
	// But not OPTARG and OPTIND as `getopts` writes them, where the name it
	// was given is marked: `set -a; getopts x: o -x6` hands a child `o=x` and
	// nothing else. Measured 2026-10-15; see the axis.
	s.AllexportMarksGetoptsOwnParameters = interp.No
	// And where OPTARG or OPTIND is already exported, a child goes on being
	// handed the value it had before `getopts` wrote it: `export OPTIND=1;
	// getopts x: o -x6` hands a child `OPTIND=1` — and so does a function
	// call's reset and restore of it: `export OPTIND=3; f() { printenv
	// OPTIND; }; f` hands the child `3`. Measured 2026-10-15; see the axis.
	s.GetoptsLeavesTheEnvironmentCopyOfItsOwnParameters = interp.Yes
	// And the opposite answer for the other errno, which is the half of the
	// pair this shell reverses. A builtin writing into a pipe nobody is
	// reading, with SIGPIPE disarmed, reports 1 here -- where `echo hi >&-`
	// reports 0 -- and says so twice: `zsh:echo:N: write error: broken pipe`
	// from the builtin, then `zsh:N: write error: broken pipe` from the
	// stream. ksh93 is the mirror image of both rows (#770).
	s.BrokenPipeWriteErrorFailsTheCommand = interp.Yes
	s.ArithIntegerOperatorRefusesFloat = interp.No
	// And a compound assignment hands back what it computed: `typeset -i n=1`
	// makes `$(( n += 0.5 ))` 1.5 here, where the plain `$(( n = n + 0.5 ))`
	// is 1 and `n` is 1 either way. Measured 2026-09-26 on zsh 5.9.2; ksh93u+
	// converts both (#4606).
	s.CompoundArithAssignmentConvertsItsValue = interp.No
	// And a change of float letter keeps the number, exactly as a change of
	// precision does: measured 2026-09-26, `typeset -F1 f=3.14159265358979;
	// typeset -E10 f` reads `3.141592654e+00` here where ksh93 reads `3.1`
	// (#4486).
	s.FloatLetterChangeRereadsTheRendering = interp.No
	// A numeral a double cannot hold saturates: `$((1e400))` is `Inf` and
	// `$((-1e400))` is `-Inf`, the same answers the arithmetic gives for a
	// value that overflowed while being computed.
	s.ArithFloatOverflowIsZero = interp.No
	// An expression reading a missing key leaves the table as it was.
	s.ArithKeyReadCreatesTheElement = interp.No
	// unanswered ArithSubscriptDollarSingleIsDecoded: no `$'…'` in an arithmetic
	// subscript reaches a reading that takes its quotes off here. See the field.
	// See interp.Semantics.ArithStoreIntoAFrozenTableIsTaken (#6826).
	s.ArithStoreIntoAFrozenTableIsTaken = interp.Yes
	// No declared-only array to take out of the set — `typeset -a a` lists
	// `a=(  )` already — and a refused store under a table's key leaves the
	// table as it was. See interp.Semantics.ArithStoreRefusedByAFreezeSetsTheArray
	// and interp.Semantics.RefusedTableStoreKeepsTheKey (#6847).
	s.ArithStoreRefusedByAFreezeSetsTheArray = interp.No
	s.RefusedTableStoreKeepsTheKey = interp.No
	// `${a[k]=x}` on a missing key assigns it and `${a[k]?m}` refuses it.
	// See interp.Semantics.AssignOrErrorOperatorDeclaresTheKey (#7187).
	s.AssignOrErrorOperatorDeclaresTheKey = interp.No
	// The splice `b[1]=(p)` over a frozen `b` is `read-only variable: b`.
	// See interp.Semantics.FrozenContainerTakesAnElementLiteral (#6833).
	s.FrozenContainerTakesAnElementLiteral = interp.No
	// A negative exponent is a float answer here, not a refusal: `2**-1`
	// is 0.5.
	s.ArithNegativeExponentIsError = interp.No
	s.ArrayScalarIsTheWholeArray = interp.Yes
	// Off in a shell that has not asked for it: `x${a}y` on `a=(1 2)` is
	// the one word `x1 2y` here, and the two words `x1y x2y` only under
	// `setopt RC_EXPAND_PARAM`, which is the one option in the panel that
	// moves this. The setopt.go entry writes the axis rather than
	// remembering the request (#4549), so `(setopt rcexpandparam)` stays in
	// the subshell. The per-expansion `${^a}` spelling does not read this
	// field at all — its parity wins — which is what keeps the flag and the
	// option one mechanism.
	s.ParamExpansionDistributesOverTheWord = interp.No
	// GLOB_ASSIGN is **off**, so a plain assignment's right-hand side is
	// text: `a=*.txt` stores the six characters here as it does in every
	// other column. The setopt.go entry writes this axis rather than
	// remembering the request (#4638), so `(setopt globassign)` stays in the
	// subshell and `emulate -R` puts it back with the rest of the vector.
	//
	// The option is what the *assignment* consults and not what makes a word
	// a pattern: `a=(*.txt)` globs in either state, because an array
	// literal's elements are ordinary words.
	s.ScalarAssignmentValueIsGlobbed = interp.No
	// A traced array literal shows what its elements came to: `x="p q";
	// a=("$x" r)` is `a=( 'p q' r )` here. The unquoted `a=($x)` is
	// `a=( 'p q' )` — one element, because nothing here splits an unquoted
	// parameter — which is this column's own expansion in its own trace.
	s.TraceArrayLiteralShowsTheExpandedElements = interp.Yes
	// And the one line an assignment list gets is written a word at a time, so
	// a warning or a refusal from a store lands inside it: `readonly r; set
	// -x; a=1 r=2` is `+zsh:1> a=1 r=2 zsh:1: read-only variable: r` and then
	// the line's own newline. Measured 2026-10-02 (#5485).
	s.TraceAssignmentListIsWrittenAsItGoes = interp.Yes
	// And a traced `[[ … ]]` pattern is expanded once for the line and again
	// for the match. See interp.Semantics.TracedConditionPatternIsExpandedTwice
	// (#6711).
	s.TracedConditionPatternIsExpandedTwice = interp.Yes
	// One line for a subscripted literal as much as for a plain one:
	// measured 2026-09-17, `typeset -A m; set -x; m=([k]=v [j]=w)` is a
	// single `m=( … )` line in zsh 5.9.2, with the shell's own separator
	// byte where the brackets were.
	s.TraceSubscriptedArrayLiteralIsElementAssignments = interp.No
	// The subscript is not resolved, though: `i=2; a[$i]=v` is `a[$i]=v`
	// here where ksh93 writes `a[2]=v`. That pair is why the two are
	// separate axes.
	s.TraceElementSubscriptIsEvaluated = interp.No
	// An element write expands its value before its subscript, as bash
	// does: `a[$(echo SUB >&2; echo 1)]=$(echo VAL >&2)x` writes VAL, then
	// SUB.
	s.ElementSubscriptIsExpandedBeforeTheValue = interp.No
	// A frozen name is refused once the value is expanded: an element write
	// never expands its subscript, and a whole literal expands every word
	// before the refusal. The only column that does either.
	s.FrozenElementIsRefusedBeforeItsSubscript = interp.Yes
	s.FrozenLiteralIsExpandedBeforeItIsRefused = interp.Yes
	// A literal that fails partway through stores nothing, subscripted or
	// not, as in bash.
	s.FailedSubscriptedLiteralKeepsWhatItStored = interp.No
	// See interp.Semantics.TableLiteralStoresEachElementAsItIsExpanded (#6770).
	s.TableLiteralStoresEachElementAsItIsExpanded = interp.No
	// See interp.Semantics.DeclarationOperandsAreStoredInTurn (#6770):
	// `x=0; typeset x=1 y=$x` leaves `y` holding 0.
	s.DeclarationOperandsAreStoredInTurn = interp.No
	// Nor an array-literal operand: `a=(0); typeset -a a=(1) b=($a)` leaves
	// `b` holding 0 (#6798). See
	// interp.Semantics.DeclarationArrayOperandsAreStoredInTurn.
	s.DeclarationArrayOperandsAreStoredInTurn = interp.No
	// A table append that fails keeps nothing, and a declaration's letter
	// does not re-create a name whose literal failed.
	s.FailedTableAppendKeepsThePairsItAdded = interp.No
	s.FailedOperandLeavesTheLettersKind = interp.No
	// Nor does a bare one make a name that was not an array one: `b=s;
	// b=(1 $((1/0)))` leaves `typeset b=s` and an unset name unset (#7078).
	s.FailedBareLiteralStillMakesTheNameAnArray = interp.No
	// Asked only under `ksharrays`, which is what makes a bare name one
	// element here: a table is read as an ordered list, so the element is the
	// first value in the order this shell lists rather than the one keyed `0`
	// — measured, `h=(a 1 0 x); $h` is `1` and not `x`.
	//
	// *Which* value that is depends on the order, and the order is not
	// reproducible: measured 2026-09-11, zsh lists a table in the order its
	// hash puts the keys in, so `m[z]=1; m[a]=2; m[m]=3` lists `z m a`
	// whichever order the three were assigned in. This shell lists by key,
	// which is ksh93's order exactly. So this answer is the right end of the
	// order to read, and the order underneath it is ours — see
	// docs/spec/semantics.md under KeyedTableOrder (#1758).
	s.KeyedTableScalarIsTheFirstValue = interp.Yes
	s.ArrayNameWithoutSubscriptIsTheList = interp.Yes
	s.AssignmentUpdatesPipelineStatus = interp.No
	// But an array literal counts: `true|false; arr=(1 2)` leaves 0 (#6088).
	// See Semantics.ArrayAssignmentUpdatesPipelineStatus.
	s.ArrayAssignmentUpdatesPipelineStatus = interp.Yes
	s.TestAndArithmeticUpdatePipelineStatus = interp.No
	// And where bash writes the record after negating one of those two, this
	// shell writes what the construct itself reported: `false | true;
	// ! [[ a = a ]]` leaves 0 here and 1 there. Measured 2026-09-11.
	s.NegatedTestRecordsThePostNegationStatus = interp.No
	// And where the other six columns let a `!` alone once `set -n` has
	// stopped anything running, this shell inverts the status it exits with
	// anyway: `zsh -n` over a file holding `! true` writes nothing on either
	// stream and exits 1, on 5.9 and 5.9.2 alike, where bash, ksh93, dash and
	// BusyBox ash all exit 0. Measured 2026-09-16 (#3179).
	s.UnrunNegationInvertsTheStatus = interp.Yes
	// And where the other six columns read nothing of a command `set -n`
	// will not run, this shell reads its words far enough to raise the
	// refusals a word's own reading makes: `zsh -n -c 'echo =nosuchcmd'`
	// writes `nosuchcmd not found` and exits 1. Measured 2026-09-19, with
	// the controls that say it is not an expansion — a command substitution
	// in the same position creates no file. See interp/noexecwords.go
	// (#3823).
	s.UnrunSimpleCommandReadsItsWords = interp.Yes
	// And whether a *compound* writes the record at all is decided by what
	// its body holds rather than by what ran: `if [[ a = b ]]; then :; fi`
	// replaces the record and `if [[ a = b ]]; then [[ b = b ]]; fi` leaves
	// it, where neither body runs. Measured 2026-09-11 (#1931).
	s.CompoundPipelineStatusRecord = interp.CompoundPipelineStatusFromTheBody
	// A construct this shell has refused still asks for another line at a
	// prompt, where the other three refuse it before the next line is read.
	// Measured 2026-09-11 with `printf 'echo one\nif; then\necho three\n'`
	// into `-i`: this shell draws PS2 twice and swallows the `echo three`,
	// which is what makes it worth an answer rather than a bug (#1893).
	s.PromptAsksAgainAfterARefusedToken = true
	// And a `#` typed at this shell's prompt is an ordinary character until
	// `interactivecomments` is set, where the other three open a comment with
	// it — bash through a `shopt` of the same name that is on by default, and
	// dash and ksh93 with no option in the matter at all. Measured 2026-09-12,
	// `printf 'echo a #b\n'` into `-f -i` on a pipe: `a #b` here and `a`
	// everywhere else.
	//
	// The prompt alone: this shell's `-c`, its `eval` and its `.` all read
	// the `#` as a comment with the option off, which is why this is the
	// front end's question and not a preset for [syntax.Dialect.Comments].
	s.PromptCommentsNeedTheOption = "interactivecomments"
	s.UnsetEndsTheProducedPipelineStatus = interp.Yes
	// `unset RANDOM; RANDOM=9; a=$RANDOM; b=$RANDOM` is two fresh numbers
	// here — measured 2026-09-12, 20191 and 4730 — where bash, ksh93, dash
	// and BusyBox ash all answer 9 twice: the assignment is a message to the
	// producer whether or not `unset` has been past (#2450). The 9 is the
	// discriminator, being one digit against the generator's five.
	s.AssignmentRestoresAnUnsetProducedParameter = interp.Yes
	s.SelectLayout = interp.SelectMenuColumns
	s.SelectPromptNeedsTerminal = interp.No
	s.AliasParsesOptions = interp.Yes
	s.AliasHasPrintOption = interp.No
	// And no `-x` either: `alias: bad option: -x`, at 1.
	s.AliasHasExportOption = interp.No
	// The two kinds nothing else in the panel has: a global alias expands
	// wherever a word stands, and a suffix alias is a second namespace
	// keyed on a command word's extension (#2081).
	s.GlobalAliases = interp.Yes
	s.SuffixAliases = interp.Yes
	s.AliasListsAsDefinitions = interp.Yes
	s.AliasRestrictsToRegularKind = interp.Yes
	s.AliasOperandsCanBePatterns = interp.Yes
	s.AliasPlusPrintsNamesOnly = interp.Yes
	s.TypeNamesAnAliasOnlyWhenExpanded = interp.No
	// The reverse of ksh93: silent about `alias nope` and not about
	// `unalias nope`.
	s.AliasReportsNotFound = interp.No
	s.UnaliasReportsNotFound = interp.Yes
	s.AliasNotFoundStatusCounts = interp.No
	// A removed alias leaves nothing behind: `unalias h` twice is 0 then
	// 1 here.
	s.AliasRemembersTheNamesItNames = interp.No
	s.AliasOptionEndsTheLookup = interp.No
	// There is no `-p` here for an operand to stand behind: `alias -p zz` is
	// `alias: bad option: -p` at 1, with or without a `--` after it.
	// Measured 2026-09-19 (#3701).
	s.AliasPrintOptionIgnoresItsOperands = interp.No
	// The two builtins that keep an assignment written in front of them here
	// without being special ones — `V=1 alias` and `V=1 hash` leave `V` set,
	// where `V=1 :` and `V=1 shift 0` do not. See
	// interp.Semantics.BuiltinsKeepingAnAssignmentPrefix for the rows and for
	// why this is not the specialness axis (#3313).
	s.BuiltinsKeepingAnAssignmentPrefix = prefixKeepingBuiltins
	s.UnaliasAllRefusesOperands = interp.Yes
	s.AliasQuoting = interp.ListingQuoteWhenNeededRuns
	s.AliasListingQuotesTheName = interp.Yes
	s.TrapQuoting = interp.ListingQuoteWhenNeededEscaped
	// `typeset -p` writes `typeset v=1`, an exported scalar as `export e=E`
	// — values in the alias style, keys in the trap one.
	s.DeclareListing = interp.DeclareListingExportSpelled
	// A listing with no operands writes the produced parameters with their
	// readings. Measured 2026-09-13, zsh 5.9.2, `env -i PATH=/usr/bin:/bin`
	// with a scratch HOME, over a script file: `typeset -i10 RANDOM=11798`
	// and `typeset -i10 SECONDS=0`, with no reference to either first, and no
	// `LINENO` row — that one is interp.ProducedDeclaration.Silent and is
	// answered per parameter rather than here.
	//
	// It re-reads the producer, which zsh does too: two listings a line
	// apart hold `RANDOM=22537` and then `RANDOM=21677`, so a listing here
	// differs from itself exactly as ksh93's does. Measure that with a
	// redirection — `typeset -p | grep` forks the listing and two forks off
	// one state draw the same number, which reads as a freeze that is not
	// there (#2518).
	s.ProducedParameterListing = interp.ProducedListingWithValue
	s.DeclareValueQuoting = interp.ListingQuoteWhenNeededRuns
	s.ListingControlEscape = interp.ControlEscapeCaret
	// A byte above ASCII is written as itself — `v=é`, the key `[é]`, and
	// `$'a\téb'` where a control byte opened the form — and `^` and `=` are
	// not: `'^'`, `'a^b'`, `'a=b'`, `'x=y=z'`. Measured 2026-09-14 over
	// `set`, a keyed `typeset -p` and an alias listing. This engine left `^`
	// bare and quoted the non-ASCII byte, which is ksh93's answer to both
	// and this shell's to neither (#2820).
	// `!` is bare here and in ksh93 and quoted in bash, which is a third
	// pairing again: `v=!`, `v=a!b` and the key `[!]` all unquoted.
	s.ListedBangIsOrdinary = interp.Yes
	s.ListedCaretIsOrdinary = interp.No
	s.ListedEqualsIsOrdinary = interp.No
	// And a `~` is quoted wherever it stands, which is bash's rule refused:
	// `v='a~b'` and `v='b~'` from a bare `set`, measured 2026-09-17 (#2298).
	s.ListedTildeIsBareWhereItCannotExpand = interp.No
	// And the subscript takes the characters: `m[~/k]` is the three-character
	// key here, where bash and ksh93 expand it. Measured 2026-09-17 on
	// 5.9.2, with `typeset -p` beside the store (#2298).
	s.SubscriptKeyExpandsALeadingTilde = interp.No
	// And a tilde prefix carrying a quote or an expansion still expands here,
	// which is this shell alone: the quotes come off and the name is looked
	// up, so `~\chet/bar` is an error about a user called `chet` where the
	// other six columns print the word as written, and `~"/bar"`, `~$x` and
	// `~+"/x"` all reach the directory. Measured 2026-09-22 against the panel
	// on eleven shapes; see Semantics.TildePrefixStopsAtAQuoteOrAnExpansion
	// (#4156).
	s.TildePrefixStopsAtAQuoteOrAnExpansion = interp.No
	// And an unset `HOME` answers a `~` the way an empty one does — `~` is
	// nothing and `~/x` is `/x` — rather than leaving the word as written.
	// Measured 2026-09-23 on 5.9.2 with `unset HOME`, which really does
	// unset it here (`${HOME-UNSET}` is `UNSET`), and with `v=a:~:b` on the
	// same line for the colon road. The startup row is not this question:
	// this shell seeds `HOME` from the password entry before any line runs,
	// so a bare `env -i zsh` has a home to read where bash has none. See
	// interp.TildeWithNoHomePolicy (#4179).
	s.TildeWithNoHome = interp.TildeWithNoHomeIsEmpty
	s.ListedNonAsciiIsOrdinary = interp.Yes
	// And as **itself** inside a `$'...'`, with no text in front of it
	// reaching for the form. Measured 2026-09-27 on zsh 5.9.2 under
	// `LC_ALL=en_US.UTF-8`: `v=$'a\téb'` lists as `$'a\téb'` and `v="a-é"`
	// as `typeset v=a-é`, where ksh93 writes a code point for both (#4807).
	s.ListedNonAsciiIsSpelledAsACodePoint = interp.No
	s.ListedNonAsciiTakesTheDollarFormAfterANonName = interp.No
	// Nor does the character itself narrow it: measured 2026-09-27 on 5.9.2
	// under `LC_ALL=en_US.UTF-8`, `°`, `€`, `×`, a non-breaking space, `Ⓐ`
	// and an emoji all list bare, where ksh93 takes every one but `Ⓐ` into
	// `$'...'` (#4829).
	s.ListedNonAsciiIsBareOnlyWhenAlphabetic = interp.No
	s.ListedAssignmentPrefixIsBare = interp.No
	// unanswered OperatorAfterTheSubscriptListingIsBad: `${!name[@]}` is a
	// bad substitution here in the *bare* form too, so there is no listing
	// for an operator to come after. Measured 2026-09-14 (#2821).
	// `export -p` is that same form narrowed to the exported names, not a
	// listing that repeats its own command word: it writes the attribute
	// letters, and it picks `typeset` where `export` will not carry the
	// value. Measured 2026-09-12, `env -i` and `-f`, from a table holding
	// `typeset -ix n5=5`, `typeset -ax A5=(1 2); export A5` and a frozen
	// export:
	//
	//	export -i n5=5
	//	typeset -ax A5=( 1 2 )
	//	export -r rr=4
	//	export -T PATH path=( /usr/bin /bin )
	//	export -i10 SHLVL=1
	//
	// The command-word form this had wrote `export A5` and `export n5=5` —
	// the value right in every case and every letter absent, with an array
	// reduced to its name (#1061). dash and ksh93 keep that form, measured:
	// their `export -p` writes no letters at all.
	s.ExportListing = interp.DeclareListingExportSpelled
	// readonly -p speaks typeset here, not readonly.
	s.ReadonlyListing = interp.DeclareListingExportSpelled
	// Only reached once POSIX_BUILTINS moves the two listings to the
	// standard's form. See Semantics.CommandWordListingNeedsAKindLetter.
	s.CommandWordListingNeedsAKindLetter = interp.Yes
	// The bare form drops the command word in both builtins, which is where
	// this shell's `readonly` parts company with its own `readonly -p` by more
	// than a word: `typeset -r R=2` with the letter, `R=2` without it.
	s.BareDeclarationListing = interp.DeclareListingPlainAssignment
	// Every letter joins, kind letters included: `typeset -ax` writes every
	// array and every exported name, and `typeset +xr` every exported name
	// and every read-only one. Measured 2026-09-10, and the same answer
	// under both signs.
	s.DeclarationListingFilter = interp.DeclarationFilterAnyLetter
	s.DeclarePrintReportsAMissingName = interp.Yes
	s.DeclarePrintReportsAMissingFunctionName = interp.No
	// `typeset -p s=5` performs nothing there: it is `no such variable: s`
	// at 1 with `s` still unset, and `typeset -p e=(1 2)` answers the same
	// way — measured 2026-09-20 on zsh 5.9.2. See
	// interp/declareprintoperand.go.
	s.DeclarePrintPerformsItsOperand = interp.DeclarePrintOperandIsANameAlone
	// And `export -p` and `readonly -p` narrow to their operands here, which
	// is the one column where those two words list at all once a name is
	// written beside the letter: measured 2026-09-20 on zsh 5.9.2, `export
	// a1=1; export a2=2; export -p a1` writes `export a1=1` alone and
	// `readonly t=6; readonly -p t` writes `typeset -r t=6`. Nothing is
	// declared — `export -p w=8` is `no such variable: w` at 1 with `w`
	// still unset. See interp/exportprintoperand.go.
	s.ExportOrReadonlyPrintWithOperands = interp.ExportPrintNarrowsToTheOperands
	s.TrapBodyLine = interp.TrapBodyLineWhereItFired
	// Where it fired for a DEBUG or ERR body too: this shell names the
	// firing line for every line of every body, so there is no second
	// answer to give.
	s.CommandTrapBodyLine = interp.TrapBodyLineWhereItFired
	s.TrapActionIsParsedWhenSet = interp.Yes
	// The strict end of the symbolic mask: one operator per clause, a who
	// before `=`, and neither `s` nor `t`.
	s.SymbolicMaskTakesMoreThanOneOperator = interp.No
	s.SymbolicMaskWhoAloneSetsIt = interp.No
	s.SymbolicMaskOmittedWhoHonorsTheMask = interp.No
	s.SymbolicMaskTakesTheSetuidLetter = interp.No
	// Neither: `bad symbolic mode permission: u`, and the same for X.
	s.SymbolicMaskTakesAPermissionCopy = interp.No
	// unanswered UmaskPermissionCopyBesideLetters: a copy is refused here
	// before anything looks at what is beside it — the line above is that
	// refusal — so a clause holding a copy and a letter cannot reach the
	// question. TestUmaskRefusesAPermissionCopy pins the refusal.
	s.SymbolicMaskTakesTheConditionalExecuteLetter = interp.No
	s.SymbolicMaskTakesTheStickyLetter = interp.No
	// A dash word is an option unless it is all digits, which is what
	// separates this from ksh93: `shift -x` is a bad option and `shift -1` is
	// a count it refuses for being below zero.
	s.ShiftOptionWords = interp.ShiftOptionWordsNonNumeric
	s.ShiftDoubleDashEndsOptions = interp.Yes
	// The marker is taken in front of a numeric operand too, so
	// `break -- 1` ends the loop and `exit -- 3` exits 3.
	s.NumericOperandDoubleDashEndsOptions = interp.Yes
	// Refused, and nothing is given up: `for i in 1 2; do break 1 2; done`
	// complains once per pass and runs to the end of the list.
	s.ExtraNumericOperand = interp.ExtraNumericOperandRefused
	s.ShiftNamesAreArrays = interp.Yes
	s.ShiftNegativeIsOutOfRange = interp.Yes
	s.ShiftCountTakesASign = interp.Yes
	s.WaitReadsOptions = interp.No
	// Job specs by command text, a second match taken rather than refused;
	// `wait` complains about a spec that names nothing, has no -n, and
	// `disown` takes the job out of the table.
	s.JobSpecsByName = interp.Yes
	s.AmbiguousJobNameIsRefused = interp.No
	s.WaitReportsAMissingJob = interp.Yes
	// Measured 2026-10-01: a finished job nothing will report is gone from the table at once: `(exit 4) & sleep 0.3; wait %%` is `no current job`, 127. See Semantics.FinishedJobLeavesTheTable.
	s.FinishedJobLeavesTheTable = interp.Yes
	// Whether a job builtin writes a finished job's notice first, with the
	// monitor on and no prompt (#6064). See
	// Semantics.FinishedJobIsReportedByAJobBuiltin.
	s.FinishedJobIsReportedByAJobBuiltin = interp.Yes
	// A file `.` reads from a startup file is numbered from its own first line (#6013). See
	// Semantics.SourcedFileContinuesTheStartupLineCount.
	s.SourcedFileContinuesTheStartupLineCount = interp.No
	// A startup file given up over a fatal error: zsh leaves the error's own status, as in a script (#6046).
	// See Semantics.StartupFileGivenUpLeavesTheStatusBefore.
	s.StartupFileGivenUpLeavesTheStatusBefore = interp.No
	// A failed expansion in a command's words: zsh keeps a failing status: `(exit 4); print $((1/0))` exits 4 (#6067).
	// See Semantics.FailedExpansionInACommandKeepsAFailingStatus.
	s.FailedExpansionInACommandKeepsAFailingStatus = interp.Yes
	// A fatal error in an interactive shell's program: zsh gives up the line of a script under `-i`, and ends a `-c` string (#6073).
	// See Semantics.InteractiveProgramErrorWhenInteractive.
	s.InteractiveProgramErrorWhenInteractive = interp.InteractiveProgramErrorCostsAScriptsLine
	// At a prompt, zsh numbers the session (#6074). See Semantics.PromptNumbersEachInputFromOne.
	s.PromptNumbersEachInputFromOne = interp.No
	// A fatal error in `.` or `eval` text: an interactive zsh gives up the text (#6057).
	// See Semantics.BorrowedTextErrorWhenInteractiveCostsTheLine.
	s.BorrowedTextErrorWhenInteractiveCostsTheLine = interp.No
	// A fatal error in an interactive shell's startup file: zsh gives up the rest of the file (#6009).
	// See Semantics.StartupFileErrorWhenInteractive.
	s.StartupFileErrorWhenInteractive = interp.StartupErrorCostsTheFile
	// A job started before `set -m`, under a terminal: zsh resumes it and waits it out (#5927). See
	// Semantics.JobStartedWithoutTheMonitorIsRefused.
	s.JobStartedWithoutTheMonitorIsRefused = interp.No
	// Measured 2026-10-01: `e=; set -x; $e` traces a bare newline. See Semantics.EmptyCommandTrace.
	s.EmptyCommandTrace = interp.EmptyCommandTracesABareLine
	// Measured 2026-10-01: `{ sleep 1 & jobs }` is `[2]  + running    sleep 1`, the brace group holding 1 while it runs. See Semantics.ACommandHoldsAJobSlot.
	s.ACommandHoldsAJobSlot = interp.Yes
	// Measured 2026-10-01: a builtin's output is held until it returns, so its
	// complaints come first on a stream that carries both. See
	// Semantics.BuiltinOutputHeldUntilItReturns.
	s.BuiltinOutputHeldUntilItReturns = interp.Yes
	// And a job it has already reported stays waitable by its process id:
	// `wait %1; wait "$p"` answers the job's status in 5.9.2.
	s.WaitRemembersAReapedJob = interp.Yes
	s.WaitReportsTheSignalThatEndedTheJob = interp.No
	s.WaitNextJob = interp.WaitNextJobAbsent
	// Nor `-p`: the word is a job spec there too, and `wait -p` is `job
	// not found: -p` at 127. Measured 2026-09-13.
	s.WaitPNamesTheFinishedJob = interp.No
	// A trapped signal cuts a `wait` short with 128 plus the signal, and the
	// form that names a job answers the same as the bare one.
	s.WaitForAJobFailsWhenInterrupted = interp.No
	s.DisownRemovesTheJob = interp.Yes
	// And it reports what it did, as bash does (#3187).
	s.DisownAlwaysFails = interp.No
	s.CommandRejectsUnknownOption = interp.No
	// Whether `command -v` answers for every name it was given, and what
	// decides the status when it found some of them. See
	// interp.Semantics.CommandReportsEveryOperand for the split.
	s.CommandReportsEveryOperand = interp.Yes
	s.CommandCountsAMissingOperand = interp.Yes
	// And whether a `command` reached through an expansion keeps the power
	// to run what it names (#3369).
	s.ExpandedCommandOnlyReports = interp.No
	// What a `command -p` search resolved is **not** remembered here: with
	// an unusable PATH, a `command -p ls` and a plain `ls` after it, the
	// second is 127 in this column and 0 in bash. See
	// interp.Semantics.DefaultPathSearchIsRemembered (#2975).
	s.DefaultPathSearchIsRemembered = interp.No
	// `command` here means an *external* program of that name and nothing
	// else: `command set -o …` is `command not found: set` at 127, not the
	// survivable spelling of a special builtin it is in the other four. The
	// state is not fixed — `posixbuiltins` moves it, and `emulate sh` and
	// `emulate ksh` turn that option on — so setopt.go reads and writes this
	// axis and the preset only says where a plain zsh starts.
	s.CommandReachesABuiltin = interp.No
	// And where it does reach one — `setopt posixbuiltins`, the only route
	// that can be asked at all — it draws no boundary. Measured 2026-09-13
	// with the option on: `eval 'set -Z; echo IN'` ends the subshell at 1 and
	// `command eval 'set -Z; echo IN'` ends it identically, as do `readonly
	// 1bad=x`, `unset 1bad`, `export 1bad=x` and `. /nonexistent/file` in the
	// same text. So the axis is a constant here where CommandReachesABuiltin
	// is not.
	s.FatalErrorEndsAtTheCommandWord = interp.No
	// Under `posixbuiltins`, the one route where the word reaches a
	// builtin, `command readonly x=5` over a frozen name reports and runs
	// the next line, at the top level and in either kind of function
	// (#6962). See
	// interp.Semantics.FrozenOperandRefusalOutlivesTheCommandWord.
	s.FrozenOperandRefusalOutlivesTheCommandWord = interp.No
	s.GetoptsRejectsUnknownOption = interp.No
	s.ShiftCountIsArithmetic = interp.Yes
	// No by construction rather than by measurement: zsh has already parsed
	// the whole action by the time a trap fires, so it never runs part of a
	// body whose rest will not parse. The two answers cannot be told apart
	// here, and this is the one that describes what zsh did.
	s.TrapBodyRunsWhatParsed = interp.No
	s.ExitTrapFiresPastTheEnd = interp.Yes
	// A subshell's own is counted from the line that opened it when the
	// body ran out, and from the `exit` or the failing command otherwise:
	// measured 2026-10-27, `(` on line 2 with a body to line 6 locates the
	// trap at 2, and an `exit 0` on line 8 at 8 (#6899).
	s.SubshellExitTrapFiresAtItsOpeningLine = interp.Yes
	// unanswered ExitTrapFiresWhereTheScriptStopped: asked only where the
	// axis above says no, and it says yes here.
	// A `kill -CHLD` naming the shell runs nothing: the condition is a child
	// of this shell changing state, and a signal the script sent is not that.
	// Measured 2026-09-26 over a script file, `trap 'echo C' CHLD` /
	// `kill -CHLD $$; echo a` / `echo b` reads `a b` (#4756).
	s.SelfAimedChildSignalRunsTheTrap = interp.No
	// A background job alone raises the condition: a foreground external
	// command, a pipeline element, a substitution and a subshell each reap a
	// child here and none of them is a job. Measured 2026-09-27 over script
	// files, `trap 'echo C' CHLD` and then `/bin/echo x`, a pipeline,
	// `v=$(/bin/echo x)` or `( /bin/echo x )`, each followed by `echo a`:
	// only `/bin/echo x & wait` runs the handler (#4780).
	s.ChildConditionCountsEveryReapedChild = interp.No
	// And a `WINCH` aimed the same way is held until this shell reads the
	// next unit of its input or finishes waiting for a child. Measured
	// 2026-09-26 over a script file, `trap 'print W' WINCH` /
	// `kill -WINCH $$; print a` / `print b` reads `a W b`, where the same
	// program under `-c` runs the handler not at all (#4755).
	s.SelfAimedWindowChangeWaitsForInputOrAChild = interp.Yes
	s.SelectAssumesUnboundedWidth = interp.Yes
	s.SelectEofEndsPromptLine = interp.Yes
	s.SelectEofIsSuccess = interp.Yes
	s.SelectTakesUnterminatedReply = interp.Yes
	s.SelectEofPrintsNewline = interp.No
	// A declaration may shadow a frozen name here: `typeset -r x=1; f() {
	// local x=2; echo $x; }` prints 2, the function runs on, and the outer
	// 1 is back — and frozen again — when it returns. bash refuses every
	// spelling of it.
	s.DeclarationMayShadowAReadonly = interp.Yes
	// unanswered DeclarationMayShadowAnEnclosingScopesReadonly: that
	// narrower question is asked only where the one above said no, and this
	// shell takes every shadow — a caller's frozen local included, measured
	// 2026-09-21 — so there is no route by which it is put here.
	// And this is the one shell in the panel where `typeset +r` takes the
	// attribute back off: `typeset -r s=1; typeset +r s; s=9` leaves 9
	// there, silently and with status 0. `declare +r` is the same word. A
	// *special* parameter still refuses — `typeset +r EPOCHSECONDS` is
	// `can't change type of a special parameter` — which this engine has no
	// specials to reach.
	s.ReadonlyAttributeCanBeRemoved = interp.Yes
	// And a declaration's own array literal replaces a frozen *scalar*
	// outright, freeze and all: `readonly q=1; typeset -g q=(b)` leaves
	// `typeset -ar q=( b )` at status 0 here, where bash refuses it. The
	// exemption stops at the container — a name already holding an array or
	// a table refuses the same operand — so it is the retype and not the
	// write. See the axis for the rows (#2250).
	s.ArrayLiteralOperandRetypesAFrozenScalar = interp.Yes
	// unanswered DeclarationRereadsAParenthesizedValue: this shell refuses
	// the assignment before anything asks what the text means. Measured
	// 2026-09-21 under `env -i` on zsh 5.9.2, `typeset -a a="(1 2)"` is
	// `a: inconsistent type for assignment` and `typeset -A m="([k]=v)"` is
	// the same sentence for `m`, so neither the array nor the table route
	// reaches the question (#2298).
	// The letter half of the same rule: a numeric type letter that moves the
	// name to a type it does not already hold carries out its own
	// assignment over the freeze, and the freeze stays on. Not one field
	// with the literal half, because the two disagree over the same frozen
	// array — `typeset -ar q=(a); typeset -gi q=4` is taken and `readonly
	// q=(a); typeset -g q=(b)` is refused. See the axis for the rows
	// (#2539).
	s.NumericTypeLetterRetypesAFrozenName = interp.Yes
	// And nothing else about a frozen name's attributes is refused here
	// either — the letter that is not a numeric type is taken just the same.
	// Measured 2026-09-13 under `env -i` on zsh 5.9.2, `readonly q=1;
	// typeset -u q` lists `typeset -ur q=1` at 0. What is still refused is a
	// *value*, which is a different question and reaches refuseReadonly
	// (#2561).
	s.AttributeOverAFrozenNameIsRefused = interp.No
	// And with no value either: measured 2026-09-20, `readonly c; typeset
	// -i c` is taken at 0 here, exactly as the same line over a frozen name
	// holding a value is (#3937).
	s.TypeLetterOverAFrozenNameWithNoValueIsRefused = interp.No
	// Nor a keyed one over a frozen name holding a value: measured
	// 2026-09-20, `c=1; readonly c; typeset -A c` is taken at 0 here
	// (#3965).
	s.KeyedLetterOverAFrozenNameHoldingAValueIsRefused = interp.No
	s.DeclaredNameWithoutValueIsEmpty = interp.Yes
	// Reached only once TYPESET_TO_UNSET has turned the field above off, and
	// then a function's declaration hides the caller's value: measured
	// 2026-10-02 on zsh 5.9.2, `-f`, `setopt typesettounset; g=1; f() { local
	// g; echo ${g-UNSET}; }; f; echo $g` is `UNSET` then `1` under `emulate
	// zsh`, `emulate sh` and `emulate ksh` alike, and so are `typeset g` and
	// `local -x g` in the function and a `local g` in a function its caller
	// had already given a local `g` (#5157).
	s.ValuelessDeclarationHidesTheOuterValue = interp.Yes
	// unanswered PrefixListingNamesADeclaredOnlyCompound: there is no
	// `${!prefix@}` in this shell at all — `typeset -A q1; echo "${!q@}"` is
	// `bad substitution` — so no listing of that shape ever reaches the axis
	// to be asked. Measured 2026-09-16 on zsh 5.9.2.
	// A declaration without a value leaves the name holding the empty string
	// under this shell's own emulation, so `typeset xyz; typeset -p xyz`
	// writes `typeset xyz=''` and never reaches the record (#2999). Under
	// `emulate sh` and `emulate ksh` it does reach it, and the row is the
	// name alone: `emulate sh; typeset X; typeset -p X` is `typeset X` at 0.
	// Measured 2026-09-27 on zsh 5.9.2, `-f`, from a script file, read both
	// through `emulate MODE` and through a copy of the reference named for
	// the mode.
	//
	// So **one value is right in every mode** and this is not a fifth field
	// on the emulation table: where the mode says a declared name is empty
	// the axis is unreachable, and where it does not the answer is yes. What
	// the mode moves is DeclaredNameWithoutValueIsEmpty, which is already on
	// that table (#4753).
	s.ValuelessDeclarationRecordsTheName = interp.Yes
	// The other route into the same state is the opposite answer here, which
	// is what split the two axes: `unset` of a local the running call
	// declared leaves the name unset with the shadow standing, and this
	// shell writes **nothing** for it under every emulation. Measured
	// 2026-09-21 and again 2026-09-27, `env -i PATH=/usr/bin:/bin LC_ALL=C`
	// with a scratch HOME, `v=global; f() { local v; unset v; typeset -p v;
	// }; f` — no output, at 0, with `${v-UNSET}` firing its default inside
	// and the global back afterwards. bash writes `declare -- v` there
	// (#4787).
	s.UnsetOfALocalRecordsTheName = interp.No
	// The name is there and the listing writes nothing for it — the third
	// state the field above cannot spell. See Semantics.ValuelessRecordIsStillAName
	// for the rows (#4053).
	s.ValuelessRecordIsStillAName = interp.Yes
	// And the same answer for a name of the *shell's* own that an `unset`
	// removed, which is the state a script reaches without a function at
	// all: `unset RANDOM; typeset -p RANDOM` is nothing at 0, where `w=1;
	// unset w; typeset -p w` on the next line is `no such variable: w` at 1.
	// The word it turns on is `special` — `HOME` against `LOGNAME`,
	// `HISTSIZE` against `MAILCHECK`, `PS1` against `TTY`, each pair holding
	// the other attribute words still. Measured 2026-09-27 on zsh 5.9.2;
	// interp/removedshellown.go has the grid (#4896).
	s.RemovedShellOwnParameterIsStillAName = interp.Yes
	// The export letter carries `-g` with it, so `typeset -x v=1` inside a
	// function declares no local — `local -x` is the spelling that still
	// does, and a name this scope has already made local stays local.
	s.ExportLetterDeclaresAGlobal = interp.Yes
	// A declaration with no letters and no value writes the name back, where
	// it is holding something already: `s=str; typeset s` prints `s=str`.
	s.ValuelessDeclarationOfAHeldNameListsIt = interp.Yes
	// And a plain word declared over a name really holding an array is an
	// inconsistent type rather than a replacement, fatally.
	s.ScalarOverACompoundIsAnInconsistentType = interp.Yes
	// An array letter given a plain word: see
	// interp.Semantics.ArrayLetterWithAWordDeclaresAScalar (#5630).
	s.ArrayLetterWithAWordDeclaresAScalar = interp.No
	// A plus-only line's value, and the letters it takes off: see
	// interp.Semantics.PlusLetterComesOffAfterTheValueLands (#5663).
	s.PlusLetterComesOffAfterTheValueLands = interp.No
	// A `-p` listing whose first option word is plus-signed: see
	// interp.Semantics.PlusSignedPrintListsNoValues (#5642).
	s.PlusSignedPrintListsNoValues = interp.No
	// `readonly -a` declares the array as well as freezing the name:
	// `readonly -a a` lists as `typeset -ar a=(  )`.
	s.ReadonlyRecordsTheCompoundAttribute = interp.Yes
	// A valueless declaration sets the name here, so an exported name of a
	// numeric type is holding `0` already and there is no valueless one for
	// this to be about. What the one-command form hands a child is nothing —
	// `typeset -ix Z; env` is silent — and a second declaration is what
	// gives the name its value and the child its entry. See
	// Runner.declarationOwnsTheStandingEmpty.
	s.NumericTypeWithNoValueReachesAChildAsZero = interp.No
	// The integer and float letters make a name a *scalar* of that type
	// here, so a declaration that also assigns an array literal is asking
	// for two kinds at once and is refused fatally — `typeset -ia z=(1 2)`
	// is `typeset: z: inconsistent type for assignment` at 1. The array
	// letter is not what triggers it and the case letters do not: `typeset
	// -i z=(1 2)` is the same refusal and `typeset -ua q=(ab cd)` is taken.
	s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = interp.Yes
	// A numeric type letter takes the container letter off the same
	// declaration: `typeset -ia z` is `typeset -i z=0` here and `typeset -iA
	// m` is `typeset -i m=0`, a scalar of that type either way. The valued
	// form of the same pairing is the refusal above rather than a collapse.
	s.NumericAttributeReplacesTheArrayAttribute = interp.Yes
	// And an array literal over a name that is not an array re-creates it,
	// dropping the letters that say what its values are — assigned and
	// appended alike, which is where this shell parts from ksh93: `typeset
	// -i p=3; p+=(5+5)` is `typeset -a p=( 3 5+5 )` here and `typeset -a -i
	// p=(3 10)` there.
	s.ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = interp.Yes
	s.AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = interp.Yes
	// A numeric letter takes a case attribute off — `typeset -l z; typeset
	// -i z` is `typeset -i z=1` — and the case letter does not take the
	// numeric one off: `typeset -i y; typeset -l y` keeps both, `typeset -il
	// y=1`. The one column that answers the two directions differently,
	// which is why they are two axes.
	s.NumericAttributeReplacesTheCaseAttribute = interp.Yes
	// But within one declaration the two letters part company: `-l` records
	// beside a numeric type letter and `-u` records nothing. Measured
	// 2026-09-12, `typeset -li v=4` lists `typeset -il v=4` where `typeset
	// -ui v=4` lists `typeset -i v=4` — upper case being the rendering a
	// based integer already has here (#2541).
	s.UpperCaseLetterBesideANumericTypeLetterRecordsNothing = interp.Yes
	// Both case letters on one declaration cancel, and take off a standing
	// one with them. Measured 2026-09-12, `typeset -lu z=Ab` lists `typeset
	// z=Ab` with the value unfolded, and `typeset -l z=Ab; typeset -lu z=Cd`
	// lists `typeset z=Cd` (#2541).
	s.TwoCaseLettersOnOneDeclarationCancel = interp.Yes
	// The letter that lists is the letter that folds: see
	// interp.Semantics.CaseListingAndFoldAreSeparate (#5671).
	s.CaseListingAndFoldAreSeparate = interp.No
	s.CaseAttributeReplacesTheNumericAttribute = interp.No
	// The same reading of an undeclared name reached through a subscript:
	// a name that is not an array here reads as a scalar, so a quoted
	// `"${a[@]}"` on one nothing declared is the one empty field `"$a"`
	// gives. This shell alone. Measured 2026-09-07 against 5.9.2, with a
	// function so the positional-parameter builtin is not a confound:
	//
	//	f() { printf '%s\n' "$#"; }
	//
	//	unset a;        f "${a[@]}"   1     no array, so a scalar reading
	//	a=();           f "${a[@]}"   0     a set, empty array
	//	typeset -a a;   f "${a[@]}"   0     the same, declared
	//	a=x;            f "${a[@]}"   1     control
	//
	// The middle two are what makes it existence rather than emptiness, and
	// they are the direction bash and ksh93 answer both halves in. Note the
	// asymmetry runs the other way from the one an empty-array axis would
	// predict: the *declared* empty array is the one with no field here.
	s.UnsetNameAtIsOneEmptyField = interp.Yes
	// And the other half of that asymmetry: an array with no elements *is*
	// a set parameter here, so `e=(); echo "[${e[@]+S}]"` writes `[S]` and
	// `${e[@]-D}` substitutes nothing. bash 5.3.20, bash 3.2.57 and ksh93u+
	// all read it as unset. Measured 2026-09-16 on zsh 5.9.2 under
	// `env -i PATH=/usr/bin:/bin LC_ALL=C`, from a file (#2298).
	s.EmptyArrayIsSet = interp.Yes
	// And `[[ -v a[@] ]]`, which is a different question: the subscript names
	// an **element** here, and no array has one called `@` or `*`. Measured
	// 2026-09-18 from a script file — `f=(x); [[ -v f[@] ]]` is false with an
	// element in the array, `typeset -A n; n[k]=v; [[ -v n[@] ]]` is false
	// with a key in the table, and `s=plain; [[ -v s[@] ]]` is *true*,
	// because a scalar has no elements to name and answers for itself
	// (#3436).
	s.ConditionWholeArraySubscript = interp.ConditionWholeArraySubscriptNamesAnElement
	// `set -u` reaches two subscripted shapes here that it reaches in no
	// other column. The length of an element that is not there is a refusal
	// — `a=(x y z); echo "${#a[9]}"` is `a[9]: parameter not set` where bash
	// 5.3.20, bash 3.2.57 and ksh93u+ all answer `0` at 0 — and a
	// whole-array subscript on a name holding nothing at all is one too:
	// `${nope[@]}` and `${nope[*]}` are refused here and in bash 3.2.57,
	// against `[]` at 0 in bash 5.3.20 and ksh93u+. `b=(); ${b[@]}` is the
	// control for the second and is `[]` at 0 here, so what it refuses is
	// the absent name rather than the empty array. Measured 2026-09-18 on
	// zsh 5.9.2 under `env -i HOME=… PATH=/usr/bin:/bin LC_ALL=C`, from a
	// script file with each row in a subshell (#2980).
	s.LengthOfAMissingElementIsRefused = interp.Yes
	s.UnsetNameWithAWholeArraySubscriptIsRefused = interp.Yes
	// The **count** of a whole array is a third shape of the same option and
	// it is refused here too, for a name holding nothing at all: `${#a[@]}`
	// on a name that was never mentioned is `a[@]: parameter not set` and
	// ends the shell, where ksh93u+ answers `0`. A name holding a string is
	// not a name holding nothing — `x=abc; ${#x[@]}` is `3` here — which is
	// what separates this from bash 5.3.20's reading of the same line, where
	// a scalar is refused alongside the absent name. Measured 2026-09-18 on
	// zsh 5.9.2 under `env -i HOME=… PATH=/usr/bin:/bin LC_ALL=C`, from a
	// script file with `echo REACHED` on the line after (#3125).
	s.WholeArrayCount = interp.WholeArrayCountRefusesANameHoldingNothing
	// And the colon form of the same operator counts the elements under
	// `[@]` rather than reading the join, so a one-element array holding the
	// empty string is a **value**: `f=(""); "${f[@]:-x}"` is `[]` and
	// `"${f[@]:+y}"` is `[y]`, where bash 5.3.20 answers `[x]` and `[]`.
	// `"${f[*]:-x}"` is `[x]` in both columns, which is what keeps the two
	// spellings apart here and is why this is not simply the element count.
	// Measured 2026-09-18 on zsh 5.9.2 under `env -i HOME=…
	// PATH=/usr/bin:/bin LC_ALL=C`, from a script file (#3425).
	s.WholeArrayColonTest = interp.WholeArrayColonTestCountsTheElementsUnderAt
	// And an attribute added to a name that already holds a value re-reads
	// it at once, as ksh93 does: `FOO=bar; typeset -i FOO` stores 0 and
	// `d=MiXeD; typeset -u d` stores MIXED. A separate question from the
	// one above, which this shell happens to answer the same way — bash
	// answers both no and ksh93 answers them differently from each other.
	s.AttributeRereadsTheValueItFinds = interp.Yes
	s.InheritedValueSurvivesADeclaredType = interp.Yes
	s.CompoundElementsGoThroughTheAttribute = interp.No
	// The case attributes fold on every *read* here and never touch the
	// store: `typeset -l lo=AB` lists back as `typeset -l lo=AB`, and
	// `typeset +l lo` reveals `AB`. See Semantics.CaseAttributeFoldsWhenRead
	// for the panel (#1755).
	s.CaseAttributeFoldsWhenRead = interp.Yes
	s.CompoundAttribute = interp.CompoundAttributeReplacesItWithAScalar
	// And the converse discards under both letters: `b=1; typeset -a b` is
	// `typeset -a b=( )` with `${#b[@]}` 0 and `$b` empty, and `a=1;
	// typeset -A a` is `typeset -A a=( )`. Measured 2026-09-08 against
	// 5.9.2 — the one column that throws the script's own value away, and
	// the one this implementation was giving every dialect.
	// A kind letter replaces the kind here, alone in the panel: measured
	// 2026-09-27, `typeset -i q; typeset -a q` lists `typeset -a q=(  )` with
	// the integer attribute gone, and `typeset -a q; typeset -F q` is
	// `typeset -F q=0.0000000000`. See
	// interp.Semantics.KindLetterReplacesTheKind (#4881).
	s.KindLetterReplacesTheKind = interp.Yes
	s.ScalarUnderAnArrayDeclaration = interp.ScalarUnderACompoundDiscardsIt
	s.ScalarUnderATableDeclaration = interp.ScalarUnderACompoundDiscardsIt
	// A name holding a compound reaches no child, with bash and against
	// ksh93 (#1380).
	s.ExportedCompoundReachesAChildAsItsFirstValue = interp.No
	// And a **subscripted operand** records no attribute at all here:
	// measured 2026-09-12, `typeset -x a[1]=v` and `export a[1]=v` both list
	// `typeset -a a=( v )` with no `x`, where the whole-name `typeset -x
	// a=(p q)` lists `typeset -ax`. So it is the subscripted operand that
	// carries nothing, and not the letter (#1380).
	s.SubscriptedOperandCarriesTheAttributes = interp.No
	// `a=(1 2); a+=x` adds a third element rather than joining the first:
	// `typeset -a a=( 1 2 x )`. The empty string is a value and gets an
	// element of its own — `a=(1 2); a+=""` is three elements — and a value
	// with a space in it is still one element.
	s.ScalarAppendedToAnArrayBecomesANewElement = interp.Yes
	// `a=(1 2 3); a=x` leaves `typeset a=x` and `${(t)a}` reads `scalar`:
	// the value is the whole of the name and the array is gone, not hidden.
	// A table goes the same way. This is what makes `for a in x y z` over a
	// name holding an array read `x`, `y`, `z` rather than the array three
	// times, and `read a` collapse it.
	//
	// This one is the option's, too: under `setopt ksharrays` the same line
	// writes the array's base and leaves the rest standing — `a=(first
	// second); a=word` is `typeset -a a=( word second )`, which is what bash
	// and ksh93 do with nothing set. It is the eighth axis setKshArrays
	// moves, and it could not move before the line below existed, since a
	// *table* under the option refuses the store rather than taking its key
	// `0` (#4618).
	s.ScalarAssignedOverACompoundReplacesTheName = interp.Yes
	// An array holding the scalar a plain store gave it: see
	// interp.Semantics.ScalarHeldUnderTheArrayLetterListsAsAScalar (#5643).
	s.ScalarHeldUnderTheArrayLetterListsAsAScalar = interp.No
	// And with `ksharrays` off nothing refuses the store first, which is what
	// makes the line above reachable for a table at all: `typeset -A h=(one
	// 1); h=string` is `typeset h=string` at 0, and so is the `h+=string`
	// spelling. Under the option both are `h: attempt to set associative
	// array to scalar` and the shell leaves — see setKshArrays, which is
	// where the seventh axis moves — and
	// Semantics.ScalarStoredOverATableIsRefused for the grid (#4617).
	s.ScalarStoredOverATableIsRefused = interp.No
	s.ArrayLiteralAssignmentStartsTheNameOver = interp.No
	s.TypesetLocalNeedsKeywordFunction = interp.No
	// A local does not inherit the export attribute of the name it shadows.
	// Measured with a real child: `export FOO=bar; f() { local FOO=baz; env;
	// }` shows the child no FOO at all here, where bash and dash show it the
	// local's value.
	s.LocalInheritsTheExportAttribute = interp.No
	s.SplitParamExpansion = interp.No
	// And an arithmetic result is never split, under `shwordsplit` or an
	// argv[0] of `sh` alike — see Semantics.ArithExpansionIsSplit.
	s.ArithExpansionIsSplit = interp.No
	// An unquoted list is joined on IFS before it is split, as in bash.
	//
	// **This was `No`, and the measurement behind that was taken in the one
	// state where the question cannot be asked.** `setopt shwordsplit` is
	// what makes this shell split a list at all; without it an unquoted list
	// is one word per element with the empty ones elided, so
	// `IFS=:; set -- x "" y; printf "[%s]" $*` is `[x][y]` — which was read
	// as "no join" and is what a shell that splits *nothing* prints. With
	// the option on the same line is `[x][][y]`, which is bash's answer and
	// the one a join produces (#4586).
	//
	// The other two rows the `No` rested on say nothing either. `IFS=:;
	// a=(x y); printf "[%s]" ${a[*]}` is `[x][y]`, and a join on `:` gives
	// `x:y`, which splits back to exactly that; and `a=("x y" z)` under
	// `shwordsplit` was recorded as `[x][y][z]` and is `[x y][z]` — `IFS` is
	// a colon there, so the space in the element was never a separator.
	//
	// **The discriminating pair is a list of empty elements**, measured
	// 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
	// /opt/homebrew/bin/zsh under `-f` with `setopt shwordsplit`, with
	// `w(){ printf '%d |' $#; for x in "$@"; do printf ' [%s]' "$x"; done; }`:
	//
	//	IFS=:; set -- '';       w $@   0 |
	//	IFS=:; set -- '' '';    w $@   2 | [] []
	//	IFS=:; set -- '' '' ''; w $@   3 | [] [] []
	//
	// One empty element is *no* field and two are *two*, which is what a
	// join gives — nothing to split, then `:` and `::`. Splitting each
	// element on its own answers those 0, 1 and 1 whichever way the empty
	// element's field is treated: keep it and the first row is 1, remove it
	// and the second is 0.
	//
	// It is the same join bash has, and the two columns then part on
	// TrailingSeparatorEndsAField alone — which is answered `Yes` here and
	// `No` there, and is the whole of the difference between
	// `IFS=:; set -- 2 ''; w $@` being `2 | [2] []` and `1 | [2]`.
	s.UnquotedListJoinsOnIFS = interp.Yes
	// And the boundary between two elements is a field break, as in ksh93
	// and unlike dash and BusyBox ash. Measured 2026-09-26 on zsh 5.9.2
	// under `shwordsplit`, which is where this shell splits a list at all:
	// `IFS=:; set -- b ':'; w x$@y` is `[xb] [] [y]`.
	s.UnquotedListBoundaryIsIFSWhitespace = interp.No
	// A subscript is not a quoting context here — it is taken exactly as
	// written, with substitutions performed and every other character kept.
	// So `m["k"]=W` stores under the three characters `"k"` and `${m[k]}`
	// finds nothing, and `${q["$v"]}` looks up a key with two quote
	// characters in it where `${q[$v]}` looks up the value's own text. It is
	// one rule with the search operand's, which is why both go through
	// Runner.searchOperand.
	s.SubscriptIsAQuotingContext = interp.No
	// An indexed subscript keeps its quoting for the arithmetic: `${a['2']}` is `bad math expression` (#5562).
	s.IndexedSubscriptKeepsItsQuoting = interp.Yes
	// And a backslash in a pattern reaches only a metacharacter here: before
	// anything else it is a literal backslash, so a raw `bet\\a` matches the
	// five characters and not `beta`. The set is this shell's pattern
	// metacharacters, measured under `setopt globsubst`, which is the only
	// way to hand this matcher a backslash the lexer has not already spent.
	// The backslash itself is in the set — `x\\y` matches one backslash there
	// and not two — which is what keeps a glob-escaped value literal.
	s.PatternEscapeReaches = `-=!*?[]()|^~#<>\`
	// Seven character-class names beyond the twelve POSIX ones, and this is
	// the only column that has more than one of them. Measured 2026-09-10, a
	// character at a time, against zsh 5.9.2:
	//
	//	ascii       one byte below 0x80 — `é` is not in it
	//	IDENT       what a parameter name may hold: alnum and `_`
	//	IFS         the field separators as `$IFS` stands
	//	IFSSPACE    the whitespace among them
	//	WORD        alnum together with `$WORDCHARS`
	//	INCOMPLETE  a byte that could begin a character and does not
	//	INVALID     a byte that could not begin one
	//
	// `IDENT` is the one that was reachable and wrong. `gitstatus` guards its
	// argument with `[[ $name != [[:IDENT:]]## ]]`, and with the name missing
	// the guard fired on every well-formed name it was given — silently, at
	// status 0, so the plugin reported a bad argument and no diagnostic of
	// this shell's said why (#1721).
	//
	// `ascii` is the one that is not this shell's own: bash 5.3 and bash 3.2
	// answer it too, and ksh93 and dash do not.
	s.PatternClasses = "ascii IDENT IFS IFSSPACE INCOMPLETE INVALID WORD"
	// And inside a bracket expression the backslash is a member of the set as
	// well as the protection for the character behind it, which no other
	// column does: `p='[\)]'; [[ $s == ${~p} ]]` matches `)` here and matches
	// a lone backslash too, and `p='[\-z]'` matches a dash, a `z` and a
	// backslash while matching no `y` — the dash behind the backslash stays a
	// member rather than becoming the range operator. Measured through `${~p}`
	// because a pattern written in the source has had its escapes spent by
	// quote removal, which is why *that* route is unanimous across the panel
	// and this one is not (#1407).
	s.BracketEscape = interp.BracketEscapeProtectsAndIsAMember
	// And the join this shell *does* perform: an unquoted `@` list reaching
	// a context that keeps no fields is joined on the first character of
	// IFS, so `IFS=-; a=(x y z); v=${a[@]}` is `x-y-z` where bash and ksh93
	// give `x y z`. With IFS set and empty it is `xy`, which is what says
	// the separator is read from IFS rather than defaulted to a space.
	s.UnsplitAtListJoinsOnIFS = interp.Yes
	// zsh 5.9.2 prints `e1: a:b:c` for `${e1?$*}` under `IFS=:`. Worth
	// stating that this shell answered neither reading before the axis: with
	// zsh's own no-split rule over the fields path it made `a:b c`.
	s.DiagnosticWordIsFields = interp.No
	// The separator that closes a value delimits here rather than being
	// absorbed, so every such split has one more field than it does in the
	// rest of the panel: `IFS=:; v='a:'` under `shwordsplit` is `[a][]` where
	// the other five give `[a]`, and `read -A` on the same line fills three
	// elements from `a:b:` where they fill two. A leading separator opens a
	// field everywhere, and whitespace is absorbed at both ends here too —
	// this is the non-whitespace tail alone.
	s.TrailingSeparatorEndsAField = interp.Yes
	// And `read` into an array asks the same question about a closing run of
	// IFS *whitespace* and answers it the same way, where the expansion above
	// absorbs it: measured 2026-09-07, `read -A r <<< " a "` is two elements
	// here and `x=" a "; set -- ${=x}` is one field. Same shell, same IFS,
	// two answers — which is why they are two fields.
	s.ReadTrailingWhitespaceEndsAField = interp.Yes
	// A line that splits into nothing fills one empty element rather than
	// none, which is ksh93's answer too and not bash's.
	s.ReadNoFieldsIsOneEmptyElement = interp.Yes
	// The whitespace half of IFS is POSIX's three and not `isspace`: with
	// `IFS` the vertical tab alone and `setopt shwordsplit`, `a\v\vb` is
	// three fields here where bash 5.3 and ksh93 give two.
	s.IFSWhitespaceIsEverySpaceCharacter = interp.No
	// A bare `read` trims the record the way a named operand's value is
	// trimmed: a line with two spaces at each end reaches REPLY with none of
	// them, which is ksh93's answer and not bash's.
	s.BareReadTakesTheLineWhole = interp.No
	s.GlobExpansionResults = interp.No
	// A word beginning `%?` is the `%?string` job spec, so that `?` is not a
	// pattern. Measured 2026-10-01; see Semantics.JobSpecQuestionMarkIsLiteral.
	s.JobSpecQuestionMarkIsLiteral = interp.Yes
	s.GlobNoMatchIsError = interp.Yes
	// And a refusal in a program's words is the program's: the script
	// carries on at 1 (#6524). See the axis for the table.
	s.UnmatchedPatternFailsOnlyTheProgram = interp.Yes
	// No parameter of GLOBIGNORE's kind, and `.` and `..` are not in what a
	// pattern may match: `echo .*` is `.dot` alone and `echo .*/` matches
	// nothing at all. Measured 2026-09-14 (#2748).
	s.GlobListsDotAndDotDot = interp.No
	// unanswered IgnoredNamesValueIsOnePattern: there is no parameter of
	// GLOBIGNORE's kind in this shell — measured 2026-09-14, setting
	// `GLOBIGNORE` and `FIGNORE` alike changes nothing about what `echo *`
	// produces — so the three questions about *how* its value is read
	// cannot be put to it. IgnoredNamesMatchTheLastComponent and
	// IgnoredNamesFollowTheParameter are unanswered beside it for the same
	// reason, and IgnoredNamesVariable being empty is what keeps any of the
	// three from ever being reached (#2748).
	// unanswered IgnoredNamesMatchTheLastComponent: see above.
	// unanswered IgnoredNamesFollowTheParameter: see above.
	// The one column that assigns: `set --; printf "<%s>" ${1:=abc}` is
	// `abc` at status 0 here and `$1` is `abc` afterwards, where the other
	// five refuse it fatally. `@` and `*` are refused here too — `not an
	// identifier` — so it is the positional alone that parts (#1541).
	s.AssignThroughExpansionMayNameAPositional = interp.Yes
	s.AssignmentPrefixPersistsOnSpecialBuiltin = interp.No
	// And `command` in front of one takes the persistence away where the
	// option puts it there. Reachable only under `setopt posixbuiltins`,
	// which is where the row above turns yes, and measured there:
	// 2026-09-18, `setopt posixbuiltins; s=base; s=C command :` leaves
	// `base` while the bare `s=P :` leaves `P` (#3448).
	s.CommandKeepsASpecialBuiltinsPrefix = interp.No
	// And a prefix to a function is transient here too, and exported while
	// the call runs: `f(){ echo "[$v]"; }; v=1; v=9 f` prints `[9]` with
	// `v=9` in a child's environment, and leaves `1` behind. Measured
	// 2026-09-12 in 5.9.2. `emulate sh` moves it to persisting, which is
	// the off-panel reading that keeps the two questions apart (#2407).
	s.AssignmentPrefixPersistsAfterAFunction = interp.No
	s.PrefixToAFunctionIsExported = interp.Yes
	// The same answer as bash, measured the same day on 5.9.2: `s+=5 kf` in
	// front of a `function`-form function shows the body `base5` (#3161).
	s.PrefixToAKeywordFunctionIsScopedToTheCall = interp.No
	// Under `posixbuiltins` a prefix in front of `local` or `typeset` persists,
	// and it is an ordinary assignment rather than the function's local:
	// measured 2026-10-29 on 5.9.2, `g() { echo "g v=$v"; }; f() { v=new
	// local y=1; g; echo "in v=$v"; }; v=old; f; echo "out v=$v"` is `new` all
	// three times. The option keeps it by making the declaration family
	// special; see its entry in setopt.go (#7009).
	s.PrefixToALocalDeclarationIsALocal = interp.No
	// An array literal written as a prefix is the array: measured 2026-10-15
	// on 5.9.2, `f(){ typeset -p x; }; x=(a b) f` lists `typeset -g -a x=( a
	// b )` and `x=(a b) printenv x` finds no `x` (#6547).
	s.PrefixArrayLiteralIsAnArray = interp.Yes
	// And it writes the binding the name already has rather than a cell of its
	// own, which is where this shell parts from bash. Measured 2026-09-21 on
	// 5.9.2 with `ff() { typeset -p foo; }`: `typeset -i foo=7; foo=bar ff`
	// lists `export -i foo=0` and `typeset -u foo=abc; foo=bar ff` lists
	// `export -u foo=BAR` — the letters still on, and the word read through
	// them. The array rows agree with bash's for a reason that is not this
	// question: a plain scalar assignment replaces an array here anyway, so
	// `foo=(asdf fdsa); foo=bar ff` lists `export foo=bar` under the overlay
	// too (#4087).
	s.AssignmentPrefixMakesAFreshCell = interp.No
	// And the prefix of an external command is expanded in its child, so a
	// side effect in it never reaches this shell: measured 2026-10-16, `i=1;
	// y=$((i++)) /usr/bin/true; echo $i` is 1 (#6784).
	s.PrefixExpandsInTheExternalCommandsChild = interp.Yes
	// A prefix to a *builtin* is the other question and this shell answers it
	// the other way: the attribute is left exactly where it was. `export z=1;
	// z=2 typeset -p z` lists `export z=2` and `c=1; c=2 typeset -p c` lists a
	// plain `typeset c=2`, so nothing is gained and nothing taken off.
	// Measured 2026-09-16 in 5.9.2 (#3437).
	// The redirections are opened first, so a substitution in a prefix's value
	// never runs when one fails: measured 2026-09-18, `w=$(echo S >&2) f >
	// /nope/x` writes the file complaint alone (#3449).
	s.PrefixExpandedBeforeTheRedirections = interp.PrefixExpandedBeforeRedirectionsNever
	// Whether a command's trace line waits for its redirections. See
	// interp.Semantics.TraceLineFollowsTheRedirections (#5547).
	s.TraceLineFollowsTheRedirections = interp.Yes
	// See interp.Semantics.TraceRedirectionsOnALineOfTheirOwn (#5558).
	s.TraceRedirectionsOnALineOfTheirOwn = interp.No
	// zsh writes each word before its store (#5546).
	// See interp.Semantics.TraceLineFollowsTheStore.
	s.TraceLineFollowsTheStore = interp.No
	// And yet the prefix is worked through **before** a declaration utility's
	// operand, which is why the two are separate axes: this shell opens the
	// redirections ahead of the prefix and still reaches the prefix ahead of
	// the operand. Measured 2026-09-19, `PRE=$(echo PRE >&2) export s=$(echo
	// OP >&2)` writes `PRE` and then `OP` (#3814).
	s.PrefixExpandedBeforeADeclarationsOperand = interp.Yes
	// And a failure that ends the shell leaves the entries already assigned:
	// measured 2026-10-30, `trap 'echo $v' EXIT; v=old; v=new w=${q?boom} true`
	// writes `new` (#7004).
	s.PrefixEntriesStayWhenAFailureEndsTheShell = interp.Yes
	s.PrefixExportAtABuiltin = interp.PrefixExportAtABuiltinUnchanged
	// And a listing with no operand sees the prefix's entry as an ordinary
	// one: its value is written and the attribute tables say the rest, so
	// `m=1; m=9 export -p` writes nothing and `m=9 typeset -p` writes
	// `typeset m=9`. Measured 2026-09-18 (#3446).
	s.PrefixInAWholeTableListing = interp.PrefixInAWholeTableListingIsAnOrdinaryEntry
	// And a declaration keeps nothing: `b=7; b=8 readonly b` reads `7` back
	// and unfrozen, `d=8 export d` reads `7`, `y=2 typeset -r y` reads `1`.
	// The temporary the prefix made is what the attribute went on, and it
	// leaves with the command (#3437).
	s.DeclarationPromotesThePrefixEntry = interp.No
	// A subscripted name in a prefix writes the element, and **this shell
	// never gives it back** — which is the opposite of what it does with the
	// scalar beside it, since a scalar prefix here outlives nothing at all,
	// not a function and not `:`. `arr=(x y z); arr[1]=P read -r j
	// </dev/null` leaves `P`, and so do a function, `:` and `eval`; only a
	// command a child runs leaves `x`, and that row is no dialect's answer.
	// Measured 2026-09-16 on zsh 5.9.2 (#3433).
	s.SubscriptedAssignmentPrefix = interp.SubscriptedPrefixStoresTheElement
	s.SubscriptedPrefixIsTakenBack = interp.No
	// An assignment prefix to a frozen name is answered by the kind of
	// command too, and on a different line from ksh93's: everything this
	// shell runs itself ends the script, and an external one does not.
	// `command` is read as the word that was *written* rather than looked
	// through, which is the row that separates the two readings. Measured
	// 2026-09-11 with `readonly x=1` and `x=2 <cmd>; echo after`:
	//
	//	/bin/echo RAN      complains, never runs it, 1, reaches `after`
	//	true, echo E       complains and ends the script at 1
	//	an alias for true  complains and ends the script at 1
	//	command true       complains, never runs it, 1, carries on
	//	command /bin/echo  the same
	//	: and a function   complains and ends the script at 1
	//
	// The refusal always costs the command where it is not fatal, and the
	// status is 1 rather than the 127 a name nothing can run would earn:
	// the lookup never happens (#1219).
	s.PrefixToARegularBuiltinIsRefused = interp.Yes
	s.PrefixRefusalFatality = interp.PrefixRefusalFatalOnACommandThisShellRuns
	s.PrefixRefusalCostsTheCommand = interp.Yes
	// hash counts only what PATH holds: a builtin or a function is "no
	// such command" to it — and so is a pathname it was handed. Measured
	// 2026-09-21: `hash /bin/ls` is `no such command: /bin/ls` at 1 with the
	// table left empty, where three of the panel say nothing at all and
	// ksh93 remembers the path as written.
	s.HashSearchesPathAlone = interp.Yes
	// Which is the same answer said from the other side: the operand is not
	// passed over in silence here, it is searched for the only way this
	// shell searches and reported when the search cannot have it (#4064).
	s.HashIgnoresAnOperandWithASlash = interp.No
	// `hash name=value` writes the command table here, where the rest of the
	// panel reads the whole word as a name to look up (#4456), and `hash -m`
	// reads the operands as patterns over either table (#4445). Both are
	// letters and shapes only this shell has; see the axes for the columns.
	s.HashDefinesAnEntryFromAnAssignment = interp.Yes
	s.HashReadsOperandsAsPatterns = interp.Yes
	// And `-v` reports each operand the call answered, over either table, in
	// the listing's own shape — a letter only this shell has as well.
	// Measured 2026-09-27; see the axis for the rows, and note that a *query*
	// prints too, which is what makes it "answered" rather than "added"
	// (#4964).
	s.HashReportsEachEntry = interp.Yes
	// And `-r` here is the whole of the call rather than a first half of
	// one: an operand beside it is `too many arguments` at 1 and the table
	// is left standing, where every other column clears and then hashes the
	// name it was given. Measured 2026-09-26 — see the axis for the panel
	// (#4744).
	s.HashClearRefusesOperands = interp.Yes
	// `hash -d` here is not bash's "forget one name": it is the table of
	// **named directories** that `~name` reads back, written as an
	// assignment. `hash -d a=/tmp; print -r -- ~a` is `/tmp`, `hash -d`
	// lists the table sorted by name and `hash -dL` writes the command that
	// would put an entry back. A named directory wins over a *user* of the
	// same name — `hash -d root=/tmp; print -r -- ~root` is `/tmp` (#2191).
	s.HashDefinesANamedDirectory = interp.Yes
	// And lists it in name order, alone in the panel — the other three print
	// their own tables' bucket order, which is a fact about their hashing
	// rather than about the language.
	s.HashListingIsSorted = interp.Yes
	// And `unsetopt hashcmds` really stops the table being filled, which is
	// the same reading bash gives `set +h` and not the one ksh93 gives
	// `trackall`. Measured 2026-09-13: `unsetopt hashcmds; ls >/dev/null;
	// hash` lists nothing here and `setopt hashcmds` starts it again, where
	// ksh93 with `set +o trackall` goes on hashing. The letter `-h` is a
	// different option in this shell and does not reach it.
	s.HashObeysCommandTracking = interp.Yes
	// A `PATH=… cmd` prefix leaves the table alone, the way ksh93's does
	// and bash 5.3's, dash's and ash's do not: the new PATH reaches the
	// child and the search and never this shell's own PATH, so there is no
	// assignment to empty the table. Measured 2026-09-13 with two copies of
	// one name on PATH, the first hashed — the prefixed run takes the
	// second copy and the table still holds the first.
	s.APrefixedPathEmptiesTheCommandHash = interp.No
	s.TildePlusMinusExpands = interp.Yes
	// A colon closes nothing in an ordinary word here — `echo ~:x` is the
	// characters as written, as in dash and BusyBox ash — which is the
	// standard's reading and is inherited rather than set; it is named here
	// because this is the column that departs from those two on the
	// neighboring question above.
	s.UnderscoreTracksTheLastArgument = interp.Yes
	// And a declaration's assignment operands are not arguments to it:
	// `export y=2; echo $_` is `export`. Measured 2026-10-03; see the axis.
	s.UnderscoreStopsAtADeclarationsAssignment = interp.Yes
	// The parameter exists before anything has put a value in it: `${_+x}`
	// is non-empty and `set -u` reads it, even though what it holds is the
	// empty string until the first command runs.
	s.UnderscoreIsAParameterAtAll = interp.Yes
	s.UnderscoreMovesOnlyBetweenInputCommands = interp.No
	// Alone in the panel, this shell writes the call's own last argument
	// into `$_` before the body runs: `: outer` then `f one two` reads `two`
	// on the first line of the body, where bash and ksh93 read `outer`.
	s.UnderscoreMovesBeforeAFunctionBody = interp.Yes
	// And zsh alone lets the text write through: the same probe leaves
	// `inner`, and a sourced file leaves its own last command's argument.
	// Measured 2026-09-23 on zsh 5.9 over a script file.
	s.UnderscoreHoldsTheCallAcrossEvalAndSource = interp.No
	// zsh reads it the way `return` reads everywhere else — the action's
	// own last command. Same probe, entered at 4, answers 123.
	s.TrapReturnStatus = interp.TrapReturnTakesTheHandlersLastStatus
	// And starts it empty regardless, alone in the panel: an exported `_`
	// is discarded rather than carried in, so the parameter says nothing
	// about the invocation until the first command has run.
	s.UnderscoreInheritsFromTheEnvironment = interp.No
	// Bases stop at 36 here, and the refusal says so.
	s.ArithBaseAbove36 = interp.No
	// A base is read in plain decimal and however long it is written:
	// `010#9` is 9 and `0010#5` is 5, so the leading zeros are padding
	// rather than the start of a constant — which they cannot be here in
	// any case, ArithLeadingZeroIsOctal being No.
	s.ArithBaseMayHaveALeadingZero = interp.Yes
	s.ArithBaseIsAtMostTwoDigits = interp.No
	// And zero is a base this shell reads through rather than refuses: the
	// digits after it are an ordinary constant, so `$(( 0#0x10 ))` is 16.
	s.ArithBaseZeroReadsTheDigitsAsWritten = interp.Yes
	// `$(( 0x ))` is 0 and `$(( 0x+1 ))` is 1, as in bash.
	s.ArithEmptyRadixDigitsAreZero = interp.Yes
	s.ArithEmptyBaseDigits = interp.ArithEmptyBaseDigitsZero
	// The one column that reads part of an over-large numeral and says so,
	// on stderr, with the script carrying on:
	// `$(( 10000000000000000000 ))` writes `number truncated after 19
	// digits: 10000000000000000000` and answers 1000000000000000000 (#3202).
	s.ArithNumeralPastTheWord = interp.NumeralPastTheWordKeepsTheDigitsThatFit
	// And the same numeral out of a variable reads identically; only dash
	// parts the two.
	s.ArithStoredNumeralPastTheWordIsRefused = interp.No
	// ${#a} of an array counts elements, and a function's $LINENO counts
	// from the function.
	s.ArrayLengthWithoutSubscriptIsCount = interp.Yes
	// `${#s[@]}` on a scalar is the width of the value, so an empty one is
	// 0 and `a b` is 3 — where every other shell in the panel reads the name
	// as a list of one and answers 1 for both. Not moved by `ksharrays`,
	// measured: the three rows are 0, 1 and 3 under the option as well.
	s.WholeSubscriptOnAScalarMeasuresIt = interp.Yes
	s.FcEmptyHistoryIsAnError = interp.Yes
	// And an editor that emptied the file is an error here too, where bash
	// runs nothing and says nothing. Measured 2026-09-21 with a stand-in
	// editor that truncates what it is handed (#4017).
	s.FcEmptyEditIsAnError = interp.Yes
	// And an event this list cannot reach is refused rather than brought to
	// the nearest end, which is the other shell's answer: `fc -l 99` on five
	// entries is `fc: no such event: 99` at 1 where bash writes all five at
	// 0. Measured 2026-09-21 against zsh 5.9.2, `zsh -f` on a script file
	// under `env -i` with a scratch `HOME`, the list planted with `print -s`
	// (#4018).
	s.FcEventOutOfRangeIsAnError = interp.Yes
	// A relative operand is counted back from this shell's own event number,
	// and a shell reading a script has none — so `fc -l -1`, `fc -l -2` and
	// `fc -l -20` all write the whole list, on five entries and on thirty.
	// The same fact gives the default range: the newest seventeen entries of
	// the list rather than the sixteen events below a current one.
	s.FcRelativeEventNeedsTheShellsOwnEventNumber = interp.Yes
	// And the newest entry is the line this shell is standing on, so the
	// roads that *run* one stop below it and say why: `fc -s 5` on five
	// entries is refused, `fc -s 99` and `fc 99` are refused in the same
	// words rather than named as events, and `fc -s` on a list of one — or
	// of none — is refused too, where on two or more it runs the oldest
	// entry and says nothing. A range's far end is brought under the
	// threshold instead: `fc -e ed 1 5` edits 1 through 4. Measured
	// 2026-09-21 on the same lists as the axis above (#4058).
	s.FcNewestEntryIsTheCurrentLine = interp.Yes
	// And an operand's digits are read past whitespace and a `+`, where
	// bash wants the sign or the first digit at the front of the word:
	// `fc -l ' -1'` and `fc -l ' 2'` and `fc -l +3` are all events here and
	// all `fc: no command found` there. The digit *prefix* itself is not
	// this axis — both shells read `fc -l 2x` as entry 2, and the core does
	// that unasked.
	// And a range whose entries would run newest first is refused on the
	// editor road rather than run that way: `fc -e ed 3 1` and
	// `fc -r -e ed 1 3` are both refused where `fc -r -e ed 3 1` edits 1, 2,
	// 3 at 0, so it is the order they would run in that is judged. `fc -l
	// 3 1` still lists backwards at 0, so it is the running and not the
	// range (#4100).
	s.FcBackwardsRangeIsAnError = interp.Yes
	s.FcNumericOperandSkipsBlanksAndASign = interp.Yes
	// And the same looseness decides where the options end: `fc -l -1x` is
	// the operand `-1x` here and `fc: -1: invalid option` at 2 in bash,
	// which reads a word as an operand only when it is a dash and nothing
	// but digits.
	s.FcOptionsEndAtADashAndADigit = interp.Yes
	s.JobControlAbsenceIsReportedFirst = interp.Yes
	// And the monitor alone is what `fg` and `bg` need. Reachable only with
	// a terminal here, since `set -m` without one is fatal in this shell —
	// measured 2026-09-15 on a pseudo-terminal, where the job line is
	// printed and the status is 0 (#2720).
	// A chain counts through what the link before it named — characters of
	// one string, elements of a list — which is the reading
	// syntax.Dialect.ChainedSubscript was written for and is not ksh93's
	// walk into a nested compound. This shell has no nested compound to
	// walk into (#2830).
	s.ChainedSubscriptReadsANestedValue = interp.No
	s.MonitorAloneResumesAJob = interp.Yes
	// And the one column that *announces* on the monitor alone: a script
	// with `set -m` writes `[1] <pid>` for a `&` job with nobody at a
	// prompt, where the other four write nothing. Measured 2026-09-15 on a
	// pseudo-terminal, which is the only place this shell has a monitor at
	// all (#2838).
	s.MonitorAloneAnnouncesAJob = interp.Yes
	// And it is the one shell that accounts for its jobs on the way out with
	// no prompt anywhere. Measured 2026-09-25 against zsh 5.9.2 on a
	// pseudo-terminal, `zsh -fm` over a script holding `sleep 3 &`: the
	// error stream gets `you have running jobs.` and then
	// `warning: 1 jobs SIGHUPed`, both before the EXIT trap, and `zsh -f`
	// over the same script writes neither. The monitor and not the prompt —
	// an interactive session with `unsetopt monitor` writes neither too
	// (#4542).
	s.MonitorAloneAccountsForJobsAtExit = interp.Yes
	// How a PATH hit is written back, before the command runs and after:
	// measured 2026-10-05 with `command -v`, `PATH=/bin/` is `/bin//ls`, `PATH=./` is `.//zz`, an empty entry is the bare name (#6044). See
	// Semantics.PathHitSpelled.
	// And what the hit is started by: measured 2026-10-05, `PATH=.` starts `z0`, though `command -v` writes `./z0`
	// (#6090). See Semantics.PathHitFromTheCurrentDirectoryRunsBare.
	// A remembered relative path written under `./` where the table answers,
	// and a table that holds only absolute entries: measured 2026-10-05
	// (#6069). See Semantics.HashedRelativePathReportedUnderDot and
	// Semantics.CommandTableHoldsOnlyAbsoluteEntries.
	// Whether `hash name` searches PATH again for a name already remembered
	// (#6110). See Semantics.HashNameSearchesAgain.
	// What waives a held exit for the exit after it (#6080). See
	// Semantics.HeldExitWaivedForTheNextCommandOnly.
	s.HeldExitWaivedForTheNextCommandOnly = interp.No
	s.HashNameSearchesAgain = interp.No
	s.HashedRelativePathReportedUnderDot = interp.No
	s.CommandTableHoldsOnlyAbsoluteEntries = interp.Yes
	s.PathHitFromTheCurrentDirectoryRunsBare = interp.Yes
	s.PathHitSpelled = interp.PathHitAsWritten
	// And the prompt is not enough without it: `set +m` at a session, or
	// `-i` on a pipe where the monitor cannot be turned on, and `exit` with a
	// running job leaves at once, says nothing, and hangs nothing up — the
	// job was still running afterwards. Measured 2026-10-05 (#6028).
	s.MonitorOffSilencesJobsAtExit = interp.Yes
	// A stopped job holds the exit back: the shell says so and stays,
	// and the next attempt leaves. Measured through a pseudo-terminal for
	// `exit` and for ^D alike.
	s.StoppedJobsHoldTheExit = interp.Yes
	// And says only the sentence: measured, zsh writes `you have running
	// jobs.` and draws the next prompt, with no table under it, whichever way
	// `checkjobs` and `checkrunningjobs` are set.
	s.HeldExitListsTheJobs = interp.No
	// `setopt hup` is on in a fresh zsh and needs no login shell: measured
	// 2026-09-25 on a pseudo-terminal with the shell started `-fiV +Z`, an
	// interactive session that is not a login shell hangs its running job up
	// and writes `zsh: warning: 1 jobs SIGHUPed`. A stopped job is left out
	// of the send — one running beside one stopped is `1 jobs SIGHUPed`, two
	// stopped is silence, and the stopped process reads `SN` afterwards
	// rather than `T`, so this shell continued it rather than hanging it up.
	// And all of it happens before the EXIT trap: with `trap 'echo
	// EXIT_TRAP' EXIT` the warning comes first. Each row is on its axis.
	s.HangupAtExitNeedsALoginShell = interp.No
	s.HangupAtExitSkipsStoppedJobs = interp.Yes
	// And a job that started before `set -m` is neither sent to nor counted:
	// `sleep 1 & set -m; sleep 1 & echo x` under `-c` on a terminal is
	// `warning: 1 jobs SIGHUPed`. Measured 2026-10-05 (#6035).
	s.HangupAtExitSkipsJobsStartedWithoutTheMonitor = interp.Yes
	s.HangupAtExitPrecedesTheExitTrap = interp.Yes
	// CDPATH moves in silence here.
	s.CdpathAnnouncesTheDirectory = interp.No
	// And CDPATH is a search beside the ordinary relative lookup, with the
	// fallback POSIX gives it (#2896).
	s.CdpathReplacesTheRelativeLookup = interp.No
	// And so does `autocd`: `setopt autocd` then `subdir` at a prompt moves
	// and writes nothing, where bash writes `cd -- subdir` first. Measured
	// 2026-09-08 through a pseudo-terminal against zsh 5.9.2 started `-f`.
	s.AutoCdAnnouncesTheSubstitution = interp.No
	s.LinenoCountsFromTheFunction = interp.Yes
	// A substring range is this shell's history-modifier syntax as well, and
	// a segment beginning with an unquoted letter is read as a modifier
	// rather than as an arithmetic offset — so `${x:i:2}` is refused where
	// the other three take a substring. Measured against zsh 5.9.2.
	s.SubstringRangeReadsModifiers = interp.Yes
	s.SubstringCountsBytes = interp.No
	// A subscript's expanded text is handed back to the bracket scanner
	// here, so a key holding `]` or `[` is read as syntax rather than as
	// the string a key is: with `key='x],b['` already stored, `(( m[$key]++
	// ))` is `not an identifier: b[]` at status 2 and the element is left
	// where it was, where bash and ksh93 both increment it. The `b[` in
	// that sentence is the tail of the key being named as a second array.
	// Measured 2026-09-13 against zsh 5.9.2 (#2581).
	s.ArithSubscriptRereadsItsExpandedText = interp.Yes
	// And a subscript whose quotation never closes is still the key it
	// looks like: measured 2026-09-20 on 5.9.2, `typeset -A a; k="q'r";
	// a[$k]=4; let "++a[$k]"` leaves 5 under the three-character key, with
	// ksh93u+ and against bash 5.3.20, which calls the subscript bad.
	//
	// Unreachable from this preset and answered anyway: no quoting is read
	// inside a subscript here — syntax.Dialect.ArithSubscriptQuoting is off
	// — so no scan has a quotation to give up on. Answered for the reason
	// ArithWholeArraySubscriptIsReportedAsBad above is: an unanswered axis
	// is a refusal, and a reading this dialect cannot reach must not be
	// able to produce one (#3796).
	s.ArithSubscriptQuotationMustClose = interp.No
	// A `let` operand's subscript is not a quoting context here either, and
	// that is measured rather than inherited from the line above: on zsh
	// 5.9.2 with `typeset -A a; a[k]=1; a['"k"']=2`, `let '++a["k"]'`
	// leaves the bare `k` at 1 and the quoted key at 2, so it stored under
	// a third key spelled with the quotes — which is what
	// SubscriptIsAQuotingContext already answers for every other subscript
	// of this preset.
	//
	// It is a pin, exactly as the axis above is: with no subscript here
	// ever a quoting context, the builtin route cannot part from the
	// expression's, and the answer exists so that route cannot reach an
	// unanswered axis (#3871).
	s.ArrivedSubscriptIsAQuotingContext = interp.No
	// And an apostrophe written inside a subscript stops nothing here
	// either: measured 2026-09-20 on 5.9.2 from a script file,
	// `typeset -A m; kq=q; (( m['$kq'] = 42 ))` stores under `'q'` — the
	// expansion performed and the apostrophes kept, which is
	// SubscriptIsAQuotingContext's `no` showing through beside this one
	// (#3942).
	s.WrittenSubscriptQuotationStopsItsExpansion = interp.No
	// And the expansion being performed does not end the key: measured in
	// the same run, `(( m[q'$kq'z] = 42 ))` is `q'q'z` here — every
	// character kept, quotation included — where ksh93u+ 2012-08-01 keeps
	// only `q`. A reading of its own rather than a pin: this column
	// performs the expansion, so it reaches the question (#3968).
	s.SubscriptQuotationEndsTheKey = interp.No
	s.EchoInterpretsEscapes = interp.Yes
	// `cdablevars` reads the variable as a named directory. Measured
	// 2026-10-02 on 5.9.2 (#5155).
	s.CdVariableIsANamedDirectory = interp.Yes
	// A `~name` naming nothing is refused, as an unmatched pattern is.
	// See the field (#5646).
	s.UnresolvedTildeIsAnError = interp.Yes
	// `~N` reads $dirstack with $PWD in front of it. See the field (#5656).
	s.NumberedTildeReadsThePushedDirectories = interp.Yes
	// And only a bare numeral of one or two characters is one; see the
	// field (#5694).
	s.ABareNumberedTildeOfThreeDigitsIsAName = interp.Yes
	// A failed expansion in a declaration's words or a `case` leaves `$?`
	// alone. See the field (#5657).
	s.FailedExpansionInADeclarationOrCaseSetsNoStatus = interp.Yes
	// And a typed declaration whose own evaluation fails ends the script at
	// 0: see interp.Semantics.DeclaredTypedValueFailureLeavesZero (#5677).
	s.DeclaredTypedValueFailureLeavesZero = interp.Yes
	// echo reads -n, -e and -E, and -e wins over -E whatever the order.
	s.EchoOptions = "neE"
	s.EchoLastEscapeFlagWins = interp.No
	s.EchoExpandsHexEscapes = interp.Yes
	// `echo '\u0041'` and `echo '\U00000041'` are both `A` here, with no
	// `-e` needed, and a hexadecimal escape with no digit after it is a NUL
	// followed by whatever was there: `\xZ`, `\uZ` and a bare `\x` are all
	// `00` and then the rest, where bash leaves the two characters standing.
	s.EchoExpandsUnicodeEscapes = interp.Yes
	// And a code point the locale cannot hold is refused rather than written
	// back: this shell reports `character not in range`, writes what came
	// before the escape, and abandons the script with the status it already
	// had. Measured 2026-09-11 under `LC_ALL=C` (#1851).
	s.UnicodeEscapeOutsideTheLocale = interp.OutsideLocaleEscapeRefused
	// And the radix character, where this shell is the panel's third answer:
	// it writes the locale's radix and reads a number at **either** that or
	// the point. Measured on macOS 15 with the host's own `de_DE.UTF-8`,
	// `printf '%.4f' 1` is `1,0000` and `printf '%.2f' 1.5` is `1,50` where
	// bash and ksh93 call the operand invalid. See
	// interp.Semantics.NumberRadix.
	s.NumberRadix = interp.RadixIsTheLocalesOwnOrThePoint
	// A value above what six bytes hold is still encoded here, the arithmetic
	// overflowing into the lead byte: measured 2026-09-22 under
	// `LC_ALL=en_US.UTF-8`, `printf '%s' $'a\UFFFFFFFFb'` is
	// `61 ff bf bf bf bf bf 62` in 5.9.2, where bash and ksh93 write `a b`.
	// The lead byte is one no decoder will take back, which is the shape of
	// the disagreement rather than an accident of it.
	s.CodePointPastSixBytesIsEncoded = interp.Yes
	// And an *unset* locale is the C locale here, on every operator that
	// reads one: under `env -i`, 5.9.2 answers 6 for `s=héllo; echo ${#s}`,
	// leaves `${(U)s}` on `café` as `CAFé`, and refuses the escape above the
	// way it refuses it under `LC_ALL=C`. bash 5.3.15 answers 5, `CAFÉ` and
	// the encoded character to the same three (#2020).
	s.UnsetLocaleIsUnicodeAware = interp.No
	// And the decoder does not reach the reader that takes the program text:
	// this shell counts a multibyte character everywhere it measures a string
	// and still walks its input a byte at a time. Measured 2026-09-23 under
	// `LC_ALL=zh_TW.Big5` with the Big5 spelling of U+03B1, whose second byte
	// is a backslash — `x=α` is `command not found: x=`, the trail byte having
	// joined the next line, `x="α"` is `unmatched "`, and a here-document body
	// holding `α$v` writes `$v` as text. bash 5.3.20 and ksh93u+ take the
	// character in all three.
	//
	// `read` is the other way round here and is a separate axis: see
	// interp.Semantics.ReadTakesAMultibyteCharacterWhole, which this shell
	// answers Yes by inheriting the preset.
	s.MultibyteCharacterIsReadWhole = interp.No
	s.EchoEmptyHexDigitRunIsNul = interp.Yes
	// `\e` is the escape character here and `\E` is two characters — the
	// opposite of ksh93.
	s.EchoExpandsEscEscape = interp.Yes
	s.EchoExpandsCapitalEscEscape = interp.No
	// read takes -r and -s, -A with the array as the first operand, and the
	// same -d, -t and -u as the others — but no counts: -N is a bad option
	// here and -n is a flag it reads and, outside completion widgets, acts
	// on not at all. -p is a bare flag too — the coprocess, not bash's
	// prompt — and with zsh's `coproc` outside this grammar there is never
	// one to read: the letter always answers `-p: no coprocess` and 1, the
	// variables untouched. zsh's -t may also stand alone as a poll; that
	// spelling is not modeled, so here it reads the word after it as its
	// seconds.
	//
	// `-q` reads one key from the same terminal `-k` reads and answers
	// whether it was a yes — 0 for `y` or `Y`, 1 for anything else, with the
	// name left holding the normalized `y` or `n` rather than the key. It is
	// zsh's letter alone: measured 2026-09-26, bash 5.3.20 and 3.2.57 call it
	// `invalid option`, ksh93u+ `unknown option` and dash `Illegal option`,
	// all at 2, so nothing here needs an axis. interp/readkeys.go carries the
	// rest of what was measured.
	// `-e` and `-E` are this shell's **echo** letters and not bash's line
	// editor: the values that would be assigned go to standard output, one
	// line per name, and `-e` writes none of them to the names. See
	// ReadEchoLettersWriteTheValues below, where the measurement is, and
	// note that they left UnimplementedOptionLetters in the same change —
	// the two tables move together (#4963).
	//
	// `-z` is the third letter that left it, and reads no input at all: it
	// takes one entry off the editor buffer stack, which `print -z` fills.
	// See editorbuffer.go (#4966).
	s.ReadOptions = "rsnpAd:t#u:k#qeEz"
	// Measured 2026-10-01: every NUL a `read` or a substitution meets is kept as a byte of the value. See Semantics.NulInAValue.
	s.NulInAValue = interp.NulKept
	// And what the echo pair means, which is the half a letter in the
	// accepted set does not say. Yes: measured 2026-09-28 under `-f`,
	// `printf '  a   b   c  \n' | read -E x y` writes `a` and `b   c` and
	// assigns both, the same line under `-e` writes them and leaves
	// `${+x}${+y}` at `00`, and `read -eE x` and `read -Ee x` both leave
	// `x` unset.
	s.ReadEchoLettersWriteTheValues = interp.Yes
	// `unset -m` reads its operands as patterns, which is this shell's
	// alone; `-n` is not here, and that is measured rather than an
	// omission — `unset -n x` is `bad option: -n` in zsh 5.9.2 where bash
	// 5.3 and ksh93 take it.
	// unanswered NamerefCycleIsRefused: this shell has no name references to
	// make a cycle of, which is measured rather than assumed — #2553's report
	// named `typeset -n` among its three spellings and it is not one.
	// Measured 2026-09-15 on zsh 5.9.2, `env -i` with a scratch HOME, inside
	// a function so all three words are reachable: `typeset -n v=1`,
	// `declare -n v=1` and `local -n v=1` are each `bad option: -n`.
	// unanswered NamerefLetterStandsAlone: there is no `n` letter to write
	// beside another one. Measured 2026-09-18 on zsh 5.9.2, `typeset -ni
	// r=v` is `typeset: bad option: -n` at 1 — the complaint names the
	// missing letter and never reaches a question about the pair (#3171).
	// unanswered ContainerWriteThroughAReferenceNamesTheReference: there is no
	// reference to write through (#3173).
	// unanswered DeclarationThroughAReferenceNamesTheOperand: there is no
	// reference for a declaration to be redirected through (#3173).
	// unanswered ExportOrReadonlyTakesAReferenceToAnElement: the same missing
	// letter, and the `(P)` flag this shell does have is an expansion rather
	// than a name, so neither builtin can be handed a reference at all.
	// Measured 2026-09-20 on zsh 5.9.2, `a=(p q r); typeset -n b='a[1]'` is
	// `typeset: bad option: -n` at 1 (#3881).
	// unanswered NamerefTargetResolvedWhenAimed: the same missing letter —
	// there is no reference for a target to be settled on. Measured
	// 2026-09-18, `typeset -n r=a[1]` is `typeset: bad option: -n` at 1
	// (#3124).
	// unanswered ArrayLetterOverAnUnaimedReferenceDropsIt: the `n` letter is
	// the missing one again, so there is no unaimed reference for an array
	// letter to land on. Measured 2026-09-22, `typeset -n foo` is `typeset:
	// bad option: -n` at 1 and `typeset -a foo` after it is an ordinary
	// array (#4178).
	// unanswered NamerefArrayRefusal: the letter is the same missing one, so
	// there is no declaration for an array to be refused under. Measured
	// 2026-09-16, `r=(a b); typeset -n r=v` is `typeset: bad option: -n` at 1
	// and the array is untouched (#3103).
	// unanswered NamerefDroppedByASubscriptedOperand: the same missing
	// letter, so there is never a reference for a subscripted operand to
	// take an attribute off. Measured 2026-09-23, `typeset -n xref` is
	// `typeset: bad option: -n` at 1 and `typeset -a xref[1]=one` after it
	// writes an ordinary array (#4178).
	// unanswered UnsetReferenceLetterRemovesANonReference: `-n` is not one of
	// this shell's letters. Measured 2026-09-12, `unset -n x` is
	// `unset: bad option: -n` at 1 and `x` keeps its value (#932).
	s.UnsetOptions = "vfm"
	// `readonly` here is `typeset -r` under another name, and since #4853 it
	// is that in the code as well — see ReadonlyWord just below. So the set is DeclareOptions less the three letters the word has
	// already spent or never had, and every letter in it reaches the
	// attribute `typeset` gives it rather than being accepted and ignored.
	//
	// Measured 2026-09-27 a letter at a time, one run per cell, from a script
	// file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME,
	// asking zsh 5.9.2 for all fifty-two letters under each word:
	//
	//	readonly takes  a f g h i l p t u x A E F H L R T U Z
	//	typeset takes   those, plus k m r z
	//
	// `r` is the letter this word *is*, so `readonly -r vv` and `readonly +r
	// vv` are each `bad option` there; `m` and `z` are `typeset`'s alone; and
	// `k` is in neither set here, since this shell has not built it — it sits
	// in UnimplementedOptionLetters under `typeset` and is a plain bad option
	// under this name, which is what zsh answers too.
	//
	// The old set was `paAft`, five letters, chosen on the rule that a letter
	// accepted and then ignored hands a script a success it did not earn.
	// That rule is why the set could not be widened while this builtin had an
	// operand loop of its own; it is not a reason to keep it narrow now that
	// the loop is the declaration's (#4853).
	//
	// unanswered ReadonlyReferenceLetter: `readonly -n` is refused here, so
	// the letter never reaches the axis. Measured 2026-09-18 under
	// `env -i PATH=/usr/bin:/bin LC_ALL=C`: `readonly -n zz` is
	// `readonly: bad option: -n` at 1. ReadonlyOptions has no `n`, which is
	// what keeps the question off this column rather than answered wrongly.
	s.ReadonlyOptions = "aAEfFgHhiLlpRtuUTxZ"
	// And the word is the declaration itself, not a freeze that happens to
	// spell a few of the same letters. Measured in the same run: every shape
	// the two words share answers alike, the complaint name apart —
	// `readonly -p v`, `readonly -f b`, a bare `readonly`, `readonly +`,
	// `readonly "a[1]"`, `readonly -L 5 v=ab`, `readonly -T TT tt` and
	// `f(){ readonly -h v=1 }` are byte-identical to their `typeset -r`
	// spellings. See Semantics.ReadonlyWord.
	s.ReadonlyWord = interp.ReadonlyWordIsTheDeclaration
	// And it is `typeset -r` in the other half too: a `readonly` written
	// inside a function declares a **local**, where every other shell in
	// the panel freezes the name the shell already has. Measured
	// 2026-09-15 over a script file, `b() { readonly B=1; }; b` leaves
	// `B` unset here and at 1 in bash, ksh93, dash and BusyBox ash —
	// including ksh93's keyword form, which is what keeps this apart from
	// TypesetLocalNeedsKeywordFunction. `export` is not the same word and
	// is not local here: it is `typeset -gx`, and the `g` is the whole
	// difference.
	s.ReadonlyDeclaresALocal = interp.Yes
	s.ReadZeroTimeout = interp.ReadZeroTimeoutFinishesWhatItStarted
	// `read -A` with no name fills the array `reply`, which is the array
	// neighbor of the `REPLY` a bare `read` fills and deliberately not the
	// same parameter: the shell is case-sensitive about which. Measured
	// 2026-09-26 under `-f`, with the other one pre-set on each row so that
	// neither reads back what it had a moment earlier — `reply=(x y); read
	// -A <<<'hello world'` leaves `reply=(hello world)` and never creates
	// `REPLY`, and `reply=scalar; read <<<'plain'` leaves `reply` alone and
	// puts the line in `REPLY`.
	s.ReadArrayDefault = interp.ReadArrayDefaultIsTheArrayReply
	// The words splice over the element as `b[2]=(p q)` does: `( x p q z )`.
	// See interp.Semantics.ReadArrayIntoAnElementRefillsTheArray (#6869).
	s.ReadArrayIntoAnElementRefillsTheArray = interp.No
	// And a *second* operand is a line this shell declines: `read -A a b`
	// is `only one array argument allowed` at 1 with nothing read, where
	// the other shell with the letter fills the first name and clears the
	// rest (#4622).
	s.ReadArrayTakesOneNameOnly = interp.Yes
	s.ReadTimeoutKeepsWhatArrived = interp.No
	s.ReadTimeoutBoundsReadability = interp.Yes
	// And that wait, and only that one, is ended by a trapped signal as the
	// deadline would end it: `read -r -t 5 l` on a fifo with nothing in,
	// INT trapped and sent 0.3 s in, is `T`, 1, `l` untouched and the line
	// written after it the next read's, where a `read` without `-t` reads
	// on through the same signal. Measured 2026-10-29 (#7032); see
	// interp.Semantics.ReadTimeoutWaitEndedByATrappedSignal.
	s.ReadTimeoutWaitEndedByATrappedSignal = interp.Yes
	// The handler of a trapped signal that arrives during a `read` that goes
	// on runs at the arrival, wherever the read is — waiting, or taking a
	// file a byte at a time — and the read then goes on. Measured 2026-10-31
	// on 5.9.2; see interp.ReadHandlerMoment (#7031).
	s.ReadHandlersRun = interp.ReadHandlersRunAsSoonAsTheyArrive
	// And a handler's `return` there ends the read as at the end of its input
	// before the function returns: `ab` in, `return 5` mid-read is `out st=1
	// l=[ab]`, the read's 1 whatever the `return` said, and the name is
	// cleared where nothing had arrived. Measured 2026-10-31 on 5.9.2; see
	// interp.Semantics.ReadLeftByAHandlersReturnAssignsWhatArrived (#7176).
	s.ReadLeftByAHandlersReturnAssignsWhatArrived = interp.Yes
	// A signal another process sends while a command substitution runs has
	// its handler run before the rest of the command's words: `echo Y
	// "$(/bin/kill -INT $$)" "$(echo E >&3)"` writes `T` before `E`, and the
	// handler sees the status from before the substitution. zsh runs it as
	// soon as it arrives, while the body is still running, which this shell
	// only meets as the substitution ends (#7224). Measured 2026-10-31 on
	// 5.9.2; see interp.Semantics.HandlersRunAsASubstitutionEnds (#7175).
	s.HandlersRunAsASubstitutionEnds = interp.Yes
	s.ArithLeadingZeroIsOctal = interp.No
	// Nothing makes a leading zero octal here, so `let` and `(( ))` read one
	// alike and the axis that parts them elsewhere has nothing to change.
	s.LetReadsALeadingZeroAsDecimal = interp.No
	// And the same for the assignment reader, which is reachable here only
	// under `setopt octal_zeroes` — the one switch in the panel that moves
	// the axis above, so this shell is the only column where the question is
	// asked of a *running* shell rather than of a preset. Measured 2026-09-17
	// on zsh 5.9.2 with the option set: `typeset -i d=010` and
	// `e=010; typeset -i e` both leave `8#10`, which is eight written in the
	// base it was read in, so the assignment goes through the expression
	// reader's octal rule rather than around it. ksh93 is the column that
	// answers otherwise, and this one does not join it (#2884).
	s.IntegerAssignmentReadsALeadingZeroAsDecimal = interp.No
	// A name an arithmetic assignment *creates* is an integer here, which
	// outlives the expression: `(( x = 5 )); x=2+3` is 5 where the same two
	// commands leave the three characters `2+3` everywhere else. Only a name
	// it creates — `x=3; (( x = 5 ))` leaves an ordinary scalar.
	s.ArithmeticAssignmentDeclaresANumber = interp.Yes
	s.FatalErrorStatusIsOne = interp.Yes
	// Except for one refusal, which leaves 0 behind when the program came
	// from an argument rather than from a file. Measured on every
	// neighboring refusal too, and they all leave 1 — see the axis.
	s.SetArrayBadNameLeavesZero = interp.Yes
	s.StoreRefusalOfADeclaredElementLeavesZero = interp.Yes
	// And a refused store through `printf -v`'s operand, which parts from
	// the identical refusal through `read`'s here: measured 2026-09-17,
	// `a=(1 2 3); ( printf -v "a[1/0]" X ); echo $?` is 0 and
	// `( read "a[1/0]" < in.txt )` is 1, by both routes, and the empty
	// subscript answers the same way at each.
	s.StoreRefusalThroughPrintfLeavesZero = interp.Yes
	// And a refused whole-array store through `set -A` or `read -A`, which
	// parts from `read`'s scalar store the same way: measured 2026-10-15,
	// `-c 'typeset -r v=1; read -A v <<< x'` exits 0 and the same with `read
	// v` exits 1, and so does `set -A HOME x` against a bare `HOME=(x)`
	// (#6486).
	s.ArrayStoreRefusalThroughSetOrReadLeavesZero = interp.Yes
	s.HeredocExpandsInTheCommandsProcess = interp.Yes
	// And a `( … )`'s body with it: `n=0; ( cat ) <<END` with `$(( n+=5 ))`
	// in it leaves `n` at 0 here, where the same body on a group, a
	// function, a builtin or a loop leaves 5. A second axis rather than a
	// reading of the line above because ksh93 answers the two rows the other
	// way round — see the field's own panel (#4700).
	s.HeredocBodyOnASubshellExpandsInTheSubshell = interp.Yes
	s.RedirectTargetExpandsInTheCommandsProcess = interp.Yes
	// And a subshell's, which is where this shell had it wrong: `( : ) >
	// "${u:=made}"` leaves `u` unset here, and `( echo RAN ) > $(( 1/0 ))`
	// writes one line, is caught by `||`, leaves 1 behind and runs the rest
	// of its own line, where a group with the same target ends the shell
	// (#4695).
	s.RedirectTargetOnASubshellExpandsInTheSubshell = interp.Yes
	// A *target* this shell expanded itself and could not is this shell's
	// own failed expansion rather than a failed redirection, which here is
	// fatal whatever it was written on — where `: < /nonexistent/f` and
	// `read x < /nonexistent/f` both carry on at 1. Measured 2026-09-26 on
	// zsh 5.9.2 under `-f` (#4689).
	s.RedirectTargetFailureIsTheRedirections = interp.No
	// And a body this shell expanded itself and could not is its own failed
	// expansion, which here is fatal: `: <<END`, `read x <<END`, a function
	// and a group all end the shell at 1 over a body of `$(( 1/0 ))`, where
	// a file that will not open on any of them carries on at 1 and is caught
	// by `||`. Measured 2026-09-26 on 5.9.2 under `-f` (#4684).
	s.HeredocBodyFailureIsTheRedirections = interp.No
	s.ArithNameValueRecurses = interp.Yes
	// And a fixed depth is what stops it: a chain of sixty distinct names
	// ending in a number is `math recursion limit exceeded`. Measured
	// 2026-09-18 (#3416).
	s.ArithRecursionBound = interp.ArithRecursionBoundedByDepth
	// `typeset +A` takes the attribute off and leaves the name an empty
	// scalar, at status 0 and with nothing said. See
	// interp.Semantics.ArrayAttributeRemoval (#4241).
	s.ArrayAttributeRemoval = interp.ArrayAttributeRemovalEmptiesTheName
	// A table literal mixing `[key]=` heads with bare words is refused whichever
	// way round it was written, and the script is given up. An empty key is a
	// key like any other here. See interp.Semantics.MixedTableLiteral and
	// .EmptyKeyInATableLiteral (#4241).
	s.MixedTableLiteral = interp.MixedTableLiteralRefused
	s.EmptyKeyInATableLiteral = interp.EmptyKeyInATableLiteralAccepted
	// And an unset name found that way is a zero like any other unset name:
	// `x=abc; $((x+1))` is 1 and the script runs on. Measured 2026-09-11 —
	// ksh93 is the panel's holdout, where it is a fatal `parameter not set`.
	s.ArithRecursedNameMustBeSet = interp.No
	s.ArithSubscriptNameMustBeSet = interp.No
	// With nounset on, though, an unset name an expression reads is refused
	// as it is in bash and ksh93: `set -u; : $((b))` is `b: parameter not
	// set`. Not fatal of itself — the construct answers, so `(( b ))` leaves
	// 2 and the line runs on where `$(( b ))` in a word stops the shell
	// (#3574).
	s.ArithUnsetNameUnderNounsetIsRefused = interp.Yes
	s.ArithNounsetRefusalIsFatal = interp.No
	s.BraceExpansion = interp.Yes
	// The fan copies the names and not the work: `i=0; echo {x,y,w}$((i++))`
	// is `x0 y0 w0` with `i` left at 1 on zsh 5.9.2, and `echo {x,y}$(echo
	// TICK >&2; echo z)` writes TICK once for the two names. The words are
	// the same either way, so the count and the variable are the whole of
	// the tell — see the panel on the axis (#4694).
	s.BraceFanExpandsEachNameOnItsOwn = interp.No
	// What the braces produced goes back into the word the parse cut rather
	// than being read again as text, so `var=baz; varx=vx; echo $var{x,y}`
	// is `bazx bazy` and `printf '[%s]' {Z..a}` keeps the backslash it
	// counted — bash answers `vx vy` and an empty element.
	s.BraceOutputRereadAsText = interp.No
	// Agrees with bash on where the scan resumes after a group that did not
	// expand: one byte past its open brace, so `{a{b,c}}` is `{ab} {ac}`
	// and `@{x}{a,b}@` is two words.
	s.BraceRescanEntersFailedGroup = interp.Yes
	// And agrees with bash on where the braces are found, which is the
	// word as written: `e=a,b; echo {$e}` is `{a,b}` here too, even though
	// a range's endpoints are read after their expansions below.
	s.BraceBodyReadAfterExpansion = interp.No
	// An alternative that came to nothing is still a field: `f {a,}` is
	// `[a] []` here and `[a]` in bash, and an alternative an expansion
	// emptied answers the same way — `unset u; f {a,$u}` is `[a] []`,
	// `f {,}` is two empty fields. ksh93 is the other column that keeps
	// it; #4800 filed the row as ksh93's alone and this column was
	// measured the same way it answers (#4800).
	s.BraceEmptyAlternativeIsAField = interp.Yes
	// A brace does not end field splitting here: under `shwordsplit`,
	// `IFS=:; v=a:b; f x{p,q}$v` is the three fields `xpa`, `xqa` and `b`,
	// and `e='a b,c'; f {$e}` is `{a` and `b,c}` — the splitter reaches
	// inside the group. ksh93 is the one column that stops.
	s.BraceStopsFieldSplitting = interp.No
	// unanswered BraceFreesProducedGroupSyntax: zsh cannot be asked it for
	// the reason below — it never matches an expansion's result against the
	// filesystem, so there is no pattern behind the brace whose group syntax
	// could be freed.
	// unanswered BraceMakesAProducedStarOrBracketText: zsh cannot be asked
	// it. The axis is about what a brace takes from a pattern an expansion
	// produced, and this shell never matches an expansion's result against
	// the filesystem at all — GlobExpansionResults is No here, so with `za`
	// in the directory `g='*'; f z$g` is the single field `z*` with no brace
	// anywhere in the word. There is nothing for a brace to take away.
	// A produced brace is data here: `e='{a,b}'; echo $e` is `{a,b}`,
	// `e='{'; echo $e{a,b}` is `{a {b` and `a=("x{p" "q}y"); echo ${a[@]}`
	// is the two words it was. ksh93 is the one column that reads them.
	s.BraceScanReadsProducedText = interp.No
	// Pads like bash — `{01..3}` is `01 02 03` — but a negative step
	// reverses the walk the endpoints chose: `{3..1..-1}` is `1 2 3` and
	// `{1..10..-4}` is `9 5 1`, bash's `1 5 9` backwards rather than the
	// `10 6 2` swapped endpoints would give.
	s.BraceRangePadsToEndpointWidth = interp.Yes
	s.BraceRangeStepPadsTheRange = interp.Yes
	s.BraceBodyIsACharacterClass = interp.No
	s.ZeroSubscriptIsTheFirstElement = interp.No
	// What a backslash in a substring range leaves; see the axis.
	s.RangeOperandBackslash = interp.RangeBackslashKeptBeforeText
	// zsh 5.9.2 keeps the export and type letters of its own parameters through `unset`; see the axis for the table.
	s.UnsetKeepsTheShellsOwnAttributes = interp.Yes
	// zsh 5.9.2 under shwordsplit drops the edge an expansion leaves at the end of a substituted word; see the axis.
	s.SubstitutedWordKeepsAnExpansionsEdges = interp.No
	// zsh 5.9.2 execs the last command of a `-c` string wherever it is; see interp.TailExec.
	s.TailExec = interp.TailExecWhereverLast
	s.BraceRangeStepSignHonored = interp.No
	s.BraceRangeNegativeStepReverses = interp.Yes
	// And the endpoints are read after the expansions in them: `n=3;
	// echo {1..$n}` is `1 2 3` where bash prints the literal `{1..3}`.
	s.BraceRangeEndpointsExpanded = interp.Yes
	// A range between two single characters spans whatever those characters
	// are — `{1..x}` is seventy-two words and `{α..γ}` is three — and takes
	// no step, so `{a..z..2}` is the word as written where bash counts
	// `a c e …`. And a body shaped like a numeric range that holds no range
	// loses its *braces* rather than standing whole: `{1..}` is `1..`,
	// `{..3}` is `..3`, and a written step of zero goes the same way.
	s.BraceCharRangeSpansAnyCharacter = interp.Yes
	s.BraceLetterRangeMayCrossCase = interp.Yes
	s.BraceRangeMissingEndCountsFromZero = interp.No
	s.BraceRangeZeroStepCountsAsOne = interp.No
	s.BraceRangeNumberMayCarryAPlus = interp.No
	s.BraceRangeThatCannotBeCounted = interp.BraceRangeFailureDropsTheBraces
	s.BracketCaretNegates = interp.Yes
	s.EqualsExpansion = interp.Yes
	s.LastPipelineElementInCurrentShell = interp.Yes
	// A process substitution may *not* stand as a condition's operand. The
	// word is read and then refused, in a sentence of this shell's own and
	// at status 2 — and the command is not started, which is the half a
	// refusal that came after the expansion would get wrong. Measured
	// 2026-09-05: `[[ x == <(x) ]]` is `process substitution <(x) cannot be
	// used here` here and runs the command in bash.
	// An empty right operand is refused, as in bash, though with this
	// shell's own wording and status: `failed to compile regex: empty
	// (sub)expression`, status 1. It is the engine underneath showing
	// through — POSIX ERE has no empty expression — and it is visible in
	// the shipped `regexp-replace`, where an accepted empty pattern would
	// write the replacement between every pair of characters (#2043).
	s.EmptyRegexOperandIsAnError = interp.Yes
	// `[[ subject -regex-match expression ]]`, which is zsh/regex's one
	// feature and is the `=~` question under another name — measured byte
	// for byte over eleven expressions, agreeing on the status, on `$MATCH`,
	// on `$match`, on `$MBEGIN`, on `$MEND` and on the refusal of an
	// expression that will not compile. See the axis (#4739).
	s.RegexMatchCondition = interp.Yes
	// unanswered RegexMatchSurvivesAFailedMatch,
	// RegexMatchOmitsGroupsThatDidNotMatch,
	// PatternMatchWritesTheMatchRecord: this shell keeps its captures
	// through the reporting parameters a pattern flag fills rather than
	// through a named array, so the record those three axes are about is never
	// written here and none of the three questions is put.
	// TestARegexMatchReportsWhatItMatched pins the shape it does
	// have.
	s.ProcessSubstitutionInCondition = interp.No
	// A substitution's body reads what the *shell* is reading, and this shell
	// is alone in it. `printf "PIPE\n" | cat <(cat)` with the shell's own
	// input a file holding OUTER prints OUTER here and PIPE in bash and
	// ksh93 — the body takes the input the element's pipe replaced, because
	// that pipe is a redirection of the element and the words were expanded
	// before it was applied. `=(cat)` in the same place answers the same,
	// which is the whole reason #1933 was filed before the file form landed.
	s.ProcessSubstitutionBodyReadsTheShellsInput = interp.Yes
	// A condition operand that is not an expression abandons the input —
	// `echo one; [[ 1+ -eq 0 ]]; echo two` writes `one`, complains, and
	// never reaches `two`. Note it is `[[ ]]` alone: the same expression
	// in `(( ))` complains and the shell goes on.
	s.ConditionArithmeticErrorIsFatal = interp.Yes
	// And the condition runs to its end first, every later arithmetic in it
	// reading 0 and every later word read as written, so the shell ends at
	// the condition's own status: `[[ ! -v 'a[1/0]' ]]` ends it at 0, and so
	// does `[[ -z ${(P)'a b'} ]]`, whose operand would not expand. See
	// interp.Semantics.ConditionFinishesAfterAFailure (#5588, #6722).
	s.ConditionFinishesAfterAFailure = interp.Yes
	// And an `&&` behind such a condition ends the line at 1, where the
	// condition alone ends a command string at 0: `[[ 1/0 -eq 1 ]] && :` is
	// 1 under `-c`, measured 2026-10-03. See
	// interp.Semantics.AndAfterAFinishedConditionFails.
	s.AndAfterAFinishedConditionFails = interp.Yes
	// `return 3abc` in a function ends the line and not only the call.
	// Measured 2026-10-03. See
	// interp.Semantics.ReturnOperandArithmeticErrorIsFatal.
	s.ReturnOperandArithmeticErrorIsFatal = interp.Yes
	// And a C-style `for` header. Not `(( ))`, which zsh reports and carries
	// on from — the two are a field apart for exactly that reason. Measured
	// 2026-09-13; see [interp.Semantics.ForHeaderArithmeticErrorIsFatal].
	s.ForHeaderArithmeticErrorIsFatal = interp.Yes
	// The arithmetic reader stops at a byte it refuses and what it had by
	// then stands, which `let` then reads for truth: `let '1 @'` is 0 here
	// and 1 in the other three. Only that failure — `let '1+'` is 1 here too.
	s.LetKeepsTheValueBeforeAnIllegalByte = interp.Yes
	// A math error inside `(( ))` leaves 2 here where the rest of the panel
	// leaves 1, and the sentence in front of it is already the same in both:
	// `(( 1+ )); echo $?` is 2 in zsh 5.9.2 and 1 in bash 5.3, while
	// `let "1+"` is 1 in both. #1625.
	s.ArithCommandErrorStatusIsTwo = interp.Yes
	s.ShiftPastEndFatal = interp.No
	s.ArrayBaseIsZero = interp.No
	// And the brackets an unbraced name carries are that element's, which is
	// the reading the grammar flag BareSubscript exists for. `ksharrays`
	// moves this with the base and the other three array axes — see
	// ksharrays.go, which is where the group and its measurement live.
	s.BareSubscriptIsASubscript = interp.Yes
	// `${a[1,3]}` is elements one through three here, where the shells that
	// read the same characters as arithmetic take the comma operator's value
	// and name element three alone. Measured on `a=(w x y z)`: `w x y` here,
	// `z` in bash 5.3.15, bash 3.2.57, bash as `sh` and ksh93.
	s.SubscriptCommaIsARange = interp.Yes
	// And a subscript on a plain string reaches into its characters: with
	// `s=hello`, `${s[2]}` is `e` here and nothing in the four that read a
	// scalar as an array of one.
	s.ScalarSubscriptIsACharacter = interp.Yes
	// An expansion result's `(`, `)` and `|` build a group where the result
	// is matched against the filesystem, which here is under `globsubst`:
	// measured 2026-10-03 on zsh 5.9.2 in a directory holding `aa`, `ab` and
	// a file named `aa|ab`, `P='aa|ab'; set -- $P` is the two names `aa ab`,
	// and so are `(aa|ab)`, `a(a|b)` and, under kshglob, `@(aa|ab)`. Without
	// the option no result is a pattern, so the question is never put. See
	// Semantics.ExpansionResultSuppliesGroupSyntax.
	s.ExpansionResultSuppliesGroupSyntax = interp.Yes
	// unanswered TableLetterReachesItsOwnOperandsSubscript: zsh refuses the
	// shape the axis is about rather than answering it — measured 2026-09-12,
	// `typeset -A m[k]=v` is `m[k]: inconsistent type for assignment` and
	// fatal, so there is no key and no evaluated subscript to choose between.
	// A keyed literal's bare element is an ordinary word here, and comes to
	// one field only because this shell does not split an unquoted expansion.
	// The empty word is the discriminator: `e=; typeset -A m=(p $e q)` is
	// `typeset -A m=( [p]=q )`, the null removed, where an assignment's value
	// would have kept it. See
	// Semantics.BareElementsInATableLiteralAreEachOneValue.
	s.BareElementsInATableLiteralAreEachOneValue = interp.No
	// And they must pair off: an odd number of fields is refused rather than
	// leaving the last one a key with nothing under it. Measured 2026-09-26,
	// `typeset -A h=(a 1 b)` is `bad set of key/value pairs for associative
	// array`, the shell exits 1 and nothing after it runs, where the even
	// `typeset -A h=(a 1 b 2)` and the repeated-key `typeset -A h=(a 1 a 2)`
	// are both taken. See Semantics.BareElementsInATableLiteralMustPairOff.
	s.BareElementsInATableLiteralMustPairOff = interp.Yes
	s.ArrayLiteralSubscriptIsAKey = interp.No
	// A `[k]+=` element joins what the literal has built, not the table it
	// replaced: `typeset -A m; m[k]=v; m=([k]+=x)` is `x`.
	s.KeyedLiteralAppendJoinsTheReplacedValue = interp.No
	// The FUNCTION_ARGZERO option, on by default and the reason this shell
	// alone moves `$0`: it names the function being run, or the file being
	// sourced, and goes back to the script's name when that call returns.
	s.DollarZeroNames = interp.DollarZeroIsTheInnermostCall
	s.BuiltinSyntaxErrorFatal = interp.No
	// A sourced file is read a command at a time and `eval`'s text is read
	// through first, which is the split that makes these two fields rather
	// than one. Measured with a side effect: a file whose second line will
	// not parse has run its first, and the same two lines inside `eval` have
	// run nothing.
	s.EvalRunsWhatItParsed = interp.No
	s.SourcedFileRunsWhatItParsed = interp.Yes
	// An error inside a file `.` read ends that file and nothing above it:
	// measured, `.` reports 126 and the sourcing file runs the command after
	// it. The status is Diagnostics.SourcedFatalStatus.
	s.FatalErrorEndsBorrowedTextOnly = interp.Yes
	s.TraceFromBorrowedTextFollowsItsRedirection = interp.No
	// Except `${x?word}`, which this shell's own manual documents as printing
	// the word and *exiting the shell* — and measured to do exactly that from
	// inside a sourced file, where an unset parameter under `set -u` two
	// lines away is caught. So it is a request to stop rather than an error.
	s.ParamErrorIsAnExitRequest = interp.Yes
	s.DotMissingFileFatal = interp.No
	// And a missing operand is `not enough arguments` at 1 with the script
	// running on, measured 2026-09-18 on zsh 5.9.2 under both spellings.
	s.DotWithNoOperandIsFatal = interp.No
	// And the mode moves nothing here either: measured 2026-09-19, this
	// shell called `sh` still writes `not enough arguments` at 1 and runs
	// the line after it, which is the row that keeps the companion from
	// being a constant inside the mode (#3818).
	s.DotWithNoOperandIsFatalInPosixMode = interp.No
	s.DotPassesArguments = interp.Yes
	// And the caller's come back over whatever the file did to them, `set`
	// included: measured 2026-09-21 on zsh 5.9.2, `set -- a b c; . ./g p q;
	// echo "$@"` with `set -- m n o p` in the file is `a b c`, where bash
	// lets the `set` stand (#4063).
	s.DotSetCancelsTheRestore = interp.No
	// A leading dash-word is the file here, so `. -p dir f` is a complaint
	// about a file called `-p` and not about an option.
	s.DotReadsOptions = interp.No
	// `eval` is the exception to the line above: it reads the `--` marker
	// and nothing else. `eval -- echo hi` prints `hi`, and `eval -q echo
	// hi` runs `-q` as a command at 127 rather than refusing the letter —
	// which `unset -q` here does refuse, so it is this builtin's answer
	// and not the shell's.
	s.EvalOptions = interp.EvalTakesTheEndMarkerOnly
	// `builtin -q` and `builtin --` are both `no such builtin`: the word is
	// a name. See interp.Semantics.BuiltinReadsOptions.
	s.BuiltinReadsOptions = interp.No
	s.DotTakesTheSearchPathOption = interp.No
	// zsh opens a directory operand, reads no commands out of it and calls
	// that a script that did nothing: measured, `. ./` is silent at status
	// 0 and `. ./ && echo ok` prints `ok`. dash agrees; bash and ksh93 do
	// not. We reported `no such file or directory: ./` at 127 — the status
	// that says the path was never opened — four times in a real startup
	// (#1577).
	s.DotDirectoryOperandIsAnError = interp.No
	// zsh and ksh93 drop the EXIT trap when an exec fails, whichever way it
	// failed; dash and BusyBox ash still run it either way, and bash runs it
	// for a PATH-search miss alone (#3983).
	s.ExecFailureRunsExitTrap = interp.No
	s.ExecFailureOnAPathnameRunsExitTrap = interp.No
	s.ExecTakesOptions = interp.Yes
	// And the words behind it are an ordinary command word list here, so a
	// builtin, a function or a precommand modifier standing in front of
	// either is reached rather than looked for on PATH: `exec :` is silent
	// and ends the shell, and `exec builtin echo hi` writes `hi`. The other
	// four columns answer both with `exec: …: not found` (#5047).
	s.ExecReachesTheShellsOwnCommands = interp.Yes
	// Both letters, and `-a` wins over `-l` in either order.
	s.ExecTakesTheLoginLetter = interp.Yes
	s.ExecTakesTheEmptyEnvironmentLetter = interp.Yes
	s.ExecLoginPrefixesTheGivenName = interp.No
	// And the options are read before the match: `exec -a foo* cmd` names the
	// command `foo*` beside a file foo1. Measured 2026-10-02 (#5138).
	s.ExecOptionsAreReadBeforeGlobbing = interp.Yes
	// And the name a command is started under can be asked for by an exported
	// `ARGV0`, which no other column reads: the variable is spent on argv[0]
	// and the child never sees it. `exec -a` is the other spelling and wins
	// where both are written.
	s.ExportedArgv0NamesTheCommand = interp.Yes
	s.TestAcceptsDoubleEqual = interp.Yes
	// Of the operators past the three-word rules this shell has only `-N`:
	// `test -a f` and `test -o errexit` are `too many arguments` here, and
	// `<` and `>` are `condition expected`.
	s.TestHasTheModifiedSinceReadOperator = interp.Yes
	// And a file written and not read since — equal times, which is every
	// file a script has just created — is `-N` true here, where bash 5.3 and
	// ksh93 want the write to be strictly the later of the two. Measured
	// 2026-09-25 with `touch -t` setting both times, so the case holds still;
	// bash 3.2 is on this side of it too.
	s.TestModifiedSinceReadCountsAnEqualTime = interp.Yes
	// `set -p` is the short spelling of `privileged` here too.
	s.SetHasThePrivilegedLetter = interp.Yes
	// `-nt` and `-ot` want both files to exist, and `-t x` is a plain
	// false rather than an integer complaint.
	s.MissingFileIsOlder = interp.No
	s.TerminalTestRequiresANumber = interp.No
	// And it is read as an arithmetic expression instead, which is what makes
	// `[[ -t / ]]` a fatal math error here where three of the four columns
	// answer a silent 1. See Semantics.TerminalTestOperandIsArithmetic, which
	// carries the panel and the `x++` row that says the operand is really
	// evaluated (#5058).
	s.TerminalTestOperandIsArithmetic = interp.Yes
	// And a lone `-t` is `-t 1` rather than a non-empty string, the one
	// reading this shell shares with ksh93: `[ -t ] >/dev/null` is 1 here
	// and 0 in dash and bash.
	s.BareTerminalTestIsDescriptorOne = interp.Yes
	// And a trailing operator with nothing behind it is read as a word,
	// which is what that answer is then applied to: `test -n xx -a -f` is 0
	// and `test -n xx -a -t` is 1 — the word, and then `-t 1` about it.
	s.TestTrailingUnaryOperatorIsAWord = interp.Yes
	// And a connective with nothing behind it is the connective still, over
	// a right operand that is missing and therefore false — the reading this
	// shell shares with dash and with no other column. Measured 2026-09-18:
	// `[ x -a ]` is 1 here, `[ x -o ]` is 0, and `[ 1 -eq 1 -a ]` is 1,
	// silently in each case. The one shape the two columns split on is
	// `[ ! x -a ]`, 1 here and 0 in dash, which is where the rule is asked
	// rather than the rule; the axis records it (#2917).
	s.TestTrailingConnectiveTakesAMissingOperand = interp.Yes
	s.PipefailOption = interp.Yes
	// A substituted element keeps the status its death produced, 128 plus
	// the signal, the same as anywhere else.
	s.PipefailSubstitutesTheBareSignal = interp.No
	s.ErrexitSeesPipefailFailure = interp.Yes
	// The default state of `ERR_RETURN`, which is off — the shell that has
	// the option is still the shell that does not do it unasked. Measured
	// 2026-09-25 on zsh 5.9.2: `unsetopt errreturn; f() { false; print
	// notreached }; f; print "after f: status=$?"` writes `notreached` and
	// `after f: status=0` at 0, in the reference and here alike. `setopt
	// errreturn` moves this axis and nothing else does; see
	// dialect/zsh/setopt.go.
	s.FailureTakesAnImplicitReturn = interp.No
	// `time` changes nothing about the judging here either: `set -e; time
	// false` stops and the ERR trap fires, over every shape measured.
	s.TimedCommandIsJudged = interp.Yes
	// And a redirection that cannot be opened on a compound command is a
	// failure both judges see.
	s.CompoundRedirectionFailureIsJudged = interp.Yes
	// This shell writes two E for `true | ( false )` as well, and for a
	// different reason: its ERR trap runs inside subshells at all, so
	// `( false )` on its own is already two. Measured 2026-09-18, `true |
	// ( exit 3 )` — the row with no failing command inside the element —
	// writes one E here and two in bash, which is what separates the two
	// mechanisms.
	s.ASubshellAsTheLastPipelineElementJudgesItself = interp.No
	// A pipeline waits for every element, not only its last, whichever
	// element runs in the shell: `{ sleep 0.3; echo first >&2; } | :; echo
	// after` writes `first` before `after`. Measured 2026-11-01 on 5.9.2; see
	// interp.Semantics.PipelineEndsWithItsLastElement (#7169).
	s.PipelineEndsWithItsLastElement = interp.No
	// Alone in refusing an argument to `times`; dash and bash ignore it.
	s.TimesRejectsArguments = interp.Yes
	// `[ ( -n x ) ]` is 0 here (#3419).
	s.TestGroupedUnaryAloneLosesTheClosingParen = interp.No
	// `[ ! ! -n x ]` is 0 here, so the four-word `!` negates the negation
	// behind it. Measured 2026-09-19 (#3700).
	s.TestFourWordsNegateANegationOnce = interp.No
	s.TestNegationBeforeAnOperatorIsAnOperand = interp.Yes
	// And the connective before a leading `!` at three words: `[ ! -a / ]`
	// is 0 here where this shell has no unary `-a` at all — `[ -a / ]` is
	// `too many arguments` at 2 — so reading the negation first would refuse
	// and reading the connective first gives the both-set guard over `!` and
	// `/`. Measured 2026-09-19 (#3717).
	s.TestThreeWordsNegateBeforeAConnective = interp.No
	s.TestFailureInsideAnUnclosedGroupIsTheParen = interp.No
	// And a group with nothing in it is `argument expected` at 2 here,
	// which is a refusal rather than a false expression (#3687).
	s.TestEmptyGroupIsFalse = interp.No
	// unanswered LocalThroughCommandDeclaresNothing: `command local a=1` is
	// `command not found: local` at 127 in this shell, so the declaration
	// this axis is about never happens and there is nothing to measure.
	s.UnterminatedBracket = interp.BracketBadPattern
	// And the same question where a `[:name:]`, a `[.x.]` or a `[=x=]`
	// inside it is what left it open: not a pattern, the same as a bare `[`.
	s.UnterminatedBracketAfterASubExpression = interp.BracketBadPattern
	// And the one column that **compiles** a bracket the member reading
	// cannot close: measured 2026-09-26 on 5.9.2, `echo []` is `no matches
	// found: []` where the other five print the word, and `echo [!]` lists
	// every one-character name — the negation of a set with no members.
	// `echo []]`, `echo []a]` and `echo []a~b]` are the controls and they
	// are ordinary sets here exactly as they are everywhere else, which is
	// why this is keyed on the bracket and not on the `]`. See
	// interp.Semantics.EmptyBracketExpressionCompiles (#4644).
	s.EmptyBracketExpressionCompiles = interp.Yes
	s.UnknownCharacterClass = interp.UnknownClassIsInert
	// The one column with neither construct: `[[.a.]]` is the three-member
	// set `[`, `.`, `a` followed by a literal `]`, so it matches `a]` where
	// four columns match `a` and one matches nothing at all. Measured
	// 2026-09-16 on 5.9.2.
	s.CollatingElements = interp.NoCollatingElements
	// `[[:]` is a bracket holding `[` and `:`, the same reading bash 5.3 gives it.
	// See interp.Semantics.UnterminatedCharacterClass (#1431).
	s.UnterminatedCharacterClass = interp.UnterminatedClassIsOrdinaryCharacters
	// This shell does not glob the result of an expansion, so the axis is
	// reached only through `${~spec}` and `setopt globsubst` — and it has an
	// answer there rather than no answer at all. Measured 2026-09-12 in a
	// directory holding `a\b` and `a*`: `v='a\*'; print -r -- ${~v}` is
	// `a\*`, so the `*` behind the backslash was not live (#1367).
	s.ValueBackslashInAPattern = interp.ValueBackslashDisarmsWhatFollows
	// The piece question never arises with that answer either — the backslash
	// stays a character of the pattern — and is answered for the reason ksh93's
	// is: an axis left open refuses the word the day the one above moves.
	s.ValueBackslashSurvivesAPatternPiece = interp.Yes
	// A bracket carrying a live `/` is a bracket that matches nothing here, and
	// this column needs no option to show it: a pattern matching nothing is an
	// error, and `print -r -- [a/b]` is `no matches found: [a/b]` on 5.9.2. See
	// interp.Semantics.BracketHoldingASlashIsStillABracket (#4158).
	s.BracketHoldingASlashIsStillABracket = interp.Yes
	// ksh93's answer for the trim at the end of a `read` value, measured the
	// same way: `printf 'a b\\ \n' | read x y` leaves `b` here (#1360).
	s.ReadTrailingEscapedSeparator = interp.ReadTrailingEscapedSeparatorTrimmed
	// The arm written first, which is this shell alone in the panel:
	// measured 2026-09-11 on 5.9.2, `x=abc`, `${x##(a|ab)}` is `bc` where
	// `${x##(ab|a)}` is `c`. The same order decides what a `(#b)` reports,
	// which the matcher already followed — and it decides a **substitution**
	// too, measured 2026-09-12: `${x//(a|ab)/X}` is `Xbc` and
	// `${x//(ab|a)/X}` is `Xc`. See
	// Semantics.LongestMatchTakesTheWrittenArm.
	s.LongestMatchTakesTheWrittenArm = interp.Yes
	// An empty pattern is an ordinary pattern that matches the empty string,
	// which is this shell alone: measured 2026-09-12 on 5.9.2, `v=abc`,
	// `${v///X}` is `XaXbXc` where bash and ksh93 leave the value alone.
	// The end of the value is not one of the positions, which is the general
	// rule below rather than anything about this pattern.
	s.EmptyReplacementPattern = interp.EmptyReplacementPatternMatchesEveryPosition
	// The anchors, and an empty pattern behind one still matches at that
	// end: `v=abcabc` gives `Xabcabc` for `${v/#/X}` and `abcabcX` for
	// `${v/%/X}`, which is bash's answer and not ksh93's (#3272).
	s.ReplacementAnchors = interp.Yes
	// And after the global `//` as well, which is this shell alone: the two
	// spellings are one construct here and two everywhere else. Measured
	// 2026-09-16 on 5.9.2 — `${v//#a/X}` on `abcabc` is `Xbcabc` where every
	// other column is `abcabc`, and the discriminating row, `${w//#a/Q}` on
	// `x#ay%bz`, is `x#ay%bz` here and `xQy%bz` there: this shell left the
	// value alone *because* it anchored (#3307).
	s.GlobalReplacementAnchors = interp.Yes
	s.AnchoredEmptyReplacementPattern = interp.Yes
	// The same empty match bash refuses, measured the same way:
	// `${v//(b|)/<>}` under extendedglob is `<>a<><>c`.
	s.ReplacementEmptyMatchDeclined = interp.EmptyMatchDeclinedAtTheEnd
	s.ExitTrapIsFunctionLocal = interp.Yes
	// And the caller's trap a call holds is lost when `set -e`,
	// `${x?word}` or an untrapped HUP ends the shell from inside the call:
	// measured 2026-10-30 on 5.9.2, `trap 'echo T' EXIT; set -e; f() {
	// false; }; f` writes nothing, where the same `false` at the top level
	// writes T (#6952, #7007), and `f() { kill -HUP $$; }` in place of the
	// `set -e` writes nothing either (#7065).
	s.StopInsideACallLosesTheHeldExitTrap = interp.Yes
	// And the shell runs no EXIT trap at all when it ends over an error it
	// reported with `set -e` on. Measured 2026-09-18 over twelve rows, each
	// written twice: a refused `set` option, a readonly reassignment, a
	// `break` outside a loop, an unset parameter under `set -u` and a
	// division by zero all lose the handler under the option and keep it
	// without; `exit 3`, a plain `false` under the option and `${x?word}` all
	// keep it. See interp.Runner.exitTrapSkippedByAFatalError (#2744).
	s.FatalErrorUnderErrexitSkipsTheExitTrap = interp.Yes
	// And through `-c` it loses them for the same errors with no option at
	// all, TRAPEXIT included. Measured 2026-10-16 on zsh 5.9.2 under `-f`:
	// a division by zero, a readonly reassignment, a refused `set` option,
	// `break` outside a loop, an unset parameter under `set -u` and a
	// substitution that would not parse each end the string with no
	// handler, where the same line from a file or standard input runs it.
	// See interp.Runner.exitTrapSkippedOnACommandString (#6666).
	s.FatalErrorOnACommandStringSkipsTheExitTrap = interp.Yes
	// A *subshell* loses it for the same errors with no option at all, and
	// for two more besides: `${x?word}`, which keeps it at the top level
	// here, and a refused `set` option, which ksh93's subshell keeps.
	// Measured 2026-09-18 over nine rows in `( … )`, each written plain and
	// under `set -e` — identical both ways. See
	// interp.SubshellExitTrapPolicy (#3612).
	s.SubshellExitTrapAfterAGiveUp = interp.SubshellExitTrapSkippedByABuiltinsUsageToo
	s.SignalHandlerSeesEarlierStatus = interp.Yes
	// And `set -e` judges `! f` and `! eval …` by the inverted status, where
	// every other negated command is exempt. Measured 2026-10-03; see the axis.
	s.ErrexitJudgesANegatedCall = interp.Yes
	// A refused `set -o` name followed by a listing ends the script at the
	// listing's 0. Measured 2026-10-03; see the axis.
	s.SetListingAfterARefusalLeavesZero = interp.Yes
	// A name may hold the locale's letters and digits past ASCII, until
	// `posix_identifiers` says otherwise — see setopt.go. Measured
	// 2026-10-02 on 5.9.2 (#5153).
	s.NamesTakeTheLocalesLetters = interp.Yes
	// `read -d £` stops at the whole `£` under a UTF-8 locale. Measured
	// 2026-10-02 on 5.9.2 (#5153).
	s.ReadDelimiterIsTheLocalesCharacter = interp.Yes
	// `printf '[%q]'` with no operand is `[]`. Measured 2026-10-02 (#5153).
	s.PrintfQuoteOfNoArgumentIsEmpty = interp.Yes
	// An element store arithmetic refuses fails the expression, as `1/0`
	// does: `(( a[0] = 1 ))` is 2. Measured 2026-10-02 on 5.9.2 (#5145).
	s.ArithStoreRefusalIsAnError = interp.Yes
	// `inf` and `nan` are the floating constants, in any case. Measured
	// 2026-10-02 on 5.9.2 (#5145).
	s.ArithInfAndNaNAreConstants = interp.Yes
	// The operand of `exit` and of `return` is an arithmetic expression here,
	// and alone in the panel: `return r` is the value of `r` and `return r+1`
	// is one more, where ksh93 reads the leading digits and gets 0, and dash
	// and bash refuse the word outright. It also declines to mask, so
	// `return 300` leaves 300 rather than 44.
	//
	// `~/.zi/bin/zi.zsh` counts the ice-mods it consumed into an integer and
	// ends `.zi-ice` with `return retval`; reading that as anything but
	// arithmetic hands back the previous command's status instead of the count,
	// and the plugin manager shifts its own command line by the wrong number.
	s.StatusArgument = interp.StatusArgArithmetic
	// A status that carries a count rather than a verdict: two dead targets
	// is 2.
	s.ExitTrapRunsOnSignalDeath = interp.No
	// So no `exit` ever runs in a trap over a death to ask about; no is the
	// answer that leaves the death alone. A fatal signal while the EXIT trap
	// runs ends the shell by that signal: measured 2026-10-30 on zsh 5.9.2,
	// `trap 'echo bye; kill -USR1 $$; echo after' EXIT; exit 3` writes bye
	// and dies of USR1, as it does for one sent from outside (#7064).
	s.ExitInTheExitTrapOverridesASignalDeath = interp.No
	s.FatalSignalInTheExitTrapEndsOnlyTheTrap = interp.No
	// A SEGV kills the shell where it lands, as every fatal signal does here,
	// so the question is never reached: measured 2026-10-30 on zsh 5.9.2,
	// `kill -SEGV $$` with an EXIT trap set writes nothing and dies of it, in
	// the trap as before it (#7069).
	s.UntrappedSegvEndsTheShellWhereItLands = interp.Yes
	// And no death is ever held for an EXIT trap here, so neither question
	// about one is asked: `kill -TERM $$ 99999999` dies as it sends, without
	// the complaint about the second operand. Measured 2026-10-30 (#7062).
	s.SelfAimedDeathIsTakenAtTheNextCommand = interp.No
	s.DeathInAFunctionEndsTheCallAsKilled = interp.No
	// Nor is anything caught for a `function name { … }` call to end: the
	// same TERM in one dies with nothing written. Measured 2026-10-31 on zsh
	// 5.9.2 (#7067).
	s.SignalDeathEndsAtAKeywordFunctionCall = interp.No
	// Nor for a subshell to wait out: the same TERM during `( /bin/sleep 2;
	// echo in )` kills it at once with no trap run, the parentheses writing
	// `in` at 2 s. Measured 2026-10-31 on zsh 5.9.2 (#7068).
	s.SignalDeathWaitsForAnUnforkedSubshell = interp.No
	// Nor a status to show for a death during a pipeline: the same TERM during
	// `/bin/sleep 2 | /bin/cat` kills it with nothing run. Measured 2026-11-01
	// on zsh 5.9.2 (#7160).
	s.DeathDuringAPipelineShowsTheStatusBeforeIt = interp.No
	// Nor a write past `ulimit -f` to give up over: measured 2026-11-01 on
	// zsh 5.9.2, `trap 'echo t $?' EXIT; ulimit -f 1; printf '%4000s' x > f`
	// dies of XFSZ with no trap run (#7173).
	s.CaughtWritePastTheFileSizeLimitIsAFatalError = interp.No
	s.ExitTrapRunsInsideTheExitingCall = interp.No
	// But it does run inside the `eval` text or the sourced file the stop
	// was raised in, once any function frames are left: `( trap…; eval
	// 'exit 0' )` is located `(eval):1` and a `.`'s is named after the file,
	// with that file's `$1`. Measured 2026-10-30 on 5.9.2 under `-f` (#6954).
	s.ExitTrapRunsInsideTheExitingText = interp.Yes
	s.QuitIgnoredWhenNotInteractive = interp.Yes
	// And zsh alone parts from bash on what `trap -` then means: it hands
	// SIGQUIT back its default action, so `trap - QUIT; kill -QUIT $$` kills
	// this shell where bash prints the next command. Measured without any
	// handler ever having been installed, so it is a disposition and not a
	// memory of what the script did — and `trap '' QUIT` afterwards brings
	// the ignore back.
	s.QuitResetRestoresTheDefault = interp.Yes
	// Interactive, an untrapped ALRM is a timeout: `zsh:1: timeout`, located
	// as any diagnostic is, and an exit at 14 that runs the EXIT trap and
	// zshexit, until `trap - ALRM` hands it back to its default action.
	// XCPU and XFSZ end it by the signal. Measured 2026-10-30 under `-i -c`
	// and at a pseudo-terminal, from the script and from another process
	// (#7058). See interp.Semantics.InteractiveAlarm.
	s.InteractiveIgnoresResourceLimitSignals = interp.No
	s.InteractiveAlarm = interp.InteractiveAlarmTimesOut
	// This shell alone: an untrapped SIGHUP ends the shell with 1 and runs
	// the EXIT trap, rather than killing it with 128 plus the number. The
	// EXIT trap is why it is not simply a different number — the line above
	// says dying does not run it, and here one runs.
	s.HangupIsAnOrderlyExit = interp.Yes
	// zsh alone: a bare `exit` there reports what the trap's own last
	// command did, so `trap "false; exit" 0` exits 1.
	s.ExitInTrapReportsEarlierStatus = interp.No
	// And a `return` written in that body names the status the shell leaves
	// with, exactly as an `exit` there does: measured 2026-09-26 over a
	// script file, `trap 'print X; return 5' EXIT; false` writes X and exits
	// 5, and a subshell's own trap makes its `$?` 5 the same way. ksh93 is
	// the other column that answers this way; dash and BusyBox ash end the
	// body and keep the status they had.
	s.ReturnInTheExitTrapNamesTheStatus = interp.Yes
	// A function-local EXIT trap is not run for a call an error ended, and
	// reads `$?` as 0 for one an `exit` ended (#6667, #6836).
	s.FunctionExitTrapSkippedByAGiveUp = interp.Yes
	s.FunctionExitTrapReadsZeroOnAnExit = interp.Yes
	s.KillListAcceptsName = interp.Yes
	// One subtraction, and what it cannot name it prints back — `kill -l
	// 160` is `160` here. No EXIT: this shell answers `kill -l 0` with 0.
	s.KillListReducesRepeatedly = interp.No
	s.KillListPrintsANumberItCannotName = interp.Yes
	s.KillListNamesZeroAsExit = interp.No
	s.SIGPrefixAccepted = interp.Yes
	s.RedirectsUseEveryTarget = interp.Yes
	// `exec {fd}< /etc/hosts; echo $fd` says 11 here and 10 in bash 5.3 and
	// ksh93 — measured 2026-09-12 — and 10 here too where the program is
	// read from standard input (2026-10-01, #5133). The same number comes
	// back from `zsocket` in `$REPLY` and from `sysopen -u name` (#1752).
	s.FirstAllocatedDescriptor = interp.AllocateDescriptorsPastTheShellsInput
	// `exec 3<<X` puts the body in a temporary file here, so `/dev/fd/3` is
	// a regular file and the descriptor is seekable: `head -1 <&3` reads a
	// block and seeks back, and the `cat <&3` after it still gets the rest of
	// the document. bash 5.3.15 and dash use a pipe and lose it. Measured
	// 2026-09-14 (#2759).
	s.HeredocBody = interp.HeredocBodyInATemporaryFile
	// A read that fails after the open worked fails the substitution here:
	// `mkdir dir; v=$(<dir)` is status 1 with a sentence, where bash 5.3 and
	// ksh93 are status 0 in silence (#1778).
	s.ReadFailureInAFileSubstitutionFailsIt = interp.Yes
	// The null-command hook: a command that is only redirections runs
	// `$NULLCMD`, or `$READNULLCMD` where its one redirection is a plain
	// `<file`. The names, not the values — a script reassigns them at will,
	// and the defaults live in the prelude where a script can see and change
	// them. `setopt cshnullcmd` and `setopt shnullcmd` move these two fields
	// rather than adding a branch; see nullcommand.go in this package.
	s.NullCommandVariable = nullCommandParameter
	s.ReadNullCommandVariable = readNullCommandParameter
	s.KillStatus = interp.KillStatusFailureCount
	// A subshell sees the parent's job table when the parent's monitor is
	// on, and sees nothing when it is off — which is every script, so this
	// used to read Cleared and was right about every case a script can
	// reach. Measured 2026-09-25 against zsh 5.9.2: `zsh -f -c 'sleep 1 &
	// jobs; (jobs)'` writes the row once, and the same line under `-fm` on a
	// pseudo-terminal writes it twice. The monitor and not the terminal and
	// not the prompt: `zsh -fi` with `unsetopt monitor` writes it once
	// again (#4538). The rows and what the subshell may do with them are on
	// interp.SubshellJobsKeptUnderTheMonitor.
	s.SubshellJobTable = interp.SubshellJobsKeptUnderTheMonitor
	s.PrintfEmptyIsNotANumber = interp.No
	s.PrintfAbsentNumberIsAnEmptyOne = interp.No
	s.PrintfStarWithoutOperandIsRefused = interp.No
	// A `*` with no operand left leaves its field out rather than setting
	// it to zero: `printf '[%.*f]'` is `[0.000000]` here, the default
	// precision, where bash and dash write `[0]`.
	s.PrintfStarWithoutOperandIsAbsent = interp.Yes
	s.PrintfStarComplaintCostsTheStatus = interp.Yes
	// zsh alone writes the bare word: `%G` of an infinity is `inf` and
	// `%10f` of one is `inf` unpadded.
	s.PrintfNonFiniteIsConverted = interp.No
	// The `'` flag, among the flags and nowhere else: `%15'd` is
	// `%15': invalid directive`.
	s.PrintfGroupingFlag = interp.Yes
	s.PrintfGroupingFlagAfterTheWidth = interp.No
	// A `*` beside a width's own digits is refused here too: `printf '%5*d' 4 42`
	// is a conversion character this shell does not have (#2824).
	s.PrintfStarBesideTheFieldDigits = interp.No

	// The width is stored in a C int and what is left is used, so
	// `printf '[%21474836470s]' x` — -10 as an int32 — is `[x         ]`
	// and `%4294967306s` is `[         x]`. Measured 2026-09-15.
	s.PrintfFieldBeyondAnInt = interp.PrintfFieldWrapsToAnInt

	// Wraps on both routes, which is what makes the literal spelling and
	// the star agree here. Measured 2026-09-15.
	s.PrintfStarBeyondAnInt = interp.PrintfStarWrapsToAnInt
	s.CaseSubjectKeepsThePreviousLine = interp.No
	// unanswered SubstringRangeThirdColonIsABadSubstitution: a third segment
	// is a *modifier list* here — `${x:1:5:t}` is `D` — so the question of
	// what to call the refusal never arises.
	s.PrintfReportsBadNumber = interp.No
	s.PrintfNumberOperand = interp.PrintfNumberArithmetic
	s.PrintfIntegerCharConstantAfterBlanks = interp.No
	// Exact, though the reading is an expression: this shell's arithmetic is
	// an integer one and `printf '%d' 123456789012345678` is the operand
	// (#2907). It is ksh93 alone that rounds.
	// A flag past a field is no flag at all: the prefix ends there and the
	// byte arrives at the scan as the conversion character (#2910).
	// A division by zero ends the reading of the operand here, which is
	// what the zero beside `printf '%d' 1/0` says (#2912).
	s.ArithDivisionByZeroYieldsAValue = interp.No
	s.PrintfFlagAfterTheField = interp.No
	s.PrintfIntegerOperandGoesThroughTheFloatingType = interp.No
	// None of C99's three: `printf '%F' 1.5` is `%F: invalid directive` at
	// 1 in zsh 5.9.2, and `%a` and `%A` the same.
	s.PrintfC99FloatConversions = interp.No
	// `%n` all the same, and the count is this pass's bytes: `printf
	// '%.10e%n\n' 1 count` leaves 16 in zsh 5.9.2, measured 2026-09-26.
	s.PrintfCountConversion = interp.Yes
	// A name the shell did not already have becomes an integer — `printf
	// 'abcd%n' n` is `typeset -i n=4` — and one the script has already made
	// is left as it was: `q=plain; printf 'abc%n' q` is `typeset q=3`, and
	// `q=` beforehand is enough to make that so.
	s.PrintfCountAttribute = interp.PrintfCountIntegerOnANewName
	// The one column whose `%n` fills an element: `arr=(x y z); printf
	// 'abcd%n' 'arr[2]'` leaves `arr[2]` at 4 and reports 0.
	s.PrintfCountOperandTakesASubscript = interp.Yes
	// And the one that refuses an operand which is present and empty:
	// `printf 'ab%ncd' ''` is `not an identifier: ` and ends the script.
	s.PrintfCountEmptyNameIsIgnored = interp.No
	// Neither refusal stops the format. `printf 'abc%nXYZ' '1bad'` writes
	// the whole `abcXYZ`, and a second `%n` after the bad one still stores —
	// `printf 'abc%nX%n' 1bad good` leaves `good` at 4, read back through an
	// EXIT trap because the shell ends once the builtin has returned. See
	// BadNameToPrintfFatal, which is what ends it.
	s.PrintfCountBadNameStopsThePass = interp.No
	s.PrintfCountFrozenNameStopsThePass = interp.No
	// unanswered PrintfHexFloatZeroFillPrecedesThePrefix: and so no fill to
	// place in it either.
	// unanswered PrintfHexFloatDefaultIsTwelveDigits: there is no `%a` here
	// to have a default precision.
	s.PrintfRefusedOperandKeepsItsLeadingNumber = interp.No
	// One complaint per operand whichever conversion asked: `printf '%f'
	// 42abc` writes one arithmetic line here, as `printf '%d' 42abc` does.
	s.PrintfFloatOperandIsEvaluatedTwice = interp.No
	s.PrintfBackslashC = interp.PrintfBackslashCStops
	s.PrintfUnfinishedConversionIsAPercent = interp.No
	// A conversion may name its argument; see
	// interp.PrintfPositionsPerPass for the measurements.
	s.PrintfArgumentPositions = interp.PrintfPositionsPerPass
	// And `printf -v` into an array stores a use of the format an element;
	// see the axis for the measurements.
	s.PrintfVTakesAnElementPerPass = interp.Yes
	// A command word that names nothing goes to this function when it is
	// defined; see the axis for the measurements.
	s.CommandNotFoundHandler = "command_not_found_handler"
	// The same two digits bash reads, and an empty digit run is a zero
	// rather than an escape left standing: `printf 'a\xZ'` is a NUL here.
	// unanswered PrintfReportsAMissingHexDigit: an empty digit run is a zero
	// here too, by the reading beside this line, so the complaint's site is
	// unreachable. The two columns that do reach it split, and
	// TestPrintfMissingHexDigit pins the pair (#3239).
	s.PrintfHexEscape = interp.PrintfHexEscapeByteOrNul
	// A `%b` argument reads the same escape the same way, NUL and all.
	s.PrintfBHexEscape = interp.PrintfHexEscapeByteOrNul
	// `\u` and `\U` at both sites, and an empty digit run is a zero here
	// too: `printf 'a\uZ'` is an `a`, a NUL and a `Z`, where bash leaves the
	// escape standing and complains.
	s.PrintfUnicodeEscape = interp.PrintfUnicodeEscapeCodePointOrNul
	s.PrintfBUnicodeEscape = interp.PrintfUnicodeEscapeCodePointOrNul
	// The format site splits the same way this shell's `%b` does, and in
	// the other direction from ksh93's format: `printf 'a\eZ'` is
	// `61 1b 5a` and `printf 'a\EZ'` is `61 5c 45 5a` in 5.9.2 (#3225).
	s.PrintfEscEscape = interp.Yes
	s.PrintfCapitalEscEscape = interp.No
	// An escape a format does not define keeps its backslash — `printf
	// '[\q][\z][\8][\-]'` is `[\q][\z][\8][\-]` — and a floating
	// conversion's exact half goes to the even neighbor: `printf '%.0f
	// %.0f %.0f' 2.5 4.5 -2.5` is `2 4 -2` and `%.2f` of 0.125 is `0.12`.
	// Measured 2026-09-18 under `LC_ALL=C` from a script file; ksh93 is the
	// one column on the other side of both.
	s.PrintfUnknownEscapeDropsTheBackslash = interp.No
	s.PrintfFloatHalf = interp.PrintfFloatHalfToEven
	// `\e` is the escape character and `\E` is two characters — the opposite
	// of ksh93, which is why one axis could not answer for both letters.
	s.PrintfBEscEscape = interp.Yes
	s.PrintfBCapitalEscEscape = interp.No
	// The octal wants its `\0`, as in ksh93.
	s.PrintfBOctalWithoutZero = interp.No
	s.PrintfBStopIsPadded = interp.Yes
	s.PrintfBTakesAField = interp.Yes
	// One of `h`, `l` and `L`, which is C89's set: `%ld` is a decimal and
	// `%lld`, `%zX` and `%jd` are invalid directives.
	s.PrintfLengthModifiers = interp.PrintfLengthModifiersC89
	// Characters, where the locale has them: `printf '[%.2s|%7s]' αβγ αβγ`
	// is `[αβ|    αβγ]` under a UTF-8 locale and `[α| αβγ]` under C, zsh
	// 5.9.2, 2026-09-16 — the one column counting characters. The `l` is
	// ignored, so `%ls` is `%s` and `%lc` is `%c`'s byte.
	s.PrintfFieldCountsCharacters = interp.Yes
	s.PrintfLongModifierCountsCharacters = interp.No
	// No `%(fmt)T`: `%(` is a directive this shell does not have.
	s.PidListingFinishesWithAJob = interp.No
	s.PrintfTimeConversion = interp.No
	s.PrintfTimeOperandIsADateString = interp.No
	s.PrintfQuote = interp.PrintfQuoteAnsiCCharacter
	// C's `#` at a value of nought, as in bash: `printf '%#x' 0` is `0`.
	s.PrintfAlternateFormAsksTheValue = interp.Yes
	// And counts it against the width: `printf '%#05x' 7` is `0x007`,
	// five characters, on zsh 5.9.2.
	s.PrintfZeroFillCountsTheAlternatePrefix = interp.Yes
	// zsh is the one shell with `$'…'` and no `\c` in it, so `$'\cA'` is the
	// two characters `cA`; and its strings are counted rather than
	// terminated, so a decoded NUL is a byte like any other.
	s.DollarSingleBackslashC = interp.DollarSingleControlAbsent
	s.DollarSingleUnknownEscape = interp.DollarSingleUnknownDropsBackslash
	s.DollarSingleNul = interp.DollarSingleNulIsAByte
	// Two digits after `\x` and no more, as in bash — but a run with no
	// digit at all is a zero byte here, where bash keeps the two characters
	// it was written as. This shell keeps the zero, so `$'\xzz'` is three
	// bytes.
	s.DollarSingleHexReadsEveryDigit = interp.No
	s.DollarSingleDigitlessEscapeIsAZeroByte = interp.Yes
	// There is no braced spelling at all, so `$'a\x{41}b'` is the zero byte
	// the axis above gives a digitless `\x`, and then `{41}b` as ordinary
	// text. Measured 2026-09-22 under `LC_ALL=C` on zsh 5.9.2 (#4165).
	s.DollarSingleBracedHex = interp.DollarSingleBracedHexAbsent
	// An octal escape past 255 keeps the low byte: `$'\401'` is 01, and
	// `$'\400'` is the zero byte this shell holds as a character of the
	// text. Measured 2026-09-18 by `od` (#3415).
	s.DollarSingleOctalPastAByteDropsTheLastDigit = interp.No
	// `\C-X` is a control character and `\M-X` the same byte with the high
	// bit set, the dash optional in both and either able to take the other as
	// its argument. ksh93 writes `\C-A` too and means `m` then `A` by it, so
	// the value names the reading rather than saying yes (#2345).
	s.DollarSingleCaretMeta = interp.DollarSingleCaretMetaMaskedWithAnOptionalDash
	// `\e` and `\E` are both the escape character here, which is **not** the
	// split this shell's `echo -e` has — there it takes `\e` and leaves `\E`
	// standing. One site's answer is not the other's, and measuring both is
	// what #3270 was about. `\?` and the two Unicode spellings are read too:
	// `3f`, and `41` from $'\u0041' and from $'\U00000041'.
	s.DollarSingleEscEscape = interp.Yes
	s.DollarSingleQuestionEscape = interp.Yes
	s.DollarSingleUnicodeEscapes = interp.Yes
	s.GetoptsAssignmentRestartsWord = interp.No
	// A cursor past the word after the last stays where it was set, as in
	// ksh93: measured 2026-10-15, `set -- -a -b -c; OPTIND=100; getopts abc
	// o` is the end of the options with OPTIND still 100 (#6575).
	s.GetoptsCursorPastTheEnd = interp.GetoptsCursorPastTheEndIsKept
	s.GetoptsNewParametersRestartTheScan = interp.No
	// unanswered GetoptsRefusedNameStillScans: a refused name operand ends
	// the shell here, so nothing downstream can read OPTIND back and say
	// whether the scan ran. Measured 2026-09-21, `set -- -a; getopts a
	// opt-var; echo "rc=$? OPTIND=$OPTIND"` writes `zsh:1: not an
	// identifier: opt-var` and stops — the echo never runs, under `emulate
	// sh` as well. The fatality itself is BadNameToGetoptsFatal's and is
	// answered there.
	// `kill %1` reaches the job's process. dash aims at the group.
	s.KillJobSpecAimsAtTheGroup = interp.No
	// This shell continues a stopped job before it sends, which is what
	// makes the signal land rather than sit pending: measured 2026-09-25
	// through a pseudo-terminal, `sleep 300 &` then `kill -STOP %1` reads
	// `TN` from `ps -o stat=`, and `kill -0 %1` — which delivers nothing at
	// all — reads `SN` afterwards, as does `kill -WINCH %1`. Every other
	// column leaves both at `T`. The `%` word is what decides: `kill -0
	// "$!"` on the same job in the same session leaves it `TN`, so the
	// number is a number here even though this shell knows whose it is. Not
	// for a signal that stops — STOP, TSTP, TTIN and TTOU all leave it `TN`
	// — and it makes no difference whether `^Z` or an explicit `kill -STOP`
	// put it there.
	s.KillJobSpecContinuesAStoppedJob = interp.Yes
	// zsh 5.9.2 sends it: `kill -0 -- -1` is 0.
	s.KillRefusesTheAllProcessesTarget = interp.No
	// zsh 5.9.2 leaves the name unset.
	s.ArrayOperandIsStoredPastAFailedOpen = interp.No
	// zsh 5.9.2 reaches the target behind either spelling.
	s.KillTakesEndOfOptionsAfterTheSignal = interp.Yes
	// Measured 2026-09-26 against zsh 5.9.2: `kill a b c` writes three
	// `illegal pid` lines and reports 3, `kill a b c d` four and 4. The
	// count is this shell's KillStatus; the four lines are this axis.
	s.KillKeepsGoingPastAnOperandThatIsNotAPid = interp.Yes
	// And past a `%` spec that names no job: `kill %99 a` and `kill a %99`
	// are both two lines, at status 2 — the count of operands that failed,
	// which is this shell's rule already. Measured 2026-09-26 (#4666).
	s.KillKeepsGoingPastAJobSpecThatNamesNoJob = interp.Yes
	// The operands are read in the order they were written.
	s.KillReadsJobSpecsBeforeTheOtherOperands = interp.No
	// The jobs-at-exit sentence is chosen by the first job in the table and
	// not by preferring a stopped one: with `%1` running and `%2` stopped
	// this shell says `you have running jobs.`, and the other way about it
	// says `you have suspended jobs.` Measured 2026-09-26 (#4544).
	s.JobsAtExitSentenceFollowsTheTableOrder = interp.Yes
	// A trim on `$@` runs over each field, as it does in bash.
	s.OperatorDistributesOverTheFieldList = interp.Yes
	// OPTIND names the word until its last letter has been read.
	s.GetoptsCountsTheWordAtItsFirstLetter = interp.No
	// The other end of that question: the word is not counted when its last
	// letter is read either. OPTIND stays on it and the *next* call moves
	// the count, so the parameter lags a word behind the scan all the way
	// along and catches up only where the options run out. An option that
	// took an argument is the exception and counts as it does everywhere
	// else (#3275).
	s.GetoptsCountsTheWordOnTheNextCall = interp.Yes
	// And the run that reports "no more options" leaves the name holding
	// whatever it held, where the other six write `?`. After a scan that
	// read something that is the last letter still standing, which is how it
	// reads as a written value and is not one.
	s.GetoptsEndOfOptionsNamesIt = interp.No
	// A `case` that runs no command ends on 0: measured 2026-10-16 on
	// zsh 5.9.2, `case a in $(false)) echo m ;; esac; echo $?` writes 0 (#6710).
	s.CaseWithoutACommandKeepsTheSubstitutionStatus = interp.No
	// A produced integer reads in the base the script declared for it:
	// measured 2026-10-16, `typeset -i2 LINENO; echo $LINENO` is `2#1` and
	// `typeset -i16 RANDOM` reads `16#…` (#6758).
	s.ProducedIntegerReadsInItsBase = interp.Yes
	// A `+`-prefixed word is an option word here, and this shell is alone in
	// it: `getopts a o +a` is 0 with `+a` in the name, the sign kept so a
	// script can tell it from `-a`. `+ab` clusters, `+a val` and `+aval` take
	// the argument an `a:` letter takes, and a letter the string does not
	// have is `bad option: +z` with `+z` in OPTARG under the silent form. A
	// lone `+` is still an operand and `++` is the option `+` rather than an
	// end-of-options word. Measured 2026-09-17 on 5.9.2.
	s.GetoptsTakesAPlusPrefixedOption = interp.Yes
	// OPTIND is local to a shell function here: the call starts at 1 and the
	// caller's position — words and the place inside a clustered word alike —
	// comes back on return. It is what lets this shell's own function
	// library parse options without resetting OPTIND by hand.
	s.GetoptsFunctionPosition = interp.GetoptsFunctionPositionIsLocal
	// A call that unsets OPTIND and assigns it again still hands the
	// caller's back: measured 2026-10-15, `f() { unset OPTIND; OPTIND=3; };
	// f; echo $OPTIND` is 1 (#6633).
	s.OptindUnsetInACallStaysUnbound = interp.No
	// Once unset, OPTIND is never written by `getopts` again, and the scan
	// goes on from its own place: measured 2026-10-16, `getopts ab o -a -b;
	// unset OPTIND; getopts ab o -a -b` reads `b` and leaves the name unset,
	// in a function and after it (#6660).
	s.GetoptsLeavesAnUnsetOptindUnset = interp.Yes
	// The same answer by a second route, and recorded rather than left to
	// the axis above: every call here has its own cursor whether or not
	// anything was declared, so a declaration inside one cannot be the
	// thing that loses the caller's place.
	s.GetoptsLocalOptindRestoresTheCursor = interp.Yes
	// `#` in an option string is another option letter here. Measured
	// 2026-09-16, zsh 5.9.2 (#2947).
	s.GetoptsOptionStringHasANumericType = interp.No
	// OPTERR is an ordinary variable here: measured 2026-09-17 on 5.9.2, a bad
	// option is one line on stderr with it set to 0 as with it set to 1.
	s.GetoptsOptErrSilencesTheComplaint = interp.No
	s.GetoptsClearsOptarg = interp.Yes
	// And the same for an option that simply takes none, which is the other
	// axis and the one this shell shares with dash and BusyBox ash.
	s.GetoptsEmptiesOptargForAnArgumentlessOption = interp.Yes
	// OPTARG and OPTIND are written through a freeze without a word, as in
	// ksh93 — measured 2026-09-16 on 5.9.2.
	// The last option's argument stands after the scan runs out, as in dash.
	s.GetoptsUnsetsOptargAtEndOfOptions = interp.No
	// The clearings this shell does make write an empty string through the
	// freeze rather than removing the name, so the freeze is still on
	// afterwards.
	s.GetoptsClearingOptargIsARealUnset = interp.No
	s.GetoptsOwnParametersIgnoreAFreeze = interp.Yes
	s.GetoptsRefusedWriteEndsTheBuiltin = interp.No
	// Unreachable while GetoptsEndOfOptionsNamesIt is no: there is no write
	// at the end of the options for a freeze to refuse, so `N=kept; readonly
	// N; set -- x; getopts ab N` is silent at 1 — measured 2026-09-17 on
	// 5.9.2. Answered so that nothing reports an axis this shell cannot be
	// asked.
	s.GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal = interp.No
	// A refused store ends the shell and leaves 0 from `-c`, as the other
	// operand stores do: measured 2026-10-15, `typeset -r v=1; set -- -a;
	// getopts a v` is the refusal and status 0, and 1 from a script file
	// (#6588).
	s.GetoptsStoreRefusalLeavesZero = interp.Yes
	// A value `read` writes to a typed name that will not evaluate ends the
	// script: measured 2026-10-15 on zsh 5.9.2, `typeset -i n=5; read n <<<
	// 1+` is `bad math expression` and nothing after it.
	s.ReadTypedValueFailureIsAnAssignmentFailure = interp.Yes
	// And that cost is the script, from a file as from `-c`: measured
	// 2026-10-15, `typeset -i n=5`, then `n=1+; echo same`, then `echo next`
	// in a script file is the complaint and nothing after it (#6631).
	s.TypedValueFailureGivesUpTheLine = interp.No
	// unanswered GetoptsTypedValueFailureIsAFreeze: `?` is a value a typed
	// name takes here — `typeset -i n; getopts x n -y` leaves 0 — so the
	// write never fails and there is nothing to ask.
	// Neither `read` axis is reachable here: this is the one column whose
	// refused write inside a builtin ends the script, and that is asked
	// first. Answered so that nothing reports an axis this shell cannot be
	// asked, the way the getopts twin above is.
	s.ReadRefusedWriteEndsTheBuiltin = interp.No
	s.ReadRefusedWriteIsOneOnTheLastName = interp.No
	// The freeze on the *name* is reached, and this is the one shell where a
	// builtin's refused write ends the script: `readonly o; getopts a: o;
	// echo reached` prints neither `reached` nor anything after it.
	s.ReadonlyRefusalInABuiltinIsFatal = interp.Yes
	// Measured 2026-09-12: `OLDPWD=/usr zsh -c 'echo $OLDPWD'` answers `$PWD`,
	// so an inherited value never arrives at all — the name is this shell's own
	// record of where it has been, and it starts where the shell started. That
	// is why `OLDPWD=/nonexistent zsh -c 'cd -'` says nothing and reports 0:
	// there is no unusable value to refuse.
	s.InheritedOldpwd = interp.InheritedOldpwdIgnored
	// The depth, counted and told to every child, with no ceiling: measured
	// 2026-09-16, zsh 5.9.2 takes an inherited 9999 to 10000 and says
	// nothing. See interp.ShellLevelPolicy.
	s.ShellLevel = interp.ShellLevelCounted
	// Read from the front, in the base its prefix names: `SHLVL=1+RANDOM`
	// is counted from 1. Measured 2026-10-02 on 5.9.2 (#5145).
	s.ShellLevelReading = interp.ShellLevelReadsTheFrontInItsBase
	// And a shell that replaces this process stands in its place: measured
	// 2026-09-18, `exec /usr/bin/env` hands over `SHLVL=0` where this shell
	// holds 1. With no floor, so an inherited `-1` reaches the replacement as
	// `-1` and it reads 0 — which is where this column parts from bash's
	// answer to the same question.
	s.ShellLevelExec = interp.ShellLevelExecNotCounted
	// What an interrupt from outside does while a foreground program runs.
	// See interp.Semantics.InterruptWaitsForTheProgram (#5416).
	s.InterruptWaitsForTheProgram = interp.InterruptEndsTheShell
	// And the starting directory is named by what the kernel reports, however
	// the parent spelled it.
	s.StartupPwdName = interp.StartupPwdNameFromTheKernel
	s.CdWithoutHomeIsAnError = interp.No
	// A shell started with no `HOME` at all seeds one from the password entry
	// of the user the process runs as, and then a `cd` with no operand goes
	// there. Measured 2026-09-26 on zsh 5.9.2: `env -u HOME zsh -c 'print -r
	// -- $HOME'` writes the entry and the same line under the name `sh` writes
	// nothing, which is why this is swapped by the emulation as well — see
	// dialect/zsh/emulate.go and Semantics.StartupFillsAnAbsentHome (#4654).
	s.StartupFillsAnAbsentHome = interp.Yes
	// And once a shell has had a home, removing the parameter leaves it an
	// empty destination rather than no destination: `HOME=/tmp; unset HOME;
	// cd` is a silent 0 here and `HOME not set` at 1 in bash and ksh93, where
	// a shell that has *never* had one says `HOME not set` in every column.
	// The same answer under every emulation, which is what keeps it off the
	// table beside cdNowhere. See Semantics.CdRemembersAHomeThatWasUnset.
	s.CdRemembersAHomeThatWasUnset = interp.Yes
	s.CdDashPrintsTheDirectory = interp.No
	// And it goes where the shell last was, whatever OLDPWD has been set to:
	// `cd /; OLDPWD=/usr; cd -` goes back. Measured 2026-10-03. See
	// interp.Semantics.CdDashFollowsTheShellsOwnRecord.
	s.CdDashFollowsTheShellsOwnRecord = interp.Yes
	// The same as bash, and it is the chunk `B01cd.ztst` ends on. Measured
	// 2026-09-26 on zsh 5.9.2 (`-f`): with `d` renamed to `e`, `cd .` is 0
	// and `$PWD` becomes `…/e` while a bare `pwd` goes on printing `…/d` —
	// the kernel's name reaches `$PWD` at the `cd` and nowhere else. See
	// Semantics.CdDestinationIsNotThere.
	s.CdDestinationIsNotThere = interp.CdDestinationNotThereEntersAndTakesTheKernelsName
	// `cd nosuch/..` is refused, where `cd real/..` is 0 — every component a
	// `..` cancels is looked at first. That is what takes `cd ..` out of a
	// directory somebody has renamed into the directory the shell is holding
	// rather than into whatever has taken the old name. See
	// Semantics.CdCancelsADotDot (#4627, #4668).
	s.CdCancelsADotDot = interp.CdDotDotLooksAtEveryCanceledComponent
	s.PrintfAssignsWithV = interp.Yes
	s.PrintfRejectsUnknownOption = interp.No
	// zsh reads no options here: `trap -p` sets a trap whose action is the
	// word `-p`, and the failure surfaces when it fires.
	s.TrapParsesOptions = interp.No
	s.TrapOneArgumentIsACondition = interp.Yes
	// And the one column that refuses a number the table cannot name.
	// Measured 2026-09-19 on linux/arm64, zsh 5.9 in Debian bookworm, the
	// script file `trap 'echo CAUGHT' $1; kill -$1 $$; echo SURVIVED`: 15
	// prints CAUGHT then SURVIVED, and 40 and 64 are `undefined signal: N`
	// with the shell then dying of the signal at 128 + N. Written here rather
	// than left to the preset, because the value a preset happens to hold is
	// not a measurement and the next reader cannot tell the two apart.
	s.TrapTakesASignalNumberItCannotName = interp.No
	s.TrapReportsAnUnknownSingleCondition = interp.No
	// ERR and DEBUG but not RETURN, and both follow the script everywhere:
	// into functions, and — alone in the panel — into subshells and command
	// substitutions, where the handler's output is captured with the rest.
	s.TrapHasErrCondition = interp.Yes
	s.TrapHasDebugCondition = interp.Yes
	s.TrapHasReturnCondition = interp.No
	// And ERR answers to `ZERR` as well, which is the name zsh's own
	// documentation gives it and the one a startup file writes. One slot
	// under two names: `trap 'echo E' ERR; trap - ZERR; false` fires nothing
	// (#2771).
	s.TrapErrConditionIsAlsoZERR = interp.Yes
	// zsh alone reads a *function* name as a condition: `TRAPZERR() { … }`
	// installs the ZERR handler with no `trap` command anywhere, and
	// `TRAPINT`, `TRAPEXIT` and `TRAPDEBUG` do the same for theirs. It is
	// the spelling that fails silently in a shell without it, because a
	// declaration parses everywhere — see interp/trapfunction.go.
	s.TrapIsNamedByAFunction = interp.Yes
	s.ErrTrapRunsInsideFunctions = interp.Yes
	s.ErrTrapRunsInSubshells = interp.Yes
	// unanswered ErrtraceReachesSubshells: see the axis; never reached here.
	// One failure, one firing, however deeply the command that failed was
	// called: `f(){ g; }; g(){ h; }; h(){ false; }; f` writes a single E,
	// and with the action printing `${funcstack[*]}` it reads `h g f` — so
	// the firing is inside the frame that failed and none of the three
	// calls is an event of its own. Read from the frame rather than counted:
	// a `return 1` judged at the call instead would write the same one E,
	// and only the action's view of the locals, the arguments and the stack
	// says which happened. A subshell still fires on both sides, which is
	// the axis above and not this one.
	s.ErrTrapRefiresForTheCommandItFiredInside = interp.ErrTrapFiresOnceForTheFailure
	// What a ^C at the prompt does with a trap on INT: zsh keeps the line unless the INT trap's action returns non-zero.
	// See Semantics.PromptInterruptTrap.
	s.PromptInterruptTrap = interp.PromptInterruptTrapDecides
	// An error inside a handler ends the handler and nothing more, for a
	// signal's, ERR's and DEBUG's alike; and an error the shell gives up
	// over raises ZERR first. Measured 2026-10-02 on 5.9.2 under `-f`, see
	// interp.HandlerErrorReach and the second axis's own comment (#5360).
	s.AHandlersErrorEnds = interp.HandlerErrorEndsTheHandler
	// Several handlers waiting together run in the order their signals came
	// in. Measured 2026-10-31 on 5.9.2; see interp.WaitingHandlerOrder
	// (#7039).
	s.WaitingHandlersRun = interp.WaitingHandlersRunAsTheyArrived
	// And one of them that lost its trap to an earlier one before its turn
	// runs nothing: `trap - USR2` in the handler of a HUP that came first
	// goes on with no USR2. Measured 2026-11-01 on 5.9.2; see
	// interp/waitingtrapgone.go (#7170).
	s.WaitingSignalWithoutItsTrapIsResent = interp.No
	// And one that arrived again while it waited runs once per arrival:
	// `kill -USR1 $$ $$` is `T T after`, as are two `/bin/kill`s 0.2 s apart
	// during one `/bin/sleep`. Measured 2026-11-01 on 5.9.2; see
	// interp/waitingsignalagain.go (#7166).
	s.WaitingSignalArrivingAgainRunsOnce = interp.No
	s.ErrTrapFiresForAnErrorTheShellGaveUpOver = interp.Yes
	// A pipeline whose last element ran here is judged once, by its status,
	// unless that element is a compound whose body judged itself: `true | {
	// false && true; }` carries on under `set -e`.
	s.FailingPipelineWhoseLastElementRanHere = interp.PipelineJudgedUnlessItsLastElementJudgedItself
	s.DebugTrapRunsInsideCalls = interp.Yes
	s.DebugActionArmingErrExitSkipsTheCommand = interp.Yes
	s.DebugTrapRefiresOnEnteringAFunction = interp.No
	// Every compound head, once each — `if`, `while`, a group, a subshell,
	// `repeat` and a function *definition* among them — and a loop's passes
	// are inside that once rather than beside it.
	s.DebugTrapCompoundHeads = interp.DebugTrapHeadsEveryCompound
	// Never reached: one head fires for the whole loop.
	s.DebugListLoopHeadKeepsTheLine = interp.No
	// A pipeline is one statement here and fires once, in the shell running
	// it, however many elements it has — and no element fires a head of its
	// own. Measured: `trap 'echo d' DEBUG; echo a | tr a-z A-Z` writes
	// `d A`, and a four-element pipeline still writes one. Commands nested
	// *inside* an element are commands in their own right and go on firing:
	// `echo A | { cat; }` writes one for the pipeline and one for the
	// `cat`.
	s.DebugTrapPipelines = interp.DebugTrapPipelineOnceForThePipeline
	// And once for a whole `&&`/`||` list, which is the same reading one
	// layer further out: measured 2026-09-25 on 5.9.2 under `-f`, `print x
	// && print y` writes one firing where bash and ksh93 write two, and
	// `$ZSH_DEBUG_CMD` at it reads the whole list back rather than the first
	// operand (#4556). An operand that is a compound fires no head of its
	// own either; what is inside one fires as usual.
	s.DebugTrapSublists = interp.DebugTrapSublistOnceForTheList
	s.DebugTrapRunsInSubshells = interp.Yes
	// Ahead of the command, which is `DEBUG_BEFORE_CMD` on — zsh's own
	// default, measured 2026-09-25 on 5.9.2 under `-f`. This is the only one
	// of these axes a script can move: `unsetopt DEBUG_BEFORE_CMD` puts
	// every firing behind the command it would have preceded, so the entry
	// in setopt.go writes this axis rather than remembering the request
	// (#4473). See Semantics.DebugTrapRunsBeforeTheCommand for the grid.
	s.DebugTrapRunsBeforeTheCommand = interp.Yes
	// The listing least is kept of: `(trap)` and `$(trap)` show nothing the
	// parent had — not even an ignored signal, though it stays ignored in
	// fact and one the subshell sets itself is shown. A pipeline element is
	// the exception and keeps the signal listing while still dropping the
	// EXIT trap from it: `trap 'echo x' USR1; trap | cat` prints the trap
	// and the same shape with an EXIT trap prints nothing. Measured, each.
	s.PipelineElementKeepsTrapListing = interp.Yes
	s.ALoneTrapCommandKeepsTrapListing = interp.No
	s.KeptTrapListingIncludesExit = interp.No
	s.SubshellHidesInheritedIgnoredTraps = interp.Yes
	s.UmaskPrintsFourDigits = interp.No
	s.UmaskSetWithSPrints = interp.No
	s.UlimitBlockIsKilobyte = interp.No
	// An empty operand is a limit of nought: `ulimit -n ""` is silent at 0
	// and leaves `ulimit -n` answering `0` (#3064). Measured 2026-09-18.
	s.UlimitEmptyOperandIsZero = interp.Yes
	s.UlimitSetsBothLimits = interp.No
	// zsh takes `hard` and refuses `soft`, which is why the two words are
	// two axes: `ulimit -n soft` here is `invalid number: soft`.
	s.UlimitTakesHardKeyword = interp.Yes
	s.UlimitTakesSoftKeyword = interp.No
	s.UlimitOperandIsArithmetic = interp.No
	s.BadOptionToSpecialBuiltinFatal = interp.No
	// And this shell's POSIX mode does not move it, which is the departure
	// from the standard's preset and from every other dialect here. The mode
	// has one door in this shell — there is no `set -o posix`, `set -o posix`
	// is `no such option` at 1 — so it is the name that reaches it, and a
	// symlinked `sh` answers `bad option: -x` and then `alive` at 0, exactly
	// as this shell does under its own name. Measured 2026-09-13 for both
	// `shift -x` and `export -q`.
	//
	// Not a claim that the mode does nothing here: the same shell under the
	// same name *does* move RedirectErrorOnSpecialBuiltinFatal, stopping on
	// `export x > /nonexistent/dir/f` where it carries on as `zsh`. Two axes,
	// one mode, opposite answers — which is why this is a field a dialect
	// fills in rather than a constant in SetPosixMode (#2583).
	s.BadOptionToSpecialBuiltinFatalInPosixMode = interp.No
	// `alias` is no more special here than POSIX makes it: the complaint is
	// said and the next command runs. Measured with `alias -g x`.
	s.AliasBadOptionFatal = interp.No
	// unanswered AliasNameCheckReachesALookup, AliasInvalidNameFatal: this
	// shell checks no alias name at all, so AliasNameRefusedCharacters is
	// empty and neither question is ever reached. Measured 2026-09-12:
	// `alias 'a b'=echo` is accepted in silence here and the name is listed
	// back, where the two shells that check refuse it (#2413).
	s.EarlierDeclarationLetterBlocksALaterPlus = interp.No
	// A letter written under both signs: see
	// interp.Semantics.ALetterUnderBothSignsComesOff (#5673).
	s.ALetterUnderBothSignsComesOff = interp.No
	// And the mirror, a minus word after a plus one: see
	// interp.Semantics.EarlierPlusMakesALaterLetterARemoval (#5667).
	s.EarlierPlusMakesALaterLetterARemoval = interp.No
	// Nor does a redirection that cannot be made end anything: the message
	// is printed and the script runs on. The starting value only — `emulate
	// sh` and `emulate ksh` move it to the POSIX answer, and `emulate zsh`
	// puts it back.
	// Read as a number like any other; nothing about the width is
	// refused.
	s.MultiDigitDuplicationTargetIsAnError = interp.No
	s.RedirectErrorOnSpecialBuiltinFatal = interp.No
	// A substitution's body does hold `-e`, which is the neighboring axis
	// answering the other way: measured 2026-09-15,
	// `set -e; echo "end[$(false; echo no)]"` is `end[]` in zsh 5.9.2 under
	// every `emulate`, and under the name `sh` too. So nothing moves it and
	// emulate.go leaves it alone.
	s.ErrExitEntersACommandSubstitution = interp.Yes
	// The same csh spelling, and one step further: a word that expanded to
	// nothing is a name too, so `>&""` opens the empty path and fails on it
	// rather than complaining about a descriptor.
	s.GreatAmpTarget = interp.GreatAmpTargetNamesAnyFile
	// zsh has no move operator, and the absence shows in two different
	// answers rather than one refusal. `<&` wants a number and says `file
	// number expected` at 1; `>&5-` is a word that names no descriptor, so
	// GreatAmpTarget above sends it to a *file* called `5-` — measured, and
	// the file really appears.
	s.FdMove = interp.FdMoveIsNotAnOperator
	// unanswered FdMoveSuffixMustBeWritten: there is no move operator, so
	// no `-` is read as one however it arrived (#6746).
	// And the reading side ends the shell, on a builtin alone.
	s.DuplicationTargetError = interp.DuplicationTargetErrorEndsTheShellOnABuiltin
	// A target a process substitution begins with more after it is refused
	// before anything is opened — `echo hi > >(cat)x` is `invalid syntax for
	// process substitution in redirection` — and ends the shell on the
	// boundary above. Measured 2026-10-16 on 5.9.2 (#6766). See
	// interp.Semantics.SubstitutionTargetWithMoreIsRefused.
	s.SubstitutionTargetWithMoreIsRefused = interp.Yes
	// A redirection target's substitution body: `set -x; cat < <(echo hi)` writes no body line, measured
	// 2026-10-16 (#6799). See
	// interp.Semantics.RedirectTargetSubstitutionBodyIsTraced.
	s.RedirectTargetSubstitutionBodyIsTraced = interp.No
	// zsh takes it and sets a global instead of refusing.
	s.LocalOutsideAFunctionIsAnError = interp.No
	// Fatal to all three, which is the one place zsh is stricter than bash
	// about a builtin's failure. It refuses fewer operands, though, and the
	// two sets it adds have only `0` in common: `export ?` is quiet and
	// `unset ?` is not, `unset 12` is quiet and `export 12` is not.
	s.BadNameToDeclarationFatal = interp.Yes
	s.BadNameToUnsetFatal = interp.Yes
	// And fatal to `read` as well, which no other member of the panel is:
	// `read 1bad; echo after` prints the refusal and stops, in a function and
	// in a subshell alike. It is a field of its own because `read` is not a
	// special builtin anywhere, so this is not the special-builtin rule
	// reaching it — the two shells whose special builtins are fatal both
	// carry on past this one.
	s.BadNameToReadFatal = interp.Yes
	// And `printf -v` ends it too: measured 2026-09-17,
	// `printf -v '1x' %s Q` is `not an identifier: 1x` and nothing after
	// it runs, the same as `read '1x'`. `a-b` gets that sentence as well,
	// which is why there is no BuiltinBadNameNumeric row beside printf's.
	s.BadNameToPrintfFatal = interp.Yes
	// And at `getopts`, measured 2026-09-18: `echo A; getopts x 1bad -x;
	// echo B` writes A and `not an identifier: 1bad` and never reaches B,
	// where the other six columns all run it.
	s.BadNameToGetoptsFatal = interp.Yes
	// A subscripted operand is a name here, as it is at `read` — measured
	// 2026-09-18, `getopts x 'o[1]' -x` is 0 with the letter in `$o`. The
	// `o[0]` spelling is `o: assignment to invalid subscript range`, which
	// is this shell's one-based arrays refusing the *position*; asking only
	// that spelling would have read this column as a refusal (#3555).
	s.GetoptsOperandTakesASubscript = interp.Yes
	// Fatal here too, and unlike bash's this does not move: `emulate sh`,
	// `emulate ksh` and `emulate zsh` all stop. It is the axis that keeps
	// this question apart from RedirectErrorOnSpecialBuiltinFatal, which zsh
	// answers the other way.
	s.UnsetReadonlyFatal = interp.Yes
	s.DeclarationNameOperands = interp.NamesAndSpecialParameters
	s.UnsetNameOperands = interp.NamesAndPositionals
	// A third set for a third builtin: `read 1` fills `$1` here, where
	// `export 1` is refused — and `read ?` is taken too, though for the
	// other reason below rather than as a name. `read 1bad`, `read a-b`,
	// `read "a b"` and `read -- -x` are each `not an identifier`.
	s.ReadNameOperands = interp.NamesAndPositionals
	// The prompt operand, taken from ksh93 and made kinder: `read "?Press
	// enter"` prompts and reads into REPLY where ksh93 refuses the empty
	// name. That is why `read ?` is quiet here — the word is a prompt, not a
	// name.
	s.ReadPromptOperand = interp.ReadPromptAloneNamesTheDefault
	// The line is read before the operand is judged, so the refusal has eaten
	// it: `printf 'AAA\nBBB\n' | { (read 1bad); cat; }` prints only BBB. In
	// a subshell because the refusal is fatal here, which is the only way to
	// have a reader left to ask.
	s.ReadRefusesABadNameBeforeReading = interp.No
	s.ReadJudgesEveryNameBeforeReading = interp.No
	// ReadCountJudgesTheNamesAfterTheFirst is left unanswered here too, and
	// so is ReadCountFillsTheFirstNameWhole, for the reason below.
	// unanswered ReadCountFillsTheFirstNameWhole: see the line above.
	// unanswered ReadCountOfZeroIsNoCount: the same reason — no `-n` here is
	// a count, so `read -n 0 v` never reaches a count of zero.
	// unanswered ReadCountOfAPipeExecOpenedFindsItsEnd: the same
	// reason. Its count is `-k`, which reads a pipe at `-u 3` as any read
	// does — `read -k 4 -u 3 l` takes abcd and the next read ef, measured
	// 2026-10-31 on zsh 5.9 — and never reaches the question.
	// unanswered ReadCountAtTheEndSticksTheDescriptor: the same reason; a
	// `-k 4 -u 3` at the end is 1 and the read after it 1 again.
	// unanswered ReadExactCountGivenUpKeepsWhatItTook: `read -N` is a bad
	// option here, and a read goes on through a trapped signal besides.
	// This shell's `-n` is a flag rather than a count and its `-k` reads
	// from the terminal — `printf 'XYZW\n' | zsh -c 'read -k 3 a'` answers
	// `not interactive and can't open terminal` — so no count ever reaches
	// the operands on this route.
	// `export a[1]=v` sets the element and reports success here, measured
	// 2026-09-07 — and so does `readonly a[1]=v` as far as the *name* check
	// goes, which then refuses it for a reason about elements rather than
	// about names. This read No, which was never reachable: the operand was
	// glob-matched before the builtin saw it and the line died as
	// `no matches found` (#1203).
	s.DeclarationTakesASubscript = interp.Yes
	// The element is written and the array's own attributes are left
	// alone: measured 2026-09-17, `a=(1 2 3); export 'a[1]'=v` leaves
	// `typeset -a a=( v 2 3 )` with no `x` on it and nothing in a child's
	// environment, and `export 'a[1]'` with no value agrees.
	s.ExportThroughASubscriptedOperandRecordsTheLetter = interp.No
	// The declaration builtins take one: `typeset a[1]=v` sets the element
	// and reports success, measured.
	s.TypesetTakesASubscript = interp.Yes
	s.UnsetTakesASubscript = interp.Yes
	// The same here: one element removed, the rest standing.
	s.UnsetElementEmptiesAnUnwrittenArrayInASubshell = interp.No
	// `read 'a[2]'` fills the element, measured 2026-09-10 on `a=(x y z)`.
	s.StoreOperandTakesASubscript = interp.Yes
	// The brackets name the whole array and the operand fills it, silently
	// and at 0: measured 2026-09-17, `r=(1 2 3); read 'r[@]'` on `Y`
	// leaves `r` as the one element `Y`. Not a split — `read 'r[@]' b` on
	// `X Y` leaves `r` as the one element `X` and fills `b` with `Y` — so
	// the operand takes its field like any other name and the spelling is
	// what makes the array one element, exactly as `r[@]=X` does.
	s.StoreOperandWholeArraySubscript = interp.StoreOperandWholeArraySubscriptNamesEveryElement
	// Over a **table** it is refused by name and the input ends: measured
	// 2026-09-17, `typeset -A m=(k v); read 'm[@]'` is `m: attempt to set
	// slice of associative array` and nothing after it runs. So this shell
	// takes the array and refuses the table, the other way round from bash
	// — the same swap the bare assignment makes.
	s.StoreOperandWholeArraySubscriptOverATable = interp.WholeArraySubscriptIsASliceOfATable
	// An element is not a name, so a declaration that would freeze, type or
	// localize the *array* refuses the operand instead of doing half of it —
	// and it ends the script. Three wordings for the three, measured
	// 2026-09-07, which is how a script tells which of them it ran into.
	// The same as ksh93 and for the same measurement, `unset` included: a
	// fatal `unset ":" ok1 ok2` removes ok1 and ok2 before it stops.
	s.BadNameDeclaresTheOperandsAfterIt = interp.Yes
	// Declared in full, letters and freeze included: `{ readonly x+=5 z=2 }
	// always { … }` leaves `z` frozen and `x` untouched. Measured 2026-10-27
	// on zsh 5.9.2 (#6510).
	s.BadNameEndsTheDeclaringNotTheAssigning = interp.No
	s.SubscriptedOperandTakesTheIntegerAttribute = interp.No
	// And the container letter, refused in the same breath — `typeset -A
	// m[k]=v` and `typeset -a n[2]=v` alike are `inconsistent type for
	// assignment`, and so is the letter over a name already a table of that
	// very kind, so it is the letter beside the subscript that is refused
	// rather than anything the letter would change.
	s.SubscriptedOperandTakesTheContainerAttribute = interp.No
	s.SubscriptedOperandTakesALocalDeclaration = interp.No
	s.ReadonlyElement = interp.ReadonlyElementRefused
	// `unset a[@]` replaces the elements with a single empty one, which is
	// this shell's reading of `unset` on a span rather than a special rule
	// for `[@]`: `unset a[2]` leaves an empty element in place too. A scalar
	// is one such span and comes back empty; an array with nothing in it has
	// no span and gains no element.
	s.UnsetArraySpan = interp.UnsetArraySpanLeavesOneEmptyElement
	// A subscript on a name nothing has set is never looked at: measured
	// 2026-09-10, `$(( nodecl[1/0] ))` is a quiet 0 and `i=0;
	// $(( nodecl[i++] ))` leaves i at 0, where bash 5.3, bash 3.2 and ksh93
	// all divide by zero and all step i. Set-ness and not emptiness — `e=`
	// then `$(( e[1/0] ))` divides by zero here too.
	s.ArithSubscriptSkippedWhenNameUnset = interp.Yes
	// And on a name that *is* set, brackets with nothing between them are the
	// subscript machinery's own refusal rather than the expression parser's:
	// `invalid subscript`, and the expression produces no value. Measured
	// across every kind the name can be — an associative array, an indexed
	// array, a plain scalar, an integer, `PATH` — which is what says the
	// question is whether the name exists rather than whether it is an array.
	//
	// The pair is what `$(( m[$w] ))` with an empty `$w` runs into, because
	// the parameters go in before the expression is parsed: undeclared is 0
	// and declared is the refusal, and this shell was giving `bad math
	// expression` to both (#1745).
	s.EmptyArithSubscript = interp.EmptyArithSubscriptIsInvalid
	// And the parameter site, which is the same sentence and a stronger
	// rule: `invalid subscript` whether or not the name exists, where the
	// arithmetic site is a silent zero on a name nothing declared. The
	// expansion is refused and the input ends (#1763).
	s.EmptyParamSubscriptIsAnError = interp.Yes
	// An empty key is a key here too, though `m[""]` is not the way to reach
	// one: the subscript is not a quoting context, so those brackets hold a
	// two-character key and `m[$w]` with an empty `$w` holds the empty one.
	// Measured 2026-09-12, both store and the table ends with two elements
	// (#1938).
	s.EmptyAssociativeKeyIsAnError = interp.No
	// Either letter takes a name that is already the other kind, and the
	// elements are gone: measured 2026-09-12, `typeset -A h; h[k]=v;
	// typeset -a h` is `typeset -a h=(  )` at status 0 and the reverse is
	// `typeset -A a=( )`. The one column that converts in both directions,
	// and the only one that loses the values doing it (#1375).
	s.TableUnderAnArrayDeclaration = interp.CompoundKindChangeEmptiesTheName
	s.ArrayUnderATableDeclaration = interp.CompoundKindChangeEmptiesTheName
	// The literal form converts and empties in both directions too, the same
	// as the valueless one. Measured 2026-09-12, `typeset -A h; h[k]=v;
	// typeset -a h=(x)` lists `typeset -a h=( x )` at 0 and `typeset -a a=(x
	// y); typeset -A a=([k]=v)` lists `typeset -A a=( [k]=v )` at 0 (#2287).
	s.TableUnderAnArrayLiteralDeclaration = interp.CompoundKindChangeEmptiesTheName
	s.ArrayUnderATableLiteralDeclaration = interp.CompoundKindChangeEmptiesTheName
	// `a[@]=Z` names every element: the name comes out holding the one value
	// whatever it held before, and `a[@]+=Z` adds one at the end. Over a
	// table the same spelling is refused by name and the input ends.
	// Measured 2026-09-12, `x=(p q); x[@]=Z` leaves `${#x[@]}` at 1, `v=s;
	// v[@]=Z` and a bare `u[@]=Z` both leave a one-element array, and
	// `typeset -A m; m[k]=v; m[@]=Z` is `m: attempt to set slice of
	// associative array` under both separators (#2285).
	s.WholeArraySubscriptAssigningAnArray = interp.WholeArraySubscriptNamesEveryElement
	s.WholeArraySubscriptAssigningATable = interp.WholeArraySubscriptIsASliceOfATable
	// But written as a key in a table literal it is an ordinary key, which
	// is this column swapping sides between the two routes: measured
	// 2026-10-15, `typeset -A a; a=([k]=1 [*]=s)` lists `( ['*']=s [k]=1 )`
	// at 0 (#6686).
	s.WholeArraySubscriptKeyInATableLiteral = interp.WholeArraySubscriptIsAnOrdinaryKey
	// And the `(P)` route answers it the same way, which is the column that
	// does *not* swap sides between the two.
	s.WholeArraySubscriptThroughAReferenceToATable = interp.WholeArraySubscriptIsASliceOfATable
	// Nor is reading one reported: measured 2026-09-12, `typeset -A m;
	// m[k]=v; w=; ${m[$w]}` is the empty string at status 0 and silent
	// (#1972).
	s.EmptyAssociativeKeyIsReportedWhenRead = interp.No
	// Nor is its *length* refused: measured 2026-09-12, `typeset -A m;
	// m[k]=v; w=; echo "[${#m[$w]}]"; echo after` is `[0]`, `after` and
	// status 0, where bash stops the script on the same line (#2286).
	s.EmptyAssociativeKeyRefusesTheLength = interp.No
	// An empty positional list is a **set** parameter here, with dash and
	// against the four bash-and-ksh columns: measured 2026-09-12, `set --;
	// "${@-word}"` is empty and `"${@+word}"` is `word` (#1941).
	s.PositionalListWithNoneIsSet = interp.Yes
	// And a fired `+` on an empty list is one empty field: `set --; set --
	// "${@:+w}"` leaves one. See Semantics.AFiredAlternateOnAnEmptyListIsOneField.
	s.AFiredAlternateOnAnEmptyListIsOneField = interp.Yes
	// unanswered SubstitutionRunsTheBodyReadWithItsLine: every body is read
	// when it runs here, so nothing was read with the line.
	// The third reading of what an empty `$@` takes with it, and the one that
	// makes the axis three-valued: only what stands *before* the list goes with
	// it, so `"$e$@"` is no argument and `"$@$e"` is one.
	s.EmptyListTakesTheWord = interp.EmptyListReachWhatStandsBeforeIt
	// The expand-first order, with dash and ksh93 (#1943).
	s.PrefixToAFrozenNameIsCheckedFirst = interp.FrozenPrefixCheckedWithTheCommand
	// And the frozen name's own value is evaluated before the refusal, so
	// `readonly r=1; r=$((1/0)) echo RAN` writes the division and never
	// names `r` (#4685).
	s.FrozenPrefixIsCheckedBeforeItsValue = interp.No
	// A `*` or `@` subscript inside an expression is the slice, joined and
	// then read as an expression: measured 2026-09-11 on 5.9.2, `typeset -A
	// m; m[k]=9; $(( m[*] ))` is 9 and `a=(1+1); $(( a[*] * 3 ))` is 6.
	s.ArithWholeArraySubscriptIsTheSlice = interp.Yes
	// So the question below it is never reached here: the slice is the
	// answer before anything can call the subscript bad. Answered rather
	// than left open because an unanswered axis is a refusal, and a reading
	// this dialect cannot reach must not be able to produce one (#1978).
	s.ArithWholeArraySubscriptIsReportedAsBad = interp.No
	// A subscript whose text expanded to nothing is not read as an
	// expression here at all: measured 2026-09-11 on 5.9.2, `a=(5 6 7); w=;
	// ${a[$w]}` is `bad math expression: empty string` and the shell ends,
	// where `${a[ ]}` is the expression running out. The written `${a[]}`
	// above is a third sentence again, which is why they are two axes.
	s.EmptySubscriptTextIsAMathError = interp.Yes
	// A literal's own reading of the same empty subscript, measured 2026-10-16
	// (#6738): see interp.Semantics.EmptySubscriptInAnIndexedLiteralIsBad.
	s.EmptySubscriptInAnIndexedLiteralIsBad = interp.No
	// And a literal whose element was refused where it would go places nothing
	// at all: measured 2026-10-30 on 5.9.2, `b=(7); b=(1 [-5]=x 3)` and `b=(1
	// [1+]=x 3)` both leave `( 7 )` for an exit trap to read (#6852). See
	// interp.Semantics.RefusedPlacementKeepsWhatTheLiteralPlaced.
	s.RefusedPlacementKeepsWhatTheLiteralPlaced = interp.No
	// And an appending literal holding any `[k]=v` is a run of element stores
	// from the first position: measured 2026-11-01 on 5.9.2, `b=(7 8 9);
	// b+=(x [5]=y)` is `( x 8 9 '' y )` (#7079). See
	// interp.Semantics.AppendingSubscriptedLiteralStoresFromTheFirstPosition.
	s.AppendingSubscriptedLiteralStoresFromTheFirstPosition = interp.Yes
	// Measured 2026-10-16 (#6781): see
	// interp.Semantics.DeclarationOperandSplitsAfterTheSubscript.
	s.DeclarationOperandSplitsAfterTheSubscript = interp.No
	// A subscript's expression stops at the first top-level `,` or `;` and
	// the rest of the text is discarded, unevaluated: measured 2026-09-12,
	// `a=(p q r s); i="2,3"; ${a[$i]}` is `q` where the comma operator would
	// give `r`, and `i="2,n=9"` leaves `n` at 0. The other half of the rule
	// that a comma has to have been *written* to separate a range: what
	// reaches an expression with one still in it arrived through a
	// substitution, and it separates nothing (#2160).
	s.SubscriptExpressionStopsAtASeparator = interp.Yes
	// And at a byte the arithmetic cannot read (#5567).
	s.SubscriptExpressionStopsAtAnUnreadableByte = interp.Yes
	// Whitespace between the brackets is refused too, and by a different
	// part of the shell: measured 2026-09-10, `a=(1 2 3); echo $(( a[ ] ))`
	// is `bad math expression: operand expected at end of string` — the
	// expression reader's own end-of-input sentence, where `a[]` is the
	// subscript machinery's `invalid subscript`. Only on an indexed name:
	// the same brackets on an associative one are a key made of spaces, and
	// on a name that is not set the subscript is never read at all (#1762).
	s.BlankArithSubscriptIsTheEmptyExpression = interp.No
	// `a[2]=(p q)` puts the words *where the subscript points* and the array
	// grows by one: `a=(x y); a[1]=(p q)` reads back `p q y`. Measured
	// 2026-09-07 against zsh 5.9.2, over the shapes that could have told a
	// different rule — an empty literal removes the element, `+=` appends at
	// the element rather than at the end, a subscript past the last element
	// pads with empties on the way, and `a[2]=([3]=p)` splices the three
	// positions that same literal would have filled on its own. bash refuses
	// the line and ksh93 builds a nested value, so this is an axis and not a
	// default (#1330).
	s.SubscriptedArrayLiteral = interp.SubscriptedArrayLiteralSplices
	// The complaint is the builtin's rather than the script's: `unset` reports
	// 1 and the next command still runs.
	s.BadSubscriptToUnset = interp.BadSubscriptReported
	// And a `test -v` operand's: see interp.Semantics.BadSubscriptToTestIsSet
	// (#5590).
	s.BadSubscriptToTestIsSet = interp.BadSubscriptEndsTheScript
	// A *store* through an operand is the opposite, and this is the column
	// the two fields exist for: measured 2026-09-17, `read 'r[1/0]' <<< Y`
	// in a script file writes `division by zero` and the script ends at 1,
	// where the identical expression handed to `unset` one line earlier only
	// left a failed builtin behind.
	s.BadSubscriptToAnOutputOperand = interp.BadSubscriptEndsTheScript
	// And a declaration ends it too, which is the site where zsh and ksh93
	// agree with each other and both part from bash: measured 2026-09-17,
	// `typeset "a[b c]"=v` in a script file writes `bad math expression` and
	// the script ends at 1, with nothing on a later line reached — from the
	// top level, a function, an `&&` list, an `if` condition and a loop body
	// alike (#3495).
	// The construct catches it and answers with its own status: measured
	// 2026-09-17, `(( a[b c] )); echo "same=$?"` prints `same=2` and the
	// line carries on, by both routes. The expansion `$(( a[b c] ))` in the
	// same shell ends the input, so the two constructs really do part here.
	s.BadSubscriptEscapesAnArithmeticCommand = interp.No
	// And an expansion that fails in the text is caught the same way: the
	// construct is traced and leaves its own 2, and the line carries on.
	// Measured 2026-10-31, `(( $((1/0)) + 1 )); echo "after st=$?"` prints
	// `after st=2`. See
	// interp.Semantics.FailedExpansionEscapesAnArithmeticCommand (#7189).
	s.FailedExpansionEscapesAnArithmeticCommand = interp.No
	s.BadSubscriptToADeclaration = interp.BadSubscriptEndsTheScript
	// An out-of-range store through each builtin, measured 2026-10-16 (#6835):
	// see interp.Semantics.OutOfRangeElementToAnOutputOperand.
	s.OutOfRangeElementToAnOutputOperand = interp.BadSubscriptEndsTheScript
	s.OutOfRangeElementToADeclaration = interp.BadSubscriptEndsTheScript
	// And a plain `a[]=6` ends it too, which is this column's answer
	// wherever a write walks into brackets it cannot use. Measured
	// 2026-09-20 on 5.9.2 from a script file: `a=(1 2 3); a[]=6; echo
	// same-line` writes `not an identifier: a[]` and exits 1 with nothing
	// after it reached, from the top level and from inside a function alike;
	// a `( … )` around it takes the refusal and the outer script lives to
	// read the array back untouched (#3949).
	s.EmptySubscriptToAnAssignment = interp.BadSubscriptEndsTheScript
	// And a valueless subscripted operand is the empty-value form under
	// another spelling, which is measured and not inferred: `a=(1 2 3);
	// typeset 'a[1]'` and `typeset 'a[1]='` both leave `( '' 2 3 )`,
	// `typeset 'a[9]'` grows the array to nine, `typeset 'a[0]'` is
	// `assignment to invalid subscript range`, and `typeset -A m; typeset
	// 'm[b c]'` puts the key in with an empty value. So the brackets are
	// read here, and a subscript that will not evaluate ends the script
	// through the axis above (#3501).
	s.ValuelessSubscriptedOperand = interp.ValuelessSubscriptedOperandWritesTheElement
	// And a name it has never heard of is left alone with its brackets
	// unread: `unset a "a[x+]"` is silent at 0, and `i=0; unset
	// "nodecl[i++]"` leaves i at 0. What counts as heard of is set-ness and
	// not emptiness — `v=` and an empty declared array both evaluate.
	s.UnsetSubscriptSkippedWhenNameUnset = interp.Yes
	// The status is the last *subscript's* and not the last operand's: with
	// an earlier `a[x+]` failing, `unset "a[x+]" "a[1]"` is 0 and `unset
	// "a[x+]" nope` is still 1, because a plain name carries no status.
	s.UnsetStatusIsTheLastSubscripts = interp.Yes
	// A negative subscript past the first element places one in front of it
	// rather than being refused: `a=(p q); a[-3]=x` is three elements with
	// `x` at the head, and `-4` and `-5` land in the same place. What this
	// shell refuses is `a[0]`, which is below its first element rather than
	// counting back from the last.
	s.NegativeSubscriptPastTheStartInserts = interp.Yes
	// A negative substring length whose end falls behind the offset is
	// refused, and the refusal ends the shell — `v=abcdef; ${v:1:-9}` is
	// `substring expression: -3 < 1` and nothing after it runs. The
	// computed **end and start** are what is named, where bash names the
	// length as written, so the two sentences share no verb. The control
	// either side: `${v:1:-2}` is `bcd` at 0 and `${v:1:-5}` — the end
	// exactly at the start — is empty at 0, so it is a strictly smaller end
	// and not a negative length. The list spelling draws the identical
	// sentence: `a=(p q r s t u); ${a[@]:1:-9}` is `substring expression:
	// -3 < 1` where `${a[@]:1:-2}` is `q r s`. Measured 2026-09-27 against
	// 5.9.2.
	s.SubstringEndBehindTheStart = interp.SubstringEndBehindStartIsRefused
	s.SubscriptBeforeTheFirstElementRead = interp.SubscriptBeforeStartIsNothing
	s.SubscriptBeforeTheFirstElementNeedsAnElement = interp.No
	// And the length is answered exactly as the read is — silently, at `0`.
	// Measured 2026-09-18 (#3591).
	s.SubscriptBeforeTheFirstElementRefusesTheLength = interp.No
	s.OperandSubscriptQuoting = interp.OperandSubscriptQuotesNothing
	s.ArithmeticOnlyBodyIsAnArithmeticExpansion = interp.No
	// `not valid in this context: a+` — the append operator is not a
	// declaration operand here.
	s.DeclarationTakesAnAppendOperand = interp.No
	// Nor with a subscript on the name: `typeset a[1]+=q` is
	// `not an identifier: a[1]+` at 1, which is the same refusal at a
	// different wording rather than a different answer.
	s.DeclarationTakesASubscriptedAppendOperand = interp.No
	// And the same operator on an *array literal* operand, which this shell
	// does not take: `typeset u+=(3 4)` is `not valid in this context: u+`
	// and the script ends.
	s.DeclarationTakesAnAppendingArrayOperand = interp.No
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.No
	// A stopped job keeps the current-job marker: measured 2026-09-12
	// through a pseudo-terminal with a scratch home directory, `sleep 40`
	// stopped with ^Z and then `sleep 41 &` lists `[1]  + suspended` and
	// `[2]  - running`, and `jobs %+` names the suspended one. #1563
	// recorded the opposite for this shell and it does not reproduce —
	// re-measured on zsh 5.9.2 with `-f`, this column agrees with bash.
	s.StoppedJobTakesTheCurrentJobMarker = interp.Yes
	// A `( … )` is a job of its own here, so its jobs are numbered from two
	// and never take the `+`. Measured — see the axis for the grid and the
	// four columns that say one (#5021).
	s.SubshellIsAJobInItsOwnTable = interp.Yes
	s.JobsListFinishedJobs = interp.No
	s.EndedJobIsListedAsRunningWithoutTheMonitor = interp.No
	// Off by default, and the one column in the panel where a script can
	// turn it on: `setopt LONG_LIST_JOBS` puts the job's pid in every notice
	// this shell writes, and the entry in setopt.go moves this axis rather
	// than remembering the request (#4491). Measured 2026-09-25 on 5.9.2
	// under `-fiV +Z` on a pseudo-terminal: `[1]  + done       :` with the
	// option off and `[1]  + 96801 done       :` with it on.
	s.JobNoticeNamesThePID = interp.No

	// `jobs`' letters: POSIX's pair, the state filters, `-d` — the directory
	// each job was started in, which is this shell's alone — and `-z` and
	// `-Z`, which are about the process title and ride
	// UnimplementedOptionLetters. `-n` and `-x`, which bash has, are bad
	// options here.
	s.JobsOptions = "dlprs"
	// The split this issue was about. zsh reads `-p` as "put the job's
	// process *group* id in the listing" and prints its ordinary rows,
	// where the other three print the ids and nothing else — so
	// `kill $(jobs -p)` is a bash idiom rather than a portable one.
	// unanswered JobsListsWhatChangedSinceTheLastReport: `jobs -n` is not a
	// letter this shell has — it is not in JobsOptions — so the axis is
	// never consulted here. ksh93 is the one column with the letter, and
	// bash's letter of the same name is a different question that stays
	// unimplemented (#3390).
	s.JobsPidsOnlyOption = interp.No
	// unanswered JobsPidsLetterOutranksTheLongLetter: this `-p` is not the
	// ids alone, so JobsPidsOnlyOption answers first and `-p` beside `-l` never
	// reaches the question.
	// Both filters at once list a job in either state here, where bash lets
	// the last letter given decide.
	s.JobsStateFiltersAccumulate = interp.Yes

	// Whether a `&` job's command appears in a `jobs` listing.
	s.JobsShowBackgroundCommand = interp.Yes
	// And the command is the statement reprinted, on one line of the body
	// arrangement. See Runner.jobCommandOnOneLine (#5342).
	s.JobCommandIsReprinted = interp.Yes
	s.JobCommandIsOneLineOfTheBodyLayout = interp.Yes
	// `${v/#%pat/rep}`: the two anchors together, or a start anchor over a
	// `%`. Measured 2026-10-02. See Semantics.ReplacementAnchorsCombine.
	s.ReplacementAnchorsCombine = interp.Yes

	// Whether a backgrounded job is announced to whoever is typing.
	// zsh announces a job only where standard input is a terminal, and not
	// on a pipe as the other two do. Answered as "announces" either way: the
	// difference is about what it is talking to rather than about the shell.
	// Whether `export -f` carries a function to a child.
	// Whether an unassigned subscript is an element.
	s.ArraysAreSparse = interp.No
	// An operator on `${a[*]}` applies to the joined string here, once:
	// `${a[*]#a}` on `(aa ab)` is `a ab` where the other two say `a b`.
	s.OperatorDistributesOverStarSubscript = interp.No
	// A replacement operand inside double quotes is double-quoted *content*
	// here, not a word of its own: measured 2026-09-07, `"${s/a/'$v'}"` on
	// `s=xay` and `v=VAL` is `x'VAL'y` — the quotes are two characters of the
	// result and the `$v` between them is still substituted — where bash 5.3
	// and ksh93 give `x$vy`. bash 3.2 agrees with this shell, which is what
	// makes it an axis rather than a fact about zsh.
	//
	// The same rule the *word* operand takes everywhere, applied one operand
	// further along: `\q` stays a backslash and a `q`, and a leading `~` does
	// not expand. Unquoted, this shell agrees with the rest again (#1209).
	s.ReplacementOperandTakesTheEnclosingQuoting = interp.Yes
	s.ExportCarriesFunctions = interp.No
	// No `-n` either, and unlike `-f` it is a letter this shell has simply
	// never heard of — so it earns the ordinary `bad option: -n` rather
	// than the ExportFunctionOptionRefused wording, and `export` fails at 1
	// with the script carrying on.
	s.ExportTakesTheAttributeOff = interp.No
	s.AnnouncesBackgroundJob = interp.Yes
	// But not with the monitor off: `unsetopt monitor` and a background job
	// is started in silence here, where bash and ksh93 still announce it.
	// Measured 2026-09-10 on a pseudo-terminal (#1738).
	s.AnnouncesBackgroundJobWithoutTheMonitor = interp.No
	// The hole gets refilled, and here it is a real hole first: measured
	// 2026-09-12, `jobs %2` is 127 after the middle job is killed and reaped,
	// 0 once a fourth job is started, and `%4` is 127.
	s.NextJobNumberRefillsAHole = interp.Yes
	s.ReportsACommandKilledBySignal = interp.No
	s.ReportsAnyKilledPipelineElement = interp.No
	s.ChildInterruptEndsTheScript = interp.No
	// An empty operand and an empty HOME are both somewhere here, as in
	// dash: `cd ""` moves to where the shell already is, which `chpwd`
	// sees, and `HOME=; cd` does the same.
	s.CdEmptyOperandIsAnError = interp.No
	s.CdEmptyHomeIsAnError = interp.No
	// `cd old new` rewrites the current directory's path here too, and
	// this shell moves in silence — the same split as `cd -`, where it is
	// the only one that prints nothing.
	s.CdSubstitutesTheOperands = interp.Yes
	s.CdSubstitutionPrintsTheDirectory = interp.No
	// Never asked, the form above having answered; recorded for the same
	// reason it is in ksh.
	s.CdRefusesExtraOperands = interp.Yes
	s.CdRefusesUnknownOption = interp.No
	// `cd -q` suppresses `chpwd` and `chpwd_functions` here, and nothing
	// else: measured, both ran on a plain `cd` and neither on `cd -q`, while
	// `cd -q -` still printed the directory. #1558.
	s.CdHasQuietOption = interp.Yes
	s.CdHasSymlinkFreeOption = interp.Yes
	// The hooks. `chpwd` is the one whose site is a builtin rather than the
	// prompt loop — see repl.HookStyle for the two that are not — and the
	// suffix is shared by every hook this shell has, wherever it fires.
	s.HookListSuffix = "_functions"
	s.DirectoryChangeHook = "chpwd"
	// Where this shell keeps the directories it has pushed, newest first and
	// **without the one it is standing in** — `dirs` puts `$PWD` in front of
	// this array rather than reading it out of it. Measured 2026-09-26 on zsh
	// 5.9.2 under `-f`: after one `cd` with `AUTO_PUSHD` on, `dirs -v` numbers
	// two entries and `$#dirstack` is **1**, not the 2 #4592 predicted.
	//
	// A plain array and not a produced view: this shell lets a script assign
	// to it, and the assignment *is* the new stack — `dirstack=(/tmp /usr)`
	// then `popd` arrives in `/tmp`. So the prelude's `pushd`, `popd` and
	// `dirs` keep it under this name and there is one stack rather than two.
	s.PushedDirectoriesParameter = "dirstack"
	// The default state of `AUTO_PUSHD`, which is off — `setopt autopushd` is
	// what moves this axis and nothing else does; see dialect/zsh/setopt.go.
	// Measured 2026-09-26: `unsetopt autopushd; cd sub; print $#dirstack`
	// writes 0, in the reference and here alike.
	s.CdPushesTheDirectoryItLeaves = interp.No
	// The other hook whose site is not the prompt loop: this one fires as the
	// shell ends, which is where a plugin tears down what it started. #2111.
	s.ExitHook = "zshexit"
	s.CdLastPathOptionWins = interp.No
	s.BadSetOptionNameFatal = interp.Yes
	// And the letter too, at zsh's own 1: `set -Z; echo one` prints
	// nothing and exits 1.
	s.BadSetOptionLetterFatal = interp.Yes
	// `set -o posix` is `no such option: posix` here, so the `sh` name is
	// the only door — and this is the shell whose `sh` emulation changes the
	// most, which is why it was asked rather than assumed. Measured 2026-09-14
	// through a symlink named `sh`: both spellings still stop at 1. The mode
	// moves neither, and this preset says so for the same reason it declines
	// the special-builtin move above (#2641).
	s.BadSetOptionNameFatalInPosixMode = interp.Yes
	s.BadSetOptionLetterFatalInPosixMode = interp.Yes
	// The other welding column, and it agrees with ksh93 word for word on
	// behavior: `set -oerrexit zzznosuch` is errexit with `zzznosuch` as $1,
	// and `set -oe` is `no such option: e`.
	s.SetOLetterAttachesItsName = interp.Yes
	// `zsh -o` with nothing behind it is `string expected after -o` at 1 and
	// no listing; `zsh -e -q` is `bad option: -q` at 1, exactly as `zsh -q`
	// is. Both measured 2026-10-03 on 5.9.2.
	s.InvocationBareOListsTheOptions = interp.No
	s.InvocationLetterRefusedUnderErrexitFails = interp.No
	// The option loop goes on *applying* the words behind one it refused,
	// which is the other half of the same loop from SetReportsEveryBadOption
	// and splits the panel the other way. Measured 2026-09-18 from a script
	// file under `env -i`: `set -Z -x -o` writes one refusal, then the whole
	// option table with `xtrace on` in it, and `set -e -Z -o` writes the
	// table with `errexit on`. The table is what the applying loop prints, so
	// a shell that stops at the bad word never writes one — bash, dash and
	// BusyBox ash do not, and ksh93 does not either although it is the column
	// that reports every bad word. The refusal is still as fatal as it was;
	// what carries on is the loop and not the script.
	s.SetAppliesTheWordsAfterARefusedOption = interp.Yes
	// Every `setopt` name is an invocation word here too, and reaching it
	// costs nothing beyond saying so: the resolver, the folding and the
	// refusal are this dialect's option table, which `-o NAME` already goes
	// through. Measured 2026-09-16 against zsh 5.9.2 — `--xtrace` traces,
	// `--noxtrace` does not, `--NO_GLOB` and `--no-glob` and `--no_glob` are
	// all `noglob`, `--shwordsplit` really splits, and `--zzznosuch` is
	// `no such option: zzznosuch` at 1 with the word as it was written.
	//
	// ksh93's `no` fallback cannot reach this column and that is what makes
	// one axis enough for both: Runner.hasSetOptionName takes a dialect with
	// an option table of its own at its word, so the substrate never strips a
	// second `no` off a name this table has already folded. It is the
	// difference between `--nonomatch`, which zsh takes (`nomatch` is a
	// canonical name), and `--nonoglob` and `--no_no_glob`, which it refuses
	// — the canonical name is `glob`, so `noglob` is already the negation and
	// there is no second one. All three answer the same here and there.
	s.LongOptionNamesASetOption = interp.Yes
	// The `set` builtin is a different question in this column, which is why
	// it is a different axis. Measured the same day: `set --zzz q` is status
	// 0 with an empty standard error and `q` as `$1`, and `set --xtrace q`
	// traces nothing — the word is swallowed whole rather than applied,
	// refused, or left to be a positional parameter. #3129.
	s.SetLongOptionWord = interp.LongOptionWordIsDiscarded
	// And the `--name` spelling takes hyphens out of the name, which is that
	// spelling's rule here and not this shell's namespace: `--no-glob` is
	// `noglob` and `-o no-glob`, `setopt no-glob` and `set -o no-glob` are
	// all `no such option: no-glob`, in one shell in one run.
	s.LongOptionNameIgnoresHyphens = interp.Yes
	// unanswered OptionNamespaceIgnoresSeparators, OptionNamespaceTakesANoPrefix:
	// this shell installs an option namespace of its own (Runner.SetOptionTable),
	// which answers every spelling before the substrate's roster is reached —
	// including both of these, since `setopt no_glob` and `setopt noglob` are
	// one name there. So the two axes govern a table this column never asks,
	// and there is no site here to put the question to (#3155, #3254).
	// unanswered LongOptionValueIsANumber: measured 2026-09-16, `--xtrace=1`
	// here is `no such option: xtrace=1` at 1 — the whole word including the
	// `=1` is looked up and refused, so this column reads no value at all and
	// the axis has nothing to answer. ksh93's reading is the only one.
	// Where the `-o` does stand alone it takes the next word regardless of
	// how it is spelled, which is the half of the panel ksh93 leaves it on
	// here: `set -o -e` is `no such option: -e` at 1.
	s.SetODeclinesADashWord = interp.No
	s.SetListsOptionsOnceAtTheEnd = interp.No
	// And it applies as it goes, asked with an action rather than a state
	// because its refusal ends a `-c` script whatever stands around it:
	// `set -e -Z -o` writes the whole option table with `errexit on` in it,
	// and `set -Z -e -o` does too — this column does not even stop at the
	// bad word.
	s.SetValidatesOptionLettersFirst = interp.No
	s.BadSetOptionNameAtInvocationExitsZero = interp.No
	s.UnknownConditionOptionIsAStatus = interp.Yes
	// And a single character given to that operator is the option *letter*
	// it abbreviates: `set -a; [[ -o a ]]` is 0 here and 1 in the three
	// other columns with the operator, which is the only shape that can show
	// it — with the option off both readings answer 1 (#4436).
	s.ConditionOptionTakesAnOptionLetter = interp.Yes
	s.ReturnOutsideAFunctionIsRefused = interp.No
	// And typed at a prompt it costs the line and not the session: measured
	// 2026-10-16 through a pseudo-terminal, `return 4; echo n2` writes
	// nothing, and the next line reads `$?` as 4 and runs (#6717).
	s.ReturnAtThePromptEndsTheSession = interp.No
	// And `break` with no loop around it stops the script here, which is the
	// opposite way round from the line above: measured, `echo t; break` on
	// one line and on two both end at status 1 with nothing after the
	// `break` running at all.
	s.LoopControlOutsideALoopIsFatal = interp.Yes
	// The count is read first: `break abc` outside a loop is `argument is
	// not positive: 0` and not the `not in while, until, select, or repeat
	// loop` this shell writes for a bare `break` in the same place.
	s.LoopControlPlaceIsJudgedBeforeTheCount = interp.No
	// Neither is a boundary: a `break` in a function body ends the caller's
	// loop, and one inside `( )` ends the subshell's copy of it. The shell
	// that stops a script over a `break` with no loop at all is the one that
	// finds a loop in both of these.
	s.FunctionCallIsALoopControlBoundary = interp.No
	s.SubshellIsALoopControlBoundary = interp.No
	// A startup file is a sourced script, so a `return` in one is accepted
	// everywhere — the split is only over what its argument does, and this
	// shell keeps it: measured through a pty, an rc of `return 3` and one
	// of `false; return 3` both leave `$?` as 3 at the first prompt, where
	// bash leaves 0 and 1. A `return` with no argument means the last
	// command's status here as everywhere.
	s.StartupFileReturnCarriesItsArgument = interp.Yes
	// A startup file that runs no command clears the status before it: measured,
	// a `.zshrc` or `.zlogin` with nothing in it after a `.zshenv` of `false`
	// leaves 0, though `$?` read inside such a file is still 1 (#5883).
	s.StartupFileThatRunsNothingLeavesZero = interp.Yes
	s.LoneDashIsAnOption = interp.Yes
	s.UnsetFunctionChecksTheName = interp.No
	s.UnsetFunctionReportsMissing = interp.Yes
	// `unset -f` is the only spelling that reaches a function here.
	s.UnsetReachesTheFunctionTable = interp.No

	// Whether a redirection target is expanded as an ordinary word.
	s.RedirectTargetIsAnOrdinaryWord = interp.No
	// And it matches a pattern the script *wrote* into the target, which is
	// the half POSIX forbids and this column keeps: `cat < only-*.txt` reads
	// the one file that matches and `printf X > only-*.txt` truncates it.
	// Measured 2026-09-16 (#3207).
	s.RedirectTargetTakesPathnameExpansion = interp.Yes

	// Whether `type --` ends the options.
	// zsh 5.9.2 has one wording for every builtin it has: `whence -v .` is
	// `. is a shell builtin`. Its `export` is a third answer — a reserved
	// word — and belongs to the grammar rather than to this axis.
	// zsh 5.9.2 ends the script from inside a subshell too, at its own
	// syntax status of 1 (#3274).
	s.SubstitutionParseErrorEscapesASubshell = interp.Yes
	// And from a here-document body as well: measured 2026-09-17, `cat <<END`
	// over a body holding `$(echo hi; for)` prints `start` and exits 1 here,
	// where bash 5.3.20 and ksh93u+ carry the script on (#3318).
	s.SubstitutionParseFailureInAHeredocBodyEndsTheShell = interp.Yes
	// Never reached: this column catches the failure at a `.` and at an
	// `eval` — Semantics.FatalErrorEndsBorrowedTextOnly — and it has no
	// here-document row to carry on from. Answered anyway, because an
	// unanswered axis is a refusal in front of a reader (#3319).
	s.SubstitutionParseFailureCarriesTheFatalStatus = interp.No
	s.TypeDistinguishesSpecialBuiltins = interp.No
	s.TypePrintsFunctionBody = interp.No
	s.TypeEndsOptionsWithDashDash = interp.Yes
	s.TypeLeadingDashWordAsksForTheBareAnswer = interp.No
	// whence -v under another name, so the letters answer in sentences:
	// -p searches PATH past the shell's own answer and words the hit the
	// way plain type does, and -f *prints* a function rather than skipping
	// it. The completion-system letters ride as unimplemented.
	// `w` names the kind — `ls: command` — which is how a syntax
	// highlighter classifies every word on the line. See interp's typeKind,
	// where its six words are measured against the other dialect's `-t`
	// (#2512). `-t` itself is a bad option here and is absent from the
	// letters, which is the whole of how that is said since #2180.
	//
	// `s` and `S` name what the path a command resolved to itself resolves
	// to — the last link and the whole chain — written as an arrow after
	// the sentence. The same answer this shell's own `whence -sv` and
	// `whence -Sv` give, measured 2026-09-26 on zsh 5.9.2 (`-f`) with
	// `c -> b -> sub/a -> /bin/ls` on PATH, byte for byte. They are here
	// rather than in `whence`'s own letters because `type` is the *shared*
	// builtin: the walk and the arrow are interp's (Runner.SymlinkArrow)
	// and only which letters exist is the dialect's.
	s.TypeOptions = "afpsSw"
	s.TypePSearchesPathPastTheShell = interp.Yes
	s.TypePathAnswerIsASentence = interp.Yes
	s.TypeFSaysTheFunctionBack = interp.Yes

	// The letters `typeset` and `local` read. `-f` prints function bodies;
	// `-F` is a *float's* precision here rather than bash's function-name
	// listing, so it rides in Diagnostics.UnimplementedOptionLetters with
	// the rest of what this shell has and this engine does not.
	//
	// `-H` hides a name's *value* from every listing and changes nothing
	// else — measured: `typeset -H h=v; echo $h` still says `v`, while
	// `typeset -p h` says `typeset h`, `set` says `h`, and `typeset +H h`
	// puts the value back. It is this shell's letter alone with that
	// meaning: bash refuses `-H` outright under both `declare` and
	// `typeset`, and ksh93's `-H` is a wholly different attribute — file
	// name mapping — that does not hide, listing back as `typeset -H h=v`.
	// So there is no axis here, only a letter one dialect has.
	//
	// `-U` keeps only the first occurrence of each element of an array, on
	// the declaration itself and on every later write — see
	// Runner.uniqueArrays. It is this shell's letter alone: bash refuses
	// `-U` under both spellings, and ksh93 does not have the letter at all,
	// answering `typeset: -U: unknown option`. Its own `-u` — uppercase —
	// is a different letter and stays what it was.
	//
	// `-h` hides a name *in scope*: a local declaration of a parameter
	// carrying it is an ordinary parameter rather than the special one it is
	// spelled like, and `+h` puts the specialness back. It is what
	// `compaudit` opens with — `local -a -U +h fpath` — and refusing the
	// letter left that line unrun, so the security check walked the caller's
	// `fpath` instead of a copy of it (#1621). This shell's letter alone with
	// that meaning: bash refuses `-h` and `+h` outright under all three
	// spellings, and ksh93's `-h` is a help string on a type definition,
	// which detaches nothing. See interp/hideinscope.go for the six rows it
	// was measured from.
	//
	// `-T SCALAR array [sep]` ties a scalar to an array, each reflecting the
	// other — which is how this shell's own `PATH` and `path` are the same
	// value. This shell's letter alone with that meaning: bash refuses `-T`
	// outright, and ksh93's `-T` declares a *type*, which is a different
	// builtin's worth of thing and stays refused by name there.
	//
	// `-m` takes the operands as *patterns* and lets every other letter on
	// the line decide what matching means — a listing under either sign on
	// its own, a declaration over the matches when a letter is written with
	// a minus, and a filtered listing when every letter is written with a
	// plus. See interp/declarematching.go for the four readings and the
	// measurements behind them. It is `typeset` and `declare` alone: this
	// shell's `local`, `export`, `readonly`, `integer` and `float` each say
	// `bad option: -m` back, which is why the letter is here and not in
	// LocalOptions. `compdump` is what needed it — `typeset +fm '_*'` is how
	// the completion system collects the function names it writes out, and
	// refusing the letter left every interactive startup with no dump
	// (#1674). ksh93's `-m` is a *rename* and bash has no such letter at
	// all, so there is no axis here, only a letter one dialect has.
	//
	// `z` is the newest of them and the first letter this engine takes in
	// silence rather than acting on — see DeclareOptionsWithoutEffect just
	// below, and note that it is `typeset` and `declare`'s alone: measured
	// 2026-09-10, `local -z`, `integer -z`, `float -z`, `export -z` and
	// `readonly -z` are each `bad option: -z` in zsh 5.9.2, so the letter
	// belongs to this table and to none of the other four (#1576).
	s.DeclareOptions = "aAEfFgHhiLlmpRrtuUTxZz"
	// And what the `m` of that set *means*: the operands are patterns and
	// every other letter on the line decides what a match is then used
	// for. ksh93 spells the same letter and moves a parameter with it, so
	// this is a disagreement about identical syntax rather than a letter
	// one dialect has — see interp.DeclareMatchingLetterPolicy.
	s.DeclareMatchingLetter = interp.DeclareMatchingLetterSelects
	// And the `M` of `functions`: it registers a shell function as a math
	// function here, where ksh93's `M` names a character mapping. The same
	// shape as the `m` letter above and for the same reason.
	s.DeclareMappingLetter = interp.DeclareMappingLetterRegistersAMathFunction
	// And the `T` of `typeset`: it ties a scalar to an array here — see
	// interp/tiedscalar.go — where ksh93's names a type. The third letter of
	// that shape and the third for the same reason.
	s.DeclareTypeLetter = interp.DeclareTypeLetterTiesAScalarAndAnArray
	// And the `H`, which is the fourth: it withholds a name's *value* from a
	// listing here — `typeset -H h=hid` lists back as `typeset h` — where
	// ksh93 records an inert attribute and writes the letter and the value
	// alike. Measured 2026-09-13; interp/declarehide.go holds it.
	s.DeclareHideValueLetter = interp.DeclareHideValueLetterHidesTheValue
	// And the lower-case letter hides a special name's *specialness* from a
	// local declaration, taking no argument — where ksh93 reads a string
	// after it. See interp/hideinscope.go.
	s.DeclareHideInScopeLetter = interp.DeclareHideInScopeLetterHidesInScope
	// `export` is this word's declaration under another name, and it takes
	// the same letters bar six. Measured 2026-09-12, a letter at a time
	// against `export -X q=4`: `-A`, `-g`, `-m`, `-x` and `-z` are
	// `bad option` and `-f` is `invalid option(s)`, and every other letter
	// above is read. See Semantics.ExportOptions for why the refused six are
	// the six they are.
	s.ExportOptions = "aFHhiLlpRrtuUTZ"
	// `-z` and `+z` are taken and do nothing here, which is measured on both
	// signs and on both sides of the builtin: `typeset -z p q` leaves `p`
	// alone and declares an empty `q` exactly as a bare `typeset q` would,
	// `typeset +z p` is 0 with `p` unchanged, and `typeset -fz f` over a
	// defined function is 0 with the function still defined and still
	// listing as itself.
	//
	// The one spelling where the letter is not inert is `typeset -fu`, which
	// is that shell's second name for `autoload`: there `-z` picks the
	// autoloaded file's syntax and is recorded on the stub, `typeset -fuz n`
	// listing as `builtin autoload -Xz` exactly as `autoload -z n` does.
	// That spelling is built now (#1753), and the letter reaches the stub
	// through declareFlags.letters rather than through this table — being
	// listed here means "no attribute", not "unread", which is why the two
	// can both be true of one letter. `autoload -z` itself has always been
	// unaffected: that word has its own table and keeps the letter.
	//
	// Refusing it was worth 68 diagnostics in one shell snapshot — a
	// completion loader's functions are named `+zi-log` and the like, so a
	// word beginning `+z` reaches `typeset` as an option bundle whenever
	// something feeds it a line it did not mean to (#1576).
	s.DeclareOptionsWithoutEffect = "z"
	// A lone `-` or `+` is an option word here rather than a name: `typeset
	// +` is the whole table's attribute words and names, and `typeset -` is
	// the bare listing. bash reads the same word as an identifier and
	// refuses it, which is why this is a field — see
	// Semantics.SignAloneIsAnOptionWord.
	s.SignAloneIsAnOptionWord = interp.Yes
	// And `export` and `readonly` read it too, which is the half ksh93 does
	// not have — see Semantics.SignAloneIsAnOptionWordToExport. Measured
	// 2026-09-12: `export +` writes the exported names and `readonly +` the
	// frozen ones, both bare and both at 0, where this engine answered
	// `not valid in this context: +` at 1 (#1756).
	s.SignAloneIsAnOptionWordToExport = interp.Yes
	// `typeset +f` is the function *names*, one bare name a line and no
	// quoting — the shape a shell snapshot reads to find what to capture.
	// See Semantics.FunctionNamesUnderPlus for the two other answers.
	s.FunctionNamesUnderPlus = interp.Yes
	// `typeset -f` written with `-u` or `-U` and names is not a listing: it
	// is `autoload` under a second word. `z` rides along and is recorded on
	// the stub without being one of these — `typeset -fz nm` marks nobody.
	// See dialect/zsh's autoloadFromDeclaration for the measurements (#1753).
	s.FunctionLettersThatMarkUndefined = "uU"
	// `-F` is a float's precision here rather than bash's function listing,
	// and the number behind it is the letter's argument and not a name:
	// `typeset -F 3 x=1.5` declares one name at three places and reads back
	// `1.500`. Reading the `3` as a second name made a plugin loader's
	// `typeset -F 3 SECONDS=0` complain once per plugin (#1453).
	//
	// It was taken in *silence* before there was a float attribute to record
	// the precision in (#1037) — right about the letter's status and stream,
	// wrong about the value, since `typeset -F v=1.5` left `1.5` where this
	// shell writes `1.5000000000`. Both halves are now modeled, so the
	// letter is an attribute here rather than an entry in
	// DeclareOptionsWithoutEffect.
	//
	// Nothing about the shape #1037 measured changes: a bare `declare -F` is
	// a filtered listing of the *float* names in this shell, not a function
	// listing, and it answers 0 with not one byte from a shell that has no
	// floats — as does `declare -F f` over a function, which declares a
	// float called `f` here and says nothing.
	// And the three width letters take one the same way: `-L`, `-R` and `-Z`
	// each name how many characters the value is presented in, under the
	// detached spelling and the attached one alike — `typeset -L 5 a=ab` and
	// `typeset -L5 a=ab` are both `[ab   ]`. See interp/fieldwidth.go for
	// the rule and for why ksh93 does not have them here (#1461).
	// `-E` joins the four here rather than standing apart: it takes a number
	// in exactly the shape `-F` does, attached or detached, and what it does
	// with the number is FloatFormatLetterE's question and not this one.
	s.DeclareOptionsTakingANumber = "EFLRZp"
	// `-p` takes one too, which is the listing's layout and not an
	// attribute: `-p1` writes an array or a table one element to a line,
	// `-p0` is the plain `-p`, and anything else is refused. See
	// Runner.readOptionNumber for the measurements.
	//
	// And the letter is a **format**: `%.*e` with n−1 places, so `typeset -E
	// 3 a=3.14159` is `3.14e+00` where ksh93's same line is `3.14`. See
	// interp/floatformat.go for the measurements (#2559).
	s.FloatFormatLetterE = interp.FloatFormatExponentWithPlaces
	// The two numeric type letters may stand together here and the **first
	// written** wins, which the parse settles: measured 2026-09-15,
	// `typeset -iE 3 a=1.5` lists as `typeset -i3 a=1` and `typeset -Ei 3
	// a=1.5` as `typeset -E a=1.50e+00`. ksh93 refuses the pair outright.
	s.NumericTypeLettersAreExclusive = interp.No
	// And the letter read *first* wins, which is the order the parse falls
	// into on its own: `typeset -iF 3 a=1.5` is `typeset -i3 a=1` and
	// `-Fi 3` is `typeset -F a=1.500`.
	s.NumericTypeLetterPrecedence = interp.NumericLetterFirstWrittenWins
	// `Z` is exclusive with `R` here and a **combination** with `L`: the two
	// stand together, the listing writes both and `${(t)}` names both, where
	// ksh93 reads the letter as a fill riding on whichever justification it
	// has. Measured 2026-09-27 on zsh 5.9.2 (aarch64-apple-darwin25.4.0),
	// `-f` from a script file under `env -i PATH=/usr/bin:/bin`:
	//
	//	typeset -L5 -Z5 v=7      typeset -L5 -Z5  scalar-left-right_zeros
	//	typeset -Z5 -L5 v=7      typeset -L5 -Z5  scalar-left-right_zeros
	//	typeset -L5 -Z5 v=00700  the value is `700  ` — the zeros come off
	//	typeset -Z5 v=7          typeset -Z5      scalar-right_zeros
	//
	// This used to read `a justification of its own`, and every row it was
	// measured on is a one-word spelling with a **detached** number — the
	// second letter is never read there at all, so those rows are produced
	// identically by both readings and were never evidence about `Z`. They
	// still agree and are the controls. See interp/fieldwidth.go, where the
	// rest of the table is (#2859, #4798).
	s.DeclareZeroFillLetter = interp.DeclareZeroFillLetterCombinesWithTheLeftJustification
	// And the fill goes in after a value's leading blanks; see the axis.
	s.ZeroFillKeepsLeadingBlanks = interp.Yes
	s.ZeroFillGoesAfterAnIntegersSign = interp.Yes
	// And where a declaration writes two of them, the name ends up with
	// **neither** — silently, at 0, with the value unpadded. Measured
	// 2026-09-27: `typeset -L5 -R5 v=7` and `typeset -Z5 -R5 v=7` both list
	// as `typeset v=7` and read `7`, where ksh93 takes the last letter.
	//
	// This used to read `the first written wins`, and the five rows it was
	// measured on are all one-word spellings with a **detached** number —
	// `typeset -LR 5 t=7`, `-RL 5`, `-ZR 5`, `-ZL 5`, `-LRZ 5`. Those still
	// answer exactly as recorded, because the second letter is never read
	// there: DeclareNumberDetachedOnlyAtTheWordEnd discards the rest of the
	// word. So every row agreed and the axis was keyed on the wrong thing,
	// which the separate-word spelling is what shows (#4766).
	s.WidthJustificationPrecedence = interp.WidthJustificationConflictLeavesNoWidth
	// The **number** is the last written, which is the reading the store
	// falls into on its own. Measured 2026-09-27 under `env -i
	// PATH=/usr/bin:/bin LC_ALL=C`: `typeset -L5 -L3 v=7` is `typeset -L3`,
	// `typeset -R5 -R3` is `-R3` and `typeset -Z5 -Z3` is `-Z3`, where ksh93
	// keeps the first number written (#4827). Which number the `L`/`Z` pair
	// takes is a rule on top of this one — see interp.declaredWidthNumber.
	s.WidthNumberPrecedence = interp.WidthNumberLastWrittenWins
	// And a width letter may stand beside the integer one, the first written
	// winning there too: measured 2026-09-18, `typeset -iL 5 a=7` lists as
	// `typeset -i5 a=7` — the `5` a base — and `typeset -Li 5 a=7` as
	// `typeset -L5 a=7`. ksh93 answers both with typeset's usage block and
	// ends the script.
	s.WidthLettersExcludeTheIntegerLetter = interp.No
	// A bare `-F` or `-E` over a name that already has a precision keeps it
	// here — measured, `typeset -F 3 x=1.5; typeset -F x` still reads
	// `1.500` — where ksh93 resets to the letter's default.
	s.BareFloatLetterResetsThePrecision = interp.No
	// And a detached number reaches its letter wherever the letter stands:
	// `typeset -El 3 a=1.5` is `1.50e+00` here and `3: invalid variable
	// name` in ksh93, which is where the two part.
	s.DeclareNumberDetachedOnlyAtTheWordEnd = interp.No
	// And an attached number takes the rest of its word: see
	// interp.Semantics.AttachedNumberEndsAtTheFirstNonDigit (#5685).
	s.AttachedNumberEndsAtTheFirstNonDigit = interp.No
	// `functions` takes none of the letters this engine acts on. Its own
	// set — -c -k -m -s -t -u -x -z -M -T -U -W, measured 2026-09-08 by
	// sweeping the alphabet in both cases — is autoloading, tracing, the
	// math-function facility and the `zsh/parameter` spellings, none of
	// which this engine has; they ride in UnimplementedOptionLetters so a
	// script is told they are missing rather than unknown. Two are the
	// exception and are implemented: `-m` is a listing narrowed by pattern,
	// which is the same walk the bare listing already does, and `-M` is the
	// math-function facility — the one letter here that is not a listing at
	// all. Its presence in this set is what tells interp the dialect has the
	// facility; the capability is interp/mathfunc.go and the name is here
	// (#1493).
	// `-u` and `-U` are the marking letters, and under this word they are
	// what they are under `typeset -f`: with operands a declaration, with
	// none the listing a bare `autoload` writes. Measured 2026-09-12 on zsh
	// 5.9.2 with `f1` autoloaded plainly and `f2` with `-U`, `functions -u`
	// is byte for byte what `autoload` with no operands writes, and
	// `functions -U` is that listing narrowed to `f2` (#1996). See
	// Semantics.FunctionLettersThatMarkUndefined, which already named them
	// for the declaration word, and Diagnostics.MarkingLettersUnderPlus for
	// the half of `-u` this shell refuses.
	s.FunctionsOptions = "mMuUxstTW"
	// `t` and `T` mark a function for tracing — see
	// Semantics.FunctionTraceLetters. They are in FunctionsOptions above
	// because this word spells them and in the declaration's own set
	// already, `typeset -ft f` and `declare -ft f` being the same line under
	// another name; `local -ft f` is `bad option: -f` in this shell, so
	// LocalOptions needs nothing.
	s.FunctionTraceLetters = "tT"
	// `-T` is the bounded one: measured 2026-09-28 on zsh 5.9.2, `functions
	// -T f` traces the line in `f` that calls `g` and leaves g's body
	// silent, where `functions -t f` traces g's body too.
	s.FunctionTraceLettersBoundToTheBody = "T"
	// And the rule the mark leans on, which is this column's alone: the
	// trace option is saved across a call and put back at the return.
	// Measured 2026-09-28 from script files under `env -i`, `f() { set -x };
	// f; print after` leaves `after` untraced here and traced in bash 5.3,
	// dash and /bin/ksh — and `f() { setopt extendedglob }; f` leaves *that*
	// option on afterwards here, so it is the trace alone and not
	// `localoptions`.
	s.FunctionCallRestoresTheTrace = interp.Yes
	// `unfunction`'s whole set, and it really is one letter: every other
	// letter of the alphabet is a bad option there in both cases, the `-f`
	// this name stands for included.
	s.UnfunctionOptions = "m"
	// `local`'s own set, measured a letter at a time against all fifty-two on
	// zsh 5.9.2, 2026-09-09: it takes `aAEFhHilLprRtTuUx` and calls every
	// other letter a bad option. Two things follow, and they pull opposite
	// ways.
	//
	// It is *narrower* than DeclareOptions where the letter names something a
	// local cannot be — `-f` is a function and `-g` is a global, and a local
	// that is either is a contradiction, so zsh refuses both here and this
	// set leaves them out.
	//
	// And it is *wider* than this set used to be: `-F` is a float, which is
	// exactly as declarable in a function as at the top level, and it was
	// missing for no reason but that the two sets were written apart. The
	// attribute itself was already built — `typeset -F x=2` was
	// `2.0000000000` and `typeset -F 3 x=1.5` was `1.500`, both matching zsh
	// — so `local -F` refused a letter this shell could already answer.
	// `_zsh_highlight_bind_widgets` opens with one, which is where a real
	// startup lost syntax highlighting (#1594).
	//
	// `E` came in once `typeset -E` was built: measured 2026-10-16, `f(){
	// local -E x=1.5; typeset -p x; }; f` is `typeset -E x=1.500000000e+00`
	// on zsh 5.9.2, where this refused the letter as a bad option long after
	// `typeset` took it.
	s.LocalOptions = "aAEFHhiLlPpRrtuUTxZ"
	// `private`'s set is neither that one nor DeclareOptions, which is why it
	// is a field of its own. Measured a letter at a time against `private -X
	// v` inside a function on zsh 5.9.2, 2026-09-27: it takes `aAEFHhiLlmpRr
	// tuUxZ` and refuses `f`, `g`, `M`, `n`, `T` and `z`. So against `local`
	// it gains `E` and `m` and loses `T`.
	//
	// `-g` is the refusal worth reading twice: that letter means *do not
	// take a scope*, and a declaration with no scope cannot be private to a
	// call, so the word and the letter are a contradiction rather than a
	// combination. `-f` is refused for the same shape of reason `local`
	// refuses it.
	//
	// `E` and `m` are left out here and **not** because the shell refuses
	// them: it takes both. They are letters this engine has not built under
	// a scoped declaration — `-E` is the float-exponent format, which
	// LocalOptions leaves out for the same reason, and `-m` reads the
	// operands as patterns, which `local` has no route for either. So they
	// are named in UnimplementedOptionLetters below instead, which is the
	// difference between telling a script the letter is on its way and
	// telling it zsh never had one. The two tables move together and
	// TestNoLetterIsBothAcceptedAndCalledMissing is the invariant.
	//
	// `T` is left out of both, because zsh's own `private` refuses it.
	s.PrivateOptions = "aAFHhiLlPpRrtuUxZ"
	// A bad `typeset` option is reported and the script goes on.
	s.TypesetBadOptionFatal = interp.No
	// A local's value refused over a produced frozen parameter ends the
	// shell at 0: measured 2026-10-03, `f() { local PPID=5; }; f; echo
	// after` exits 0 with nothing after the refusal. See
	// Semantics.ProducedFreezeUnderALocalEndsAtZero.
	s.ProducedFreezeUnderALocalEndsAtZero = interp.Yes
	// And the refusal takes the whole line with it: measured 2026-10-03 on
	// zsh 5.9.2, `typeset -Q a=(1)` is `bad option: -Q` at 1 and `a` does
	// not exist afterwards. See
	// Semantics.RefusedDeclarationKeepsItsArrayLiterals.
	s.RefusedDeclarationKeepsItsArrayLiterals = interp.No
	// `integer` here is `typeset` with the letter prepended rather than a
	// declaration of its own, and its letter set is *narrower* than
	// typeset's: measured 2026-09-06, `integer -a`, `-A`, `-f`, `-F`, `-T`
	// and `-U` are bad options where this shell's typeset takes all six. So
	// the set is its own field and not DeclareOptions over again — a shell
	// that reused them would accept `integer -A m`, which is an associative
	// array in no shell that has the word.
	//
	// And the three width letters, which `integer` takes as `typeset` does:
	// `integer -Z 10 n=-42` is `-000000042` and `integer -L 6 n=-4` is
	// `-4····`, measured 2026-10-02 (#5142).
	s.IntegerOptions = "gHhiLlprRtuxZ"
	// And `float` is the same word one letter along, with the same set
	// narrowed the same way: measured 2026-09-16 under `-f`, a letter at a
	// time against `float -X zz=1.5`, this shell's `float` refuses `-a`,
	// `-A`, `-G`, `-i`, `-m`, `-T`, `-U` and `-z` and takes the rest. So it
	// is `integer`'s letters with the integer one traded for the two float
	// ones, which is what the two words are.
	s.FloatOptions = "EFgHhlprtux"
	// `-i16` and `-i 16` are an output base here — `integer -i 16 b=255` is
	// `16#FF` — and this engine has no base to keep, so it refuses by name.
	s.IntegerAttributeTakesABase = interp.Yes
	// The alphabet this shell counts an output base in, and its length is
	// the largest base it can spell — see Semantics.IntegerBaseDigits.
	s.IntegerBaseDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	s.IntegerBaseComesFromTheValueAssigned = interp.Yes
	s.IntegerBaseNegativeIsTwosComplement = interp.No
	// `16#FF` by default, and `0xFF` under `setopt C_BASES` — the one
	// option in the panel that moves this, so the entry in setopt.go writes
	// the axis rather than remembering the request (#4502). Off here
	// because `C_BASES` is off in a shell that has not set it.
	s.IntegerBaseMarkIsCSpelled = interp.No
	// Ten is a base like any other here: measured, `typeset -i10 d=255`
	// lists back as `typeset -i10 d=255`, and a bare `typeset -i a` over a
	// name declared `-i16` leaves it reading `16#FF`.
	s.IntegerBaseTenIsNoBase = interp.No
	// The letter is ordinary in this shell, so a plus word on `integer`
	// means what it means on `typeset`: `integer n=5; integer +i n; n=3+4`
	// is `3+4` here and `integer +x e` unexports, and `typeset -p n` says a
	// plain `typeset n=5`. ksh93, where the name carries the type, answers
	// every one of those the other way.
	s.IntegerPlusFormTakesAttributesOff = interp.Yes
	// `set -A name value …` assigns an array through a name a variable
	// holds, which is this shell's spelling and ksh93's alike.
	s.SetArrayLetter = interp.Yes
	// `shift -p` shifts off the end of the list rather than off its front.
	// The one letter `shift` takes in this shell, measured a letter at a
	// time against zsh 5.9.2 (#4600).
	s.ShiftFromTheEndLetter = interp.Yes
	// The name ends the options here: `set -A ff -x -y` stores the two dash
	// words as elements and `set -A dd -- 1 2` stores three, the `--` among
	// them. ksh93 keeps parsing and answers both the other way.
	s.SetArrayOptionsContinuePastTheName = interp.No
	// `set -A a` with no values leaves an array with no elements here —
	// `typeset -a a=(  )` — where ksh93 unsets the name outright. Both count
	// 0, so the difference shows only to a script that asks whether the name
	// is set at all.
	s.SetArrayWithNoValuesUnsetsTheName = interp.No
	// And the plus form over a name that is not an array, which is a store
	// this column makes and the other does not. Measured 2026-09-27 on zsh
	// 5.9.2 under `-f`: `s=v; set +A s` lists `typeset -a s=(  )`, and the
	// export attribute and the integer letter come off with the re-creation
	// (#4810).
	s.SetArrayEmptyPrependMakesANonArrayAnEmptyArray = interp.Yes
	// `typeset -g x=new` with a `local x` in front assigns the *local* —
	// the letter only widens where a new declaration would land, it does
	// not reach past what already stands. Measured: `in=new out=out`
	// against the other engine's `in=in out=new`.
	s.DeclareGlobalReachesPastALocal = interp.No
	// A bare `local` — and a bare `set` — list every parameter the shell
	// has, special parameters and tied arrays included: a fact about this
	// engine's parameter table, refused as unimplemented rather than
	// approximated.
	// `-` is a parameter here — the option letters in effect — so `local -`
	// is a declaration of it and not the save the other three read it as.
	// Measured 2026-09-15: the options a body sets are still set after it
	// returns.
	s.LocalDashSavesTheShellOptions = interp.No
	s.BareLocalListing = interp.BareLocalListsEveryParameter
	// And `local -p name` lists whatever the name reaches, marking a global
	// as one: measured 2026-09-21, `string=global; f() { local -p string; }`
	// writes `typeset -g string=global` at 0 here where bash refuses it.
	s.LocalListingIsTheRunningCallsOwn = interp.No
	// The same table for the other word, which is what zsh writes for it.
	s.BareTypesetListing = interp.BareLocalListsEveryParameter
	// The listing is `name=value` in the same shape dash and ksh93 write,
	// with the quoting bash uses — measured against all six panel members
	// from the same three values. Real zsh's listing also carries its special
	// parameters and its tied arrays, which this parameter table has not got;
	// that is a difference in what a shell *holds* rather than in what a
	// listing looks like, and modeling it as a form of its own only bought a
	// refusal where a script wanted a listing.
	s.SetListing = interp.SetListingAssignments
	// No array and no name: a coprocess here is reached by `print -p` and
	// `read -p`, measured — `${COPROC[0]}` is empty after `coproc cat`.
	s.CoprocEndsInAnArray = interp.No
	// The ends are numbered where any other allocation goes rather than at
	// the top of the table, which a shell publishing no array still shows in
	// what is left for it to hand out. Measured 2026-09-13: `exec
	// {a}>/dev/null {b}>/dev/null {c}>/dev/null` answers `11 12 13` with no
	// coprocess running and `12 13 15` with one (#2596).
	//
	// Four of them, the first and fourth kept, measured 2026-10-17 (#6661):
	// the `12 13 15` above is ends on 11 and 14. See
	// interp.CoprocEndsFirstAndFourthFromTheBase.
	s.CoprocessEndPlacement = interp.CoprocEndsFirstAndFourthFromTheBase
	// And a process substitution's far end goes there too, which is the
	// reading that makes 11 the base rather than a number of its own:
	// measured 2026-09-21, `echo <(true) <(true) <(true)` is `/dev/fd/11
	// /dev/fd/12 /dev/fd/13`, and an `exec {a}</dev/null` first moves the
	// substitution to 12.
	s.SubstitutionEndPlacement = interp.SubstitutionEndsWhereAnyDescriptorGoes
	// And what the shell holds for itself is on those numbers too: a saved
	// copy of each stream a redirection list replaces here, and the reading
	// end of each pipe. Measured 2026-10-15, `{ echo <(true) <(true); }
	// </dev/null` is `12 13` and `echo x | echo <(true)` is 12 (#6473).
	s.OwnDescriptors = interp.SavedStreamsAndPipeReadEndsTakeNumbers
	// And nothing is taken back when the coprocess ends: both ends stay open
	// and stay reachable by their letters. Measured 2026-09-12 — a `print -p`
	// after the coprocess has gone writes into a pipe with no reader and the
	// shell dies on SIGPIPE at 141, where the other two shells with a
	// coprocess refuse the write instead. The death itself is #770's, not
	// this axis's: a builtin's broken-pipe write is answered here with the
	// status rather than the signal, so this column reports 1 where zsh
	// reports 141 until that lands (#2411).
	s.ReapedCoprocessEnds = interp.CoprocEndsSurviveTheCoprocess
	// `p` after `>&` or `<&` is the coprocess here too, which is the same
	// facility the `-p` letters are: `coproc cat; exec 3>&p; print -u3 x`
	// reaches the coprocess through a 3. It **lends** the end rather than
	// handing it over — measured 2026-09-16, `print -p` after that `exec`
	// still writes to the coprocess, where ksh93's letter finds none. The
	// refusal with none running names the facility rather than the word,
	// `coprocess: bad file descriptor`, which is what the diagnostic below
	// carries (#2345).
	s.CoprocessNamedByARedirection = interp.CoprocessRedirectionDuplicatesTheEnd
	s.SetListingQuoting = interp.ListingQuoteWhenNeededRuns

	// A descriptor number the process cannot hold is not checked here: with
	// `ulimit -n 6`, `exec 8>f` reports success and prints nothing, where
	// bash and ksh93 hand the kernel's refusal back. Rarely reachable, since
	// a number this shell reads is one digit and the shell picks its own for
	// `{name}>f`.
	// The one column that says so when a close through a variable finds
	// nothing open — see interp.Semantics.FdVariableFailedCloseIsReported.
	s.FdVariableFailedCloseIsReported = interp.Yes
	// And a frozen name refuses the close outright, where bash closes it and
	// says nothing. The close reads the name rather than writing it — a
	// successful one leaves the number in place — so this is a rule about
	// the name's attribute and not a consequence of the assignment;
	// `A04redirect.ztst` stops on it under `Error closing file descriptor
	// using readonly variable`. See
	// interp.Semantics.ReadonlyFdVariableRefusesAClose.
	s.ReadonlyFdVariableRefusesAClose = interp.Yes
	// And a frozen name is refused before the target is opened: `readonly
	// v=1; true {v}>/nodir/f` is `can't allocate file descriptor to readonly
	// parameter v` and not the open's failure, measured 2026-10-15 (#6571).
	// See interp.Semantics.FdVariableReadonlyRefusedFirst.
	s.FdVariableReadonlyRefusedFirst = interp.Yes
	// But a `{name}` store is made before the command's prefix, which the
	// command sees: `f() { echo "in[$v]"; }; v=pre f {v}</dev/null; echo
	// "[$v]"` is `in[0] [11]`, measured 2026-10-15 (#6569). See
	// interp.Semantics.FdVariableStoredAfterThePrefix.
	s.FdVariableStoredAfterThePrefix = interp.No
	// unanswered FdVariableMakesABareListASubshell: a `{name}` in front of
	// the command word opens a brace group here, so `{v}>/dev/null` runs a
	// command named `v` and a command of redirections alone carries no
	// `{name}`, measured 2026-10-15 on zsh 5.9.2 (#6629).
	// And `set -C` protects a name that already holds an open descriptor, so
	// `exec {m}>f1; exec {m}>f2` is refused rather than reallocating. The
	// name is what is protected and not the file — the same file twice is
	// refused too — and `>|` does not override it. `A04redirect.ztst` stops
	// on this under `NO_CLOBBER prevents overwriting parameter with allocated
	// fd`. See interp.Semantics.NoclobberProtectsAnFdVariable.
	s.NoclobberProtectsAnFdVariable = interp.Yes
	// And a `{name}` store makes a name that is not holding a scalar an
	// integer: `true {v}</dev/null; typeset -p v` is `typeset -i v=11`,
	// measured 2026-10-15 (#6550). See
	// interp.Semantics.FdVariableStoreTyping.
	s.FdVariableStoreTyping = interp.FdVariableStoreMakesANewNameAnInteger
	// And a program is handed the environment from after its own `{name}`
	// store: `set -a; /usr/bin/env {v}</dev/null` shows it `v=11`, measured
	// 2026-10-15 (#6521). See
	// interp.Semantics.FdVariableReachesTheProgramsEnvironment.
	s.FdVariableReachesTheProgramsEnvironment = interp.Yes
	// And a digit-led word that is not all digits is refused at the store,
	// fatally: `echo {1a}</dev/null; echo st=$?` writes `not an identifier:
	// 1a` and ends the shell at 1, measured 2026-10-15 (#6522). See
	// interp.Semantics.FdVariableBadName.
	s.FdVariableBadName = interp.FdVariableBadNameIsAFatalStoreError
	// unanswered FdVariableOnASubshellIsTheSubshells: a `{name}` behind
	// parentheses never parses here — `( : ) {v}</dev/null` is ``parse error
	// near `{'`` in zsh 5.9.2 under every emulation, measured 2026-10-15
	// (#6507, syntax.Dialect.FdVariableRefusedAfterACompound) — so there is no
	// allocation for the question to be about.
	// A command word written with a leading `%` is a job specification here,
	// read as the script wrote it: `"%prep"`, `\%prep` and a parameter
	// holding the text are all ordinary command names, where bash takes all
	// three. `A04redirect.ztst` opens with the ztst driver's own `%prep` and
	// `%test` markers, which is how the file's first two differing lines come
	// to be about job control.
	s.JobSpecCommandWord = interp.JobSpecCommandWordAsWritten
	s.FdNumberBoundedByOpenFileLimit = interp.No
	// No descriptor-number ceiling of this shell's own (#3210), and `read
	// -t` takes no argument here at all, so the word after it is an operand
	// and never an expression (#3209). The `-v` echo of a file's last line
	// writes no newline the input did not have (#3130) — measured on a file
	// with no final newline; the `-c` route is a different mechanism there
	// and is named in the axis.
	s.DescriptorNumberCeiling = interp.NoDescriptorNumberCeiling
	s.VerboseEchoAddsAMissingNewline = interp.No
	s.ReadTimeoutOperandIsArithmetic = interp.No

	// `[[ -v ]]` reads more kinds of name here than either of the other two
	// shells that have the operator: a positional, as bash does, and every
	// parameter spelled as one punctuation character — `?`, `#`, `$`, `!`,
	// `*` and `-` — which is this shell's alone. `@` is unset here as it is
	// everywhere, so it is excluded outright rather than by an axis.
	// Measured on zsh 5.9.2.
	s.ParameterIsSetSeesPositionals = true
	s.ParameterIsSetSeesSpecials = true

	// A subscript the condition receives as *text* is expanded again before
	// it is looked up, and this shell writes no bracket of its own into that
	// text: it has no written-subscript reading, so `k='x y'; kk='$k'` makes
	// `[[ -v m[$kk] ]]` set here where bash stops at the key `$k`. Measured
	// 2026-09-19 on zsh 5.9.2, and the same answer for the quoted operand
	// and for one that arrives out of a value. See
	// interp.Semantics.ConditionIsSetExpandsAFlatSubscript (#3298).
	s.ConditionIsSetExpandsAFlatSubscript = interp.Yes

	// And a declaration's operand expands its subscript: `typeset 'd[$k]'=Q`
	// writes the key `x y`, and `typeset 'arr[$i]'=Q` the element `$i`
	// counts to. See
	// interp.Semantics.DeclarationOperandExpandsItsSubscript (#3298).
	s.DeclarationOperandExpandsItsSubscript = interp.Yes

	// The store through a builtin's name operand rounds too, and `test -v`
	// with it. `unset` is where this shell parts from the one that has a
	// name for the whole group, and it is the only surface in it where the
	// two disagree — which is why the three are three fields rather than
	// one. Measured 2026-09-19 on zsh 5.9.2, from a script file with
	// standard input on /dev/null, over `k='x y'` and a table holding one
	// element under the key `x y`:
	//
	//	unset -v 'a[$k]'      the element **survives**
	//	unset 'a[$k]'         the element **survives**
	//	printf -v 'c[$k]' P   the key `x y`
	//	read 'b[$k]' <<<Z     the key `x y`
	//	test -v 'g[$k]'       true
	//	[ -v 'g[$k]' ]        true
	//
	// The control for the first two rows is the same operand with nothing in
	// it to expand: `unset 'a[plain]'` takes the element away here, so the
	// survival above is the round not happening rather than the quoted
	// brackets not reaching `unset`. See
	// interp.Semantics.UnsetExpandsAFlatSubscript (#3298).
	s.UnsetExpandsAFlatSubscript = interp.No
	s.OutputOperandExpandsAFlatSubscript = interp.Yes
	s.TestIsSetExpandsAFlatSubscript = interp.Yes
	return s
}
