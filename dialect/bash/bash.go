// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package bash answers the substrate's questions the way bash 5 does.
package bash

import (
	"os"
	"strconv"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what bash 5 parses.
func Dialect() syntax.Dialect {
	d := syntax.Core()
	// bash expands them interactively and needs `shopt -s expand_aliases`
	// otherwise, which is not modeled yet — so no route, rather than a route
	// this shell only takes with an option set. Measured on all three.
	d.ExpandAliases = syntax.AliasOnNoRoute
	// And a body's newlines do not count: bash alone leaves the whole of an
	// expanded body on the line the alias word was written on.
	d.AliasBodyCountsLines = false
	// The utilities that take an array assignment as an operand. bash has
	// all five.
	d.DeclarationUtilities = map[string]bool{
		"declare": true, "typeset": true, "local": true,
		"export": true, "readonly": true,
	}
	d.CaseContinue = true
	d.ParamCaseChange = true
	// `${x@Q}` and the rest of the letter family, which no other shell in
	// the panel has: the others report a bad substitution when the
	// expansion is reached.
	d.ParamTransformations = true
	d.ParamIndirection = true
	// `$"..."`, the locale-translatable string: with no catalog it is a
	// plain double-quoted string with the `$` stripped. Not core, because
	// dash and zsh keep the `$` as a literal.
	d.DollarDoubleQuote = true
	// `${ cmd;}`, a command substitution that runs in the current shell
	// so that what it assigns survives. The space after the brace is the
	// whole of the grammar: `${x}` is a parameter and `${ x}` is not.
	d.CurrentShellSubstitution = true
	d.FunctionKeywordParens = true
	// A name followed by `(` is a function definition here, whether or not
	// the `)` comes next.
	d.FuncDefAtParen = true
	// And having committed, the body must be compound: `f() echo hi` is a
	// syntax error here and a one-command function in the other three.
	d.FuncBodyMustBeCompound = true
	// And inside `[[ ]]`, which is the only place bash reads them.
	d.ExtendedPatternInCondition = true
	// A bare `|` in a `=~` operand belongs to the regular expression.
	d.RegexTakesAlternation = true
	// `time -p`, the POSIX report format. bash and ksh93 read the flag;
	// zsh leaves `-p` to the pipeline, which is why it is not core.
	d.TimePosixFlag = true
	// `coproc cat` with the near ends in COPROC. zsh's coprocess speaks
	// `print -p` rather than an array and is a different feature.
	d.Coproc = true
	// And the way that array is closed: `exec {COPROC[1]}>&-` names the
	// element holding the feed. Not core because zsh has the `{name}` token
	// and still reads a subscripted one as a word.
	d.FdVariableSubscript = true
	return d
}

// Semantics is what bash 5 means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	// The panel's holdout on the login profile, measured on all four
	// non-interactive routes and in both bash 5.3 and the 3.2 macOS ships:
	// `exec -a -bash bash script.sh` reads neither ~/.bash_profile nor
	// /etc/profile, where dash, ksh93 and zsh all read theirs. Being a login
	// shell is not the part it declines — `shopt login_shell` is on — it is
	// that a non-interactive shell reads no startup file unless `--login`
	// was written out, and this front end has no `--login` to write.
	s.LoginProfileWhenNonInteractive = false
	// `-c` and `-s` together: the command string names the operands here,
	// so `sh -sc CMD name a` has `$0` of `name` and one parameter — the
	// same answer in the 3.2 macOS ships. ksh93 and zsh let `-s` name them.
	s.StdinOptionNamesTheOperands = interp.No
	s.CommandNotFoundStatusIsNotFound = interp.No
	s.SetFTurnsOffGlobbing = interp.Yes
	// Measured: `echo $-` reports `hB` — hashall and braceexpand — under
	// -c, a script file and standard input alike, before the route letters
	// the semantics field deliberately leaves out.
	s.DefaultOptionLetters = "hB"
	s.ArrayScalarIsTheWholeArray = interp.No
	// A subscript inside a literal is an expression: `a=([1+1]=c)` lands at 2.
	s.ArrayLiteralSubscriptIsAKey = interp.No
	// The `@` family's letter is checked against the value rather than
	// against the spelling: `${u@QQ}` on an unset name is empty at status 0
	// and the same word on a set one is a bad substitution. Measured on
	// four spellings and on an empty array, which counts as no value.
	s.TransformLetterCheckedOnlyWhenValued = interp.Yes
	s.AssignmentUpdatesPipelineStatus = interp.Yes
	s.UnsetEndsTheProducedPipelineStatus = interp.No
	s.SelectLayout = interp.SelectMenuVerticalThenColumns
	s.SelectPromptNeedsTerminal = interp.No
	s.AliasParsesOptions = interp.Yes
	s.AliasHasPrintOption = interp.Yes
	s.AliasReportsNotFound = interp.Yes
	s.UnaliasReportsNotFound = interp.Yes
	s.AliasNotFoundStatusCounts = interp.No
	s.UnaliasAllRefusesOperands = interp.No
	s.AliasQuoting = interp.ListingQuoteAlwaysEscaped
	s.TrapQuoting = interp.ListingQuoteAlwaysEscaped
	// `declare -p` writes `declare -- v="1"` — double quotes, unlike the
	// single-quoting listings above.
	s.DeclareListing = interp.DeclareListingClustered
	s.DeclareValueQuoting = interp.ListingQuoteAlwaysDouble
	s.ExportListing = interp.DeclareListingClustered
	s.ReadonlyListing = interp.DeclareListingClustered
	s.DeclarePrintReportsAMissingName = interp.Yes
	s.TrapActionIsParsedWhenSet = interp.No
	s.TrapParseFailureNamesWhereItFired = interp.No
	s.SymbolicMaskTakesMoreThanOneOperator = interp.Yes
	s.SymbolicMaskSetsWithoutAWho = interp.Yes
	s.SymbolicMaskWhoAloneSetsIt = interp.No
	s.SymbolicMaskTakesTheSetuidLetter = interp.Yes
	s.SymbolicMaskTakesTheStickyLetter = interp.Yes
	s.ShiftReadsOptions = interp.No
	s.WaitReadsOptions = interp.Yes
	// Job specs by command text, with a second match refused as ambiguous;
	// `wait` complains about a spec that names nothing, has -n, and
	// `disown` takes the job out of the table.
	s.JobSpecsByName = interp.Yes
	s.AmbiguousJobNameIsRefused = interp.Yes
	s.WaitReportsAMissingJob = interp.Yes
	s.WaitNWaitsForTheNextJob = interp.Yes
	// A trapped signal cuts a `wait` short with 128 plus the signal, and the
	// form that names a job answers the same as the bare one.
	s.WaitForAJobFailsWhenInterrupted = interp.No
	s.DisownRemovesTheJob = interp.Yes
	s.CommandRejectsUnknownOption = interp.Yes
	s.GetoptsRejectsUnknownOption = interp.Yes
	s.ShiftCountIsArithmetic = interp.No
	s.TrapBodyRunsWhatParsed = interp.Yes
	s.ReportsAKilledCommandInACommandSubstitution = interp.No
	s.SelectAssumesUnboundedWidth = interp.No
	s.SelectEofEndsPromptLine = interp.No
	s.SelectEofIsSuccess = interp.No
	s.SelectTakesUnterminatedReply = interp.No
	s.SelectEofPrintsNewline = interp.Yes
	s.AssignmentPrefixPersistsOnSpecialBuiltin = interp.No
	// echo reads -n, -e and -E, the last of -e/-E deciding, with the hex
	// and ESC escapes on top of the XSI set.
	s.EchoOptions = "neE"
	s.EchoLastEscapeFlagWins = interp.Yes
	s.EchoExpandsHexEscapes = interp.Yes
	s.EchoExpandsEscEscape = interp.Yes
	// read takes -r and -s plus the argument letters: -a names the array in
	// the option's argument, -d a delimiter, -n and -N the two counts, -p a
	// prompt for when the input is a terminal, -t a timeout and -u a
	// descriptor. A short -n keeps its text and reports 1; a short -N keeps
	// its text too.
	s.ReadOptions = "rsa:d:n:N:p:t:u:"
	s.ReadZeroTimeout = interp.ReadZeroTimeoutPolls
	s.ReadPartialCountSucceeds = interp.No
	s.ReadExactCountKeepsPartial = interp.Yes
	s.ReadTimeoutKeepsWhatArrived = interp.Yes
	// A directory the PATH search walked past leaves no trace: with nothing
	// runnable anywhere, bash says the name was never found at all.
	s.DirectoryOnPathIsACandidate = interp.No
	// set -E and -T carry the traps set -Eeuo pipefail scripts rely on.
	s.SetHasTraceLetters = interp.Yes
	s.JobControlAbsenceIsReportedFirst = interp.Yes
	s.TildePlusMinusExpands = interp.Yes
	s.UnderscoreTracksTheLastArgument = interp.Yes
	// hash counts builtins and functions and announces its empty table.
	s.FatalErrorStatusIsOne = interp.Yes
	s.ArithNameValueRecurses = interp.Yes
	// bash has no floats, so `2**-1` has no integer answer and stops the
	// expression; the two shells with floats answer 0.5 instead.
	s.ArithNegativeExponentIsError = interp.Yes
	s.IndirectionYieldsName = interp.No
	s.BraceExpansion = interp.Yes
	// `{01..3}` is `01 02 03`; `{10..1..3}` is `10 7 4 1` and `{1..10..-3}`
	// climbs anyway — the endpoints decide the direction and a step
	// contributes magnitude alone, so `{3..1..-1}` stays `3 2 1`.
	s.BraceRangePadsToEndpointWidth = interp.Yes
	s.BraceRangeStepSignHonored = interp.No
	s.BraceRangeNegativeStepReverses = interp.No
	s.BracketCaretNegates = interp.Yes
	s.RegexQuotingMakesLiteral = interp.Yes
	s.ShiftPastEndFatal = interp.No
	s.DeclaredNameWithoutValueIsEmpty = interp.No
	// `local u` hides the caller's `u` — the local exists unset.
	s.ValuelessDeclarationHidesTheOuterValue = interp.Yes
	s.TypesetLocalNeedsKeywordFunction = interp.No
	s.ReadonlyReassignmentFatal = interp.No
	s.ReadonlyReassignmentFatalFromCommandString = interp.Yes
	s.ReadonlyReassignmentByDeclarationFatal = interp.No
	s.BuiltinSyntaxErrorFatal = interp.No
	s.DotMissingFileFatal = interp.No
	s.DotPassesArguments = interp.Yes
	// The only shell in the panel that looks in the current directory once
	// PATH has missed. Measured: `PATH=/usr/bin:/bin; . f.sh` finds an f.sh in
	// the current directory here, and is "not found" in the other three.
	s.DotFallsBackToCurrentDirectory = interp.Yes
	s.ExecFailureRunsExitTrap = interp.Yes
	s.ExecTakesOptions = interp.Yes
	s.TestAcceptsDoubleEqual = interp.Yes
	// `f -nt missing` holds when f exists, in `test` and `[[ ]]` alike.
	s.MissingFileIsOlder = interp.Yes
	s.UmaskPrintsFourDigits = interp.Yes
	s.UmaskSetWithSPrints = interp.Yes
	s.PipefailOption = interp.Yes
	s.ErrexitSeesPipefailFailure = interp.Yes
	s.UnterminatedBracket = interp.BracketLiteral
	s.ExitArgument = interp.ExitArgNumeric
	s.TraceAssignmentsSeparately = interp.Yes
	// bash reports success if it signaled anything at all, where the others
	// count failures one way or another.
	s.ExitTrapRunsOnSignalDeath = interp.Yes
	// 5.3 ignores an untrapped QUIT when it is not interactive, where 3.2
	// dies by it — a divergence between two builds of the same shell, and
	// this preset is 5.3.
	s.QuitIgnoredWhenNotInteractive = interp.Yes
	s.HangupIsAnOrderlyExit = interp.No
	s.ExitInTrapReportsEarlierStatus = interp.Yes
	s.KillListAcceptsName = interp.Yes
	s.SIGPrefixAccepted = interp.Yes
	s.RedirectsWriteToEveryTarget = interp.No
	s.KillStatus = interp.KillStatusAnySuccess
	s.PrintfEmptyIsNotANumber = interp.Yes
	s.PrintfReportsBadNumber = interp.Yes
	s.PrintfBackslashC = interp.PrintfBackslashCLiteral
	// `%(fmt)T`: an epoch through a date format, with -1 for now and -2 for
	// when the shell started. This shell alone in the panel — 3.2 has it not
	// either, which is why the two bash columns of the corpus differ here.
	s.PrintfTimeConversion = interp.Yes
	s.PrintfQuote = interp.PrintfQuoteBackslash
	// `$'\cA'` is 0x01 and `$'\c1'` is 0x11: the character uppercased and
	// masked to five bits, with `\c?` reading as DEL since 5.x.
	s.DollarSingleBackslashC = interp.DollarSingleControlMasked
	s.DollarSingleUnknownEscape = interp.DollarSingleUnknownKeepsBackslash
	s.DollarSingleNulTruncates = interp.Yes
	s.GetoptsAssignmentRestartsWord = interp.Yes
	s.GetoptsClearsOptarg = interp.No
	s.CdWithoutHomeIsAnError = interp.Yes
	s.CdDashPrintsTheDirectory = interp.Yes
	s.PrintfAssignsWithV = interp.Yes
	s.PrintfRejectsUnknownOption = interp.Yes
	s.TrapParsesOptions = interp.Yes
	s.TrapPrintsWithP = interp.Yes
	s.TrapPrintsBareWithConditions = interp.No
	s.TrapPrintsBareWithP = interp.Yes
	s.TrapListsSignalsWithL = interp.Yes
	s.TrapOneArgumentIsACondition = interp.Yes
	s.TrapReportsAnUnknownSingleCondition = interp.Yes
	s.TrapSingleUnknownConditionIsUsage = interp.Yes
	// All three pseudo-conditions, RETURN being this shell's alone. None of
	// them follows the script into a function it was not set in, or into a
	// subshell — measured: `trap 'echo E' ERR; f(){ false; :; }; f` prints
	// nothing, and a command substitution captures no handler output.
	s.TrapHasErrCondition = interp.Yes
	s.TrapHasDebugCondition = interp.Yes
	s.TrapHasReturnCondition = interp.Yes
	s.ErrTrapRunsInsideFunctions = interp.No
	s.ErrTrapRunsInSubshells = interp.No
	s.DebugTrapRunsInsideCalls = interp.No
	s.DebugTrapRunsInSubshells = interp.No
	// The shell that keeps the parent's trap listing across every boundary
	// but a process substitution — `(trap)`, `$(trap)`, `trap | cat` and
	// `trap &` all print what the parent had, EXIT trap included, though a
	// handled signal no longer fires there. Measured: `trap 'echo x' USR1;
	// (kill -USR1 $BASHPID; echo alive)` dies of the default action while
	// `(trap)` still lists the trap.
	s.SubshellKeepsTrapListing = interp.Yes
	s.PipelineElementKeepsTrapListing = interp.Yes
	s.BackgroundJobKeepsTrapListing = interp.Yes
	s.KeptTrapListingIncludesExit = interp.Yes
	s.UlimitBlockIsKilobyte = interp.Yes
	s.UlimitHasResidentSet = interp.Yes
	s.UlimitHasProcessCount = interp.Yes
	s.UlimitSetsBothLimits = interp.Yes
	s.BadOptionToSpecialBuiltinFatal = interp.No
	s.LocalOutsideAFunctionIsAnError = interp.Yes
	s.LocalOutsideAFunctionIsFatal = interp.No
	// bash reports every operand that is not a name, exports the ones that
	// are, and carries on with a status of 1.
	s.BadNameToDeclarationFatal = interp.No
	s.BadNameToUnsetFatal = interp.No
	s.DeclarationNameOperands = interp.PlainNamesOnly
	// bash 5.3's bare `unset` checks nothing — `unset 1x`, `unset "a b"` and
	// `unset -- -` are all quiet — while `unset -v 1x` refuses. bash 3.2
	// refused all of them, so the panel's two bash columns differ here.
	s.UnsetNameOperands = interp.AnythingIsAName
	s.DeclarationTakesASubscript = interp.No
	s.UnsetTakesASubscript = interp.Yes
	// `unset a[@]` empties the array: `a=(x y z)` comes back with no elements
	// and `${#a[@]}` is 0. Measured in both bash builds, and the same for
	// `a[*]`. A name that holds a scalar is refused rather than emptied, and
	// one that holds nothing at all is quietly left alone.
	s.UnsetArrayAt = interp.UnsetArrayAtRemovesEveryElement
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.No
	s.JobsListFinishedJobs = interp.Yes

	// `jobs`' letters. bash has the widest set in the panel: POSIX's `-l`
	// and `-p`, the state filters `-r` and `-s`, `-n` for what has changed
	// since it last said, and `-x` — which is not a listing at all but a
	// command to run with its job specs replaced by process ids. The last
	// two ride UnimplementedOptionLetters and are refused by name.
	s.JobsOptions = "lprs"
	// `jobs -p` here is the process ids and nothing else, which is what
	// makes `kill $(jobs -p)` mean what it is written to mean.
	s.JobsPidsOnlyOption = interp.Yes
	// With both filters at once the last letter given decides, so
	// `jobs -rs` lists the stopped jobs and `jobs -sr` the running ones.
	s.JobsStateFiltersAccumulate = interp.No

	// Whether a `&` job's command appears in a `jobs` listing.
	s.JobsShowBackgroundCommand = interp.Yes

	// Whether a backgrounded job is announced to whoever is typing.
	// Whether `export -f` carries a function to a child.
	// Whether an unassigned subscript is an element.
	s.ArraysAreSparse = interp.Yes
	// An operator on `${a[*]}` trims each element before the join here.
	s.OperatorDistributesOverStarSubscript = interp.Yes
	s.ExportCarriesFunctions = interp.Yes
	s.AnnouncesBackgroundJob = interp.Yes
	s.ReportsACommandKilledBySignal = interp.Yes
	s.ReportsAnyKilledPipelineElement = interp.No
	s.ChildInterruptEndsTheScript = interp.No
	s.CdRefusesUnknownOption = interp.Yes
	s.CdLastPathOptionWins = interp.Yes
	s.BadSetOptionNameFatal = interp.No
	s.ReturnOutsideAFunctionIsRefused = interp.Yes
	s.LoneDashIsAnOption = interp.No
	s.UnsetFunctionChecksTheName = interp.No
	s.UnsetFunctionReportsMissing = interp.No

	// Whether a redirection target is expanded as an ordinary word.
	s.RedirectTargetIsAnOrdinaryWord = interp.Yes

	// Whether `type --` ends the options.
	s.TypePrintsFunctionBody = interp.Yes
	s.TypeEndsOptionsWithDashDash = interp.Yes
	// The one shell in the panel with `-t` at all: one bare word per name,
	// and silence with status 1 for a name that is nothing.
	s.TypeNamesTheKindWithDashT = interp.Yes
	// The rest of type's letters, all implemented here: -a for every
	// resolution, -p speaking only where the plain answer would have been a
	// file, -P forcing the PATH search, -f leaving functions out.
	s.TypeOptions = "afpPt"
	s.TypePSearchesPathPastTheShell = interp.No
	s.TypePathAnswerIsASentence = interp.No
	s.TypeFSaysTheFunctionBack = interp.No

	// The letters `declare` and `local` read — one set under two names
	// here, with `-F` naming functions rather than setting a float's
	// precision, which no other engine spells this way. The nameref and
	// trace letters this shell also has ride in
	// Diagnostics.UnimplementedOptionLetters.
	s.DeclareOptions = "aAfFgilprux"
	s.LocalOptions = "aAgilprux"
	// A bad `declare` option is reported and the script goes on.
	s.TypesetBadOptionFatal = interp.No
	// `declare -g x=new` writes the global cell even with a `local x`
	// standing in front of the name.
	s.DeclareGlobalReachesPastALocal = interp.Yes
	// A bare `local` writes the running function's own locals, each as a
	// clustered declaration — `declare -i n`, `declare -- x`.
	s.BareLocalListing = interp.BareLocalListsLocals
	// A bare `set` writes the variables and then every defined function,
	// values quoted only where they must be, `'\''` for an embedded quote
	// and `$'...'` once a control character appears.
	s.SetListing = interp.SetListingAssignmentsThenFunctions
	s.SetListingQuoting = interp.ListingQuoteWhenNeededEscaped

	return s
}

