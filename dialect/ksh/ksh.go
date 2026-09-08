// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package ksh answers the substrate's questions the way ksh93 does.
package ksh

import (
	"context"
	"fmt"
	"strconv"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what ksh93 parses.
func Dialect() syntax.Dialect {
	d := syntax.Core()
	// ksh93 expands them in a script too.
	d.ExpandAliases = syntax.AliasOnEveryRoute
	// And a body's newlines are lines of the program: `$LINENO` after a
	// two-line body reads one more than the physical line.
	d.AliasBodyCountsLines = true
	// ksh93 has neither `local` nor `declare`, so `local a=(x)` is the same
	// syntax error there that `echo a=(x)` is — the rule follows the name
	// into the shell that has it.
	d.DeclarationUtilities = map[string]bool{
		"typeset": true, "export": true, "readonly": true,
	}
	d.ParamIndirection = true
	// A single `;` written where a command belongs is stepped over — `; b`,
	// `a ; ; b`, `a & ; b`, `a |& ; b`, `a && ; b` and `a || ; b` all run —
	// and it is stepped over rather than standing in for anything: `false ||
	// ; echo two` prints `two` where `true || ; echo two` prints nothing, so
	// the command after it is the operator's own right-hand side.
	//
	// Both limits are measured, and both separate this shell from zsh, which
	// takes every one of them: a *second* one is refused, `a || ; ; b` being
	// `` `;' unexpected ``, and it is refused after a **bar**, `a | ; b`
	// being the same — while `a |& ; b` is taken, because that spelling
	// terminates a command here rather than joining two (#1141). That last
	// pair is the same asymmetry #1115 found, and it is what says the bar is
	// a separate question from the and-or rather than one rule about control
	// operators.
	d.SeparatorWhereACommandBelongs = syntax.OneSeparatorExceptAfterABar
	// And where a separator was stepped over and nothing came after it at
	// all, an empty command stands there and succeeds: `false || ;` answers
	// 0 here and 1 in zsh, which drops the operator instead. The `&&` row
	// agrees in both — 1 either way, because the left-hand side failed — so
	// the `||` is the only shape that tells the two readings apart.
	d.AbsentAndOrOperandIsAnEmptyCommand = true
	// A loop's variable may be written with quoting or an escape in it, and
	// the quoting comes off before the word is read as a name: `for "i" in a
	// b` binds `i` here and is refused by the other four. `for 'i'`,
	// `for i""`, `for "i"x` and `for \i` too, so the escape travels with the
	// quotes. A name out of an *expansion* is refused here as everywhere,
	// which is why this is the quoting half alone (#1076).
	d.ForNameMayBeQuoted = true
	// And a word that is not a name at all parses, the complaint coming when
	// the loop is reached — so `ksh -n` accepts a script this used to refuse.
	// interp.Semantics.ForNameWhenTheLoopRuns is what happens then (#1110).
	d.ForNameCheckedWhenTheLoopRuns = true
	// A colon written before a trim is ignored here: `${v:#hel*}` is
	// `${v#hel*}` and comes to `lo`, where zsh reads the same six characters
	// as an element exclusion and bash refuses them as arithmetic. All four
	// trims, measured — `:#`, `:##`, `:%` and `:%%` — and nothing else: the
	// replacement, the case changes and a colon with a space after it stay
	// arithmetic errors, and `${v:2}` is still an offset.
	d.ParamColonBeforeTrimIsIgnored = true
	// A function body that is not compound may carry no redirection here:
	// `f() echo hi` runs and `f() >out`, `f() echo hi >out` and `f() x=1
	// >out` are all a syntax error at the operator. A braced body is not
	// this rule — `f() { :; } >out` is accepted.
	d.FuncBodyTakesNoRedirection = true
	// `$"..."`, the locale-translatable string: with no catalog it is a
	// plain double-quoted string with the `$` stripped. Not core, because
	// dash and zsh keep the `$` as a literal.
	d.DollarDoubleQuote = true
	// ksh93 alone refuses an unrecognized ${...} operator while reading the
	// script; the other three wait until the expansion is reached.
	d.BadSubstitutionAtParseTime = true
	// `${ cmd;}`, a command substitution that runs in the current shell
	// so that what it assigns survives. The space after the brace is the
	// whole of the grammar: `${x}` is a parameter and `${ x}` is not.
	d.CurrentShellSubstitution = true
	// A here-document inside parentheses that hold a program ends at the
	// closing one: `v=$(cat <<EOF` / `a` / `EOF)` is accepted and `v` is `a`.
	// dash and zsh read the body from the whole input instead, so the `)`
	// goes into it and the construct is never closed (#963).
	d.HeredocEndsAtClosingParen = true
	// The measured ksh93 is 93u+ 2012, which has no `&>`. Later ksh93u+m
	// does, twelve years apart under the same name — which is the divergence
	// syntax.Dialect's own comment names as the reason its fields are called
	// after constructs rather than after shells. A dialect built for the
	// newer build sets this back to true; the panel measures the older one.
	d.AmpersandRedirect = false
	// `times` is a reserved word here, so `times foo` is a syntax error
	// rather than a builtin ignoring an argument — the one place in the panel
	// where which builtin a shell has changes what parses.
	d.TimesIsReserved = true
	// Floating point, which POSIX has not and these two do.
	d.ArithFloat = true
	// Extended patterns wherever a pattern may stand.
	d.ExtendedPattern = true
	// And inside `[[ ]]`, which is the only place bash reads them.
	d.ExtendedPatternInCondition = true
	// `[[ -v name ]]`. Measured on ksh93u+, which has the operator and reads
	// fewer kinds of name through it than the other two; see
	// interp.Semantics.ParameterIsSetSeesPositionals.
	d.ParameterIsSetTest = true
	// A bare `|` in a `=~` operand belongs to the regular expression.
	d.RegexTakesAlternation = true
	// `cmd |&` is this shell's coprocess: an operator that terminates a
	// command, not the pipe carrying both streams that bash and zsh spell
	// the same way. PipeBothStreams stays off, which is what keeps the two
	// readings from ever both applying — see syntax.Dialect.
	d.CoprocPipeOperator = true
	// `time -p`, the POSIX report format, which bash also reads and zsh
	// does not.
	d.TimePosixFlag = true
	// The end of input closes a quote here: `echo "abc` prints abc, and the
	// old backquote form behaves the same way. The substitutions do not.
	d.CloseQuotesAtEOF = true
	// `exec {a[1]}>&-`: the name inside the braces may be a subscripted one.
	// Measured — this shell closes the descriptor the element holds, as bash
	// does and zsh does not.
	d.FdVariableSubscript = true
	return d
}

// Semantics is what ksh93 means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	// An unquoted list is its elements taken one at a time, never their
	// join: `IFS=:; set -- "x:" y; printf "[%s]" $@` is `[x][y]` here and
	// `[x][][y]` in bash, which joins to `x::y` first.
	s.UnquotedListJoinsOnIFS = interp.No
	// An associative array's subscript is a quoting context here, as it is
	// in bash: `m["k"]=W` stores under `k`.
	s.SubscriptIsAQuotingContext = interp.Yes
	// bash's answer on the other join, against its own on the one above:
	// `IFS=-; a=(x y z); v=${a[@]}` is `x y z` here where zsh gives
	// `x-y-z`. The two axes partition the panel differently, which is why
	// neither can stand in for the other.
	s.UnsplitAtListJoinsOnIFS = interp.No
	s.CommandNotFoundStatusIsNotFound = interp.No
	s.SetFTurnsOffGlobbing = interp.Yes
	// `-c` and `-s` together: `-s` names the operands here, so `sh -sc CMD
	// name a` keeps the shell in `$0` and makes both operands parameters.
	// bash and dash let the command string name them instead.
	s.StdinOptionNamesTheOperands = interp.Yes
	// And the sign of the `c` is not decoration here: `sh +c CMD name a`
	// keeps CMD in `$0` and makes both operands parameters, where the other
	// three read `+c` as `-c` and name `$0` from the first operand. All four
	// run the command string either way.
	s.PlusSignedCommandStringIsDollarZero = true
	// Measured from a script file, where `echo $-` reports `hB`. The
	// letters describing the route come from Runner.Route.
	s.DefaultOptionLetters = "hB"
	// And `imBE` for `-i script.sh`, which is why this axis replaces rather
	// than appends: the `h` is *gone*. Measured 2026-09-05 — `set -o`
	// reports `trackall on` for a script and off when interactive, which is
	// that letter, while `rc on` appears only when interactive, which is the
	// `E`.
	//
	// The `m` in that row is not written here and must not be: it is a
	// monitor that is really on — `set -o` says `monitor on` under `-i
	// script.sh` in this shell and off in the other two — so the letter has
	// to come from the runner or not at all. That this shell turns job
	// control on there and the front end does not is measured, recorded in
	// docs/spec/invocation.md, and a separate question.
	s.InteractiveOptionLetters = "BE"
	// Login-ness written out, in both spellings. Measured 2026-09-05 with a
	// scratch home directory: `ksh -l -c cmd` and `ksh --login -c cmd` each
	// read `~/.profile`, which is the file the POSIX preset already names.
	// The interactive file stays `$ENV` and the escape hatches stay empty —
	// this shell has none, so a startup file that breaks it is escaped by
	// moving the file, which is measured and is what it does.
	s.StartupFileOptions = interp.StartupFileOptions{Login: "-l --login"}
	// `ksh -c 'echo $-'` reports `chsB` — both route letters, where bash
	// shows `c` alone and dash and zsh show neither. Read down its rows and
	// ksh93's rule for `s` is "no script file was named" where the other
	// three's is "the program came from standard input"; this is the one
	// invocation where those two differ.
	s.CommandStringShowsCInDollarDash = interp.Yes
	s.LoginShowsLInDollarDash = interp.Yes
	s.CommandStringShowsSInDollarDash = interp.Yes
	s.ArithIntegerOperatorRefusesFloat = interp.Yes
	// A negative exponent is a float answer here, not a refusal: `2**-1`
	// is 0.5.
	s.ArithNegativeExponentIsError = interp.No
	s.ArrayScalarIsTheWholeArray = interp.No
	s.ArrayNameWithoutSubscriptIsTheList = interp.No
	// A subscript inside a literal is the text between the brackets, and a
	// literal written with one declares a keyed array: `typeset -p` answers
	// `-A` and `${a[2]}` does not find what `[1+1]=c` stored.
	s.ArrayLiteralSubscriptIsAKey = interp.Yes
	s.SelectLayout = interp.SelectMenuVertical
	s.SelectPromptNeedsTerminal = interp.Yes
	s.AliasParsesOptions = interp.Yes
	s.AliasHasPrintOption = interp.Yes
	// It complains about `alias nope` and says nothing about `unalias nope`,
	// which is why these are two questions.
	s.AliasReportsNotFound = interp.Yes
	s.UnaliasReportsNotFound = interp.No
	s.AliasNotFoundStatusCounts = interp.Yes
	s.UnaliasAllRefusesOperands = interp.No
	s.AliasQuoting = interp.ListingQuoteWhenNeededDollar
	s.TrapQuoting = interp.ListingQuoteWhenNeededDollar
	// `typeset -p` writes `typeset -x -r n=5` — separate flags — and a name
	// with no attributes as a bare `v=1`; a missing name is passed over in
	// silence, status 0, which is measured rather than a shortcut.
	s.DeclareListing = interp.DeclareListingBareAssignments
	s.DeclareValueQuoting = interp.ListingQuoteWhenNeededDollar
	s.ListingControlEscape = interp.ControlEscapeHex
	s.ExportListing = interp.DeclareListingCommandWord
	s.ReadonlyListing = interp.DeclareListingCommandWord
	// And the bare form is not the `-p` form here: the command word goes and
	// what is left is a plain assignment — `export` writes `V='a b'` where
	// `export -p` writes `export V='a b'`. Measured; zsh does the same.
	s.BareDeclarationListing = interp.DeclareListingPlainAssignment
	s.DeclarePrintReportsAMissingName = interp.No
	s.TrapActionIsParsedWhenSet = interp.No
	s.TrapParseFailureNamesWhereItFired = interp.Yes
	s.SymbolicMaskTakesMoreThanOneOperator = interp.Yes
	s.SymbolicMaskSetsWithoutAWho = interp.Yes
	s.SymbolicMaskWhoAloneSetsIt = interp.Yes
	s.SymbolicMaskTakesTheSetuidLetter = interp.Yes
	s.SymbolicMaskTakesTheStickyLetter = interp.Yes
	// Every dash word is an option here, digits and all: `shift -1` and
	// `shift -0` are both refused as options this shell does not have, which
	// is why a negative count is only reachable after the marker.
	s.ShiftOptionWords = interp.ShiftOptionWordsAny
	s.ShiftDoubleDashEndsOptions = interp.Yes
	s.ShiftNegativeIsOutOfRange = interp.Yes
	s.WaitReadsOptions = interp.Yes
	// Job specs by command text, a second match taken rather than refused.
	// A `wait` whose spec names nothing says nothing at all and reports 0;
	// there is no -n, and disown only shields a job from a HUP this engine
	// never forwards, so the listing keeps it.
	s.JobSpecsByName = interp.Yes
	s.AmbiguousJobNameIsRefused = interp.No
	s.WaitReportsAMissingJob = interp.No
	s.WaitNWaitsForTheNextJob = interp.No
	// The lone divergence on an interrupted wait: a bare one reports this
	// shell's own 256 plus the signal, and one that names a job — `wait $!`
	// or `wait %1` — reports a plain 1 instead.
	s.WaitForAJobFailsWhenInterrupted = interp.Yes
	s.DisownRemovesTheJob = interp.No
	// Both Yes, re-measured with a letter ksh93 does not own (-q): the first
	// probes used -x and -a, which are real ksh93 options, and recorded No
	// off ksh93's own features.
	s.CommandRejectsUnknownOption = interp.Yes
	s.GetoptsRejectsUnknownOption = interp.Yes
	s.ShiftCountIsArithmetic = interp.Yes
	s.TrapBodyRunsWhatParsed = interp.No
	s.ReportsAKilledCommandInACommandSubstitution = interp.Yes
	s.TrapBodyLine = interp.TrapBodyLineOffsetFromWhereItFired
	s.ExitTrapFiresPastTheEnd = interp.No
	s.SelectEofEndsPromptLine = interp.No
	s.SelectEofIsSuccess = interp.No
	s.SelectTakesUnterminatedReply = interp.No
	s.SelectEofPrintsNewline = interp.No
	// hash is an alias for `alias -t` here, and a name that resolves to
	// nothing is a silent success.
	s.HashReportsAMissingName = interp.No
	s.TildePlusMinusExpands = interp.Yes
	// A defined f-g stops the script; a.b is an invalid discipline function.
	s.PunctuatedFunctionNameIsRefused = interp.Yes
	// max+1 stays at the maximum, and a value that names another variable
	// is chased until it is a number.
	s.ArithOverflowSaturates = interp.Yes
	// A negative substring length is nothing at all here.
	//
	// A quoted `"${a[@]}"` is not next to it any more. This shell was the
	// one column recorded as handing the quotes one empty field there, on
	// the strength of `a=(); set -- "${a[@]}"` counting `n=1` — but `a=()`
	// is no array literal here at all. It builds a *compound* variable, and
	// the field that row counted held the text `(`, newline, `)` rather than
	// nothing. Asked with `set -A a`, which is this shell's way to declare
	// an array, there is no field; and `set -A a` on an array that already
	// has elements leaves the name unset, so there is no declared-and-empty
	// state to answer for either. Measured 2026-09-07 against 93u+
	// 2012-08-01. The yes here gave every `f "${a[@]}"` on a name nothing
	// had filled yet one spurious empty argument.
	s.SubstringNegativeLengthIsEmpty = interp.Yes
	// [ a -eq 1 ] is a plain false here, no sentence, status 1.
	s.TestIntegerRefusalIsSilent = interp.Yes
	// `f -nt missing` holds when f exists, and `-t x` is a plain false
	// rather than dash's and bash's integer complaint.
	s.MissingFileIsOlder = interp.Yes
	s.TerminalTestRequiresANumber = interp.No
	s.ArithNameValueRecurses = interp.Yes
	// This shell has no `local`, and it answers all the same: `typeset` in a
	// *keyword*-defined function is a local here — see
	// TypesetLocalNeedsKeywordFunction — and the shadow it declares over a
	// frozen name is a writable one, as in zsh. Measured 2026-09-07:
	//
	//	typeset -r x=1
	//	function f { typeset x=2; echo "in=[$x]"; }
	//	f; echo "st=$? out=[$x]"
	//	→ in=[2], st=0 out=[1]
	//
	// Written `f() { … }` instead there is no scope to shadow into, so the
	// same `typeset` is an ordinary assignment to the frozen global and is
	// fatal — which is TypesetLocalNeedsKeywordFunction and
	// ReadonlyReassignmentFatal doing their jobs, and not an answer to this.
	// Reading that fatality as this axis's answer is what put `No` here, and
	// it made a keyword function refuse a declaration this shell takes
	// (#1177).
	s.DeclarationMayShadowAReadonly = interp.Yes
	// The attribute cannot come off, though, which is where this shell parts
	// from zsh and why the two are separate questions: `typeset +r x` on a
	// frozen name is `typeset: x: is read only` and ends the script.
	s.ReadonlyAttributeCanBeRemoved = interp.No
	s.DeclaredNameWithoutValueIsEmpty = interp.No
	// But an attribute added to a name that already holds a value re-reads
	// that value at once: `FOO=bar; typeset -i FOO` stores 0 over the text,
	// and `d=MiXeD; typeset -u d` stores MIXED. bash waits for the next
	// assignment.
	s.AttributeRereadsTheValueItFinds = interp.Yes
	s.InheritedValueSurvivesADeclaredType = interp.No
	s.CompoundElementsGoThroughTheAttribute = interp.Yes
	s.CompoundAttribute = interp.CompoundAttributeFoldsEveryElement
	s.ArrayLiteralAssignmentStartsTheNameOver = interp.Yes
	// echo reads -n and -e; a word carrying -E is an operand. \e expands,
	// \x does not.
	s.EchoOptions = "ne"
	// `\E` is the escape character here and `\e` is two characters — the
	// opposite of zsh, which is why one axis could not answer for both.
	s.EchoExpandsEscEscape = interp.No
	s.EchoExpandsCapitalEscEscape = interp.Yes
	// read takes -r and -s plus -A, whose array is the first operand where
	// bash's -a takes it as the option's argument, and the same -d, -n, -N,
	// -t and -u. -p is a bare flag naming the coprocess as the source —
	// not bash's prompt — and there is no coprocess to name: ksh93's `|&`
	// is not in this grammar, so the letter always answers `no query
	// process` and 1, the variables untouched. A short -n is a success
	// here; a short -N reports 1 and leaves the variable empty.
	s.ReadOptions = "rspAd:n:N:t:u:"
	// ksh93 takes `-n` and refuses `-m`, with its own usage line after it.
	s.UnsetOptions = "vfn"
	s.ReadZeroTimeout = interp.ReadZeroTimeoutTakesWhatIsWaiting
	s.ReadPartialCountSucceeds = interp.Yes
	s.ReadExactCountKeepsPartial = interp.No
	s.ReadTimeoutKeepsWhatArrived = interp.No
	// typeset in a keyword function hides the caller's value, as bash's
	// local does.
	s.ValuelessDeclarationHidesTheOuterValue = interp.Yes
	s.TypesetLocalNeedsKeywordFunction = interp.Yes
	// There is no `local` here, so this is reached only through `typeset` in
	// a keyword function — where a child is told nothing about the shadowed
	// name, as in zsh. This shell arrives at that from further away, and the
	// axis below is the rest of the road: its `typeset` takes the export
	// attribute off any name it assigns, at the top level as well as in a
	// function.
	s.LocalInheritsTheExportAttribute = interp.No
	// The unscoped half of the same behavior. `export FOO=bar; typeset
	// FOO=baz` leaves this shell holding `baz` and telling no child about
	// it, where the other two hand `baz` over; `export FOO` afterwards puts
	// the attribute back.
	s.DeclarationAssignmentClearsTheExportAttribute = interp.Yes
	s.FatalErrorStatusIsOne = interp.Yes
	// A loop variable that is not a name parses here and ends the script when
	// the loop is reached, at 1. The same for `select`: re-measured on 93u+
	// with stdin closed, `select $n in a b` stops the script exactly as
	// `for $n in a b` does, so the loop keyword is not an axis (#1110).
	s.ForNameWhenTheLoopRuns = interp.ForNameEndsTheScript
	// The one shell in the panel that reports *every* option word `set`
	// cannot use before it gives up, with a single usage block after them
	// all. The other four stop at the first, which three of them do because
	// the refusal is fatal there and the loop never reaches the second — so
	// this is a rule of its own here rather than a consequence of the
	// fatality, since this shell's refusal is fatal too and it still prints
	// both lines first (#1170).
	s.SetReportsEveryBadOption = interp.Yes
	s.HeredocExpandsInTheCommandsProcess = interp.Yes
	s.ArithInvalidOctalDigitIsError = interp.No
	s.IndirectionYieldsName = interp.Yes
	s.BraceExpansion = interp.Yes
	// The one shell that strips a range endpoint's zeros — `{01..3}` is
	// `1 2 3` — and takes a written step's sign at its word, so `{10..1..3}`
	// is `10` alone and `{1..10..-3}` is `1`. A negative step that agrees
	// with the endpoints keeps their order: `{3..1..-1}` is `3 2 1`.
	s.BraceRangePadsToEndpointWidth = interp.No
	s.BraceRangeStepSignHonored = interp.Yes
	s.BraceRangeNegativeStepReverses = interp.No
	s.BracketCaretNegates = interp.Yes
	s.LastPipelineElementInCurrentShell = interp.Yes
	// Nor here. ksh93 refuses `[[ $v == <(cmd) ]]` earlier still — while
	// reading, as `` `<(' unexpected `` — so the answer is the same no and
	// only the moment differs. What this dialect does not yet reproduce is
	// that moment: it reads the word and refuses it at the run.
	s.ProcessSubstitutionInCondition = interp.No
	s.UnterminatedBracket = interp.BracketLiteral
	s.ExitArgument = interp.ExitArgLenient
	s.UnsetPositionalIsAllowed = interp.Yes
	s.TraceShowsItsOwnDisabling = interp.No
	s.TraceAssignmentsSeparately = interp.Yes
	// An unparseable `eval` is reported and survived here, unlike dash, but a
	// file `.` cannot open still ends the script — so the two halves of the
	// POSIX "a special builtin's failure is fatal" rule are answered
	// differently, which is why they are two axes.
	s.BuiltinSyntaxErrorFatal = interp.No
	// An error inside a file `.` read ends that file and nothing above it:
	// measured, `.` reports 1 and the sourcing file runs the command after
	// it — for an unset parameter under `set -u`, a readonly assignment, a
	// division by zero and a bad substitution alike.
	s.FatalErrorEndsBorrowedTextOnly = interp.Yes
	// And `${x?word}` is one of those errors here rather than a request to
	// stop: `.` reports 1 for it too and the sourcing file carries on, which
	// is the half of this zsh answers the other way.
	s.ParamErrorIsAnExitRequest = interp.No
	s.DotPassesArguments = interp.Yes
	s.ExecFailureRunsExitTrap = interp.No
	s.ExecTakesOptions = interp.Yes
	s.TestAcceptsDoubleEqual = interp.Yes
	s.SignalDeathStatusIsTwoFiftySix = interp.Yes
	s.PipefailOption = interp.Yes
	// And the one place ksh93's 256-plus-the-signal convention stops: an
	// element pipefail substitutes for the last one, and which a signal
	// killed, is reported as the signal's number alone — 13 and 15, not 269
	// and 271.
	s.PipefailSubstitutesTheBareSignal = interp.Yes
	s.ErrexitSeesPipefailFailure = interp.No
	// Alone in the panel: `PATH=` finds nothing here, where dash, bash and
	// zsh still search the current directory.
	s.EmptyPathIsTheCurrentDirectory = interp.No
	s.ExitTrapRunsOnSignalDeath = interp.Yes
	s.QuitIgnoredWhenNotInteractive = interp.No
	s.HangupIsAnOrderlyExit = interp.No
	s.ExitInTrapReportsEarlierStatus = interp.Yes
	s.KillListAcceptsName = interp.Yes
	s.SIGPrefixAccepted = interp.Yes
	s.RedirectsWriteToEveryTarget = interp.No
	s.KillStatus = interp.KillStatusAnyFailure
	s.SubshellJobTable = interp.SubshellJobsKept
	s.PrintfOutputPrecedesComplaint = interp.Yes
	s.PrintfEmptyIsNotANumber = interp.No
	s.PrintfReportsBadNumber = interp.No
	s.PrintfBackslashC = interp.PrintfBackslashCControl
	// `printf 'a%5'` is `a%` here and reports success: the unfinished
	// conversion becomes one literal character and the prefix is dropped.
	s.PrintfUnfinishedConversionIsAPercent = interp.Yes
	// Every digit that follows, and more than two of them make the value a
	// code point rather than a byte: `\xff` is one byte and `\x0ff` is
	// U+00FF in UTF-8. An empty digit run is a zero.
	s.PrintfHexEscape = interp.PrintfHexEscapeCodePoint
	// And no `\x` at all in a `%b` argument, which is this shell alone and
	// the reason the two sites are two axes: `printf '%b' 'a\x41Z'` is the
	// six characters as written where the same escape in a format is an `A`.
	s.PrintfBHexEscape = interp.PrintfHexEscapeAbsent
	// `\E` is the escape character and `\e` is two characters — the opposite
	// of zsh, which is why one axis could not answer for both letters.
	s.PrintfBEscEscape = interp.No
	s.PrintfBCapitalEscEscape = interp.Yes
	// The octal wants its `\0`: `printf '%b' 'a\101Z'` is `a\101Z` and
	// `a\0101Z` is `aAZ`.
	s.PrintfBOctalWithoutZero = interp.No
	// This shell alone: what a `\c` left is written as it stands, so
	// `printf '[%5b]' 'a\cb'` is `[a` where the other five pad it. A
	// property of the stop — with nothing stopping it this shell pads and
	// truncates like the rest.
	s.PrintfBStopIsPadded = interp.No
	// The same set bash takes, and ignored the same way. ksh93 will also
	// read a width *after* the modifier — `%l5d` is a padded 42 there — but
	// that is its free-order conversion prefix rather than this axis: `%5-d`
	// works there too, with no modifier in it at all.
	s.PrintfLengthModifiers = interp.PrintfLengthModifiersC99
	// ksh93's `%T` is a different conversion under the same letter: its
	// operand is a date *string* and a number earns a warning and the current
	// time. Not the one bash has, and not modeled — see
	// docs/spec/semantics.md.
	s.PrintfTimeConversion = interp.No
	s.PrintfQuote = interp.PrintfQuoteSingle
	// The same `\c` as the printf format, and the arithmetic is bit 6
	// toggled rather than bash's five-bit mask: `$'\c1'` is `q`, not 0x11.
	s.DollarSingleBackslashC = interp.DollarSingleControlToggled
	s.DollarSingleUnknownEscape = interp.DollarSingleUnknownDropsBackslash
	s.DollarSingleNulTruncates = interp.Yes
	s.GetoptsAssignmentRestartsWord = interp.Yes
	s.GetoptsClearsOptarg = interp.No
	s.CdWithoutHomeIsAnError = interp.Yes
	s.CdDashPrintsTheDirectory = interp.Yes
	s.PrintfAssignsWithV = interp.No
	s.PrintfRejectsUnknownOption = interp.Yes
	s.TrapParsesOptions = interp.Yes
	s.TrapPrintsWithP = interp.Yes
	s.TrapPrintsBareWithConditions = interp.Yes
	s.TrapPrintsBareWithP = interp.No
	s.TrapListsSignalsWithL = interp.No
	// The one dialect that refuses `trap EXIT`, and the refusal is fatal.
	s.TrapOneArgumentIsACondition = interp.No
	// ERR and DEBUG but not RETURN. Both follow the script into functions;
	// only DEBUG follows it into a subshell — measured, a command
	// substitution there captures the DEBUG handler's output and not the
	// ERR handler's.
	s.TrapHasErrCondition = interp.Yes
	s.TrapHasDebugCondition = interp.Yes
	s.TrapHasReturnCondition = interp.No
	s.ErrTrapRunsInsideFunctions = interp.Yes
	s.ErrTrapRunsInSubshells = interp.No
	s.DebugTrapRunsInsideCalls = interp.Yes
	s.DebugTrapRunsInSubshells = interp.Yes
	// `(trap)` and `$(trap)` still list the parent's traps, EXIT included —
	// measured, and the working state is still reset: `trap 'echo x' USR1;
	// (trap)` prints the trap it will not fire. A pipeline element or a
	// background job lists nothing: `trap | cat` is empty where `(trap)` is
	// not, which is the split issue #339 measured.
	s.SubshellKeepsTrapListing = interp.Yes
	s.KeptTrapListingIncludesExit = interp.Yes
	s.UmaskPrintsFourDigits = interp.Yes
	s.UmaskSetWithSPrints = interp.No
	s.UlimitBlockIsKilobyte = interp.No
	s.UlimitHasResidentSet = interp.Yes
	s.UlimitHasProcessCount = interp.Yes
	s.UlimitSetsBothLimits = interp.Yes
	s.BadOptionToSpecialBuiltinFatal = interp.Yes
	// A redirection that cannot be made is a special builtin's failure too,
	// and this shell keeps the POSIX rule without needing a mode to be in:
	// `exec 3>/nope/x` stops the script at 1.
	// The number is read and the failure is the descriptor's, not the
	// word's — this shell has no width rule of its own here.
	s.MultiDigitDuplicationTargetIsAnError = interp.No
	s.RedirectErrorOnSpecialBuiltinFatal = interp.Yes
	// `>&` is a duplication and nothing else here: `echo hi >&qq` is
	// `qq: bad file unit number` and no file is made.
	s.GreatAmpTarget = interp.GreatAmpTargetIsADescriptor
	s.DuplicationTargetErrorOnABuiltinIsFatal = interp.No
	// Fatal to `export` and `readonly` and not to `unset`, which prints the
	// same kind of complaint, returns 1 and carries on. Not `unset` being
	// less special: a bad *option* to it is fatal, just above.
	s.BadNameToDeclarationFatal = interp.Yes
	s.BadNameToUnsetFatal = interp.No
	// And not to `read`, which is not a special builtin in any shell.
	s.BadNameToReadFatal = interp.No
	// Not fatal here either, and ksh93 goes further than any other member of
	// the panel in saying so: it calls the refusal a *warning*.
	s.UnsetReadonlyFatal = interp.No
	s.DeclarationNameOperands = interp.PlainNamesOnly
	s.UnsetNameOperands = interp.PlainNamesOnly
	s.ReadNameOperands = interp.PlainNamesOnly
	// The prompt operand is this shell's invention: `read "v?Name: "` reads
	// into v and writes the prompt at a terminal. A `?` with nothing in front
	// of it leaves an empty name, and ksh93 refuses that rather than reading
	// into REPLY — `read "?p"` is `read: : invalid variable name`, the
	// complaint naming the empty word it was left with. zsh took the form and
	// not that half of it.
	s.ReadPromptOperand = interp.ReadPromptNeedsANameBeforeIt
	// The first operand is judged before the stream is touched:
	// `printf 'AAA\nBBB\n' | { read 1bad; cat; }` prints both lines.
	s.ReadRefusesABadNameBeforeReading = interp.Yes
	s.DeclarationTakesASubscript = interp.Yes
	// The declaration builtins take one as well — `typeset a[1]=v` creates
	// the element — so this shell gives the two the same answer where bash
	// splits them.
	s.TypesetTakesASubscript = interp.Yes
	s.UnsetTakesASubscript = interp.Yes
	// And nothing a declaration carries makes it refuse the element:
	// measured 2026-09-07, `typeset -i a[1]=0x10` reads back 16,
	// `readonly a[1]=v` writes v and freezes `a` over it, and a declaration
	// inside a function writes the caller's array, which is this shell's
	// answer about scope rather than about subscripts.
	// The refusal is fatal and the rest of the operand list is declared
	// anyway: measured 2026-09-07, `export ok1=1 ":" ok2=2` read back from
	// an EXIT trap leaves both set, wherever the bad name stands.
	// `read -A` of a line that splits into nothing leaves one empty element
	// rather than none — measured 2026-09-07, and the same for a line of
	// nothing but IFS whitespace. bash leaves none. The closing whitespace
	// run itself opens no field here, which is where this shell parts from
	// zsh.
	s.ReadNoFieldsIsOneEmptyElement = interp.Yes
	s.ReadTrailingWhitespaceEndsAField = interp.No
	s.BadNameDeclaresTheOperandsAfterIt = interp.Yes
	s.SubscriptedOperandTakesTheIntegerAttribute = interp.Yes
	s.SubscriptedOperandTakesALocalDeclaration = interp.Yes
	s.ReadonlyElement = interp.ReadonlyElementWritten
	// `@` is not a spelling for the whole array here. The brackets hold an
	// arithmetic expression as they do everywhere else, `@` is not one, and
	// the operand is reported as a bad subscript with the array left as it
	// was — the only shell in the panel that does not clear it.
	// A subscript naming no element of a name that is no array is quiet here:
	// `a=v; unset "a[1]"` says nothing and succeeds, where bash refuses it.
	// `a[0]` names the string — a scalar is the one element at the base — and
	// takes the whole name away, which both shells that read elements do.
	s.UnsetSubscriptOnAScalarIsAnError = interp.No
	s.UnsetArraySpan = interp.UnsetArraySpanIsAnExpression
	// And the complaint is the builtin's: `unset` reports 1 and the script
	// goes on, which is what makes `unset a[@]` survivable here.
	s.BadSubscriptToUnsetFatal = interp.No
	// A negative subscript past the first element is refused here too, and
	// the refusal ends the script.
	s.NegativeSubscriptPastTheStartInserts = interp.No
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.Yes
	s.JobsListFinishedJobs = interp.Yes

	// `jobs`' letters, as its own usage line gives them: `-lnp`. The state
	// filters `-r` and `-s` are unknown options here, and `-n` rides
	// UnimplementedOptionLetters — ksh93 reads it as the jobs that have
	// stopped or ended since it last said, which is not bash's reading of
	// the same letter.
	s.JobsOptions = "lp"
	s.JobsPidsOnlyOption = interp.Yes

	// Whether a `&` job's command appears in a `jobs` listing.
	s.JobsShowBackgroundCommand = interp.No

	// Whether a backgrounded job is announced to whoever is typing.
	// Whether `export -f` carries a function to a child.
	// Whether an unassigned subscript is an element.
	s.ArraysAreSparse = interp.Yes
	// An operator on `${a[*]}` trims each element before the join here.
	s.OperatorDistributesOverStarSubscript = interp.Yes
	s.ExportCarriesFunctions = interp.No
	// No `-n` either: measured, `export: -n: unknown option` with the usage
	// line under it, and the script ends there.
	s.ExportTakesTheAttributeOff = interp.No
	s.AnnouncesBackgroundJob = interp.Yes
	// The panel's dissenter, and the only cell of the interactive table that
	// was measured and not reproduced: `monitor on` and `imBE` under
	// `-i script.sh` with no terminal anywhere, announcing its background
	// jobs into a pipe.
	s.InteractiveMonitorNeedsATerminal = interp.No
	// A subshell here is not a process of its own, so a fatal signal aimed
	// at the shell from inside one lands on the thing that was about to run
	// the next command. Measured: `(kill -TERM $$; echo inner)` prints
	// nothing at all, where the five that fork print `inner` and then die.
	s.SubshellRunsOnAfterSignalingTheShell = interp.No
	// And it announces both ends of a job on that route: measured on
	// `-i script.sh` through a pseudo-terminal, `[1]\t<pid>` as the job
	// starts and `[1] +  Done  sleep 0.3 &` as it ends.
	s.InteractiveScriptAnnouncesJobs = interp.Yes
	// `$!` before any background command is set and empty here, so `set -u`
	// has nothing to say about it: measured, `set -u; echo "[$!]"; echo
	// "st=$?"` writes `[]` and then `st=0`. Stated rather than left
	// unanswered, because ksh93 is on the quiet side of a two-against-two
	// split and an unanswered field would read as "not yet measured".
	//
	// Note this is *not* UnsetPositionalIsAllowed reached from another route.
	// ksh93 does let an unset `$1` be empty, and it lets `$!` be empty too,
	// but the two are separate answers: bash refuses both and dash refuses
	// both, while zsh refuses `$1` and not `$!`.
	s.LastBackgroundPidIsUnsetBeforeAnyJob = interp.No
	// A job started with `&` reads an empty standard input, not the shell's:
	// measured 2026-09-07, `ksh -c '/bin/cat & wait; echo ---; /bin/cat' < f`
	// writes `---` and then the file's line on ksh93u+. POSIX XCU 2.9.3.
	//
	// `UnlessClosed` and not the plain answer, which is this column's alone
	// among the four that substitute: `exec 0<&-; /bin/cat & wait` is silent
	// at 0 in dash and bash and `cat: stdin: Bad file descriptor` here. What
	// it cannot dup it leaves alone, so a closed fd 0 reaches the job closed.
	s.BackgroundJobInput = interp.BackgroundJobInputEmptyUnlessClosed
	// And it reads as nothing rather than as a zero: `echo "[$!]"` is `[]`,
	// which is what makes ksh93's empty a *set* parameter with no value where
	// zsh's is a value.
	s.LastBackgroundPidIsZeroBeforeAnyJob = interp.No
	s.ReportsACommandKilledBySignal = interp.Yes
	s.ReportsAnyKilledPipelineElement = interp.No
	s.ChildInterruptEndsTheScript = interp.Yes
	s.CdRefusesUnknownOption = interp.Yes
	s.CdLastPathOptionWins = interp.Yes
	s.BadSetOptionNameFatal = interp.Yes
	s.UnknownConditionOptionIsAStatus = interp.No
	s.ReturnOutsideAFunctionIsRefused = interp.No
	// A startup file is a sourced script, so a `return` in one is accepted
	// everywhere — the split is only over what its argument does, and this
	// shell keeps it: measured through a pty, an rc of `return 3` and one
	// of `false; return 3` both leave `$?` as 3 at the first prompt, where
	// bash leaves 0 and 1. A `return` with no argument means the last
	// command's status here as everywhere.
	s.StartupFileReturnCarriesItsArgument = interp.Yes
	s.LoneDashIsAnOption = interp.No
	s.UnsetFunctionChecksTheName = interp.Yes
	s.UnsetFunctionReportsMissing = interp.No

	// Whether a redirection target is expanded as an ordinary word.
	s.RedirectTargetIsAnOrdinaryWord = interp.No

	// Whether `type --` ends the options.
	s.TypePrintsFunctionBody = interp.No
	s.TypeEndsOptionsWithDashDash = interp.Yes
	// No `-t` here: the letter is refused the way `whence` refuses any
	// option it does not have, usage line and all.
	s.TypeNamesTheKindWithDashT = interp.No
	// whence -v's other letters: -a lists every resolution, -p and -f are
	// the PATH search and the function skip — and -p searches past the
	// shell's own answer here, naming the bare path.
	s.TypeOptions = "afp"
	s.TypePSearchesPathPastTheShell = interp.Yes
	s.TypePathAnswerIsASentence = interp.No
	s.TypeFSaysTheFunctionBack = interp.No

	// The letters `typeset` reads here. `-f` prints functions *verbatim* in
	// this engine — it keeps the source text, which this one does not — so
	// it rides in Diagnostics.UnimplementedOptionLetters with the floats
	// and the padding letters; `-g` it simply does not have. There is no
	// `local` (see Register), so LocalOptions stays empty.
	s.DeclareOptions = "aAilprux"
	// typeset is one of this shell's own special builtins, so any of its
	// failures ends the script — a bad option included.
	s.TypesetBadOptionFatal = interp.Yes
	// `integer` is the same declaration under a second name, and this shell
	// hands it typeset's whole letter grammar — measured 2026-09-06, every
	// letter typeset takes is either accepted by `integer` or refused by it
	// for conflicting with the type, and none is refused as unknown. So the
	// set is typeset's and so is Diagnostics.UnimplementedOptionLetters for
	// it, which is why `integer -f` here says the letter is missing rather
	// than claiming this shell has never heard of it.
	s.IntegerOptions = "aAilprux"
	// `-i16` and `-i 16` are an output base here — `integer -i 16 b=255` is
	// `16#ff` — and this engine has no base to keep, so it refuses by name.
	s.IntegerAttributeTakesABase = interp.Yes
	// Lower case first and on into upper, then two more: measured, base 64
	// spells 61 `Z`, 62 `@` and 63 `_`, so the alphabet is sixty-four long
	// and `typeset -i64 a=100` is `64#1A`.
	s.IntegerBaseDigits = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ@_"
	s.IntegerBaseComesFromTheValueAssigned = interp.No
	s.IntegerBaseNegativeIsTwosComplement = interp.Yes
	// The letter always names a base here and ten is what it names when
	// nothing is written: measured, `typeset -i10 d=255` lists back as
	// `typeset -i d=255` with no base word, and a later bare `typeset -i a`
	// — or `integer a`, the same declaration under another word — takes the
	// base off a name that had one and leaves `255` standing.
	s.IntegerBaseTenIsNoBase = interp.Yes
	// The name carries the type in this shell, so a plus word on `integer`
	// removes nothing: `integer n=5; integer +i n; n=3+4` is 7 here, and
	// `integer -x e=1; integer +x e` leaves e exported — where this shell's
	// own `typeset +x` does unexport. `typeset -p n` says the same from the
	// value side, `typeset -l -i n=5` against zsh's plain `typeset n=5`.
	s.IntegerPlusFormTakesAttributesOff = interp.No
	// `set -A name value …` assigns an array through a name a variable
	// holds, which is this shell's spelling and zsh's alike.
	s.SetArrayLetter = interp.Yes
	// And the refusal of a name that is not one leaves 1 by both routes
	// here, which is the answer that makes the other shell's 0 a quirk of
	// that shell rather than a rule about the letter.
	s.SetArrayBadNameLeavesZeroFromCommandString = interp.No
	// And the option parse carries on past the name here: `set -A ff -x -y`
	// takes `-x` as xtrace and refuses `-y`, and `set -A dd -- 1 2` stores
	// two elements because the `--` still ends the options. So the values are
	// exactly the words that would have become the positional parameters.
	s.SetArrayOptionsContinuePastTheName = interp.Yes
	// `set -A a` with no values *unsets* the name here, where zsh leaves an
	// array with no elements — measured with `typeset -p a`, which writes
	// nothing at all in this shell.
	s.SetArrayWithNoValuesUnsetsTheName = interp.Yes
	// A bare `set` lists the variables alone, values bare until one needs
	// quoting and `$'...'` from there.
	s.SetListing = interp.SetListingAssignments
	s.SetListingQuoting = interp.ListingQuoteWhenNeededDollar

	// A `{name}>f` descriptor goes back with the command's other
	// redirections, and closing through a name that holds nothing is not
	// worth a word here.
	s.FdVariableOutlivesTheCommand = interp.No
	// And a descriptor `exec` opened is this shell's alone: measured, and its
	// manual says so — a file descriptor number greater than 2 opened by
	// `exec`'s redirection list is closed when it invokes another program.
	// One the caller opened still crosses, and so does one the command
	// redirects itself.
	s.ExecOpenedFdReachesACommand = interp.No
	s.FdVariableBadCloseIsAnError = interp.No
	// And a number the process cannot hold is refused, as it is in bash and
	// unlike dash and zsh — in this shell's own words and quoting a different
	// errno: with `ulimit -n 6`, `exec 8>f` is `bad file unit number [Invalid
	// argument]`.
	s.FdNumberBoundedByOpenFileLimit = interp.Yes

	return s
}

