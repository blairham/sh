// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package zsh answers the substrate's questions the way zsh does.
package zsh

import (
	"os"
	"strconv"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what zsh parses.
func Dialect() syntax.Dialect {
	d := syntax.Core()
	// zsh does not expand under `-c` even with the option set.
	// Measured 2026-09-05: `zsh -c 'alias hi=...; hi'` does not expand and
	// the same two lines in a file, or on standard input, do. The route is
	// the whole of the difference — nothing about the shell changes.
	d.ExpandAliases = syntax.AliasFromScriptFile | syntax.AliasOnStandardInput
	// And a body's newlines are lines of the program, as they are in the two
	// that expand by every route.
	d.AliasBodyCountsLines = true
	// zsh has all five, like bash.
	d.DeclarationUtilities = map[string]bool{
		"declare": true, "typeset": true, "local": true,
		"export": true, "readonly": true,
	}
	d.FunctionKeywordParens = true
	// `()` is one token to this lexer, which shows in the one place a
	// diagnostic names the last token it read: `f()` with the input running
	// out reports as parse error near `()' rather than naming the closing
	// paren on its own.
	d.EmptyParensAreOneToken = true
	// `}` is reserved wherever a word may stand here, which is what lets a
	// brace group close without a terminator — and what makes `echo }` a
	// syntax error rather than a brace on the output.
	d.CloseBraceAlwaysReserved = true
	// Floating point, which POSIX has not and these two do.
	d.ArithFloat = true
	// A bare `(a|b)` inside a pattern word, which makes `@(abc|xyz)` a
	// literal `@` followed by a group here rather than an extended pattern.
	d.PatternAlternation = true
	// The parenthesized flag group an expansion may open with — `${(U)x}`,
	// `${(%):-%x}` — which is this dialect's alone: the other three call
	// the whole expansion a bad substitution.
	d.ParamExpansionFlags = true
	return d
}

// Semantics is what zsh means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	s.CommandNotFoundStatusIsNotFound = interp.No
	s.SetFTurnsOffGlobbing = interp.No
	// `-c` and `-s` together: `-s` names the operands here, so `sh -sc CMD
	// name a` keeps the shell in `$0` and makes both operands parameters.
	// bash and dash let the command string name them instead.
	s.StdinOptionNamesTheOperands = interp.Yes
	// The reading side of the same option: zsh's short spelling of noglob
	// is `-F`, and that capital is what its `$-` reports.
	s.NoglobLetterIsF = interp.No
	// `set -h` is histignoredups here — a history option — not the command
	// tracking the letter abbreviates in bash and ksh93.
	s.SetHLetterTracksCommands = interp.No
	// Job control wants the terminal: with none, `set -m` is refused —
	// `can't change option: -m`, at 1, fatally like every `set` failure
	// here. Measured; bash and ksh93 grant the same request silently.
	s.MonitorNeedsATerminal = interp.Yes
	// Measured: `echo $-` reports `569X` under -c, a script file and
	// standard input alike — letters from zsh's own single-letter option
	// namespace, which shares almost nothing with the other shells'.
	s.DefaultOptionLetters = "569X"
	// The panel's holdout: `echo hi >&-` is status 0 here and 1 in the other
	// three — the text is quietly lost and nothing is said about a simple
	// command's own closed stream. What zsh prints when the stream was
	// closed by `exec >&-` instead — a `write error` with the status still 0
	// — is measured in docs/spec/semantics.md and not reproduced.
	s.BuiltinWriteErrorFailsTheCommand = interp.No
	s.ArithIntegerOperatorRefusesFloat = interp.No
	// A negative exponent is a float answer here, not a refusal: `2**-1`
	// is 0.5.
	s.ArithNegativeExponentIsError = interp.No
	s.ArrayScalarIsTheWholeArray = interp.Yes
	s.AssignmentUpdatesPipelineStatus = interp.No
	s.UnsetEndsTheProducedPipelineStatus = interp.Yes
	s.SelectLayout = interp.SelectMenuColumns
	s.SelectPromptNeedsTerminal = interp.No
	s.AliasParsesOptions = interp.Yes
	s.AliasHasPrintOption = interp.No
	// The reverse of ksh93: silent about `alias nope` and not about
	// `unalias nope`.
	s.AliasReportsNotFound = interp.No
	s.UnaliasReportsNotFound = interp.Yes
	s.AliasNotFoundStatusCounts = interp.No
	s.UnaliasAllRefusesOperands = interp.Yes
	s.AliasQuoting = interp.ListingQuoteWhenNeededEscaped
	s.TrapQuoting = interp.ListingQuoteWhenNeededPlain
	// `typeset -p` writes `typeset v=1`, an exported scalar as `export e=E`
	// — values in the alias style, keys in the trap one.
	s.DeclareListing = interp.DeclareListingExportSpelled
	s.DeclareValueQuoting = interp.ListingQuoteWhenNeededEscaped
	s.ExportListing = interp.DeclareListingCommandWord
	// readonly -p speaks typeset here, not readonly.
	s.ReadonlyListing = interp.DeclareListingExportSpelled
	s.DeclarePrintReportsAMissingName = interp.Yes
	s.TrapBodyLine = interp.TrapBodyLineWhereItFired
	s.TrapActionIsParsedWhenSet = interp.Yes
	// The strict end of the symbolic mask: one operator per clause, a who
	// before `=`, and neither `s` nor `t`.
	s.SymbolicMaskTakesMoreThanOneOperator = interp.No
	s.SymbolicMaskSetsWithoutAWho = interp.No
	s.SymbolicMaskWhoAloneSetsIt = interp.No
	s.SymbolicMaskTakesTheSetuidLetter = interp.No
	s.SymbolicMaskTakesTheStickyLetter = interp.No
	s.ShiftReadsOptions = interp.Yes
	s.WaitReadsOptions = interp.No
	// Job specs by command text, a second match taken rather than refused;
	// `wait` complains about a spec that names nothing, has no -n, and
	// `disown` takes the job out of the table.
	s.JobSpecsByName = interp.Yes
	s.AmbiguousJobNameIsRefused = interp.No
	s.WaitReportsAMissingJob = interp.Yes
	s.WaitNWaitsForTheNextJob = interp.No
	// A trapped signal cuts a `wait` short with 128 plus the signal, and the
	// form that names a job answers the same as the bare one.
	s.WaitForAJobFailsWhenInterrupted = interp.No
	s.DisownRemovesTheJob = interp.Yes
	s.CommandRejectsUnknownOption = interp.No
	s.GetoptsRejectsUnknownOption = interp.No
	s.ShiftCountIsArithmetic = interp.Yes
	// No by construction rather than by measurement: zsh has already parsed
	// the whole action by the time a trap fires, so it never runs part of a
	// body whose rest will not parse. The two answers cannot be told apart
	// here, and this is the one that describes what zsh did.
	s.TrapBodyRunsWhatParsed = interp.No
	s.ExitTrapFiresPastTheEnd = interp.Yes
	s.SelectAssumesUnboundedWidth = interp.Yes
	s.SelectEofEndsPromptLine = interp.Yes
	s.SelectEofIsSuccess = interp.Yes
	s.SelectTakesUnterminatedReply = interp.Yes
	s.SelectEofPrintsNewline = interp.No
	s.DeclaredNameWithoutValueIsEmpty = interp.Yes
	s.TypesetLocalNeedsKeywordFunction = interp.No
	// A local does not inherit the export attribute of the name it shadows.
	// Measured with a real child: `export FOO=bar; f() { local FOO=baz; env;
	// }` shows the child no FOO at all here, where bash and dash show it the
	// local's value.
	s.LocalInheritsTheExportAttribute = interp.No
	s.SplitParamExpansion = interp.No
	s.GlobExpansionResults = interp.No
	s.GlobNoMatchIsError = interp.Yes
	s.AssignmentPrefixPersistsOnSpecialBuiltin = interp.No
	// hash counts only what PATH holds: a builtin or a function is "no
	// such command" to it.
	s.HashSearchesPathAlone = interp.Yes
	s.TildePlusMinusExpands = interp.Yes
	s.UnderscoreTracksTheLastArgument = interp.Yes
	// Bases stop at 36 here, and the refusal says so.
	s.ArithBaseAbove36 = interp.No
	// ${#a} of an array counts elements, and a function's $LINENO counts
	// from the function.
	s.ArrayLengthWithoutSubscriptIsCount = interp.Yes
	s.FcEmptyHistoryIsAnError = interp.Yes
	s.JobControlAbsenceIsReportedFirst = interp.Yes
	// CDPATH moves in silence here.
	s.CdpathAnnouncesTheDirectory = interp.No
	s.LinenoCountsFromTheFunction = interp.Yes
	s.EchoInterpretsEscapes = interp.Yes
	// echo reads -n, -e and -E, and -e wins over -E whatever the order.
	s.EchoOptions = "neE"
	s.EchoLastEscapeFlagWins = interp.No
	s.EchoExpandsHexEscapes = interp.Yes
	s.EchoExpandsEscEscape = interp.Yes
	// read takes -r and -s, -A with the array as the first operand, and the
	// same -d, -t and -u as the others — but no counts: -N is a bad option
	// here and -n is a flag it reads and, outside completion widgets, acts
	// on not at all. -p is a bare flag too — the coprocess, not bash's
	// prompt — and with zsh's `coproc` outside this grammar there is never
	// one to read: the letter always answers `-p: no coprocess` and 1, the
	// variables untouched. zsh's -t may also stand alone as a poll; that
	// spelling is not modeled, so here it reads the word after it as its
	// seconds.
	s.ReadOptions = "rsnpAd:t:u:"
	s.ReadZeroTimeout = interp.ReadZeroTimeoutFinishesWhatItStarted
	s.ReadTimeoutKeepsWhatArrived = interp.No
	s.ArithLeadingZeroIsOctal = interp.No
	s.FatalErrorStatusIsOne = interp.Yes
	s.ArithNameValueRecurses = interp.Yes
	s.BraceExpansion = interp.Yes
	// Pads like bash — `{01..3}` is `01 02 03` — but a negative step
	// reverses the walk the endpoints chose: `{3..1..-1}` is `1 2 3` and
	// `{1..10..-4}` is `9 5 1`, bash's `1 5 9` backwards rather than the
	// `10 6 2` swapped endpoints would give.
	s.BraceRangePadsToEndpointWidth = interp.Yes
	s.BraceRangeStepSignHonored = interp.No
	s.BraceRangeNegativeStepReverses = interp.Yes
	s.BracketCaretNegates = interp.Yes
	s.EqualsExpansion = interp.Yes
	s.LastPipelineElementInCurrentShell = interp.Yes
	s.ShiftPastEndFatal = interp.No
	s.ArrayBaseIsZero = interp.No
	s.ArrayLiteralSubscriptIsAKey = interp.No
	s.DollarZeroInFunctionIsFunctionName = interp.Yes
	s.BuiltinSyntaxErrorFatal = interp.No
	s.DotMissingFileFatal = interp.No
	s.DotPassesArguments = interp.Yes
	// zsh and ksh93 drop the EXIT trap when an exec fails; dash and bash
	// still run it.
	s.ExecFailureRunsExitTrap = interp.No
	s.ExecTakesOptions = interp.Yes
	s.TestAcceptsDoubleEqual = interp.Yes
	// `-nt` and `-ot` want both files to exist, and `-t x` is a plain
	// false rather than an integer complaint.
	s.MissingFileIsOlder = interp.No
	s.TerminalTestRequiresANumber = interp.No
	s.PipefailOption = interp.Yes
	s.ErrexitSeesPipefailFailure = interp.Yes
	// Alone in refusing an argument to `times`; dash and bash ignore it.
	s.TimesRejectsArguments = interp.Yes
	s.UnterminatedBracket = interp.BracketBadPattern
	s.ExitTrapIsFunctionLocal = interp.Yes
	s.SignalHandlerSeesEarlierStatus = interp.Yes
	s.ExitArgument = interp.ExitArgLenient
	// A status that carries a count rather than a verdict: two dead targets
	// is 2.
	s.ExitTrapRunsOnSignalDeath = interp.No
	s.QuitIgnoredWhenNotInteractive = interp.Yes
	// zsh alone: a bare `exit` there reports what the trap's own last
	// command did, so `trap "false; exit" 0` exits 1.
	s.ExitInTrapReportsEarlierStatus = interp.No
	s.KillListAcceptsName = interp.Yes
	s.SIGPrefixAccepted = interp.Yes
	s.RedirectsWriteToEveryTarget = interp.Yes
	s.KillStatus = interp.KillStatusFailureCount
	s.SubshellJobTable = interp.SubshellJobsCleared
	s.PrintfEmptyIsNotANumber = interp.No
	s.PrintfReportsBadNumber = interp.No
	s.PrintfBackslashC = interp.PrintfBackslashCStops
	// No `%(fmt)T`: `%(` is a directive this shell does not have.
	s.PrintfTimeConversion = interp.No
	s.PrintfQuote = interp.PrintfQuoteBackslash
	// zsh is the one shell with `$'…'` and no `\c` in it, so `$'\cA'` is the
	// two characters `cA`; and its strings are counted rather than
	// terminated, so a decoded NUL is a byte like any other.
	s.DollarSingleBackslashC = interp.DollarSingleControlAbsent
	s.DollarSingleUnknownEscape = interp.DollarSingleUnknownDropsBackslash
	s.DollarSingleNulTruncates = interp.No
	s.GetoptsAssignmentRestartsWord = interp.No
	s.GetoptsClearsOptarg = interp.Yes
	s.CdWithoutHomeIsAnError = interp.No
	s.CdDashPrintsTheDirectory = interp.No
	s.PrintfAssignsWithV = interp.Yes
	s.PrintfRejectsUnknownOption = interp.No
	// zsh reads no options here: `trap -p` sets a trap whose action is the
	// word `-p`, and the failure surfaces when it fires.
	s.TrapParsesOptions = interp.No
	s.TrapOneArgumentIsACondition = interp.Yes
	s.TrapReportsAnUnknownSingleCondition = interp.No
	// ERR and DEBUG but not RETURN, and both follow the script everywhere:
	// into functions, and — alone in the panel — into subshells and command
	// substitutions, where the handler's output is captured with the rest.
	s.TrapHasErrCondition = interp.Yes
	s.TrapHasDebugCondition = interp.Yes
	s.TrapHasReturnCondition = interp.No
	s.ErrTrapRunsInsideFunctions = interp.Yes
	s.ErrTrapRunsInSubshells = interp.Yes
	s.DebugTrapRunsInsideCalls = interp.Yes
	s.DebugTrapRunsInSubshells = interp.Yes
	// The listing least is kept of: `(trap)` and `$(trap)` show nothing the
	// parent had — not even an ignored signal, though it stays ignored in
	// fact and one the subshell sets itself is shown. A pipeline element is
	// the exception and keeps the signal listing while still dropping the
	// EXIT trap from it: `trap 'echo x' USR1; trap | cat` prints the trap
	// and the same shape with an EXIT trap prints nothing. Measured, each.
	s.PipelineElementKeepsTrapListing = interp.Yes
	s.KeptTrapListingIncludesExit = interp.No
	s.SubshellHidesInheritedIgnoredTraps = interp.Yes
	s.UmaskPrintsFourDigits = interp.No
	s.UmaskSetWithSPrints = interp.No
	s.UlimitBlockIsKilobyte = interp.No
	s.UlimitHasResidentSet = interp.No
	s.UlimitHasProcessCount = interp.Yes
	s.UlimitSetsBothLimits = interp.No
	s.BadOptionToSpecialBuiltinFatal = interp.No
	// zsh takes it and sets a global instead of refusing.
	s.LocalOutsideAFunctionIsAnError = interp.No
	// Fatal to all three, which is the one place zsh is stricter than bash
	// about a builtin's failure. It refuses fewer operands, though, and the
	// two sets it adds have only `0` in common: `export ?` is quiet and
	// `unset ?` is not, `unset 12` is quiet and `export 12` is not.
	s.BadNameToDeclarationFatal = interp.Yes
	s.BadNameToUnsetFatal = interp.Yes
	s.DeclarationNameOperands = interp.NamesAndSpecialParameters
	s.UnsetNameOperands = interp.NamesAndPositionals
	s.DeclarationTakesASubscript = interp.No
	s.UnsetTakesASubscript = interp.Yes
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.No
	s.JobsListFinishedJobs = interp.No

	// `jobs`' letters: POSIX's pair, the state filters, and three of zsh's
	// own — `-d` adds the directory the job was started in, `-z` and `-Z`
	// are about the process title — which ride UnimplementedOptionLetters.
	// `-n` and `-x`, which bash has, are bad options here.
	s.JobsOptions = "lprs"
	// The split this issue was about. zsh reads `-p` as "put the job's
	// process *group* id in the listing" and prints its ordinary rows,
	// where the other three print the ids and nothing else — so
	// `kill $(jobs -p)` is a bash idiom rather than a portable one.
	s.JobsPidsOnlyOption = interp.No
	// Both filters at once list a job in either state here, where bash lets
	// the last letter given decide.
	s.JobsStateFiltersAccumulate = interp.Yes

	// Whether a `&` job's command appears in a `jobs` listing.
	s.JobsShowBackgroundCommand = interp.Yes

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
	s.ExportCarriesFunctions = interp.No
	s.AnnouncesBackgroundJob = interp.Yes
	s.ReportsACommandKilledBySignal = interp.No
	s.ReportsAnyKilledPipelineElement = interp.No
	s.ChildInterruptEndsTheScript = interp.No
	s.CdRefusesUnknownOption = interp.No
	s.CdLastPathOptionWins = interp.No
	s.BadSetOptionNameFatal = interp.Yes
	s.ReturnOutsideAFunctionIsRefused = interp.No
	s.LoneDashIsAnOption = interp.Yes
	s.UnsetFunctionChecksTheName = interp.No
	s.UnsetFunctionReportsMissing = interp.Yes

	// Whether a redirection target is expanded as an ordinary word.
	s.RedirectTargetIsAnOrdinaryWord = interp.No

	// Whether `type --` ends the options.
	s.TypePrintsFunctionBody = interp.No
	s.TypeEndsOptionsWithDashDash = interp.Yes
	// zsh's word-per-name option is `-w`, with its own vocabulary; `-t` is
	// a bad option there.
	s.TypeNamesTheKindWithDashT = interp.No
	// whence -v under another name, so the letters answer in sentences:
	// -p searches PATH past the shell's own answer and words the hit the
	// way plain type does, and -f *prints* a function rather than skipping
	// it. The completion-system letters ride as unimplemented.
	s.TypeOptions = "afp"
	s.TypePSearchesPathPastTheShell = interp.Yes
	s.TypePathAnswerIsASentence = interp.Yes
	s.TypeFSaysTheFunctionBack = interp.Yes

	// The letters `typeset` and `local` read. `-f` prints function bodies;
	// `-F` is a *float's* precision here rather than bash's function-name
	// listing, so it rides in Diagnostics.UnimplementedOptionLetters with
	// the rest of what this shell has and this engine does not.
	s.DeclareOptions = "aAfgilprux"
	s.LocalOptions = "aAilprux"
	// A bad `typeset` option is reported and the script goes on.
	s.TypesetBadOptionFatal = interp.No
	// `typeset -g x=new` with a `local x` in front assigns the *local* —
	// the letter only widens where a new declaration would land, it does
	// not reach past what already stands. Measured: `in=new out=out`
	// against the other engine's `in=in out=new`.
	s.DeclareGlobalReachesPastALocal = interp.No
	// A bare `local` — and a bare `set` — list every parameter the shell
	// has, special parameters and tied arrays included: a fact about this
	// engine's parameter table, refused as unimplemented rather than
	// approximated.
	s.BareLocalListing = interp.BareLocalListsEveryParameter
	s.SetListing = interp.SetListingEveryParameter

	return s
}

