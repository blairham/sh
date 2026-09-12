// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package dash answers the substrate's questions the way dash does.
package dash

import (
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what dash parses.
func Dialect() syntax.Dialect {
	// dash is the POSIX shell language and nothing more.
	d := syntax.POSIX()
	// dash expands aliases in a script, with no option to turn on, and by
	// every route: `-c`, a file and standard input all expand.
	d.AliasesExpandUnlessTold = true
	d.ExpandAliasesInProgramText = syntax.RouteOnEveryRoute
	// And it splices the body's text, so a newline in one is a line of the
	// program: everything after an expansion shifts down by one per newline.
	// Set here rather than inherited because this dialect starts from the
	// bare POSIX vector rather than from the core.
	d.AliasBodyCountsLines = true
	// Not a construct it adds but how it reads one it already has: a name
	// followed by `(` is a function definition here, whether or not the `)`
	// comes next, which is what decides the token a malformed one is blamed
	// on.
	d.FuncDefAtParen = true
	// And having committed, it checks: a name with punctuation is refused
	// once the parens close — `Bad function name`.
	d.FunctionNamePunctuation = false
	// One operator may stand where a case pattern belongs, opening an arm
	// that matches nothing — measured by running it, not inferred from the
	// error it causes elsewhere.
	d.CasePatternAcceptsOperator = true
	// The loop-variable position is read as a name whatever stands there, so
	// `for` with nothing after it is `Bad for loop variable` here and an
	// unexpected token in the other three.
	d.ForNonWordIsANameError = true
	return d
}

// Semantics is what dash means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	// `export a+=2` is `a+: bad variable name` here, so the append operator
	// is not an operand this shell's declarations take.
	s.DeclarationTakesAnAppendOperand = interp.No
	// An unquoted list is its elements taken one at a time, never their
	// join — the reading POSIX describes, and the one an empty element
	// disappears under: `IFS=:; set -- x "" y` is two fields here and three
	// in bash.
	s.UnquotedListJoinsOnIFS = interp.No
	// `${1:=abc}` is `1: bad variable name` and ends the script at 2 — a
	// positional is no more assignable through an expansion here than `@`
	// is (#1541).
	s.AssignThroughExpansionMayNameAPositional = interp.No
	// The one shell in the panel that expands a here-document body in the
	// shell rather than in the process the redirection is for, so what the
	// body writes is still there afterwards: `unset u; cat <<END` with a
	// body of `${u:=zz}` leaves `u=zz` here and leaves it unset in bash,
	// ksh93 and zsh. Its own failure half still costs only the command —
	// `set -u` on an unset name in such a body reports 2 and the script
	// carries on — which is unanimous and so is not asked.
	s.HeredocExpandsInTheCommandsProcess = interp.No
	// And the same for a redirection's *target*, where this shell is alone
	// again and where the split costs more: `cat /dev/null > "${u:=made}"`
	// leaves `u` set here and unset in the other three, and `> "$NOPE"`
	// under `set -u` ends the script here where the other three lose only
	// the command. A second axis rather than the one above, because the two
	// positions do not share an answer — a body's failed expansion costs
	// this shell the command and not the script (#1228).
	s.RedirectTargetExpandsInTheCommandsProcess = interp.No
	// POSIX makes an unquoted `$@` behave as `$*` where nothing is split,
	// and this shell complies: `IFS=-; set -- x y z; v=${@}` is `x-y-z`
	// here and in zsh, against `x y z` in bash and ksh93. It has no arrays,
	// so the positional spelling is the whole of the question here.
	s.UnsplitAtListJoinsOnIFS = interp.Yes
	s.CommandNotFoundStatusIsNotFound = interp.Yes
	s.SetFTurnsOffGlobbing = interp.Yes
	// Neither editing mode is ever selected on its own here — measured
	// 2026-09-11, `set -o` reports both off in a script and under `-i`.
	s.InteractiveSelectsEmacs = interp.No
	// This shell has no braces to expand and no `-B` either, so the letter
	// is refused as the invalid option it is rather than asked about.
	s.SetBTurnsOffBraceExpansion = interp.No
	// The panel's only shell with no multibyte decoder: `s=héllo; echo
	// ${#s}` is 6 here in every locale, where bash, ksh93 and zsh answer 6
	// under `LC_ALL=C` and 5 under a UTF-8 one. Measured 2026-09-05 across
	// LC_ALL, LC_CTYPE and LANG, and dash does not move for any of them.
	s.MultibyteEncodingIsHonored = interp.No
	// The one shell that refuses the -h letter POSIX names.
	s.SetHasTheHLetter = interp.No
	// Job control wants the tty: with none, `set -m` earns the remark
	// `can't access tty; job control turned off` — a remark, measured, not
	// a failure: the option stays off and `set` still reports 0.
	s.MonitorNeedsATerminal = interp.Yes
	// dash leaves it off with no terminal too, remarking `can't access tty;
	// job control turned off` — the same sentence its `set -m` refusal uses,
	// from the same shell, about two different questions.
	s.InteractiveMonitorNeedsATerminal = interp.Yes
	// The same, and measured the same way: dash forks for `( )`, so the
	// subshell outlives the signal its `kill` sent the shell.
	s.SubshellRunsOnAfterSignalingTheShell = interp.Yes
	// dash has somebody to tell on this route, and tells them exactly one
	// thing: measured on `-i script.sh` through a pseudo-terminal it writes
	// `[1] + Done sleep 0.3` and never the line that starts the job. The
	// start is AnnouncesBackgroundJob, which dash answers No, and the two
	// fields are separate because of exactly this shell.
	s.InteractiveScriptAnnouncesJobs = interp.Yes
	// And `$!` before any background command is unset here as it is in bash,
	// in its own words and with its own status: measured,
	// `set -u; echo "[$!]"` writes `!: parameter not set` — the name without
	// its `$` — and stops at 2.
	s.LastBackgroundPidIsUnsetBeforeAnyJob = interp.Yes
	// A job started with `&` reads an empty standard input, not the shell's:
	// measured 2026-09-07, `dash -c '/bin/cat & wait; echo ---; /bin/cat' < f`
	// writes `---` and then the file's line. POSIX XCU 2.9.3.
	s.BackgroundJobInput = interp.BackgroundJobInputEmpty
	// And it reads as nothing rather than as a zero: `echo "[$!]"` is `[]`.
	s.LastBackgroundPidIsZeroBeforeAnyJob = interp.No
	// DefaultOptionLetters stays empty on purpose: measured, dash's `$-`
	// starts blank however it is invoked, save the `s` of the
	// standard-input route, which is unanimous and comes from Runner.Route.
	// Under `-c` it stays blank — neither letter, where bash and ksh93 show
	// `c` and ksh93 also shows `s`.
	//
	// InteractiveOptionLetters stays empty too, and here that is an answer
	// rather than a silence: measured 2026-09-05, `-i script.sh` reports `i`
	// and nothing else, so the interactive set is the same empty set. dash is
	// the one shell in the panel that adds nothing at a prompt, which is what
	// makes the other three's additions evidence rather than a coincidence.
	// Login-ness written out, and the short spelling only: measured
	// 2026-09-05, `dash -l -c cmd` reads `~/.profile` and `dash --login`
	// is refused outright with `Illegal option --` at status 2, where the
	// other three take both. dash has no escape hatch of any kind, which is
	// also measured — a startup file that breaks it is escaped by moving
	// the file.
	s.StartupFileOptions = interp.StartupFileOptions{Login: "-l"}
	// The panel's only shell that goes on to read standard input as a program
	// once the `-c` string has run, when `-s` was given too. Measured
	// 2026-09-12: `printf 'echo LINE1\necho LINE2\n' | dash -sc 'echo
	// FROM-C'` writes all three lines where the other five write FROM-C
	// alone, and the order the two options are written in makes no
	// difference.
	s.StdinOptionSurvivesTheCommandString = true
	// Semantics.SystemStartupFiles stays at the POSIX preset's `/etc/profile`,
	// which is measured for this shell and not only inherited: `dash -l -c
	// 'echo ${PATH%%:*}'` with a `~/.profile` that reports the same thing
	// sees `path_helper`'s answer in both places, where a shell that read no
	// system file would have seen the inherited one.
	// And no version option, which is left as the zero value deliberately:
	// measured 2026-09-11, `dash --version` is `Illegal option --` at status
	// 2, the same refusal every long option but `--login`'s absence gets.
	// This shell is the panel's only one that will not name its version.
	s.CommandStringShowsCInDollarDash = interp.No
	s.LoginShowsLInDollarDash = interp.No
	s.CommandStringShowsSInDollarDash = interp.No
	// The order of the letters, and this shell is the reason the axis is a
	// sequence rather than a discipline: it is neither sorted, nor the order
	// the script set them in, nor capitals apart. Measured 2026-09-12, the
	// `s` rows by feeding the program on standard input and the last through
	// a pseudo-terminal:
	//
	//	set -f; set -u; set -e             ufe
	//	set -a -b -C -e -f -u -v -E -I     ubaCEvIfe
	//	set -x -a -C -e -u -v              uaCvxe
	//	set -C -e, on stdin                Cse
	//	set -x -C -v -u -a -f -e, on stdin uaCvxsife
	//	-i -c, at a terminal               mi
	//
	// `n` is in the string and is the one letter here nobody can measure
	// directly: the option it stands for stops the `echo` that would read
	// `$-`. Its place is the one the rest of the order implies — this
	// shell's `set -o` listing runs `errexit noglob ignoreeof interactive
	// monitor noexec stdin xtrace verbose vi emacs noclobber allexport
	// notify nounset`, which is this string reversed, and `noexec` sits
	// between `stdin` and `interactive` there. The `mi` row is the check on
	// that: `m` precedes `i` in `$-` exactly as the reversal predicts, and
	// nothing else in the panel puts them that way round.
	s.DollarDashLetterOrder = "ubaCEVvxsnmiIfe"
	// This shell has no `typeset`, but it can still be asked and it answers:
	// `readonly` is POSIX and `local` is the one declaration here, so the
	// two words that make the question both exist. Measured 2026-09-07:
	//
	//	readonly x=1
	//	f() { local x=2; echo "in=[$x]"; }
	//	f
	//	→ local: x: is read only, and the script ends
	//
	// So `No`, and now for a measured reason rather than because the
	// question looked out of reach — it was reached with the shell's own
	// spelling of the freeze rather than with bash's.
	s.DeclarationMayShadowAReadonly = interp.No
	// ReadonlyAttributeCanBeRemoved stays unanswered: there is no `typeset`
	// or `declare` here to write a plus form with, and `readonly +r x` is a
	// bad variable name rather than an option — so nothing can ask it, and
	// nothing will reach the refusal an unanswered axis makes.
	s.DeclaredNameWithoutValueIsEmpty = interp.No
	// $(( )) with nothing in it wants a primary and stops the script.
	s.EmptyArithExpressionIsAnError = interp.Yes
	// A bare read wants a name here, where the other three fill REPLY.
	s.ReadRequiresAVariableName = interp.Yes
	// The odd one out on `echo … | sh`: dash takes the program off standard
	// input in blocks and keeps what it took, so a `read` in the script
	// finds end of input and the data line is run as a command. The other
	// three read a line at a time and leave the rest on the descriptor.
	s.StdinProgramReadInBlocks = true
	// `-c` and `-s` together: the command string names the operands here,
	// so `sh -sc CMD name a` has `$0` of `name` and one parameter. ksh93
	// and zsh let `-s` name them instead.
	s.StdinOptionNamesTheOperands = interp.No
	// read takes -r and, alone among its letters, bash's -p prompt — an
	// argument, printed only to a terminal. The rest of bash's set (-s, the
	// counts, -d, -t, -u) is refused as unknown here.
	s.ReadOptions = "rp:"
	// dash has the two POSIX letters and calls anything else illegal.
	s.UnsetOptions = "vf"
	s.ExportListing = interp.DeclareListingCommandWord
	s.ReadonlyListing = interp.DeclareListingCommandWord
	// dash single-quotes every listed value; it has no declare, so this
	// style exists for the two -p listings alone.
	s.DeclareValueQuoting = interp.ListingQuoteAlwaysEscaped
	s.EchoInterpretsEscapes = interp.Yes
	// Neither spelling of the escape character: this shell's set is the XSI
	// list alone, so `\e` and `\E` are the two characters they are written
	// as.
	// This shell expands escapes without `-e` and still has neither of these:
	// `echo 'a\u0041Z'` is the characters as written.
	s.EchoExpandsUnicodeEscapes = interp.No
	s.EchoExpandsEscEscape = interp.No
	s.EchoExpandsCapitalEscEscape = interp.No
	s.LengthOfSpecialIsCount = interp.No
	s.UnterminatedBracket = interp.BracketNoMatch
	// The bash column's reading of a value's backslash, measured the same
	// way and on the same day: `v='a\*'; set -- $v` is `a\*` here, so the
	// `*` behind the backslash is not a metacharacter (#1367).
	s.ValueBackslashInAPattern = interp.ValueBackslashQuotesWhatFollows
	// An empty positional list is a set parameter here, with zsh: measured
	// 2026-09-12, `set --; "${@-word}"` is empty and `"${@+word}"` is
	// `word`, where bash and ksh93 answer the other way round (#1941).
	s.PositionalListWithNoneIsSet = interp.Yes
	// The value is expanded and the redirection opened before the prefix is
	// checked, so a failure in either is what gets reported and the frozen
	// name is never named. Measured 2026-09-12, with ksh93 and zsh against
	// the three bash builds (#1943).
	s.PrefixToAFrozenNameIsCheckedFirst = interp.No
	// The mask reaches the trim, so an escaped IFS whitespace character
	// closing a `read` value is data and stays — this shell alone.
	// Measured 2026-09-12: `printf 'a b c\\ \n' | read x y` leaves `b c `
	// here and `b c` in the other five (#1360).
	s.ReadTrailingEscapedSeparator = interp.ReadTrailingEscapedSeparatorKept
	// `.` with no filename at all is not an error here: dash does nothing and
	// reports success, where the other three complain. Everything else about
	// `.` and `eval` is the POSIX answer, which dash keeps and the others have
	// each moved away from.
	s.DotWithNoOperandIsAnError = interp.No
	// And a directory operand is no error either: measured, `. ./` is
	// silent at status 0, which zsh agrees with and bash and ksh93 do not.
	s.DotDirectoryOperandIsAnError = interp.No
	s.ExitTrapRunsOnSignalDeath = interp.No
	s.QuitIgnoredWhenNotInteractive = interp.No
	s.HangupIsAnOrderlyExit = interp.No
	s.ExitInTrapReportsEarlierStatus = interp.Yes
	s.KillListAcceptsName = interp.No
	s.SIGPrefixAccepted = interp.No
	s.RedirectsUseEveryTarget = interp.No
	s.KillStatus = interp.KillStatusAnyFailure
	s.SubshellJobTable = interp.SubshellJobsCleared
	s.PrintfEmptyIsNotANumber = interp.No
	s.PrintfReportsBadNumber = interp.Yes
	s.PrintfBackslashC = interp.PrintfBackslashCLiteral
	s.PrintfUnfinishedConversionIsAPercent = interp.No
	// No `\x` in a format at all: `printf 'a\x41Z'` is the six characters
	// as written, which is the whole panel's one holdout.
	s.PrintfHexEscape = interp.PrintfHexEscapeAbsent
	// Nor in a `%b` argument, and neither spelling of the escape character.
	s.PrintfBHexEscape = interp.PrintfHexEscapeAbsent
	// No `\u` or `\U` at either site: `printf 'a\u0041Z'` is the ten
	// characters as written, as it is for this shell's `\x`.
	s.PrintfUnicodeEscape = interp.PrintfUnicodeEscapeAbsent
	s.PrintfBUnicodeEscape = interp.PrintfUnicodeEscapeAbsent
	s.PrintfBEscEscape = interp.No
	s.PrintfBCapitalEscEscape = interp.No
	// The octal needs no `\0` here, which is the one thing this shell and
	// bash agree on that ksh93 and zsh do not.
	s.PrintfBOctalWithoutZero = interp.Yes
	s.PrintfBStopIsPadded = interp.Yes
	// None: `%ld` is the conversion `l`, which dash does not have.
	s.PrintfLengthModifiers = interp.PrintfLengthModifiersAbsent
	// No `%(fmt)T`: `%(` is a directive this shell does not have.
	s.PidListingFinishesWithAJob = interp.No
	s.PrintfTimeConversion = interp.No
	s.PrintfTimeOperandIsADateString = interp.No
	s.PrintfQuote = interp.PrintfQuoteAbsent
	// The three `$'…'` axes are left unanswered on purpose: dash has no
	// `$'…'` at all — `$'a\tb'` is the six characters it was written as,
	// dollar included — so the grammar refuses the form before any of them
	// can be asked. An answer here would be an invention.
	s.GetoptsAssignmentRestartsWord = interp.Yes
	s.GetoptsClearsOptarg = interp.No
	// Measured 2026-09-12: `OLDPWD=/nonexistent dash -c 'echo $OLDPWD'` answers
	// the path it was given, and `cd -` then answers `can't cd to` it at 2.
	s.InheritedOldpwd = interp.InheritedOldpwdTaken
	s.CdWithoutHomeIsAnError = interp.No
	s.CdDashPrintsTheDirectory = interp.Yes
	s.PrintfAssignsWithV = interp.No
	s.PrintfRejectsUnknownOption = interp.Yes
	// dash has no options here at all, so every letter is refused.
	s.TrapParsesOptions = interp.Yes
	s.TrapPrintsWithP = interp.No
	s.TrapPrintsBareWithConditions = interp.No
	s.TrapPrintsBareWithP = interp.No
	s.TrapListsSignalsWithL = interp.No
	s.TrapOneArgumentIsACondition = interp.Yes
	s.TrapReportsAnUnknownSingleCondition = interp.Yes
	s.TrapSingleUnknownConditionIsUsage = interp.No
	// The sole holdout on the pseudo-conditions: `trap … ERR` is refused
	// with the same words any word that names no signal gets, and DEBUG
	// and RETURN with it. Measured, and the refusal does not end the
	// script — the status is 1 and the next command runs.
	s.TrapHasErrCondition = interp.No
	s.TrapHasDebugCondition = interp.No
	s.TrapHasReturnCondition = interp.No
	// A subshell's listing shows only what survived the entry — the ignored
	// signals — in every boundary measured: `(trap)`, `$(trap)`, a pipeline
	// element and a background job all print nothing for a handled trap and
	// `trap -- '' INT` for an ignored one. The POSIX answer, restated here
	// because it was measured rather than assumed. Whether a kept listing
	// includes EXIT is left unanswered: nothing is ever kept to ask it of.
	s.SubshellKeepsTrapListing = interp.No
	s.PipelineElementKeepsTrapListing = interp.No
	s.BackgroundJobKeepsTrapListing = interp.No
	s.SubshellHidesInheritedIgnoredTraps = interp.No
	s.UmaskPrintsFourDigits = interp.Yes
	// dash parses no options for `alias`, so `-p` is a name there.
	s.AliasParsesOptions = interp.No
	s.AliasHasPrintOption = interp.No
	// It reads no options at all, so `-g` and `-s` are names it cannot
	// find rather than kinds it has.
	s.GlobalAliases = interp.No
	s.SuffixAliases = interp.No
	s.AliasListsAsDefinitions = interp.No
	s.AliasRestrictsToRegularKind = interp.No
	s.AliasOperandsCanBePatterns = interp.No
	s.AliasPlusPrintsNamesOnly = interp.No
	s.TypeNamesAnAliasOnlyWhenExpanded = interp.No
	s.AliasReportsNotFound = interp.Yes
	s.UnaliasReportsNotFound = interp.Yes
	s.AliasNotFoundStatusCounts = interp.No
	s.UnaliasAllRefusesOperands = interp.No
	s.AliasQuoting = interp.ListingQuoteAlwaysDoubled
	s.TrapQuoting = interp.ListingQuoteAlwaysDoubled
	s.TrapActionIsParsedWhenSet = interp.No
	s.TrapParseFailureNamesWhereItFired = interp.No
	s.SymbolicMaskTakesMoreThanOneOperator = interp.Yes
	s.SymbolicMaskWhoAloneSetsIt = interp.No
	s.SymbolicMaskTakesTheSetuidLetter = interp.Yes
	s.SymbolicMaskTakesTheStickyLetter = interp.No
	// No options and no marker: `shift -x`, `shift -1` and `shift --` are all
	// numbers this shell calls illegal, which is the one wording it has here.
	s.ShiftOptionWords = interp.ShiftOptionWordsNone
	s.ShiftDoubleDashEndsOptions = interp.No
	s.ShiftNamesAreArrays = interp.No
	s.ShiftNegativeIsOutOfRange = interp.No
	s.WaitReadsOptions = interp.Yes
	// Only numbers, `%%`, `%+` and `%-` resolve here: a `%name` is a job
	// that is not there. `wait` complains about it with its own wording and
	// status, and there is no -n and no disown at all.
	s.JobSpecsByName = interp.No
	s.WaitReportsAMissingJob = interp.Yes
	s.WaitNWaitsForTheNextJob = interp.No
	// A trapped signal cuts a `wait` short with 128 plus the signal, and the
	// form that names a job answers the same as the bare one.
	s.WaitForAJobFailsWhenInterrupted = interp.No
	s.CommandRejectsUnknownOption = interp.Yes
	s.GetoptsRejectsUnknownOption = interp.No
	s.ShiftCountIsArithmetic = interp.No
	s.TrapBodyRunsWhatParsed = interp.Yes
	s.ReportsAKilledCommandInACommandSubstitution = interp.Yes
	s.UmaskSetWithSPrints = interp.No
	s.UlimitBlockIsKilobyte = interp.No
	s.UlimitHasResidentSet = interp.Yes
	s.UlimitHasProcessCount = interp.No
	s.UlimitSetsBothLimits = interp.Yes
	s.BadOptionToSpecialBuiltinFatal = interp.Yes
	// And so is a redirection that cannot be made, which is POSIX's rule
	// verbatim: `exec 3>/nope/x` stops the script, at 2 like every other
	// fatal error here.
	// The one shell in the panel that will not take a duplication target
	// wider than one digit — `>&10` and `>&08` alike, and `>&$n` once the
	// word has expanded to one. It words it as a syntax error and stops.
	s.MultiDigitDuplicationTargetIsAnError = interp.Yes
	s.RedirectErrorOnSpecialBuiltinFatal = interp.Yes
	// dash refuses the word while parsing — `Syntax error: Bad fd number`,
	// and nothing in the line runs. The grammar takes it here, so the
	// refusal is this axis and the wording is the ordinary one.
	s.GreatAmpTarget = interp.GreatAmpTargetIsADescriptor
	s.DuplicationTargetErrorOnABuiltinIsFatal = interp.No
	s.LocalOutsideAFunctionIsAnError = interp.Yes
	s.LocalOutsideAFunctionIsFatal = interp.Yes
	// A special builtin's failure is fatal, and a bad name is one — for all
	// three of them.
	s.BadNameToDeclarationFatal = interp.Yes
	s.BadNameToUnsetFatal = interp.Yes
	// `read` is not one of the three, so its bad name is reported at dash's
	// usual 2 and the script carries on.
	s.BadNameToReadFatal = interp.No
	// And so is a readonly name it is asked to remove.
	s.UnsetReadonlyFatal = interp.Yes
	s.DeclarationNameOperands = interp.PlainNamesOnly
	s.UnsetNameOperands = interp.PlainNamesOnly
	s.ReadNameOperands = interp.PlainNamesOnly
	// No prompt operand: `read "v?p"` is `v?p: bad variable name`.
	s.ReadPromptOperand = interp.ReadOperandIsAllName
	// dash reads the line first and refuses afterwards, so the line is gone:
	// `printf 'AAA\nBBB\n' | { read 1bad; cat; }` prints only BBB.
	s.ReadRefusesABadNameBeforeReading = interp.No
	// ReadCountJudgesTheNamesAfterTheFirst is left unanswered: dash's
	// `read` has no count letter — `-n` and `-N` are both `Illegal option`
	// — so nothing here can ask it.
	s.DeclarationTakesASubscript = interp.No
	// No arrays at all, so no subscripted operand is a name to any
	// declaration here either.
	// A fatal refusal takes the operands after it with it: measured
	// 2026-09-07, `export ok1=1 ":" ok2=2` leaves ok1 set and ok2 unset.
	// No arrays and no `read -a`, so neither edge question can be reached.
	// Answered anyway rather than left to refuse: the preset's reading is
	// the ordinary field split, which is this shell's everywhere else.
	s.ReadTrailingWhitespaceEndsAField = interp.No
	s.ReadNoFieldsIsOneEmptyElement = interp.No
	s.BadNameDeclaresTheOperandsAfterIt = interp.No
	s.TypesetTakesASubscript = interp.No
	s.UnsetTakesASubscript = interp.No
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.Yes
	// And a stopped job keeps the current-job marker, which is a separate
	// question from the order the rows come out in: measured 2026-09-12
	// through a pseudo-terminal, `sleep 40` stopped with ^Z and then
	// `sleep 41 &` lists `[1] + Suspended` before `[2] - Running`. This
	// shell will not resolve `%+` or `%-` at all — both are `No current
	// job` — so the listing is the whole of the evidence here.
	s.StoppedJobTakesTheCurrentJobMarker = interp.Yes
	s.JobsListFinishedJobs = interp.Yes

	// `jobs`' letters: POSIX's pair and nothing else. `-r`, `-s`, `-n` and
	// `-x` are all "Illegal option" here, which is why the letter set is a
	// dialect answer rather than one string in the engine.
	s.JobsOptions = "lp"
	s.JobsPidsOnlyOption = interp.Yes

	// Whether a `&` job's command appears in a `jobs` listing.
	s.JobsShowBackgroundCommand = interp.No

	// Whether a backgrounded job is announced to whoever is typing.
	// Whether `export -f` carries a function to a child.
	s.ExportCarriesFunctions = interp.No
	// And whether it has `-n` at all. It does not: measured, dash answers
	// `export: Illegal option -n` and the script ends there, `export` being
	// a special builtin.
	s.ExportTakesTheAttributeOff = interp.No
	s.AnnouncesBackgroundJob = interp.No
	// And nothing with the monitor off either, which is the same answer
	// reached twice: this shell announces neither end of a job in any state
	// (#1738).
	s.AnnouncesBackgroundJobWithoutTheMonitor = interp.No
	s.ReportsACommandKilledBySignal = interp.Yes
	s.ReportsAnyKilledPipelineElement = interp.Yes
	s.ChildInterruptEndsTheScript = interp.No
	// An empty operand and an empty HOME are both somewhere here: `cd ""`
	// and `HOME=; cd` say nothing and report 0, staying put.
	s.CdEmptyOperandIsAnError = interp.No
	s.CdEmptyHomeIsAnError = interp.No
	// And everything after the first operand is ignored rather than
	// refused: `cd alpha beta` goes to alpha and says nothing.
	s.CdSubstitutesTheOperands = interp.No
	s.CdSubstitutionPrintsTheDirectory = interp.No
	s.CdRefusesExtraOperands = interp.No
	s.CdRefusesUnknownOption = interp.Yes
	s.CdHasQuietOption = interp.No
	s.CdLastPathOptionWins = interp.Yes
	s.BadSetOptionNameFatal = interp.Yes
	// dash has no `[[ ]]` to ask it in; answered so that a shell built from
	// this preset with the construct turned back on is not left refusing.
	s.UnknownConditionOptionIsAStatus = interp.No
	s.ReturnOutsideAFunctionIsRefused = interp.No
	// `break` with no loop around it is ignored here, silently: measured,
	// `echo t; break; echo after` prints both and ends at 0, with no
	// wording to go with it.
	s.LoopControlOutsideALoopIsFatal = interp.No
	// A call is a boundary and a subshell is not, which is the pairing that
	// makes these two fields rather than one: `f(){ break; }` called from a
	// loop leaves the loop running, and `( break )` inside one leaves only
	// the subshell. Silently in both cases — the complaint is
	// LoopControlOutsideALoop's, and this shell writes none.
	s.FunctionCallIsALoopControlBoundary = interp.Yes
	s.SubshellIsALoopControlBoundary = interp.No
	// A startup file is a sourced script, so a `return` in one is accepted
	// everywhere — the split is only over what its argument does, and this
	// shell keeps it: measured through a pty, an rc of `return 3` and one
	// of `false; return 3` both leave `$?` as 3 at the first prompt, where
	// bash leaves 0 and 1. A `return` with no argument means the last
	// command's status here as everywhere.
	s.StartupFileReturnCarriesItsArgument = interp.Yes
	s.LoneDashIsAnOption = interp.No
	// And a lone `+` is a name to `export` and `readonly` here as well —
	// measured 2026-09-12, `export +` is `export: +: bad variable name` at
	// 2. There is no `typeset` in this shell for the other half of the
	// question, so only this field is answered.
	s.SignAloneIsAnOptionWordToExport = interp.No
	s.UnsetFunctionChecksTheName = interp.No
	s.UnsetFunctionReportsMissing = interp.No

	// Whether a redirection target is expanded as an ordinary word.
	s.RedirectTargetIsAnOrdinaryWord = interp.No

	// Whether `type --` ends the options.
	s.TypePrintsFunctionBody = interp.No
	s.TypeEndsOptionsWithDashDash = interp.No

	// `local` reads no options at all here — `local -r x` declares a
	// variable named `-r` and then refuses it as the bad name it is — so
	// LocalOptions stays empty. A bare `local` writes nothing, and a bare
	// `set` lists the variables alone, every value single-quoted with an
	// embedded quote doubled out: `'quo'"'"'te'`.
	s.BareLocalListing = interp.BareLocalListsNothing
	s.SetListing = interp.SetListingAssignments
	s.SetListingQuoting = interp.ListingQuoteAlwaysDoubled
	// Unreachable behind the answer above — dash's `type` has no options at
	// all, so `-t` is a name — but the axis is answered rather than left
	// looking forgotten.
	s.TypeNamesTheKindWithDashT = interp.No

	// A descriptor number the process cannot hold is not checked here: with
	// `ulimit -n 6`, `exec 8>f` reports success and the descriptor is
	// unusable afterwards, where bash and ksh93 hand the kernel's refusal
	// back. Unreachable for a two-digit number, which this shell does not
	// read as one at all.
	s.FdNumberBoundedByOpenFileLimit = interp.No

	return s
}