// Diagnostics is how ksh93 reports failure.
// kshKillUsage is printed on its own for `kill` with no operands, and again
// after an unknown option — with no shell name or line in front of it either
// time, which is not how ksh93 prints anything else.
const kshKillUsage = "Usage: kill [-lL] [-n signum] [-s signame] job ...\n" +
	"   Or: kill [ options ] -l [arg ...]"

func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		TypeKeyword: "%[1]s is a keyword",
		// ksh93's `type` is `whence -v`, and the message says so.
		TypeExternal:            "%[1]s is a tracked alias for %[2]s",
		TypeFunction:            "%[1]s is a function",
		TypeNotFound:            "whence: %[1]s: not found",
		CommandVNotFound:        "command: %[1]s: not found",
		JobStarted:              "[%[1]d]\t%[2]d",
		JobNoticeShowsAmpersand: true,
		// `[1] + ` then a 25-wide state, and the leading space on the
		// running one is ksh's own: a stopped job's `Stopped` starts a
		// column earlier than a running job's `Running`, and the two lines
		// end in the same place.
		JobLine: "[%[1]d] %[2]s %-25[3]s%[4]s",
		// `jobs -l`: the process id after the marker with a tab behind it,
		// and the same 25-wide state column.
		JobLineLong: "[%[1]d] %[2]s %[3]d\t%-25[4]s%[5]s",
		// The process id and the words, with a colon between them and no
		// command after: this shell names the line and the process but does
		// not say back what was running.
		KilledCommandNotice: "%[1]d: %[2]s",
		ParamNull:           "parameter null",
		// The array alone is named, not the subscript that was written.
		BadArraySubscript: "%[1]s: subscript out of range",
		// The same sentence from `unset`, with the builtin named in front of
		// it as this shell names it in front of the arithmetic one below.
		UnsetSubscriptBeforeTheFirstElement: "unset: %[1]s: subscript out of range",
		UnsetBadFunctionName:                "unset: %[1]s: invalid function name",
		// The builtin names itself in front of the arithmetic sentence, which
		// it does not do for the identical failure in an expansion.
		UnsetBadSubscript:    "unset: %[1]s",
		SetInvalidOptionName: "set: %[1]s: bad option(s)",
		// The letter as the script spelled it: `set +q` is refused as `+q`
		// here, as it is in bash.
		SetInvalidOptionLetter: "set: %[1]s: unknown option",
		// The one shell in the panel that repeats `set`'s usage line under
		// a bad option *name* as well as under a bad letter.
		SetInvalidOptionNameUsage: true,
		// At an invocation ksh93 prints its own usage instead, and names
		// itself by the last element of the word it was invoked by — where
		// bash spells the whole path. Measured through a link named
		// `myksh`, which is what it called itself.
		InvocationUsage:    "Usage: %[2]s [-cilrsDEabefhkmnprtuvxBCGH] [-R file] [-o[option]] [arg ...]",
		SignalDescriptions: signalDescriptions(),
		JobRunning:         " Running",
		// The spec is not named. Measured on `jobs %9`, which says exactly
		// `jobs: no such job` — the one wording in this area that uses
		// neither verb, and it is the shell rather than a truncation: `%nope`
		// produces the same line.
		NoSuchJob:  "%[1]s: no such job",
		JobStopped: "Stopped",
		// ^Z prints the listing's own row straight after the echoed `^Z`, as
		// dash does; `fg` names the command alone. `bg` writes the job
		// number, a tab, the command and an `&` with no space before it —
		// measured through a pseudo-terminal on 2026-09-05.
		JobResumedInBackground: "[%[1]d]\t%[3]s&",
		JobUnknownCommand:      "<command unknown>",
		// ksh93 lists a job that has already ended as "Running", and it is
		// not a reaping race — it still says so after `wait`. Recorded as
		// the word ksh uses rather than corrected, which would be inventing
		// a shell that does not exist. The leading space is ksh's too: see
		// JobLine below.
		JobDone:       " Running",
		JobDoneNotice: " Done",
		// The name first, then the verb with the OS string bracketed after
		// it — the same shape ksh93 uses for `.`, which DotCannotOpen
		// already says.
		CannotOpen: "%[1]s: cannot open [%[2]s]",
		// No BuiltinWriteError: `echo hi >&-` reports 1 here and says
		// nothing, which is the semantics axis answering and the wording
		// staying empty.
		// Measured: ksh93 reports a failed open at the line before the redirect.
		RedirectFailureLine: interp.LineBeforeRedirect,
		// A descriptor number the process cannot hold names the thing rather
		// than the number, and quotes the errno the other refusing shell does
		// not: `ulimit -n 6; exec 8>f` is `bad file unit number [Invalid
		// argument]` here and `8: Bad file descriptor` in bash.
		FdNumberOverLimit: "bad file unit number [Invalid argument]",
		// A target that expanded to nothing gets neither the reason nor the
		// "create" wording, whichever direction the redirection was.
		EmptyRedirectTarget: "%[1]s: cannot open",
		// A word after `>&` or `<&` that is not a descriptor is refused as a
		// bad unit number, and a word that came to nothing takes the same
		// "cannot open" the ordinary empty target does.
		DuplicationTargetIsNotADescriptor: "%[2]s: bad file unit number",
		EmptyDuplicationTarget:            "%[2]s: cannot open",
		CannotCreate:                      "%[1]s: cannot create [%[2]s]",
		NoclobberRefusal:                  "%[1]s: file already exists [%[2]s]",
		ArithFailureStatus:                1,
		ArithInfinity:                     "inf",
		ArithNotANumber:                   "nan",
		ArithFloatDigits:                  15,
		ArithFloatKeepsPoint:              false,
		SelectPrompt:                      "#? ",
		// A script operand that names nothing is a command that is not there,
		// worded and numbered as one — and one that is there and will not
		// open gets the bracketed reason ksh93 puts around every errno, at
		// the 126 an unrunnable command carries. It is the only shell in the
		// panel that words the two differently without also numbering them
		// differently from bash.
		ScriptNotFound:              "%[1]s: not found",
		ScriptNotFoundStatus:        127,
		ScriptNotReadable:           "%[1]s: cannot open [%[2]s]",
		ScriptNotReadableStatus:     126,
		Location:                    interp.LocationLineWordAfterFirst,
		TraceQuoting:                interp.QuoteDollar,
		ScriptLocation:              interp.LocationLineWord,
		BuiltinLocation:             interp.LocationBracketLineAfterFirst,
		ScriptBuiltinLocation:       interp.LocationBracketLine,
		ParseFailureNamesItsOwnLine: true,
		ReadonlyVariable:            "%s: is read only",
		// A warning rather than an error, in so many words, and the only
		// member of the panel that says so.
		UnsetReadonly: "unset: warning: %s: is read only",
		ShiftTooMany:  "shift: %[2]s: bad number",
		// The same sentence for a count below zero, which is only reachable
		// here after the end-of-options marker.
		ShiftNegativeCount:   "shift: %[2]s: bad number",
		StdinBuiltinLocation: interp.LocationBracketLine,
		ArithError:           "%[1]s: %[2]s",
		DivisionByZero:       "divide by zero",
		// ksh93 names the innermost keyword still awaiting a partner: `if`
		// on its own, and the `then` inside it once that has been consumed.
		EvalNaming:             interp.SourceBeforeLocation,
		SourceFileNaming:       interp.SourceBeforeLocation,
		SourceFileIsTheBuiltin: true,
		// A failing offset is blamed together with what follows it in the
		// range: `${x:1+:2}` names `1+:2`. A failing length has nothing after
		// it and is named on its own.
		SubstringErrorNamesTheWholeRange: true,
		// An operand failure is two sentences here, and which one is said
		// turns on whether the expression ran out or found something it
		// could not use: `$((1+))` against `$((%))`.
		ArithOperandExpected:  "arithmetic syntax error",
		ArithExpressionRanOut: "more tokens expected",
		ArithOperatorExpected: "arithmetic syntax error",
		// A digit the base does not have is the same sentence.
		DigitTooGreatForBase: "arithmetic syntax error",
		// ksh93 does not call this a bad substitution: it is a syntax error
		// naming the character it could not read.
		BadSubstitution: "syntax error at line %[2]d: `%[1]s' unexpected",
		// Except for the `@` operator family, the one bad substitution ksh93
		// defers to run time — measured, `${x@Q}` in a branch never taken is
		// silent — and when reached it is reported as a bad substitution
		// after all, not with the parse wording above.
		// The whole word as it was written, quotes and all: `echo
		// "[${x@QQ}]"` is refused as `"[${x@QQ}]": bad substitution` and
		// `echo pre${x@QQ}post` as `pre${x@QQ}post: bad substitution`, so
		// what is named is the source of the word rather than the `${…}`.
		BadSubstitutionAtRun:   "%[1]s: bad substitution",
		BadSubstitutionNames:   interp.NamesTheWholeWord,
		FunctionNameInvalid:    "%[1]s: invalid function name",
		FunctionNameDiscipline: "%[1]s: invalid discipline function",
		SyntaxUnexpected:       "syntax error at line %[3]d: `%[1]s' unexpected",
		// A parse failure by every other measure, and 1 rather than this
		// dialect's syntax-error status.
		ForNameStatus: 1,
		ForName:       "%[1]s: invalid variable name",
		Unterminated:  "syntax error at line %[6]d: `%[3]s' unmatched",
		// Nothing is unmatched when nothing was open, so the end of input is
		// named as the thing that was unexpected instead.
		UnterminatedNoConstruct: "syntax error at line %[6]d: `end of file' unexpected",
		// Only the substitutions can go unmatched here — a quote the input
		// runs out inside is closed and run, which is the grammar flag.
		// The `"` case is a `${` that began inside a double quote, and it
		// is worded as the quote character standing where it should not.
		UnmatchedQuote:    "syntax error at line %[4]d: `%[1]s' unmatched",
		UnmatchedCmdSubst: "syntax error at line %[4]d: `(' unmatched",
		// A process substitution reaches a different *diagnosis* here, not
		// only a different sentence: `$(` is an unmatched parenthesis named
		// at the opener's line, and `<(` is the end of the file named at the
		// line the file ended on. Measured — `cat <(echo hi` in a script
		// answers "syntax error at line 2: `end of file' unexpected" where
		// `v=$(echo hi` answers "syntax error at line 1: `(' unmatched".
		UnmatchedProcSubst:  "syntax error at line %[5]d: `end of file' unexpected",
		UnmatchedBraceSubst: "%[3]s{: bad substitution",
		// Exactly what it says for `$(`: this shell blames the parenthesis
		// and the opener's line and does not distinguish the two
		// constructs. Written out rather than shared with UnmatchedCmdSubst
		// so that a later change to one cannot silently move the other.
		UnmatchedArithSubst: "syntax error at line %[4]d: `(' unmatched",
		SyntaxErrorStatus:   3,
		// The status is never reached — a file `.` cannot open ends the script
		// here — but the wording is, and it names the operand and the reason in
		// brackets rather than after a colon.
		DotCannotOpen:          ".: %[1]s: cannot open [%[2]s]",
		DotNoOperandUnprefixed: true,
		// A value that is not a number is a parameter name in ksh93, so the
		// failure is that the name is unset rather than that the text is not
		// a number.
		InvalidNumber: "%[1]s: parameter not set",
		// No name in front of it and no `.:` either: ksh93 prints a usage
		// line bare, the same way it prints `kill`'s.
		DotNoOperand:       "Usage: . [ options ] name [arg ...]",
		DotNoOperandStatus: 2,
		// The reason goes in brackets, as it does for `.`.
		// A command word names no builtin; `exec` names itself.
		CannotExecute:     "%[1]s: cannot execute [%[2]s]",
		ExecCannotExecute: "exec: %[1]s: cannot execute [%[2]s]",
		// Not "cannot execute": ksh93 distinguishes a missing command from one
		// that will not run, and only the second gets the brackets.
		ExecNotFound:           "exec: %[1]s: not found",
		GetoptsBadOption:       "-%[1]s: unknown option",
		GetoptsMissingArgument: "-%[1]s: argument expected",
		CdCannotChange:         "cd: %[1]s: [%[2]s]",
		// One message for both, where bash names which variable was missing.
		CdHomeNotSet:              "cd: bad directory",
		CdOldpwdNotSet:            "cd: bad directory",
		PrintfBadVerb:             "printf: %[1]s: unknown format specifier",
		PrintfBadOption:           "printf: %[1]s: unknown option",
		TrapConditionRequired:     "trap: condition(s) required",
		PrintfBadOptionShowsUsage: true,
		UmaskBadMask:              "umask: %[1]s: bad number",
		// The name leads and the builtin follows it, which is the reverse of
		// everyone else — and unprefixed.
		AliasNotFound:             "%[2]s: %[1]s not found",
		AliasNotFoundUnprefixed:   true,
		UnaliasUsage:              "Usage: unalias [-a] name...",
		UnaliasUsageUnprefixed:    true,
		UmaskBadSymbolicMode:      "umask: %[1]s: bad format",
		UmaskBadOption:            "umask: %[1]s: unknown option",
		UmaskUsage:                "Usage: umask [-S] [mask]",
		UmaskUsageUnprefixed:      true,
		LetNoExpression:           "Usage: let [ options ] [expr ...]",
		LetNoExpressionStatus:     2,
		LetNoExpressionUnprefixed: true,
		UlimitBadOption:           "not supported",
		UlimitBadNumber:           "ulimit: %[1]s: parameter not set",
		BuiltinBadOption:          "%[1]s: %[2]s: unknown option",
		BadOptionNaming:           interp.BadOptionWholeWord,
		WaitBadJob:                "wait: %[1]s: Arguments must be %%job, process ids, or job pool names",
		WaitBadJobStatus:          1,
		// ksh93 has `--version` here, which this shell does not.
		UnimplementedOptionLetters: map[string]string{
			// `set` letters ksh93 has and this shell does not: -b job
			// notices, -k assignment-anywhere, -p privileged, -r
			// restricted, -s sorting the positional parameters, -t one
			// command, -A assigning an array, and -B -G -H, its brace
			// expansion, globstar and history-expansion switches. Measured
			// 2026-09-05 by asking ksh93 for every letter of the alphabet
			// in both cases and both signs.
			// `-A` has left this list: it assigns an array and is
			// implemented, in Semantics.SetArrayLetter.
			"set": "bkprstBGH",
			// ksh93 answers --version on most builtins, and has its own
			// letters for these two.
			"wait": "-",
			// What is left of read's letters: compound -C, -S's csv
			// splitting and -v's default text. `-p` is implemented: it
			// reads the coprocess `cmd |&` started, and is
			// ReadNoCoprocess's wording when none is running.
			"read": "-CSv",
			"type": "-qv",
			// `jobs -n`: the jobs that have stopped or ended since this
			// shell last said so, which needs a record of what it has
			// already reported — and reads differently from bash's letter
			// of the same name, which counts a job that has only just
			// started as a change.
			"jobs": "-n",
			// typeset's letters this engine does not hold: the verbatim
			// function listings (-f and the floats' -F), namerefs, padding
			// and alignment, mappings and the rest of its usage line.
			"typeset": "-bfFhmnstCEHLMRSTXZ",
			// `integer` reads typeset's letters, so it is missing exactly
			// the ones typeset is missing — including the `--version` this
			// shell answers on both names.
			"integer": "-bfFhmnstCEHLMRSTXZ",
		},
		// ksh93's one sentence for a dead -u descriptor, the number not
		// named; the non-number wordings per letter are not modeled yet, so
		// those fall back to the substrate's.
		// `set -A` with no name says which operand is missing, and prints
		// set's usage under it — measured, `set: -A: name argument expected`.
		SetArrayNeedsAName: "set: %[1]s: name argument expected",
		// And `set -A ro q` over a frozen name names the builtin where a
		// plain assignment to the same name does not: `set: ro: is read
		// only` against `ro: is read only`.
		ReadonlyVariableInDeclaration: "%[2]s: %[1]s: is read only",
		ReadonlyRefusalNamesBuiltin:   map[string]bool{"set": true},
		// And `typeset` names itself for a refused *removal* alone. Measured
		// 2026-09-07: `typeset +r x` is `<script>[2]: typeset: x: is read
		// only` where `typeset x=2` on the same name is `<script>: line 2:
		// x: is read only`. One word, two shapes, two sentences — which is
		// what the second table is for.
		ReadonlyRemovalNamesBuiltin: map[string]bool{"typeset": true},
		ReadBadFileDescriptor:       "read: bad file unit number [Bad file descriptor]",
		// ksh93 calls the coprocess the query process, and `read -p` with
		// none running says so — the only reachable answer here, this
		// grammar having no `|&`.
		ReadNoCoprocess: "read: no query process",
		// ksh93's `type` is `whence -v`, and a refused option says so —
		// measured with `type -t echo`, whose complaint and usage line both
		// name `whence`.
		// ksh93's `type` is `whence -v`, and its `integer` is `typeset` — the
		// second name is a spelling and the builtin says so, both in the
		// complaint and in the usage line under it. Measured with
		// `integer -Q w=1`, whose two lines name typeset throughout.
		BuiltinComplaintName: map[string]string{"type": "whence", "integer": "typeset"},
		// Two wordings, split between `export` and the other two, and the
		// operand quoted back as given.
		BuiltinBadName: map[string]string{
			"export":   "%[1]s: %[2]s: is not an identifier",
			"readonly": "%[1]s: %[2]s: invalid variable name",
			// `set -A 1bad v` — the array letter's name operand, and it
			// takes readonly's wording rather than export's.
			"set":   "%[1]s: %[2]s: invalid variable name",
			"unset": "%[1]s: %[2]s: invalid variable name",
			// `typeset ':'` and `typeset 1x`, both `invalid variable name`
			// and both fatal. `integer` reaches this entry rather than one of
			// its own, because this shell's `integer 1x` calls itself
			// `typeset` — see BuiltinComplaintName above.
			"typeset": "%[1]s: %[2]s: invalid variable name",
			// `read` takes readonly's wording too, and it names the part in
			// front of a prompt `?`: `read "1bad?p"` is `1bad`, and
			// `read "?p"` is the empty word the split left.
			"read": "%[1]s: %[2]s: invalid variable name",
		},
		BuiltinBadNameKeepsValue: true,
		BuiltinUsageUnprefixed:   true,
		BuiltinUsage: map[string]string{
			"set": "Usage: set [-sabefhkmnprtuvxBCGH] [-A name] [-o[option]] [arg ...]",
			// The same usage block under the array letter, which is what
			// this shell prints for `set -A` with no name after it.
			"export":   "Usage: export [-p] [name[=value]...]",
			"readonly": "Usage: readonly [-p] [name[=value]...]",
			"read": "Usage: read [-ACprsSv] [-d delim] [-u fd] [-t timeout] [-n count] [-N count]\n" +
				"            [var?prompt] [var ...]",
			"trap": "Usage: trap [-p] [action condition ...]",
			// Two spaces before the ellipsis, as written.
			"type": "Usage: whence [-afpqv] name  ...",
			// Three lines, exactly as the engine wraps them.
			"typeset": "Usage: typeset [-bflmnprstuxACHS] [-a[type]] [-i[base]] [-E[n]] [-F[n]] [-L[n]]\n" +
				"               [-M[mapping]] [-R[n]] [-X[n]] [-h string] [-T[tname]] [-Z[n]]\n" +
				"               [name[=value]...]\n" +
				"   Or: typeset [ options ] -f [name...]",
			// The same three lines under `integer`, which is what this shell
			// prints: the usage belongs to the builtin and not to the word
			// that reached it.
			"integer": "Usage: typeset [-bflmnprstuxACHS] [-a[type]] [-i[base]] [-E[n]] [-F[n]] [-L[n]]\n" +
				"               [-M[mapping]] [-R[n]] [-X[n]] [-h string] [-T[tname]] [-Z[n]]\n" +
				"               [name[=value]...]\n" +
				"   Or: typeset [ options ] -f [name...]",
			"wait": "Usage: wait [ options ] [job ...]",
			// The letters spelled out, unlike `wait`'s and `shift`'s, which
			// really do say "[ options ]". Measured with `jobs -Q`.
			"jobs":  "Usage: jobs [-lnp] [job ...]",
			"shift": "Usage: shift [ options ] [n]",
			"unset": "Usage: unset [-nfv] name...",
			// This engine spells the limits as one word rather than as a
			// letter each. Measured with `ulimit -Q`, which is the refusal
			// that had been printing its complaint with no usage under it.
			"ulimit": "Usage: ulimit [-HSalimits] [limit]",
		},
		// `ulimit -a`, row for row as the engine writes it. The rows this
		// platform's engine calls unsupported, and the constant pipe and
		// socket buffers, are fixed text rather than resource limits.
		UlimitListing: []interp.UlimitListingRow{
			{Prefix: "address space limit (Kibytes)  (-M)  ", Res: interp.ResourceAddressSpace, Scale: 1024},
			{Prefix: "core file size (blocks)        (-c)  ", Res: interp.ResourceCore},
			{Prefix: "cpu time (seconds)             (-t)  ", Res: interp.ResourceCPUTime, Scale: 1},
			{Prefix: "data size (Kibytes)            (-d)  ", Res: interp.ResourceData, Scale: 1024},
			{Prefix: "file size (blocks)             (-f)  ", Res: interp.ResourceFileSize},
			{Prefix: "locks                          (-x)  ", Fixed: "not supported"},
			{Prefix: "locked address space (Kibytes) (-l)  ", Res: interp.ResourceLockedMemory, Scale: 1024},
			{Prefix: "message queue size (Kibytes)   (-q)  ", Fixed: "not supported"},
			{Prefix: "nice                           (-e)  ", Fixed: "not supported"},
			{Prefix: "nofile                         (-n)  ", Res: interp.ResourceOpenFiles, Scale: 1},
			{Prefix: "nproc                          (-u)  ", Res: interp.ResourceProcesses, Scale: 1},
			{Prefix: "pipe buffer size (bytes)       (-p)  ", Fixed: "512"},
			{Prefix: "max memory size (Kibytes)      (-m)  ", Res: interp.ResourceResidentSet, Scale: 1024},
			{Prefix: "rtprio                         (-r)  ", Fixed: "not supported"},
			{Prefix: "socket buffer size (bytes)     (-b)  ", Fixed: "512"},
			{Prefix: "sigpend                        (-i)  ", Fixed: "undefined"},
			{Prefix: "stack size (Kibytes)           (-s)  ", Res: interp.ResourceStack, Scale: 1024},
			{Prefix: "swap size (Kibytes)            (-w)  ", Fixed: "not supported"},
			{Prefix: "threads                        (-T)  ", Fixed: "not supported"},
			{Prefix: "process size (Kibytes)         (-v)  ", Res: interp.ResourceAddressSpace, Scale: 1024},
		},
		PrintfUsage:           "Usage: printf [ options ] format [string ...]",
		PrintfUsageUnprefixed: true,
		TrapBadSignal:         "trap: %[1]s: bad trap",
		KillNoSuchProcess:     "kill: %[1]s: no such process",
		KillNotPermitted:      "kill: %[1]s: permission denied",
		KillInvalidSignal:     "kill: %[1]s: unknown signal name",
		KillNotAPid:           "kill: %[1]s: Arguments must be %%job, process ids, or job pool names",
		KillUsage:             kshKillUsage,
		// An unknown option is a usage error to ksh93 in both senses: it prints
		// the usage after the complaint, and it reports the usage status where
		// an unknown signal *name* reports 1.
		KillIllegalOption:         "kill: -%[1]s: unknown option\n" + kshKillUsage,
		KillMissingSignalArgument: "kill: %[1]s: signame argument expected\n" + kshKillUsage,
		KillUsageStatus:           2,
		KillBadOptionStatus:       2,
		KillUsageUnprefixed:       true,
		KillTargetUnprefixed:      true,
		TestUnaryExpected:         "%[2]s: %[1]s: unknown operator",
		TestBinaryExpected:        "%[2]s: %[1]s: unknown operator",
		TestIntegerExpected:       "%[2]s: %[1]s: integer expected",
		TestTooManyArguments:      "%[2]s: too many arguments",
		TestOperandExpected:       "%[2]s: argument expected",
		TestMissingBracket:        "[: ']' missing",
		OptionListingHeader:       "Current option settings",
		OptionListingWidth:        25,
		PlusOListsActive:          true,
		// Labeled lines, one figure each, and no children's times at all —
		// genuinely less information than the other three report.
		TimesLayout:   interp.TimesUserAndSystem,
		TimesDecimals: 2,
		// The `time` keyword's report is bash's shape with two decimals —
		// and a bare `time` reports the shell's own user and sys, no real,
		// where bash reports a run of nothing.
		TimeDecimals: 2,
		TimeBare:     interp.TimeBareShellUserSys,
	}
}

