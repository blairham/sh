// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package ash answers the substrate's questions the way BusyBox ash does.
//
// # What was measured, and what nothing checks
//
// Every answer below was measured by running BusyBox v1.37.0's `ash` — the
// `/bin/ash` of the `alpine:3` image — over the whole oracle corpus and over
// hand-written probes, on 2026-09-12. No BusyBox source was read; see
// CLEANROOM.md, and docs/spec/ash.md for the method and the numbers.
//
// It is nonetheless the first dialect in this tree whose behavior **no
// instrument grades**. The oracle panel locates its shells with
// exec.LookPath, and there is no ash binary on a macOS machine — `brew
// install busybox` has no formula, and BusyBox's published binaries are Linux
// ELF — so ash has no column in the golden record and `make
// conformance-dialects` has no row for it. The answers here are therefore
// *measured* but not *maintained*: nothing in `make check` notices when one
// of them drifts. #2263 is the follow-on that fixes it.
//
// Read a comment that cites a measurement as evidence; read the absence of
// one as an unanswered question rather than as agreement with dash.
package ash

import (
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what ash parses.
//
// It starts from POSIX like every preset — not from dash, whose file this one
// must not be read as a delta against. The two shells are both NetBSD ash
// descendants, and the issue that asked for this dialect predicted the
// grammars would nearly coincide. Measured, they do not: this shell takes
// `$'…'`, `[[ ]]`, `<(…)`, `function`, `${v:1:2}`, `${v/a/b}`, `**`,
// `base#digits`, `++`, `&>` and `>|`, every one of which dash refuses.
func Dialect() syntax.Dialect {
	d := syntax.POSIX()
	// Nine constructs beyond POSIX, each run and watched rather than read
	// about. `echo $'a\tb'` writes a tab; `[[ a = a ]]` succeeds where dash
	// answers `[[: not found`; `cat <(echo hi)` writes hi; `function f {
	// …; }` and `function f() { …; }` both define; `${v:1:3}` of `abcdef` is
	// `bcd`; `${v/b/X}` of `abc` is `aXc`.
	d.DollarSingleQuote = true
	d.DoubleBracket = true
	d.ProcessSubstitution = true
	d.FunctionKeyword = true
	d.FunctionKeywordParens = true
	d.ParamSubstring = true
	d.ParamSubstitution = true
	// `$((2**3))` is 8 and `$((10#08))` is 8, where dash refuses both; `i=1;
	// echo $((i++)) $i` is `1 2`; `$((1,2))` is 2.
	d.ArithExponent = true
	d.ArithExplicitBase = true
	d.ArithIncDec = true
	d.ArithComma = true
	// The two redirection spellings: `echo x &>/dev/null` is silent, and
	// `set -C; echo x >| f` overwrites.
	d.AmpersandRedirect = true
	d.ClobberOverrideMarker = true
	// `time echo hi` prints the three-row summary this shell words its own
	// way, so the keyword is here rather than the external.
	d.TimeKeyword = true
	// A name followed by `(` is a function definition, whatever comes next:
	// `f(x) { :; }` is refused for the word rather than for the parenthesis,
	// which is what decides the token a malformed definition is blamed on.
	d.FuncDefAtParen = true
	// And a name with punctuation in it is a name: `a.b() { echo hi; }; a.b`
	// prints hi.
	d.FunctionNamePunctuation = true
	// One operator may stand where a case pattern belongs: `case a in ;)
	// echo x;; *) echo def;; esac` reaches the default arm rather than
	// failing to parse.
	d.CasePatternAcceptsOperator = true
	// The loop-variable position is read as a name whatever stands there, so
	// `for ; do :; done` and `for "i" in a; …` are both `bad for loop
	// variable`.
	d.ForNonWordIsANameError = true
	// Aliases expand in a script and on standard input and *not* under `-c`:
	// `ash -c 'alias foo=echo; foo'` answers `foo: not found`, while the same
	// two lines in a file and piped in both expand. dash expands on every
	// route, which is the measured split between the two siblings.
	d.ExpandAliases = syntax.RouteFromScriptFile | syntax.RouteOnStandardInput
	// And a body's newlines are input lines, the answer three of the four
	// existing dialects give.
	d.AliasBodyCountsLines = true
	// Deliberately absent, each refused when run: arrays (`a=(1 2 3)` is
	// `unexpected "("`, `${a[0]}` is `bad substitution`), the C-style `for`,
	// `select`, `<<<`, `(( ))`, `$[…]`, floating-point arithmetic, `${v^^}`,
	// `${!x}`, `${v@Q}`, `|&`, `;;&`, `$(<f)`, `{fd}>f`, `@(…)`, brace
	// expansion and `+=` — `v+=b` is a command this shell cannot find.
	return d
}

// Semantics is what ash means where the shells conflict.
//
// The measured shape is the finding the issue behind this package asked for,
// and it is not the one the issue predicted: ash sides with bash rather than
// with dash on `[^x]`, on `echo` needing `-e`, on a valueless `local`, on
// `**` and on `base#digits`. It is dash's sibling by ancestry and not by
// behavior.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()

	// ---- expansion and words ----

	// `export a+=2` is `a+: bad variable name`, so the append operator is not
	// an operand this shell's declarations take.
	s.DeclarationTakesAnAppendOperand = interp.No
	// POSIX makes an unquoted `$@` behave as `$*` where nothing is split, and
	// this shell complies: `IFS=-; set -- x y z; v=${@}` is `x-y-z`.
	s.UnsplitAtListJoinsOnIFS = interp.Yes
	// An empty positional list is a set parameter: `set --; "${@-word}"` is
	// empty and `"${@+word}"` is `word`.
	s.PositionalListWithNoneIsSet = interp.Yes
	// `${#@}` with three parameters is 5 — the width of `a b c` — rather than
	// the count.
	s.LengthOfSpecialIsCount = interp.No
	// The panel's dash column is the one with no multibyte decoder; this
	// shell has one and does not need a locale to use it. Measured with the
	// harness's fixed `LC_ALL=C`: `s=héllo; echo ${#s}` is 5 here and 6 in
	// dash.
	s.MultibyteEncodingIsHonored = interp.Yes
	// A value's backslash quotes what follows it rather than standing as a
	// character of the pattern: `v='a\*'; set -- $v` is `a\*`.
	s.ValueBackslashInAPattern = interp.ValueBackslashQuotesWhatFollows
	// `case x in [^a]) echo negates ;; *) echo plain ;; esac` reaches the
	// first arm. dash is the panel's sole holdout on reading the caret as a
	// negation, and this shell — its sibling — is not with it, so the "dash
	// alone" argument in interp/semantics.go survives ash rather than losing
	// to it. That prediction is the one #497 was filed on and is the one it
	// refuted.
	s.BracketCaretNegates = interp.Yes
	// An unterminated bracket in a pattern matches nothing rather than being
	// read as a literal: `case a in [a) echo one;; *) echo def;; esac`
	// reaches the default arm.
	s.UnterminatedBracket = interp.BracketNoMatch
	// `$(( ))` with nothing in it is 0 at status 0, where dash wants a
	// primary and stops the script.
	s.EmptyArithExpressionIsAnError = interp.No

	// ---- invocation and options ----

	s.CommandNotFoundStatusIsNotFound = interp.Yes
	s.SetFTurnsOffGlobbing = interp.Yes
	// No braces to expand and no `-B` to turn them off: `set -B` is `illegal
	// option -B` and ends the script.
	s.SetBTurnsOffBraceExpansion = interp.No
	// Nor the `-h` POSIX names: `set -h` is refused the same way.
	s.SetHasTheHLetter = interp.No
	// `$-` under `-c` is `c`, and `set -e -u` makes it `uce` — so the letter
	// is shown, where dash shows nothing at all on that route.
	s.CommandStringShowsCInDollarDash = interp.Yes
	s.LoginShowsLInDollarDash = interp.No
	s.CommandStringShowsSInDollarDash = interp.No
	// Both spellings of the login option are taken — `ash -l -c cmd` and
	// `ash --login -c cmd` each run the command — where dash refuses the long
	// one outright.
	s.StartupFileOptions = interp.StartupFileOptions{Login: "-l --login"}
	// VersionOption is left at its zero deliberately: `ash --version` is `bad
	// option '--version'`. This shell will not name its version through an
	// option, which is the same answer dash gives and reached the same way.
	//
	// Job control wants the tty, and says so rather than failing: `set -m`
	// with no terminal remarks `can't access tty; job control turned off` and
	// still reports 0.
	s.MonitorNeedsATerminal = interp.Yes
	s.InteractiveMonitorNeedsATerminal = interp.Yes

	// ---- builtins ----

	// A bare `read` fills REPLY, where dash wants a name.
	s.ReadRequiresAVariableName = interp.No
	// `read` takes rather more than dash's pair: `-r`, `-p`, `-t` and `-n`
	// were each run and each accepted.
	s.ReadOptions = "rp:t:n:"
	// `unset` has the two POSIX letters and calls anything else illegal:
	// `unset -q x` is `illegal option -q`.
	s.UnsetOptions = "vf"
	// `export -n` is accepted and reports 0, which dash refuses outright.
	s.ExportTakesTheAttributeOff = interp.Yes
	// `export -f` is `illegal option -f`, so a function does not travel.
	s.ExportCarriesFunctions = interp.No
	s.ExportListing = interp.DeclareListingCommandWord
	s.ReadonlyListing = interp.DeclareListingCommandWord
	// Every listed value is single-quoted with an embedded quote doubled out:
	// `v="quo'te"; set` writes `v='quo'"'"'te'`, and `alias` writes its
	// bodies the same way.
	s.DeclareValueQuoting = interp.ListingQuoteAlwaysDoubled
	s.SetListing = interp.SetListingAssignments
	s.SetListingQuoting = interp.ListingQuoteAlwaysDoubled
	s.AliasQuoting = interp.ListingQuoteAlwaysDoubled
	s.TrapQuoting = interp.ListingQuoteAlwaysDoubled
	// A bare `local` in a function writes nothing.
	s.BareLocalListing = interp.BareLocalListsNothing
	// `local` reads no options: `local -r x` declares a variable named `-r`
	// and then refuses it as the bad name it is, so LocalOptions stays empty.
	//
	// A valueless declaration leaves the name unset and hides the outer
	// value: `x=outer; f(){ local x; echo "[$x]"; }; f` is `[]` here and
	// `[outer]` in dash. That pairing is bash's, and it is the sharpest of
	// the five places this shell takes bash's side over its sibling's.
	s.DeclaredNameWithoutValueIsEmpty = interp.No
	s.ValuelessDeclarationHidesTheOuterValue = interp.Yes
	// `local` outside a function is refused and the refusal is fatal:
	// `local x=1` at the top level is `local: not in a function` and the
	// script ends at 2.
	s.LocalOutsideAFunctionIsAnError = interp.Yes
	s.LocalOutsideAFunctionIsFatal = interp.Yes
	// No `typeset` and no `declare` — both are `not found` — so neither can
	// be asked about a subscript or a readonly.
	s.DeclarationTakesASubscript = interp.No
	s.TypesetTakesASubscript = interp.No
	s.UnsetTakesASubscript = interp.No
	s.DeclarationNameOperands = interp.PlainNamesOnly
	s.UnsetNameOperands = interp.PlainNamesOnly
	s.ReadNameOperands = interp.PlainNamesOnly
	// No prompt operand: `read "v?p"` names one variable spelled `v?p`.
	s.ReadPromptOperand = interp.ReadOperandIsAllName
	// A special builtin's failure ends the script, and a bad name and a bad
	// option are both failures: `unset 1bad`, `export 1bad`, `unset r` over a
	// readonly and `export -Z` each stop at 2.
	s.BadNameToDeclarationFatal = interp.Yes
	s.BadNameToUnsetFatal = interp.Yes
	s.UnsetReadonlyFatal = interp.Yes
	s.BadOptionToSpecialBuiltinFatal = interp.Yes
	s.BadSetOptionNameFatal = interp.Yes
	// `read` is not one of the three: `printf 'x\n' | read 1bad` reports at 1
	// — not dash's 2 — and the script carries on.
	s.BadNameToReadFatal = interp.No
	// `echo` needs `-e` to interpret an escape, and has `-E` and `-n` beside
	// it: `echo "a\tb"` writes the backslash, `echo -e "a\tb"` writes a tab,
	// and `echo -E "a\tb"` writes the backslash again. dash is the shell that
	// interprets without asking, so this is the second bash-ward answer.
	s.EchoInterpretsEscapes = interp.No
	s.EchoOptions = "neE"
	s.EchoExpandsUnicodeEscapes = interp.No
	s.EchoExpandsEscEscape = interp.No
	s.EchoExpandsCapitalEscEscape = interp.No
	// `printf 'a\x41Z'` is `aAZ`, so the hex escape is here where dash has
	// none at all, and `%b` takes it too. `\u` is not: `printf 'aAZ'` is
	// the text as written.
	s.PrintfHexEscape = interp.PrintfHexEscapeByte
	s.PrintfBHexEscape = interp.PrintfHexEscapeByte
	s.PrintfUnicodeEscape = interp.PrintfUnicodeEscapeAbsent
	s.PrintfBUnicodeEscape = interp.PrintfUnicodeEscapeAbsent
	s.PrintfBEscEscape = interp.No
	s.PrintfBCapitalEscEscape = interp.No
	// `printf '%b\n' 'a\101b'` is `aAb`, so the octal needs no `\0`.
	s.PrintfBOctalWithoutZero = interp.Yes
	// And what it stopped is not padded out: `printf "%b|" "a\c"` is `a` and
	// nothing after it.
	s.PrintfBStopIsPadded = interp.No
	// `[ a == a ]` is 0, so `test` takes the doubled operator beside `=`.
	s.TestAcceptsDoubleEqual = interp.Yes
	// `set -o` lists `pipefail`, which dash's does not have.
	s.PipefailOption = interp.Yes
	// A frozen name in a command prefix is refused before anything else
	// happens: `unset u; readonly x=1; x=${u:=set} /bin/true` is `x: is read
	// only` and leaves `u` unset, so neither the value nor the command was
	// reached. dash answers the other way (#1943).
	s.PrefixToAFrozenNameIsCheckedFirst = interp.Yes
	// `printf 'a\cb'` writes `a` and stops, where dash writes the letter.
	s.PrintfBackslashC = interp.PrintfBackslashCStops
	// `printf '%ld\n' 5` is 5, so the length modifiers are read rather than
	// refused as conversions.
	s.PrintfLengthModifiers = interp.PrintfLengthModifiersC89
	// No `%q` and no `%(fmt)T`: both are `invalid format`.
	s.PrintfQuote = interp.PrintfQuoteAbsent
	s.PrintfTimeConversion = interp.No
	// An empty operand to a numeric conversion is reported, where dash reads
	// it as a zero and says nothing.
	s.PrintfEmptyIsNotANumber = interp.Yes
	s.PrintfReportsBadNumber = interp.Yes
	s.PrintfUnfinishedConversionIsAPercent = interp.No
	// `printf -v x '%s' hi` assigns nothing: `-v` is read as the format.
	s.PrintfAssignsWithV = interp.No
	s.PrintfRejectsUnknownOption = interp.No
	// `cd ""` and `HOME=; cd` both say nothing and report 0; `cd alpha beta`
	// goes to alpha and says nothing; `cd -` prints where it went.
	s.CdWithoutHomeIsAnError = interp.No
	s.CdEmptyOperandIsAnError = interp.No
	s.CdEmptyHomeIsAnError = interp.No
	s.CdSubstitutesTheOperands = interp.No
	s.CdSubstitutionPrintsTheDirectory = interp.No
	s.CdRefusesExtraOperands = interp.No
	s.CdRefusesUnknownOption = interp.Yes
	s.CdHasQuietOption = interp.No
	s.CdLastPathOptionWins = interp.Yes
	s.CdDashPrintsTheDirectory = interp.Yes
	// `umask` prints four digits, and `umask -S` prints the symbolic form.
	s.UmaskPrintsFourDigits = interp.Yes
	s.UmaskSetWithSPrints = interp.No
	s.SymbolicMaskTakesMoreThanOneOperator = interp.Yes
	s.SymbolicMaskWhoAloneSetsIt = interp.No
	s.SymbolicMaskTakesTheSetuidLetter = interp.Yes
	s.SymbolicMaskTakesTheStickyLetter = interp.No
	// `shift -1` is `Illegal number: -1`, so every dash word is read as the
	// count and there is no `--` to end options with.
	s.ShiftOptionWords = interp.ShiftOptionWordsNone
	s.ShiftDoubleDashEndsOptions = interp.No
	s.ShiftNamesAreArrays = interp.No
	s.ShiftNegativeIsOutOfRange = interp.No
	s.ShiftCountIsArithmetic = interp.No
	// `command -Z true` is `illegal option -Z` at 2.
	s.CommandRejectsUnknownOption = interp.Yes
	s.GetoptsRejectsUnknownOption = interp.No
	s.GetoptsAssignmentRestartsWord = interp.Yes
	s.GetoptsClearsOptarg = interp.No
	// `alias` reads no options at all, so `-p` is a name it cannot find and
	// `-g` and `-s` are neither kinds nor letters.
	s.AliasParsesOptions = interp.No
	s.AliasHasPrintOption = interp.No
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
	// `type` has no `-t` that names a kind, and `type -- cd` reads the `--`
	// as a name rather than as the end of options.
	s.TypePrintsFunctionBody = interp.No
	s.TypeEndsOptionsWithDashDash = interp.No
	s.TypeNamesTheKindWithDashT = interp.No
	// `ulimit -a` is laid out with bash's labels and letters rather than
	// dash's; the block unit and the resources it knows were read off that
	// listing.
	s.UlimitBlockIsKilobyte = interp.No
	s.UlimitHasResidentSet = interp.Yes
	s.UlimitHasProcessCount = interp.Yes
	s.UlimitSetsBothLimits = interp.Yes

	// ---- traps, signals and jobs ----

	// `trap 'echo t' SIGINT` is accepted, where dash takes only the bare
	// name.
	s.SIGPrefixAccepted = interp.Yes
	// `trap 'echo e' ERR` is accepted too — the sole holdout on the
	// pseudo-conditions is dash, and this shell is not with it.
	s.TrapHasErrCondition = interp.Yes
	s.TrapHasDebugCondition = interp.No
	s.TrapHasReturnCondition = interp.No
	// `trap` reads options and has none of them: `-p` and `-l` are both
	// `illegal option`.
	s.TrapParsesOptions = interp.Yes
	s.TrapPrintsWithP = interp.No
	s.TrapPrintsBareWithConditions = interp.No
	s.TrapPrintsBareWithP = interp.No
	s.TrapListsSignalsWithL = interp.No
	s.TrapOneArgumentIsACondition = interp.Yes
	s.TrapReportsAnUnknownSingleCondition = interp.Yes
	s.TrapSingleUnknownConditionIsUsage = interp.No
	s.TrapActionIsParsedWhenSet = interp.No
	s.TrapParseFailureNamesWhereItFired = interp.No
	s.TrapBodyRunsWhatParsed = interp.Yes
	// A subshell's listing shows only what survived the entry, which is the
	// POSIX answer and is measured rather than assumed.
	s.SubshellKeepsTrapListing = interp.No
	s.PipelineElementKeepsTrapListing = interp.No
	s.BackgroundJobKeepsTrapListing = interp.No
	s.SubshellHidesInheritedIgnoredTraps = interp.No
	// An EXIT trap does not run when a signal kills the shell: `trap 'echo
	// bye' EXIT; kill -TERM $$` writes nothing and dies at 143.
	s.ExitTrapRunsOnSignalDeath = interp.No
	s.QuitIgnoredWhenNotInteractive = interp.No
	s.HangupIsAnOrderlyExit = interp.No
	s.ExitInTrapReportsEarlierStatus = interp.Yes
	s.KillListAcceptsName = interp.Yes
	s.KillStatus = interp.KillStatusAnyFailure
	s.SubshellJobTable = interp.SubshellJobsCleared
	// A job started with `&` reads an empty standard input: `ash -c
	// '/bin/cat & wait; echo ---' < f` writes `---` and nothing else.
	s.BackgroundJobInput = interp.BackgroundJobInputEmpty
	// `set -u; echo "[$!]"` before any background command is `!: parameter
	// not set` at 2, so the parameter is unset rather than zero.
	s.LastBackgroundPidIsUnsetBeforeAnyJob = interp.Yes
	s.LastBackgroundPidIsZeroBeforeAnyJob = interp.No
	// A `jobs` listing keeps a job that has already ended, and shows the
	// `&`-started command's own text.
	s.JobsListNewestFirst = interp.Yes
	// StoppedJobTakesTheCurrentJobMarker is deliberately left unanswered.
	// The measurement wants a ^Z at a pseudo-terminal, which is exactly what
	// the container this shell was measured in could not give it, so there
	// is nothing recorded to put here — and the package comment above says
	// to read that silence as an unanswered question rather than as
	// agreement with dash. The axis is read and never asked, so an
	// unanswered dialect gets the answer five of the six measured columns
	// give rather than a refusal (#1563).
	s.JobsListFinishedJobs = interp.Yes
	s.JobsOptions = "lp"
	s.JobsPidsOnlyOption = interp.Yes
	s.JobsShowBackgroundCommand = interp.No
	s.AnnouncesBackgroundJob = interp.No
	s.AnnouncesBackgroundJobWithoutTheMonitor = interp.No
	s.ReportsACommandKilledBySignal = interp.Yes
	s.ReportsAnyKilledPipelineElement = interp.Yes
	s.ReportsAKilledCommandInACommandSubstitution = interp.Yes
	s.ChildInterruptEndsTheScript = interp.No
	s.SubshellRunsOnAfterSignalingTheShell = interp.Yes
	// Only numbers and the `%%`, `%+`, `%-` forms resolve; `%name` is a job
	// that is not there, and `wait`, `jobs`, `fg` and `bg` each say so.
	s.JobSpecsByName = interp.No
	s.WaitReadsOptions = interp.Yes
	s.WaitReportsAMissingJob = interp.Yes
	// `wait -n` is 127 rather than a wait, so the letter is not an option
	// here.
	s.WaitNWaitsForTheNextJob = interp.No
	s.WaitForAJobFailsWhenInterrupted = interp.No

	// ---- control flow and redirection ----

	// `break` in a function reaches the caller's loop: `f(){ break; }; for i
	// in 1 2; do f; echo "i=$i"; done` writes nothing at all, where dash
	// writes both rounds. A call is not a boundary here and a subshell is,
	// which is the opposite pairing from dash's on the first half.
	s.FunctionCallIsALoopControlBoundary = interp.No
	s.SubshellIsALoopControlBoundary = interp.No
	// `break` with no loop around it is ignored, silently: `echo t; break;
	// echo after` prints both and ends at 0.
	s.LoopControlOutsideALoopIsFatal = interp.No
	s.ReturnOutsideAFunctionIsRefused = interp.No
	s.StartupFileReturnCarriesItsArgument = interp.Yes
	// `. ` with no operand is refused at 2, where dash does nothing and
	// reports success. A directory operand is no error in either.
	s.DotWithNoOperandIsAnError = interp.Yes
	s.DotDirectoryOperandIsAnError = interp.No
	// Only the last of several targets is used: `echo hi >a >b` leaves a
	// empty.
	s.RedirectsUseEveryTarget = interp.No
	// A redirection a special builtin cannot make ends the script: `exec
	// 3>/nope/x` stops at 1.
	s.RedirectErrorOnSpecialBuiltinFatal = interp.Yes
	s.DuplicationTargetErrorOnABuiltinIsFatal = interp.No
	// A duplication target wider than one digit is not refused while
	// parsing: `echo hi >&10` reaches the kernel and comes back `dup2(10,1):
	// Bad file descriptor`, where dash refuses the word outright.
	s.MultiDigitDuplicationTargetIsAnError = interp.No
	// And `>&word` names a file rather than being refused: `f=/tmp/gw; echo
	// hi >&$f` writes the file. dash is the panel's holdout on this.
	s.GreatAmpTarget = interp.GreatAmpTargetNamesAFile
	s.RedirectTargetIsAnOrdinaryWord = interp.No
	// A here-document body and a redirection target are both expanded in the
	// shell rather than in the process the redirection is for, so what they
	// assign is still there afterwards: `cat /dev/null > "${u:=made}"` leaves
	// `u` set.
	s.HeredocExpandsInTheCommandsProcess = interp.No
	s.RedirectTargetExpandsInTheCommandsProcess = interp.No
	// A descriptor number the process cannot hold is not checked before the
	// open.
	s.FdNumberBoundedByOpenFileLimit = interp.No
	// `[[ ]]` is a builtin here rather than a keyword — `type '[['` answers
	// `[[ is a shell builtin` — so an unknown option inside it is a status
	// rather than a parse failure.
	s.UnknownConditionOptionIsAStatus = interp.Yes
	s.UnsetFunctionChecksTheName = interp.No
	s.UnsetFunctionReportsMissing = interp.No
	s.StdinProgramReadInBlocks = false
	s.StdinOptionNamesTheOperands = interp.No
	s.LoneDashIsAnOption = interp.No
	// A lone `+` is a name to `export`: `export +` is `+: bad variable name`.
	s.SignAloneIsAnOptionWordToExport = interp.No
	s.ReadTrailingEscapedSeparator = interp.ReadTrailingEscapedSeparatorTrimmed
	s.ReadTrailingWhitespaceEndsAField = interp.No
	s.ReadNoFieldsIsOneEmptyElement = interp.No
	s.ReadRefusesABadNameBeforeReading = interp.No
	s.BadNameDeclaresTheOperandsAfterIt = interp.No
	s.InteractiveSelectsEmacs = interp.No

	// ---- axes this dialect did not answer, swept for and measured ----
	//
	// #2272 was reported as one row — `read -t` refusing in the shipped
	// binary — and the row was the symptom of a shape. A `Semantics` axis
	// added after a dialect is written is unanswered there, and an
	// unanswered axis *refuses at run time* while `go test ./...` stays
	// green, so the whole set is swept rather than the one that was
	// noticed: every corpus snippet was run through this shell's binary and
	// every "no dialect was chosen" collected, which found nineteen sites
	// and not one — and then twenty, because answering one uncovers the
	// next question on the same path, so the sweep was re-run until it
	// stopped moving.
	//
	// Measured the same way the rest of this file was: BusyBox v1.37.0's
	// `/bin/ash` in the `alpine:3` image, 2026-09-12, each probe being the
	// corpus case that reaches the axis. The three the sweep found and this
	// block does *not* answer are named at the end, with what they answered
	// and why a value cannot be written yet — read those as open questions,
	// which is what docs/spec/ash.md says the absence of a measurement
	// means here.

	// `echo -e -E 'm\tn'` writes a tab, and so does `echo -E -e 'm\tn'`:
	// `-e` wins whichever way round the two are written, so the last flag
	// does not decide. bash is the panel's shell that lets it (#2272).
	s.EchoLastEscapeFlagWins = interp.No

	// `read -t 0` polls. `printf "a\nb\n" > f; exec < f; read -t 0 v` is
	// status 0 with `v` empty and the *next* `read` still finds `a`, so it
	// answered whether input was waiting and consumed none of it; on an
	// empty file it is status 0 as well, the end of a stream being ready to
	// a shell that only asks. bash's reading (#2272).
	s.ReadZeroTimeout = interp.ReadZeroTimeoutPolls
	// And a non-zero `-t` bounds the whole read rather than the wait for the
	// first byte. Measured with the case the axis was written for: a byte
	// into a fifo at once and the rest of the line 0.4s later, under `read
	// -t 0.1`, is status 1 with the variable left alone — where zsh answers
	// 0 with the whole line. This is the row #2272 was filed on (#644).
	s.ReadTimeoutBoundsReadability = interp.No
	// An expired `-t` touches no name, so the variable keeps what it held —
	// and this axis only became reachable once the one above was answered,
	// which is the layering the sweep had to be re-run to see. Measured with
	// the probe ReadTimeoutKeepsWhatArrived documents, half a line and then a
	// stall: `{ printf part; sleep 0.5; printf 'ial\n'; } | { v=old; read -t
	// 0.2 v; echo "$? [$v]"; }` is `1 [old]` here, against bash 5.3's `142
	// [part]`. ksh93's row (#2272).
	s.ReadTimeoutKeepsWhatArrived = interp.No
	// A count does not stop `read` judging the names after the first:
	// `printf 'XYZW\n' | read -n 3 a 1bad b` complains `read: '1bad': bad
	// variable name` at 1, exactly as it does without the count, and `-n 3`
	// really is a count here — `read -n 3 v` of `abcdef` leaves `abc`. ksh93
	// is the shell a count quiets (#2272).
	s.ReadCountJudgesTheNamesAfterTheFirst = interp.Yes

	// `set -e` stops for a failure only pipefail saw: `set -eo pipefail;
	// false | true; echo reached` reaches nothing and the shell ends at 1,
	// where a plain `set -e; false | true` reaches the echo. ksh93 is the
	// column that runs on (#2272).
	s.ErrexitSeesPipefailFailure = interp.Yes
	// The status pipefail hands back for an element a signal killed is the
	// ordinary 128-plus-the-signal and not the bare number: a `{ echo "$v";
	// } | true` over a value grown past the pipe buffer is 141, and so is
	// `yes | head -1`. ksh93's 13 is the other answer (#2272).
	s.PipefailSubstitutesTheBareSignal = interp.No

	// `${1:=abc}` does not assign to a positional: after `set --` it is `1:
	// bad variable name` and the script ends. zsh alone assigns (#2272).
	s.AssignThroughExpansionMayNameAPositional = interp.No
	// A quoted replacement operand's own quotes quote, and are removed:
	// `s=xay; v=VAL; echo "${s/a/'$v'}"` is `x$vy` rather than `x'VAL'y`, so
	// the single quotes kept `$v` from expanding and then went. The
	// backslash row agrees — `\q`, `\{`, `\\`, `\"`, `\}` and `\$v` all lose
	// the backslash — which is bash 5.3's and ksh93's reading (#2272).
	s.ReplacementOperandTakesTheEnclosingQuoting = interp.No
	// An empty pattern in a span replacement matches nothing, whatever the
	// value holds: `v=abc; e=` makes `${v///X}`, `${v//$e/X}`, `${e///X}`
	// and `${e//x/X}` come to `abc`, `abc`, empty and empty. ksh93 replaces
	// in the third and zsh in every position (#2272).
	s.EmptyReplacementPattern = interp.EmptyReplacementPatternMatchesNothing

	// `$'\q\8'` keeps both characters, which is bash's answer and not the
	// dropping one ksh93 and zsh share (#2272).
	s.DollarSingleUnknownEscape = interp.DollarSingleUnknownKeepsBackslash
	// There is no `\c` inside `$'…'` at all: `$'\cA\cz'` is the six
	// characters as written. zsh is the other column with none (#2272).
	s.DollarSingleBackslashC = interp.DollarSingleControlAbsent
	// And no caret or meta spelling either: `$'\C-A'`, `$'\CA'`, `$'\M-x'`
	// and `$'\M-\C-?'` are all kept as written, so the escape vocabulary
	// this shell's `$'…'` has is neither zsh's nor ksh93's (#2272).
	s.DollarSingleCaretMeta = interp.No

	// `$((2**-1))` is `exponent less than 0` — no float answer, which is
	// bash's side of the split (#2272).
	s.ArithNegativeExponentIsError = interp.Yes

	// A declaration does not shadow a readonly: `readonly x=1; f() { local
	// x=2; }; f` is `local: line 1: x: is read only` and the script ends,
	// which is dash's and bash's answer rather than ksh93's and zsh's
	// (#2272).
	s.DeclarationMayShadowAReadonly = interp.No
	// A valueless declaration of a name its own scope already holds lists
	// nothing: `f() { local v=1; local v; }` is silent at 0, and the value
	// stays — `local FOO=x; local FOO` still reads `x`. zsh is the column
	// that lists (#2272).
	s.ValuelessDeclarationOfAHeldNameListsIt = interp.No

	// `jobs -p` does not finish a job the way a state listing does: after a
	// background `sleep` has ended, `jobs -p` prints the id and the next
	// bare `jobs` still reports it `Done`. ksh93 is the column that forgets
	// it there (#2272). The plain `jobs -p` in the issue answered 0 for
	// this reason — the axis is only reached once there is a finished job to
	// list.
	s.PidListingFinishesWithAJob = interp.No

	// Three the sweep reached and this file deliberately leaves unanswered,
	// each with what BusyBox answered and what stands in the way of writing
	// it down. None is a guess deferred; each is a measurement the vector
	// cannot yet hold.
	//
	//   DollarSingleNulTruncates — a *third* reading (#2276). `x=$'a\0b'` leaves
	//   `ab` at length 2: the NUL is neither kept (zsh, length 3) nor the
	//   end of the span (bash and ksh93, length 1) but dropped, and the
	//   octal and hex spellings agree. `Answer` has no room for it, so a
	//   value here would have to be one of the two wrong ones.
	//
	//   ReadonlyRecordsTheCompoundAttribute — this shell has no letter to
	//   ask it with (#2277). `readonly -a a` is `readonly: illegal option -a` and
	//   there is no `typeset` at all. The refusal our binary reaches is
	//   `readonly`'s option set being fixed in the interpreter rather than
	//   the dialect's, which is a gap ksh93 has today for the same reason —
	//   it refuses the same corpus row, on `main`, for want of the same
	//   letter.
	//
	//   Diagnostics.UlimitListing — measured in full (#2278: fifteen rows, `core
	//   file size (blocks)         (-c) unlimited` and its fellows), and
	//   five of them — `-e`, `-i`, `-q`, `-r`, `-x` — name resources no
	//   [interp.Resource] constant does. It was also measured on Linux,
	//   where those five exist; the table a macOS build should print is a
	//   second measurement and not this one.

	return s
}

