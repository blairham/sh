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
	// `$[expr]`, the older spelling of `$((expr))`. Measured
	// 2026-09-06: `echo $[1+1]` is 2 here and in the 3.2 macOS ships,
	// and the same text is the literal `$[1+1]` in ksh93 and dash.
	// bash has documented it as deprecated for years and both builds
	// in the panel still take it (#900).
	d.DollarBracketArith = true
	// A double quote inside an arithmetic expression is taken out of the
	// text before anything reads it. Measured 2026-09-10 in 5.3.15 and as
	// `sh`: with `n=5`, `$(( "1" + 1 ))` is 2, `$(( "n" + 1 ))` is 6 and
	// `$(( 1 + "2" ))` is 3 — and `$(( 1"0" ))` is 10, which is what says
	// the bytes are gone rather than stepped over: ksh93 and zsh read the
	// first three the same way and refuse that one. Its failures quote back
	// the text without the quotes in it, `$(( "1" "2" ))` being reported
	// against `1 2`, which falls out of the same removal.
	//
	// 3.2.57 refuses all four with `syntax error: operand expected`, so
	// this is a version line inside one lineage; the preset is the current
	// build (#1223).
	d.ArithDoubleQuote = syntax.ArithDoubleQuoteRemoved
	// bash expands them interactively and needs `shopt -s expand_aliases`
	// otherwise, which is not modeled yet — so no route, rather than a route
	// this shell only takes with an option set. Measured on all three.
	d.ExpandAliases = syntax.RouteOnNoRoute
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
	// A `${…}` operand may carry a process substitution here, and only here.
	// Measured 2026-09-05 across the panel: `${u:-<(:)}` is a path in bash
	// 3.2 and 5.3 and the five characters `<(:)` in ksh93, zsh and dash —
	// which is not for want of the construct, since ksh93 and zsh both have
	// it in an ordinary word. So it is a question about the position, and
	// bash is the only column that answers yes.
	d.ProcessSubstitutionInParamOperand = true
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
	// A here-document inside parentheses that hold a program ends at the
	// closing one: `v=$(cat <<EOF` / `a` / `EOF)` is accepted and `v` is `a`.
	// dash and zsh read the body from the whole input instead, so the `)`
	// goes into it and the construct is never closed (#963).
	d.HeredocEndsAtClosingParen = true
	d.FunctionKeywordParens = true
	// A name followed by `(` is a function definition here, whether or not
	// the `)` comes next.
	d.FuncDefAtParen = true
	// And having committed, the body must be compound: `f() echo hi` is a
	// syntax error here and a one-command function in the other three.
	d.FuncBodyMustBeCompound = true
	// A `function` keyword whose name is not a name parses, the complaint
	// coming when the definition is reached — so `bash -n` accepts a script
	// this used to refuse, and `function _p_${w} { … }` names the word it
	// disliked instead of saying only that a name was expected.
	// interp.Semantics.FunctionNameWhenTheDefinitionRuns is what happens
	// then (#1296).
	d.FunctionNameCheckedWhenTheDefinitionRuns = true
	// And inside `[[ ]]`, which is the only place bash reads them.
	d.ExtendedPatternInCondition = true
	// `[[ -v name ]]`, which asks whether a parameter is set. Not core: bash
	// 3.2 is the one panel shell with `[[ ]]` and without the operator, and it
	// cannot read the line at all — `conditional binary operator expected`.
	// This preset is 5.3, which has it.
	d.ParameterIsSetTest = true
	// A bare `|` in a `=~` operand belongs to the regular expression.
	d.RegexTakesAlternation = true
	// `time -p`, the POSIX report format. bash and ksh93 read the flag;
	// zsh leaves `-p` to the pipeline, which is why it is not core.
	d.TimePosixFlag = true
	// `a |& b` — a pipe carrying the left side's standard error along with
	// its standard output. A bash 4 feature: bash 3.2 lexes the two bytes as
	// a bar and an ampersand and refuses the line, which is why this is not
	// core and why the three bash columns of a measurement do not agree.
	d.PipeBothStreams = true
	// A `for` or `select` whose variable is not a name parses here, and the
	// complaint comes when the loop is reached — so `bash -n` accepts a
	// script this used to refuse, which is what a CI syntax check runs.
	// interp.Semantics.ForNameWhenTheLoopRuns is what happens then (#1110).
	d.ForNameCheckedWhenTheLoopRuns = true
	// And where the variable is missing outright, the newline this shell
	// terminates its input with is the token left standing: `for` alone is
	// `syntax error near unexpected token `newline'` rather than the
	// unfinished-construct wording a bare `while` gets (#1319).
	d.ForNameEndOfInputIsANewline = true
	// `coproc cat` with the near ends in COPROC. zsh's coprocess speaks
	// `print -p` rather than an array and is a different feature.
	d.Coproc = true
	d.CoprocName = true
	// And the way that array is closed: `exec {COPROC[1]}>&-` names the
	// element holding the feed. Not core because zsh has the `{name}` token
	// and still reads a subscripted one as a word.
	d.FdVariableSubscript = true
	// `exec 10>f` names descriptor ten. bash alone reads a number of more
	// than one digit there; to the other three the digits are a word, so
	// that line runs a command called `10`.
	d.MultiDigitFdNumber = true
	return d
}