// Diagnostics is how zsh reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		TypeKeyword: "%[1]s is a reserved word",
		// The only one that names itself in the line.
		TypeFunction:           "%[1]s is a shell function from zsh",
		TypeNotFound:           "%[1]s not found",
		TypeNotFoundUnprefixed: true,
		// And on standard output, which is the other half of treating it as
		// an answer rather than a complaint: `type -- nope ls` prints the
		// miss and the hit on one stream, in the order they were asked for.
		// Measured: `type nope 1>/dev/null` prints nothing.
		TypeNotFoundOnStdout: true,
		// `command -V` complains the way `type` does, shell's name and all.
		CommandVNotFound: "%[1]s not found",
		// A function said back keeps its opening brace on the header's
		// line: `f () {`. The other engine gives the brace a line of its
		// own, which is the wording's fallback.
		FunctionListingHeader: "%[1]s () %[2]s",
		JobStarted:            "[%[1]d] %[2]d",
		// Two spaces before the marker, one after, and a state of its own in
		// lower case in an 11-wide column. zsh never lists a finished job,
		// so it needs no word for one.
		JobLine: "[%[1]d]  %[2]s %-11[3]s%[4]s",
		// `jobs -l`, and `jobs -p` too: the process id after the marker,
		// with the same 11-wide state column after it.
		JobLineLong: "[%[1]d]  %[2]s %[3]d %-11[4]s%[5]s",
		// The builtin's name is in the location here rather than in the
		// sentence, which is this shell's rule for every message.
		// About its table rather than about the function, and the builtin
		// is named in the location as it is for every message here.
		UnsetFunctionNotFound:    "no such hash table element: %[1]s",
		DeclareNoSuchVariable:    "no such variable: %[1]s",
		LocationNamesTheFunction: true,
		SetInvalidOptionName:     "no such option: %[1]s",
		SetInvalidOptionStatus:   1,
		// A denied `set -m` echoes the spelling it was asked with — `-m` or
		// `monitor` — and fails at 1, fatally like every `set` failure here.
		MonitorDenied:       "can't change option: %[1]s",
		MonitorDeniedStatus: 1,
		// zsh knows `-f` — it means functions to its own typeset — so what
		// it refuses is the combination, and it says so without naming the
		// letter it names in every other refusal.
		ExportFunctionOptionRefused: "invalid option(s)",
		JobRunning:                  "running",
		// Only ever seen in a completion notice: zsh's listing never
		// mentions a job that has ended.
		JobDone:    "done",
		JobStopped: "suspended",
		// The reason first and the name after it, which is zsh's shape and
		// nobody else's. Lowercased, which LowercaseReason already says.
		// Same either way — zsh does not distinguish opening from creating.
		CannotOpen:               "%[2]s: %[1]s",
		CannotCreate:             "%[2]s: %[1]s",
		NotABuiltin:              "no such builtin: %[1]s",
		CommandStringParsedWhole: true,
		ArithInfinity:            "Inf",
		ArithNotANumber:          "NaN",
		ArithFloatDigits:         17,
		ArithFloatKeepsPoint:     true,
		SelectPrompt:             "?# ",
		// The reason before the name is zsh's shape everywhere, and here it
		// drops the reason altogether: a script operand that is missing, one
		// that is mode 000 and a directory all get the same sentence and the
		// same 127. So zsh knows the file would not open and declines to say
		// which way — the one shell in the panel that splits neither the
		// wording nor the status.
		ScriptNotFound:          "can't open input file: %[1]s",
		ScriptNotFoundStatus:    127,
		ScriptNotReadableStatus: 127,
		Location:                interp.LocationTightLine,
		BadSubstitution:         "bad substitution",
		// The position is 1-based and counts from the `$`: `${(Y)x}` errors
		// at 4, and a group that runs out of text errors just past the end.
		ExpansionFlagsError: "error in flags near position %[1]d in '%[2]s'",
		BadPattern:          "bad pattern: %s",
		BuiltinBadSubscript: map[string]string{
			"export":   "%[3]s: assignment to invalid subscript range",
			"readonly": "%[2]s: can't create readonly array elements",
		},
		SubscriptRefusalNamesBuiltin: map[string]bool{"readonly": true},
		UnimplementedOptionLetters: map[string]string{
			// read's letters about a terminal or the line editor — raw -k
			// keys, -q's one keystroke, -e/-E echoing, -z and the zle pair
			// -c/-l. The -p coprocess is implemented as its measured
			// refusal — see ReadNoCoprocess. zsh's read also says nothing
			// at all about a dead -u descriptor and reports 1, which is why
			// no ReadBadFileDescriptor wording appears here.
			"read": "kqeEzcl",
			// typeset's letters this engine does not hold: floats (-E -F),
			// namerefs (-n), padding and alignment (-L -R -Z), uniqueness,
			// hiding, ties and the rest. The same set under both names, and
			// for `local` too.
			"typeset": "bcEFhHkLmnRtTUZ",
			"type":    "mvwsS",
			// jobs' letters that are zsh's own: -d names the directory the
			// job was started in, and -z and -Z are about the process
			// title rather than about the job table.
			"jobs":    "dzZ",
			"declare": "bcEFhHkLmnRtTUZ",
			"local":   "bcEFhHkLmnRtTUZ",
		},
		// The builtin's name is stripped to the location prefix as ever:
		// `zsh:read:1: -p: no coprocess`, measured with no coprocess to
		// read, which is the only state this shell has.
		ReadNoCoprocess: "read: -p: no coprocess",
		// `zsh:read:1: argument expected: -d` — the letter after the
		// sentence, unlike everyone else, and status 1 like every other
		// option complaint here.
		OptionNeedsArgument: "%[1]s: argument expected: -%[2]s",
		ReadonlyVariable:    "read-only variable: %s",
		// `ulimit -a`, row for row as the engine writes it — the flag
		// first, no pipe row, and no resident-set row at all.
		UlimitListing: []interp.UlimitListingRow{
			{Prefix: "-t: cpu time (seconds)              ", Res: interp.ResourceCPUTime, Scale: 1},
			{Prefix: "-f: file size (blocks)              ", Res: interp.ResourceFileSize},
			{Prefix: "-d: data seg size (kbytes)          ", Res: interp.ResourceData, Scale: 1024},
			{Prefix: "-s: stack size (kbytes)             ", Res: interp.ResourceStack, Scale: 1024},
			{Prefix: "-c: core file size (blocks)         ", Res: interp.ResourceCore},
			{Prefix: "-v: address space (kbytes)          ", Res: interp.ResourceAddressSpace, Scale: 1024},
			{Prefix: "-l: locked-in-memory size (kbytes)  ", Res: interp.ResourceLockedMemory, Scale: 1024},
			{Prefix: "-u: processes                       ", Res: interp.ResourceProcesses, Scale: 1},
			{Prefix: "-n: file descriptors                ", Res: interp.ResourceOpenFiles, Scale: 1},
		},
		TraceQuoting:   interp.QuoteShell,
		TraceStyle:     interp.TraceNameLine,
		TraceForHeader: interp.TraceForAssign,
		// zsh names the last token it read and nothing else.
		EvalNaming:       interp.SourceReplacesShell,
		SourceFileNaming: interp.SourceReplacesShell,
		EvalSourceName:   "(eval)",
		// A runtime failure at the top level of a sourced file names the
		// file — `./inc.sh:1: command not found: nosuch` — while one inside
		// a function still names the function, which the answer above wins.
		LocationNamesTheCurrentFile: true,
		// zsh does not quote the expression, where the other three do.
		ArithError:                   "%[2]s",
		ArithInvalidBase:             "invalid base (must be 2 to 36 inclusive): %[1]s",
		OptionListingWidth:           22,
		KillListing:                  interp.KillListingSpaceJoined,
		FcNoSuchEvent:                "no such event: 1",
		NoJobControl:                 "no job control in this shell.",
		FdVariableWithoutADescriptor: "parameter %[1]s does not contain a file descriptor",
		ArithOperandExpected:         "bad math expression: operand expected at end of string",
		ArithOperatorExpected:        "bad math expression: operator expected at `%[1]s'",
		SyntaxUnexpected:             "parse error near `%[1]s'",
		// zsh names itself and stops when a function's body never began.
		// `f() ;` reports as zsh: parse error near `;' where `if true` — an
		// input that ran out just as much — reports the line as well, as
		// zsh:1: parse error near `true'.
		MissingFuncBodyOmitsTheLine: true,
		ForName:                     "parse error near `%[1]s'",
		Unterminated:                "parse error near `%[5]s'",
		UnmatchedQuote:              "unmatched %[1]s",
		UnmatchedCmdSubst:           "parse error near `%[3]s'",
		UnmatchedBraceSubst:         "closing brace expected",
		SyntaxErrorStatus:           1,
		// zsh alone answers "a syntax error" differently depending on where it
		// read the text: 1 from -c, 126 from a file `.` opened.
		SourcedSyntaxErrorStatus: 126,
		DotCannotOpen:            ".: no such file or directory: %[1]s",
		DotCannotOpenStatus:      127,
		DotNoOperand:             ".: not enough arguments",
		DotNoOperandStatus:       1,
		// zsh leads with the reason, lowercased, and names the command after
		// it — the reverse of the other three.
		// zsh alone says something for a shift it survives; bash says
		// nothing, and the two that speak up here treat it as fatal.
		ShiftTooMany:         "shift count must be <= $#",
		StdinLocation:        interp.LocationNameOnly,
		StdinBuiltinLocation: interp.LocationBuiltinNameOnly,
		CannotExecute:        "%[2]s: %[1]s",
		// zsh says "command not found" for a bare name it could not resolve,
		// where the other three say "not found".
		// zsh leads with the complaint and names the command after it, for a
		// command word exactly as for `exec`.
		NotFound:     "command not found: %[1]s",
		ExecNotFound: "command not found: %[1]s",
		// The builtin's name comes from the location here, not the message.
		// No "kill:" in front of any of these: zsh puts the builtin's name in
		// the location instead, which NamesBuiltinInLocation already says.
		// No "printf:" in front: zsh puts the builtin in the location.
		// The reason first and the operand after it, which is the reverse of
		// everyone else — and lowercased, which LowercaseReason already says.
		GetoptsBadOption:       "bad option: -%[1]s",
		GetoptsMissingArgument: "argument expected after -%[1]s option",
		CdCannotChange:         "%[2]s: %[1]s",
		PrintfBadVerb:          "%[2]s: invalid directive",
		UmaskBadMask:           "bad umask",
		// The builtin's name comes from the location, as everywhere in zsh.
		UnaliasNotFound:        "no such hash table element: %[2]s",
		UnaliasAllWithOperands: "-a: too many arguments",
		UnaliasUsage:           "not enough arguments",
		UnaliasNoOperandStatus: 1,
		// The builtin's name comes from the location here, as everywhere in
		// zsh, so it is not in the wording.
		UmaskBadSymbolicMode:     "bad symbolic mode permission: %[2]s",
		UmaskBadSymbolicOperator: "bad symbolic mode operator: %[2]s",
		UmaskBadOption:           "bad option: %[1]s",
		UmaskBadOptionStatus:     1,
		LetNoExpression:          "not enough arguments",
		UlimitBadOption:          "bad option: -%[1]s",
		UlimitBadNumber:          "invalid number: %[1]s",
		UlimitBadOptionStatus:    1,
		BuiltinBadOption:         "%[1]s: bad option: %[2]s",
		BadOptionNaming:          interp.BadOptionFirstUnknownLetter,
		WaitBadJob:               "wait: job not found: %[1]s",
		WaitBadJobStatus:         127,
		// The builtin's name rides the location, as ever.
		WaitNoSuchJob:                    "wait: %[1]s: no such job",
		KillNoSuchJob:                    "kill: %[1]s: no such job",
		DisownNoCurrentJob:               "disown: no current job",
		WaitNotOurChild:                  "wait: pid %[1]d is not a child of this shell",
		TrapCouldNotParse:                "couldn't parse trap command",
		UmaskWhoAloneIsANumericComplaint: true,
		// The builtin's name comes from the location, so it is not in these.
		// The reason leads for `export` and `readonly` and trails for `unset`,
		// which is why this is a map.
		BuiltinBadName: map[string]string{
			"export":   "not valid in this context: %[2]s",
			"readonly": "not valid in this context: %[2]s",
			"unset":    "%[2]s: invalid parameter name",
			"local":    "not valid in this context: %[2]s",
		},
		// An operand that starts with a digit is a different complaint, for
		// the two that have one. `unset` says the same to both.
		BuiltinBadNameNumeric: map[string]string{
			"export":   "not an identifier: %[2]s",
			"readonly": "not an identifier: %[2]s",
			"local":    "not an identifier: %[2]s",
		},
		BuiltinBadOptionStatus:    1,
		PrintfUsage:               "not enough arguments",
		PrintfUsageStatus:         1,
		TrapBadSignal:             "undefined signal: %[1]s",
		KillNoSuchProcess:         "kill %[1]s failed: no such process",
		KillNotPermitted:          "kill %[1]s failed: operation not permitted",
		KillInvalidSignal:         "unknown signal: %[3]s",
		KillIllegalOption:         "unknown signal: %[3]s",
		KillNotAPid:               "illegal pid: %[1]s",
		KillMissingSignalArgument: "%[1]s: argument expected",
		KillUsage:                 "not enough arguments",
		// -L, capitalized, and measured against the zsh the panel resolves:
		// 5.9.2 from Homebrew says -L where Apple's /bin/zsh 5.9 says -l.
		// Probing whichever zsh came first on PATH is how the lowercase one
		// got here.
		KillUnknownSignalHint: "type kill -L for a list of signals",
		// zsh is the one dialect that does not treat "nothing to signal" as a
		// usage error worth a different number from any other failure.
		KillUsageStatus:      1,
		TestUnaryExpected:    "unknown condition: %[1]s",
		TestBinaryExpected:   "condition expected: %[1]s",
		TestIntegerExpected:  "integer expression expected: %[1]s",
		TestTooManyArguments: "too many arguments",
		TestOperandExpected:  "argument expected",
		TestMissingBracket:   "']' expected",
		TimesDecimals:        2,
		TimesArguments:       "times: too many arguments",
		// The `time` keyword reports one line per pipeline element that
		// forked, labeled with the element as written, and nothing for one
		// that did not — `time true` prints nothing at all here. A bare
		// `time` reports a `shell` and a `children` line in the same shape.
		TimeLayout:   interp.TimePerCommand,
		TimeBare:     interp.TimeBareShellAndChildren,
		PathNotFound: "no such file or directory: %[1]s",
		// zsh lowercases every strerror string it quotes, where the other
		// three print the C string as it comes.
		// zsh names the builtin that is speaking between its own name and the
		// line: `zsh:shift:1:`. A rule rather than a handful of cases, and the
		// only shell in the panel that does it.
		NamesBuiltinInLocation: true,
		LowercaseReason:        true,
		DirectoryReason:        "Permission denied",
		HashNotFound:           "no such command: %[1]s",
	}
}