// Apply adjusts the substrate to ksh93: one removal and one addition.
//
// Which builtins a shell provides is neither grammar nor a conflict of
// meaning, so it is not a Dialect flag or a Semantics axis. It is what the
// extension seam is for, and a dialect uses it exactly as anything else built
// on the substrate would — the difference being that this one also takes
// something away.
//
// `local` goes because ksh93 is the only shell in the panel without it: it
// spells the same idea `typeset` and reports `local` as a command that was not
// found, so a script finding it here would be relying on something the real
// shell does not have.
//
// `source` arrives because ksh93 has it as a synonym for `.` and dash does
// not, which makes the name a dialect's answer. It is the same function under
// a second name rather than a second implementation.
func Apply(r *interp.Runner) {
	// The `set -o` names beyond the ones every shell has.
	r.AddSetOptions(
		"braceexpand",
		"histexpand",
		"keyword",
		"pipefail",
		"privileged",
		"trackall",
	)
	// ksh93 has a `builtin` of its own and it is a different command: it
	// *registers* builtins rather than running one. With no operands it
	// lists the table; each operand is a name to add, and one that is not
	// already a builtin here is not found, at 1 — with the builtin's own
	// name as the whole prefix, measured.
	r.Register("builtin", func(rr *interp.Runner, _ context.Context, args []string) int {
		if len(args) == 0 {
			for _, name := range rr.BuiltinNames() {
				_, _ = fmt.Fprintln(rr.Stdout, name)
			}
			return 0
		}
		status := 0
		for _, name := range args {
			if _, ok := rr.Builtin(name); !ok {
				_, _ = fmt.Fprintf(rr.Stderr, "builtin: %s: not found\n", name)
				status = 1
			}
		}
		return status
	})
	// No `compgen` here; it is bash's alone.
	r.Unregister("compgen")
	r.Unregister("complete")
	// And neither `mapfile` nor its other name; both are bash's alone.
	r.Unregister("mapfile")
	r.Unregister("readarray")
	// This shell has no `enable`.
	r.Unregister("enable")
	r.SetDynamic("RANDOM", func(*interp.Runner) string { return interp.Randoms() })
	// Three decimal places, where the two other shells that have SECONDS
	// report whole seconds.
	r.SetDynamic("SECONDS", func(rr *interp.Runner) string {
		return strconv.FormatFloat(rr.SecondsFrom(), 'f', 3, 64)
	})
	r.Unregister("local")
	if dot, ok := r.Builtin("."); ok {
		r.Register("source", dot)
	}
	// `integer` is `typeset` with the type already decided, and it is one of
	// the two shells that has the word: zsh's own `add-zsh-hook` opens with
	// `integer del list help`, which is why a shell claiming to be either of
	// them needs it. Registered rather than built here, so that both
	// dialects get the *same* declaration — see interp/integerbuiltin.go.
	r.Register("integer", interp.IntegerBuiltin())
	// And the assignment rule follows the name the way it follows `declare`:
	// `integer n=5+2` is a declaration's operand and not a word to split.
	r.SetDeclaring("integer")
	// ksh93's own spellings of "what would this run" and "write this out",
	// both pervasive in real ksh scripts. See whence.go and print.go.
	registerWhence(r)
	registerPrint(r)
}