// Semantics is what bash 5 means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	// An unquoted list is joined on the first character of IFS and split
	// back, which is what makes `IFS=:; set -- x "" y; printf "[%s]" $@`
	// three fields here and two in ksh93 and dash. The join is also what
	// loses a *trailing* empty element — `set -- x y ""` is `x:y:`, and a
	// trailing separator makes no field — so this one answer is what the
	// panel's two shapes of disagreement both come from.
	s.UnquotedListJoinsOnIFS = interp.Yes
	// An associative array's subscript is a quoting context here: the key is
	// the text inside its quotes, so `m["k"]=W` stores under `k` and
	// `${m["k"]}` reads it back. zsh takes the subscript as written and
	// stores under the three characters.
	s.SubscriptIsAQuotingContext = interp.Yes
	// The other join, and the opposite answer: where an unquoted `@` list
	// reaches a context that keeps no fields, this shell rejoins it on a
	// hard space rather than on IFS. `IFS=-; a=(x y z); v=${a[@]}` is
	// `x y z` here and `x-y-z` in zsh, and the same in a `case` subject, a
	// `[[ ]]` operand and a here-document body. The `*` spelling is core and
	// does not come through here — `v=${a[*]}` is `x-y-z` in this shell.
	s.UnsplitAtListJoinsOnIFS = interp.No
	// The panel's holdout on the login profile, measured on all four
	// non-interactive routes and in both bash 5.3 and the 3.2 macOS ships:
	// `exec -a -bash bash script.sh` reads neither ~/.bash_profile nor
	// /etc/profile, where dash, ksh93 and zsh all read theirs. Being a login
	// shell is not the part it declines — `shopt login_shell` is on — it is
	// that a non-interactive shell reads no startup file unless `--login`
	// was written out, and this front end has no `--login` to write.
	s.LoginProfileWhenNonInteractive = false
	// And the file it reads instead, on those same non-interactive routes.
	// Measured 2026-09-05 with a scratch HOME: `BASH_ENV=f bash script.sh`,
	// `bash -c` and a program piped in all source f, in bash 5.3 and in the
	// 3.2 macOS ships, where dash, ksh93 and zsh do nothing with the name.
	// It is not read at a prompt, which is where `$ENV` and a file of bash's
	// own name take over, and not read in POSIX mode — `bash --posix` and
	// bash invoked as `sh` read neither this nor `$ENV`.
	s.NonInteractiveStartupVariable = "BASH_ENV"
	// And the files it reads by name. Measured 2026-09-05 through a
	// pseudo-terminal with a scratch home directory holding a marker for
	// every name any shell in the panel reads.
	//
	// The profile is a chain and exactly one link of it runs: with all three
	// present bash reads `~/.bash_profile`, with that one absent it reads
	// `~/.bash_login`, and with both absent `~/.profile`. bash 3.2 agrees.
	s.LoginStartupFiles = ".bash_profile .bash_login .profile"
	// `bash -i` reads `~/.bashrc` and nothing else, and does not read `$ENV`
	// — that is the same file under POSIX mode, where `~/.bashrc` goes
	// unread; the front end asks the mode rather than the dialect.
	s.InteractiveStartupFile = ".bashrc"
	// The panel's holdout on ordering, and the reason every bash tutorial
	// tells a person to source `~/.bashrc` from their `~/.bash_profile` by
	// hand: `bash -l -i` reads the profile and stops. zsh reads both.
	s.InteractiveStartupFileWhenLogin = interp.No
	// The escape hatches, all three spelled long. `--rcfile` and
	// `--init-file` are the same option, measured to behave identically, and
	// both are carried because a person's muscle memory has one of them.
	s.StartupFileOptions = interp.StartupFileOptions{
		Login:               "-l --login",
		SuppressLogin:       "--noprofile",
		SuppressInteractive: "--norc",
		NameInteractive:     "--rcfile --init-file",
	}
	// What `bash --version` writes. Measured 2026-09-11: the real shell
	// answers on standard output at status 0, and the first line is the one
	// scripts read — see version, in prelude.go, for why the tag is there.
	s.VersionOption = interp.VersionOption{Spellings: "--version", Text: versionLine()}
	// `-c` and `-s` together: the command string names the operands here,
	// so `sh -sc CMD name a` has `$0` of `name` and one parameter — the
	// same answer in the 3.2 macOS ships. ksh93 and zsh let `-s` name them.
	s.StdinOptionNamesTheOperands = interp.No
	s.CommandNotFoundStatusIsNotFound = interp.No
	s.SetFTurnsOffGlobbing = interp.Yes
	// The one shell in the panel that selects an editing mode for itself.
	// Measured 2026-09-11 on 5.3.15 and on the 3.2 macOS ships, and under an
	// `argv[0]` of `sh` as well: `set -o` reports `emacs off` in a script and
	// `emacs on` under `-i`, with or without a terminal.
	s.InteractiveSelectsEmacs = interp.Yes
	// `set +B` stops `{a,b}` expanding and `set -B` puts it back — measured
	// 2026-09-11 on bash 5.3.15, both directions, and the letter leaves `$-`
	// while it is off. So it is not one-way the way `noexec` is (#1856).
	s.SetBTurnsOffBraceExpansion = interp.Yes
	// Measured: `echo $-` reports `hB` — hashall and braceexpand — under
	// -c, a script file and standard input alike, before the letters that
	// describe the route.
	s.DefaultOptionLetters = "hB"
	// And `hiBH` for `-i script.sh`, so the interactive set adds the
	// history-expansion letter and keeps everything else — measured
	// 2026-09-05 on bash 5.3.15, where `set -o` reports `histexpand on`
	// and `history on` there and both off for a script. `i` is not written
	// here: it is unanimous and comes from the runner.
	//
	// bash 3.2 answers `hiB` for the same invocation and `hiBHc` for `-i
	// -c`, so it disagrees with itself by route as well as with its later
	// build. 5.3 is the panel member that counts, as it is everywhere else.
	s.InteractiveOptionLetters = "hBH"
	// `bash -c 'echo $-'` reports `hBc`; ksh93 agrees and dash and zsh do
	// not. The `s` of the standard-input route is not added under `-c`
	// here — ksh93 alone does that.
	s.CommandStringShowsCInDollarDash = interp.Yes
	s.LoginShowsLInDollarDash = interp.No
	s.CommandStringShowsSInDollarDash = interp.No
	s.ArrayScalarIsTheWholeArray = interp.No
	// And the one element a plain `$m` on a keyed table gives is the one
	// keyed `0`, which is nothing at all where no such key was written.
	s.KeyedTableScalarIsTheFirstValue = interp.No
	s.ArrayNameWithoutSubscriptIsTheList = interp.No
	// A subscript inside a literal is an expression: `a=([1+1]=c)` lands at 2.
	s.ArrayLiteralSubscriptIsAKey = interp.No
	// The `@` family's letter is checked against the value rather than
	// against the spelling: `${u@QQ}` on an unset name is empty at status 0
	// and the same word on a set one is a bad substitution. Measured on
	// four spellings and on an empty array, which counts as no value.
	s.TransformLetterCheckedOnlyWhenValued = interp.Yes
	s.AssignmentUpdatesPipelineStatus = interp.Yes
	s.TestAndArithmeticUpdatePipelineStatus = interp.Yes
	// And the record for one of those two is written *after* a `!` in front
	// of it: `false | true; ! [[ a = a ]]` leaves 1 here and 0 in zsh.
	// Measured 2026-09-11 on 5.3.15 and 3.2.57 alike. An ordinary command is
	// not affected — `! false` records 1 in both — so this is a rule about
	// the construct rather than about negation (#1513).
	s.NegatedTestRecordsThePostNegationStatus = interp.Yes
	// A compound's body does not decide whether it writes the record here.
	// This shell's own rule is a third thing rather than the opposite of
	// zsh's — it keeps what the last pipeline *inside* the compound left, so
	// `if false; then :; fi` holds 1 and `case a in b) :;; esac` leaves the
	// record alone — and that is #2016 rather than this axis, which no is
	// what this shell already did.
	s.CompoundBodyDecidesThePipelineStatusRecord = interp.No
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
	s.ListingControlEscape = interp.ControlEscapeOctal
	s.ExportListing = interp.DeclareListingClustered
	s.ReadonlyListing = interp.DeclareListingClustered
	// The bare form is this shell's `-p` form exactly, in both builds and in
	// the POSIX mode: `export` writes `declare -x V="a b"` however it is asked.
	s.BareDeclarationListing = interp.DeclareListingClustered
	// Two attribute letters on a listing with no names: the kind letters
	// narrow and the rest join. Measured 2026-09-10 over a table holding an
	// array, an integer array, an exported array, an association, an
	// integer, a readonly and an exported name — `declare -ir` writes the
	// integers *and* the read-only names, where `declare -ai` writes only
	// the array that is also an integer and `declare -aA` writes nothing.
	s.DeclarationListingFilter = interp.DeclarationFilterKindNarrowsAny
	s.DeclarePrintReportsAMissingName = interp.Yes
	s.TrapActionIsParsedWhenSet = interp.No
	s.TrapParseFailureNamesWhereItFired = interp.No
	s.SymbolicMaskTakesMoreThanOneOperator = interp.Yes
	s.SymbolicMaskSetsWithoutAWho = interp.Yes
	s.SymbolicMaskWhoAloneSetsIt = interp.No
	s.SymbolicMaskTakesTheSetuidLetter = interp.Yes
	s.SymbolicMaskTakesTheStickyLetter = interp.Yes
	// No dash word is an option: `shift -x` complains about a number and
	// `shift -1` is a count out of range. The marker is honored all the same.
	s.ShiftOptionWords = interp.ShiftOptionWordsNone
	s.ShiftDoubleDashEndsOptions = interp.Yes
	s.ShiftNamesAreArrays = interp.No
	s.ShiftNegativeIsOutOfRange = interp.Yes
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
	// `echo -e '\u0041'` and `echo -e '\U00000041'` are both `A` in 5.3.15,
	// and a run with no digit at all is left as written with a complaint —
	// the leaving is what the second axis records here.
	s.EchoExpandsUnicodeEscapes = interp.Yes
	// A code point the locale cannot hold leaves the escape standing,
	// normalized to four or eight upper-case digits, and the command carries
	// on. Measured 2026-09-11 under `LC_ALL=C` (#1851).
	s.UnicodeEscapeOutsideTheLocale = interp.OutsideLocaleEscapeWritten
	s.EchoEmptyHexDigitRunIsNul = interp.No
	// Both spellings of the escape character, which is this shell alone in
	// the panel: ksh93 has only `\E` and zsh only `\e`.
	s.EchoExpandsEscEscape = interp.Yes
	s.EchoExpandsCapitalEscEscape = interp.Yes
	// read takes -r and -s plus the argument letters: -a names the array in
	// the option's argument, -d a delimiter, -i the text a line editor would
	// be seeded with, -n and -N the two counts, -p a prompt for when the
	// input is a terminal, -t a timeout and -u a descriptor. A short -n
	// keeps its text and reports 1; a short -N keeps its text too.
	s.ReadOptions = "rsa:d:i:n:N:p:t:u:"
	// `-n` unsets through a name reference. bash 3.2 does not have it —
	// `unset -n x` is an invalid option there and under `--posix` — so
	// this is 5.3's set, which is the binary the panel measures.
	s.UnsetOptions = "vfn"
	s.ReadZeroTimeout = interp.ReadZeroTimeoutPolls
	s.ReadPartialCountSucceeds = interp.No
	s.ReadExactCountKeepsPartial = interp.Yes
	s.ReadTimeoutKeepsWhatArrived = interp.Yes
	// A directory the PATH search walked past leaves no trace: with nothing
	// runnable anywhere, bash says the name was never found at all.
	s.DirectoryOnPathIsACandidate = interp.No
	// set -E and -T carry the traps set -Eeuo pipefail scripts rely on.
	s.SetHasTraceLetters = interp.Yes
	// `set -t`, which this shell also spells `set -o onecmd`: the line that
	// set it finishes and nothing more is read. Measured on a script file, on
	// standard input and at the invocation — `bash -t script.sh` runs the
	// first line of it and stops.
	s.SetHasTheTLetter = interp.Yes
	// And the one route it does not reach. Measured: a `-c` string that turns
	// the option on runs to its end anyway, with `$-` showing `t` throughout.
	// The other shell with the letter stops there, which is what makes this
	// an axis rather than a rule.
	s.OneCommandStopsACommandString = interp.No
	s.JobControlAbsenceIsReportedFirst = interp.Yes
	// A stopped job holds the exit back: the shell says so and stays,
	// and the next attempt leaves. Measured through a pseudo-terminal for
	// `exit` and for ^D alike.
	s.StoppedJobsHoldTheExit = interp.Yes
	// And writes the job table under the sentence while `checkjobs` is on —
	// measured, `[1]+  Stopped ./ticker` and `[2]-  Running sleep 40 &`
	// below `There are stopped jobs.`, and nothing below it with the option
	// off.
	s.HeldExitListsTheJobs = interp.Yes
	// `autocd` says what it did before doing it: with the option on, a bare
	// `subdir` writes `cd -- subdir` and then moves. Measured 2026-09-08
	// through a pseudo-terminal against bash 5.3.15, which is the only route
	// the name works on at all — zsh, the other shell with the option, moves
	// in silence.
	s.AutoCdAnnouncesTheSubstitution = interp.Yes
	s.TildePlusMinusExpands = interp.Yes
	s.UnderscoreTracksTheLastArgument = interp.Yes
	// Alone in the panel, bash writes `$_` before the first command runs,
	// and what it writes is argv[0]: the same binary reached through a
	// symlink named `sh` writes `sh`. An `_` the environment carried wins
	// over it, which is the default answer and is left alone here.
	s.UnderscoreStartsAtTheInvocation = interp.Yes
	// hash counts builtins and functions and announces its empty table.
	s.FatalErrorStatusIsOne = interp.Yes
	s.HeredocExpandsInTheCommandsProcess = interp.Yes
	s.RedirectTargetExpandsInTheCommandsProcess = interp.Yes
	// A loop variable that is not a name is checked when the loop runs here,
	// and the loop fails while the script carries on: `for $n in a b` prints
	// the complaint, the loop reports 1, and `echo "st=$?"` after it runs
	// and the script exits 0. POSIX mode moves it — see SetPosixMode — which
	// is the whole of the bash-as-`sh` column and is a mode rather than a
	// build: bash 3.2 and 5.3 agree under their own names.
	s.ForNameWhenTheLoopRuns = interp.ForNameFailsTheLoop
	// And the same shape for a function definition's name: the definition
	// fails, reports 1 and the script carries on — `function _p_${w} { :; };
	// echo st=$?; echo after` prints the complaint, `st=1` and `after`, and
	// exits 0. POSIX mode moves it to the syntax-error status exactly as it
	// moves the loop's, which is the bash-as-`sh` column.
	s.FunctionNameWhenTheDefinitionRuns = interp.FuncNameFailsTheDefinition
	s.ArithNameValueRecurses = interp.Yes
	// And an unset name found that way is a zero like any other unset name:
	// `x=abc; $((x+1))` is 1 and the script runs on. Measured 2026-09-11 —
	// ksh93 is the panel's holdout, where it is a fatal `parameter not set`.
	s.ArithRecursedNameMustBeSet = interp.No
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
	// Braces finish before parameters begin, so a range cannot be built
	// from one: `n=3; echo {1..$n}` is the literal `{1..3}`.
	s.BraceRangeEndpointsExpanded = interp.No
	s.BracketCaretNegates = interp.Yes
	// One character-class name beyond the twelve POSIX ones. Measured
	// 2026-09-10, `[[ $c = [[:ascii:]] ]]` a character at a time: `a` is in
	// it, `é` and `日` are not, and bash 3.2 answers the same. ksh93 and dash
	// have no such name and match nothing with it, silently, which is what
	// this shell does with any other name outside the roster.
	s.PatternClasses = "ascii"
	s.RegexQuotingMakesLiteral = interp.Yes
	// A process substitution may stand as a condition's operand here, and is
	// performed there: `[[ $v == <(cmd) ]]` runs cmd and matches against the
	// path, which is false for anything a script would have written down.
	// This shell alone — zsh refuses the word and ksh93 will not read it.
	s.ProcessSubstitutionInCondition = interp.Yes
	// bash diagnoses an unreadable condition operand and carries on: the
	// condition is false at status 1 and the next command runs. zsh and
	// ksh93 abandon the input there.
	s.ConditionArithmeticErrorIsFatal = interp.No
	s.ShiftPastEndFatal = interp.No
	// A declaration will not shadow a frozen name here: `typeset -r x=1;
	// f() { local x=2; echo $x; }` says `local: x: readonly variable`, then
	// prints the *outer* 1, and the function runs on — the builtin reports
	// 1 and nothing is abandoned. zsh takes the shadow instead. Measured
	// over `local x=2`, `local x`, `typeset x=3`, `local -r x=4` and
	// `local y=1 x=5 z=2`, all five refused here and all five taken there,
	// which is what makes it one field rather than five.
	s.DeclarationMayShadowAReadonly = interp.No
	// And the attribute cannot be taken off again: `typeset +r x` on a
	// frozen name is refused with the builtin named, reports 1 and leaves
	// the freeze standing. True of bash 3.2 and of the same binary under
	// argv[0] of `sh` alike, and of a name the shell was *started* frozen
	// with — `typeset +r UID` draws the identical refusal.
	s.ReadonlyAttributeCanBeRemoved = interp.No
	s.DeclaredNameWithoutValueIsEmpty = interp.No
	// The export letter says nothing about scope here: `declare -x v=1`
	// inside a function is an ordinary local.
	s.ExportLetterDeclaresAGlobal = interp.No
	// And a valueless declaration of a standing name is silent.
	s.ValuelessDeclarationOfAHeldNameListsIt = interp.No
	// A plain word declared over a name holding an array replaces it and
	// says nothing.
	s.ScalarOverACompoundIsAnInconsistentType = interp.No
	// `readonly -a a` freezes the name and records no kind: it lists as
	// `declare -r a`, with no `a` in the cluster.
	s.ReadonlyRecordsTheCompoundAttribute = interp.No
	// One reader for both: `$((010))` and `typeset -i d=010` are eight
	// alike, where ksh93 answers eight and ten.
	s.IntegerAssignmentReadsALeadingZeroAsDecimal = interp.No
	// And one reader for a value too: `k=010; $((k))` is eight here.
	s.ArithStoredValueReadsALeadingZeroAsDecimal = interp.No
	// An attribute added to a name that already holds a value waits for the
	// next assignment: `FOO=bar; typeset -i FOO` still reads `bar`, and
	// `d=MiXeD; typeset -u d` still reads `MiXeD`. ksh93 and zsh re-read on
	// the spot and store 0 and MIXED.
	s.AttributeRereadsTheValueItFinds = interp.No
	s.InheritedValueSurvivesADeclaredType = interp.Yes
	s.CompoundElementsGoThroughTheAttribute = interp.Yes
	s.CompoundAttribute = interp.CompoundAttributeKeepsTheElements
	// The converse: an array or table letter given to a name already holding
	// a scalar promotes that value to the first element. Measured 2026-09-08,
	// `b=1; typeset -a b` lists as `declare -a b=([0]="1")` and `a=1;
	// typeset -A a` as `declare -A a=([0]="1" )`, and `$b` still reads `1`
	// under both. bash 3.2.57 answers the array letter the same way and has
	// no `-A` at all.
	s.ScalarUnderAnArrayDeclaration = interp.ScalarUnderACompoundBecomesTheFirstElement
	s.ScalarUnderATableDeclaration = interp.ScalarUnderACompoundBecomesTheFirstElement
	// `a=(1 2); a+=x` joins the first element and leaves the rest standing —
	// `declare -a a=([0]="1x" [1]="2")`, two elements, in 5.3.15, in the same
	// binary under argv[0] of `sh` and in 3.2.57. The value lands at the base
	// whether or not an element is there: `a=([5]=q); a+=x` is
	// `([0]="x" [5]="q")`.
	s.ScalarAppendedToAnArrayBecomesANewElement = interp.No
	// `a=(1 2 3); a=x` is `declare -a a=([0]="x" [1]="2" [2]="3")`: the
	// first element written, the rest standing, still an array of three.
	// The table spelling answers the same — `m=x` over `declare -A m` is
	// `([0]="x" [k]="v" )`. Both hold in 5.3.15, in the same binary under
	// argv[0] of `sh`, and in 3.2.57.
	s.ScalarAssignedOverACompoundReplacesTheName = interp.No
	s.ArrayLiteralAssignmentStartsTheNameOver = interp.No
	// `local u` hides the caller's `u` — the local exists unset.
	s.ValuelessDeclarationHidesTheOuterValue = interp.Yes
	s.TypesetLocalNeedsKeywordFunction = interp.No
	s.ReadonlyReassignmentFatal = interp.No
	// `${1:=abc}` is refused rather than assigned: `$1: cannot assign in
	// this way`, in 5.3.15, in the same binary under argv[0] of `sh` and in
	// 3.2.57 alike. Only zsh assigns (#1541).
	s.AssignThroughExpansionMayNameAPositional = interp.No
	// An assignment prefix to a frozen name is reported and costs nothing
	// at all: measured 2026-09-11 in 5.3.15 and in 3.2.57, `readonly x=1;
	// x=2 /bin/echo RAN; echo after` prints the complaint, `RAN` and
	// `after` at status 0 — the command runs with the name still holding
	// its old value — and the same holds for a regular builtin, a special
	// one, a function and `command`. Uniform across every kind, which is
	// what separates this shell from the two that answer by kind (#1219).
	s.PrefixToARegularBuiltinIsRefused = interp.Yes
	s.PrefixRefusalFatality = interp.PrefixRefusalNeverFatal
	s.PrefixRefusalCostsTheCommand = interp.No
	// A failed expansion gives up the line here and the shell carries on at
	// the next one, which is this shell alone among the four. Measured over
	// both routes and both separators — see the axis for the 2x2 — on a bad
	// substitution, a division by zero, a bad subscript and an arithmetic
	// expression the parser refused.
	s.FailedExpansionAbandonsTheLine = interp.Yes
	s.ReadonlyReassignmentByDeclarationFatal = interp.No
	s.BuiltinSyntaxErrorFatal = interp.No
	// An error inside a file `.` read ends the shell here, not just the file:
	// measured, a sourced file whose third line is `echo X${NOPE}` under
	// `set -u` prints nothing after it in the sourcing file either, and bash
	// exits 1. Same under argv[0] of `sh` and in bash 3.2.
	s.FatalErrorEndsBorrowedTextOnly = interp.No
	// `${x?word}` is an error here rather than a request to stop, which shows
	// at the one boundary this shell does give up a file at: measured, a
	// `$BASH_ENV` whose second line is `echo X${NOPE?msg}` stops there and
	// the script the shell was started for still runs.
	s.ParamErrorIsAnExitRequest = interp.No
	s.DotMissingFileFatal = interp.No
	s.DotPassesArguments = interp.Yes
	// A directory operand is an error here and success in zsh and dash.
	// Measured, `. ./` is `bash: line 1: .: ./: is a directory` at 1, and
	// the script carries on — the same status and the same survival a file
	// that is not there gets, with a sentence of its own.
	s.DotDirectoryOperandIsAnError = interp.Yes
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
	// A substituted element keeps the status its death produced, 128 plus
	// the signal, the same as anywhere else.
	s.PipefailSubstitutesTheBareSignal = interp.No
	s.ErrexitSeesPipefailFailure = interp.Yes
	s.UnterminatedBracket = interp.BracketLiteral
	// A longest prefix trim over an alternation takes the longest arm,
	// whichever order the arms were written in. Measured 2026-09-11 with
	// extended patterns on, `x=abc`: `${x##@(a|ab)}` and `${x##@(ab|a)}`
	// are both `c` in 5.3.15 and in 3.2.57.
	s.LongestPrefixTrimTakesTheWrittenArm = interp.No
	s.StatusArgument = interp.StatusArgNumeric
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
	s.RedirectsUseEveryTarget = interp.No
	s.KillStatus = interp.KillStatusAnySuccess
	s.SubshellJobTable = interp.SubshellJobsKeptOutsideACompound
	s.PrintfEmptyIsNotANumber = interp.Yes
	s.PrintfReportsBadNumber = interp.Yes
	s.PrintfBackslashC = interp.PrintfBackslashCLiteral
	// A format that ends inside a conversion is an error here, with a
	// second wording of its own — see PrintfMissingVerb.
	s.PrintfUnfinishedConversionIsAPercent = interp.No
	// `\x41` is an `A`, and at most two digits: `\x0ff` is 0x0f then an
	// `f`. A `\x` with no digit after it stands as written, with a warning
	// on standard error and a status that is still zero.
	s.PrintfHexEscape = interp.PrintfHexEscapeByte
	// A `%b` argument reads the same `\x` a format does here, and both of
	// the escape-character spellings: `\e` and `\E` are both 0x1b.
	s.PrintfBHexEscape = interp.PrintfHexEscapeByte
	// `\u0041` is an `A` and `\U00000041` is the same `A`, at both sites:
	// four digits after the one and eight after the other, fewer accepted,
	// and the value written as UTF-8. A `\u` with no digit after it stands
	// as written, with a warning on standard error and a status that is
	// still zero — the same shape this shell's `\x` has. bash 3.2 has none
	// of it, which is why the corpus's `bash32` column differs from the other
	// two; `bash` and `bash-as-sh` agree, the escape being 5.3's rather than
	// something argv[0] turns off.
	s.PrintfUnicodeEscape = interp.PrintfUnicodeEscapeCodePoint
	s.PrintfBUnicodeEscape = interp.PrintfUnicodeEscapeCodePoint
	s.PrintfBEscEscape = interp.Yes
	s.PrintfBCapitalEscEscape = interp.Yes
	// `printf '%b' 'a\101Z'` is `aAZ`: the octal needs no `\0` to introduce
	// it. Not this shell's answer for an `echo` argument, where `\101` is
	// four characters — the two sites are two tables.
	s.PrintfBOctalWithoutZero = interp.Yes
	// What a `\c` left goes through the conversion's field like any other
	// text: `printf '[%5b]' 'a\cb'` is `[    a`.
	s.PrintfBStopIsPadded = interp.Yes
	// `%zX`, `%ld`, `%jd` and any run of the letters, all of them read and
	// thrown away: `%hhd` with 300 is 300.
	s.PrintfLengthModifiers = interp.PrintfLengthModifiersC99
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
	s.DollarSingleCaretMeta = interp.No
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
	// A redirection that cannot be made is where bash parts from POSIX and
	// from three of the panel: `exec 3>/nope/x; echo after` complains and
	// prints `after` at status 0. It is the starting value rather than a
	// fixed one — `set -o posix` moves it, which is the whole of the
	// bash-as-`sh` column; see the posix option below.
	// bash reads the number and fails at run time if nothing is open there:
	// `10: Bad file descriptor`, status 1, and the script carries on.
	s.MultiDigitDuplicationTargetIsAnError = interp.No
	s.RedirectErrorOnSpecialBuiltinFatal = interp.No
	// `echo hi >&qq` writes the file `qq` and puts both output streams in
	// it, which is the csh spelling bash kept. A word that expanded to
	// nothing is not a name, and is refused as a descriptor instead.
	s.GreatAmpTarget = interp.GreatAmpTargetNamesAFile
	s.DuplicationTargetErrorOnABuiltinIsFatal = interp.No
	s.LocalOutsideAFunctionIsAnError = interp.Yes
	s.LocalOutsideAFunctionIsFatal = interp.No
	// bash reports every operand that is not a name, exports the ones that
	// are, and carries on with a status of 1.
	s.BadNameToDeclarationFatal = interp.No
	s.BadNameToUnsetFatal = interp.No
	// `read` is not a special builtin anywhere, so its bad name is not fatal
	// here either — `read 1bad; echo after` prints both lines.
	s.BadNameToReadFatal = interp.No
	// The refusal to unset a readonly name is reported and not fatal — and
	// this is the one of bash's `unset` answers that POSIX mode moves, which
	// is why it is not BadNameToUnsetFatal read twice. `set -o posix` makes
	// bash 5.3 stop here and changes nothing about `unset 1x`. bash 3.2 is
	// fatal in neither mode; this preset is bash 5's.
	s.UnsetReadonlyFatal = interp.No
	s.DeclarationNameOperands = interp.PlainNamesOnly
	// bash 5.3's bare `unset` checks nothing — `unset 1x`, `unset "a b"` and
	// `unset -- -` are all quiet — while `unset -v 1x` refuses. bash 3.2
	// refused all of them, so the panel's two bash columns differ here.
	s.UnsetNameOperands = interp.AnythingIsAName
	// `read` sits back on the strict answer whatever `unset` does: `read 1`,
	// `read "a b"` and `read -- -x` are each `not a valid identifier` in
	// bash 5.3 and in the 3.2 that macOS ships.
	s.ReadNameOperands = interp.PlainNamesOnly
	// And bash has no prompt operand — `read "v?p"` is the bad name `v?p`,
	// quoted back whole. The prompt is `-p` here and only `-p`.
	s.ReadPromptOperand = interp.ReadOperandIsAllName
	// bash 5.3 judges the first operand before it goes to the stream:
	// `printf 'AAA\nBBB\n' | { read 1bad; cat; }` prints both lines. bash
	// 3.2 reads first and prints only BBB, so the panel's two bash columns
	// differ here as they do on UnsetNameOperands; this preset is bash 5's.
	s.ReadRefusesABadNameBeforeReading = interp.Yes
	// A count changes nothing about which operands are judged here:
	// `read -n 3 a 1bad` refuses `1bad` and fills a with the three
	// characters, exactly as `read a 1bad` refuses it.
	s.ReadCountJudgesTheNamesAfterTheFirst = interp.Yes
	s.DeclarationTakesASubscript = interp.No
	// And a *declaration* takes one where `export` does not, which is this
	// shell alone splitting the two: `export a[1]=v` is `not a valid
	// identifier` and `typeset a[1]=v` creates the element, measured.
	s.TypesetTakesASubscript = interp.Yes
	s.UnsetTakesASubscript = interp.Yes
	// And what a declaration of an element may also do to the array: the
	// integer attribute lands on it and the element is stored converted —
	// `typeset -i a[1]=0x10` reads back 16 — and inside a function the array
	// becomes local, holding the element, with the caller's array back on
	// return. Both measured 2026-09-07.
	//
	// Semantics.ReadonlyElement is deliberately left unanswered: bash's
	// answer is a third one — `typeset -r a[1]=v` creates the array frozen
	// and empty, reports `a: readonly variable` and reports success — and
	// naming it here would need the refusal to run after the freeze rather
	// than instead of it (#1203).
	// Never asked here, because a bad name is not fatal in this shell: it
	// reports each one and declares the good ones by carrying on. The answer
	// is the one the same binary gives under an argv[0] of `sh`, where the
	// fatality is on and only the operands in front of the bad one are
	// declared.
	// `read -a` reads its edges as the ordinary field split does: a closing
	// run of separators is absorbed, whitespace or not, and a line with no
	// fields in it fills no elements at all — measured 2026-09-07, where
	// ksh93 and zsh both leave one empty element.
	s.ReadTrailingWhitespaceEndsAField = interp.No
	s.ReadNoFieldsIsOneEmptyElement = interp.No
	s.BadNameDeclaresTheOperandsAfterIt = interp.No
	s.SubscriptedOperandTakesTheIntegerAttribute = interp.Yes
	s.SubscriptedOperandTakesALocalDeclaration = interp.Yes
	// A single subscript on a name that is no array is refused rather than
	// ignored: `a=v; unset "a[1]"` says `a: not an array variable` and fails,
	// where the other shell that reads a subscript as an element says
	// nothing. `a[0]` names the string itself and takes the whole name away
	// in both — measured on 5.3.15. 3.2.57 refuses that one too, so a corpus
	// case here splits the two bash columns; this column is the graded one.
	s.UnsetSubscriptOnAScalarIsAnError = interp.Yes
	// `unset a[@]` empties the array: `a=(x y z)` comes back with no elements
	// and `${#a[@]}` is 0. Measured in both bash builds, and the same for
	// `a[*]`. A name that holds a scalar is refused rather than emptied, and
	// one that holds nothing at all is quietly left alone.
	s.UnsetArraySpan = interp.UnsetArraySpanRemovesTheElements
	// Brackets with nothing between them are named and then answered: the
	// subscript is reported and the operand is zero, and the script carries
	// on at status 0. Measured 2026-09-10 in 5.3.15 and in 3.2.57, and it
	// does not depend on the name — declared, undeclared, a table, an array
	// and a scalar all report alike, which is what separates this from zsh's
	// answer and is why the axis is not a rule about arrays.
	//
	// The real shell writes the sentence *twice* for one subscript, in both
	// builds; that is an artifact of evaluating the word twice rather than a
	// fact about the construct, and once is what this writes.
	s.EmptyArithSubscript = interp.EmptyArithSubscriptIsReported
	// The same text where a *parameter expansion* reads it, and a different
	// answer: `${a[]}` is refused outright — `[${a[]}]: bad substitution`,
	// naming the quoting run — where the arithmetic site reports and carries
	// on with zero. Identical in 3.2.57 and under argv[0] of `sh` (#1763).
	s.EmptyParamSubscriptIsAnError = interp.Yes
	// The brackets of an arithmetic subscript hold an expression, and `*` is
	// not one: measured 2026-09-11 on 5.3.15, `typeset -A m; m[k]=9;
	// $(( m[*] ))` is 0, where the expansion `"${m[*]}"` is 9.
	s.ArithWholeArraySubscriptIsTheSlice = interp.No
	// A subscript that *expanded* to nothing is the expression that is zero,
	// so `${a[$w]}` with an empty `$w` is element zero — measured 2026-09-11
	// on 5.3.15, `a=(5 6 7); w=; ${a[$w]}` is `5` at status 0, and `${a[ ]}`
	// beside it is too.
	s.EmptySubscriptTextIsAMathError = interp.No
	// Whitespace between the brackets is not that text and is not answered
	// by it: measured 2026-09-10 in 5.3.15 and in 3.2.57, `a=(1 2 3); echo
	// $(( a[ ] ))` is `1` with a clean stream — the blank expression is
	// zero, and zero names the first element — where `a[]` one character
	// shorter writes `a[]: bad array subscript` and answers with a flat
	// zero. The same split holds with the subscript as an assignment
	// target: `(( a[ ] = 9 ))` writes element zero (#1762).
	s.BlankArithSubscriptIsTheEmptyExpression = interp.Yes
	// `a[1]=(p q)` is refused and the script ends: measured 2026-09-07 in
	// 5.3.15 and in 3.2.57, which give the same sentence at status 1 and run
	// nothing after it. It does not depend on what the name holds — an array,
	// a scalar, a declared table and an unset name are all refused alike —
	// and it does not evaluate the subscript, which is quoted back as
	// written (#1330).
	s.SubscriptedArrayLiteral = interp.SubscriptedArrayLiteralRefused
	// A subscript that will not evaluate ends the script here, as a bad
	// expression does wherever one is written.
	s.BadSubscriptToUnsetFatal = interp.Yes
	// An element write over a name holding a string promotes the string
	// first, so a subscript counting back from the end finds the element it
	// just made: `a=abc; a[-1]=x` writes the first one.
	s.NegativeSubscriptCountsOverAPromotedScalar = interp.Yes
	// A negative subscript that counts back past the first element is
	// refused rather than placed in front of it, and the refusal ends the
	// script. Measured in both bash builds.
	s.NegativeSubscriptPastTheStartInserts = interp.No
	// `declare a=1; declare a+=2` is `12`: a declaration's operand carries
	// the append operator here, where the other three refuse the name `a+`.
	s.DeclarationTakesAnAppendOperand = interp.Yes
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
	// A *replacement* operand is a word of its own, so its quotes quote and
	// are removed: measured 2026-09-07 in 5.3.15 and as `sh`, `"${s/a/'$v'}"`
	// on `s=xay` and `v=VAL` is `x$vy`, the `$v` never substituted. 3.2.57
	// gives `x'VAL'y`, so this moved inside the lineage and the preset is the
	// current build. The *word* operand takes the enclosing quoting in both,
	// unanimously with the rest of the panel, which is why that one is not an
	// axis (#1209).
	s.ReplacementOperandTakesTheEnclosingQuoting = interp.No
	s.ExportCarriesFunctions = interp.Yes
	// `export -n V` takes the attribute off and leaves V set: measured, the
	// name keeps its value in the shell and stops reaching a child. bash is
	// the only shell in the panel with the letter.
	s.ExportTakesTheAttributeOff = interp.Yes
	s.AnnouncesBackgroundJob = interp.Yes
	// And it keeps announcing with the monitor off, which is measured
	// 2026-09-10 on a pseudo-terminal: `set +m` then a background job still
	// prints `[1] <pid>` here where zsh goes quiet. The other end of the job
	// is silent in every column (#1738).
	s.AnnouncesBackgroundJobWithoutTheMonitor = interp.Yes
	// bash 5.3 leaves the monitor off under `-i script.sh` with no terminal,
	// and says so twice — `cannot set terminal process group` and `no job
	// control in this shell`. It grants an explicit `set -m` there all the
	// same, which is what keeps this question and MonitorNeedsATerminal
	// apart.
	s.InteractiveMonitorNeedsATerminal = interp.Yes
	// A subshell is a process of its own here — measured, a child started
	// inside one reports the subshell rather than the shell as its parent —
	// so a signal aimed at `$$` never reaches it and it finishes its body.
	s.SubshellRunsOnAfterSignalingTheShell = interp.Yes
	// And bash is the one member of the panel that announces nothing on
	// this route. Measured on `-i script.sh` through a pseudo-terminal:
	// 5.3.15, 3.2.57 and 3.2 run as `sh` all print neither the start nor
	// the `Done` row, where the other three print at least one. It is the
	// route and not the terminal — `bash -i < script` announces both.
	s.InteractiveScriptAnnouncesJobs = interp.No
	// `$!` before any background command is *unset*, not set and empty:
	// measured, `set -u; echo "[$!]"` writes `$!: unbound variable` and stops
	// at 127 in 5.3.15, in 3.2.57 and in 3.2 run as `sh`. Without `set -u` it
	// expands to nothing, which is the other four columns' answer too, so the
	// difference only shows where a script asked to be told.
	s.LastBackgroundPidIsUnsetBeforeAnyJob = interp.Yes
	// A job started with `&` reads an empty standard input, not the shell's:
	// measured 2026-09-07, `bash -c '/bin/cat & wait; echo ---; /bin/cat' < f`
	// writes `---` and then the file's line, in 5.3.15 and 3.2.57 alike. What
	// POSIX XCU 2.9.3 specifies, and what keeps the script's own `read` from
	// losing the lines a background job would otherwise eat.
	s.BackgroundJobInput = interp.BackgroundJobInputEmpty
	// And it reads as nothing rather than as a zero: measured,
	// `echo "[$!]"` writes `[]` in all three bash columns. Stated rather
	// than left unanswered — the field is read without asking, so unanswered
	// gives the same behavior, but only a written answer says it was
	// measured.
	s.LastBackgroundPidIsZeroBeforeAnyJob = interp.No
	s.ReportsACommandKilledBySignal = interp.Yes
	s.ReportsAnyKilledPipelineElement = interp.No
	s.ChildInterruptEndsTheScript = interp.No
	// `cd ""` is refused where `HOME=` is not, which is the pair no single
	// field could express: `cd: null directory` at 1 for the operand, and
	// nothing at all at 0 for the empty HOME.
	s.CdEmptyOperandIsAnError = interp.Yes
	s.CdEmptyHomeIsAnError = interp.No
	// No substitution form here: a second operand is simply one too many,
	// and it is refused rather than ignored. Measured, `cd a b` says `cd:
	// too many arguments` and stays where it is.
	s.CdSubstitutesTheOperands = interp.No
	s.CdSubstitutionPrintsTheDirectory = interp.No
	s.CdRefusesExtraOperands = interp.Yes
	s.CdRefusesUnknownOption = interp.Yes
	s.CdHasQuietOption = interp.No
	s.CdLastPathOptionWins = interp.Yes
	s.BadSetOptionNameFatal = interp.No
	s.UnknownConditionOptionIsAStatus = interp.No
	s.ReturnOutsideAFunctionIsRefused = interp.Yes
	// `break` with no loop around it is reported and then ignored here: the
	// next command on the line runs and the status stays 0. Measured with
	// `echo t; break; echo after` — `after` prints and `$?` is 0.
	s.LoopControlOutsideALoopIsFatal = interp.No
	// Both boundaries, and it says so: `f(){ break; }` called from a loop and
	// `( break )` inside one each draw the complaint once per pass and leave
	// the loop running. bash 3.2 answers the opposite way on both, which is
	// why neither is the core's.
	s.FunctionCallIsALoopControlBoundary = interp.Yes
	s.SubshellIsALoopControlBoundary = interp.Yes
	// A startup file is a sourced script, so a `return` in one is accepted
	// everywhere — the split is only over what its argument does. bash
	// discards it: measured through a pty, an rc of `return 3` leaves `$?`
	// as 0 at the first prompt and one of `false; return 3` leaves 1, while
	// `(exit 5)` as the last line leaves 5. So the file's status does
	// carry out and the number on the `return` does not.
	s.StartupFileReturnCarriesItsArgument = interp.No
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
	// A lone `-` or `+` is a *name* here and not an option word, and not one
	// a script may declare: measured 2026-09-10, ``declare -`` is ``declare:
	// `-': not a valid identifier`` at 1 where zsh and ksh93 both read it as
	// an option word and list. See Semantics.SignAloneIsAnOptionWord.
	s.SignAloneIsAnOptionWord = interp.No
	// `declare +f` takes the function attribute off rather than naming the
	// functions, which leaves the bare `declare` listing — every variable
	// and then every function. This engine has no bare listing for this
	// shell, so the letter goes on writing bodies (#1754). `declare -F` is
	// the names-only spelling here and is unaffected: it has its own letter.
	s.FunctionNamesUnderPlus = interp.No
	// `-i` takes no output base here, which is what separates this shell
	// from the two that have `integer`: measured 2026-09-06, `typeset -i2
	// c=5` is `-2: invalid option` and `typeset -i 16 b=255` calls the `16`
	// a name that is not a valid identifier. So nothing about a base is
	// refused and both words go on meaning what they meant — and there is no
	// `integer` here at all, which is why IntegerOptions stays empty.
	s.IntegerAttributeTakesABase = interp.No
	s.IntegerBaseComesFromTheValueAssigned = interp.No
	s.IntegerBaseNegativeIsTwosComplement = interp.No
	// Answered for completeness rather than because a script can reach it:
	// there is no base to be the absence of, `-i10` being an invalid option
	// and a bare `10` not a valid identifier.
	s.IntegerBaseTenIsNoBase = interp.No
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
	// The array is why this shell's `coproc` takes a name: the ends arrive in
	// it, and a script writes `echo hi >&"${COPROC[1]}"`.
	s.CoprocEndsInAnArray = interp.Yes
	s.SetListingQuoting = interp.ListingQuoteWhenNeededEscaped

	// `[[ -v 1 ]]` and `[[ -v 0 ]]` ask about a positional parameter here,
	// where ksh93 declines to read a digit as a name at all. The parameters
	// spelled as one punctuation character are the other way round: `[[ -v ?
	// ]]` is unset here and set in zsh. Measured on bash 5.3.15, and it is
	// the operator rather than the lookup — `[[ -n ${?+s} ]]` is set here.
	s.ParameterIsSetSeesPositionals = true
	return s
}