// Diagnostics is how bash 5 reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		TypeKeyword:  "%[1]s is a shell keyword",
		TypeFunction: "%[1]s is a function",
		TypeNotFound: "type: %[1]s: not found",
		// The same complaint from `command -V`, blaming `command`.
		CommandVNotFound: "command: %[1]s: not found",
		// The target as it was written, not as it expanded.
		AmbiguousRedirect: "%[1]s: ambiguous redirect",
		JobStarted:        "[%[1]d] %[2]d",
		// Measured from a terminal: `[1]+` then two spaces, the state in a
		// 27-wide column, then the command — with the `&` back on it while
		// the job runs and gone once it has ended.
		ExpansionFailureStatusFromCommandString: 127,
		// The word, not the `${…}` inside it: `echo "[${x@QQ}]"` is refused
		// as `[${x@QQ}]: bad substitution`. The run of the word that shares
		// the expansion's quoting, and with the quotes off — `echo
		// 'lit'"${x@QQ}"` names `${x@QQ}` alone, where `echo
		// "pre${x@QQ}post"` names all of `pre${x@QQ}post`.
		BadSubstitution:      "%[1]s: bad substitution",
		BadSubstitutionNames: interp.NamesTheQuotingRun,
		ParamNullOrNotSet:    "parameter null or not set",
		// The letter as the script spelled it, sign and all: `set +q` is
		// refused as `+q` here where dash and zsh write `-q` either way.
		// The refusal is this shell's ordinary bad-option complaint, so
		// `set`'s usage line follows it — and does not follow the different
		// sentence a bad `-o` name earns, which is why
		// SetInvalidOptionNameUsage is not set.
		SetInvalidOptionLetter: "set: %[1]s: invalid option",
		// bash refuses a `set` option letter from its own command-line
		// parser: its name, the sentence, and the whole shell usage block
		// under it, with no location and no second name. The long spelling
		// goes to the builtin instead and reads as one — see
		// InvocationNameRefusalNamesTheShell.
		InvocationUsage: "Usage:\t%[1]s [GNU long option] [option] ...\n" +
			"\t%[1]s [GNU long option] [option] script-file ...\n" +
			"GNU long options:\n" +
			"\t--debug\n\t--debugger\n\t--dump-po-strings\n\t--dump-strings\n" +
			"\t--help\n\t--init-file\n\t--login\n\t--noediting\n\t--noprofile\n" +
			"\t--norc\n\t--posix\n\t--pretty-print\n\t--rcfile\n\t--restricted\n" +
			"\t--verbose\n\t--version\n" +
			"Shell options:\n" +
			"\t-ilrsD or -c command or -O shopt_option\t\t(invocation only)\n" +
			"\t-abefhkmnptuvxBCEHPT or -o option",
		InvocationNameRefusalNamesTheShell: true,
		JobLine:                            "[%[1]d]%[2]s  %-27[3]s%[4]s",
		// `jobs -l`: the same 27-wide state column, with the process id
		// spending one of the two spaces after the marker rather than
		// pushing the rest of the row along.
		JobLineLong: "[%[1]d]%[2]s %[3]d %-27[4]s%[5]s",
		// The same twenty-seven-column state field the listing above uses,
		// with the process id in front of it rather than the job number: one
		// formatter, said in two places.
		KilledCommandNotice: "%5[1]d %-27[2]s%[3]s",
		// SIGTERM alone, and 5.3 alone: no location and no process id.
		KilledCommandNoticeBareForTerminate: "%-27[1]s%[2]s",
		JobRunning:                          "Running",
		JobStopped:                          "Stopped",
		JobDone:                             "Done",
		JobExited:                           "Exit %[1]d",
		JobRunningShowsAmpersand:            true,
		// No verb at all: the name, then the OS string. Same either way —
		// bash does not distinguish opening from creating.
		CannotOpen: "%[1]s: %[2]s",
		// A builtin whose write went nowhere: `echo hi >&-` is
		// `echo: write error: Bad file descriptor`, and the same shape with
		// printf, pwd or type in front — the builtin is a verb, not a set of
		// messages.
		BuiltinWriteError: "%[1]s: write error: %[2]s",
		// Measured: bash reports a failed open at the redirect's own line.
		RedirectFailureLine:     interp.LineOfRedirect,
		CannotCreate:            "%[1]s: %[2]s",
		NoclobberRefusal:        "%[1]s: cannot overwrite existing file",
		NamesTheInputInLocation: true,
		EchoesTheOffendingLine:  true,
		// A script operand it could not read, worded the same way as `.` and
		// as a redirection: the path, then the operating system's own text.
		// The two numbers are the measurement — 127 for a path that names
		// nothing, the number a missing command carries, and 126 for one that
		// is there and will not open, the number an unrunnable one carries.
		// Measured on a missing path, a missing parent, a dangling symlink, a
		// mode-000 file and a directory.
		ScriptNotFound:          "%[1]s: %[2]s",
		ScriptNotFoundStatus:    127,
		ScriptNotReadableStatus: 126,
		SelectPrompt:            "#? ",
		Location:                interp.LocationLineWord,
		NotFound:                "%s: command not found",
		UnboundVariable:         "%s: unbound variable",
		UnboundPositional:       "$%s: unbound variable",
		NumericArgument:         "%[1]s: %[2]s: numeric argument required",
		// `unset a[@]` where `a` holds a scalar. Identical in bash 3.2.
		UnsetNotAnArray:       "unset: %[1]s: not an array variable",
		ArithError:            `%[1]s: %[2]s (error token is "%[3]s")`,
		DivisionByZero:        "division by 0",
		ArithNegativeExponent: "exponent less than 0",

		// bash reserves its generic arithmetic wording for operands that are
		// not literals, so a bad digit gets a reason of its own.
		DigitTooGreatForBase:     "value too great for base",
		ArithErrorNamesThePrefix: true,
		// set -o pads to fifteen and tabs; kill -l numbers five to a row.
		OptionListingWidth:  15,
		OptionListingTabbed: true,
		KillListing:         interp.KillListingNumbered,
		TraceQuoting:        interp.QuoteShell,
		TraceForHeader:      interp.TraceForSource,
		// bash names the construct and the line it opened on, and nothing
		// about what would have closed it.
		EvalNaming:       interp.SourceBeforeLocation,
		SourceFileNaming: interp.SourceReplacesShell,
		// Runtime diagnostics move with the source too: a failure inside a
		// sourced file, or inside a function defined in one, names that file
		// as written — `./inc.sh: line 1: nosuch: command not found` — where
		// dash and ksh93 keep the script's own name.
		LocationNamesTheCurrentFile: true,
		UnterminatedEndsOnNextLine:  true,
		ArithOperandExpected:        "arithmetic syntax error: operand expected",
		ArithOperatorExpected:       "arithmetic syntax error in expression",
		ArithBadOperator:            "arithmetic syntax error: invalid arithmetic operator",
		ArithFailureStatus:          1,
		SyntaxUnexpected:            "syntax error near unexpected token `%[1]s'",
		// A parse failure by every other measure, and 1 rather than this
		// dialect's syntax-error status.
		ForNameStatus: 1,
		ForName:       "`%[1]s': not a valid identifier",
		Unterminated:  "syntax error: unexpected end of file from `%[1]s' command on line %[2]d",
		// With nothing open to name — `f()` with no body — the sentence
		// stops after the diagnosis rather than naming an empty construct.
		UnterminatedNoConstruct: "syntax error: unexpected end of file",
		// One sentence for every unmatched delimiter, always naming the
		// closer — only the line it lands on differs by construct.
		UnmatchedQuote:            "unexpected EOF while looking for matching `%[2]s'",
		UnmatchedCmdSubst:         "unexpected EOF while looking for matching `%[2]s'",
		UnmatchedBraceSubst:       "unexpected EOF while looking for matching `%[2]s'",
		UnmatchedReportedAtOpener: true,
		CmdSubstUnmatchedAtEnd:    true,
		SyntaxErrorStatus:         2,
		// Measured: `.` of a file it cannot open reports 1 and carries on,
		// where a missing operand is 2 — two numbers for what reads like one
		// failure, which is why they are two fields.
		DotCannotOpen:       "%[1]s: %[2]s",
		DotCannotOpenStatus: 1,
		// bash names neither the builtin nor the operation: just the command
		// and the reason, the same shape it uses for `.`.
		CannotExecute: "%[1]s: %[2]s",
		// bash names the builtin only when the command was not found at all.
		ExecNotFound: "exec: %[1]s: not found",
		// bash is the only one that says which *kind* of operator it wanted.
		TestUnaryExpected:    "%[2]s: %[1]s: unary operator expected",
		TestBinaryExpected:   "%[2]s: %[1]s: binary operator expected",
		TestIntegerExpected:  "%[2]s: %[1]s: integer expected",
		TestTooManyArguments: "%[2]s: too many arguments",
		TestOperandExpected:  "%[2]s: argument expected",
		TestMissingBracket:   "[: missing `]'",
		// `kill` puts the process in parentheses and the reason after a dash,
		// which is the only wording in the panel a script could not confuse
		// with a message about a signal name.
		GetoptsBadOption:       "illegal option -- %[1]s",
		GetoptsMissingArgument: "option requires an argument -- %[1]s",
		// bash names itself and no line here, where it gives a line to
		// everything else it says.
		GetoptsNamesNoLine:          true,
		CdCannotChange:              "cd: %[1]s: %[2]s",
		CdHomeNotSet:                "cd: HOME not set",
		CdOldpwdNotSet:              "cd: OLDPWD not set",
		PrintfBadNumber:             "printf: %[1]s: invalid number",
		PrintfBadVerb:               "printf: `%[1]s': invalid format character",
		PrintfBadOption:             "printf: %[1]s: invalid option",
		TrapBarePrintNeedsCondition: "trap: -P requires at least one signal name",
		PrintfBadOptionShowsUsage:   true,
		LetNoExpression:             "let: expression expected",
		UlimitBadOption:             "ulimit: -%[1]s: invalid option",
		// `ulimit -a`, row for row as the engine writes it. The pipe row is
		// not a resource limit and never moves; its value is this machine
		// family's constant.
		UlimitListing: []interp.UlimitListingRow{
			{Prefix: "core file size              (blocks, -c) ", Res: interp.ResourceCore},
			{Prefix: "data seg size               (kbytes, -d) ", Res: interp.ResourceData, Scale: 1024},
			{Prefix: "file size                   (blocks, -f) ", Res: interp.ResourceFileSize},
			{Prefix: "max locked memory           (kbytes, -l) ", Res: interp.ResourceLockedMemory, Scale: 1024},
			{Prefix: "max memory size             (kbytes, -m) ", Res: interp.ResourceResidentSet, Scale: 1024},
			{Prefix: "open files                          (-n) ", Res: interp.ResourceOpenFiles, Scale: 1},
			{Prefix: "pipe size                (512 bytes, -p) ", Fixed: "1"},
			{Prefix: "stack size                  (kbytes, -s) ", Res: interp.ResourceStack, Scale: 1024},
			{Prefix: "cpu time                   (seconds, -t) ", Res: interp.ResourceCPUTime, Scale: 1},
			{Prefix: "max user processes                  (-u) ", Res: interp.ResourceProcesses, Scale: 1},
			{Prefix: "virtual memory              (kbytes, -v) ", Res: interp.ResourceAddressSpace, Scale: 1024},
		},
		UlimitBadNumber:    "ulimit: %[1]s: invalid number",
		BuiltinBadOption:   "%[1]s: %[2]s: invalid option",
		WaitBadJob:         "wait: `%[1]s': not a pid or valid job spec",
		WaitNoSuchJob:      "wait: %[1]s: no such job",
		AmbiguousJobSpec:   "%[1]s: %[2]s: ambiguous job spec",
		KillNoSuchJob:      "kill: %[1]s: no such job",
		DisownNoCurrentJob: "disown: current: no such job",
		WaitBadJobStatus:   1,
		WaitNotOurChild:    "wait: pid %[1]d is not a child of this shell",
		UnimplementedOptionLetters: map[string]string{
			// `set` letters bash has and this shell does not: -b job
			// notices, -k assignment-anywhere, -p privileged, -t one
			// command, -B brace expansion, -H history expansion, -P
			// physical paths. Measured 2026-09-05 by asking bash 5.3 for
			// every letter of the alphabet in both cases and both signs;
			// the ones missing from here it refuses itself, and those get
			// SetInvalidOptionLetter.
			"set": "bkprtBHP",
			// Options these builtins have here and this shell does not.
			"wait": "fp",
			// disown's sweepers: -a for every job, -h for HUP shielding
			// alone, -r for the running ones.
			"disown": "ahr",
			// `jobs -n` lists what has changed since the shell last said,
			// which needs a record of what it has already reported — and
			// the two shells with the letter do not agree on what counts:
			// a job that has only just started is a change here and is not
			// one in ksh93. `jobs -x` is not a listing at all: it runs a
			// command with the job specs in its arguments replaced by
			// process ids.
			"jobs": "nx",
			// What is left of read's letters: readline editing and the text
			// -i seeds it with — about a line editor this runner does not
			// hold. The -p prompt is implemented: parsed always, printed
			// only to a terminal, which is the measured whole of it.
			"read": "Eei",
			// The nameref and trace attributes, under both of the builtin's
			// names — and `local`'s extras: the same two, `-I` inheritance,
			// and the function letters, which this shell takes and ignores
			// where no operand is a function.
			"declare": "Int",
			"typeset": "Int",
			"local":   "fFInt",
			// The callbacks: -C runs a command every -c elements, which is
			// about progress display and is deferred rather than parsed and
			// ignored — under either of the command's two names.
			"mapfile":   "Cc",
			"readarray": "Cc",
		},
		// bash's own words for the two -u failures it can meet here; the
		// non-number wordings per letter are not modeled yet, so those fall
		// back to the substrate's.
		ReadBadFileDescriptor: "read: %[1]s: invalid file descriptor: Bad file descriptor",
		// 128 plus SIGALRM, the signal a timeout is.
		ReadTimeoutStatus: 142,
		HereDocumentAtEOF: "warning: here-document at line %[1]d " +
			"delimited by end-of-file (wanted `%[2]s')",
		// bash names the builtin for its own two spellings and not for the
		// two POSIX has: `declare: r: readonly variable` against a plain
		// `r: readonly variable` from `export`.
		ReadonlyVariableInDeclaration: "%[2]s: %[1]s: readonly variable",
		ReadonlyRefusalNamesBuiltin:   map[string]bool{"declare": true, "typeset": true},
		// `declare -p nosuch` — the name it was invoked by is in front,
		// which declarePrint writes, so the wording carries only the rest.
		DeclareNoSuchVariable:  "%[1]s: not found",
		TrapPrintsSignalPrefix: "SIG",
		// One wording for all three, and the operand quoted back exactly as
		// given: `export 1x=v` says `1x=v', not `1x'.
		BuiltinBadName: map[string]string{
			"export":   "%[1]s: `%[2]s': not a valid identifier",
			"readonly": "%[1]s: `%[2]s': not a valid identifier",
			"unset":    "%[1]s: `%[2]s': not a valid identifier",
			"local":    "%[1]s: `%[2]s': not a valid identifier",
		},
		BuiltinBadNameKeepsValue: true,
		BuiltinUsageUnprefixed:   true,
		BuiltinUsage: map[string]string{
			"set":    "set: usage: set [-abefhkmnptuvxBCEHPT] [-o option-name] [--] [-] [arg ...]",
			"export": "export: usage: export [-fn] [name[=value] ...] or export -p [-f]",
			// The refusal lines a bad option earns from these two, measured
			// with a letter nobody has (-q).
			"command": "command: usage: command [-pVv] command [arg ...]",
			"getopts": "getopts: usage: getopts optstring name [arg ...]",
			// Not one line under two names: `typeset` loses the brackets
			// around its name operand where `declare` keeps them. As
			// written, both.
			"declare": "declare: usage: declare [-aAfFgiIlnrtux] [name[=value] ...] or declare -p [-aAfFilnrtux] [name ...]",
			"typeset": "typeset: usage: typeset [-aAfFgiIlnrtux] name[=value] ... or typeset -p [-aAfFilnrtux] [name ...]",
			"local":   "local: usage: local [option] name[=value] ...",
			"read": "read: usage: read [-Eers] [-a array] [-d delim] [-i text] " +
				"[-n nchars] [-N nchars] [-p prompt] [-t timeout] [-u fd] [name ...]",
			"mapfile": "mapfile: usage: mapfile [-d delim] [-n count] [-O origin] [-s count] " +
				"[-t] [-u fd] [-C callback] [-c quantum] [array]",
			"readarray": "readarray: usage: readarray [-d delim] [-n count] [-O origin] [-s count] " +
				"[-t] [-u fd] [-C callback] [-c quantum] [array]",
			"readonly": "readonly: usage: readonly [-aAf] [name[=value] ...] or readonly -p",
			"trap":     "trap: usage: trap [-Plp] [[action] signal_spec ...]",
			"type":     "type: usage: type [-afptP] name [name ...]",
			"jobs":     "jobs: usage: jobs [-lnprs] [jobspec ...] or jobs -x command [args]",
			"wait":     "wait: usage: wait [-fn] [-p var] [id ...]",
			"unset":    "unset: usage: unset [-f] [-v] [-n] [name ...]",
		},
		PrintfUsage:            "printf: usage: printf [-v var] format [arguments]",
		UmaskBadMask:           "umask: %[1]s: octal number out of range",
		AliasNotFound:          "%[1]s: %[2]s: not found",
		UnaliasNotFound:        "%[1]s: %[2]s: not found",
		AliasListPrefix:        "alias ",
		UnaliasUsage:           "unalias: usage: unalias [-a] name [name ...]",
		UnaliasUsageUnprefixed: true,
		// bash names the character and says which kind it wanted.
		UmaskBadSymbolicMode:     "umask: `%[2]s': invalid symbolic mode character",
		UmaskBadSymbolicOperator: "umask: `%[2]s': invalid symbolic mode operator",
		UmaskBadOption:           "umask: %[1]s: invalid option",
		UmaskUsage:               "umask: usage: umask [-p] [-S] [mode]",
		UmaskUsageUnprefixed:     true,
		// The usage carries no location, as `kill`'s does not.
		PrintfUsageUnprefixed:     true,
		TrapBadSignal:             "trap: %[1]s: invalid signal specification",
		KillNoSuchProcess:         "kill: (%[1]s) - No such process",
		KillNotPermitted:          "kill: (%[1]s) - Operation not permitted",
		KillInvalidSignal:         "kill: %[1]s: invalid signal specification",
		KillIllegalOption:         "kill: %[1]s: invalid signal specification",
		KillNotAPid:               "kill: `%[1]s': not a pid or valid job spec",
		KillMissingSignalArgument: "kill: %[1]s: option requires an argument",
		KillUsage: "kill: usage: kill [-s sigspec | -n signum | -sigspec] pid | jobspec ... " +
			"or kill -l [sigspec]",
		// The usage is the one bash diagnostic with no location in front of
		// it — every other message here carries "bash: line N:".
		KillUsageUnprefixed: true,
		KillUsageStatus:     2,
		TimesDecimals:       3,
		// The `time` keyword's report: three decimals where ksh93 gives
		// the same lines two.
		TimeDecimals: 3,
		// A path that is not there is the OS reason and does not name the
		// builtin; a bare name off PATH does the reverse.
		PathNotFound: "%[1]s: No such file or directory",
		// bash names the path it tried, absolute, where the other three
		// report the operand as written.
		NamesResolvedPath: true,
		// A bare `hash` announces the table, on standard output.
		HashEmptyTable: "hash: hash table empty",
		// Two lines, which is bash rather than a mistake: it prints the
		// complaint and then a usage line, and only the first carries the
		// shell's own prefix.
		DotNoOperand: ".: filename argument required\n" +
			".: usage: . [-p path] filename [arguments]",
		DotNoOperandStatus: 2,
	}
}

