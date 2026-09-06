// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package zsh answers the substrate's questions the way zsh does.
package zsh

import (
	"os"
	"os/user"
	"strconv"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what zsh parses.
func Dialect() syntax.Dialect {
	d := syntax.Core()
	// `$[expr]`, zsh's other arithmetic spelling and the older one.
	// Measured 2026-09-06: `echo $[1+1]` is 2, and the same text is
	// the literal `$[1+1]` in ksh93 and dash. Reading it as a glob is
	// what made the failure `no matches found`, which points a
	// person at globbing rather than at arithmetic (#900).
	d.DollarBracketArith = true
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
	// Short loops: a loop header that has ended may be followed straight by
	// its body, and the body may be left out. Measured 2026-09-05, the other
	// four panel shells refuse every shape — `for i (a b) { echo $i }`,
	// `for i (a b) echo $i`, `while (( i<2 )) echo $i`, `for i in a b; echo
	// $i` and `while false` alone all run here and are syntax errors there.
	//
	// The one that reads as this construct and is not is `while cond; { … }`:
	// the `;` keeps the condition list going, so the brace group is tested
	// rather than run and the body is empty. `i=0; while (( i<2 )); { echo
	// $i; i=$((i+1)) }` counts up without stopping here, which is what says
	// so.
	d.ShortForm = true
	// `for key value ( a 1 b 2 ) { … }`: the loop takes one word per name on
	// every pass, and a short final pass leaves the names it did not reach
	// empty. Independent of the two flags above, measured over all four
	// combinations.
	d.ForMultipleNames = true
	d.Repeat = true
	d.Foreach = true
	d.AnonymousFunction = true
	// The same reach: a body may have nothing in it — `{ }`, `( )`, `while
	// cond; do done`, and a condition too. Every shape, and this shell alone.
	d.EmptyCompoundBody = true
	// Floating point, which POSIX has not and these two do.
	d.ArithFloat = true
	// A bare `(a|b)` inside a pattern word, which makes `@(abc|xyz)` a
	// literal `@` followed by a group here rather than an extended pattern.
	d.PatternAlternation = true
	// The same alternation reaching the `case` arm's own pattern list, where
	// one of the alternatives may be written as nothing: `(|https|git|ftp)`
	// matches one of those schemes or none at all. `~/.zi/bin/lib/zsh/install.zsh`
	// is written that way and the other four shells refuse the line.
	d.CasePatternMayBeEmpty = true
	// `<->` is a number and `<1-9>` a bounded one, where every other panel
	// shell reads the `<` as a redirection. Measured 2026-09-05 on zsh
	// 5.9.2: `[[ 1 = <-> ]]` is 0 here and a syntax error in bash 5.3, bash
	// 3.2, bash-as-sh and ksh93, and dash — which has no `[[ ]]` — tries to
	// open a file called `-`.
	d.NumericRangePattern = true
	// A `(` where an argument may stand belongs to the word: `echo MY ( x )`
	// is two words there and a syntax error in the other four. Measured
	// 2026-09-06 on zsh 5.9.2 — `unknown file attribute:` names the space
	// inside the group, so the group was read as a qualifier list — and
	// `setopt no_glob; echo MY ( x )` prints `MY ( x )`, which is what says
	// the words are the same either way.
	d.GlobQualifiers = true
	// `cmd &!` and `cmd &|` background a job and let go of it. zsh's alone:
	// bash 5.3 and ksh93 parse `&!` as `&` and a negation and keep the job,
	// and `&|` is a syntax error in every bash and in dash.
	d.BackgroundAndDisown = true
	// The parenthesized flag group an expansion may open with — `${(U)x}`,
	// `${(%):-%x}` — which is this dialect's alone: the other three call
	// the whole expansion a bad substitution.
	d.ParamExpansionFlags = true
	// A `~` between the `${` and the parameter, which makes the result of
	// the substitution eligible for tilde expansion and filename generation.
	// This shell alone: measured 2026-09-06, bash 5.3, bash 3.2 and dash all
	// answer `bad substitution` when the expansion is reached and ksh93
	// refuses it while reading. `~/.zi/bin/zi.zsh` uses it eighteen times in
	// nineteen lines, and a refusal there leaves eighteen variables empty.
	d.ParamTildeFlag = true
	// An `=` in the same slot, which splits the result of the substitution
	// into words on IFS whatever SH_WORD_SPLIT says. This shell alone:
	// measured 2026-09-06, bash 5.3, bash 3.2, bash-as-sh and dash all
	// answer `bad substitution` when the expansion is reached and ksh93
	// refuses it while reading with the `=` named. `is-at-least`, the
	// version predicate in this shell's own function library, is `${=1}`
	// and `${=2:-$ZSH_VERSION}` on lines 27 and 28 — refused, both version
	// arrays are empty and the comparison answers true for every version.
	d.ParamSplitFlag = true
	// A subscript's own parenthesized flag group: `${a[(re)value]}`, the
	// first element equal to the operand. This shell alone — measured
	// 2026-09-06, bash 5.3, bash 3.2, bash-as-sh and ksh93 all read the same
	// text as arithmetic and fail there, and dash has no arrays to subscript
	// — and it is additive rather than a conflict because this shell reads a
	// group it does not recognize as arithmetic too. `~/.zi/bin/zi.zsh` has
	// sixty-four of them, six of which stand between the plugin manager and
	// its first definition.
	d.ArraySubscriptFlags = true
	d.ParamElementSelection = true
	// The length may carry an operator here, and it measures what the
	// operator *leaves*: `v=abc; echo ${#v#a}` is 2. bash, bash 3.2, bash as
	// `sh`, dash and ksh93 all call the same text a bad substitution, so
	// this is the grammar that has the construct rather than a different
	// arithmetic over one they share. Uniform across every operator this
	// shell has — the trims, the substring, the replacement, the four
	// conditionals and the element exclusion — measured 2026-09-06.
	d.ParamLengthTakesAnOperator = true
	// An expansion where a parameter name would be — `${${v}#a}`, which is
	// how this shell applies one expansion to the result of another and is
	// idiomatic here rather than a corner. Measured 2026-09-05 on zsh 5.9.2
	// against the rest of the panel: bash 3.2 and 5.3 answer `bad
	// substitution` and ksh93 a syntax error, so this shell alone.
	d.NestedParamExpansion = true
	// A parameter written without braces carries a subscript here, and `$#a`
	// is a count rather than `$#` with a letter after it. Measured 2026-09-05
	// on zsh 5.9.2: `a=(x y z); echo $a[1]` prints `x` and `echo $#a` prints
	// `3`, where bash 3.2, bash 5.3 and dash print `x[1]` and `0a`. This
	// shell alone, which is why it is set here and nowhere else.
	d.BareSubscript = true
	// And a parameter that is not a name carries one too: `${@[1]}` is the
	// first positional parameter here and `${1[2]}` the second character of
	// the first. Measured 2026-09-05 on zsh 5.9.2 against the rest of the
	// panel, where every one of them refuses the expansion — bash calls it a
	// bad substitution when it is reached, ksh93 refuses `[' while reading,
	// dash says `Bad substitution`.
	d.SpecialParamSubscript = true
	// And `#` in arithmetic is the character-code operator: `$((#b))` on
	// `zebra` is 122 and `$((##a))` is 97. Measured 2026-09-05 against the
	// rest of the panel, every one of which calls the same text an arithmetic
	// syntax error. Worth naming carefully, because it looks like a length
	// and is not one — `a=(1 2); $((#a))` is 49, the code of the `1`, where
	// the count is `$(( $#a ))`.
	d.ArithCharacterCode = true
	// `a |& b`, read exactly as bash 5.3 reads it: the left side's standard
	// error joins its standard output on the pipe. Not the ksh93 reading of
	// the same two characters, which is a coprocess and a different slot in
	// the grammar — measured, because ksh93 refuses `a | ; b` and accepts
	// `a |& ; b`, so its `|&` ends a command where this one joins two.
	d.PipeBothStreams = true
	// A coprocess, spelled with the same word as bash's and with no name
	// before it: `coproc MY { cat; }` is `parse error near `}'` here, because
	// `MY {` is a simple command and the `}` closes nothing. The near ends
	// are reached with `print -p` and `read -p` rather than through an array,
	// which is CoprocEndsInAnArray rather than a grammar flag.
	d.Coproc = true
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
	// zsh leaves it off with no terminal, silently. With one it puts `m` in
	// `$-` and announces its jobs while its own `set -o` still lists
	// `monitor off` — zsh disagreeing with itself rather than an answer to
	// this.
	s.InteractiveMonitorNeedsATerminal = interp.Yes
	// The same, and zsh is worth stating rather than assuming: it forks for
	// a `( )` that has a command after it, which is where this was measured.
	s.SubshellRunsOnAfterSignalingTheShell = interp.Yes
	// And it announces both ends of a job on that route: measured on
	// `-i script.sh` through a pseudo-terminal, `[1] <pid>` as the job
	// starts and `[1]  + done       sleep 0.3` as it ends.
	s.InteractiveScriptAnnouncesJobs = interp.Yes
	// `$!` before any background command is `0` here and empty in the other
	// five columns — a number nothing ever had. Measured,
	// `sh -c 'echo "[$!]"'` writes `[0]`.
	s.LastBackgroundPidIsZeroBeforeAnyJob = interp.Yes
	// And that zero is *set*, so `set -u` carries on: measured,
	// `set -u; echo "[$!]"; echo "st=$?"` writes `[0]` then `st=0`. It is the
	// same side of that split as ksh93 and for a different reason — ksh93 has
	// nothing there and does not mind, zsh has a value.
	//
	// Stated even though it is the unanswered default, because zsh refuses an
	// unset `$1` and does not refuse `$!`, and a reader checking that pair
	// needs to see the second answer written down.
	s.LastBackgroundPidIsUnsetBeforeAnyJob = interp.No
	// Measured: `echo $-` reports `569X` under -c, a script file and
	// standard input alike — letters from zsh's own single-letter option
	// namespace, which shares almost nothing with the other shells'. The
	// `s` of the standard-input route is added on top of it and comes from
	// Runner.Route; under `-c` zsh shows neither route letter, where bash
	// and ksh93 show `c`.
	s.DefaultOptionLetters = "569X"
	// And `569XZi` when interactive, so the interactive set is the same four
	// plus the line-editor letter. Measured 2026-09-05 under `-i script.sh`
	// and at a pseudo-terminal alike; `i` comes from the runner.
	s.InteractiveOptionLetters = "569XZ"
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
	// And zsh reads it for a login shell as well as a plain one, where bash
	// reads only its profile.
	s.InteractiveStartupFileWhenLogin = interp.Yes
	// One escape hatch, spelled two ways, and it means *every* file: `zsh -f
	// -l -i` reads no `.zshenv`, no `.zprofile`, no `.zshrc` and no
	// `.zlogin`. It is not the POSIX `-f` — measured, `zsh -f -c 'echo
	// /etc/pas*'` still expands the pattern, so the letter is spent on this
	// here and on globbing everywhere else.
	s.StartupFileOptions = interp.StartupFileOptions{
		// Both spellings of login-ness, which zsh has like the other three.
		Login:       "-l --login",
		SuppressAll: "-f --no-rcs",
	}
	s.CommandStringShowsCInDollarDash = interp.No
	s.LoginShowsLInDollarDash = interp.Yes
	s.CommandStringShowsSInDollarDash = interp.No
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
	s.ArrayNameWithoutSubscriptIsTheList = interp.Yes
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
	s.ListingControlEscape = interp.ControlEscapeCaret
	s.ExportListing = interp.DeclareListingCommandWord
	// readonly -p speaks typeset here, not readonly.
	s.ReadonlyListing = interp.DeclareListingExportSpelled
	// The bare form drops the command word in both builtins, which is where
	// this shell's `readonly` parts company with its own `readonly -p` by more
	// than a word: `typeset -r R=2` with the letter, `R=2` without it.
	s.BareDeclarationListing = interp.DeclareListingPlainAssignment
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
	// A dash word is an option unless it is all digits, which is what
	// separates this from ksh93: `shift -x` is a bad option and `shift -1` is
	// a count it refuses for being below zero.
	s.ShiftOptionWords = interp.ShiftOptionWordsNonNumeric
	s.ShiftDoubleDashEndsOptions = interp.Yes
	s.ShiftNegativeIsOutOfRange = interp.Yes
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
	// And an attribute added to a name that already holds a value re-reads
	// it at once, as ksh93 does: `FOO=bar; typeset -i FOO` stores 0 and
	// `d=MiXeD; typeset -u d` stores MIXED. A separate question from the
	// one above, which this shell happens to answer the same way — bash
	// answers both no and ksh93 answers them differently from each other.
	s.AttributeRereadsTheValueItFinds = interp.Yes
	s.TypesetLocalNeedsKeywordFunction = interp.No
	// A local does not inherit the export attribute of the name it shadows.
	// Measured with a real child: `export FOO=bar; f() { local FOO=baz; env;
	// }` shows the child no FOO at all here, where bash and dash show it the
	// local's value.
	s.LocalInheritsTheExportAttribute = interp.No
	s.SplitParamExpansion = interp.No
	// An unquoted list is its elements, never their join: `IFS=:; a=(x y);
	// printf "[%s]" ${a[*]}` is `[x][y]` here, and with `shwordsplit` on,
	// `a=("x y" z)` is `[x][y][z]` — neither of which a join can produce.
	s.UnquotedListJoinsOnIFS = interp.No
	// And the join this shell *does* perform: an unquoted `@` list reaching
	// a context that keeps no fields is joined on the first character of
	// IFS, so `IFS=-; a=(x y z); v=${a[@]}` is `x-y-z` where bash and ksh93
	// give `x y z`. With IFS set and empty it is `xy`, which is what says
	// the separator is read from IFS rather than defaulted to a space.
	s.UnsplitAtListJoinsOnIFS = interp.Yes
	s.GlobExpansionResults = interp.No
	s.GlobNoMatchIsError = interp.Yes
	s.AssignmentPrefixPersistsOnSpecialBuiltin = interp.No
	// hash counts only what PATH holds: a builtin or a function is "no
	// such command" to it.
	s.HashSearchesPathAlone = interp.Yes
	s.TildePlusMinusExpands = interp.Yes
	s.UnderscoreTracksTheLastArgument = interp.Yes
	// And starts it empty regardless, alone in the panel: an exported `_`
	// is discarded rather than carried in, so the parameter says nothing
	// about the invocation until the first command has run.
	s.UnderscoreInheritsFromTheEnvironment = interp.No
	// Bases stop at 36 here, and the refusal says so.
	s.ArithBaseAbove36 = interp.No
	// ${#a} of an array counts elements, and a function's $LINENO counts
	// from the function.
	s.ArrayLengthWithoutSubscriptIsCount = interp.Yes
	s.FcEmptyHistoryIsAnError = interp.Yes
	s.JobControlAbsenceIsReportedFirst = interp.Yes
	// A stopped job holds the exit back: the shell says so and stays,
	// and the next attempt leaves. Measured through a pseudo-terminal for
	// `exit` and for ^D alike.
	s.StoppedJobsHoldTheExit = interp.Yes
	// CDPATH moves in silence here.
	s.CdpathAnnouncesTheDirectory = interp.No
	s.LinenoCountsFromTheFunction = interp.Yes
	// A substring range is this shell's history-modifier syntax as well, and
	// a segment beginning with an unquoted letter is read as a modifier
	// rather than as an arithmetic offset — so `${x:i:2}` is refused where
	// the other three take a substring. Measured against zsh 5.9.2.
	s.SubstringRangeReadsModifiers = interp.Yes
	s.EchoInterpretsEscapes = interp.Yes
	// echo reads -n, -e and -E, and -e wins over -E whatever the order.
	s.EchoOptions = "neE"
	s.EchoLastEscapeFlagWins = interp.No
	s.EchoExpandsHexEscapes = interp.Yes
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
	s.ReadOptions = "rsnpAd:t:u:"
	// `unset -m` reads its operands as patterns, which is this shell's
	// alone; `-n` is not here, and that is measured rather than an
	// omission — `unset -n x` is `bad option: -n` in zsh 5.9.2 where bash
	// 5.3 and ksh93 take it.
	s.UnsetOptions = "vfm"
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
	// A process substitution may *not* stand as a condition's operand. The
	// word is read and then refused, in a sentence of this shell's own and
	// at status 2 — and the command is not started, which is the half a
	// refusal that came after the expansion would get wrong. Measured
	// 2026-09-05: `[[ x == <(x) ]]` is `process substitution <(x) cannot be
	// used here` here and runs the command in bash.
	s.ProcessSubstitutionInCondition = interp.No
	s.ShiftPastEndFatal = interp.No
	s.ArrayBaseIsZero = interp.No
	// `${a[1,3]}` is elements one through three here, where the shells that
	// read the same characters as arithmetic take the comma operator's value
	// and name element three alone. Measured on `a=(w x y z)`: `w x y` here,
	// `z` in bash 5.3.15, bash 3.2.57, bash as `sh` and ksh93.
	s.SubscriptCommaIsARange = interp.Yes
	// And a subscript on a plain string reaches into its characters: with
	// `s=hello`, `${s[2]}` is `e` here and nothing in the four that read a
	// scalar as an array of one.
	s.ScalarSubscriptIsACharacter = interp.Yes
	s.ArrayLiteralSubscriptIsAKey = interp.No
	// The FUNCTION_ARGZERO option, on by default and the reason this shell
	// alone moves `$0`: it names the function being run, or the file being
	// sourced, and goes back to the script's name when that call returns.
	s.DollarZeroNamesTheInnermostCall = interp.Yes
	s.BuiltinSyntaxErrorFatal = interp.No
	// An error inside a file `.` read ends that file and nothing above it:
	// measured, `.` reports 126 and the sourcing file runs the command after
	// it. The status is Diagnostics.SourcedFatalStatus.
	s.FatalErrorEndsBorrowedTextOnly = interp.Yes
	// Except `${x?word}`, which this shell's own manual documents as printing
	// the word and *exiting the shell* — and measured to do exactly that from
	// inside a sourced file, where an unset parameter under `set -u` two
	// lines away is caught. So it is a request to stop rather than an error.
	s.ParamErrorIsAnExitRequest = interp.Yes
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
	// A substituted element keeps the status its death produced, 128 plus
	// the signal, the same as anywhere else.
	s.PipefailSubstitutesTheBareSignal = interp.No
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
	// This shell alone: an untrapped SIGHUP ends the shell with 1 and runs
	// the EXIT trap, rather than killing it with 128 plus the number. The
	// EXIT trap is why it is not simply a different number — the line above
	// says dying does not run it, and here one runs.
	s.HangupIsAnOrderlyExit = interp.Yes
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
	s.PrintfUnfinishedConversionIsAPercent = interp.No
	// The same two digits bash reads, and an empty digit run is a zero
	// rather than an escape left standing: `printf 'a\xZ'` is a NUL here.
	s.PrintfHexEscape = interp.PrintfHexEscapeByteOrNul
	// A `%b` argument reads the same escape the same way, NUL and all.
	s.PrintfBHexEscape = interp.PrintfHexEscapeByteOrNul
	// `\e` is the escape character and `\E` is two characters — the opposite
	// of ksh93, which is why one axis could not answer for both letters.
	s.PrintfBEscEscape = interp.Yes
	s.PrintfBCapitalEscEscape = interp.No
	// The octal wants its `\0`, as in ksh93.
	s.PrintfBOctalWithoutZero = interp.No
	s.PrintfBStopIsPadded = interp.Yes
	// One of `h`, `l` and `L`, which is C89's set: `%ld` is a decimal and
	// `%lld`, `%zX` and `%jd` are invalid directives.
	s.PrintfLengthModifiers = interp.PrintfLengthModifiersC89
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
	// Nor does a redirection that cannot be made end anything: the message
	// is printed and the script runs on. The starting value only — `emulate
	// sh` and `emulate ksh` move it to the POSIX answer, and `emulate zsh`
	// puts it back.
	// Read as a number like any other; nothing about the width is
	// refused.
	s.MultiDigitDuplicationTargetIsAnError = interp.No
	s.RedirectErrorOnSpecialBuiltinFatal = interp.No
	// The same csh spelling, and one step further: a word that expanded to
	// nothing is a name too, so `>&""` opens the empty path and fails on it
	// rather than complaining about a descriptor.
	s.GreatAmpTarget = interp.GreatAmpTargetNamesAnyFile
	// And the reading side ends the shell, on a builtin alone.
	s.DuplicationTargetErrorOnABuiltinIsFatal = interp.Yes
	// zsh takes it and sets a global instead of refusing.
	s.LocalOutsideAFunctionIsAnError = interp.No
	// Fatal to all three, which is the one place zsh is stricter than bash
	// about a builtin's failure. It refuses fewer operands, though, and the
	// two sets it adds have only `0` in common: `export ?` is quiet and
	// `unset ?` is not, `unset 12` is quiet and `export 12` is not.
	s.BadNameToDeclarationFatal = interp.Yes
	s.BadNameToUnsetFatal = interp.Yes
	// Fatal here too, and unlike bash's this does not move: `emulate sh`,
	// `emulate ksh` and `emulate zsh` all stop. It is the axis that keeps
	// this question apart from RedirectErrorOnSpecialBuiltinFatal, which zsh
	// answers the other way.
	s.UnsetReadonlyFatal = interp.Yes
	s.DeclarationNameOperands = interp.NamesAndSpecialParameters
	s.UnsetNameOperands = interp.NamesAndPositionals
	s.DeclarationTakesASubscript = interp.No
	s.UnsetTakesASubscript = interp.Yes
	// `unset a[@]` replaces the elements with a single empty one, which is
	// this shell's reading of `unset` on a span rather than a special rule
	// for `[@]`: `unset a[2]` leaves an empty element in place too. A scalar
	// is one such span and comes back empty; an array with nothing in it has
	// no span and gains no element.
	s.UnsetArraySpan = interp.UnsetArraySpanLeavesOneEmptyElement
	// The complaint is the builtin's rather than the script's: `unset` reports
	// 1 and the next command still runs.
	s.BadSubscriptToUnsetFatal = interp.No
	// A negative subscript past the first element places one in front of it
	// rather than being refused: `a=(p q); a[-3]=x` is three elements with
	// `x` at the head, and `-4` and `-5` land in the same place. What this
	// shell refuses is `a[0]`, which is below its first element rather than
	// counting back from the last.
	s.NegativeSubscriptPastTheStartInserts = interp.Yes
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
	// No `-n` either, and unlike `-f` it is a letter this shell has simply
	// never heard of — so it earns the ordinary `bad option: -n` rather
	// than the ExportFunctionOptionRefused wording, and `export` fails at 1
	// with the script carrying on.
	s.ExportTakesTheAttributeOff = interp.No
	s.AnnouncesBackgroundJob = interp.Yes
	s.ReportsACommandKilledBySignal = interp.No
	s.ReportsAnyKilledPipelineElement = interp.No
	s.ChildInterruptEndsTheScript = interp.No
	s.CdRefusesUnknownOption = interp.No
	s.CdLastPathOptionWins = interp.No
	s.BadSetOptionNameFatal = interp.Yes
	s.UnknownConditionOptionIsAStatus = interp.Yes
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
	// `-T SCALAR array [sep]` ties a scalar to an array, each reflecting the
	// other — which is how this shell's own `PATH` and `path` are the same
	// value. This shell's letter alone with that meaning: bash refuses `-T`
	// outright, and ksh93's `-T` declares a *type*, which is a different
	// builtin's worth of thing and stays refused by name there.
	s.DeclareOptions = "aAfFgHilpruUTx"
	// `-F` is a float's precision here rather than bash's function listing,
	// and this engine has no float attribute to record it in. Taken in
	// silence at 0, which is what real zsh answers to every shape of it —
	// measured, not one byte on either stream — rather than refused, which
	// is what a caller enumerating functions with `declare -F` used to get
	// from a shell that has nothing to say to it (#1037).
	s.DeclareOptionsWithoutEffect = "F"
	s.LocalOptions = "aAHilpruUTx"
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
	s.SetListingQuoting = interp.ListingQuoteWhenNeededEscaped

	// A descriptor number the process cannot hold is not checked here: with
	// `ulimit -n 6`, `exec 8>f` reports success and prints nothing, where
	// bash and ksh93 hand the kernel's refusal back. Rarely reachable, since
	// a number this shell reads is one digit and the shell picks its own for
	// `{name}>f`.
	s.FdNumberBoundedByOpenFileLimit = interp.No

	return s
}

// Diagnostics is how zsh reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		// zsh names itself, not the path it was invoked by. `/bin/zsh` and a
		// symlink called `myzsh` both say `zsh:`, and so does the shell run
		// as `exec -a weirdname /bin/zsh` — measured all three ways, because
		// the first alone looks like a base name rather than a fixed one.
		// The other three shells print argv[0] whole.
		SelfName:    "zsh",
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
		UnsetFunctionNotFound: "no such hash table element: %[1]s",
		// The array alone, and a sentence about the assignment rather than
		// about the subscript.
		BadArraySubscript: "%[1]s: assignment to invalid subscript range",
		// The identical sentence from `unset`, and the builtin is *not* in
		// the location for it — the store is speaking rather than `unset` —
		// which is why the two routes need two fields even where one shell
		// words them alike.
		UnsetSubscriptBeforeTheFirstElement: "%[1]s: assignment to invalid subscript range",
		// Through a literal it is the subscript alone, and the literal is
		// named for what it is rather than by the variable it fills.
		BadArrayLiteralSubscript: "bad subscript for direct array assignment: %[2]s",
		DeclareNoSuchVariable:    "no such variable: %[1]s",
		LocationNamesTheFunction: true,
		SetInvalidOptionName:     "no such option: %[1]s",
		// The bare letter with a dash written in front of it: this shell
		// echoes `-q` for `set +q` as dash does, rather than the sign it was
		// asked with. `set` is named in the location, not in the sentence.
		SetInvalidOptionLetter: "bad option: -%[2]s",
		SetInvalidOptionStatus: 1,
		// A denied `set -m` echoes the spelling it was asked with — `-m` or
		// `monitor` — and fails at 1, fatally like every `set` failure here.
		MonitorDenied:       "can't change option: %[1]s",
		MonitorDeniedStatus: 1,
		// `[[ -o nosuchoption ]]`. The same words `setopt` uses for the same
		// mistake, and measured to be: one message, said at the condition's
		// own location rather than a builtin's. The status is 3, which is
		// neither of the two a condition otherwise gives.
		UnknownConditionOption:       "no such option: %[1]s",
		UnknownConditionOptionStatus: 3,
		// zsh knows `-f` — it means functions to its own typeset — so what
		// it refuses is the combination, and it says so without naming the
		// letter it names in every other refusal.
		ExportFunctionOptionRefused: "invalid option(s)",
		JobRunning:                  "running",
		// Only ever seen in a completion notice: zsh's listing never
		// mentions a job that has ended.
		JobDone:    "done",
		JobStopped: "suspended",
		// ^Z is a sentence rather than a listing row here, and the shell
		// names itself in it: `zsh: suspended  sleep 40`, two spaces, no job
		// number. Under a newline of its own, as bash's is.
		JobStoppedNotice:           "%[3]s: suspended  %[4]s",
		JobStoppedNoticeOnANewLine: true,
		// `fg` and `bg` both print a listing row with a state no listing
		// ever shows — `[1]  + continued  sleep 3` — so it is spelled here
		// rather than beside JobRunning and JobStopped. The shape is
		// JobLine's, with `continued` in the 11-wide state column.
		JobResumedInForeground: "[%[1]d]  %[2]s continued  %[3]s",
		JobResumedInBackground: "[%[1]d]  %[2]s continued  %[3]s",
		// The shell names itself here too, and the held `exit` reports
		// nothing — measured, `echo $?` after the refusal says 0, where
		// bash's says 1.
		StoppedJobsAtExit: "%[1]s: you have suspended jobs.",
		// The reason first and the name after it, which is zsh's shape and
		// nobody else's. Lowercased, which LowercaseReason already says.
		// Same either way — zsh does not distinguish opening from creating.
		CannotOpen: "%[2]s: %[1]s",
		// zsh names nobody: `cat <&qq` and `cat <&""` are both `file number
		// expected`, so the wording uses neither verb and the empty case has
		// nothing of its own to say. The writing side never reaches this —
		// GreatAmpTarget sends it to the file.
		DuplicationTargetIsNotADescriptor: "file number expected",
		CannotCreate:                      "%[2]s: %[1]s",
		NotABuiltin:                       "no such builtin: %[1]s",
		CommandStringParsedWhole:          true,
		// Reading a program from standard input, a line that does not parse
		// is reported and the next line is read anyway. This shell alone,
		// and this route alone: the same program in a file stops it.
		StdinProgramSurvivesAParseFailure: true,
		ArithInfinity:                     "Inf",
		ArithNotANumber:                   "NaN",
		ArithFloatDigits:                  17,
		ArithFloatKeepsPoint:              true,
		SelectPrompt:                      "?# ",
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
			// zsh gives a single letter to far more of its options than the
			// rest of the panel does: measured 2026-09-05, it refuses only
			// b, c, j, q and z of the fifty-two, and has the other
			// forty-seven. These are the ones it has and this shell does
			// not, so a script asking for one is told it is missing rather
			// than told this shell knows better than zsh what zsh has.
			"set": "dgiklprstwyABDEFGHIJKLMNOPQRSTUVWXYZ",
			// read's letters about a terminal or the line editor — raw -k
			// keys, -q's one keystroke, -e/-E echoing, -z and the zle pair
			// -c/-l. The -p coprocess is implemented as its measured
			// refusal — see ReadNoCoprocess. zsh's read also says nothing
			// at all about a dead -u descriptor and reports 1, which is why
			// no ReadBadFileDescriptor wording appears here.
			"read": "kqeEzcl",
			// typeset's letters this engine does not hold: floats (-E -F),
			// namerefs (-n), padding and alignment (-L -R -Z), and the
			// rest. The same set under both names, and for `local` too.
			// `-H`, `-U` and `-T` have left this list — they are
			// implemented, in DeclareOptions above.
			"typeset": "bcEhkLmnRtZ",
			"type":    "mvwsS",
			// jobs' letters that are zsh's own: -d names the directory the
			// job was started in, and -z and -Z are about the process
			// title rather than about the job table.
			"jobs":    "dzZ",
			"declare": "bcEhkLmnRtZ",
			"local":   "bcEFhkLmnRtZ",
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
		// The one dialect that words the refusal to unset exactly as it words
		// the refusal to assign, and the only one that does not name `unset`.
		UnsetReadonly: "read-only variable: %s",
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
		// The same split ksh93 makes, said the other way round: the text
		// that could not be an operand is named where there is one, and the
		// end of the string is named where there is not.
		// A substring range that begins with a letter is a modifier list
		// here, and an unknown modifier is named unless the segment began
		// with a known one — `${x:i}` names `i` and `${x:ha}` names nothing.
		UnrecognizedModifier:      "unrecognized modifier `%[1]s'",
		UnrecognizedModifierAlone: "unrecognized modifier",
		ArithOperandExpected:      "bad math expression: operand expected at `%[1]s'",
		ArithExpressionRanOut:     "bad math expression: operand expected at end of string",
		ArithOperatorExpected:     "bad math expression: operator expected at `%[1]s'",
		// The operator names its two-character spelling whichever of the two
		// was written: `$((#\))` says `after ##` as readily as `$((##))`.
		ArithCharacterMissing: "bad math expression: character missing after ##",
		SyntaxUnexpected:      "parse error near `%[1]s'",
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
		// And the same number for a sourced file given up over an *error*,
		// where a fatal error that reaches the top of a script reports 1.
		SourcedFatalStatus:  126,
		DotCannotOpen:       ".: no such file or directory: %[1]s",
		DotCannotOpenStatus: 127,
		DotNoOperand:        ".: not enough arguments",
		DotNoOperandStatus:  1,
		// zsh leads with the reason, lowercased, and names the command after
		// it — the reverse of the other three.
		// zsh alone says something for a shift it survives; bash says
		// nothing, and the two that speak up here treat it as fatal.
		ShiftTooMany: "shift count must be <= $#",
		// A sentence of its own for the other end, naming neither the count
		// nor the word.
		ShiftNegativeCount:   "argument to shift must be non-negative",
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
		PrintfMissingVerb:      "%[1]s: invalid directive",
		UmaskBadMask:           "bad umask",
		// The builtin's name comes from the location, as everywhere in zsh.
		UnaliasNotFound:        "no such hash table element: %[2]s",
		UnaliasAllWithOperands: "-a: too many arguments",
		UnaliasUsage:           "not enough arguments",
		UnsetPatternUsage:      "%[1]s: not enough arguments",
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
		WaitNoSuchJob: "wait: %[1]s: no such job",
		KillNoSuchJob: "kill: %[1]s: no such job",
		// The wording is the shared one; the number is not. Measured,
		// `jobs %9` reports 127 — a command that is not there — which is the
		// same number this shell's `wait` gives a job that is not there.
		NoSuchJobStatus:                  127,
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
			// `typeset` says the same as `local`, measured — and it is
			// reached only from `typeset -T`, which is the one shape of the
			// builtin that checks its operands are names. A bare
			// `typeset ':'` is refused in this shell and taken here; see the
			// issue filed from #1045.
			"typeset": "not valid in this context: %[2]s",
			"declare": "not valid in this context: %[2]s",
		},
		// An operand that starts with a digit is a different complaint, for
		// the two that have one. `unset` says the same to both.
		BuiltinBadNameNumeric: map[string]string{
			"export":   "not an identifier: %[2]s",
			"readonly": "not an identifier: %[2]s",
			"local":    "not an identifier: %[2]s",
			"typeset":  "not an identifier: %[2]s",
			"declare":  "not an identifier: %[2]s",
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
		KillUsageStatus:     1,
		TestUnaryExpected:   "unknown condition: %[1]s",
		TestBinaryExpected:  "condition expected: %[1]s",
		TestIntegerExpected: "integer expression expected: %[1]s",
		// The whole substitution as it was written, not its inside.
		ProcessSubstitutionNotInCondition: "process substitution %[1]s cannot be used here",
		TestTooManyArguments:              "too many arguments",
		TestOperandExpected:               "argument expected",
		TestMissingBracket:                "']' expected",
		TimesDecimals:                     2,
		TimesArguments:                    "times: too many arguments",
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
	// The styles database, which a real rc file fills in before it does
	// anything else. See zstyle.go.
	registerZstyle(r)
	// The two of `zsh/zutil`'s remaining builtins that can be learned by
	// running the real one: an option parser for shell functions, and a
	// string formatter. `zregexparse`, the fourth, is a completion-system
	// internal the manual describes in one sentence and is deliberately
	// absent — and `zmodload zsh/zutil` loads anyway, because a missing
	// builtin refuses by name at the word that runs it. See zparseopts.go,
	// zformat.go and the rule at the top of zmodload.go.
	registerZparseopts(r)
	registerZformat(r)
	// And the line editor's key table, which a real rc file also reaches for.
	// See bindkey.go.
	registerBindkey(r)
	// The module loader, which answers per module rather than pretending to
	// load anything. See zmodload.go.
	registerZmodload(r)
	// And the other half of a module system: a name defined from `$fpath`
	// the first time it is called. See autoload.go.
	registerAutoload(r)
	// Five of `zsh/parameter`'s thirty-three: this shell's own tables read
	// through as associations, produced when they are asked for rather than
	// stored. See parameter.go.
	registerParameterModule(r)
	// This shell's richer `echo`, and not ksh93's builtin of the same
	// spelling: different letters, a different escape set and different
	// wordings, all measured side by side. See print.go.
	registerPrint(r)
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
	// The login name for that same uid, for the `%n` prompt escape. Read
	// here for the reason the uid above is: nothing a script does changes
	// it, two shells in one program genuinely have the same one, and it has
	// no other source. Measured as a fact about the *process* rather than
	// about the environment — `%n` ignores `$USER`, `$LOGNAME` and
	// `$USERNAME` however they are set — so it is not read out of a variable.
	if u, err := user.Current(); err == nil {
		r.SetPromptUser(u.Username)
	}
	r.SetSpecial("EUID", strconv.Itoa(os.Geteuid()))
	r.SetDynamic("RANDOM", func(*interp.Runner) string { return interp.Randoms() })
	r.SetDynamic("SECONDS", func(rr *interp.Runner) string {
		return strconv.Itoa(int(rr.SecondsFrom()))
	})
	// A NUL as well as the three whitespace characters, which is zsh's alone.
	r.SetSpecial("IFS", " \t\n\x00")
	tieTheBuiltInPairs(r)
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