// Diagnostics is how bash 5 reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		// A math complaint raised by a builtin names it: `let '1+'` is
		// `bash: line 1: let: 1+: arithmetic syntax error: …`.
		ArithErrorNamesTheBuiltin: true,
		// The three loops POSIX has, spelled with this shell's own quoting.
		// It says this and carries on, which is the axis beside it.
		LoopControlOutsideALoop: "%[1]s: only meaningful in a `for', `while', or `until' loop",
		TypeKeyword:             "%[1]s is a shell keyword",
		TypeFunction:            "%[1]s is a function",
		TypeNotFound:            "type: %[1]s: not found",
		// The same complaint from `command -V`, blaming `command`.
		CommandVNotFound: "command: %[1]s: not found",
		// The target as it was written, not as it expanded.
		AmbiguousRedirect: "%[1]s: ambiguous redirect",
		// The reading side of the csh spelling, which bash never opens as a
		// file: `cat <&qq` is the redirect being ambiguous, and `<&""` is a
		// descriptor that is not one — named as it was *written*, quotation
		// marks and all. bash 3.2 names the descriptor number instead, which
		// is a version difference rather than a dialect one; 5.3 is what this
		// preset is.
		DuplicationTargetIsNotADescriptor: "%[2]s: ambiguous redirect",
		EmptyDuplicationTarget:            "%[1]s: Bad file descriptor",
		JobStarted:                        "[%[1]d] %[2]d",
		// The one line all three members of the panel's bash write when an
		// interactive shell has no terminal to run the monitor on: 5.3.15 and
		// 3.2.57 as `bash`, and 3.2 run as `sh`, which writes it with its own
		// name in front. 5.3.15 writes a further line above it about the
		// terminal process group, and it is not reproduced — see
		// Diagnostics.NoJobControlAtStartup.
		NoJobControlAtStartup: "no job control in this shell",
		// Silent for a count above `$#` — there is no ShiftTooMany here —
		// and a sentence for one below zero, naming the word as written.
		ShiftNegativeCount: "shift: %[2]s: shift count out of range",
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
		// ^Z prints the listing's own row — JobStoppedNotice is left empty
		// for that — under a newline of its own, because the terminal has
		// just echoed `^Z` where the cursor was.
		JobStoppedNoticeOnANewLine: true,
		// `fg` names the command alone, which is the empty default. `bg`
		// puts the row's head in front of it and the `&` after: measured,
		// `[1]- sleep 40 &`, with one space after the marker rather than the
		// two a listing row has.
		JobResumedInBackground: "[%[1]d]%[2]s %[3]s &",
		// No name and no punctuation before it, and the held `exit` reports
		// 1 — measured through a pseudo-terminal: `echo $?` after the
		// refusal says 1, where the ^D that was refused leaves the status
		// alone.
		StoppedJobsAtExit:       "There are stopped jobs.",
		RunningJobsAtExit:       "There are running jobs.",
		StoppedJobsAtExitStatus: 1,
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
		// The sigil written back, which no other column does: `${@:=w}`
		// with no parameters is `$@: cannot assign in this way`, and
		// `${1:=w}` names `$1`. Identical in 3.2.57 and in the same binary
		// under argv[0] of `sh` (#1541).
		AssignThroughExpansionBadName: "$%[1]s: cannot assign in this way",
		UnboundPositional:             "$%s: unbound variable",
		NumericArgument:               "%[1]s: %[2]s: numeric argument required",
		// A subscript before the first element, named as it was written:
		// `a[x-2]`, not the -1 it evaluated to. Identical in bash 3.2.
		BadArraySubscript:             "%[1]s[%[2]s]: bad array subscript",
		ArithEmptySubscript:           "%[1]s[]: bad array subscript",
		ArrayLiteralThroughASubscript: "%[1]s[%[2]s]: cannot assign list to array member",
		// Through a literal the element is named as it stands between the
		// parentheses, with no array name in front of it. bash 3.2 says the
		// same and does not end the script, which is the one place the two
		// builds differ here.
		BadArrayLiteralSubscript: "[%[2]s]=%[3]s: bad array subscript",
		// Reached from `unset` the same boundary drops the array's name and
		// keeps the bare subscript, with the builtin named in front. bash 3.2
		// words it without the builtin — and refuses every negative subscript
		// besides, which is an absence rather than a wording; the preset
		// follows 5.3.
		UnsetSubscriptBeforeTheFirstElement: "unset: [%[2]s]: bad array subscript",
		// `unset a[@]` where `a` holds a scalar. Identical in bash 3.2.
		UnsetNotAnArray: "unset: %[1]s: not an array variable",
		ArithError:      `%[1]s: %[2]s (error token is "%[3]s")`,
		// The expression is quoted back from its first non-blank
		// character, and a construct names itself in front of it.
		ArithErrorSkipsLeadingSpace: true,
		ArithErrorNamesTheConstruct: true,
		DivisionByZero:              "division by 0",
		ArithNegativeExponent:       "exponent less than 0",

		// bash reserves its generic arithmetic wording for operands that are
		// not literals, so a bad digit gets a reason of its own.
		DigitTooGreatForBase:     "value too great for base",
		ArithErrorNamesThePrefix: true,
		// set -o pads to fifteen and tabs; kill -l numbers five to a row.
		// The width is named rather than written, because `shopt -o -s`
		// writes this listing narrowed and has to pad it the same way.
		OptionListingWidth:  setOptionListingWidth,
		OptionListingTabbed: true,
		KillListing:         interp.KillListingNumbered,
		TraceQuoting:        interp.QuoteShell,
		TraceForHeader:      interp.TraceForSource,
		// The panel's only shell that says how deep the text it is reading
		// came from: `set -x; eval :` traces `+ eval :` and then `++ :`.
		// Measured 2026-09-11 on 5.3.15 and 3.2.57 alike.
		TracePrefixRepeatsAtIndirection: true,
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
		// A substring range puts the parameter in front of the sentence, where
		// the same shell blames a bad *subscript* on the expression alone.
		SubstringRangeError:   "%[1]s: %[2]s",
		ArithOperandExpected:  "arithmetic syntax error: operand expected",
		ArithOperatorExpected: "arithmetic syntax error in expression",
		ArithBadOperator:      "arithmetic syntax error: invalid arithmetic operator",
		ArithFailureStatus:    1,
		SyntaxUnexpected:      "syntax error near unexpected token `%[1]s'",
		// Inside `[[ ]]` the same token gets two lines and neither is the
		// one above: a sentence about the construct at the `[[`'s line, then
		// a shorter `near` at the token's. Measured on bash 5.3.15 —
		// `[[ -n x` newline `-z "" ]]` names line 1 and then line 2.
		CondSyntaxPreamble:   "syntax error in conditional expression: unexpected token `%[1]s'",
		CondSyntaxUnexpected: "syntax error near `%[1]s'",
		// And a `[[` the input ran out inside of gets a line of its own,
		// naming the closer it was waiting for. Measured: this shell writes
		// it for `[[` and for nothing else — `if`, `for`, `case`, `{` and
		// `(` left open each get the one ordinary line.
		CondUnterminatedPreamble: "unexpected EOF while looking for `%[1]s'",
		// A parse failure by every other measure, and 1 rather than this
		// dialect's syntax-error status.
		ForNameStatus: 1,
		ForName:       "`%[1]s': not a valid identifier",
		// The same sentence for a function definition's name, which is
		// measured rather than borrowed: `` `_p_${w}': not a valid
		// identifier `` is what bash writes there, word for word. This
		// dialect never reaches the *expanded*-name route the field also
		// serves — PunctuatedFunctionNameIsRefused is No here, `f-g()` being
		// a perfectly good definition — so there is one wording to keep.
		FunctionNameInvalid: "`%[1]s': not a valid identifier",
		Unterminated:        "syntax error: unexpected end of file from `%[1]s' command on line %[2]d",
		// With nothing open to name — `f()` with no body — the sentence
		// stops after the diagnosis rather than naming an empty construct.
		UnterminatedNoConstruct: "syntax error: unexpected end of file",
		// One sentence for every unmatched delimiter, always naming the
		// closer — only the line it lands on differs by construct.
		UnmatchedQuote:      "unexpected EOF while looking for matching `%[2]s'",
		UnmatchedCmdSubst:   "unexpected EOF while looking for matching `%[2]s'",
		UnmatchedBraceSubst: "unexpected EOF while looking for matching `%[2]s'",
		// The same sentence as the other three openers, with the closer it
		// echoes coming from the construct: `)` for `$((1+2` and `]` for
		// `$[1+2`, which is measured and is why the closer is a verb rather
		// than being written into each wording.
		UnmatchedArithSubst:       "unexpected EOF while looking for matching `%[2]s'",
		UnmatchedReportedAtOpener: true,
		CmdSubstUnmatchedAtEnd:    true,
		SyntaxErrorStatus:         2,
		// Measured: `.` of a file it cannot open reports 1 and carries on,
		// where a missing operand is 2 — two numbers for what reads like one
		// failure, which is why they are two fields.
		DotCannotOpen:       "%[1]s: %[2]s",
		DotCannotOpenStatus: 1,
		// The one `.` failure bash gives a sentence of its own, and the one
		// it names the builtin in — `source ./` says `source:` where `. ./`
		// says `.:`, which is why the format takes the invoked name. Its
		// reason is spelled in lower case, unlike the strerror text
		// DotCannotOpen carries, so it is written out rather than passed in.
		DotIsADirectory: "%[3]s: %[1]s: is a directory",
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
		GetoptsNamesNoLine: true,
		CdCannotChange:     "cd: %[1]s: %[2]s",
		CdHomeNotSet:       "cd: HOME not set",
		// An empty operand, which is a fifth branch and not a path that
		// would not open: the sentence names no path at all.
		CdEmptyOperand: "cd: null directory",
		// A second operand is one too many here, and the refusal is a usage
		// error rather than a directory that would not open — 2 where
		// CdStatus is 1. No usage block goes with it.
		CdTooManyOperands:           "cd: too many arguments",
		CdTooManyOperandsStatus:     2,
		CdOldpwdNotSet:              "cd: OLDPWD not set",
		PrintfBadNumber:             "printf: %[1]s: invalid number",
		PrintfBadVerb:               "printf: `%[1]s': invalid format character",
		PrintfMissingVerb:           "printf: `%[1]s': missing format character",
		PrintfMissingHexDigit:       `printf: missing hex digit for \x`,
		PrintfMissingUnicodeDigit:   `printf: missing unicode digit for \%s`,
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
			// notices, -k assignment-anywhere, -p privileged, -B brace
			// expansion, -H history expansion, -P physical paths. Measured
			// 2026-09-05 by asking bash 5.3 for every letter of the alphabet
			// in both cases and both signs; the ones missing from here it
			// refuses itself, and those get SetInvalidOptionLetter.
			//
			// `-t` left this list when the option behind it was built. The
			// two tables are one table: the letter and `set -o onecmd` are
			// the same request, so a letter listed here while the name is
			// wired would refuse what the name grants — see
			// Semantics.SetHasTheTLetter.
			"set": "bkprHP",
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
			// What is left of read's letters: readline editing, which is a
			// line editor this runner does not hold. -i is off this list
			// because there is nothing left for it to do — it seeds the
			// editor -e opens, and with -e refused here there is never one
			// to seed, which is also bash's own answer wherever the input
			// is not a terminal. The -p prompt is implemented: parsed
			// always, printed only to a terminal, which is the measured
			// whole of it.
			"read": "Ee",
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
		// `local` belongs here with the other two: `typeset -r x=1; f() {
		// local x=2; }` says `local: x: readonly variable` in this shell,
		// naming the word the script wrote. It was missing because `local`
		// reached the refusal only through the assignment, where the
		// builtin's name had already been put aside (#1159).
		ReadonlyRefusalNamesBuiltin: map[string]bool{
			"declare": true, "typeset": true, "local": true,
		},
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
			// The declaration builtins, which say the same as the rest. Named
			// rather than left to the default so that the table is the whole
			// answer for every builtin that asks it.
			"typeset": "%[1]s: `%[2]s': not a valid identifier",
			"declare": "%[1]s: `%[2]s': not a valid identifier",
			// `read 1bad` says the same, and quotes the word back whole:
			// `read "v?p"` is `` `v?p' `` here, because bash has no prompt
			// operand to split it at.
			"read": "%[1]s: `%[2]s': not a valid identifier",
		},
		BuiltinBadNameKeepsValue: true,
		BuiltinUsageUnprefixed:   true,
		BuiltinHelpStatus:        2,
		BuiltinHelp:              builtinHelp(),
		BuiltinUsage:             builtinUsage(),
		// bash words this as a statement about the operator that was
		// waiting rather than about the token it met: `[[ $k == (a|b) ]]`
		// is "unexpected argument `(' to conditional binary operator", where
		// ksh93 and zsh name the token and stop.
		CondOperand:            "unexpected argument `%[1]s' to conditional %[2]s operator",
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
	// The long names of the options that are on, as a readonly produced
	// variable bound to the option state in both directions. The core keeps
	// the state and this names it; the other three leave the name an ordinary
	// string and read nothing out of it at startup, which is measured.
	r.SetShellOptions("SHELLOPTS")
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
	// The login name for that same uid, for the `\u` prompt escape, and the
	// machine's name for `\h` and `\H`. Asked here for the reason the uid
	// above is, and asked through interp.LoginName rather than through a
	// closure of this package's own, because zsh's `%n` is the same question
	// under another spelling — the escape's *letter* is this dialect's and
	// resolving a login name is not.
	//
	// This is #1446. `%n` had an asker and `\u` had none, so the drawer fell
	// back to a lookup of its own that read `$USER` and `$LOGNAME`; with
	// neither set — `env -i`, `sudo -i`, a container, a cron job, a login
	// shell a daemon started — `\u` drew nothing, and the `\u@\h:\w\$ `
	// that most distributions ship rendered `@host:~$`. Every other escape in
	// that prompt was byte-identical to bash 5.3.15, which is what made it
	// look like a working prompt.
	//
	// Deferred rather than resolved here, which #1423 measured and this must
	// not undo: `user.Current` is 0.83-1.10 ms on darwin — Directory Services
	// — and Apply runs on every invocation, so asking eagerly would spend a
	// millisecond of every `bash -c ':'` on an escape that route cannot draw.
	// SetPromptUserFunc asks at the first draw and keeps the answer, which is
	// the right trade in both directions: a prompt is redrawn on every
	// keystroke that redraws the line, and a login name does not change while
	// a shell runs, so a resolver called per draw would pay that millisecond
	// per *character typed*.
	r.SetPromptUserFunc(interp.LoginName)
	r.SetPromptHostFunc(interp.MachineName)
	// And the table those two resolvers answer *for*, which is the half that
	// was missing: a runner told who the user is and which machine this is,
	// and never told that `\u` and `\h` are the letters that ask, draws the
	// letters as text. #1455.
	//
	// Installed here rather than only in the front end so that the escape
	// language travels with the dialect. `driver.Shell.PromptStyle` hands the
	// same value to the same runner on the way to the prompt drawer, and the
	// two agree because they are the same function's result — but an embedder
	// who applies this dialect to a Runner of their own never builds a Shell,
	// and before this they got a bash with no prompt language at all. zsh's
	// Apply had installed its table since #1090 and the other three had not,
	// which is the asymmetry #1455 reports.
	r.SetPromptStyle(PromptStyle())
	// $LINES and $COLUMNS follow the window, which this shell does by
	// default: `shopt checkwinsize` in bash 5.3.15 is `on`, and a session
	// there reports `COLUMNS=80 LINES=24` at its first prompt with nothing in
	// a startup file asking for it. bash 3.2.57 answers `off` to the same
	// question and is the one column of the panel this default is not right
	// for; one bash dialect answers for both, and it answers with the
	// version people are running.
	//
	// The default is here rather than in the option table because it is a
	// fact about *this shell*, in the same way the option names above are:
	// the capability itself belongs to the core, where zsh reaches it under
	// no name at all. See interp.Runner.TracksWindowSize.
	r.SetTracksWindowSize(true)
	// Where the script is, which is a stack rather than a value — see
	// callstack.go for why it cannot be stored.
	registerCallStack(r)
	// The same stack, one step up: see caller.go.
	registerCaller(r)
	// Rebinding a key, in readline's vocabulary. The editor performs the
	// actions and this names them; see bind.go, and repl/widgets.go for why
	// the two halves are apart.
	registerBind(r)
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
	// `**` with nothing behind it crosses levels here too, so `d/**` reaches
	// every one of them. Set unconditionally rather than through `globstar`,
	// because the walk asks it only where `globstar` has already said `**`
	// crosses at all — and `globstar` is the only name this shell has for
	// either half. Measured 2026-09-07: `shopt -s globstar; echo **` in a
	// directory holding `ax`, `bx` and `cx/dx/ax` lists every level here and
	// answers `ax bx cx` in zsh (#1339).
	r.SetMatchOption(interp.StarStarAloneCrossesDirectories, true)
	// An `&` in a `${v/pat/rep}` replacement is the text the pattern
	// matched, which is this shell alone in the panel — bash 3.2, ksh93 and
	// zsh all answer `a[&]c` for `v=abc; ${v/b/[&]}` where this one answers
	// `a[b]c`. It is on with nothing said and `shopt -u patsub_replacement`
	// turns it off, so the default belongs here and the name belongs in
	// shopt.go (#1862).
	r.SetMatchOption(interp.ReplacementAmpersandIsTheMatch, true)
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
