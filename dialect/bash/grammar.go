// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"github.com/blairham/sh/syntax"
)

// Dialect is what bash 5 parses.
func Dialect() syntax.Dialect {
	d := syntax.Core()
	// A process substitution written in a subscript is held as its laid-out
	// text — see syntax.Dialect.ProcSubstInSubscriptIsReprinted. Through the
	// function listing's own reader, asked when a body is met rather than
	// built here, since that reader parses with this very dialect.
	d.ProcSubstInSubscriptIsReprinted = procSubstBodyLaidOut
	// A NUL byte in the program's text is not there. See the flag.
	d.SourceDropsNulBytes = true
	// `$[expr]`, the older spelling of `$((expr))`. Measured
	// 2026-09-06: `echo $[1+1]` is 2 here and in the 3.2 macOS ships,
	// and the same text is the literal `$[1+1]` in ksh93 and dash.
	// bash has documented it as deprecated for years and both builds
	// in the panel still take it (#900).
	d.DollarBracketArith = true
	// `time -- cmd`, where the `--` is consumed and selects the POSIX report on
	// its own, and `time time cmd`, which reports once. Both measured
	// 2026-09-23 against a `TIMEFORMAT` the POSIX layout cannot produce, so the
	// two reports are told apart — see the two flags for the rows (#4161).
	d.TimeIgnoresADoubleDash = true
	d.TimeFoldsARepeatedKeyword = true
	// Sixteen here-documents to a command, and the seventeenth is refused.
	// Measured 2026-09-23 on 5.3.20 and on the 5.3.15 in the image this shell's
	// own suite is graded in: sixteen `<<EOF` on one `cat` runs, seventeen is
	// `maximum here-document count exceeded` at the command's line and the input
	// ends at 2, and two commands of sixteen each in one file both run. dash,
	// ksh93 and zsh have no bound at all. It is CVE-2014-7186's bound, and that
	// CVE's own regression test is where this was found (#4143).
	d.HeredocMax = 16
	// `++` and `--` are operators here only against something that can be
	// assigned to. Measured 2026-09-22: `$(( ++7 ))` is **7** — two signs —
	// and `$(( 7++ ))` is `operand expected` blamed on `+ `, where ksh93 and
	// zsh take the operator and then refuse the target. See
	// syntax.Dialect.ArithIncDecNeedsAPlace (#2420).
	d.ArithIncDecNeedsAPlace = true
	// Inside a `"` run written in an operand that itself stands in double
	// quotes, a backslash escapes whatever follows it: with `u` unset,
	// `"${u-"A\pB"}"` is `ApB` here and `A\pB` in dash, zsh and BusyBox ash.
	// Measured 2026-09-22 in 5.3.20, under the `sh` name and in 3.2.57 alike,
	// so it is not a version line. See
	// syntax.Dialect.NestedQuoteInAQuotedOperandEscapesAnything.
	d.NestedQuoteInAQuotedOperandEscapesAnything = true
	// A `$( … )` body is parsed while the line that holds it is read, so a
	// body that will not parse refuses the line before any of it runs — and
	// refuses it even where the substitution is in a branch nothing takes.
	// The older spelling is not: `` v=`if` `` is read when the word is
	// expanded here. Measured 2026-09-19; 3.2 reads neither with the line,
	// so this is a version line inside one lineage exactly as
	// ArithDoubleQuote is. See syntax.Dialect.SubstitutionBodyRead (#2857).
	d.SubstitutionBodyRead = syntax.NewerSubstitutionBodyReadWithItsLine
	// And a body it refuses settles the read: nothing after the substitution
	// is reached, so no closing parenthesis is looked for. See
	// syntax.Dialect.SubstitutionBodyRefusalEndsTheRead for the panel
	// (#4134).
	d.SubstitutionBodyRefusalEndsTheRead = true
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
	// Where an operator belonged, a byte that begins no token is a missing
	// operand behind a group and a bad operator anywhere else, and the reader
	// has already taken the token behind an operand standing there. Measured
	// 2026-10-04 on 5.3.20; see docs/spec/arithmetic.md.
	d.ArithBadByteAfterAGroupWantsAnOperand = true
	d.ArithLeftoverOperandReadsTheNextToken = true
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
		// The sixth, and the one that declares nothing: `eval foo=(1 2)` is
		// `done` at 0 in bash 5.3.20 and in 3.2.57, and a syntax error in
		// ksh93u+ and dash 0.5.12. Measured 2026-09-20 from script files
		// under `env -i PATH=/usr/bin:/bin LC_ALL=C`, alongside `echo`,
		// `command`, `printf`, `:`, `set`, `unset`, a function and
		// `/bin/echo`, every one of which is a syntax error in bash too — so
		// the reading belongs to a list of words and not to builtins as a
		// class. What the word does with the operand is the other half, and
		// it is registered on the runner: see interp.Runner.RejoinArrayOperand.
		"eval": true,
		// The seventh, and the second that declares nothing: `let x=(2 + 3)`
		// leaves `x` at 5 in bash 5.3.20 and in 3.2.57, where ksh93u+, zsh
		// 5.9.2 and dash 0.5.12 each refuse the parenthesis while reading the
		// line. Measured 2026-09-21 from script files under `env -i
		// PATH=/usr/bin:/bin LC_ALL=C`. The refusal is a **parse** failure, so
		// a `let` line written this way used to cost every later line of the
		// file rather than one answer (#2298). `command let x=(1)` is a
		// syntax error in bash too, which is the list saying it is about the
		// word and not about what the word resolves to.
		"let": true,
	}
	// A compound assignment's word does not end at its `)` here: `declare
	// a=(1 2)x` is one word and a scalar, where ksh93 and zsh store the
	// array and then look for a command called `x`. See
	// [syntax.Dialect.CompoundAssignmentWordRunsPastItsParenthesis], which
	// carries the panel.
	d.CompoundAssignmentWordRunsPastItsParenthesis = true
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
	// And the prefix spelling, which this column and ksh93 have and the
	// third reader of `${!x}` has not — see
	// syntax.Dialect.ParamIndirectionPrefixListing.
	d.ParamIndirectionPrefixListing = true
	// And where that indirection begins: the specials bash indirects *through*
	// — `${!#}` is `$3` and `${!@}` complains about the value of `$@` rather
	// than about a name — plus a digit, which is an ordinary positional here,
	// and the `[` that opens `${!x[@]}`. Every other operator character ends
	// the name, so `${!%}` and `${!,}` are `$!` with an operator behind them.
	// See syntax.Dialect.ParamBangNameContinues for the seven columns measured
	// a character at a time (#3966).
	d.ParamBangNameContinues = "0123456789#?@*["
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
	// See syntax.Dialect.HeredocLeadingContinuationStripsTheNextLine, measured 2026-10-03.
	d.HeredocLeadingContinuationStripsTheNextLine = true
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
	// And a name may hold an `=` where the word is not an assignment:
	// `1x=() { :; }` defines `1x=`. See the field (#5719).
	d.FunctionNameMayHoldAnEquals = true
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
	d.NameReferenceTest = true
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
	// An array literal that never closes is an unfinished bracket here
	// rather than a refused token: `a=(` is `unexpected EOF while looking
	// for matching `)'` at the parenthesis's own line, where this shell used
	// to answer `line 2: syntax error: unexpected end of file`. See
	// syntax.Dialect.ArrayLiteralRunningOutIsUnmatched for the panel —
	// ksh93 answers the other way and was already right.
	d.ArrayLiteralRunningOutIsUnmatched = true
	d.CoprocName = true
	// And may begin any element of a pipeline, where zsh refuses it after a
	// bar. Measured 2026-10-01; see syntax.Dialect.CoprocAfterABar.
	d.CoprocAfterABar = true
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