// Apply adds what bash has and the substrate does not.
//
// `source` is a synonym for `.`, and is a dialect's answer rather than the
// core's because dash does not have it at all — `command -v source` reports
// "not found" there. It is the same function under a second name rather than a
// second implementation, which is the only way the two cannot drift apart.
func Apply(r *interp.Runner) {
	// The `set -o` names this shell has and the others do not all have,
	// measured by asking each of the four to turn every name off. This one
	// has the most, and five of them belong to it alone.
	r.AddSetOptions(
		"braceexpand",
		"errtrace",
		"functrace",
		"hashall",
		"histexpand",
		"history",
		"interactive-comments",
		"keyword",
		"onecmd",
		"physical",
		"pipefail",
		"posix",
		"privileged",
	)
	// The statuses of the last pipeline's elements. The core keeps the
	// record and this names it; ksh93 and dash have no name for it at all.
	r.SetPipelineStatus("PIPESTATUS")
	// What the last `=~` captured — the whole match, then the groups. The
	// core keeps the record and this names it; ksh93 and zsh keep their
	// captures under names and shapes of their own, never this one.
	r.SetRegexMatch("BASH_REMATCH")
	// Parameters bash provides and the others do not all have. Which
	// variables a shell supplies is the same kind of question as which
	// builtins it has, so it is answered here rather than as an axis.
	//
	// A read of the process, in library code, on purpose. The purity rule
	// covers dialect/ as well as interp/ — an embedder links this and Apply
	// runs inside the Runner — but it is about state a Runner owns and two
	// Runners could disagree about. A uid is neither: nothing a script does
	// changes it, two shells in one program genuinely have the same one, and
	// $UID has no other source. Same class as $$, which .golangci.yml has
	// blessed since it was written.
	r.SetSpecial("UID", strconv.Itoa(os.Getuid()))
	r.SetSpecial("EUID", strconv.Itoa(os.Geteuid()))
	// Where the script is, which is a stack rather than a value — see
	// callstack.go for why it cannot be stored.
	registerCallStack(r)
	// The same stack, one step up: see caller.go.
	registerCaller(r)
	// A function carried to a child through the environment, under the name
	// bash gives it. The other three do not carry functions at all.
	r.SetFunctionExport("BASH_FUNC_", "%%")
	// And how they are laid out, which is this shell's taste rather than
	// anything the printer should know.
	r.SetFunctionLayout(FunctionLayout(), ExportedFunctionLayout())
	r.SetDynamic("RANDOM", func(*interp.Runner) string { return interp.Randoms() })
	r.SetDynamic("SECONDS", func(rr *interp.Runner) string {
		return strconv.Itoa(int(rr.SecondsFrom()))
	})
	if dot, ok := r.Builtin("."); ok {
		r.Register("source", dot)
	}
	// The builtin this shell alone answers to; the other three say "command
	// not found", so it is registered here rather than taken away there.
	r.Register("shopt", biShopt)
	// `declare` is `typeset` under a second name rather than a second
	// implementation. ksh93 has only the older name and dash has neither, so
	// which names exist is a dialect's answer and not an axis.
	if typeset, ok := r.Builtin("typeset"); ok {
		r.Register("declare", typeset)
		// And the assignment rule follows the name: `declare x=*` stores the
		// character here, where in a shell without the name it would be an
		// ordinary command with an ordinary globbed argument.
		r.SetDeclaring("declare")
	}
}