// Diagnostics is how dash reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		// A duplication target wider than one digit is refused before the
		// descriptor is looked at, and worded as a syntax error even though
		// the parse succeeded — no number, no file, one sentence.
		MultiDigitDuplicationTarget: "Syntax error: Bad fd number",

		TypeKeyword:  "%[1]s is a shell keyword",
		TypeFunction: "%[1]s is a shell function",
		// `a is an alias for echo hi`, body raw.
		TypeAlias:     "%[1]s is an alias for %[2]s",
		CommandVAlias: "alias %[1]s=%[2]s",
		// No shell name and no location in front of it, alone among the
		// messages dash prints.
		TypeNotFound:           "%[1]s: not found",
		TypeNotFoundUnprefixed: true,
		// And on standard output, where the sentences about the names it
		// *could* account for go. dash has no option letters on `type` at
		// all, so `type -t f cd if ls` reads every word as a name and prints
		// five lines — two misses and three answers — in one stream, in
		// order. Measured: `type nope 1>/dev/null` prints nothing.
		TypeNotFoundOnStdout: true,
		// `command -V` says it the same way, shell's name and all.
		CommandVNotFound: "%[1]s: not found",
		// And a missing *command*'s status rather than a plain failure.
		TypeNotFoundStatus: 127,
		// `[1] + ` — a space each side of the marker — then a 27-wide state.
		// The command column is empty for a `&` job because dash kept no
		// text for one, which JobKeepsBackgroundCommand answers; a job dash
		// stopped itself does print its command here.
		JobLine: "[%[1]d] %[2]s %-27[3]s%[4]s",
		// `jobs -l`: the process id after the marker, and the state column
		// narrowed by exactly what the id took — dash is the one shell in
		// the panel that keeps the command column where it was. The 21 is
		// the measured width for a five-digit id; nothing sees the
		// difference, because dash's command column is empty for a `&` job
		// and what moves is trailing whitespace.
		JobLineLong: "[%[1]d] %[2]s %[3]d %-21[4]s%[5]s",
		// The words and nothing else — no process id, no command, and alone
		// among this shell's messages, no name and no line in front of it.
		KilledCommandNotice: "%[2]s",
		// The operator is part of the sentence here: this shell writes `-o`
		// whichever way it was asked.
		ParamNullOrNotSet:    "parameter not set or null",
		SetInvalidOptionName: "set: Illegal option -o %[1]s",
		// The sign is part of the sentence for the same reason `-o` is:
		// this shell writes `-q` for `set +q` as well as for `set -q`, so
		// the wording takes the bare letter and spells the dash itself.
		SetInvalidOptionLetter: "set: Illegal option -%[2]s",
		UnimplementedOptionLetters: map[string]string{
			// `set` letters dash has and this shell does not: -b job
			// notices, -i interactive, -s reading standard input, and the
			// three about its line editor and end-of-file — -E emacs, -V
			// vi, -I ignoreeof. Measured 2026-09-05 by asking dash for
			// every letter of the alphabet in both cases and both signs.
			"set": "bisEIV",
		},
		// Said whichever spelling asked, so the verb goes unused; status 0,
		// the field's default, is what makes it a remark rather than an
		// error.
		MonitorDenied: "set: can't access tty; job control turned off",
		// The same sentence without `set: ` in front of it, because nothing
		// asked: this one is the shell's own decision at startup rather than a
		// builtin refusing. It is located, like every other dash diagnostic —
		// `<name>: 0: …`, at the line it has not reached yet, which
		// InvocationNamesTheUnreadLine already spells.
		NoJobControlAtStartup: "can't access tty; job control turned off",
		// And it names the script it was handed rather than itself, which is
		// `$0` — measured, `-i script.sh` writes the script's path here and
		// `-i -c` and `-i -s` write dash's own. bash writes its own name on
		// all three.
		NoJobControlAtStartupNamesTheScript: true,
		// Backticks alone: this shell numbers a `$( … )` body from the file
		// like the other three, and a backquoted one from one.
		BackquotedSubstitutionRestartsLines: true,
		KilledCommandNoticeUnprefixed:       true,
		JobRunning:                          "Running",
		// dash names the signal that stopped it rather than calling it
		// stopped: `Suspended: 18`, where 18 is SIGTSTP.
		JobStopped: "Suspended: %[1]d",
		// ^Z prints the listing's own row, straight after the `^Z` the
		// terminal echoed — no newline of its own, which is where dash and
		// ksh93 part company with bash and zsh. Measured through a
		// pseudo-terminal on 2026-09-05.
		//
		// `fg` names the command alone, which is the empty default; `bg`
		// prints the job number and the command, with no marker and no `&`.
		JobResumedInBackground: "[%[1]d] %[3]s",
		JobDone:                "Done",
		JobExited:              "Done(%[1]d)",
		Location:               interp.LocationColonLine,
		// The one shell in the panel that tells neither failure from the
		// other: a script operand that is missing and one that will not open
		// share a wording and a status, and the status is the 2 it gives a
		// usage error rather than the 127 or 126 the other three reach for.
		// The reason is its own — `No such file`, from FileNotFound — and it
		// writes the line it has not reached yet, `<shell>: 0: cannot open …`,
		// which nothing else in the panel does.
		ScriptNotFound:               "cannot open %[1]s: %[2]s",
		ScriptNotFoundStatus:         2,
		ScriptNotReadableStatus:      2,
		InvocationNamesTheUnreadLine: true,
		// dash names what it wanted instead, and calls the token by its
		// class rather than by name.
		// A bad digit is not a diagnosis dash reaches at all: the literal
		// simply ends there and what follows is left over, so `08`, `09`,
		// `0778` and `0x1z` are all "expecting EOF" — the same sentence it
		// gives $((1 2)). Ours calls it a digit too great for its base and
		// dash words that as the leftovers it would have seen.
		DigitTooGreatForBase:  "expecting EOF",
		ArithOperandExpected:  "expecting primary",
		ArithOperatorExpected: "expecting EOF",
		SyntaxUnexpected:      "Syntax error: \"%[1]s\" unexpected",
		SyntaxUnexpectedWord:  "Syntax error: word unexpected",
		// And the newline is unquoted with it — `newline unexpected` beside
		// `";;" unexpected` — and blamed on the line it ends (#1364).
		// A line typed at a prompt is numbered by the session: the second
		// line typed is line 2, for a parse failure and for a command that
		// was not found alike (#2022).
		PromptCountsTheSessionsLines:     true,
		SyntaxUnexpectedNewline:          "Syntax error: newline unexpected",
		UnexpectedNewlineIsOnTheNextLine: true,
		SyntaxExpecting:                  " (expecting \"%[1]s\")",
		// dash does not name the token when it is a redirection operator.
		SyntaxRedirectUnexpected: "Syntax error: redirection unexpected",
		ForName:                  "Syntax error: Bad for loop variable",
		Unterminated:             "Syntax error: end of file unexpected (expecting \"%[4]s\")",
		// With nothing open there is nothing it could have been expecting,
		// and the parenthetical goes rather than standing empty.
		UnterminatedNoConstruct: "Syntax error: end of file unexpected",
		UnmatchedQuote:          "Syntax error: Unterminated quoted string",
		UnmatchedBackquote:      "Syntax error: EOF in backquote substitution",
		UnmatchedCmdSubst:       "Syntax error: end of file unexpected (expecting \")\")",
		UnmatchedBraceSubst:     "Syntax error: Missing '}'",
		// `echo ${x` with the input running out at the newline after the
		// name is a line earlier here than `echo ${ echo hi` is, although
		// this shell has no command form and reads both as an ordinary
		// expansion. See UnmatchedBraceSubstDropsTheNameNewline for the
		// rows (#2232).
		UnmatchedBraceSubstDropsTheNameNewline: true,
		// This shell names the two characters that never came rather than
		// the one the other dialect echoes, so the closer is written in
		// rather than taken from the construct. It has no `$[` at all —
		// `echo $[1+2` runs and prints the text — so there is one spelling
		// to word here and not two.
		UnmatchedArithSubst: "Syntax error: Missing '))'",
		SyntaxError:         "Syntax error: %[1]s",
		BadSubstitution:     "Bad substitution",
		// The bare name and the shell's own sentence for a name nothing may
		// be assigned to: `${@:=w}` with no parameters is `@: bad variable
		// name`, and `${1:=w}` is `1: bad variable name`. It exits 2 where
		// the others exit 1, which FatalErrorStatusIsOne already answers
		// (#1541).
		AssignThroughExpansionBadName: "%[1]s: bad variable name",
		// The builtin in front, which its plain form does not have.
		ReadonlyVariableInDeclaration: "%[2]s: %[1]s: is read only",
		// And `local`, the third and last declaration this shell has:
		// measured 2026-09-07, `readonly x=1; f() { local x=2; }; f` is
		// `local: x: is read only`. It names all three of them, where bash
		// names only the two POSIX has not got (#1168).
		ReadonlyRefusalNamesBuiltin: map[string]bool{"export": true, "readonly": true, "local": true},
		ReadonlyVariable:            "%s: is read only",
		UnsetReadonly:               "unset: %s: is read only",
		InvalidNumber:               "Illegal number: %s",
		// A conditional with no `:` says which byte it wanted; a missing
		// value is the ordinary `expecting primary`, so ArithConditionalValue
		// stays empty.
		ArithConditionalColon: "expecting ':'",
		NumericArgument:       "%[1]s: Illegal number: %[2]s",
		ArithError:            "arithmetic expression: %[2]s: \"%[1]s\"",
		FileNotFound:          "No such file",
		TestNamesFirstOperand: true,
		// 2 rather than the 1 the other three report, for a read and a write
		// alike. Not fatal — the script carries on — so this is a different
		// question from FatalErrorStatusIsOne, which is about a failure that
		// ends the shell.
		RedirectFailureStatus: 2,
		// A verb, and a different one for each direction.
		CannotOpen:   "cannot open %[1]s: %[2]s",
		CannotCreate: "cannot create %[1]s: %[2]s",
		// Under `set -C` a target that is not a regular file is opened
		// rather than created — the file is already there — and dash is the
		// one shell whose verb follows: `cannot open d: Is a directory` with
		// the option on, `cannot create d: Is a directory` with it off,
		// measured 2026-09-10 against dash 0.5.12.
		NoclobberFallbackIsAnOpen: true,
		// The name twice — `dash: 1: echo: echo: I/O error` — and its own
		// fixed reason whatever the errno was, so the format never mentions
		// %[2]s. Measured on echo, printf, pwd and type: the doubling is the
		// builtin's, not the location's.
		BuiltinWriteError: "%[1]s: %[1]s: I/O error",
		// Its own text for ENOENT, and a different one each way: a read that
		// finds nothing is "No such file", a write that cannot make one is
		// "Directory nonexistent". The OS says "No such file or directory"
		// for both.
		DirectoryNotFound: "Directory nonexistent",
		ShiftTooMany:      "shift: can't shift that many",
		SyntaxErrorStatus: 2,
		// dash ends the script rather than reporting a status here, so only the
		// wording speaks — and it uses two, where the other three use one.
		//
		// The reason verb goes unused in the first: dash truncates strerror's
		// "No such file or directory" to "No such file" for this message
		// alone, so the text is spelled out rather than taken from the error.
		// An indexed format may ignore an argument, which is what makes that
		// safe.
		DotCannotOpen: ".: cannot open %[1]s: No such file",
		DotNotFound:   ".: %[1]s: not found",
		// A command word names no builtin; `exec` names itself.
		CannotExecute:     "%[1]s: %[2]s",
		ExecCannotExecute: "exec: %[1]s: %[2]s",
		ExecNotFound:      "exec: %[1]s: not found",
		// One message for either operator position.
		// dash names no pid at all, and prints a blank line after the one
		// message it has for a target — measured rather than assumed, because
		// an invisible trailing newline is exactly what a golden record is for.
		// No reason at all, so `cd` onto a file and `cd` onto nothing read
		// identically here — the one shell whose message cannot tell you
		// which it was.
		GetoptsBadOption:       "Illegal option -%[1]s",
		GetoptsMissingArgument: "No arg for -%[1]s option",
		// Nothing in front of it at all — the only diagnostic in the panel
		// that names neither the shell nor a line.
		GetoptsUnprefixed: true,
		CdCannotChange:    "cd: can't cd to %[1]s",
		CdStatus:          2,
		PrintfBadNumber:   "printf: %[1]s: expected numeric value",
		// Every complaint about an argument is 2 here, as it is elsewhere.
		PrintfBadVerbStatus:     2,
		PrintfBadVerb:           "printf: %[2]s: invalid directive",
		PrintfMissingVerb:       "printf: missing format character",
		PrintfMissingVerbStatus: 2,
		PrintfBadOption:         "printf: Illegal option %[1]s",
		UmaskBadMask:            "umask: Illegal number: %[1]s",
		// No shell and no line in front of either, which dash does almost
		// nowhere else.
		AliasNotFound:             "%[1]s: %[2]s not found",
		AliasNotFoundUnprefixed:   true,
		UnaliasNotFound:           "%[1]s: %[2]s not found",
		UnaliasNotFoundUnprefixed: true,
		// dash quotes the whole argument back and does not say what in it
		// was wrong, so there is no operator wording to go with this.
		UmaskBadSymbolicMode:  "umask: Illegal mode: %[1]s",
		UmaskBadOption:        "umask: Illegal option %[1]s",
		UmaskBadMaskStatus:    2,
		UlimitBadOption:       "ulimit: Illegal option -%[1]s",
		UlimitBadNumber:       "ulimit: bad number",
		UlimitBadNumberStatus: 2,
		BuiltinBadOption:      "%[1]s: Illegal option %[2]s",
		// The same sentence kill already had for its own missing argument,
		// measured for the builtins' shared reader with `read -p`.
		OptionNeedsArgument: "%[1]s: No arg for -%[2]s option",
		ShiftBadNumber:      "shift: Illegal number: %[1]s",
		WaitBadJob:          "wait: Illegal number: %[1]s",
		WaitBadJobStatus:    2,
		WaitNoSuchJob:       "wait: No such job: %[1]s",
		WaitNoSuchJobStatus: 2,
		KillNoSuchJob:       "kill: No such job: %[1]s",
		// The same shape for `jobs`, `fg` and `bg`, which is dash's house
		// order everywhere: the sentence first and the spec after it.
		// Measured on `jobs %9`, `fg %9` and `bg %9` — dash is the one shell
		// that reaches this for all three in a script.
		NoSuchJob: "%[1]s: No such job: %[2]s",
		// And dash's usage number rather than a plain failure, which is what
		// it reports for every one of the three.
		NoSuchJobStatus:       2,
		LocalOutsideAFunction: "local: not in a function",
		// One wording for all three, naming the part in front of any `=`.
		BuiltinBadName: map[string]string{
			"export":   "%[1]s: %[2]s: bad variable name",
			"readonly": "%[1]s: %[2]s: bad variable name",
			"unset":    "%[1]s: %[2]s: bad variable name",
			"local":    "%[1]s: %[2]s: bad variable name",
			// `read` says the same and carries dash's 2 with it, which is
			// how a caller tells it from the 1 that means end of input.
			"read": "%[1]s: %[2]s: bad variable name",
		},
		// A `local` name that starts with a digit is refused with the
		// builtin's name left off — `1y: bad variable name` against
		// `local: -r: bad variable name` — where its other bad names keep
		// it. Measured from both shapes.
		BuiltinBadNameNumeric: map[string]string{
			"local": "%[2]s: bad variable name",
		},
		BuiltinBadNameStatus: 2,
		// `ulimit -a`, row for row as the engine writes it.
		UlimitListing: []interp.UlimitListingRow{
			{Prefix: "time(seconds)        ", Res: interp.ResourceCPUTime, Scale: 1},
			{Prefix: "file(blocks)         ", Res: interp.ResourceFileSize},
			{Prefix: "data(kbytes)         ", Res: interp.ResourceData, Scale: 1024},
			{Prefix: "stack(kbytes)        ", Res: interp.ResourceStack, Scale: 1024},
			{Prefix: "coredump(blocks)     ", Res: interp.ResourceCore},
			{Prefix: "memory(kbytes)       ", Res: interp.ResourceResidentSet, Scale: 1024},
			{Prefix: "locked memory(kbytes) ", Res: interp.ResourceLockedMemory, Scale: 1024},
			{Prefix: "process              ", Res: interp.ResourceProcesses, Scale: 1},
			{Prefix: "nofiles              ", Res: interp.ResourceOpenFiles, Scale: 1},
			{Prefix: "vmemory(kbytes)      ", Res: interp.ResourceAddressSpace, Scale: 1024},
		},
		PrintfUsage:             "printf: usage: printf format [arg ...]",
		TrapBadSignal:           "trap: %[1]s: bad trap",
		TrapBadSignalUnprefixed: true,
		KillNoSuchProcess:       "kill: No such process\n",
		KillNotPermitted:        "kill: Operation not permitted\n",
		KillInvalidSignal:       "kill: invalid signal number or name: %[1]s",
		// The first character alone: dash stopped reading there.
		KillIllegalOption:         "kill: Illegal option -%[2]s",
		KillNotAPid:               "kill: Illegal number: %[1]s",
		KillMissingSignalArgument: "kill: No arg for %[1]s option",
		KillUsage: "kill: Usage: kill [-s sigspec | -signum | -sigspec] [pid | job]... or\n" +
			"kill -l [exitstatus]",
		// dash answers every complaint about its arguments with 2, which is the
		// one dialect where the option and the operand are not two questions.
		KillUsageStatus:     2,
		KillBadOptionStatus: 2,
		KillArgumentStatus:  2,
		// `[ -Q x -a -n x ]` is `[: -Q: unexpected operator` here, naming the
		// first of the two operands the primary is left with (#1290).
		TestUnknownLongOperator: interp.TestUnknownOperatorLeavesAnOperand,
		TestUnaryExpected:       "%[2]s: %[1]s: unexpected operator",
		TestBinaryExpected:      "%[2]s: %[1]s: unexpected operator",
		TestIntegerExpected:     "%[2]s: Illegal number: %[1]s",
		TestTooManyArguments:    "%[2]s: too many arguments",
		TestOperandExpected:     "%[2]s: argument expected",
		TestMissingBracket:      "[: missing ]",
		// Six decimal places, the most of any shell in the panel.
		TimesDecimals: 6,
		// dash hands the path to execve rather than checking first, so a
		// directory comes back as a permission error.
		DirectoryReason:     "Permission denied",
		ReadArgCount:        "read: arg count",
		OptionListingHeader: "Current option settings",
		OptionListingWidth:  16,
		KillListing:         interp.KillListingZeroFirst,
		// And when that directory was the PATH search's only match, dash
		// names it in the message and still numbers the failure 127.
		DirectoryOnPathStatus: 127,
	}
}