// Apply adds what zsh has and the substrate does not.
//
// `source` is a synonym for `.`, which dash does not have at all — so the name
// is a dialect's answer, and it is the same function under a second name
// rather than a second implementation.
func Apply(r *interp.Runner) {
	// This shell has an `enable`, but a different one: it works on hash
	// tables and takes none of bash's options — `enable -n` is a bad option
	// there. Claiming a bash-shaped one would be worse than not having it.
	// Not removed but replaced: zsh has an `enable`, and it is a different
	// builtin from the one the core carries. See enable.go.
	registerEnable(r)
	// How zsh scripts actually change options, and how they change shells.
	// See setopt.go and emulate.go.
	registerSetopt(r)
	registerEmulate(r)
	// This shell's own question about a name, under two names: `whence` and
	// `where`. Not ksh93's builtin under the same spelling — the stream, the
	// statuses, the letters and every wording differ. See whence.go.
	registerWhence(r)
	// No `compgen` here; it is bash's alone.
	r.Unregister("compgen")
	r.Unregister("complete")
	// And neither `mapfile` nor its other name; both are bash's alone.
	r.Unregister("mapfile")
	r.Unregister("readarray")
	// The `set -o` names beyond the ones every shell has.
	//
	// `trackall` is here because zsh has *both* spellings of command
	// tracking — `set -o hashall` and `set -o trackall` each succeed, and
	// each moves the same state — where bash has only the first and ksh93
	// only the second. It was missing until the membership was re-measured
	// against the panel rather than read off `semantics.md`, which said
	// trackall belonged to ksh93 alone; `set +o trackall` was a refusal
	// here and a no-op in the shell this dialect imitates. Pinned by
	// `opt/command-tracking-has-two-long-names`.
	r.AddSetOptions(
		"braceexpand",
		"hashall",
		"histexpand",
		"histignoredups",
		"onecmd",
		"physical",
		"pipefail",
		"privileged",
		"trackall",
	)
	// The statuses of the last pipeline's elements. The core keeps the
	// record and this names it; ksh93 and dash have no name for it at all.
	r.SetPipelineStatus("pipestatus")
	// A read of the process, in library code, on purpose. The purity rule
	// covers dialect/ as well as interp/ — an embedder links this and Apply
	// runs inside the Runner — but it is about state a Runner owns and two
	// Runners could disagree about. A uid is neither: nothing a script does
	// changes it, two shells in one program genuinely have the same one, and
	// $UID has no other source. Same class as $$, which .golangci.yml has
	// blessed since it was written.
	r.SetSpecial("UID", strconv.Itoa(os.Getuid()))
	r.SetSpecial("EUID", strconv.Itoa(os.Geteuid()))
	r.SetDynamic("RANDOM", func(*interp.Runner) string { return interp.Randoms() })
	r.SetDynamic("SECONDS", func(rr *interp.Runner) string {
		return strconv.Itoa(int(rr.SecondsFrom()))
	})
	// A NUL as well as the three whitespace characters, which is zsh's alone.
	r.SetSpecial("IFS", " \t\n\x00")
	if dot, ok := r.Builtin("."); ok {
		r.Register("source", dot)
	}
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
	// How a function is laid out when something says one back — `typeset
	// -f` here; this shell writes nothing into the environment, so the
	// exported arrangement is the shown one.
	r.SetFunctionLayout(FunctionLayout(), FunctionLayout())
}