// Diagnostics is how ash reports failure.
//
// Two shapes run through the whole table and are worth reading first. A
// message the shell itself speaks carries no line under `-c` and a `line N`
// in a script — `ash: nosuchcmd: not found` against `s.sh: line 3:
// nosuchcmd: not found` — which is Location and ScriptLocation below. And the
// wordings are lower-case where dash's are capitalized: `syntax error:
// unexpected "("` against `Syntax error: "(" unexpected`, which is the single
// largest family of differences between the two siblings and accounts for
// 458 rows of the corpus on its own.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		// Measured on `-c`, on a script file and on standard input: only the
		// script route names a line. `ash -c 'echo a; nosuchcmd'` is `ash:
		// nosuchcmd: not found` however deep the failure is, and the same two
		// lines in a file are `s.sh: line 2: nosuchcmd: not found`.
		Location:       interp.LocationNameOnly,
		ScriptLocation: interp.LocationLineWord,
		StdinLocation:  interp.LocationNameOnly,
		// A sourced file and an `eval` are both named between the shell and
		// the line, which is bash's and ksh93's placing rather than dash's:
		// `ash: ./p.sh: line 3: NOPE: parameter not set`, and `ash: eval:
		// line 0: nosuchcmd: not found`.
		SourceFileNaming: interp.SourceBeforeLocation,
		EvalNaming:       interp.SourceBeforeLocation,

		// The parse failures, in this shell's own order: the complaint first
		// and the token after it, all lower case.
		SyntaxUnexpected:         `syntax error: unexpected "%[1]s"`,
		SyntaxUnexpectedWord:     "syntax error: unexpected word",
		SyntaxUnexpectedNewline:  "syntax error: unexpected newline",
		SyntaxRedirectUnexpected: "syntax error: unexpected redirection",
		SyntaxExpecting:          ` (expecting "%[1]s")`,
		SyntaxError:              "syntax error: %[1]s",
		SyntaxErrorStatus:        2,
		ForName:                  "syntax error: bad for loop variable",
		Unterminated:             `syntax error: unexpected end of file (expecting "%[4]s")`,
		UnterminatedNoConstruct:  "syntax error: unexpected end of file",
		UnmatchedQuote:           "syntax error: unterminated quoted string",
		UnmatchedBackquote:       "syntax error: unterminated quoted string",
		UnmatchedCmdSubst:        `syntax error: unexpected end of file (expecting ")")`,
		UnmatchedBraceSubst:      "syntax error: missing '}'",
		UnmatchedArithSubst:      "syntax error: missing '))'",
		BadSubstitution:          "syntax error: bad substitution",

		// Arithmetic says one thing about every way an expression can be
		// wrong: `$((1 2))`, `$((1+))`, `$((08))`, `$(('a'))` and `$((0b101))`
		// are all `arithmetic syntax error`. So the reason is the whole of the
		// message and the expression is never quoted back, which is what the
		// bare `%[2]s` says.
		ArithError:            "%[2]s",
		ArithOperandExpected:  "arithmetic syntax error",
		ArithOperatorExpected: "arithmetic syntax error",
		DigitTooGreatForBase:  "arithmetic syntax error",
		ArithConditionalColon: "arithmetic syntax error",

		// The command-resolution family. Neither a name nor a line in front
		// of the `not found`, which is the shape ksh93 uses too.
		TypeKeyword:            "%[1]s is a shell keyword",
		TypeFunction:           "%[1]s is a function",
		TypeAlias:              "%[1]s is an alias for %[2]s",
		CommandVAlias:          "alias %[1]s=%[2]s",
		TypeNotFound:           "%[1]s: not found",
		TypeNotFoundUnprefixed: true,
		TypeNotFoundOnStdout:   true,
		CommandVNotFound:       "%[1]s: not found",
		TypeNotFoundStatus:     127,
		CannotExecute:          "%[1]s: %[2]s",
		ExecCannotExecute:      "%[1]s: %[2]s",
		ExecNotFound:           "%[1]s: not found",
		DirectoryReason:        "Permission denied",
		DirectoryOnPathStatus:  127,

		// `.` and the files it reads. The quotes around the name are this
		// shell's and are not decoration: `. nosuchfile` is `ash: .: line 0:
		// can't open 'nosuchfile': No such file or directory`, with the OS's
		// own text after the colon rather than dash's truncation of it.
		DotCannotOpen: ".: can't open '%[1]s': %[2]s",
		DotNotFound:   ".: %[1]s: not found",

		// Redirection. One verb each way, and the OS's reason reworded: an
		// open that finds nothing is `no such file`, a create that cannot
		// make one is `nonexistent directory`.
		CannotOpen:            "can't open %[1]s: %[2]s",
		CannotCreate:          "can't create %[1]s: %[2]s",
		FileNotFound:          "no such file",
		DirectoryNotFound:     "nonexistent directory",
		RedirectFailureStatus: 1,

		// The declarations. One wording for all of them, naming the part in
		// front of any `=`.
		BuiltinBadName: map[string]string{
			"export":   "%[2]s: bad variable name",
			"readonly": "%[2]s: bad variable name",
			"unset":    "%[2]s: bad variable name",
			"local":    "%[2]s: bad variable name",
			"read":     "read: '%[2]s': bad variable name",
		},
		BuiltinBadNameStatus:          2,
		ReadonlyVariable:              "%s: is read only",
		ReadonlyVariableInDeclaration: "%[1]s: is read only",
		UnsetReadonly:                 "%s: is read only",
		LocalOutsideAFunction:         "not in a function",

		// The option refusals: lower case, and the letter alone.
		SetInvalidOptionName:   "illegal option -o %[1]s",
		SetInvalidOptionLetter: "illegal option -%[2]s",
		BuiltinBadOption:       "illegal option %[2]s",
		OptionNeedsArgument:    "%[1]s: No arg for -%[2]s option",
		UlimitBadOption:        "unrecognized option: %[1]s",
		UmaskBadOption:         "illegal option %[1]s",
		PrintfBadOption:        "illegal option %[1]s",

		// Numbers. The capital is this shell's, on a message it otherwise
		// words like the lower-case ones around it.
		InvalidNumber:   "Illegal number: %s",
		NumericArgument: "%[1]s: Illegal number: %[2]s",
		ShiftBadNumber:  "Illegal number: %[1]s",
		UmaskBadMask:    "illegal mode: %[1]s",
		// The whole operand quoted back, with no word for what in it was
		// wrong — the same message a numeric mask gets.
		UmaskBadSymbolicMode: "illegal mode: %[1]s",
		UmaskBadMaskStatus:   2,
		UlimitBadNumber:      "bad number",

		// The jobs family: the spec first and the sentence after it, which is
		// the reverse of dash's order.
		NoSuchJob:           "%[2]s: no such job",
		NoSuchJobStatus:     2,
		KillNoSuchJob:       "%[1]s: no such job",
		WaitNoSuchJob:       "%[1]s: no such job",
		WaitNoSuchJobStatus: 2,

		// `kill`. The applet-level messages carry neither a line nor a
		// builtin, which the unprefixed flags say.
		KillNoSuchProcess:   "can't kill pid %[1]s: No such process\n",
		KillInvalidSignal:   "bad signal name '%[1]s'",
		KillIllegalOption:   "illegal option -%[2]s",
		KillNotAPid:         "Illegal number: %[1]s",
		KillUsageStatus:     2,
		KillBadOptionStatus: 2,
		KillArgumentStatus:  2,
		KillListing:         interp.KillListingZeroFirst,

		// `getopts` names nothing in front of its two complaints, as dash
		// does.
		GetoptsBadOption:       "Illegal option -%[1]s",
		GetoptsMissingArgument: "No arg for -%[1]s option",
		GetoptsUnprefixed:      true,

		// `cd`, with the OS's reason where dash gives none.
		CdCannotChange: "can't cd to %[1]s: %[2]s",
		CdStatus:       2,

		// `test`. The middle word is blamed rather than the first — `[ a b c
		// ]` is `b: unknown operand` — and an unknown operator leaves an
		// operand behind, so the same wording names the word after it.
		TestNamesFirstOperand:   false,
		TestUnknownLongOperator: interp.TestUnknownOperatorLeavesAnOperand,
		TestUnaryExpected:       "%[1]s: unknown operand",
		TestBinaryExpected:      "%[1]s: unknown operand",
		TestIntegerExpected:     "%[1]s: out of range",
		TestTooManyArguments:    "unknown operand",
		TestOperandExpected:     "argument expected",
		TestMissingBracket:      "missing ]",

		// The remarks a shell with no terminal makes about job control, both
		// spellings of the same sentence.
		MonitorDenied:         "can't access tty; job control turned off",
		NoJobControlAtStartup: "can't access tty; job control turned off",

		// `alias -g` is a name this shell cannot find, said with nothing in
		// front of it at all — no shell, no line.
		AliasNotFound:             "%[1]s: %[2]s not found",
		AliasNotFoundUnprefixed:   true,
		UnaliasNotFound:           "%[1]s: %[2]s not found",
		UnaliasNotFoundUnprefixed: true,
		// `set -o` prints a name and its state in two columns and no header,
		// where dash writes `Current option settings` over its own.
		OptionListingWidth: 16,

		ParamNullOrNotSet: "parameter not set or null",
		ShiftTooMany:      "can't shift that many",
		TimesDecimals:     3,
		JobRunning:        "Running",
		JobDone:           "Done",
		JobExited:         "Done(%[1]d)",
		PrintfBadNumber:   "invalid number '%[1]s'",
		PrintfBadVerb:     "%[2]s: invalid format",
		PrintfMissingVerb: "%[1]s: invalid format",
	}
}

// Apply makes any adjustment that is not a vector value.
//
// Every call here is a builtin this shell does not have, checked by asking
// it: `typeset`, `declare`, `disown`, `mapfile`, `readarray`, `compgen`,
// `complete`, `builtin` and `enable` are each `not found`. `let` is *not*
// among them — `let "x=1+1"` sets x to 2 here — which is the one place this
// shell keeps a builtin dash gives up.
func Apply(r *interp.Runner) {
	// The prompt table, installed for the same reason the other dialects
	// install theirs: what a shell does to a prompt parameter before drawing
	// it is the dialect's answer, and "nothing but expansion" is an answer
	// rather than an absence.
	r.SetPromptStyle(PromptStyle())
	r.Unregister("typeset")
	r.Unregister("declare")
	r.Unregister("disown")
	r.Unregister("mapfile")
	r.Unregister("readarray")
	r.Unregister("compgen")
	r.Unregister("complete")
	r.Unregister("builtin")
	r.Unregister("enable")
	// `fc` is an external here, as it is for dash: `command -v fc` resolves a
	// path rather than naming a builtin.
	r.Unregister("fc")
}
