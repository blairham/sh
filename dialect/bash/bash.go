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
	// A `$( … )` body is parsed while the line that holds it is read, so a
	// body that will not parse refuses the line before any of it runs — and
	// refuses it even where the substitution is in a branch nothing takes.
	// The older spelling is not: `` v=`if` `` is read when the word is
	// expanded here. Measured 2026-09-19; 3.2 reads neither with the line,
	// so this is a version line inside one lineage exactly as
	// ArithDoubleQuote is. See syntax.Dialect.SubstitutionBodyRead (#2857).
	d.SubstitutionBodyRead = syntax.NewerSubstitutionBodyReadWithItsLine
	// A here-document a one-line `$( )` or `<( )` opened is fed from the
	// lines after the enclosing command, and the substitution yields the
	// body. 5.3's and not 3.2's, and not the backquoted spelling's; see
	// syntax.Dialect.HeredocBodyFromAfterTheCommand for the panel (#3711).
	d.HeredocBodyFromAfterTheCommand = syntax.HeredocBodyAfterEitherParenthesizedSpelling
	// And a `'` inside a double-quoted operand keeps what it opens out of
	// that read: `echo before; echo "${v-'$(if)'}"; echo after` writes
	// `before` here and is refused outright by dash and BusyBox ash, which
	// read the same body with the same line. Measured 2026-09-19 — and in
	// POSIX mode bash joins them, which this dialect does not yet follow.
	// See syntax.Dialect.AQuotedOperandHidesASubstitutionFromItsLine.
	d.AQuotedOperandHidesASubstitutionFromItsLine = true
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
	// A quotation inside a subscript holds its brackets here: `declare -A a;
	// a[']']=5; (( a[']'] ))` is 5 in 5.3.20 and under the `sh` name alike.
	// See syntax.Dialect.ArithSubscriptQuoting (#3302).
	d.ArithSubscriptQuoting = true
	// bash is the panel's holdout: it expands interactively and needs `shopt
	// -s expand_aliases` anywhere else. Measured 2026-09-12 on all three
	// routes, and in `eval`, `$( )`, a sourced file and a trap body — nothing
	// expands until the option is on, and everything does once it is.
	d.AliasesExpandUnlessTold = false
	// And once it is on, the program text expands by every route: `bash -c
	// $'shopt -s expand_aliases\nalias t=echo\nt TOP'` writes TOP, and so do
	// the same three lines in a file and on standard input. The route set
	// used to be empty here, which conflated "off until asked" with a rule
	// about routes and is what #2109 split apart.
	d.ExpandAliasesInProgramText = syntax.RouteOnEveryRoute
	// And a body's newlines do not count: bash alone leaves the whole of an
	// expanded body on the line the alias word was written on.
	d.AliasBodyCountsLines = false
	// A reserved word may be aliased and the alias wins: `alias for=echo` on
	// one line makes `for x in 1` on the next a command, in 5.3 and in the
	// 3.2 macOS ships alike. POSIX mode takes it back — that half is
	// interp.Runner.SetPosixMode, in both directions — which is what makes
	// it a mode this shell enters and leaves rather than a build.
	d.AliasesExpandReservedWords = true
	// And a value's trailing blank goes on working when the body also opens
	// a quote, so the word past the quote is offered to the table in turn.
	// bash offers the next *word* of the resulting line; dash, ksh93, zsh and
	// BusyBox ash offer the text immediately after the value, which is inside
	// the quote and is no word. `alias c='CEE'; alias q='echo "x '` used as
	// `q b" c` is `x  b CEE` here and `x  b c` in those four (#2685).
	d.AliasTrailingBlankReachesPastAnOpenConstruct = true
	// A backslash an alias body ends with reaches the newline after the
	// alias word, where it is an ordinary line continuation and joins the
	// next line to the word. zsh and bash 3.2 are the columns that do not.
	// See syntax.Dialect.AliasBodyBackslashJoinsTheNextLine (#2710).
	d.AliasBodyBackslashJoinsTheNextLine = true
	// The utilities that take an array assignment as an operand. bash has
	// all five.
	d.DeclarationUtilities = map[string]bool{
		"declare": true, "typeset": true, "local": true,
		"export": true, "readonly": true,
	}
	d.CaseContinue = true
	// A subscript written at command position runs to its matching `]`, so
	// `m[foo bar]=v` is the element keyed `foo bar` rather than the command
	// `m[foo`. Measured 2026-09-12 in 5.3.15, 3.2.57 and as `sh` — all three
	// read it back as `declare -A m=(["foo bar"]="v" )` — against zsh 5.9.2,
	// which refuses the same text with `bad pattern: m[foo`. See
	// [syntax.Dialect.SubscriptSpansSeparators] for the rest of the rows.
	d.SubscriptSpansSeparators = true
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
	// A single quote written anywhere inside a double-quoted `${ … }` quotes
	// what follows it, so the scan for the closing brace runs past a `}`
	// standing between two quotes: `echo "[${v-'}'}]"` is `['}']` here and
	// `[''}]` in the same build invoked as `sh`, in zsh, ksh93, dash and ash,
	// where only a *pattern* operand's quote does that. bash 3.2 answers with
	// 5.3. See Dialect.QuoteProtectsTheClosingBrace (#2399).
	d.QuoteProtectsTheClosingBrace = syntax.BraceQuoteProtectsEveryOperand
	// And POSIX mode takes it back to the core's reading, for the *word*
	// operand alone. Measured 2026-09-14 on 5.3.15 over `v=Vx}y; printf
	// '[%s]' "${v-'a}b'}"`: `[Vx}y]` by default and `[Vx}yb'}]` under
	// `set -o posix`, `--posix`, `POSIXLY_CORRECT=1` and the name `sh`, with
	// `s=a}b; printf '[%s]' "${s#'a}'}"` unmoved at `[b]` under every one of
	// them. Set here and nowhere else: zsh's own mode leaves the axis alone,
	// and the zero value is what says so. See
	// syntax.Dialect.QuoteProtectsTheClosingBraceInPosixMode (#2604).
	d.QuoteProtectsTheClosingBraceInPosixMode = syntax.BraceQuoteMovesToAPatternOnly
	// A backslash and both quotes hold a `]` back from ending a `${a[ … ]}`
	// subscript, so `${m['a]b']}` reads back the key `m['a]b']=v` wrote.
	// `$'…'` does not, and that is the one cell where this shell and ksh93
	// part. Measured 2026-09-15 on 5.3.20 with `a=(9 8 7); echo
	// "[${a['0]'+1]}]"`: an arithmetic error naming `'0]'+1` where the quote
	// protects, and the text `[1]]` — the `+` read as the alternate-value
	// operator — where it does not. Unmoved by `--posix` and by the name
	// `sh`. See Dialect.SubscriptQuoteProtectsTheClosingBracket (#2299).
	d.SubscriptQuoteProtectsTheClosingBracket = syntax.SubscriptBackslashQuotes |
		syntax.SubscriptSingleQuotes | syntax.SubscriptDoubleQuotes
	// A syntax error between a compound assignment's parentheses ends that
	// line and not the file: `a=(p & q)` is reported, the line is thrown away
	// unrun, and the next one runs. bash 3.2 answers the same; `sh`, zsh,
	// ksh93 and dash all stop. See
	// Dialect.CompoundAssignmentErrorGivesUpTheLine (#2380).
	d.CompoundAssignmentErrorGivesUpTheLine = true
	// `$"..."`, the locale-translatable string: with no catalog it is a
	// plain double-quoted string with the `$` stripped. Not core, because
	// dash and zsh keep the `$` as a literal.
	d.DollarDoubleQuote = true
	// `${ cmd;}`, a command substitution that runs in the current shell
	// so that what it assigns survives. The space after the brace is the
	// whole of the grammar: `${x}` is a parameter and `${ x}` is not.
	d.CurrentShellSubstitution = true
	// `${| cmd;}`, the same substitution taking its value from what the body
	// left in `$REPLY` rather than from what it printed — the one expansion
	// that lets a function return a value without a subshell and without the
	// caller naming a variable. bash 5.3 alone: bash 3.2 and zsh call it a bad
	// substitution, dash calls it a bad substitution, and ksh93 — which has
	// the blank form — answers `` `|' unexpected `` (#2656).
	d.ReplySubstitution = true
	// A here-document inside parentheses that hold a program ends at the
	// closing one: `v=$(cat <<EOF` / `a` / `EOF)` is accepted and `v` is `a`.
	// dash and zsh read the body from the whole input instead, so the `)`
	// goes into it and the construct is never closed (#963).
	d.HeredocEndsAtClosingParen = true
	// And the looser half of it: at the end of a substitution's text a
	// here-document that never saw its delimiter retries the last line as a
	// prefix, consuming the delimiter and parsing the rest of the line as
	// more of the substitution. bash 5.3 only — bash 3.2 takes the line as
	// body, and ksh93 refuses the shape outright. See
	// syntax.Dialect.HeredocLastLineIsADelimiterPrefix for the rows and for
	// the control that keeps a well-formed document from ending early.
	d.HeredocLastLineIsADelimiterPrefix = true
	// A body line joined out of two physical ones is compared against the
	// delimiter whole, so `A\` over `BC` ends an `ABC` document. dash and
	// BusyBox ash take only a join that began at the start of a line and
	// ksh93 takes neither (#2430).
	d.HeredocDelimiterAcrossAContinuation = syntax.HeredocDelimiterOnTheJoinedLine
	// A `<<-` delimiter written with a leading tab — quoted, since nothing
	// else can start with one — is met by a body line spelled exactly like
	// it before its tabs are stripped. zsh and ksh93 strip the delimiter too
	// and dash meets it with nothing. See syntax.HeredocDelimiterTabs.
	d.StrippedHeredocDelimiter = syntax.HeredocLineAsWrittenMeetsTheDelimiter
	// The end of the input at a `case` arm's pattern is the newline that
	// would have ended the line, so `case x in x) :;; zzz` names the newline
	// where the other five columns name the end of the file or the `case`
	// that never closed. See syntax.Dialect.CaseWordRunsOutAsANewline for
	// the two spellings that separate a reading from an accident (#2251).
	d.CaseWordRunsOutAsANewline = true
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
	// And the word it carries is the *source text*, which is also what is
	// tested for being a name: `function 'f' { … }` is refused here where
	// the two shells that remove the quotes define `f`. See
	// syntax.Dialect.FunctionNameIsSourceText for the six columns (#1566).
	d.FunctionNameIsSourceText = true
	// And the `function` keyword's name is any word *written bare*, whatever
	// its characters: `function a=2 { :; }`, `function [ { :; }` and
	// `function a*b { :; }` all define here and are called by those names,
	// where a word carrying quoting or an expansion is refused above. The
	// accepted set was the identifier rule plus punctuation, which is
	// narrower than this shell by `= [ { } ~ ? *` at least (#3193). See
	// syntax.Dialect.FunctionKeywordNameIsAnyBareWord for the rows.
	d.FunctionKeywordNameIsAnyBareWord = true
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
	// A `!` with no pipeline after it is a pipeline of its own here, and the
	// negation of a success: `true; !` and `false; !` both answer 1. It
	// reaches a statement terminator and nothing further — `{ ! ; echo x; }`
	// runs where `{ ! }`, `( ! )` and `! && echo two` are all refused — which
	// is what separates this value from ksh93's and zsh's. bash 3.2 refuses
	// every one of them, and `bash --posix` on this build takes them, so it
	// is the version and not POSIX mode (#948).
	d.BareNegationReach = syntax.BareNegationBeforeATerminator
	// And a second `!` inverts the first rather than being refused:
	// `! ! true` answers 0 and `! ! false` answers 1.
	d.RepeatedNegationToggles = true
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
	d.EndOfInputIsANewlineWhereNoneCouldStand = true
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
	// A here-document whose delimiter never arrived gains the newline its
	// last line never had: `printf 'cat <<X\nbody'` writes five bytes here
	// and four in dash, ksh93 and zsh. Both bash builds measured agree
	// (#1020).
	s.UnterminatedHeredocGainsATrailingNewline = interp.Yes
	// A substitution body that does not parse is a failed *expansion* here
	// rather than a failed script: the refusal is reported, the word expands
	// to the empty string, and the statement and the script carry on at 0.
	// dash, ksh93 and zsh all abandon the input instead, which is what this
	// shell's answer was until #2703 — so `echo A; echo "`+"`echo \`echo n`"+`"; echo B`
	// stopped at `A` where bash prints `A`, an empty line and `B`.
	s.SubstitutionParseErrorIsFatal = interp.No
	// But where it *is* fatal — the `$( … )` spelling, which this axis is not
	// asked for — a subshell does not contain it: bash 5.3.20 ends the script
	// from inside `( … )`, a pipeline element and an enclosing `$( … )`
	// alike. bash 3.2.57 is the third answer and does not even end the
	// subshell, which is the row above rather than this one (#3274).
	s.SubstitutionParseErrorEscapesASubshell = interp.Yes
	// And a **here-document body** is the one redirection half it does not
	// end the script from. Measured 2026-09-17 on bash 5.3.20: `cat <<END`
	// over a body holding `$(echo hi; for)` reports the refusal, does not run
	// `cat`, and runs the rest of the script with 1 left behind — where the
	// *target* of the same redirection, `cat < "$(echo hi; for)"`, ends the
	// script at 2. The two halves are measured apart because this column is
	// the one that splits them (#3318).
	s.SubstitutionParseFailureInAHeredocBodyEndsTheShell = interp.No
	// The 1 that carries on is a fatal error's number and not a syntax
	// error's, and the borrowed-text rows are what say so rather than the
	// here-document row alone: measured 2026-09-17, `v=$(echo hi; for)` at
	// the top level of a script exits 2 here, where the same failure inside a
	// file `.` read or inside an `eval` argument exits **1**. A plain syntax
	// error in that sourced file is the control and is a different shape
	// entirely — `.` reports 2 and the script runs on (#3319).
	s.SubstitutionParseFailureCarriesTheFatalStatus = interp.Yes
	// An associative array's subscript is a quoting context here: the key is
	// the text inside its quotes, so `m["k"]=W` stores under `k` and
	// `${m["k"]}` reads it back. zsh takes the subscript as written and
	// stores under the three characters.
	s.SubscriptIsAQuotingContext = interp.Yes
	// `unset` of a name a *calling* function made local takes the binding
	// away here, so the next scope out answers for the rest of the script:
	// the panel's other three leave the name unset until the call that
	// declared it returns. Measured on bash 5.3.20 and bash 3.2.57 alike —
	// see interp.Semantics.UnsetRemovesAnEnclosingLocal for the rows and for
	// the four that say the binding is gone rather than hidden. This is the
	// one column a script can move, with `shopt -s localvar_unset`.
	s.UnsetRemovesAnEnclosingLocal = interp.Yes
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
	// The parameter whose patterns take names back out of a pathname
	// expansion, and the hidden-name switch an assignment to it writes. Both
	// measured on bash 5.3.15 and bash 3.2.57, 2026-09-13 — the run is in
	// Semantics.IgnoredNamesVariable, which also records why this follows
	// the assignment rather than the value. ksh93's `FIGNORE` is the same
	// facility under another name and does not answer alike; it is filed
	// rather than guessed at.
	s.IgnoredNamesVariable = "GLOBIGNORE"
	// And the order the expansion comes back in, which is 5.3's and is this
	// column's alone: no other shell in the panel has a parameter for it.
	// See interp.Semantics.SortOrderVariable.
	s.SortOrderVariable = "GLOBSORT"
	s.IgnoredNamesRevealHiddenNames = true
	// A colon-separated list, matched against the *word* the expansion
	// produced, and a state an assignment latches rather than the
	// parameter's own: an inherited value does nothing until something
	// assigns it. ksh93's parameter of the same kind answers the other way
	// to all three.
	s.IgnoredNamesValueIsOnePattern = interp.No
	s.IgnoredNamesMatchTheLastComponent = interp.No
	s.IgnoredNamesFollowTheParameter = interp.No
	// `.` and `..` are not in what a pattern may match. bash 3.2 lists them
	// and 5.3 does not, so this is the version's answer and not bash's; no
	// preset here is bash 3.2.
	s.GlobListsDotAndDotDot = interp.No
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
	// The system-wide file, and this shell has one only in the login slot.
	// Measured 2026-09-12: a login bash's `~/.bash_profile` reports
	// `path_helper`'s `${PATH%%:*}` and not the inherited one, so
	// `/etc/profile` ran in front of it — in 5.3, in 3.2 and under an
	// argv[0] of `sh` alike.
	//
	// **No system-wide run-commands file**, which is measured rather than
	// assumed and is the interesting half. `/etc/bashrc` exists on this
	// machine and sets `PS1` and `checkwinsize`; a `~/.bashrc` that reports
	// `$PS1` sees bash's own `\s-\v\$ ` default, and `shopt checkwinsize`
	// answers `off` in 3.2. So bash reaches `/etc/bashrc` only through
	// `/etc/profile`, which sources it by hand for a login shell, and a
	// shell that named it here would read it twice.
	s.SystemStartupFiles = interp.SystemStartupFiles{Login: "profile"}
	// The panel's holdout on ordering, and the reason every bash tutorial
	// tells a person to source `~/.bashrc` from their `~/.bash_profile` by
	// hand: `bash -l -i` reads the profile and stops. zsh reads both.
	s.InteractiveStartupFileWhenLogin = interp.No
	// The escape hatches, all three spelled long. `--rcfile` and
	// `--init-file` are the same option, measured to behave identically, and
	// both are carried because a person's muscle memory has one of them.
	s.StartupFileOptions = interp.StartupFileOptions{
		Login: "-l --login",
		// `--noprofile` names the *slot* and not the file: measured, `bash
		// --noprofile -l -c 'echo ${PATH%%:*}'` answers the inherited head,
		// so it suppressed `/etc/profile` along with `~/.bash_profile`.
		SuppressLogin:       "--noprofile",
		SuppressInteractive: "--norc",
		NameInteractive:     "--rcfile --init-file",
	}
	// What `bash --version` writes. Measured 2026-09-11: the real shell
	// answers on standard output at status 0, and the first line is the one
	// scripts read — see version, in prelude.go, for why the tag is there.
	s.VersionOption = interp.VersionOption{Spellings: "--version", Text: versionLine()}
	// And `--help`, which is the same usage block Diagnostics already writes
	// under a refused option with a version line over it and six lines under
	// it. Measured 2026-09-18 on bash 5.3.20 with standard input on
	// /dev/null: `bash --help` and `bash --badopt` were diffed line by line
	// and the twenty-two lines between them are byte-identical, which is why
	// the block is drawn from the one place rather than copied here.
	//
	// On standard output and at 0, where the refusal is standard error at 2 —
	// the word is a request this shell has rather than one it does not, and
	// before #3268 it was answered as `--help: invalid option` with the right
	// block under the wrong sentence.
	//
	// The trailer names the shell twice, as the block above it does. The last
	// two lines are addresses and the blank line before them is part of the
	// output, measured.
	s.HelpOption = interp.HelpOption{
		Spellings: "--help",
		Text:      helpVersionLine(),
		Trailer: "Type `%[1]s -c \"help set\"' for more information about shell options.\n" +
			"Type `%[1]s -c help' for more information about shell builtin commands.\n" +
			"Use the `bashbug' command to report bugs.\n" +
			"\nbash home page: <http://www.gnu.org/software/bash>\n" +
			"General help using GNU software: <http://www.gnu.org/gethelp/>",
	}
	// The option that writes the program back instead of running it, which
	// is this shell's alone in the panel. What it writes is the *listing*
	// layout applied to a file and not a formatter's output — every comment
	// and the shebang with it are gone — and the status a parse failure
	// exits is 1 where a run of the same script exits 2, both measured.
	// See interp.ScriptListingOption and ScriptListingLayout.
	s.ScriptListingOption = interp.ScriptListingOption{
		Spellings:          "--pretty-print",
		ParseFailureStatus: 1,
	}
	// The options that ask for the strings the program marked for
	// translation — `$"…"` — rather than a run of it. This shell's alone in
	// the panel, and the only one of the five that reads the mark at all.
	// Nothing here translates anything: see interp.StringCatalogOption.
	s.StringCatalogOption = interp.StringCatalogOption{
		Spellings:               "-D --dump-strings",
		PortableObjectSpellings: "--dump-po-strings",
	}
	// `-O shopt_option`, which is this shell's alone: the letter whose next
	// word is a name in the `shopt` table rather than a `set` option. The
	// usage block above already advertises it — `-ilrsD or -c command or -O
	// shopt_option (invocation only)` is bash's own line, printed here under
	// a refused option — and until #3264 it was advertising an option the
	// front end did not have, so `bash -O checkhash -c 'shopt checkhash'`
	// took `checkhash` as the script and exited 127 looking for a file
	// nobody named. The measurement across the panel is on the axis; the
	// names the letter reaches are in shopt.go.
	//
	// bash 3.2.57 has it too, measured, so it is not gated on the version.
	s.ShellOptionInvocationLetter = "O"
	// `-c` and `-s` together: the command string names the operands here,
	// so `sh -sc CMD name a` has `$0` of `name` and one parameter — the
	// same answer in the 3.2 macOS ships. ksh93 and zsh let `-s` name them.
	s.StdinOptionNamesTheOperands = interp.No
	s.CommandNotFoundStatusIsNotFound = interp.No
	// A pathname operand comes back unchanged from `command -v`, `command -V`,
	// `type`, `type -p`, `type -P` and `type -a` alike — `./bb/tool` stays
	// `./bb/tool` — against POSIX's reading, which only ksh93 follows.
	// Measured 2026-09-14 in 5.3.15 and in the 3.2.57 macOS ships.
	s.APathnameOperandIsReportedAbsolute = interp.No
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
	// And the feature that letter names. bash has a history expander and
	// starts an interactive session with it on — measured 2026-09-15 through
	// a pseudo-terminal with a two-row prompt, where `echo one two three`
	// followed by `echo !!` echoes `echo echo one two three` to standard
	// error and prints `echo one two three`, with nothing configured and no
	// `set -H` typed. bash 3.2 answers identically, and so does the same
	// binary invoked as `sh`.
	s.HistoryExpansion = interp.Yes
	// `set -k` and `set -o keyword`, and this is the column that reaches
	// *past* a declaration utility with them: measured 2026-09-16, `set -k;
	// export E1=e1` leaves `export` listing the environment and `E1` unset.
	// See Semantics.KeywordAssignments and keywordassign.go.
	s.KeywordAssignments = interp.Yes
	s.KeywordPromotesADeclarationsOperand = interp.Yes
	// A declaration utility is recognized from the command word as written,
	// and only from an unquoted literal one: `cmd=export; $cmd v=$b`,
	// `\export v=$b`, `'export' v=$b`, `"export" v=$b`, `expor't' v=$b` and
	// `e=; $e export v=$b` all split the value where `export v=$b` keeps it.
	// Measured 2026-09-16 on bash 5.3.20 with `b='x y'`, for export, readonly,
	// local, typeset and declare alike. See #3339.
	s.DeclarationCommandWord = interp.DeclarationByUnquotedLiteralWord
	// And `command` in front of one does not keep the rule: `command export
	// v=$b` splits, in every spelling of the prefix. Measured the same day.
	s.CommandPrefixKeepsADeclaration = interp.No
	s.HistoryExpansionAtAPrompt = interp.Yes
	// And bash is the one column that uses the expander where nobody is
	// typing. Measured 2026-09-16 from a script file, from `-c` and from
	// standard input alike: after `set -o history` and `set -H`, `echo !!`
	// writes `echo echo one two three` to standard error and runs it, while
	// zsh with `setopt banghist` and ksh93 with `set -H` both print the two
	// characters. See Semantics.HistoryExpansionInAScript.
	s.HistoryExpansionInAScript = interp.Yes
	// And POSIX mode takes double-quoted text out of its reach. See
	// Semantics.HistoryExpansionSparesDoubleQuotesInPosixMode.
	s.HistoryExpansionSparesDoubleQuotesInPosixMode = interp.Yes
	// And an event's words are the shell's own. See
	// Semantics.HistoryWords.
	s.HistoryWords = interp.HistoryWordsShell
	// And a comment is left as written. See
	// Semantics.HistoryCommentStopsExpansion.
	s.HistoryCommentStopsExpansion = interp.Yes
	// `enable` has a `-d`, so a refusal names the operand rather than the
	// letter: measured 2026-09-18, `enable -d notbuiltin` is `enable:
	// notbuiltin: not a shell builtin` at 1 in 5.3.20 and in 3.2.57 alike,
	// where zsh answers `bad option: -d`. See
	// Semantics.EnableUnloadsABuiltin.
	s.EnableUnloadsABuiltin = interp.Yes
	// And a `G` in front of an `s` substitutes once per word. Measured
	// 2026-09-18 after `echo foo boo`, `!!:Gs/o/0/` is `ech0 f0o b0o`,
	// which zsh and ksh93 both refuse as a modifier they do not know. See
	// Semantics.HistoryWordwiseSubstitutionModifier.
	s.HistoryWordwiseSubstitutionModifier = interp.Yes
	// `bash -c 'echo $-'` reports `hBc`; ksh93 agrees and dash and zsh do
	// not. The `s` of the standard-input route is not added under `-c`
	// here — ksh93 alone does that.
	// `+i` takes the prompt back. Measured 2026-09-16 on bash 5.3.20 and
	// bash 3.2.57, the program on a pipe: `bash -i +i -c 'echo $-'` and
	// `bash +i -c 'echo $-'` both report `hBc`, with no `i` and no job-control
	// notice, where `-i` alone reports `hiBHc` and announces one.
	s.PlusSignedInteractiveLetterStillPrompts = interp.No
	s.CommandStringShowsCInDollarDash = interp.Yes
	s.LoginShowsLInDollarDash = interp.No
	s.CommandStringShowsSInDollarDash = interp.No
	// And the order it publishes them in: the lowercase letters sorted, then
	// the uppercase ones sorted, then the letter naming the route it was
	// invoked by. Measured 2026-09-12 on bash 5.3.15, and every row of it is
	// a shell disagreeing with the order the script wrote:
	//
	//	set -f; set -u; set -e   efhuBc
	//	set -C                   hBCc
	//	set -C -e                ehBCc
	//	set -a                   ahBc
	//	-i -c, at a terminal     himBHc
	//	set -e -C, on stdin      ehBCs
	//
	// The last two are what say the trailing letter is the route's and not
	// `c` in particular: `i` and `m` sort in with the rest where `s` does
	// not. Which of `c` and `s` comes first is not measurable here — this
	// shell never shows both, being the one that answers `No` to
	// CommandStringShowsSInDollarDash — so the pair is written in the order
	// the substrate produces them.
	s.DollarDashLetterOrder = "aefhkilmntuvxBCEHTcs"
	s.ArrayScalarIsTheWholeArray = interp.No
	// And the one element a plain `$m` on a keyed table gives is the one
	// keyed `0`, which is nothing at all where no such key was written.
	s.KeyedTableScalarIsTheFirstValue = interp.No
	s.ArrayNameWithoutSubscriptIsTheList = interp.No
	// A subscript inside a literal is an expression: `a=([1+1]=c)` lands at 2.
	s.ArrayLiteralSubscriptIsAKey = interp.No
	// A `[k]+=` element of a replacing keyed literal joins the value the
	// *name* held, which `m=(…)` is in the middle of discarding:
	// `typeset -A m; m[k]=v; m=([k]+=x)` is `vx` here and `x` in the other
	// two — see Semantics.KeyedLiteralAppendJoinsTheReplacedValue.
	s.KeyedLiteralAppendJoinsTheReplacedValue = interp.Yes
	// `typeset -A m[k]=v` — see Semantics.TableLetterReachesItsOwnOperandsSubscript.
	s.TableLetterReachesItsOwnOperandsSubscript = interp.Yes
	// bash reads a group built out of an expansion — see
	// Semantics.ExpansionResultSuppliesGroupSyntax.
	s.ExpansionResultSuppliesGroupSyntax = interp.Yes
	// `typeset -r a[1]=v` freezes the array and then loses the element write
	// to the freeze — see Semantics.ReadonlyElement. `readonly a[1]=v` never
	// reaches it here: that spelling is refused as a bad name first.
	s.ReadonlyElement = interp.ReadonlyElementFrozenFirst
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
	// And a compound writes the record for nobody: what stands after one is
	// whatever the last pipeline that actually *ran* inside it left, so
	// `if false; then :; fi` holds the condition's 1 and `case a in b) :;;
	// esac` leaves the record from before it untouched, having run nothing.
	// A redirection on the compound does not change that, which is the
	// opposite of the rule the neighboring axes follow. Measured 2026-09-11
	// on 5.3.15 and 3.2.57 alike (#2016).
	s.CompoundPipelineStatusRecord = interp.CompoundPipelineStatusFromWhatRan
	s.UnsetEndsTheProducedPipelineStatus = interp.No
	// The lines of `eval`'s text continue the line the `eval` is written on,
	// and `$LINENO` moves with them — measured with the whole `eval` on one
	// physical line, which is the only arrangement that can tell this from
	// the physical reading (#2462).
	s.EvalTextContinuesTheCallersLines = interp.Yes
	s.SelectLayout = interp.SelectMenuVerticalThenColumns
	s.SelectPromptNeedsTerminal = interp.No
	s.AliasParsesOptions = interp.Yes
	s.AliasHasPrintOption = interp.Yes
	// `alias -x` is `alias: -x: invalid option` here, at 2.
	s.AliasHasExportOption = interp.No
	// Neither of the other two kinds: `alias -g` and `alias -s` are both
	// `invalid option` here, measured on 5.3 and 3.2 alike.
	s.GlobalAliases = interp.No
	s.SuffixAliases = interp.No
	s.AliasListsAsDefinitions = interp.No
	s.AliasRestrictsToRegularKind = interp.No
	s.AliasOperandsCanBePatterns = interp.No
	s.AliasPlusPrintsNamesOnly = interp.No
	// The command hash, which bash keeps more of than the rest of the panel
	// and reads more strictly. It trusts what it remembered: a hashed path
	// that has gone is reported as that *path* at 127, where the other three
	// walk PATH again and run the next copy. The four letters past `-r`
	// follow from keeping a table worth addressing — `-l` writes it back as
	// commands, `-p` puts an entry in by hand, `-d` takes one out and `-t`
	// reports one. Measured 2026-09-13 against 5.3.15.
	s.CommandHashIsTrusted = interp.Yes
	s.HashListsAsCommands = interp.Yes
	s.HashTakesAPathToRemember = interp.Yes
	s.HashForgetsOneName = interp.Yes
	s.HashReportsThePath = interp.Yes
	// Not sorted: bash walks its own table's buckets, which is not an order
	// this shell can or should reproduce. See interp.Runner.hashedCommandNames.
	s.HashListingIsSorted = interp.No
	// And `set +h` really stops it: nothing is remembered until the option
	// comes back. zsh reads its own `hashcmds` the same way; ksh93 keeps
	// hashing with `trackall` off, and zsh's *letter* `-h` is a history
	// option that never reaches the table.
	s.HashObeysCommandTracking = interp.Yes
	// And the builtin itself closes with it, which zsh's reading of the same
	// option does not: every spelling answers one sentence at 1 here.
	s.HashRefusesWhileTrackingIsOff = interp.Yes
	// And a lookup that only *reports* where a command is leaves the table
	// alone, alone in the panel: `type ls >/dev/null; hash` is an empty
	// table here and holds `ls` in zsh, ksh93 and dash.
	s.ALookupRemembersThePath = interp.No
	s.TypeNamesAnAliasOnlyWhenExpanded = interp.Yes
	s.AliasReportsNotFound = interp.Yes
	s.UnaliasReportsNotFound = interp.Yes
	s.AliasNotFoundStatusCounts = interp.No
	// A removed alias leaves nothing behind: `alias h=1; unalias h;
	// unalias h` is 0 then 1 here, as it is in zsh, dash and BusyBox ash.
	s.AliasRemembersTheNamesItNames = interp.No
	s.AliasOptionEndsTheLookup = interp.No
	// `-p` is the whole listing and stops reading: every operand behind it
	// is discarded, so `alias a=1 b=2; alias -p a` writes both lines,
	// `alias -p nosuch` is silent at 0 where `alias nosuch` is
	// `alias: nosuch: not found` at 1, and `alias -p z=1` defines nothing.
	// The letter and not any option word — `alias -- a` lists `a` alone and
	// `alias -- nosuch` reports. Measured 2026-09-19 (#3701).
	s.AliasPrintOptionIgnoresItsOperands = interp.Yes
	// A command word that is exactly `-` is a command name here and is
	// reported as one: `- echo hi` is `command not found` at 127 and the
	// script carries on. zsh is the column that throws the word away (#3236).
	s.LoneDashInCommandPositionIsDiscarded = interp.No
	s.UnaliasAllRefusesOperands = interp.No
	s.AliasQuoting = interp.ListingQuoteAlwaysEscaped
	s.AliasListingQuotesTheName = interp.No
	s.TrapQuoting = interp.ListingQuoteAlwaysEscaped
	// `declare -p` writes `declare -- v="1"` — double quotes, unlike the
	// single-quoting listings above.
	s.DeclareListing = interp.DeclareListingClustered
	// And a listing with no operands writes the produced parameters with no
	// reading beside them: `declare -i RANDOM`, `declare -- SECONDS`,
	// `declare -- LINENO`, `declare -- EPOCHSECONDS`. Measured 2026-09-13,
	// bash 5.3.15, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over a
	// script file — and the same rows from the same binary under argv[0] of
	// `sh`, so the shape is bash's and not the invocation's.
	//
	// Not what the *named* `declare -p RANDOM` writes in that shell, which is
	// `declare -i RANDOM="16735"` — see interp.ProducedDeclaration, which is
	// where the letters come from and which this deliberately does not
	// re-derive. The two forms disagreeing in one shell is why the axis
	// exists (#2518).
	//
	// bash writes the reading once the parameter has been read, and then
	// writes the *same* one again — two listings a line apart both hold
	// `declare -i RANDOM="22963"`, where ksh93 and zsh draw again. So the
	// gate and the cache are one behavior: a parameter nothing has expanded
	// has no last reading to write. Neither half is modeled here, and both
	// are #2722.
	//
	// One letter moves with that gate and is left as the after state on
	// purpose. `SECONDS` lists as `declare -- SECONDS` until something reads
	// it and `declare -i SECONDS="0"` afterwards — measured 2026-09-13, and
	// it is the only one of the seven that moves; `RANDOM`, `SRANDOM` and
	// `BASHPID` carry `-i` from the start and `LINENO`, `EPOCHSECONDS` and
	// `EPOCHREALTIME` carry none either side. This engine writes `-i` in
	// both states, which is the letter the named `declare -p SECONDS` writes
	// in bash whatever has been read (#2451) and is the after state here.
	s.ProducedParameterListing = interp.ProducedListingLastReading
	s.DeclareValueQuoting = interp.ListingQuoteAlwaysDouble
	s.ListingControlEscape = interp.ControlEscapeOctal
	// A `#` in a listed value is quoted only where a comment could begin —
	// as the value's first byte — and nowhere else. Measured 2026-09-12 with
	// a bare `set`: `ab#cd`, `a#b#c`, `1#b` and `tail#` all list unquoted and
	// `#abcd` does not. It shows in `set` and not in `declare -p`, whose
	// values are double-quoted whatever is in them (#2299).
	s.ListedHashIsBareUnlessItOpensTheValue = interp.Yes
	// And a `~` by the same rule and in the same listings: `a~b` and `b~`
	// are bare, `~b` and `~` are quoted, and a key follows its value —
	// `[a~b]="1"` against `["~b"]="1"`. Measured 2026-09-17 on 5.3.20 and on
	// the 3.2.57 macOS ships, which agree; zsh and ksh93 quote every one.
	// The two position rules compose: `a#~b` and `a~b#c` are bare here and
	// `~a#b` is quoted (#2298).
	s.ListedTildeIsBareWhereItCannotExpand = interp.Yes
	// And the other side of the same character: the leading unquoted `~` of
	// an associative subscript is expanded before the text becomes a key, on
	// a store and on a read alike — `m[~/k]` is the element `$HOME/k` names.
	// Measured 2026-09-17 on 5.3.20; ksh93 agrees and zsh takes the
	// characters (#2298).
	s.SubscriptKeyExpandsALeadingTilde = interp.Yes
	// The three bytes the panel does not agree about in a listed word.
	// Measured 2026-09-14 over `set` and over a keyed `typeset -p`, which
	// agree: `=` is ordinary here — `v=a=b` and the keys `[a=b]`, `[=x]`,
	// `[1=2]`, `[x=y=z]` all bare — and so is a byte above ASCII, `v=é` and
	// `[é]`, which stays itself inside a `$'...'` a control byte opened:
	// `$'a\téb'`. `^` is not: `'^'` and `'a^b'`, where ksh93 leaves both
	// bare. This engine had the ksh93 answer to all three (#2820).
	s.ListedBangIsOrdinary = interp.No
	s.ListedCaretIsOrdinary = interp.No
	s.ListedEqualsIsOrdinary = interp.Yes
	// The character and not the escape, which is bash's answer in every
	// locale but `C` — measured 2026-09-15 on one binary, `LC_ALL=C`
	// writing `$'\303\251'` and every other setting, including none at
	// all, writing `é`. The corpus runs in `C` and records that cell;
	// this shell carries the reading a person's terminal sees.
	s.ListedNonAsciiIsOrdinary = interp.Yes
	// And no bare assignment head: the `=` rule here is the character, not
	// a prefix, so `x=y=z` is bare rather than `x='y=z'`.
	s.ListedAssignmentPrefixIsBare = interp.No
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
	s.SymbolicMaskWhoAloneSetsIt = interp.No
	s.SymbolicMaskTakesTheSetuidLetter = interp.Yes
	// Both are 5.x additions: bash 3.2 calls the `u` of `g=u` and the `X`
	// of `u=X` invalid symbolic mode characters, and this preset is 5.3.
	s.SymbolicMaskTakesAPermissionCopy = interp.Yes
	// And a copy written beside permission letters replaces what they
	// accumulated rather than joining them: from `umask 222`, `umask -S
	// g=uw` is `g=rwx` and `g=wu` is `g=rx`, which is the spelling that
	// parts this from dash (#3074). Measured 2026-09-18.
	s.UmaskPermissionCopyBesideLetters = interp.UmaskPermissionCopyReplaces
	s.SymbolicMaskTakesTheConditionalExecuteLetter = interp.Yes
	s.SymbolicMaskTakesTheStickyLetter = interp.Yes
	// No dash word is an option: `shift -x` complains about a number and
	// `shift -1` is a count out of range. The marker is honored all the same.
	s.ShiftOptionWords = interp.ShiftOptionWordsNone
	s.ShiftDoubleDashEndsOptions = interp.Yes
	// `--` in front of a numeric operand is taken and the number behind it
	// read: `break -- 1` ends the loop, `exit -- 3` exits 3. Identical in
	// 3.2 and in the same binary called as `sh`.
	s.NumericOperandDoubleDashEndsOptions = interp.Yes
	// A second operand is one too many, and the refusal costs the rest of
	// the statement — the loop stops, the `; echo` behind it never runs,
	// and the next line does. Measured 2026-09-16 on 5.3.20 in a script
	// file; 3.2.57 says the same sentence at status 1.
	s.ExtraNumericOperand = interp.ExtraNumericOperandGivesUpTheStatement
	s.ShiftNamesAreArrays = interp.No
	s.ShiftNegativeIsOutOfRange = interp.Yes
	s.WaitReadsOptions = interp.Yes
	// Job specs by command text, with a second match refused as ambiguous;
	// `wait` complains about a spec that names nothing, has -n, and
	// `disown` takes the job out of the table.
	s.JobSpecsByName = interp.Yes
	s.AmbiguousJobNameIsRefused = interp.Yes
	s.WaitReportsAMissingJob = interp.Yes
	// And a job it has already reported stays waitable by its process id
	// once the job has left the table — measured on both routes into it,
	// `wait %1; wait "$p"` and `wait "$p"` twice, in 5.3.15 and 3.2.57
	// alike. See the axis for the narrower answer this same binary gives
	// when it is invoked as `sh`.
	s.WaitRemembersAReapedJob = interp.Yes
	// Nothing is said about the signal that ended a job a `wait` reaped,
	// measured — where the same death in the foreground is reported.
	s.WaitReportsTheSignalThatEndedTheJob = interp.No
	s.WaitNextJob = interp.WaitNextJobFirstToFinish
	// And `-p var` beside it, which names the job the status came from.
	// bash 5's letter alone: the 3.2 build answers `wait: -p: invalid
	// option`. Measured 2026-09-13.
	s.WaitPNamesTheFinishedJob = interp.Yes
	// A trapped signal cuts a `wait` short with 128 plus the signal, and the
	// form that names a job answers the same as the bare one.
	s.WaitForAJobFailsWhenInterrupted = interp.No
	s.DisownRemovesTheJob = interp.Yes
	// And it reports what it did: 0 for a job it disowned, 1 with `no such
	// job` for one it could not find (#3187).
	s.DisownAlwaysFails = interp.No
	s.CommandRejectsUnknownOption = interp.Yes
	// Whether `command -v` answers for every name it was given, and what
	// decides the status when it found some of them. See
	// interp.Semantics.CommandReportsEveryOperand for the split.
	s.CommandReportsEveryOperand = interp.Yes
	s.CommandCountsAMissingOperand = interp.No
	// And whether a `command` reached through an expansion keeps the power
	// to run what it names (#3369).
	s.ExpandedCommandOnlyReports = interp.No
	// The word takes the named builtin's specialness away and draws no
	// boundary of its own: a fatal error raised *inside* it is still fatal.
	// Measured 2026-09-13 in subshells under `env -i`, three producers that
	// this shell does end a script for — `command eval 'echo "${NOPE?bad}"'`,
	// `readonly rv=1; command eval 'rv=2'` and `set -u; command eval 'echo
	// "${NOPE}"'` — each ends the subshell at 1 with the word and without it,
	// under this shell's own name and under `sh` alike. Both builds agree.
	s.FatalErrorEndsAtTheCommandWord = interp.No
	s.GetoptsRejectsUnknownOption = interp.Yes
	s.ShiftCountIsArithmetic = interp.No
	s.TrapBodyRunsWhatParsed = interp.Yes
	// A signal body, an EXIT body and a RETURN body count from their own
	// first line — the zero value, and what TrapBodyLine leaves alone —
	// while a DEBUG or ERR body counts from the line the condition fired
	// on. Measured with a two-line body: fired before a command on line
	// 4, `echo $LINENO` on the body's first line prints 4 and a failure
	// on its second is located at line 5, where the same body on a signal
	// trap prints 1 and locates the failure at line 2. bash 5.3,
	// bash-as-sh and bash 3.2 all answer this way.
	s.CommandTrapBodyLine = interp.TrapBodyLineOffsetFromWhereItFired
	s.ReportsAKilledCommandInACommandSubstitution = interp.No
	// `set -e` is the one option a `$(…)` body does not simply inherit
	// here. Measured 2026-09-15, `set -e; echo "end[$(false; echo no)]"` is
	// `end[no]` in bash 5.3.20 and in bash 3.2.57, and the option is
	// genuinely off in the body rather than unenforced — `$-` inside it
	// carries no `e` and `set -o` reports `errexit off`. The starting value
	// only: this is the `inherit_errexit` switch below, and `set -o posix`
	// turns it on by the other door.
	s.ErrExitEntersACommandSubstitution = interp.No
	// A `${ … ;}` body is a variable scope here as well as a frame: a
	// declaration inside one is local to the body and the shell's own name
	// comes back after it. Measured 2026-09-16 from a script file —
	// `x=outer; v=${ local x=in; printf %s "$x"; }` is `[in]` with `x` still
	// `outer`, and `declare` and `typeset` answer the same — where ksh93u+,
	// the only other column with the construct, leaves `x` as `in`. Nested
	// in a call the scopes nest: with `local x=fn` around it the name comes
	// back as `fn`.
	//
	// A plain assignment is not this and still writes the shell's own name,
	// which is what keeps the spelling different from the forked one. And
	// `local` being legal inside a body follows from the scope rather than
	// being a second decision — in `$( … )` it is refused here exactly as it
	// is at the top level.
	s.CurrentShellSubstitutionBodyIsAScope = interp.Yes
	s.SelectAssumesUnboundedWidth = interp.No
	s.SelectEofEndsPromptLine = interp.No
	s.SelectEofIsSuccess = interp.No
	s.SelectTakesUnterminatedReply = interp.No
	s.SelectEofPrintsNewline = interp.Yes
	s.AssignmentPrefixPersistsOnSpecialBuiltin = interp.No
	// And `command` in front of one takes the persistence away where the
	// option puts it there. Reachable only under `set -o posix`, which is
	// where the row above turns yes, and measured there: 2026-09-18,
	// `set -o posix; s=base; s=C command :` leaves `base` while the bare
	// `s=P :` leaves `P` (#3448).
	s.CommandKeepsASpecialBuiltinsPrefix = interp.No
	// A prefix to a *function* is the command's environment for the length
	// of the call and nothing afterwards: `f(){ echo "[$v]"; }; v=1; v=9 f`
	// prints `[9]`, a command the body starts is told `v=9`, and `$v` is `1`
	// on the next line. Measured 2026-09-12 in 5.3.15 and in 3.2.57, and the
	// same either way round the `posix` switch in 5.3 — 3.2 is the build
	// that moves under it, which is what macOS `/bin/sh` is (#2407).
	s.AssignmentPrefixPersistsAfterAFunction = interp.No
	s.PrefixToAFunctionIsExported = interp.Yes
	// A prefix in front of a `function`-form function writes this shell's
	// own cell, exactly as it does in front of a POSIX-form one: measured
	// 2026-09-18 on 5.3.20, `s=base; function kf { echo "[$s]"; }; s+=5 kf`
	// shows the body `base5`, which is the append reading the shell's value
	// (#3161).
	s.PrefixToAKeywordFunctionIsScopedToTheCall = interp.No
	// And a prefix to a *builtin* is exported too, which is the reading that
	// makes it the command's environment rather than a value this shell holds
	// for one line: `v=1; v=9 eval 'env | grep "^v="'` hands the child `v=9`
	// and `c=1; c=2 declare -p c` reads `declare -x c="2"` on a name nobody
	// exported. Measured 2026-09-16 in 5.3.20 and in 3.2.57, and bash is the
	// only column that does either (#3437).
	// The prefix is worked through **before** the redirections are opened, so
	// a substitution in a value still runs when one fails: measured
	// 2026-09-18, `f() { :; }; w=$(echo S >&2) f > /nope/x` writes `S` and
	// then the file complaint, where zsh, dash and BusyBox ash write the
	// complaint alone (#3449).
	s.PrefixExpandedBeforeTheRedirections = interp.PrefixExpandedBeforeRedirectionsAlways
	// And a declaration utility's operand is reached **before** that prefix,
	// which is the opposite half of the same sequence: measured 2026-09-19,
	// `PRE=$(echo PRE >&2) export s=$(echo OP >&2)` writes `OP` and then
	// `PRE` here and in bash 3.2, where ksh93 and zsh write `PRE` first
	// (#3814).
	s.PrefixExpandedBeforeADeclarationsOperand = interp.No
	s.PrefixExportAtABuiltin = interp.PrefixExportAtABuiltinOn
	// And a listing with **no operand** does not see that prefix at all: it
	// answers from the shell's own variables, so `export k=1; k=9 export -p`
	// writes `declare -x k="1"` on the same line whose `k=9 typeset -p k`
	// writes `declare -x k="9"`. A name the prefix created is in no
	// whole-table listing here. Measured 2026-09-18 (#3446).
	s.PrefixInAWholeTableListing = interp.PrefixInAWholeTableListingIsNotThere
	// A declaration naming `-x` or `-r` over the name its own prefix is
	// standing in front of keeps the prefix's value: `b=7; b=8 readonly b`
	// leaves `declare -rx b="8"`, and so do `export` and `typeset -r`. Not
	// the special-builtin rule above — `b=8 :` leaves `7` here — and not
	// every declaration: `x=2 declare x`, `i=2 declare -i i` and `t=2
	// declare +x t` all leave the shell's own value standing (#3437).
	s.DeclarationPromotesThePrefixEntry = interp.Yes
	// A subscripted name in a prefix is refused by name and the command runs
	// anyway: `a[1]=v f` writes `` `a[1]': not a valid identifier ``, calls
	// `f` with the element untouched, and reports what `f` reported. The
	// other entries of the same prefix are applied — `w=5 a[1]=1 f` shows
	// the body `w` — so the refusal is of the word and not of the prefix.
	// Measured 2026-09-16 on bash 5.3.20 (#3433).
	//
	// unanswered SubscriptedPrefixIsTakenBack: nothing is stored to take
	// back. The axis is asked only where SubscriptedAssignmentPrefix writes
	// the element, and this dialect refuses it instead.
	s.SubscriptedAssignmentPrefix = interp.SubscriptedPrefixIsRefused
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
	// An *unset* locale is UTF-8-capable here, which is this shell alone in
	// the panel and is measured on three operators at once: under `env -i`,
	// 5.3.15 answers 5 for `s=héllo; echo ${#s}`, uppercases `café` to
	// `CAFÉ`, and writes `61 c3 a9 5a` for `echo -e 'a\u00e9Z'`. ksh93u+ and
	// zsh 5.9.2 answer 6 and `CAFé` there, and 3.2.57 answers 6 — so this is
	// the modern build's reading rather than the family's (#2020).
	s.UnsetLocaleIsUnicodeAware = interp.Yes
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
	// `readonly` takes the kind letters and the function letter as well as
	// POSIX's `-p`. Measured 2026-09-12: `readonly -f zz` is `zz: not a
	// function` at 1, so the letter is read; `-i`, `-x`, `-r`, `-g`, `-l`,
	// `-u` and `-t` are all `invalid option` at 2. bash 3.2 has no `-A` —
	// its usage line is `readonly [-af]` — which is a change within bash
	// rather than a difference between shells, and this is 5.3's set,
	// the binary the panel measures.
	s.ReadonlyOptions = "paAfn"
	// And what the `n` of that set does, which is not what the same letter
	// does on `export`: nothing is taken off. Measured 2026-09-18 from a
	// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch
	// HOME — `v=1; readonly -n r=v` is 0 and `declare -p r` is `declare --
	// r="v"`, which `r=5` then writes; `readonly q=1; readonly -n q` leaves
	// `q` frozen; and `readonly -n z=2` over a frozen `z` is the ordinary
	// `z: readonly variable` at 1. So the letter suppresses the freeze this
	// call would have made and does nothing else (#3464).
	s.ReadonlyReferenceLetter = interp.ReadonlyReferenceLetterDeclaresAnUnfrozenName
	// Measured 2026-09-12: `x=1; unset -n x` leaves `x` at 1 and reports 0,
	// where ksh93 removes it. `-n` names the reference and this shell reads a
	// name that is not one as naming nothing at all — far enough that
	// `unset -n 1x` is silent at 0 while plain `unset 1x` refuses the
	// identifier, though a readonly name is still refused (#932).
	s.UnsetReferenceLetterRemovesANonReference = interp.No
	// A name reference that reaches itself through *another* reference is
	// made here rather than refused, and the complaint arrives when
	// something reads through it. Measured 2026-09-15 on 5.3.15: `declare -n
	// a=b; declare -n b=a` is a silent 0, and `echo "$a"` then writes
	// `warning: a: circular name reference` followed by an empty line. The
	// direct `declare -n r=r` is refused in both shells and is the core's
	// answer rather than this axis — see interp/nameref.go.
	s.NamerefCycleIsRefused = interp.No
	// A `-n` declaration over a name carrying an array is refused **after**
	// the two refusals about the name itself, and the array *attribute* is
	// enough: measured 2026-09-16 on 5.3.20, `typeset -a r; typeset -n r=v`
	// is `r: reference variable cannot be an array` at 1 even though `r`
	// holds nothing, while `r=(a b); typeset -n r=r` is the self-reference
	// sentence instead. ksh93 answers both halves the other way round
	// (#3103).
	s.NamerefArrayRefusal = interp.NamerefArrayCheckedLastOnTheAttribute
	// The `n` letter is read beside every other one here and each pair is
	// decided on its own. Measured 2026-09-18 on 5.3.20, `env -i` with a
	// scratch HOME, from a file, with `v=1`: `declare -nx r=v` and
	// `declare -nr r=v` make an exported and a frozen reference, `declare
	// -nu r=v` aims one at `V`, `declare -na r=v` makes a plain array, and
	// only `declare -ni r=v` is refused — at 1, silently, for a reason
	// about the *value* rather than about the pair. So there is no bundle
	// this shell will not read.
	s.NamerefLetterStandsAlone = interp.No
	// The target is the **text** the declaration was written with, looked up
	// again at every read: measured 2026-09-18 on 5.3.20, `a=(x y z); i=2;
	// declare -n r=a[i]` lists `declare -n r="a[i]"` and reads `x` after
	// `i=0`, and `declare -n s=u; declare -n s2=s` lists `s2="s"` and follows
	// `s` wherever it is re-aimed. Which is what lets `declare -n r=a[@]`
	// stand at all — `@` is not arithmetic, and here it never has to be
	// (#3124, #3172).
	s.NamerefTargetResolvedWhenAimed = interp.No
	// And a refusal an assigning declaration reaches through a reference is
	// spoken of under the operand: measured 2026-09-18 on 5.3.20, with `u=1;
	// readonly u; declare -n s=u`, `declare s=9` is `declare: s: readonly
	// variable` where the valueless `declare -i s` beside it is `declare: u:
	// readonly variable` (#3173).
	s.DeclarationThroughAReferenceNamesTheOperand = interp.Yes
	s.ReadZeroTimeout = interp.ReadZeroTimeoutPolls
	s.ReadPartialCountSucceeds = interp.No
	s.ReadExactCountKeepsPartial = interp.Yes
	s.ReadTimeoutKeepsWhatArrived = interp.Yes
	s.ReadTimeoutBoundsReadability = interp.No
	// A directory the PATH search walked past leaves no trace: with nothing
	// runnable anywhere, bash says the name was never found at all.
	s.DirectoryOnPathIsACandidate = interp.No
	// And the candidate kept is the first one that **existed**, whatever it
	// was — which is a third reading beside the two PathCandidateReport
	// already held, and a row `$d` alone on PATH cannot reach. Measured
	// 2026-09-18 with `$d` a directory named `zzcmd` and `$g` a
	// non-executable file of that name: `$d:$g` is `zzcmd: command not
	// found` at 127 here where every other column reaches the file and says
	// `Permission denied` at 126, and the `$g:$d` control is the file in
	// every column. So the directory is kept and suppresses the later row,
	// rather than being walked past (#3578).
	s.PathCandidateReported = interp.FirstExistingCandidate
	// And what a `command -p` search resolved goes into the command hash here,
	// where it does not in the other four: with `PATH=/nonexistent_zz`, a
	// `command -p ls` and a plain `ls` after it, the second is 0 in this
	// column and 127 in zsh, dash, ksh93 and BusyBox ash. So a later bare name
	// runs a program the script's own PATH cannot reach, with nothing in the
	// script saying so. See interp.Semantics.DefaultPathSearchIsRemembered
	// (#2975).
	s.DefaultPathSearchIsRemembered = interp.Yes
	// set -E and -T carry the traps set -Eeuo pipefail scripts rely on.
	s.SetHasTheErrtraceLetter = interp.Yes
	s.SetHasTheFunctraceLetter = interp.Yes
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
	// And the monitor alone is what `fg` and `bg` need: `set -m` in a script
	// with no terminal is granted here, and `fg` then runs the job.
	// Measured 2026-09-15 with and without a pseudo-terminal, and the answer
	// is the same both times (#2720).
	s.MonitorAloneResumesAJob = interp.Yes
	// The start notice is the other half of the same seam, and it answers
	// the other way: with the monitor on and nobody at a prompt, `sleep 1 &`
	// says nothing here. Measured 2026-09-15 on a pseudo-terminal, a script
	// file — the one route the shell that does announce can be asked on
	// (#2838).
	s.MonitorAloneAnnouncesAJob = interp.No
	// A stopped job holds the exit back: the shell says so and stays,
	// and the next attempt leaves. Measured through a pseudo-terminal for
	// `exit` and for ^D alike.
	s.StoppedJobsHoldTheExit = interp.Yes
	// And a `wait` for one gives up rather than waiting on a process that is
	// not going to finish. Measured 2026-09-12 under `set -m`: a bare `wait`
	// warns and reports 0, and a `wait` naming the job or its process id
	// reports 145 — 128 plus SIGSTOP. See the axis for what the rest of the
	// panel does, and for why the monitor is the condition (#2227).
	s.WaitGivesUpOnAStoppedJob = interp.Yes
	// And `kill` reads a signal written onto its option with no space:
	// `kill -n9` and `kill -sKILL` both send. Measured 2026-09-12; bash 3.2
	// refuses both, which is why this is an axis and not the engine (#2227).
	s.KillReadsASignalJoinedToItsOption = interp.Yes
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
	// And there is a parameter to move: `${_+x}` is non-empty before a
	// command has run, which the preset denies because POSIX names no such
	// parameter and dash and BusyBox ash have not got one.
	s.UnderscoreIsAParameterAtAll = interp.Yes
	// And every simple command moves it, wherever that command stands —
	// inside a loop, inside a function body, behind a `;` on one line. The
	// narrowed reading is ksh93's alone.
	s.UnderscoreMovesOnlyBetweenInputCommands = interp.No
	// A function body opens with what the *caller* had rather than with the
	// call's own last argument: `: outer` then `f one two` reads `outer` on
	// the first line of the body here and `two` in zsh. What the caller
	// reads once the call returns is `two` in both, and in ksh93, and is not
	// an axis.
	s.UnderscoreMovesBeforeAFunctionBody = interp.No
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
	// And a fixed depth is what stops it: a chain of sixty distinct names
	// ending in a number is `expression recursion level exceeded` rather
	// than the number. Measured 2026-09-18 (#3416).
	s.ArithRecursionBound = interp.ArithRecursionBoundedByDepth
	// And an unset name found that way is a zero like any other unset name:
	// `x=abc; $((x+1))` is 1 and the script runs on. Measured 2026-09-11 —
	// ksh93 is the panel's holdout, where it is a fatal `parameter not set`.
	s.ArithRecursedNameMustBeSet = interp.No
	s.ArithSubscriptNameMustBeSet = interp.No
	// With nounset *on*, though, every arithmetic read of a name is the
	// option's business: `set -u; : $((b))` is `b: unbound variable` here
	// where the two fields above leave it zero. And it is fatal wherever it
	// is written — this is an expansion failure, and under `set -u` those
	// end the shell — so `(( b ))` and `let "x=b"` stop the script too,
	// where the same two constructs failing for an ordinary reason do not
	// (#3574).
	s.ArithUnsetNameUnderNounsetIsRefused = interp.Yes
	s.ArithNounsetRefusalIsFatal = interp.Yes
	// bash has no floats, so `2**-1` has no integer answer and stops the
	// expression; the two shells with floats answer 0.5 instead.
	s.ArithNegativeExponentIsError = interp.Yes
	// unanswered ArithFloatOverflowIsZero: the same absence of floats. A
	// numeral this shell cannot hold is refused while it is being read —
	// `$((1e400))` is `value too great for base` — so there is no value for
	// an overflow rule to decide, and the question never reaches the axis.
	// unanswered ChainedSubscriptReadsANestedValue: the grammar for a chained
	// subscript is not this shell's — `${a[1][2]}` is a bad substitution or
	// a pattern here — so there is no chain for a reading to be about.
	// Measured 2026-09-15 (#2830).
	s.IndirectionYieldsName = interp.No
	// And an operator written after `${!name[@]}` puts the `!` back to being
	// that indirection: the listing is the bare form only. Measured
	// 2026-09-14 on a script file, under `-c` and on standard input alike —
	// `typeset -A w; w[k]=tgt; tgt=HELLO; echo "${!w[@]#H}"` answers `ELLO`,
	// and `a=(p q); echo "${!a[@]#x}"` says `p q: invalid variable name`,
	// which is the indirection failing on the text it was handed (#2821).
	s.OperatorAfterTheSubscriptListingIsBad = interp.No
	s.BraceExpansion = interp.Yes
	// A group that does not expand does not end the word, and the scan
	// resumes one byte past its open brace rather than past its close, so a
	// list nested inside it is still found: `@{x}{a,b}@` is `@{x}a@ @{x}b@`,
	// `{a{b,c}}` is `{ab} {ac}`, and the unclosed `{a{b,c}` is `{ab} {ac`.
	s.BraceRescanEntersFailedGroup = interp.Yes
	// `{01..3}` is `01 02 03`; `{10..1..3}` is `10 7 4 1` and `{1..10..-3}`
	// climbs anyway — the endpoints decide the direction and a step
	// contributes magnitude alone, so `{3..1..-1}` stays `3 2 1`.
	s.BraceRangePadsToEndpointWidth = interp.Yes
	s.BraceRangeStepSignHonored = interp.No
	s.BraceRangeNegativeStepReverses = interp.No
	// Braces finish before parameters begin, so a range cannot be built
	// from one: `n=3; echo {1..$n}` is the literal `{1..3}`.
	s.BraceRangeEndpointsExpanded = interp.No
	// A character range is letters only — `{1..x}` is the word as written —
	// and it takes a step: `{a..z..2}` is `a c e …`. A step of zero counts as
	// one rather than never arriving, so `{1..2..0}` is `1 2`, and a body
	// that is range-shaped with a gap in it is the word as written.
	s.BraceCharRangeSpansAnyCharacter = interp.No
	s.BraceRangeMissingEndCountsFromZero = interp.No
	s.BraceRangeZeroStepCountsAsOne = interp.Yes
	s.BraceRangeNumberMayCarryAPlus = interp.Yes
	s.BraceRangeThatCannotBeCounted = interp.BraceRangeFailureKeepsTheWord
	s.BracketCaretNegates = interp.Yes
	// One character-class name beyond the twelve POSIX ones. Measured
	// 2026-09-10, `[[ $c = [[:ascii:]] ]]` a character at a time: `a` is in
	// it, `é` and `日` are not, and bash 3.2 answers the same. ksh93 and dash
	// have no such name and match nothing with it, silently, which is what
	// this shell does with any other name outside the roster.
	s.PatternClasses = "ascii"
	// A negative *length* is refused for a list where it is accepted for a
	// string: `${a[@]:1:-1}` and `${@:1:-1}` are `-1: substring expression
	// < 0` at status 1, while `${v:1:-1}` of `abcdef` is `bcde`. The
	// subject decides, not the sign (#1735). bash 3.2 refuses the list form
	// the same way and the string form as well, which is a version
	// difference rather than a dialect's.
	s.ListSliceNegativeLengthIsAnError = interp.Yes
	s.RegexQuotingMakesLiteral = interp.Yes
	// An empty right operand is refused rather than matched: `[[ abc =~ "" ]]`
	// names an empty subexpression and exits 2, where Go's engine would
	// compile it and match at every position. bash 3.2 refuses it too, with
	// the same status and no diagnostic.
	s.EmptyRegexOperandIsAnError = interp.Yes
	// A failed `=~` empties the record rather than leaving the match before
	// last in it, and a group that took no part keeps its number as an empty
	// element — `[[ abcd =~ (b)(z)?(c) ]]` is four elements here (#2916).
	// Both measured 2026-09-18.
	s.RegexMatchSurvivesAFailedMatch = interp.No
	s.RegexMatchOmitsGroupsThatDidNotMatch = interp.No
	// A pattern match writes nothing there. Measured 2026-09-19 on 5.3.20:
	// `[[ abcd =~ (b)(c) ]]` fills BASH_REMATCH and `[[ abcd == a*d ]]`,
	// `case`, a glob and `${v#he}` all leave it exactly as that left it.
	s.PatternMatchWritesTheMatchRecord = interp.No
	// A process substitution may stand as a condition's operand here, and is
	// performed there: `[[ $v == <(cmd) ]]` runs cmd and matches against the
	// path, which is false for anything a script would have written down.
	// This shell alone — zsh refuses the word and ksh93 will not read it.
	s.ProcessSubstitutionInCondition = interp.Yes
	// And a substitution's body reads the input of the command it stands in,
	// which inside a pipeline element is that element's pipe: `printf
	// "PIPE\n" | cat <(cat)` with the shell reading a file of OUTER prints
	// PIPE here, in bash 3.2, and under `sh`. zsh is the one that parts.
	s.ProcessSubstitutionBodyReadsTheShellsInput = interp.No
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
	// And the name is still *recorded*: `typeset xyz; typeset -p xyz` writes
	// `declare -- xyz` at 0 while `${xyz-unset}` fires its default, so the
	// name is declared and unset at once. Measured 2026-09-15 under `env -i
	// PATH=/usr/bin:/bin` on bash 5.3.20 and on the same binary as `sh`,
	// through `-c` and from a file. ksh93 is the column that parts here,
	// which is what makes it a question of its own (#2999).
	s.ValuelessDeclarationRecordsTheName = interp.Yes
	// A table the letters merely declared is not among the names a prefix
	// listing comes to: `declare -A q1; echo "[${!q@}]"` is `[]`, and the
	// same declaration with `=()` behind it is `[q1]`. Measured 2026-09-16 on
	// bash 5.3.20 under `env -i PATH=/usr/bin:/bin LC_ALL=C`, from a file.
	// bash 3.2.57 is the column that parts here — it lists a valueless
	// `declare -a` — so the answer is this build's rather than the family's.
	s.PrefixListingNamesADeclaredOnlyCompound = interp.No
	// An array that exists and holds no elements is unset to the `-`/`+`
	// test: `e=(); echo "[${e[@]+S}]"` is `[]` and `${e[@]-D}` is `[D]`.
	// Stated rather than inherited from the standard's value, because the
	// standard has no arrays and this is a measurement: bash 5.3.20 and
	// bash 3.2.57 both, 2026-09-16, from a file. zsh is the column that
	// parts (#2298).
	s.EmptyArrayIsSet = interp.No
	// And `[[ -v a[@] ]]`, which is not that question asked through another
	// operator: measured 2026-09-18, `${n[@]+S}` is `S` for a one-key table
	// here and `[[ -v n[@] ]]` is false on the same line. The subscript names
	// the array itself — `f=(x)` is set through it, `e=()` is not, following
	// EmptyArrayIsSet above — and a table falls through to the key lookup,
	// which `n[@]=x` then answers yes (#3436).
	s.ConditionWholeArraySubscript = interp.ConditionWholeArraySubscriptNamesTheParameter
	// The **count** of a whole array asks a third question again, and it is
	// the widest of the three: under `set -u`, `${#a[@]}` is refused unless
	// the name holds a list. A scalar is refused — `x=abc; ${#x[@]}` is
	// `x: unbound variable` while `${x[@]}` on the same line is `abc` — and
	// so is a name that was never mentioned; `a=(); ${#a[@]}` is `0`, which
	// is the row that says an assigned-and-empty array is not what this is
	// about. zsh 5.9.2 draws the line one name over (a scalar counts there)
	// and ksh93u+ refuses nothing.
	//
	// And the refusal **gives up the line rather than the shell**, which is
	// the opposite of what the same option does here one construct over:
	// `echo "c=${#a[@]}"; echo SAME` writes the sentence, never runs `SAME`,
	// leaves 1 behind and runs the next line, where `${#a}` on the same
	// unset name ends the shell. Measured 2026-09-18 on bash 5.3.20 and bash
	// 3.2.57 alike, under `env -i HOME=… PATH=/usr/bin:/bin LC_ALL=C`, from a
	// script file with `echo REACHED` on the line after (#3125).
	s.WholeArrayCount = interp.WholeArrayCountRefusesANameHoldingNoArray
	s.WholeArrayCountRefusalAbandonsTheLine = interp.Yes
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
	// And an exported name with no value reaches no child, whatever letters
	// its declaration wrote: `declare -ix Z; env` hands over nothing.
	s.NumericTypeWithNoValueReachesAChildAsZero = interp.No
	// A type letter and an array literal on one declaration is an array of
	// that type: `declare -ia z=(1 2)` is `declare -ai z=([0]="1" [1]="2")`,
	// and `declare -i z=(1 2)` with no array letter is the same line.
	s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = interp.No
	// A container letter and a numeric one stand together too: `declare -ai
	// z` is `declare -ai z`, an array of integers.
	s.NumericAttributeReplacesTheArrayAttribute = interp.No
	// And an array literal over a name that is not an array types its
	// elements rather than re-creating the name, assigned or appended alike:
	// `declare -i a; a=(5+5 6+6)` is `declare -ai a=([0]="10" [1]="12")`.
	s.ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = interp.No
	s.AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = interp.No
	// The letters that say what a name's values are stand together here in
	// both orders: `declare -l z; declare -i z` and the reverse are both
	// `declare -il z="1"`.
	s.NumericAttributeReplacesTheCaseAttribute = interp.No
	// And `-u` beside the same letter records exactly as `-l` does. Measured
	// 2026-09-12, `typeset -ui v=4` lists `declare -iu v="4"` (#2541).
	s.UpperCaseLetterBesideANumericTypeLetterRecordsNothing = interp.No
	// Both case letters on one declaration cancel, and take off a standing
	// one with them. Measured 2026-09-12, `typeset -lu z=Ab` lists `declare
	// -- z="Ab"` with the value unfolded, and `typeset -l z=Ab; typeset -lu
	// z=Cd` lists `declare -- z="Cd"` (#2541).
	s.TwoCaseLettersOnOneDeclarationCancel = interp.Yes
	s.CaseAttributeReplacesTheNumericAttribute = interp.No
	// One reader for both: `$((010))` and `typeset -i d=010` are eight
	// alike, where ksh93 answers eight and ten.
	s.IntegerAssignmentReadsALeadingZeroAsDecimal = interp.No
	// And one reader for a value too: `k=010; $((k))` is eight here.
	s.ArithStoredValueReadsALeadingZeroAsDecimal = interp.No
	// The same octal rule is why `010#5` is not base ten here: the zero opens
	// an octal constant, so the `#` behind it is a byte no numeral can hold
	// and the literal fails — `010#5: invalid number`, where `08#5` is
	// `value too great for base` because the `8` is a digit the base cannot
	// reach. Both readings fall out of this one answer.
	s.ArithBaseMayHaveALeadingZero = interp.No
	s.ArithBaseIsAtMostTwoDigits = interp.No
	s.ArithBaseZeroReadsTheDigitsAsWritten = interp.No
	// A radix prefix with nothing after it is a finished number worth zero:
	// `$(( 0x ))` is 0 and `$(( 0x+1 ))` is 1, in 5.3 and 3.2 alike.
	s.ArithEmptyRadixDigitsAreZero = interp.Yes
	// A numeral past the word goes round it. `$(( 10000000000000000000 ))`
	// is -8446744073709551616 and `$(( 18446744073709551616 ))` is 0 — the
	// second being the row that says this is modular and not a clamp at the
	// top of the unsigned word. Measured identically in 5.3.20, 3.2.57 and
	// under the `sh` name (#3202).
	s.ArithNumeralPastTheWord = interp.NumeralPastTheWordWraps
	// And the same numeral out of a variable reads identically; only dash
	// parts the two.
	s.ArithStoredNumeralPastTheWordIsRefused = interp.No
	// And for `let`: `let "x=010"` is eight, the same as `(( ))`.
	s.LetReadsALeadingZeroAsDecimal = interp.No
	// An arithmetic assignment leaves an ordinary scalar: `(( x = 5 ));
	// declare -p x` is `declare -- x="5"`, and a later `x=2+3` is the three
	// characters.
	s.ArithmeticAssignmentDeclaresAnInteger = interp.No
	// An attribute added to a name that already holds a value waits for the
	// next assignment: `FOO=bar; typeset -i FOO` still reads `bar`, and
	// `d=MiXeD; typeset -u d` still reads `MiXeD`. ksh93 and zsh re-read on
	// the spot and store 0 and MIXED.
	s.AttributeRereadsTheValueItFinds = interp.No
	s.InheritedValueSurvivesADeclaredType = interp.Yes
	s.CompoundElementsGoThroughTheAttribute = interp.Yes
	// The case attributes fold once, on the value being stored: `declare -l
	// v=AB` holds `ab`, and taking the letter off leaves `ab` behind.
	s.CaseAttributeFoldsWhenRead = interp.No
	s.CompoundAttribute = interp.CompoundAttributeKeepsTheElements
	// The converse: an array or table letter given to a name already holding
	// a scalar promotes that value to the first element. Measured 2026-09-08,
	// `b=1; typeset -a b` lists as `declare -a b=([0]="1")` and `a=1;
	// typeset -A a` as `declare -A a=([0]="1" )`, and `$b` still reads `1`
	// under both. bash 3.2.57 answers the array letter the same way and has
	// no `-A` at all.
	s.ScalarUnderAnArrayDeclaration = interp.ScalarUnderACompoundBecomesTheFirstElement
	s.ScalarUnderATableDeclaration = interp.ScalarUnderACompoundBecomesTheFirstElement
	// A name holding an array or a table reaches no child at all: measured
	// 2026-09-12, `typeset -x a=(p q)` and `a=(p q); export a` both leave
	// nothing named `a` in a child's environment, and so does an exported
	// table. `export b=1` beside it is the control and does arrive, so it is
	// the compound and not the export (#1380).
	s.ExportedCompoundReachesAChildAsItsFirstValue = interp.No
	// And a subscripted operand's letters land on the name: `typeset -x
	// a[1]=v` lists `declare -ax a=([1]="v")` here (#1380).
	s.SubscriptedOperandCarriesTheAttributes = interp.Yes
	s.StoreRefusalOfADeclaredElementLeavesZero = interp.No
	// The refusal is not fatal here at all, so the builtin's own status is
	// what a script reads and there is no give-up whose number could move.
	s.StoreRefusalThroughPrintfLeavesZero = interp.No
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
	// And POSIX mode moves both of those: measured 2026-09-18 from a script
	// file with `set -o posix; readonly v=1` in front of it, `v=3 true;
	// echo pre=$?` writes the complaint, never the `pre=`, and reports 1 on
	// the next line, while `v=3 :` ends the script at 1. See
	// Semantics.PosixModeSharpensAPrefixRefusal.
	s.PosixModeSharpensAPrefixRefusal = interp.Yes
	// A failed expansion gives up the line here and the shell carries on at
	// the next one, which is this shell alone among the four. Measured over
	// both routes and both separators — see the axis for the 2x2 — on a bad
	// substitution, a division by zero, a bad subscript and an arithmetic
	// expression the parser refused.
	s.FailedExpansionAbandonsTheLine = interp.Yes
	// And the command that named a `>(cmd)` does not wait for its body: the
	// body is a process holding this shell's standard output, so its bytes
	// land afterwards rather than before. Measured as an ordering — `printf
	// "PIPE\n" | tee >(read -r v; sleep 0.3; printf "[%s]" "$v") >/dev/null;
	// printf AFTER` is `AFTER[PIPE]` here and `[PIPE]AFTER` in zsh (#2197).
	s.WritingSubstitutionIsWaitedForAtTheCommand = interp.No
	s.ReadonlyReassignmentByDeclarationFatal = interp.No
	// And `export x=2` and `readonly x=2` carry on too, until `set -o posix`
	// moves this one — see interp.Semantics.ReadonlyReassignmentBySpecialBuiltinFatal.
	s.ReadonlyReassignmentBySpecialBuiltinFatal = interp.No
	// A frozen name refuses a declaration's array literal here as much as it
	// refuses anything else: measured in 5.3 and in 3.2, `readonly q=1;
	// declare -g q=(b)` is `q: readonly variable` and the name is untouched,
	// where zsh retypes it. Both builds, so it is not a version's answer.
	s.ArrayLiteralOperandRetypesAFrozenScalar = interp.No
	// And the letter half of the same rule is refused too. Measured
	// 2026-09-12 under `env -i`, `readonly q=1; typeset -i q=4` is
	// `typeset: q: readonly variable` in both builds and the name is left at
	// 1; `-F` is not even a float here — it is the function-listing letter,
	// and the complaint is about `-f` — and `export -i` is an invalid
	// option. So no spelling of the numeric type letter reaches a frozen
	// name (#2539).
	s.NumericTypeLetterRetypesAFrozenName = interp.No
	// And the wider rule the same probe found on its valueless half: a
	// declaration naming a value-shaping letter over a frozen name is
	// refused whole here, so nothing of it lands. Measured 2026-09-13 under
	// `env -i`, after a `readonly q=1`: `typeset -i q`, `typeset -u q` and
	// `typeset -a q` are each `typeset: q: readonly variable` at 1 with the
	// listing still `declare -r q="1"`, where `typeset -x q`, `typeset -t
	// q`, `typeset -r q` and a bare `typeset q` are all taken. **bash 3.2
	// is the other way on that half** and takes every one of them, which is
	// why this preset is bash 5.3's answer and the record carries 3.2's in
	// its own column (#2561).
	s.AttributeOverAFrozenNameIsRefused = interp.Yes
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
	// And neither is a missing operand: `.` and `source` with none write
	// `filename argument required` and the builtin's usage, leave 2 behind,
	// and the script runs on. Measured 2026-09-18 on bash 5.3.20.
	s.DotWithNoOperandIsFatal = interp.No
	// POSIX mode moves it, and the invocation name is only a door into that
	// mode: measured 2026-09-19, `set -o posix` in front of the bare `.`
	// ends plain bash's script at 2 exactly as this binary called `sh` does,
	// and the control — `false` on the line before — runs on under both. So
	// the departure is the mode's and not argv[0]'s (#3818).
	s.DotWithNoOperandIsFatalInPosixMode = interp.Yes
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
	// `. -p list file`, which is a 5.x addition: bash 3.2 calls the letter
	// an invalid option and prints a usage line without it.
	s.DotReadsOptions = interp.Yes
	s.DotTakesTheSearchPathOption = interp.Yes
	// `. -p list file`, which is a 5.x addition: bash 3.2 calls the letter
	// an invalid option and prints a usage line without it.
	s.DotReadsOptions = interp.Yes
	s.DotTakesTheSearchPathOption = interp.Yes
	// `eval` reads a leading dash-word the way every other builtin here
	// does: `eval -- cmd` runs `cmd`, and `eval -q cmd` is `eval: -q:
	// invalid option` with the usage line, at 2. There are no letters to
	// know, so the refusal is the whole of what the reading does.
	s.EvalOptions = interp.EvalReadsOptions
	// See interp.Semantics.BuiltinReadsOptions.
	s.BuiltinReadsOptions = interp.Yes
	s.ExecFailureRunsExitTrap = interp.Yes
	s.ExecTakesOptions = interp.Yes
	// Both letters, and `-l` reaches the name `-a` chose: `exec -l -a NAME`
	// hands the replacement `-NAME`, where zsh hands it `NAME`.
	s.ExecTakesTheLoginLetter = interp.Yes
	s.ExecTakesTheEmptyEnvironmentLetter = interp.Yes
	s.ExecLoginPrefixesTheGivenName = interp.Yes
	s.TestAcceptsDoubleEqual = interp.Yes
	// The operators past the three-word rules. `test -a f` is `-e`'s question
	// where two words say the letter is an operator rather than the
	// connective, `test -o errexit` asks whether a `set -o` name is on, and
	// `test -N f` asks whether the file was written since it was read.
	// `umask -p` prints the mask as a command that would set it again, and
	// `set -p` is the short spelling of `set -o privileged`.
	s.UmaskHasTheReusableLetter = interp.Yes
	s.SetHasThePrivilegedLetter = interp.Yes
	s.TestHasTheFileExistsLetter = interp.Yes
	s.TestHasTheShellOptionOperator = interp.Yes
	s.TestHasTheModifiedSinceReadOperator = interp.Yes
	// And both ordering operators, which is the answer POSIX's XSI option
	// gives — where ksh93 has only the greater one.
	s.TestStringOrder = interp.TestStringOrderBoth
	// `f -nt missing` holds when f exists, in `test` and `[[ ]]` alike.
	s.MissingFileIsOlder = interp.Yes
	s.UmaskPrintsFourDigits = interp.Yes
	s.UmaskSetWithSPrints = interp.Yes
	s.PipefailOption = interp.Yes
	// A substituted element keeps the status its death produced, 128 plus
	// the signal, the same as anywhere else.
	s.PipefailSubstitutesTheBareSignal = interp.No
	s.ErrexitSeesPipefailFailure = interp.Yes
	// `time` changes nothing about the judging here: measured 2026-09-18,
	// `set -e; time false; echo survived` stops and `trap 'printf E' ERR;
	// time false` writes E, for a simple command, a pipeline, a group, a
	// function call and a loop alike.
	s.TimedCommandIsJudged = interp.Yes
	// And a redirection that cannot be opened on a compound command is a
	// failure both judges see, exactly as it is on a simple one.
	s.CompoundRedirectionFailureIsJudged = interp.Yes
	// A subshell written as the last element of a pipeline is judged twice:
	// once inside the element's own process, where the trap is still set, and
	// once for the pipeline. Measured 2026-09-18, `trap 'printf E' ERR; true
	// | ( exit 3 )` writes `EE` — and that element has no failing command in
	// it, so the second E is the subshell command itself and not its body. A
	// group holding the same subshell writes one E, a function whose body is
	// one writes one, and an element that is not the last does not fire at
	// all.
	s.ASubshellAsTheLastPipelineElementJudgesItself = interp.Yes
	// `[ ( -n x ) ]` is 0: the group is read like any other (#3419).
	s.TestGroupedUnaryAloneLosesTheClosingParen = interp.No
	// A four-word `!` in front of a negation negates it, the way a
	// recursive reading predicts: `[ ! ! -n x ]` is 0 here and 1 in the one
	// column that drops the outer negation. Measured 2026-09-19 (#3700).
	s.TestFourWordsNegateANegationOnce = interp.No
	// And at three words the connective is read before a leading `!`. The
	// probe that says so names a file that **exists**, because this shell
	// has a unary `-a`: `[ -a / ]` is 0, so negating it first would give 1,
	// and `[ ! -a / ]` is 0 — the both-set guard over the two non-empty
	// strings. `[ ! -a x ]` answers 0 under either reading and pins nothing.
	// dash and BusyBox ash refuse both, and ksh93u+ answers 1. Measured
	// 2026-09-19 (#3717).
	s.TestThreeWordsNegateBeforeAConnective = interp.No
	s.TestFailureInsideAnUnclosedGroupIsTheParen = interp.No
	// And a group with nothing in it is refused rather than false:
	// `[ ( ) ]` is `[: (: unary operator expected` at 2 (#3687).
	s.TestEmptyGroupIsFalse = interp.No
	// And `command local a=1` declares the local, which is what makes the
	// two shells that drop it a split rather than a rule (#3370).
	s.LocalThroughCommandDeclaresNothing = interp.No
	s.UnterminatedBracket = interp.BracketLiteral
	// And the same question where a `[:name:]`, a `[.x.]` or a `[=x=]`
	// inside it is what left it open: a literal `[`, and the rest of the pattern behind it — `[[:alpha:]` takes `[` plus one of `:alpha`'s five characters, which is the same reading the axis above gives a bare `[`.
	s.UnterminatedBracketAfterASubExpression = interp.BracketLiteral
	// And inside a bracket expression the backslash protects the character
	// behind it and puts nothing of its own in the set: `[\)]` is the
	// one-character set `)`, and `[a\-z]` is the three members a, `-` and z,
	// the escape being what stops the dash reading as the range operator.
	// zsh adds the backslash to the set as well and BusyBox ash protects
	// nothing, so this value is what five of the seven columns share (#3271).
	s.BracketEscape = interp.BracketEscapeProtectsTheMember
	s.UnknownCharacterClass = interp.UnknownClassIsInert
	// `[[.a.]]` is the collating element `a` and `[[=a=]]` its equivalence
	// class, so both match `a`; zsh is the one column where they are the
	// ordinary characters they spell, and BusyBox ash the one that reads the
	// delimiters and finds an element in no body at all.
	//
	// A body of more than one character is a **name** from the portable
	// character set here and nowhere else in the panel: measured 2026-09-18
	// under `LC_ALL=C`, `[[.hyphen.]]` matches `-`, `[[.period.]]` matches
	// `.` and `[[=space=]]` matches a space, where ksh93 and dash match none
	// of the three. A name outside the roster is a body that is not an
	// element like any other, and the axis above says what that does to the
	// bracket — here, nothing: `[a[.nosuch.]b]` matches a and b and no
	// letter of the body (#3378).
	s.CollatingElements = interp.ACollatingElementMayBeNamed
	// `[[:]` is a bracket holding `[` and `:`: nothing closes the name, so there is no name and the two characters are ordinary members.
	// See interp.Semantics.UnterminatedCharacterClass (#1431).
	s.UnterminatedCharacterClass = interp.UnterminatedClassIsOrdinaryCharacters
	// A backslash that arrived in a value takes the metacharacter status off
	// what follows it and stays in the text itself: measured 2026-09-12 in a
	// directory holding `a\b` and `a*`, `v='a\*'; set -- $v` is the word
	// `a\*` in 5.3.15, in 3.2.57 and as `sh` — neither name matched, so the
	// `*` was not live and the backslash was not removed. ksh93 is the one
	// column that reads it the other way (#1367).
	s.ValueBackslashInAPattern = interp.ValueBackslashQuotesWhatFollows
	// An escaped IFS whitespace character closing a `read` value is trimmed
	// off a value that took the *remainder* of the line and left alone on a
	// value that was its own field. Measured 2026-09-12 on 5.3.15, 3.2.57
	// and as `sh`: `printf 'a b c\\ \n' | read x y` leaves `b c` where
	// `printf 'a b\\ c\\ \n' | read x y` leaves `b c ` — the escaped space
	// in the middle joins the two words into one field, so there is one
	// field per name and no remainder to trim (#1360).
	s.ReadTrailingEscapedSeparator = interp.ReadTrailingEscapedSeparatorTrimmedFromARemainder
	// A longest match over an alternation takes the longest arm, whichever
	// order the arms were written in. Measured 2026-09-11 with extended
	// patterns on, `x=abc`: `${x##@(a|ab)}` and `${x##@(ab|a)}` are both `c`
	// in 5.3.15 and in 3.2.57, and 2026-09-12 for the substitution the axis
	// also governs — `${x//@(a|ab)/X}` and `${x//@(ab|a)/X}` are both `Xc`.
	s.LongestMatchTakesTheWrittenArm = interp.No
	// An empty pattern is declined outright, whatever the value: measured
	// 2026-09-12 on 5.3.15, on 3.2.57 and as `sh`, `v=abc` and `e=`,
	// `${v///X}` and `${v/$e/X}` are both `abc`, and `${e///X}` is empty
	// where the control `${e//*/X}` is `X`. Not a rule about empty matches —
	// with extglob on, `${v//@(|)/<>}` is `<>a<>b<>c` in the same build.
	s.EmptyReplacementPattern = interp.EmptyReplacementPatternMatchesNothing
	// `#` and `%` after the `/` are anchors here, and an anchored pattern
	// that is empty still matches at that end: `v=abcabc` gives `Xbcabc`,
	// `abcabX`, `Xabcabc` and `abcabcX` for `${v/#a/X}`, `${v/%c/X}`,
	// `${v/#/X}` and `${v/%/X}`. ksh93 takes the anchor and declines the
	// empty pattern behind it, and BusyBox ash has no anchors at all
	// (#3272).
	s.ReplacementAnchors = interp.Yes
	s.AnchoredEmptyReplacementPattern = interp.Yes
	// And the anchor is read after a single `/` only: after the global `//`
	// the character is the pattern's own first byte. Measured 2026-09-16 —
	// `${v//#a/X}` on `abcabc` is `abcabc` and, over a value holding the
	// character, `${w//#a/Q}` on `x#ay%bz` is `xQy%bz`, the `#a` found
	// inside. bash 3.2.57 and ksh93 agree; zsh is the one column that reads
	// an anchor there (#3307).
	s.GlobalReplacementAnchors = interp.No
	// And the empty match a global replacement refuses is the one at the end
	// of the value: `${v//@(b|)/<>}` is `<>a<><>c`, which takes the empty
	// match sitting where `b` ended and leaves the end alone.
	s.ReplacementEmptyMatchDeclined = interp.EmptyMatchDeclinedAtTheEnd
	s.StatusArgument = interp.StatusArgNumeric
	s.TraceAssignmentsSeparately = interp.Yes
	// A traced array literal is the words the script wrote, not what they
	// came to: `x="p q"; a=("$x" r)` is `a=("$x" r)` here, and the ordering
	// says so too — `a=($(echo x))` is traced and *then* the substitution
	// runs. Both builds agree.
	s.TraceArrayLiteralShowsTheExpandedElements = interp.No
	// And a subscripted literal is one line here too, the words as written.
	s.TraceSubscriptedArrayLiteralIsElementAssignments = interp.No
	// And the subscript is the text as written: `i=2; a[$i]=v` is
	// `a[$i]=v`, and so is `a[i]=v`.
	s.TraceElementSubscriptIsEvaluated = interp.No
	// bash reports success if it signaled anything at all, where the others
	// count failures one way or another.
	s.ExitTrapRunsOnSignalDeath = interp.Yes
	// 5.3 ignores an untrapped QUIT when it is not interactive, where 3.2
	// dies by it — a divergence between two builds of the same shell, and
	// this preset is 5.3.
	s.QuitIgnoredWhenNotInteractive = interp.Yes
	// And `trap - QUIT` does not take that ignore away: measured, `trap -
	// QUIT; kill -QUIT $$; echo after` prints after and exits 0, with or
	// without a handler having been installed and removed first.
	s.QuitResetRestoresTheDefault = interp.No
	s.HangupIsAnOrderlyExit = interp.No
	s.ExitInTrapReportsEarlierStatus = interp.Yes
	// A subshell's own EXIT trap runs however the subshell ended. Measured
	// 2026-09-18 over the nine rows of interp.SubshellExitTrapPolicy, each
	// plain and under `set -e` (#3612).
	s.SubshellExitTrapAfterAGiveUp = interp.SubshellExitTrapAlwaysRuns
	// `kill -n signum` is an option here, and `-s` with nothing after it is
	// an option missing its argument rather than a signal named `s`.
	s.KillReadsTheNumberOption = interp.Yes
	s.KillOptionWithNoArgumentIsASignalName = interp.No
	s.KillListAcceptsName = interp.Yes
	// `kill -l 129` is HUP and `kill -l 0` is EXIT; one subtraction, and a
	// number that still names nothing is refused.
	s.KillListReducesRepeatedly = interp.No
	s.KillListPrintsANumberItCannotName = interp.No
	s.KillListNamesZeroAsExit = interp.Yes
	// The one column that writes an empty line for a signal it has no name
	// for, rather than the number. Measured 2026-09-17 on Linux, where the
	// question arises: `kill -l 32` is a blank line at 0 here and `32` in
	// dash, zsh and BusyBox ash (#3287).
	s.KillListLeavesAnUnnamedSignalBlank = interp.Yes
	s.SIGPrefixAccepted = interp.Yes
	s.RedirectsUseEveryTarget = interp.No
	s.KillStatus = interp.KillStatusAnySuccess
	s.SubshellJobTable = interp.SubshellJobsKeptOutsideACompound
	s.PrintfEmptyIsNotANumber = interp.Yes
	s.PrintfAbsentNumberIsAnEmptyOne = interp.No
	s.PrintfStarWithoutOperandIsRefused = interp.No
	s.PrintfStarComplaintCostsTheStatus = interp.Yes
	// C's own reading, which is C's library: `%G` of an infinity is `INF`
	// and `%10f` of one pads to ten.
	s.PrintfNonFiniteIsConverted = interp.Yes
	// The `'` flag, among the flags and nowhere else: `%'d` groups and
	// `%15'd` is `` `'': invalid format character ``.
	s.PrintfGroupingFlag = interp.Yes
	s.PrintfGroupingFlagAfterTheWidth = interp.No
	// A `*` beside a width's own digits is refused here too: `printf '%5*d' 4 42`
	// is a conversion character this shell does not have (#2824).
	s.PrintfStarBesideTheFieldDigits = interp.No

	// Refused at INT_MAX itself, where ash gives up one past it:
	// `printf '[%2147483647s]' x` is the complaint here and two billion
	// characters of padding there. Measured 2026-09-15.
	s.PrintfFieldBeyondAnInt = interp.PrintfFieldRefused

	// The complaint is the number's range rather than the field's, and the
	// field is still laid out: `printf 'A[%*s]B' 21474836470 x` is `A[x]B`
	// at 1 after `printf: 21474836470: Result too large`. Measured
	// 2026-09-15, and the other half of PrintfFieldBeyondAnInt above.
	s.PrintfStarBeyondAnInt = interp.PrintfStarIsOutOfRange
	s.CaseSubjectKeepsThePreviousLine = interp.No
	s.SubstringRangeThirdColonIsABadSubstitution = interp.No
	s.PrintfReportsBadNumber = interp.Yes
	s.PrintfNumberOperand = interp.PrintfNumberLeadingNumber
	// The digits are written back exactly: `printf '%d' 123456789012345678`
	// is the operand, not a double's nearest neighbor (#2907).
	// A flag past a field is no flag at all: the prefix ends there and the
	// byte arrives at the scan as the conversion character (#2910).
	// unanswered ArithDivisionByZeroYieldsAValue: the value a division by
	// zero leaves behind is only visible through a `printf` operand, and
	// this shell's printf evaluates no operand -- so the question has no
	// site here. Every other expression abandons the command, here as in
	// the two columns that do evaluate.
	s.PrintfFlagAfterTheField = interp.No
	s.PrintfIntegerOperandGoesThroughTheFloatingType = interp.No
	// C99's three: `%F` is `1.500000`, `%a` is `0x1.8p+0` and `%A` is
	// `0X1.8P+0`, all measured 2026-09-14 on bash 5.3.15 under `LC_ALL=C`.
	s.PrintfC99FloatConversions = interp.Yes
	// And the shortest run of digits that names the value, which is C's
	// default: `printf '%a' 0.1` is `0x1.999999999999ap-4` here.
	s.PrintfHexFloatDefaultIsTwelveDigits = interp.No
	// And a `%a`'s zero fill goes between the `0x` and the digits, which is
	// where C puts it.
	s.PrintfHexFloatZeroFillPrecedesThePrefix = interp.No
	// unanswered PrintfRefusedOperandKeepsItsLeadingNumber: the question is
	// what survives an *arithmetic* failure, and this shell runs no
	// arithmetic over a printf operand. Keeping the number at the front is
	// its whole reading and is PrintfNumberLeadingNumber above, so there is
	// nothing left here to decide. The same goes for
	// PrintfFloatOperandIsEvaluatedTwice: an evaluation that never happens
	// cannot happen twice.
	s.PrintfBackslashC = interp.PrintfBackslashCLiteral
	// A format that ends inside a conversion is an error here, with a
	// second wording of its own — see PrintfMissingVerb.
	s.PrintfUnfinishedConversionIsAPercent = interp.No
	// `\x41` is an `A`, and at most two digits: `\x0ff` is 0x0f then an
	// `f`. A `\x` with no digit after it stands as written, with a warning
	// on standard error and a status that is still zero.
	// And it says so when the digit run is empty, which is the half a
	// dialect answers separately from the wording: `printf 'a\xZb'` writes
	// the complaint, the two characters and a zero.
	s.PrintfReportsAMissingHexDigit = interp.Yes
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
	// Both spellings of the escape character in a *format*, in every build
	// of this shell: `printf 'a\eZ'` and `printf 'a\EZ'` are both
	// `61 1b 5a` in 5.3.20, in 3.2.57 and under argv[0] `sh` (#3225). The
	// `%b` site answers the same way, which is what makes this the one
	// column where the two sites cannot tell each other apart.
	s.PrintfEscEscape = interp.Yes
	s.PrintfCapitalEscEscape = interp.Yes
	// An escape a format does not define keeps its backslash — `printf
	// '[\q][\z][\8][\-]'` is `[\q][\z][\8][\-]` — and a floating
	// conversion's exact half goes to the even neighbor: `printf '%.0f
	// %.0f %.0f' 2.5 4.5 -2.5` is `2 4 -2` and `%.2f` of 0.125 is `0.12`.
	// Measured 2026-09-18 under `LC_ALL=C` from a script file; ksh93 is the
	// one column on the other side of both.
	s.PrintfUnknownEscapeDropsTheBackslash = interp.No
	s.PrintfFloatHalf = interp.PrintfFloatHalfToEven
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
	// A `%s` field is bytes, and an `l` on `%s` or `%c` makes it characters
	// where the locale has them: `printf '[%.2s|%.2ls|%lc]' αβγ αβγ αβγ` is
	// `[α|αβ|α]` under a UTF-8 locale, bash 5.3.20, 2026-09-16. bash 3.2.57
	// has no wide reading and writes `[α|α|` and the byte 0xce, so the `l`
	// answer is this build's rather than the family's (#2298).
	s.PrintfFieldCountsCharacters = interp.No
	s.PrintfLongModifierCountsCharacters = interp.Yes
	// `%(fmt)T`: an epoch through a date format, with -1 for now and -2 for
	// when the shell started. This shell alone in the panel — 3.2 has it not
	// either, which is why the two bash columns of the corpus differ here.
	s.PidListingFinishesWithAJob = interp.No
	s.PrintfTimeConversion = interp.Yes
	s.PrintfTimeOperandIsADateString = interp.No
	s.PrintfQuote = interp.PrintfQuoteAnsiCWord
	// C's `#` at a value of nought: `printf '%#x' 0` is `0` and
	// `printf '%#.0o' 0` is `0`, bash 5.3.20 and bash 3.2.57 alike.
	s.PrintfAlternateFormAsksTheValue = interp.Yes
	// And it counts that prefix against the width a `0` flag fills:
	// `printf '%#05x' 7` is `0x007` and is five characters, bash 5.3.20
	// and bash 3.2.57 alike.
	s.PrintfZeroFillCountsTheAlternatePrefix = interp.Yes
	// `$'\cA'` is 0x01 and `$'\c1'` is 0x11: the character uppercased and
	// masked to five bits, with `\c?` reading as DEL since 5.x.
	s.DollarSingleBackslashC = interp.DollarSingleControlMasked
	s.DollarSingleUnknownEscape = interp.DollarSingleUnknownKeepsBackslash
	s.DollarSingleNul = interp.DollarSingleNulEndsTheSpan
	// Two digits after `\x`, and an escape with no digit at all stays the
	// two characters it was written as: `$'\xzz'` is `\xzz` here.
	s.DollarSingleHexReadsEveryDigit = interp.No
	s.DollarSingleDigitlessEscapeIsAZeroByte = interp.No
	// An octal escape past 255 keeps the low byte: `$'\401'` is 01 and
	// `$'\777'` is ff. Measured 2026-09-18 by `od` (#3415).
	s.DollarSingleOctalPastAByteDropsTheLastDigit = interp.No
	// And neither `\C` nor `\M` is an escape here: `$'\C-A'` and `$'\M-x'`
	// are kept as written, where zsh reads them as one byte apiece and ksh93
	// reads a different escape out of the same spelling (#2345).
	s.DollarSingleCaretMeta = interp.DollarSingleCaretMetaAbsent
	// `$'\e'` and `$'\E'` are both the escape character, `$'\?'` is the
	// question mark alone, and both Unicode spellings are read as a code
	// point: $'\u0041' and $'\U00000041' are `41`. The last is a bash 4 feature
	// and the 3.2 macOS ships keeps the characters as written, which is a
	// version this dialect does not model — the preset is 5.3 (#3270).
	s.DollarSingleEscEscape = interp.Yes
	s.DollarSingleQuestionEscape = interp.Yes
	s.DollarSingleUnicodeEscapes = interp.Yes
	s.GetoptsAssignmentRestartsWord = interp.Yes
	// `kill %1` reaches the job's process. dash aims at the group.
	s.KillJobSpecAimsAtTheGroup = interp.No
	// bash 5.3.20 sends it: `kill -0 -- -1` is 0.
	s.KillRefusesTheAllProcessesTarget = interp.No
	// bash 5.3.20 and 3.2.57 both store it: `typeset a=(VAL) 2>/nope/x`
	// leaves `a` holding VAL. A scalar operand is not stored, which is
	// what keeps this to the array literal.
	s.ArrayOperandIsStoredPastAFailedOpen = interp.Yes
	// bash 5.3.20 reaches the target behind either spelling.
	s.KillTakesEndOfOptionsAfterTheSignal = interp.Yes
	// A trim on `$@` runs over each field. dash and BusyBox ash run it
	// over the joined list once.
	s.OperatorDistributesOverTheFieldList = interp.Yes
	// OPTIND names the word until its last letter has been read: `-abc`
	// reads `a` with OPTIND still 1. dash and BusyBox ash count it at the
	// first letter instead.
	s.GetoptsCountsTheWordAtItsFirstLetter = interp.No
	// And it does not wait for the next call to count it either: the word is
	// counted at its last letter, which is POSIX's own wording and four of
	// the seven columns. The run that reports "no more options" writes `?`
	// into the name (#3275).
	s.GetoptsCountsTheWordOnTheNextCall = interp.No
	s.GetoptsEndOfOptionsNamesIt = interp.Yes
	// A `+`-prefixed word is an operand and ends the scan: measured
	// 2026-09-17 on 5.3.20, on 3.2.57 and under the name `sh`, `getopts a o
	// +a` is 1 with the name `?`.
	s.GetoptsTakesAPlusPrefixedOption = interp.No
	// `getopts 'n#' o` is two options here, neither of which takes anything:
	// `-n 5` leaves the 5 as an operand and `-n5` reports 5 as unknown.
	// Measured 2026-09-16, bash 5.3.20 (#2947).
	s.GetoptsOptionStringHasANumericType = interp.No
	// OPTERR is this shell's alone, and setting it to zero turns the
	// `getopts` diagnostic off without moving to the silent form — which is
	// the only spelling for "keep `?` and lose the noise", the leading colon
	// changing the name and OPTARG as well. Measured 2026-09-17 on 5.3.20, on
	// 3.2.57 and under the name `sh`: a bad option and a missing argument are
	// each one line on stderr at OPTERR=1 and none at OPTERR=0, with the name,
	// the status and OPTARG the same either way.
	s.GetoptsOptErrSilencesTheComplaint = interp.Yes
	s.GetoptsClearsOptarg = interp.No
	s.GetoptsEmptiesOptargForAnArgumentlessOption = interp.No
	// A freeze on OPTARG or OPTIND is consulted here — `readonly OPTARG;
	// getopts a: o` writes `OPTARG: readonly variable` — and costs the
	// builtin nothing: the name is still filled in and the status is still
	// 0. Both measured 2026-09-16 on 5.3.20 and 3.2.57 alike.
	// The call that reports "no more options" unsets OPTARG, so the last
	// option's argument is not left standing for a script to read after its
	// loop.
	s.GetoptsUnsetsOptargAtEndOfOptions = interp.Yes
	// And that clearing — like the ones a bad option and a missing argument
	// make, and unlike the one an argument-less option makes — takes the name
	// away rather than writing over it, so a `readonly OPTARG` neither stops
	// it nor survives it. bash alone; measured 2026-09-16 on 5.3.20, on
	// 3.2.57 and under the name `sh`.
	s.GetoptsClearingOptargIsARealUnset = interp.Yes
	s.GetoptsOwnParametersIgnoreAFreeze = interp.No
	s.GetoptsRefusedWriteEndsTheBuiltin = interp.No
	// A freeze on the **name** is reported and the script runs on, wherever
	// the scan was: measured 2026-09-17 on 5.3.20 and 3.2.57, `readonly N`
	// over `set -- -a v` and over `set -- x` alike.
	s.GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal = interp.No
	// `read` stops at the first name a freeze refuses and leaves the names
	// after it alone, which is the other answer from `getopts` above and is
	// why the two are separate axes: `readonly a; printf 'x y\n' | { read a
	// b; }` leaves b untouched here and fills it in ksh93.
	s.ReadRefusedWriteEndsTheBuiltin = interp.Yes
	// And the status parts "I stopped early" from "a write failed": 2 where a
	// name was left to fill and 1 where the frozen name was the last one.
	// Measured over three names in 5.3.20, 2026-09-16 (#3208).
	s.ReadRefusedWriteIsOneOnTheLastName = interp.Yes
	// And the refusal is not the end of the script: `getopts a: o; echo
	// reached` prints `reached`, where a plain `x=2` over a frozen `x` gives
	// up the rest of the line in every shell here.
	s.ReadonlyRefusalInABuiltinIsFatal = interp.No
	// The scan position is shared with the caller across a shell function
	// call, which is why every option-parsing helper here begins with
	// `local OPTIND=1`: without it a second call starts where the first
	// stopped and reads nothing.
	s.GetoptsFunctionPosition = interp.GetoptsFunctionPositionIsShared
	// A `local OPTIND` gives the call the whole cursor and gives the caller
	// the whole cursor back — the position inside a clustered word along
	// with the number. bash 3.2 restores only the number, so a loop whose
	// body calls a function that declares one never finishes there; this is
	// the newer answer and the one `getopts ab o; g; getopts ab o` reads `b`
	// from (#2226).
	s.GetoptsLocalOptindRestoresTheCursor = interp.Yes
	// Measured 2026-09-12: `OLDPWD=/nonexistent bash -c 'echo ${OLDPWD-UNSET}'`
	// answers UNSET, and the same with a directory answers the directory. A
	// plain file is dropped and a mode-000 directory is kept, so the test is a
	// stat rather than the one `cd` makes. bash 3.2 drops an inherited OLDPWD
	// whatever it names, which is a fourth answer and not this dialect's.
	s.InheritedOldpwd = interp.InheritedOldpwdTakenIfADirectory
	// The depth, counted and told to every child — and the one column of the
	// panel with a ceiling on it: a level of 1000 or more is refused with a
	// warning and the count starts again. See interp.ShellLevelPolicy.
	s.ShellLevel = interp.ShellLevelCountedToACeiling
	// And a shell that replaces this process stands in its place rather than
	// under it: measured 2026-09-18, `exec /usr/bin/env` from a script file
	// under `env -i` hands over `SHLVL=0` where this shell holds 1, so the
	// shell it starts reads the same number this one did. The floor above is
	// what makes an inherited `-1` read 1 on the far side rather than 0.
	s.ShellLevelExec = interp.ShellLevelExecNotCounted
	// `PWD` is a different answer from OLDPWD's here: the starting directory
	// is named by what the kernel reports, in 5.3 and 3.2 alike.
	s.StartupPwdName = interp.StartupPwdNameFromTheKernel
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
	// A number this kernel delivers that no name in the table covers is a
	// condition here. Measured 2026-09-19 on linux/arm64, bash 5.2.15 in
	// Debian bookworm, the script file `trap 'echo CAUGHT' $1; kill -$1 $$;
	// echo SURVIVED`: 15, 40 and 64 all print CAUGHT then SURVIVED at 0.
	s.TrapTakesASignalNumberItCannotName = interp.Yes
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
	// And the command that *ran* a failure fires the condition again, but
	// only where a trap was set before that command began. Both halves are
	// measured, from a script file with the action printing $LINENO: with
	// the trap set at the top and `set -E` carrying it in, `g(){ false; }`
	// fires at the body's line and again at the call's; with the trap set
	// for the first time *inside* g it fires at the body's line alone, with
	// or without the option. `. ./lib.sh` over a file holding `false` fires
	// twice with nothing asked, because a sourced file never bounded this
	// trap in the first place.
	s.ErrTrapRefiresForTheCommandItFiredInside = interp.ErrTrapRefiresWhereItWasSetFirst
	// Reached only under `shopt -s lastpipe`: the last element is judged as a
	// statement and then the pipeline is, so `true | false` and `true |
	// /usr/bin/false` both write EE there.
	s.FailingPipelineWhoseLastElementRanHere = interp.LastElementJudgedThenThePipeline
	s.DebugTrapRunsInsideCalls = interp.No
	s.DebugTrapRefiresOnEnteringAFunction = interp.Yes
	// The heads whose own work is a word or an expression: `case`, `[[`,
	// `((` and `select` once each, the list `for` on every pass and each of
	// the arithmetic `for`'s three expressions every time one is evaluated —
	// written or not. `if`, `while`, a group, a subshell and a function
	// definition write nothing. The zero value, and the same in all three
	// bash columns.
	s.DebugTrapCompoundHeads = interp.DebugTrapHeadsWordAndArithmetic
	// A pipeline fires once for each element that is a **simple command**,
	// and it fires in the shell running the pipeline rather than in the
	// element — which is what lets a trap fire at all here, since this
	// shell does not carry one into a subshell. Measured: `trap 'echo d'
	// DEBUG; echo a | tr a-z A-Z` writes `d d A`, both actions in lower
	// case because neither went down the pipe, and `case x in x) echo hi;;
	// esac | cat` writes one firing rather than two — an element that is
	// not a simple command fires nothing, whatever head it would fire for
	// standing on its own.
	s.DebugTrapPipelines = interp.DebugTrapPipelinePerSimpleElement
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
	// An empty operand is refused here — `ulimit -n ""` is `: invalid
	// number` at 1 — where bash 3.2.57 reads it as nought and lowers the
	// limit to 0. A 5.x change, so the newer answer stands, as it does for
	// the permission copy above (#3064). Measured 2026-09-18.
	s.UlimitEmptyOperandIsZero = interp.No
	// And CDPATH is a search *beside* the ordinary relative lookup: a miss
	// falls back to the operand as it stands, and a `./` operand is not
	// searched at all (#2896).
	s.CdpathReplacesTheRelativeLookup = interp.No
	s.UlimitSetsBothLimits = interp.Yes
	// `ulimit -n hard` and `-n soft`, which bash alone has both of.
	s.UlimitTakesHardKeyword = interp.Yes
	s.UlimitTakesSoftKeyword = interp.Yes
	s.UlimitOperandIsArithmetic = interp.No
	s.BadOptionToSpecialBuiltinFatal = interp.No
	// `alias` is no more special here than POSIX makes it: the complaint is
	// said and the next command runs. Measured with `alias -g x`.
	s.AliasBadOptionFatal = interp.No
	// A name an alias may not carry, swept over every printable ASCII
	// character on 2026-09-12: whitespace and the shell's own
	// metacharacters, plus `/`. The five ksh93 refuses beside them —
	// `* ? [ { }` — are taken here, which is why the set is a value and not
	// an axis (#2413). `=` cannot be in it: the first one separates the name
	// from the value, so a name reaching the check never holds one.
	s.AliasNameRefusedCharacters = "\t\n \"$&'()/;<>\\`|"
	// Only a definition is checked. `alias 'a$b'` is `alias: a$b: not found`
	// here, which is the answer any name it does not hold gets.
	s.AliasNameCheckReachesALookup = interp.No
	// And the complaint costs the builtin 1 and the script nothing: with the
	// refusal on line 2 of three, line 3 still runs and the shell exits 0.
	s.AliasInvalidNameFatal = interp.No
	s.EarlierDeclarationLetterBlocksALaterPlus = interp.No
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
	// `exec 5<f; exec 6<&5-` moves the descriptor and reports 0, in 5.3, in
	// 3.2 and under argv[0] of `sh` alike. Two steps rather than one, which
	// shows twice: `true 6<&5-` leaves 5 closed once the command has ended,
	// where the plain `true 5<&-` is undone; and `{v}<&$w-` hands the name
	// the number *above* the one it moved from, because the destination is
	// chosen before the source is given up.
	s.FdMove = interp.FdMoveDuplicatesThenCloses
	s.DuplicationTargetError = interp.DuplicationTargetErrorCarriesOn
	s.LocalOutsideAFunctionIsAnError = interp.Yes
	s.LocalOutsideAFunctionIsFatal = interp.No
	// bash reports every operand that is not a name, exports the ones that
	// are, and carries on with a status of 1.
	s.BadNameToDeclarationFatal = interp.No
	s.BadNameToUnsetFatal = interp.No
	// `read` is not a special builtin anywhere, so its bad name is not fatal
	// here either — `read 1bad; echo after` prints both lines.
	s.BadNameToReadFatal = interp.No
	// And `getopts`: measured 2026-09-18, `echo A; getopts x 1bad -x; echo
	// "B st=$?"` writes all three lines here, so the refusal costs the
	// script nothing beyond its own status (#3555).
	s.BadNameToGetoptsFatal = interp.No
	// And `printf -v` refuses the same operands without ending anything:
	// measured 2026-09-17, `printf -v '1x' %s Q` is
	// ``printf: `1x': not a valid identifier`` at 2 with the rest of the
	// line still running, and `a-b` is the same sentence.
	s.BadNameToPrintfFatal = interp.No
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
	// unanswered ExportThroughASubscriptedOperandRecordsTheLetter: the
	// operand is refused one complaint earlier here, as
	// ``export: `a[1]': not a valid identifier`` at 1, so no element
	// is written and no letter could land.
	s.DeclarationTakesASubscript = interp.No
	// And a *declaration* takes one where `export` does not, which is this
	// shell alone splitting the two: `export a[1]=v` is `not a valid
	// identifier` and `typeset a[1]=v` creates the element, measured.
	s.TypesetTakesASubscript = interp.Yes
	s.UnsetTakesASubscript = interp.Yes
	// The one element goes and the rest of the array stands, inside a
	// subshell as at the top level.
	s.UnsetElementEmptiesAnUnwrittenArrayInASubshell = interp.No
	// `read 'a[2]'` and `printf -v 'a[2]'` fill the element, measured
	// 2026-09-10 on `a=(x y z)` — `x Q z`, in 3.2 as well as 5.3.
	s.StoreOperandTakesASubscript = interp.Yes
	// And `getopts` does not, in the shell that fills `read 'a[1]'`:
	// measured 2026-09-18, `getopts x 'o[1]' -x` draws this shell's
	// not-a-valid-identifier refusal at 1, brackets and all (#3555).
	s.GetoptsOperandTakesASubscript = interp.No
	// `r[@]` and `r[*]` on a builtin's operand are refused by the operand
	// as written and say nothing about arithmetic: measured 2026-09-17,
	// `r=(1 2 3); read 'r[@]' <<< Y; echo "same=$?"` is
	// `r[@]: bad array subscript`, `same=1`, the array whole and the rest
	// of the line still running. `printf -v 'r[@]'` answers identically,
	// 1 included — the number here is the refusal's and not the
	// builtin's own 2.
	s.StoreOperandWholeArraySubscript = interp.StoreOperandWholeArraySubscriptIsBad
	// Over a **table** the same brackets are an ordinary key, silently and
	// at 0: measured 2026-09-17, `typeset -A m=([k]=v); read 'm[@]'` on
	// `Z` leaves the keys `@` and `k` standing. So the array is refused
	// here and the table is taken, which is the way round this shell
	// answers the bare assignment too.
	s.StoreOperandWholeArraySubscriptOverATable = interp.WholeArraySubscriptIsAnOrdinaryKey
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
	// And the container letter, which this shell takes: `typeset -A m[k]=v`
	// puts `v` under the key `k` and `typeset -a n[2]=v` puts it at index 2.
	s.SubscriptedOperandTakesTheContainerAttribute = interp.Yes
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
	// An empty key is refused rather than stored: measured 2026-09-12 with
	// `declare -A m`, `m[""]=4` and `m[$w]=4` with an empty `$w` are both
	// `bad array subscript`, the table is untouched and the status is 1,
	// where ksh93 and zsh store. `m[ ]=7` is the control and stores under
	// one space in every column, so this is emptiness and not blankness
	// (#1938).
	s.EmptyAssociativeKeyIsAnError = interp.Yes
	// Neither array letter may take a name that is already the other kind:
	// measured 2026-09-12, `typeset -A h; h[k]=v; typeset -a h` is
	// `typeset: h: cannot convert associative to indexed array`, the table
	// is untouched, the status is 1 and the next command runs — and the
	// reverse direction is the same refusal in the other words. Identical
	// under argv[0] `sh`; 3.2.57 has no `-A` to reach it (#1375).
	s.TableUnderAnArrayDeclaration = interp.CompoundKindChangeRefused
	s.ArrayUnderATableDeclaration = interp.CompoundKindChangeRefused
	// The literal form is refused too, and it costs more: the complaint
	// names no builtin and the rest of the command list does not run.
	// Measured 2026-09-12 with `typeset -A h; h[k]=v` in front of it and the
	// commands newline-separated, `typeset -a h=(x)` is `h: cannot convert
	// associative to indexed array` at 1 with the table intact and the next
	// *line* running; the same four commands under `;` print nothing after
	// the complaint, by either invocation route (#2287).
	s.TableUnderAnArrayLiteralDeclaration = interp.CompoundKindChangeAbandonsTheLine
	s.ArrayUnderATableLiteralDeclaration = interp.CompoundKindChangeAbandonsTheLine
	// `a[@]=Z` over an indexed array is a bad subscript, and it costs the
	// command list rather than the input: measured 2026-09-12, `x=(p q);
	// x[@]=Z; echo "st=$?"; echo after` prints only `x[@]: bad array
	// subscript` where the same commands on separate lines print `st=1` and
	// `after`. Over a *table* the same spelling is an ordinary key and is
	// taken silently at 0, which is this shell swapping sides with zsh
	// between the two questions (#2285).
	s.WholeArraySubscriptAssigningAnArray = interp.WholeArraySubscriptIsABadSubscript
	s.WholeArraySubscriptAssigningATable = interp.WholeArraySubscriptIsAnOrdinaryKey
	// And a *read* whose key comes out empty is reported too, with a
	// different subject and a different outcome: measured 2026-09-12,
	// `typeset -A m; m[k]=v; w=; ${m[$w]}` writes `m: bad array subscript` —
	// the name alone — and still answers the empty string at status 0, once
	// per read. `${m[ ]}` is the control and says nothing, so this is
	// emptiness rather than blankness (#1972).
	s.EmptyAssociativeKeyIsReportedWhenRead = interp.Yes
	// And the *length* of that same element is refused outright, which is a
	// third answer to one emptiness rather than a louder second: measured
	// 2026-09-12, `typeset -A m; m[k]=v; w=; echo "[${#m[$w]}]"; echo after`
	// writes `[$w]: bad array subscript` — the subscript as written, brackets
	// and all, with no name in front of it — leaves 1 behind and runs neither
	// the `echo` it is in nor the one after it, where ksh93 and zsh both
	// print `[0]` and `after` at 0. The length operator alone: every other
	// operator over the same emptiness takes the read's report and its 0. A
	// name the letters merely declared is not this either — `typeset -A m;
	// ${#m[$w]}` is a silent `0` where `typeset -A m; m=()` in front of it is
	// refused (#2286).
	s.EmptyAssociativeKeyRefusesTheLength = interp.Yes
	// `$@` with no positional parameters is **unset** here, so a colon-less
	// conditional fires: measured 2026-09-12 after `set --`, `"${@-word}"`
	// is `word` and `"${@+word}"` is empty in 5.3.15, 3.2.57 and as `sh`,
	// where dash and zsh answer the other way round (#1941).
	s.PositionalListWithNoneIsSet = interp.No
	// The prefix is checked before the command's values are expanded and
	// before its redirections are opened, and this shell is alone in it.
	// Measured 2026-09-12 with `readonly x=1`: `x=$((1/0)) /bin/echo RAN`
	// says `x: readonly variable`, prints `RAN` and never mentions the
	// division, and `x=2 /bin/echo RAN >/nope/f` names the name and then the
	// file where dash, ksh93 and zsh name the file alone (#1943).
	s.PrefixToAFrozenNameIsCheckedFirst = interp.FrozenPrefixCheckedFirst
	// The brackets of an arithmetic subscript hold an expression, and `*` is
	// not one: measured 2026-09-11 on 5.3.15, `typeset -A m; m[k]=9;
	// $(( m[*] ))` is 0, where the expansion `"${m[*]}"` is 9.
	s.ArithWholeArraySubscriptIsTheSlice = interp.No
	// It is *reported* instead, and the expression carries on with zero:
	// measured 2026-09-12, `a=(3 4 5); $(( a[*] ))` writes `a[*]: bad array
	// subscript` and answers 0 at status 0, and `$(( a[*] + 1 ))` on `(3)`
	// answers 1. Identical in 3.2.57 and under argv[0] `sh`, and the write
	// reports too — `(( a[*] = 5 ))` says it once and stores nothing. An
	// association is silent and zero here and never reaches the question
	// (#1978).
	s.ArithWholeArraySubscriptIsReportedAsBad = interp.Yes
	// A subscript whose quotation never closes is a bad subscript here, not
	// a key found by scanning again with the quoting ignored. Measured
	// 2026-09-20 on 5.3.20 from a script file, `typeset -A a; k="q'r";
	// a[$k]=4; let "++a[$k]"` writes `a[q'r]: bad array subscript` twice
	// and leaves the element at 4, where ksh93u+ and zsh 5.9.2 both answer
	// 5. `(( a[$k]++ ))` beside it is 5 in every column, which is the
	// control that says this is the already-expanded operand and not the
	// key, and `shopt -s assoc_expand_once` moves this column to 5 too —
	// see interp.Runner.ExpandsAnOperandsSubscriptAgain (#3796).
	s.ArithSubscriptQuotationMustClose = interp.Yes
	// A subscript that *expanded* to nothing is the expression that is zero,
	// so `${a[$w]}` with an empty `$w` is element zero — measured 2026-09-11
	// on 5.3.15, `a=(5 6 7); w=; ${a[$w]}` is `5` at status 0, and `${a[ ]}`
	// beside it is too.
	s.EmptySubscriptTextIsAMathError = interp.No
	// The comma inside a subscript is the arithmetic operator here, as it is
	// everywhere else: measured 2026-09-12, `a=(p q r s); i="2,3";
	// ${a[$i]}` is the *third* element, which is the operator's right
	// operand. This dialect has no ranges for the question to be about
	// (#2160).
	s.SubscriptExpressionStopsAtASeparator = interp.No
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
	// A subscript that will not evaluate gives up the *command* here — the
	// rest of its line, and the enclosing function, list, `if`, loop or
	// subshell whole — and the script carries on at the next top-level
	// command with 1 behind it. Measured 2026-09-17 at all five sites; it
	// used to say "ends the script", which is what a probe written inside
	// `( … )` or under `-c` sees and is not what a script sees (#3485).
	s.BadSubscriptToUnset = interp.BadSubscriptAbandonsTheCommand
	// And the store a `read 'r[…]'` or a `printf -v 'r[…]'` operand walks
	// into gives up exactly as much, which is the half zsh answers
	// differently.
	s.BadSubscriptToAnOutputOperand = interp.BadSubscriptAbandonsTheCommand
	// And a declaration gives up exactly as much again — `declare 'a[b c]'=v`,
	// `typeset` and `local`, measured 2026-09-17 at the top level, in a
	// function, in an `&&` list, in an `if` condition, in a loop body, inside
	// `( … )` and inside `$( … )`. This is the site where the panel parts
	// company with bash twice over: zsh and ksh93 both end the script here,
	// where at `unset` both carry on (#3495).
	//
	// `readonly` and `export` never reach it: the bracketed operand is
	// refused one complaint earlier as ``readonly: `a[b c]': not a valid
	// identifier`` at 1, with the rest of the line still running.
	// A subscript inside `(( ))` is given up as the subscript's failure
	// here, which is the rule every other bracketed site follows in this
	// column: measured 2026-09-17, `a=(1 2 3); (( a[b c] )); echo "same=$?"`
	// prints no `same=`, the next line reads 1, and a `-c` string is given
	// up whole. `(( b c ))` — the construct's own arithmetic — is reported
	// and the line carries on, which is the control.
	s.BadSubscriptEscapesAnArithmeticCommand = interp.Yes
	s.BadSubscriptToADeclaration = interp.BadSubscriptAbandonsTheCommand
	// And with no value the brackets are never read at all, which is why the
	// axis above is reachable here only through an operand carrying one:
	// measured 2026-09-17, `a=(1 2 3); declare 'a[b c]'` is silent at 0 and
	// the array is untouched, `declare 'a[9]'` leaves it three long, and
	// `i=0; declare 'a[i++]'` leaves i at 0. The brackets say the *name* is
	// an array and nothing more — `declare a[3]` is `declare -a a` with
	// `${#a[@]}` at 0 (#1380).
	s.ValuelessSubscriptedOperand = interp.ValuelessSubscriptedOperandDeclaresTheName
	// But a name it has never heard of takes its brackets with it: `unset a
	// "a[x+]"` is silent at 0 here as in zsh, and `i=0; unset "nodecl[i++]"`
	// leaves i at 0. bash 3.2 agrees, so this is not a version split.
	s.UnsetSubscriptSkippedWhenNameUnset = interp.Yes
	// A failed operand's status is kept for the whole builtin rather than
	// overwritten by a later one. The subscript route cannot ask it — a bad
	// expression ends the command list here — but the readonly route can:
	// `readonly r=1; x=1; unset r x` is 1 in either order.
	s.UnsetStatusIsTheLastSubscripts = interp.No
	// An element write over a name holding a string promotes the string
	// first, so a subscript counting back from the end finds the element it
	// just made: `a=abc; a[-1]=x` writes the first one.
	s.NegativeSubscriptCountsOverAPromotedScalar = interp.Yes
	// A negative subscript that counts back past the first element is
	// refused rather than placed in front of it, and the refusal ends the
	// script. Measured in both bash builds.
	s.NegativeSubscriptPastTheStartInserts = interp.No
	s.SubscriptBeforeTheFirstElementRead = interp.SubscriptBeforeStartIsReported
	s.SubscriptBeforeTheFirstElementNeedsAnElement = interp.No
	// And the *length* of that element is a second answer here, which no
	// other column has: measured 2026-09-18, `a=(x y z); echo
	// "[${#a[-4]}]"; echo after` writes `[-4]: bad array subscript` and then
	// `after`, with the `echo` abandoned — where the read one line up names
	// the array, expands to nothing and carries on. Same rows for `a=()` and
	// for `a=x`; a name holding nothing at all is `[0]` and silent, which is
	// the length being answered before the subscript is looked at (#3591).
	s.SubscriptBeforeTheFirstElementRefusesTheLength = interp.Yes
	s.OperandSubscriptQuoting = interp.OperandSubscriptEveryQuote
	s.ArithmeticOnlyBodyIsAnArithmeticExpansion = interp.No
	// `declare a=1; declare a+=2` is `12`: a declaration's operand carries
	// the append operator here, where the other three refuse the name `a+`.
	s.DeclarationTakesAnAppendOperand = interp.Yes
	// And with a subscript on the name too: `typeset -a a; a[1]=p;
	// typeset a[1]+=q` leaves `pq` in the element. The one utility that
	// does not reach it is `export`, which refuses the brackets
	// themselves — see DeclarationTakesASubscript.
	s.DeclarationTakesASubscriptedAppendOperand = interp.Yes
	// And the same operator on an *array literal* operand, which this shell
	// takes too: `typeset u+=(3 4)` is silent at status 0 here where bash
	// 3.2.57 refuses the name `u+` it leaves behind.
	s.DeclarationTakesAnAppendingArrayOperand = interp.Yes
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.No
	// A stopped job keeps the current-job marker: measured 2026-09-12
	// through a pseudo-terminal, `sleep 40` stopped with ^Z and then
	// `sleep 41 &` lists `[1]+  Stopped` and `[2]-  Running`, and `jobs %+`
	// and `jobs %-` name those same two. bash 3.2.57 agrees. With two jobs
	// stopped and a third backgrounded it is `[1]-  [2]+  [3]` — the second
	// stopped job current, the first its runner-up, and the background job
	// unmarked at all, which no reading of the table's order produces.
	s.StoppedJobTakesTheCurrentJobMarker = interp.Yes
	s.JobsListFinishedJobs = interp.Yes
	s.EndedJobIsListedAsRunningWithoutTheMonitor = interp.No

	// `jobs`' letters. bash has the widest set in the panel: POSIX's `-l`
	// and `-p`, the state filters `-r` and `-s`, `-n` for what has changed
	// since it last said, and `-x` — which is not a listing at all but a
	// command to run with its job specs replaced by process ids. The last
	// two ride UnimplementedOptionLetters and are refused by name.
	s.JobsOptions = "lprs"
	// `jobs -p` here is the process ids and nothing else, which is what
	// makes `kill $(jobs -p)` mean what it is written to mean.
	// unanswered JobsListsWhatChangedSinceTheLastReport: `jobs -n` is not a
	// letter this shell has — it is not in JobsOptions — so the axis is
	// never consulted here. ksh93 is the one column with the letter, and
	// bash's letter of the same name is a different question that stays
	// unimplemented (#3390).
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
	// The two letters that are attributes of a *function* here, in the order
	// a listing writes them — `declare -frx d` for one that is both. bash is
	// the only shell in the panel with the notion at all: zsh reads `-F` as a
	// float's precision and refuses `export -f`, and ksh93, dash and BusyBox
	// ash each end the script over `readonly -f`. Measured 2026-09-16 on
	// 5.3.20 and 3.2.57, which agree line for line (#3192). See
	// Semantics.FunctionAttributeLetters for what the letters do.
	//
	// `t` joined the two in #3051. It is the trace mark, measured on
	// 5.3.20 the same day as the other two: `declare -ft a` is a silent 0,
	// `declare -F` writes `declare -ft a`, `declare -Ft` narrows to the
	// marked functions, and a function carrying all three lists as `declare
	// -frtx a`. What a traced function *does* here — inherit the DEBUG and
	// RETURN traps, which `set -o functrace` asks for wholesale — is not
	// built, so the mark is recorded and listed and read by nothing.
	s.FunctionAttributeLetters = "rtx"
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
	// And bash alone leaves a hole a hole. Measured 2026-09-12 with three
	// jobs started and the middle one killed and reaped, `jobs %2` is still
	// no-such-job afterwards and the new job answers `%4` — where dash,
	// ksh93, zsh and ash all put it back in `%2`.
	s.NextJobNumberRefillsAHole = interp.No
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
	// The other route is the other way round. Measured 2026-09-12 on `-i -c`
	// through a pseudo-terminal, with the job held open on a fifo the string
	// releases and then reaps: bash writes `[1] <pid>` where dash and ash write
	// nothing at all. So the two columns disagree and this is a second field.
	s.InteractiveCommandStringAnnouncesJobs = interp.Yes
	// And bash alone keeps the `Done` row back for a prompt. On that same
	// `-i -c` run 5.3.15 writes the start and never the end — with `wait`, with
	// `wait %1`, and with a whole second of `sleep` after the job died — while
	// the same program on a pipe, where `-i` draws a prompt between the lines,
	// writes `[1]+  Done` at the prompt after `wait`. It is the prompt and not
	// the route. 3.2.57 writes the row on both, which is why this is a field:
	// without it bash would have no single answer to give.
	s.FinishedJobNoticeNeedsAPrompt = interp.Yes
	// `$!` before any background command is *unset*, not set and empty:
	// measured, `${!-unset}` takes its default and `${!+set}` is empty, and
	// `set -u; echo "[$!]"` writes `$!: unbound variable` and stops at 127 in
	// 5.3.15, in 3.2.57 and in 3.2 run as `sh`. Without `set -u` it expands
	// to nothing, which is three other columns' answer too, so the noisy half
	// only shows where a script asked to be told.
	s.LastBackgroundPid = interp.LastBackgroundPidUnset
	// A job started with `&` reads an empty standard input, not the shell's:
	// measured 2026-09-07, `bash -c '/bin/cat & wait; echo ---; /bin/cat' < f`
	// writes `---` and then the file's line, in 5.3.15 and 3.2.57 alike. What
	// POSIX XCU 2.9.3 specifies, and what keeps the script's own `read` from
	// losing the lines a background job would otherwise eat.
	s.BackgroundJobInput = interp.BackgroundJobInputEmpty
	s.ProcessSubstitutionIsTheLastBackgroundJob = interp.Yes
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
	s.CdHasSymlinkFreeOption = interp.No
	s.CdLastPathOptionWins = interp.Yes
	s.BadSetOptionNameFatal = interp.No
	// Neither spelling ends the script here: `set -o zzznosuch; echo one;
	// set -Z; echo two` prints both words and leaves at 0. Written rather
	// than inherited so that the agreement between the two is a measured
	// answer — the substrate assumed it until #2629 and was wrong about
	// BusyBox ash.
	s.BadSetOptionLetterFatal = interp.No
	// The one column in the panel with a POSIX mode, and it moves both:
	// `set -o posix; set -o zzznosuch; echo "st=$?"; echo after` prints the
	// complaint and nothing else at 2, and the same line with `set -Z` does
	// too. Measured 2026-09-14 on 5.3.15, and the `sh` name is the other
	// door to the same place — `bash-as-sh` stops for both spellings where
	// `bash` stops for neither. `set +o posix` puts the two answers above
	// back (#2641).
	//
	// bash 3.2.57 takes the letter and leaves the name — `set -o posix; set
	// -o zzznosuch` is still 1 and still carries on there, under `-o posix`
	// and under the `sh` name alike. This preset claims 5.3 and says so
	// here, because a reader arriving from macOS's /bin/bash would otherwise
	// read this as a claim about their shell.
	// A function named after a special builtin is refused in POSIX mode and
	// defined outside it — and the check is at the **definition**, so a
	// script may enter the mode from inside the same input and be refused by
	// it: `bash -c 'set -o posix; export() { :; }; printf b'` writes
	// ``export': is a special builtin`` and prints nothing. Measured
	// 2026-09-18 a name at a time against the roster: all sixteen refuse,
	// `.`, `:` and `source` included, while `local`, `true`, `read`, `cd`,
	// `echo`, `alias` and `pwd` define at 0 — so it is this shell's list of
	// special builtins and not a table of its own, unlike ksh93's and
	// dash's, which are the parser's (#2987).
	s.SpecialBuiltinNameIsNotAFunctionNameInPosixMode = interp.Yes
	s.BadSetOptionNameFatalInPosixMode = interp.Yes
	s.BadSetOptionLetterFatalInPosixMode = interp.Yes
	// `-o` takes the next word and never the rest of its own: measured,
	// `set -oe x` is `x: invalid option name` with errexit left off, and
	// `bash -oerrexit -c cmd` refuses `-c` the same way. With no word behind
	// it, `set -oe` writes the option table and then turns errexit on.
	s.SetOLetterAttachesItsName = interp.No
	// Measured 2026-09-16, bash 5.3.20 and 3.2: `bash --xtrace -c 'echo ran'`
	// is `--xtrace: invalid option` followed by the table of the seventeen
	// GNU long options it does have, and nothing runs. A `set -o` name is not
	// among them, so a `--name` word here is a name bash looks up in a table
	// of its own rather than in the option namespace.
	s.LongOptionNamesASetOption = interp.No
	// unanswered LongOptionNameIgnoresHyphens: the fold is a rule of the
	// `--name` spelling, and the axis above says this shell has no such
	// spelling. There is no site here to put the question to.
	// unanswered LongOptionValueIsANumber: the `=value` it reads rides on a
	// `--name` option word, and the axis above says this shell has no such
	// word. There is no site here to put the question to.
	// An option name is read exactly as it is written here — no separators
	// come out of it and no `no` goes in front of it. Measured 2026-09-18 on
	// bash 5.3.20: `set -o err-exit` and `set -o noerrexit` are each
	// `set: <word>: invalid option name` at 2, where `set -o errexit` beside
	// them is 0 (#3155, #3254).
	s.OptionNamespaceIgnoresSeparators = interp.No
	s.OptionNamespaceTakesANoPrefix = interp.No
	// And the next word is only taken when it does not look like options
	// itself: `set -o -e` here writes the option table and turns errexit
	// **on**, where the name that would have been refused is `-e`. Only at
	// the builtin — `bash -o -e -c cmd` on the command line that starts the
	// shell is `-e: invalid option name`, which is the front end's own
	// position-sensitive parse and not this axis.
	s.SetODeclinesADashWord = interp.Yes
	s.SetListsOptionsOnceAtTheEnd = interp.No
	// A bare `-` turns `-x` and `-v` off here, and a bare `+` is consumed with no effect (#2699).
	s.BareOptionWord = interp.BareDashClearsTraceAndVerbose
	// And every option word's letters are read before any of them is
	// applied: `command set -e -Z` here leaves errexit **off**, where dash
	// and BusyBox ash leave it on. The pass that does it knows letters only —
	// `command set -e -o zzznosuch` is errexit on — and a letter welded
	// behind an `-o` is past it, which is why `set -ozzznosuch` reports 1 and
	// carries on where `set -Z` is 2 and ends an `sh` script.
	s.SetValidatesOptionLettersFirst = interp.Yes
	s.BadSetOptionNameAtInvocationExitsZero = interp.No
	s.UnknownConditionOptionIsAStatus = interp.No
	s.ReturnOutsideAFunctionIsRefused = interp.Yes
	// `break` with no loop around it is reported and then ignored here: the
	// next command on the line runs and the status stays 0. Measured with
	// `echo t; break; echo after` — `after` prints and `$?` is 0.
	s.LoopControlOutsideALoopIsFatal = interp.No
	// The place is looked at before the count: `break abc` with no loop to
	// leave names the loops POSIX has and never mentions `abc`, and the
	// script carries on at 0. Identical in 3.2 and, but for the silence,
	// in the same binary called as `sh`.
	s.LoopControlPlaceIsJudgedBeforeTheCount = interp.Yes
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
	// A plain `unset NAME` falls through to the function table when the
	// name holds no variable, which is bash's alone: measured 2026-09-16,
	// 5.3.20, the same binary as `sh` and 3.2.57 all answer 127 to a call
	// after `unset b`, where zsh, ksh93u+, dash and BusyBox ash all still
	// run the function.
	s.UnsetReachesTheFunctionTable = interp.Yes

	// Whether a redirection target is expanded as an ordinary word.
	s.RedirectTargetIsAnOrdinaryWord = interp.Yes
	// The target is split and matched, which is the half of the reading
	// above that POSIX forbids — and this is the shell that changes sides
	// with the mode. `set -o posix` and the `sh` name both turn it off:
	// `cat < only-*.txt` becomes `No such file or directory` with the match
	// sitting there, `printf X > only-*.txt` creates a file called
	// `only-*.txt`, and `e="a b"; > $e` writes a file called `a b` rather
	// than being ambiguous. `> {c,d}` stays ambiguous in both modes, which
	// is what keeps this axis apart from the one above. Measured in 5.3.20
	// and 3.2.57, 2026-09-16 (#3207).
	s.RedirectTargetTakesPathnameExpansion = interp.Yes

	// Whether `type --` ends the options.
	// `type .` is `. is a shell builtin` in bash 5.3.20 and in bash 3.2.57
	// alike — one answer across twelve years of this shell. The same binary
	// invoked as `sh` says `a special shell builtin`, which is the POSIX
	// preset's row and not this one.
	s.TypeDistinguishesSpecialBuiltins = interp.No
	s.TypePrintsFunctionBody = interp.Yes
	s.TypeEndsOptionsWithDashDash = interp.Yes
	// type's letters, all implemented here: -a for every resolution, -p
	// speaking only where the plain answer would have been a file, -P
	// forcing the PATH search, -f leaving functions out, and -t — the one
	// shell in the panel with it at all — answering one bare word per name
	// and staying silent with status 1 for a name that is nothing.
	s.TypeOptions = "afpPt"
	s.TypePSearchesPathPastTheShell = interp.No
	s.TypePathAnswerIsASentence = interp.No
	s.TypeFSaysTheFunctionBack = interp.No

	// The letters `declare` and `local` read — one set under two names
	// here, with `-F` naming functions rather than setting a float's
	// precision, which no other engine spells this way. `-n` is here since
	// #2553: it is the **name reference**, a language feature rather than an
	// attribute, and interp/nameref.go is the whole of it. `-I` joined them
	// in #3434: it is the per-declaration spelling of `localvar_inherit`,
	// and interp/localinherit.go is the whole of it. `-t` joined them in
	// #3101: it is the **trace attribute**, which this shell records and
	// lists back and nothing else here reads — see Runner.traced.
	s.DeclareOptions = "aAfFgiIlnprtux"
	// unanswered FloatFormatLetterE: this shell has no `-E` on a declaration
	// to give a rendering to. Measured 2026-09-15 on 5.3.15 and 3.2.57
	// alike, `declare -E 3 a=1.5` is `declare: -E: invalid option` (#2559).
	// unanswered DeclareNumberDetachedOnlyAtTheWordEnd: no letter here takes
	// a detached number at all — DeclareOptionsTakingANumber is empty and
	// `-i` takes no base, so `declare -i 8 n=64` is `8: not a valid
	// identifier` — and there is no position for the rule to be about.
	// unanswered BareFloatLetterResetsThePrecision: there is no float
	// precision here to keep or reset; `-F` is the *function* listing under
	// this name, and `declare -F 3 a=1.5` is ``cannot use `-f' to make
	// functions``.
	// unanswered NumericTypeLetterPrecedence: nor can a rank between them be
	// asked, for the same reason — `-F` is the function listing here and
	// there is no second numeric letter to rank against `-i` (#2419).
	// unanswered DeclareHideInScopeLetter: nor a lower-case `h` under either
	// reading. Measured 2026-09-14, `typeset -h s q=1` and `declare -h s
	// q=1` are both `invalid option` with the usage line, so neither the
	// hide-in-scope attribute nor the string argument can be put to it.
	// unanswered DeclareZeroFillLetter: there is no `Z` letter here under
	// either reading. Measured 2026-09-18 on 5.3.20 and 3.2 alike, `declare
	// -Z 4 d=7` is `declare: -Z: invalid option` with the usage line, so the
	// question of whether it is a justification or a fill cannot be put.
	// unanswered WidthJustificationPrecedence: nor can a pair of them be
	// written to rank — `declare -LR 5 a=7` is the same refusal, at the `L`.
	// unanswered WidthLettersExcludeTheIntegerLetter: nor the integer letter
	// beside one, for the same reason (#2859).
	// unanswered NumericTypeLettersAreExclusive: the pair cannot be written.
	// `-E` is not a letter at all and `-F` is the function listing, so
	// `declare -iF a=1` is that same refusal in 3.2 and `-i: invalid option`
	// in 5.3 — neither of them the question this asks.
	// unanswered DeclareMatchingLetter: bash has no `m` letter under either
	// reading. Measured 2026-09-13 on 5.3 and 3.2 alike, `declare -m q=1` is
	// `declare: -m: invalid option` followed by the usage line, so the
	// question of what the letter *means* never arises (#2345).
	// unanswered DeclareMappingLetter: the same, for `typeset -M tolower v`.
	// unanswered DeclareTypeLetter: nor a `T` letter under either reading.
	// Measured 2026-09-13 on 5.3.15 and 3.2.57 alike, `declare -T q=1` is
	// `declare: -T: invalid option` at 2 with the usage line under it, so
	// neither the tie nor the type can be asked for here (#2419).
	// unanswered DeclareHideValueLetter: the same, for `declare -H q=1`.
	// `local -n` is the common spelling of a reference and not an afterthought
	// of `declare -n`: a function taking the name of a variable to fill in is
	// what the letter exists for, and the letter has to be on this word for
	// the fresh binding to be the function's own. `-I` is here for the same
	// reason from the other side: it is what says the fresh binding is *not*
	// the function's own but a copy of the caller's, so it belongs on the
	// word that makes the binding (#3434).
	s.LocalOptions = "aAgiIlnprtux"
	// A bad `declare` option is reported and the script goes on.
	s.TypesetBadOptionFatal = interp.No
	// A lone `-` or `+` is a *name* here and not an option word, and not one
	// a script may declare: measured 2026-09-10, ``declare -`` is ``declare:
	// `-': not a valid identifier`` at 1 where zsh and ksh93 both read it as
	// an option word and list. See Semantics.SignAloneIsAnOptionWord.
	s.SignAloneIsAnOptionWord = interp.No
	// Nor on `export` and `readonly`: the sign is a name to them as well,
	// measured on bash 5.3.15 and 3.2.57 alike — ``export: `+': not a valid
	// identifier``.
	s.SignAloneIsAnOptionWordToExport = interp.No
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
	// `local -` saves the `set` table for the life of the call: measured
	// 2026-09-15, `f() { local -; set -f; }; f; echo $-` comes back without
	// the letter. The `set` table alone — a `shopt` the body changed
	// survives the return.
	s.LocalDashSavesTheShellOptions = interp.Yes
	s.BareLocalListing = interp.BareLocalListsLocals
	// The *declaration* word lists something else again, and it is this
	// shell's `set` listing exactly: every variable as an assignment, then
	// every function. Measured 2026-09-12 with a scrubbed environment,
	// `diff <(declare) <(set)` empty. It was left unanswered while the two
	// values the form carried were neither of those, so a bare `declare`
	// refused; `declare +f` and `declare +F` are the same listing reached by
	// taking the function attribute off, and had nothing to fall through to
	// (#1754).
	s.BareTypesetListing = interp.BareLocalListsWhatSetLists
	// A bare `set` writes the variables and then every defined function,
	// values quoted only where they must be, `'\''` for an embedded quote
	// and `$'...'` once a control character appears.
	s.SetListing = interp.SetListingAssignmentsThenFunctions
	// The array is why this shell's `coproc` takes a name: the ends arrive in
	// it, and a script writes `echo hi >&"${COPROC[1]}"`.
	s.CoprocEndsInAnArray = interp.Yes
	// And the array goes when the coprocess does. Measured 2026-09-12:
	// `declare -p CP` answers `not found`, both descriptors are gone, and
	// `echo x >&${CP[1]}` is an ambiguous redirect at 1 rather than a write
	// into a pipe nobody is reading (#2411).
	s.ReapedCoprocessEnds = interp.CoprocEndsGoWithTheCoprocess
	// And the ends are put out of the way of the numbers a script allocates
	// for itself. Measured 2026-09-13: four coprocesses in a row publish
	// `63 60`, `62 58`, `61 56` and `59 54`, while `exec {v}>/dev/null` with
	// one running still answers 10 (#2596).
	s.CoprocessEndPlacement = interp.CoprocEndsAtTheTopOfTheTable
	// A duplication's descriptor number has no ceiling of this shell's own
	// — `echo x >&99` is the ordinary bad-descriptor sentence (#3210) — the
	// `-v` echo ends a line the input did not end (#3130), and `read -t`
	// takes a written number rather than an expression (#3209).
	s.DescriptorNumberCeiling = interp.NoDescriptorNumberCeiling
	s.VerboseEchoAddsAMissingNewline = interp.Yes
	s.ReadTimeoutOperandIsArithmetic = interp.No
	s.SetListingQuoting = interp.ListingQuoteWhenNeededEscaped

	// `[[ -v 1 ]]` and `[[ -v 0 ]]` ask about a positional parameter here,
	// where ksh93 declines to read a digit as a name at all. The parameters
	// spelled as one punctuation character are the other way round: `[[ -v ?
	// ]]` is unset here and set in zsh. Measured on bash 5.3.15, and it is
	// the operator rather than the lookup — `[[ -n ${?+s} ]]` is set here.
	s.ParameterIsSetSeesPositionals = true

	// And `[[ -v a[k] ]]` reads the subscript the script wrote rather than
	// the one left standing after the operand is expanded, so a key with a
	// bracket in it is found when the bracket was quoted or came out of an
	// expansion. bash is alone in the panel here — see
	// interp.Semantics.ConditionIsSetReadsTheWrittenSubscript, including the
	// row where bash's own `test -v` answers the other way.
	s.ConditionIsSetReadsTheWrittenSubscript = interp.Yes

	// And `[[ a[k] -eq 5 ]]` reads it the same way, which is this shell's
	// alone: ksh93 reads a quotation inside a subscript in `(( ))` and still
	// refuses it here. See
	// interp.Semantics.ConditionArithmeticReadsTheWrittenSubscript (#3302).
	s.ConditionArithmeticReadsTheWrittenSubscript = interp.Yes

	// A subscript that reaches the condition as *text* is expanded once more
	// before it is looked up, so `k='x y'; [[ -v 'm[$k]' ]]` asks about the
	// key `x y` and not about the two characters `$k`. Only where no bracket
	// was written: the axis above has already answered `[[ -v m[$kk] ]]`,
	// where bash stops at the one expansion the written subscript gets. See
	// interp.Semantics.ConditionIsSetExpandsAFlatSubscript (#3298).
	s.ConditionIsSetExpandsAFlatSubscript = interp.Yes

	// And a declaration's operand expands its subscript the same way, in
	// every spelling of the utility: `declare 'd[$k]'=Q` writes the key
	// `x y`, and `declare 'arr[$i]'=Q` writes the element `$i` counts to.
	// This one does not move with `shopt -s assoc_expand_once` — measured
	// 2026-09-19, the key is `x y` with the option set and unset alike — so
	// it is the dialect's answer rather than a switch. See
	// interp.Semantics.DeclarationOperandExpandsItsSubscript (#3298).
	s.DeclarationOperandExpandsItsSubscript = interp.Yes

	// And the three surfaces where a script *can* turn the round off, which
	// is what `shopt -s assoc_expand_once` and its synonym name. The axis
	// says this shell rounds; the switch beside it, which only this dialect
	// wires, is what a script uses to stop it. See
	// interp.Runner.ExpandsAnOperandsSubscriptAgain.
	//
	// Measured 2026-09-19 on bash 5.3.20, from a script file with standard
	// input on /dev/null, over `k='x y'` and a table holding one element
	// under the key `x y` — the left column is this shell's default and the
	// right the same probe under `-O assoc_expand_once`:
	//
	//	unset -v 'a[$k]'      the element is gone   it survives
	//	unset 'a[$k]'         the element is gone   it survives
	//	printf -v 'c[$k]' P   the key `x y`         the key `$k`
	//	read 'b[$k]' <<<Z     the key `x y`         the key `$k`
	//	test -v 'g[$k]'       true                  false
	//	[ -v 'g[$k]' ]        true                  false
	//	[[ -v 'g[$k]' ]]      true                  **true**
	//
	// The last row is why `[[ -v ]]` is answered by the axis above and not
	// by one of these: the same question reaches the keyword and the builtin
	// and only the builtin's answer is a script's to change. The controls
	// are the same operands with nothing in them to expand — `unset
	// 'a[plain]'` removes the element and `test -v 'g[nope]'` is false in
	// every column — which say the rows above turn on the round rather than
	// on whether a quoted subscript reaches the builtin at all.
	s.UnsetExpandsAFlatSubscript = interp.Yes
	s.OutputOperandExpandsAFlatSubscript = interp.Yes
	s.TestIsSetExpandsAFlatSubscript = interp.Yes
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
		// A count that is a number and is not positive gets its own
		// sentence here, and this is the one column that then carries on:
		// `for i in 1 2; do break 0; echo tail; done; echo after` prints
		// `after` at status 0 with no `tail`, so the loop still ends and the
		// count was taken as 1. The other four end the script. A word that
		// is no number at all takes NumericArgument's sentence — `break:
		// abc: numeric argument required` — and ends the script here too
		// (#2800).
		LoopControlCountOutOfRange: "%[1]s: %[2]s: loop count out of range",
		// The builtin names itself, because bash's layout puts the shell and
		// the line in front of it: `bash: line 3: break: too many arguments`.
		NumericOperandTooMany: "%[1]s: too many arguments",
		TypeKeyword:           "%[1]s is a shell keyword",
		TypeFunction:          "%[1]s is a function",
		// The third sentence for an external, once the name is in the
		// command hash: measured 2026-09-18, `type ls` is `ls is /bin/ls`
		// on a first lookup and `ls is hashed (/bin/ls)` after an `ls` has
		// run, and `command -V` writes whichever of the two `type` writes
		// (#3579).
		TypeHashedExternal: "%[1]s is hashed (%[2]s)",
		// The only wording in the panel that carries its own quotes, and the
		// only verb that is not "alias".
		TypeAlias:     "%[1]s is aliased to `%[2]s'",
		CommandVAlias: "alias %[1]s=%[2]s",
		TypeNotFound:  "type: %[1]s: not found",
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
		// And the line above it, which bash 5.3 writes and bash 3.2 does
		// not. Measured 2026-09-12 with no terminal on any of the three
		// standard streams, on `-ic` and `-lic` alike:
		//
		//	bash: cannot set terminal process group (91050): Inappropriate ioctl for device
		//	bash: no job control in this shell
		//
		// The errno half is fixed text rather than a rendered error, and
		// deliberately: this line is only ever written by a shell that has
		// no terminal, so the failure it describes is always the same one.
		// See Diagnostics.CannotSetTerminalProcessGroup (#1036).
		CannotSetTerminalProcessGroup: "cannot set terminal process group (%[1]d): Inappropriate ioctl for device",
		// One sentence for both ends of the range, naming the word as
		// written. The difference between them is not the wording: a count
		// below zero is always complained about, and a count above `$#` is
		// silent until `shopt -s shift_verbose` — see
		// Runner.ReportsShiftPastTheEnd, which Configure turns off here
		// because this shell's default is the quiet one (#3465).
		//
		// Measured 2026-09-17 on bash 5.3.20 and 3.2.57 from a script file,
		// with the option on: `set -- a; shift 3` is `shift: 3: shift count
		// out of range` at 1 with `$#` untouched, and `shift` with nothing
		// left drops the slot rather than filling it — hence the second
		// wording. `shift 0` at `$#` of nought is 0 and silent, and so is a
		// count that lands exactly on `$#`, so the complaint really is the
		// range and not the emptiness.
		//
		// bash 3.2 names the marker where 5.3 names the count for
		// `shift -- 5`, which is a change within bash and is the `bash32`
		// column's to record rather than this preset's — the same age
		// `break -- -1` already has in the corpus.
		ShiftTooMany:            "shift: %[2]s: shift count out of range",
		ShiftTooManyWithNoCount: "shift: shift count out of range",
		ShiftNegativeCount:      "shift: %[2]s: shift count out of range",
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
		// `${!v}` refusing its source, which 5.3 has and 3.2 does not; see
		// interp.Runner.refuseIndirection.
		IndirectionUndeclared: "%[1]s: invalid indirect expansion",
		IndirectionNotAName:   "%[1]s: invalid variable name",
		// The same sentence this shell writes for a name nothing declared,
		// and a different one from ksh93's — see the field.
		IndirectionUnaimedReference: "%[1]s: invalid indirect expansion",
		// The letter as the script spelled it, sign and all: `set +q` is
		// refused as `+q` here where dash and zsh write `-q` either way.
		// The refusal is this shell's ordinary bad-option complaint, so
		// `set`'s usage line follows it — and does not follow the different
		// sentence a bad `-o` name earns, which is why
		// SetInvalidOptionNameUsage is not set.
		SetInvalidOptionLetter: "set: %[1]s: invalid option",
		// The two spellings answer alike here, and they are written rather
		// than left to the zero so that the sameness is a measurement.
		// bash 3.2 is why that matters: the *same shell* three versions back
		// reports 1 for the name and 2 for the letter. Measured 2026-09-13
		// on 5.3.15 with `set -o zzznosuch` and `set -Z` (#2629).
		SetInvalidOptionNameStatus:   2,
		SetInvalidOptionLetterStatus: 2,
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
		// A `--word` this front end could not place: the same sentence a
		// refused letter gets, with the whole word in it, and the same
		// twenty-two-line block under it. Measured 2026-09-16 on bash 5.3.20
		// with standard input on /dev/null — `bash --badopt` and `bash -q`
		// each write one sentence and then the identical block, both at 2 —
		// where this shell wrote a single `unknown option "--badopt"` and no
		// block at all (#2298). The word is echoed whole: `--initfile` is
		// refused as `--initfile` and never as the `--init-file` it is one
		// character from.
		InvocationBadLongOption:            "%[1]s: invalid option",
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
		// `bg` on a job that is not stopped: the builtin is named in the
		// sentence and the job by number, and the status is **0** — the
		// shell complains and reports success, where zsh complains and
		// reports 1. Measured 2026-09-15 from a script with `set -m` on a
		// pseudo-terminal and again at an interactive prompt, the same both
		// times (#2838).
		JobAlreadyInBackground: "%[1]s: job %[2]d already in background",
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
		// And a duplication whose source is not open names the word the
		// script *wrote*: `n=10; echo x >&$n` is `$n: Bad file descriptor`
		// here where ksh93 and zsh both say `10`. The sentence is the
		// substrate's; only the naming moves (#734).
		NamesTheDuplicationTargetAsWritten: true,
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
		// The newlines between a `$(` and the first command of its body
		// count for nothing here, so that command is on the opener's line —
		// `echo "$(` on line 2 with the command under it reports `$LINENO`
		// as 2 where zsh, ksh93 and dash all report 3, and three blank lines
		// between them still report 2. Measured 2026-09-17 (#3362).
		SubstitutionBodyStartsAtItsOpenersLine: true,
		// And a *backquoted* body's refusal is placed at the failure's own
		// file line plus the newlines inside the backquotes, where the
		// substitution stands in the command's first token: `` v=`echo hi⏎for`
		// `` opening line 2 is reported at line 4 where `` cat `echo hi⏎for` ``
		// is reported at 3, and `` v=1 w=`echo hi⏎for` `` is 3 again.
		// Measured 2026-09-18 over forty-four shapes (#3553).
		BackquotedSubstitutionFailureAddsItsBodysNewlines: true,
		EchoesTheOffendingLine:                            true,
		// A substitution body read at expansion time and refused is named
		// as the construct it came from, which goes with the failure being
		// the word's rather than the script's — see
		// interp.Semantics.SubstitutionParseErrorIsFatal (#2703).
		SubstitutionParseFailureNamesTheConstruct: true,
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
		// And no line at all at a prompt: measured 2026-09-11 under `-i`,
		// `if; then` is `bash: syntax error near unexpected token `;'` where
		// the same line in a script is `s.sh: line 1: …`. The line *inside* a
		// sentence stays — `unexpected end of file from `if' command on line
		// 1` is what it writes at a prompt too — which is why this is the
		// location and not a second wording.
		PromptLocation: interp.LocationNameOnly,
		// And a builtin's complaint drops the line there too, which needs
		// saying separately because BuiltinLocation is a field of its own:
		// measured 2026-09-11 under `-i`, `cd /nope` is `bash: cd: /nope: No
		// such file or directory` where the same line in a script names one
		// (#2024).
		PromptBuiltinLocation: interp.LocationNameOnly,
		NotFound:              "%s: command not found",
		UnboundVariable:       "%s: unbound variable",
		// And the **count** of a whole array names the bare name where every
		// other subscripted refusal here writes the brackets back:
		// `${#a[@]}` is `a: unbound variable` and `${a[@]}` on the same
		// unset name in bash 3.2.57 is `a[@]: unbound variable`. Measured
		// 2026-09-18 on bash 5.3.20 and bash 3.2.57, from a script file
		// under `set -u` (#3125).
		WholeArrayCountNamesTheBareName: true,
		// The sigil written back, which no other column does: `${@:=w}`
		// with no parameters is `$@: cannot assign in this way`, and
		// `${1:=w}` names `$1`. Identical in 3.2.57 and in the same binary
		// under argv[0] of `sh` (#1541).
		AssignThroughExpansionBadName: "$%[1]s: cannot assign in this way",
		UnboundPositional:             "$%s: unbound variable",
		NumericArgument:               "%[1]s: %[2]s: numeric argument required",
		// A subscript before the first element, named as it was written:
		// `a[x-2]`, not the -1 it evaluated to. Identical in bash 3.2.
		BadArraySubscript:                  "%[1]s[%[2]s]: bad array subscript",
		SubscriptBeforeTheFirstElementRead: "%[1]s: bad array subscript",
		// The length's subject is the subscript as written, brackets and
		// all, where the read's is the array's name.
		SubscriptBeforeTheFirstElementLength: "[%[1]s]: bad array subscript",
		CannotConvertTableToArray:            "%[2]s: %[1]s: cannot convert associative to indexed array",
		CannotConvertArrayToTable:            "%[2]s: %[1]s: cannot convert indexed to associative array",
		// One verb rather than two: the literal form's complaint comes from
		// the assignment and names no builtin. See
		// Semantics.TableUnderAnArrayLiteralDeclaration.
		CannotConvertTableToArrayAtTheAssignment: "%[1]s: cannot convert associative to indexed array",
		CannotConvertArrayToTableAtTheAssignment: "%[1]s: cannot convert indexed to associative array",
		EmptyAssociativeKeyRead:                  "%[1]s: bad array subscript",
		EmptyAssociativeKeyLength:                "[%[1]s]: bad array subscript",
		ArithEmptySubscript:                      "%[1]s[]: bad array subscript",
		ArithEmptySubscriptTarget:                "`%[1]s[]': not a valid identifier",
		// A declaration's operand with a value complains about the
		// *subscript*, in the words the read of one gets; with no value the
		// whole operand is refused as a name, builtin and all. Measured
		// 2026-09-17 on bash 5.3.20, a script file: `typeset 'a[]'=v` is
		// `a[]: bad array subscript` and `typeset 'a[]'` is ``typeset:
		// `a[]': not a valid identifier``, both at 1 with the rest of the
		// line still running.
		DeclarationEmptySubscript:          "%[1]s[]: bad array subscript",
		ValuelessDeclarationEmptySubscript: "%[2]s: `%[1]s[]': not a valid identifier",
		SubscriptedPrefixIsNotAName:        "`%[1]s': not a valid identifier",
		ArithWholeArraySubscript:           "%[1]s[%[2]s]: bad array subscript",
		ArithSubscriptUnclosedQuote:        "%[1]s[%[2]s]: bad array subscript",
		ArrayLiteralThroughASubscript:      "%[1]s[%[2]s]: cannot assign list to array member",
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
		// A conditional missing either of its two values is a sentence of its
		// own here, apart from the ordinary `operand expected`: `$(( 1 ? ))`
		// and `$(( 1 ? 2 : ))` are both `expression expected`.
		ArithConditionalThen:        "expression expected",
		ArithConditionalElse:        "expression expected",
		ArithConditionalColon:       "`:' expected for conditional expression",
		ArithErrorSkipsLeadingSpace: true,
		ArithErrorNamesTheConstruct: true,
		DivisionByZero:              "division by 0",
		ArithNegativeExponent:       "exponent less than 0",

		// bash reserves its generic arithmetic wording for operands that are
		// not literals, so a bad digit gets a reason of its own.
		DigitTooGreatForBase: "value too great for base",
		// A byte that is no digit in any base is a second sentence here:
		// `010#5` and `0#5` are `invalid number` where `08#5` and `1@2` are
		// the one above. See Diagnostics.ArithByteIsNoDigit.
		ArithByteIsNoDigit: "invalid number",
		// And a base outside 2..64 is a third: `$(( 1#0 ))`.
		ArithInvalidBase:         "invalid arithmetic base",
		ArithRecursionLimit:      "expression recursion level exceeded",
		ArithErrorNamesThePrefix: true,
		// set -o pads to fifteen and tabs; kill -l numbers five to a row.
		// The width is named rather than written, because `shopt -o -s`
		// writes this listing narrowed and has to pad it the same way.
		OptionListingWidth:  setOptionListingWidth,
		OptionListingTabbed: true,
		KillListing:         interp.KillListingNumbered,
		TraceQuoting:        interp.QuoteShell,
		// And the widest character set of the three, with a position rule
		// the other two do not have: `~a` and `#a` are quoted and `a~b` and
		// `a#b` are not, so the tilde and the hash count only where they
		// would have started an expansion or a comment. `^` and `!` are
		// quoted anywhere, which is bash alone, and `=` is quoted nowhere,
		// which is also bash alone. bash 3.2.57 agrees on every row, so this
		// is not a version split. Measured 2026-09-12 over 36 words.
		TraceMetacharacters: interp.TraceMetacharacters{
			Anywhere: "*?[]{}^!",
			Leading:  "~#",
		},
		TraceForHeader: interp.TraceForSource,
		// The header as written, once, before the subject is expanded — the
		// same reading the `for` header gets, and from the same field the
		// parser keeps for it.
		TraceCaseHeader: interp.TraceCaseSource,
		// An appending prefix is traced as the value it came to and with no
		// operator — `v=14; v+=5 cmd` is `+ v=145` — where a bare `w+=2`
		// keeps both. This shell alone; see interp/xtraceprefix.go.
		TracePrefixAppendIsTheJoinedValue: true,
		// A condition's operands are *not* quoted here, which is the one
		// place bash's two trace renderings part: `x="a b"; echo "$x"` traces
		// `echo 'a b'` and `[[ $x == y ]]` traces `[[ a b == y ]]`. Measured
		// 2026-09-12 on 5.3.15 and 3.2.57 alike; the zero value says it, and
		// it is written out because the neighboring TraceQuoting says the
		// opposite two lines up.
		TraceConditionQuoting: interp.QuoteNever,
		// The panel's only shell that says how deep the text it is reading
		// came from: `set -x; eval :` traces `+ eval :` and then `++ :`.
		// Measured 2026-09-11 on 5.3.15 and 3.2.57 alike.
		TracePrefixRepeatsAtIndirection: true,
		// An empty *value* is written bare here where an empty argument is
		// written as two quotes — see TraceEmptyAssignmentValueIsBare.
		TraceEmptyAssignmentValueIsBare: true,
		// `export ev=1` is traced twice — `+ export ev=1` and then `+ ev=1`
		// — and `typeset -x tx=1` once, on the same binary in the same run,
		// so it is the command word and not the export attribute that
		// decides. `local` and `declare -x` are one line too. Measured
		// 2026-09-19 on 5.3.20 and 3.2.57 alike; one line per operand, in
		// order, so `export e1=1 e2=2` is three lines. See
		// interp/xtracedeclaration.go.
		TraceRepeatsAScalarOperandAfter: []string{"export", "readonly"},
		// An operand whose value is a parenthesized list goes the other way:
		// it is taken off the command line and written in front, where a
		// scalar stays where it was. `typeset b=(3 4)` is `+ b=('3' '4')` and
		// then `+ typeset b`, measured 2026-09-19 on 5.3.20 and 3.2.57 alike,
		// and every element is quoted whether or not it needs to be — which
		// is what the second field says. See interp/xtracearrayoperand.go.
		TraceDeclarationArrayOperand:        interp.TraceOperandSplitBefore,
		TraceArrayOperandQuotesEveryElement: true,
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
		SubstringRangeError: "%[1]s: %[2]s",
		// And the negative length a list slice refuses names the length
		// alone, with no parameter in front of it: `-1: substring
		// expression < 0`.
		ListSliceNegativeLength: "%[1]s: substring expression < 0",
		ArithOperandExpected:    "arithmetic syntax error: operand expected",
		ArithOperatorExpected:   "arithmetic syntax error in expression",
		ArithBadOperator:        "arithmetic syntax error: invalid arithmetic operator",
		ArithMissingCloseParen:  "missing `)'",
		ArithFailureStatus:      1,
		SyntaxUnexpected:        "syntax error near unexpected token `%[1]s'",
		// And, inside a `$( … )` body, what the shell was still looking
		// for. See Diagnostics.SubstitutionBodyExpecting for the six rows
		// and for the two shapes that get nothing (#3467).
		SubstitutionBodyExpecting: " while looking for matching `%[1]s'",
		// A refused word is echoed as it was written: `"zzz"` keeps its
		// quotes and `$x` is not the name `x`. See UnexpectedWordNaming.
		UnexpectedWordNaming: interp.UnexpectedWordIsSourceText,
		// Inside `[[ ]]` the same token gets two lines and neither is the
		// one above: a sentence about the construct at the `[[`'s line, then
		// a shorter `near` at the token's. Measured on bash 5.3.15 —
		// `[[ -n x` newline `-z "" ]]` names line 1 and then line 2.
		CondSyntaxPreamble: "syntax error in conditional expression: unexpected token `%[1]s'",
		// And the one position it words as a statement about what it was
		// waiting for: a newline behind a term whose shape is not yet
		// settled. Measured 2026-09-18 on 5.3.20 and 3.2 alike (#3627).
		CondTermUndecidedPreamble: "unexpected token `%[1]s', conditional binary operator expected",
		// And a *second* sentence for a token standing where a condition was
		// to begin, with no sentence at all for the closer itself — see
		// CondCommandPreamble, and CondGroupUnclosed for the line each open
		// `(` adds under both (#2909).
		CondCommandPreamble:  "unexpected token `%[1]s' in conditional command",
		CondGroupUnclosed:    "expected `%[1]s'",
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
		// A C-style `for` header with the wrong number of separators, worded
		// by the count: too few is a statement about the expression that was
		// not there, too many blames the separator. Measured 2026-09-12 on
		// 5.3.15, 3.2.57 and the same binary as `sh`, which all agree —
		// `for ((i=0))`, `for (())`, `for ((;))` and `for ((1;2))` give the
		// first, `for ((;;;))` and `for ((1;2;3;4))` the second (#2225).
		ForArithHeader:    "syntax error: arithmetic expression required",
		ForArithSeparator: "syntax error: `;' unexpected",
		// And a second line under either, echoing the header alone rather
		// than the line it was written on — the only failure in this dialect
		// whose echo is not the source line.
		ForArithHeaderEcho: "syntax error: `%[1]s'",
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
		DotCannotOpen: "%[1]s: %[2]s",
		// `. -p list f` whose list missed, which is not the sentence a file
		// that would not open gets: lower case, and naming the builtin.
		DotSearchPathMiss:   ".: %[1]s: file not found",
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
		// And it has a phrase of its own for a file the kernel would not
		// start that turned out not to be shell text either. Measured
		// 2026-09-13, `./elf64_hdr.bin` and `exec ./elf64_hdr.bin` both:
		// `<file>: cannot execute binary file: Exec format error` at 126,
		// where zsh says `exec format error: <file>` and ksh93 says
		// `<file>: cannot execute [Exec format error]` — the strerror alone,
		// arranged by their own CannotExecute.
		BinaryFileReason: "cannot execute binary file: Exec format error",
		// bash names the builtin only when the command was not found at all.
		ExecNotFound: "exec: %[1]s: not found",
		// bash is the only one that says which *kind* of operator it wanted.
		TestUnaryExpected:    "%[2]s: %[1]s: unary operator expected",
		TestBinaryExpected:   "%[2]s: %[1]s: binary operator expected",
		TestIntegerExpected:  "%[2]s: %[1]s: integer expected",
		TestTooManyArguments: "%[2]s: too many arguments",
		TestOperandExpected:  "%[2]s: argument expected",
		TestMissingBracket:   "[: missing `]'",
		// An empty `=~` right operand, which this shell names the construct
		// for and quotes the operand back in — the operand being empty, so
		// the quotes close on nothing. Status 2, the construct's own failure,
		// where zsh calls it a match that did not happen and leaves 1.
		// Measured 2026-09-18, `[[ abc =~ "" ]]` in a script file (#3279).
		EmptyRegexOperand: "[[: invalid regular expression `': empty (sub)expression",
		// A function named after a special builtin, quoted the way this
		// shell quotes a reserved word. See
		// Semantics.SpecialBuiltinNameIsNotAFunctionName, which is the state
		// it is written in (#2987).
		FunctionNameIsASpecialBuiltin: "`%[1]s': is a special builtin",
		// A word this shell's POSIX mode divided one way and the run divides
		// the other, where the `${` no longer closes. It blames the **brace**
		// here and names the word as it was written — a different sentence
		// from the parse-time failure the same text gets under the protecting
		// reading alone, which blames the quote. Measured 2026-09-18 on bash
		// 5.3.20 as `sh`: `"${v-'a}"` expanded after `set +o posix` is
		// ``bad substitution: no closing `}' in "${v-'a}"`` at 1, with the
		// words beside it on the same line expanding normally (#2969).
		SecondReadingBadSubstitution: "bad substitution: no closing `}' in %[1]s",
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
		PrintfBadHexNumber:          "printf: %[1]s: invalid hex number",
		PrintfBadOctalNumber:        "printf: %[1]s: invalid octal number",
		PrintfNumberOutOfRange:      "printf: %[1]s: Result too large",
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
		// The seventeen rows bash 5.3 prints on Linux, which is the whole of
		// its table: a kernel without a limit drops that row and moves
		// nothing else, so the eleven macOS prints are these with `-R`,
		// `-e`, `-i`, `-q`, `-r` and `-x` taken out — measured 2026-09-18 on
		// bash 5.3.20 (aarch64-apple-darwin25) and 5.3.9 in the panel's
		// Alpine image, byte-identical row for row, padding included
		// (#2806).
		//
		// `pipe size` is the platform's own number rather than a limit and
		// is 1 block here and 8 there, which is what makes it a resource
		// read through the hooks instead of a constant in this table. The
		// letter is still read and still cannot be set: `ulimit -p 8` is
		// `ulimit: pipe size: cannot modify limit: Invalid argument` at 1,
		// for every operand including the value the row already holds.
		UlimitListing: []interp.UlimitListingRow{
			{Prefix: "real-time non-blocking time  (microseconds, -R) ", Letter: 'R', Res: interp.ResourceRealtimeTime, Scale: 1},
			{Prefix: "core file size              (blocks, -c) ", Letter: 'c', Res: interp.ResourceCore},
			{Prefix: "data seg size               (kbytes, -d) ", Letter: 'd', Res: interp.ResourceData, Scale: 1024},
			{Prefix: "scheduling priority                 (-e) ", Letter: 'e', Res: interp.ResourceSchedulingPriority, Scale: 1},
			{Prefix: "file size                   (blocks, -f) ", Letter: 'f', Res: interp.ResourceFileSize},
			{Prefix: "pending signals                     (-i) ", Letter: 'i', Res: interp.ResourcePendingSignals, Scale: 1},
			{Prefix: "max locked memory           (kbytes, -l) ", Letter: 'l', Res: interp.ResourceLockedMemory, Scale: 1024},
			{Prefix: "max memory size             (kbytes, -m) ", Letter: 'm', Res: interp.ResourceResidentSet, Scale: 1024},
			{Prefix: "open files                          (-n) ", Letter: 'n', Res: interp.ResourceOpenFiles, Scale: 1},
			{Prefix: "pipe size                (512 bytes, -p) ", Letter: 'p', Res: interp.ResourcePipeBuffer, Scale: 512},
			{Prefix: "POSIX message queues         (bytes, -q) ", Letter: 'q', Res: interp.ResourceMessageQueues, Scale: 1},
			{Prefix: "real-time priority                  (-r) ", Letter: 'r', Res: interp.ResourceRealtimePriority, Scale: 1},
			{Prefix: "stack size                  (kbytes, -s) ", Letter: 's', Res: interp.ResourceStack, Scale: 1024},
			{Prefix: "cpu time                   (seconds, -t) ", Letter: 't', Res: interp.ResourceCPUTime, Scale: 1},
			{Prefix: "max user processes                  (-u) ", Letter: 'u', Res: interp.ResourceProcesses, Scale: 1},
			{Prefix: "virtual memory              (kbytes, -v) ", Letter: 'v', Res: interp.ResourceAddressSpace, Scale: 1024},
			{Prefix: "file locks                          (-x) ", Letter: 'x', Res: interp.ResourceFileLocks, Scale: 1},
		},
		UlimitBadNumber: "ulimit: %[1]s: invalid number",
		// bash names the resource by the same label `ulimit -a` gives it.
		UlimitCannotChange: "ulimit: %[1]s: cannot modify limit: %[3]s",
		BuiltinBadOption:   "%[1]s: %[2]s: invalid option",
		WaitBadJob:         "wait: `%[1]s': not a pid or valid job spec",
		WaitNoSuchJob:      "wait: %[1]s: no such job",
		// `fg` and `bg` in a script, which has no job control: bash refuses
		// before it reads the operand and words it the same for both. Spelled
		// out rather than left to the shared fallback because empty here
		// means silence — see Diagnostics.NoJobControl, which is ksh93's
		// answer.
		NoJobControl:       "%[1]s: no job control",
		AmbiguousJobSpec:   "%[1]s: %[2]s: ambiguous job spec",
		KillNoSuchJob:      "kill: %[1]s: no such job",
		DisownNoCurrentJob: "disown: current: no such job",
		WaitBadJobStatus:   1,
		WaitNotOurChild:    "wait: pid %[1]d is not a child of this shell",
		// A job given up on because it stopped, in the two wordings bash has
		// for it: the bare `wait` names the job and its process, and the one
		// that named a job speaks from inside its own wait (#2227).
		WaitJobStopped:    "wait: warning: job %[1]d[%[2]d] stopped",
		WaitForJobStopped: "warning: wait_for_job: job %[1]d is stopped",
		// The kinds of variable a function cannot be. See
		// interp.Diagnostics.VariableOnlyLettersOnAFunctionLine.
		UnsetFunctionAndVariable: "unset: cannot simultaneously unset a function and a variable",
		DeclareMakesNoFunction:   "%[1]s: cannot use `-f' to make functions",
		VariableOnlyLettersOnAFunctionLine: map[string]string{
			"declare": "aAin",
			"typeset": "aAin",
		},
		UnimplementedOptionLetters: map[string]string{
			// `set` letters bash has and this shell does not: -b job
			// notices, -k assignment-anywhere, -r restricted, -H history
			// expansion, -P physical paths. Measured 2026-09-05 by asking
			// bash 5.3 for every letter of the alphabet in both cases and
			// both signs; the ones missing from here it refuses itself, and
			// those get SetInvalidOptionLetter.
			//
			// Two names have left this sentence without leaving the string,
			// which is the drift #3088 swept for — a comment naming a letter
			// as missing is read the same way the string is. `-B` is gone
			// because brace expansion is built and `set -B` is silently 0;
			// `-p` is gone because it is refused under its *name*, `set:
			// privileged: not implemented`, and never reaches this table.
			//
			// `-H` left it when the expander was built (#3093): the letter
			// and `set -o histexpand` are one request, so leaving it here
			// while the name was wired would refuse what the name grants —
			// the same pairing `-t` describes below.
			//
			// `-t` left this list when the option behind it was built. The
			// two tables are one table: the letter and `set -o onecmd` are
			// the same request, so a letter listed here while the name is
			// wired would refuse what the name grants — see
			// Semantics.SetHasTheTLetter.
			"set": "brP",
			// Options these builtins have here and this shell does not.
			"wait": "f",
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
			// `local`'s function letters, which this shell takes and ignores
			// where no operand is a function. `-n` left this list when name
			// references landed (#2553), `-I` when inheritance did (#3434)
			// and `-t` when the trace attribute did (#3101); the two tables
			// are one table, so a letter named here while DeclareOptions
			// spells it would refuse what the attribute grants. `declare`
			// and `typeset` have left the table entirely with the last of
			// them.
			"local": "fF",
		},
		// bash's own words for the two -u failures it can meet here; the
		// non-number wordings per letter are not modeled yet, so those fall
		// back to the substrate's.
		ReadBadFileDescriptor: "read: %[1]s: invalid file descriptor: Bad file descriptor",
		ReadBadTimeout:        "read: %[1]s: invalid timeout specification",
		ReadBadDescriptorSpec: "read: %[1]s: invalid file descriptor specification",
		// 128 plus SIGALRM, the signal a timeout is.
		ReadTimeoutStatus: 142,
		HereDocumentAtEOF: "warning: here-document at line %[1]d " +
			"delimited by end-of-file (wanted `%[2]s')",
		// And what it says when it feeds one of those from the lines after
		// the enclosing command instead, which is the `$( )` and `<( )`
		// spellings only — measured 2026-09-20, one document and two (#3711).
		HeredocCarriedOutOfSubstitution: "warning: command substitution: " +
			"%[1]d unterminated here-document%[2]s",
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
		// The three sentences a frozen function has. Measured 2026-09-16 on
		// 5.3.20 and 3.2.57 alike, each at status 1 and none of them fatal:
		//
		//	readonly -f nosuchfn    readonly: nosuchfn: not a function
		//	b() { :; }              b: readonly function
		//	unset -f b              unset: b: cannot unset: readonly function
		//
		// The second carries the shell's ordinary location prefix and names
		// no builtin, because no builtin was written — it is a definition
		// that was refused.
		ReadonlyNotAFunction:      "readonly: %[1]s: not a function",
		ReadonlyFunctionRedefined: "%[1]s: readonly function",
		UnsetReadonlyFunction:     "unset: %[1]s: cannot unset: readonly function",
		// `declare -p nosuch` — the name it was invoked by is in front,
		// which declarePrint writes, so the wording carries only the rest.
		DeclareNoSuchVariable: "%[1]s: not found",
		// The three sentences a name reference has. Measured 2026-09-15 on
		// 5.3.15, `env -i` with a scratch HOME:
		//
		//	declare -n r=1bad    declare: `1bad': invalid variable name
		//	                     for name reference                    st 1
		//	declare -n r=r       declare: r: nameref variable self
		//	                     references not allowed                st 1
		//	declare -n a=b       silent 0, and `echo "$a"` then writes
		//	declare -n b=a       warning: a: circular name reference
		//
		// The target is quoted back with bash's own `'` pair, the way every
		// bad-name refusal on this builtin quotes one — see BuiltinBadName,
		// which is the sentence this replaces for the `n` letter alone.
		NamerefBadTarget:     "`%[1]s': invalid variable name for name reference",
		NamerefSelfReference: "%[1]s: nameref variable self references not allowed",
		// The same sentence ksh93 writes, and the only one the two shells
		// share on this letter — carried here so the dialect says which
		// wording it means rather than leaning on a fallback.
		NamerefCannotBeAnArray: "%[1]s: reference variable cannot be an array",
		// Spoken as the shell rather than as the builtin, measured: the line
		// is `bash: line 1: warning: a: circular name reference` with no
		// `declare:` in it, where the two refusals above carry the name.
		NamerefCircularWarning:               "warning: %[1]s: circular name reference",
		NamerefArrayLiteralDropsTheAttribute: "warning: %[1]s: removing nameref attribute",
		// And the write through one, which bash reports as a depth rather
		// than as a circle: `f() { local -n r=r; r=SET; }` writes `warning:
		// r: maximum nameref depth (8) exceeded` and the value reaches the
		// global cell. Measured 2026-09-15 on 5.3.20 (#3048).
		NamerefDepthWarning: "warning: %[1]s: maximum nameref depth (8) exceeded",
		// The second sentence a refused `{name}>` store writes, after
		// whatever the store itself said: measured 2026-09-17, `declare -n
		// s; exec {s}>/dev/null` is ``exec: `10': not a valid identifier``
		// and then this, at 1, and `readonly s=1` in front of the same line
		// is `s: readonly variable` and then this. No other column was asked:
		// zsh refuses `typeset -n` and ksh93 has no nameref of this spelling.
		CannotAssignFdToVariable: "%[1]s: cannot assign fd to variable",
		TrapPrintsSignalPrefix:   "SIG",
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
			// And `getopts`, whose name operand says the same — and says it
			// to `o[1]` as well, in a shell that fills `read 'a[1]'`: this
			// builtin does not take a subscript here. Measured 2026-09-18
			// (#3555).
			"getopts": "%[1]s: `%[2]s': not a valid identifier",
			// And `printf -v 1bad`, which is the same sentence under this
			// builtin's name at this builtin's own status — measured
			// 2026-09-17, ``printf: `1x': not a valid identifier`` at 2.
			// Named rather than left to the default for the reason the two
			// declaration builtins above are: the table is meant to be the
			// whole answer for every builtin that asks it.
			"printf": "%[1]s: `%[2]s': not a valid identifier",
		},
		// `printf` counts the identical refusal as a usage error where `read`
		// counts it as a failure: measured 2026-09-17, a script file,
		// `printf -v '1x' %s Q` and `printf -v 'a[]' X` are both 2 with the
		// rest of the line still running, where `read '1x'` and `read 'a[]'`
		// are 1. See Diagnostics.BuiltinBadNameStatusFor.
		BuiltinBadNameStatusFor:  map[string]int{"printf": 2},
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
		AliasInvalidName:       "%[1]s: `%[2]s': invalid alias name",
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
		// $TIMEFORMAT, read every time the keyword runs. The wording names
		// the variable as well as the offending character, where ksh93
		// names only the character.
		TimeFormatVariable:     "TIMEFORMAT",
		TimeFormatBadDirective: "%[1]s: `%[2]s': invalid format character",
		// A path that is not there is the OS reason and does not name the
		// builtin; a bare name off PATH does the reverse.
		PathNotFound: "%[1]s: No such file or directory",
		// bash names the path it tried, absolute, where the other three
		// report the operand as written.
		NamesResolvedPath: true,
		// `shopt -s failglob` refusing a pattern that matched nothing.
		// Three words where zsh's is four, measured 2026-09-13 on bash
		// 5.3.15: `no match: nosuch*` against zsh's `no matches found:
		// nosuch*`. Same event, same position prefix, different sentence,
		// which is why the substrate carries neither.
		GlobNoMatch: "no match: %s",
		// A bare `hash` announces the table, on standard output.
		HashEmptyTable: "hash: hash table empty",
		// The `hits<TAB>command` table, which is bash's alone: it is the only
		// column that keeps a count to print. See interp.HashListingForm.
		HashListing:  interp.HashListingHitsAndPath,
		HashDisabled: "hash: hashing disabled",
		// Two lines, which is bash rather than a mistake: it prints the
		// complaint and then a usage line, and only the first carries the
		// shell's own prefix.
		DotNoOperand: "%[1]s: filename argument required\n" +
			"%[1]s: usage: %[1]s [-p path] filename [arguments]",
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
		// `emacs` and `nolog` moved out of the substrate's common table when
		// the ash column was asked for them and had neither (#3366). Both
		// are in this shell's own `set -o` listing, measured on bash 5.3.20.
		"emacs",
		"errtrace",
		"nolog",
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
	// And the listing for it, on the same terms the call-stack arrays are on:
	// `declare -p PIPESTATUS` is `declare -a PIPESTATUS=([0]="0")` in bash
	// 5.3.20 and was `PIPESTATUS: not found` here (#3099).
	r.SetDynamicDeclaration("PIPESTATUS", interp.ProducedDeclaration{Array: true, ListsItsElements: true})
	// What the last `=~` captured — the whole match, then the groups. The
	// core keeps the record and this names it; ksh93 and zsh keep their
	// captures under names and shapes of their own, never this one.
	r.SetRegexMatch("BASH_REMATCH")
	// The long names of the options that are on, as a readonly produced
	// variable bound to the option state in both directions. The core keeps
	// the state and this names it; the other three leave the name an ordinary
	// string and read nothing out of it at startup, which is measured.
	r.SetShellOptions("SHELLOPTS")
	// And the same binding for this shell's *other* option namespace, which
	// the core does not have: the `shopt` names that are on, produced and
	// readonly, read back out of the environment at startup. bash 3.2 has no
	// such variable, so this is bash 5's answer — see bashOptions.
	r.SetOptionList("BASHOPTS", bashOptions, applyInheritedBashOptions)
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
	// And the attributes this shell's own parameters carry, which were
	// missing from every one of them — see markOwnParameterAttributes, and
	// note that the readonly half changes an answer rather than a listing.
	markOwnParameterAttributes(r)
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
	// The history list and the two files it is kept in. A script has one —
	// bash maintains a list and writes a history file with no terminal
	// anywhere — so this is a builtin rather than something the prompt owns.
	// See history.go.
	registerHistory(r)
	// Stopping the shell itself, which is two halves: a refusal a script sees
	// and a stop only a binary that owns the process may make. See
	// suspend.go, and interp.Runner.StopThisProcess for the split.
	registerSuspend(r)
	// A function carried to a child through the environment, under the name
	// bash gives it. The other three do not carry functions at all.
	r.SetFunctionExport("BASH_FUNC_", "%%")
	// And how they are laid out, which is this shell's taste rather than
	// anything the printer should know.
	shown, exported := FunctionLayout(), ExportedFunctionLayout()
	// The one part of the arrangement that needs the runner: a `$'…'` is
	// written back as the characters it stands for, and what an escape comes
	// to is this shell's answer rather than the printer's (#3196).
	shown.AnsiCQuotedWordIsItsValue = r.AnsiCValue
	exported.AnsiCQuotedWordIsItsValue = r.AnsiCValue
	r.SetFunctionLayout(shown, exported)
	// And how a whole script is written back, which is that same arrangement
	// with a top level and one header spelling apart. The decoder is attached
	// here for the reason it is above: the escapes are this shell's answer.
	listed := ScriptListingLayout()
	listed.AnsiCQuotedWordIsItsValue = r.AnsiCValue
	r.SetScriptListingLayout(listed)
	// The command the shell is running, which a DEBUG action reads to find
	// out which one it fired for. See bashcommand.go.
	registerRunningCommand(r)
	r.SetDynamic("RANDOM", func(rr *interp.Runner) string { return rr.Randoms() })
	// And an assignment seeds it, which is what makes a script that uses
	// `RANDOM` reproducible: measured 2026-09-14, `RANDOM=42` twice in one
	// shell gives the same pair of numbers both times here, in ksh93u+ and
	// in zsh 5.9.2. Without the writer the assignment was heard and stored
	// for the producer to find, and the producer had no state to find it
	// with (#2827).
	r.SetDynamicWriter("RANDOM", func(rr *interp.Runner, value string) { rr.SeedRandoms(value) })
	// How the two of them list back, which a produced parameter has to be
	// told rather than carry: `declare -p RANDOM` is
	// `declare -i RANDOM="16735"` in bash 5.3 and was `RANDOM: not found`
	// here, from the name this shell had just expanded a number for (#2451).
	r.SetDynamicDeclaration("RANDOM", interp.ProducedDeclaration{Integer: true})
	r.SetDynamic("SECONDS", func(rr *interp.Runner) string {
		return strconv.Itoa(int(rr.SecondsFrom()))
	})
	// `-i`, but only once something has expanded it: the operand-less listing
	// writes `declare -- SECONDS` in a shell that has never read the
	// parameter and `declare -i SECONDS="0"` after a plain `: $SECONDS`. It
	// is the one name of the seven that moves, and the *named*
	// `typeset -p SECONDS` writes the letter in both states (#2722).
	r.SetDynamicDeclaration("SECONDS", interp.ProducedDeclaration{
		Integer: true, IntegerOnceRead: true,
	})
	// And the line, which this shell lists with *no* attribute where bash
	// 3.2 writes `-i` — `declare -- LINENO="1"`, measured on 5.3.15. It is
	// registered by the core rather than here, so this is a declaration for
	// a producer the dialect did not install, which the seam allows for
	// exactly this: the letters are the dialect's fact and the producer is
	// not (#2451).
	r.SetDynamicDeclaration("LINENO", interp.ProducedDeclaration{})
	// The four parameters of this shell's own that were simply absent, and
	// which are not the core's to supply — see shellparameters.go (#3098).
	registerShellParameters(r)
	// And the directory stack as a parameter, which is a view onto what the
	// prelude's `pushd` keeps rather than a store of its own.
	registerDirectoryStack(r)
	// The clock, as this shell reads it since 5.0. In a file of its own
	// because it is not the `zsh/datetime` registration under another name —
	// see epoch.go for the row-by-row measurement (#1158).
	registerEpochClock(r)
	// The alias table written as an association, readable and writable.
	registerBashAliases(r)
	registerBashCmds(r)
	if dot, ok := r.Builtin("."); ok {
		r.Register("source", dot)
	}
	// The builtin this shell alone answers to; the other three say "command
	// not found", so it is registered here rather than taken away there.
	r.Register("shopt", biShopt)
	registerLogout(r)
	// And the same table reached from the command line that started the
	// shell, where there is no builtin to run: `bash -O checkhash` moves one
	// of these names before the first line of the script. See shopt.go, and
	// Semantics.ShellOptionInvocationLetter for the letter the front end
	// reads (#3264).
	r.SetShellOptionNamespace(shoptMoveAtInvocation, shoptListAll)
	// This shell's only self-documenting builtin, and there is no other name
	// for it — a script reaching for it used to get 127. See help.go, which
	// carries what is answered and what is deliberately refused.
	r.Register("help", biHelp)
	// `**` with nothing behind it crosses levels here too, so `d/**` reaches
	// every one of them. Set unconditionally rather than through `globstar`,
	// because the walk asks it only where `globstar` has already said `**`
	// crosses at all — and `globstar` is the only name this shell has for
	// either half. Measured 2026-09-07: `shopt -s globstar; echo **` in a
	// directory holding `ax`, `bx` and `cx/dx/ax` lists every level here and
	// answers `ax bx cx` in zsh (#1339).
	r.SetMatchOption(interp.StarStarAloneCrossesDirectories, true)
	// A symbolic link to a directory is one of the levels `**/` lists here,
	// even though the walk never goes inside one. Set for the same reason as
	// the line above: `globstar` is the only name this shell has, and the
	// walk asks this only where that option has already said `**` crosses.
	// Measured 2026-09-12 against bash 5.3.15 in a directory holding `r/x`
	// and a symlink `s` to `r`: `shopt -s globstar; echo **/` is `r/ s/`
	// here and `r/` in zsh, while `echo **/x` is `r/x` in both — the link is
	// named and not entered (#2360).
	r.SetMatchOption(interp.StarStarSeesLinkedDirectories, true)
	// And a run of `**` components is one component here, which is the third
	// question the same option name has to answer for. Without it the run is
	// a cross product and every name in it comes back once per way of
	// splitting it, because nothing anywhere takes duplicates out of a
	// pathname expansion. Measured 2026-09-13 in a tree of directories `a`
	// and `b` nested three deep: `shopt -s globstar; echo **/**/` is 14
	// names here and 48 in zsh, where `a/a/a/` appears four times (#2298).
	r.SetMatchOption(interp.RepeatedStarStarIsOneComponent, true)
	// A `**` that matched zero levels is the directory the walk stood in, so
	// `d/**` names `d/` ahead of what is under it. Set for the same reason as
	// the three above — `globstar` is the only name this shell has for any of
	// them — and it is the fourth question that one name has to answer.
	// Measured 2026-09-16 against ksh93u+, which is the only column that
	// reads it the other way: `printf '[%s]' d/**` is `[d/][d/e][d/e/f]`
	// here and `[d/e][d/e/f]` there (#3152).
	r.SetMatchOption(interp.StarStarZeroLevelIsTheDirectoryItStartsFrom, true)
	// And a `**` may begin inside a directory the pattern reached through a
	// symbolic link, which is where the walk starts rather than where it may
	// go: this shell never follows a link it meets on the way down. Measured
	// 2026-09-16 with `s` a symlink to `r` and `r/x` under it: `s/**` is
	// `[s/][s/x]` here and in zsh, and no match at all in ksh93 (#3152).
	r.SetMatchOption(interp.StarStarPatternsReadLinkedDirectories, true)
	// And a component behind a `**` looks inside a level the walk listed and
	// never entered, which the two other columns with the construct do not:
	// the walk stays bounded and only the set handed to the next component
	// is wider. Measured 2026-09-18 with `r/x`, a symlink `s` to `r` and a
	// second symlink `a/b/sl` to `r`: `./**/x` is `[./a/b/sl/x][./r/x][./s/x]`
	// here, `[./r/x]` in ksh93 and in zsh — and this shell names the linked
	// level for `**/` exactly as ksh93 does, which is what makes the two
	// separate questions. The walk that begins the word is this shell's own
	// exemption and the option records it: `**/x` is `[r/x]` here while the
	// same pattern with a `.` or an absolute root in front of it is not
	// (#3176).
	r.SetMatchOption(interp.ComponentBehindStarStarSeesLinkedLevels, true)
	// An `&` in a `${v/pat/rep}` replacement is the text the pattern
	// matched, which is this shell alone in the panel — bash 3.2, ksh93 and
	// zsh all answer `a[&]c` for `v=abc; ${v/b/[&]}` where this one answers
	// `a[b]c`. It is on with nothing said and `shopt -u patsub_replacement`
	// turns it off, so the default belongs here and the name belongs in
	// shopt.go (#1862).
	r.SetMatchOption(interp.ReplacementAmpersandIsTheMatch, true)
	// And the other default this preset holds the other way round from the
	// core: `shift` past the end is **silent** here, where dash, ksh93 and
	// zsh each write their own sentence. The wording is in this dialect's
	// Diagnostics all the same, because it is the wording
	// `shopt -s shift_verbose` asks for — so the option turns a capability
	// on rather than installing a sentence, and the listing reads the
	// capability back (#3465).
	r.SetReportsShiftPastTheEnd(false)
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