// Apply makes any adjustment that is not a vector value. dash needs none:
// it has `local`, which is the only builtin the panel disagrees about.
func Apply(r *interp.Runner) {
	// The prompt table, installed for the same reason the other three
	// dialects install theirs even though this one's has no escape language
	// in it: what a shell does to a prompt parameter before drawing it is the
	// dialect's answer, and "nothing but expansion" is an answer rather than
	// an absence. A runner told it draws `\u` as `\u` because this table says
	// so, not because nobody told it anything. #1455.
	r.SetPromptStyle(PromptStyle())
	// dash has no `builtin`.
	r.Unregister("builtin")
	// No `compgen` here; it is bash's alone.
	r.Unregister("compgen")
	// fc really is an external here: `command -v fc` answers /usr/bin/fc.
	r.Unregister("fc")
	r.Unregister("complete")
	// And neither `mapfile` nor its other name; both are bash's alone.
	r.Unregister("mapfile")
	r.Unregister("readarray")
	// dash has neither `typeset` nor `declare`. The core provides `typeset`
	// because three of the four do; the one that does not takes it away, the
	// same way ksh93 takes `local` away.
	r.Unregister("typeset")
	// dash has no disown: real process groups or not, the name is simply
	// not a builtin there and resolves like any other missing command.
	r.Unregister("disown")
	// And no `let`. The other three evaluate arithmetic with it; dash has
	// only `$(( ))`, and reports `let: not found` like any other command it
	// has never heard of.
	r.Unregister("let")
	// This shell has no `enable` either.
	r.Unregister("enable")
}
