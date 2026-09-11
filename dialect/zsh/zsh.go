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
	// `$[expr]`, zsh's other arithmetic spelling and the older one.
	// Measured 2026-09-06: `echo $[1+1]` is 2, and the same text is
	// the literal `$[1+1]` in ksh93 and dash. Reading it as a glob is
	// what made the failure `no matches found`, which points a
	// person at globbing rather than at arithmetic (#900).
	d.DollarBracketArith = true
	// `exec {1}>&-` closes the descriptor a *positional parameter* holds,
	// which is how a prompt theme's scheduler closes the one it was handed.
	// zsh alone — see the flag for what bash and ksh93 answer instead.
	d.FdVariablePositional = true
	// zsh does not expand under `-c` even with the option set.
	// Measured 2026-09-05: `zsh -c 'alias hi=...; hi'` does not expand and
	// the same two lines in a file, or on standard input, do. The route is
	// the whole of the difference — nothing about the shell changes.
	d.ExpandAliases = syntax.RouteFromScriptFile | syntax.RouteOnStandardInput
	// And a body's newlines are lines of the program, as they are in the two
	// that expand by every route.
	d.AliasBodyCountsLines = true
	// zsh has all five, like bash.
	d.DeclarationUtilities = map[string]bool{
		"declare": true, "typeset": true, "local": true,
		"export": true, "readonly": true,
	}
	// `nocorrect` is a reserved word here and in no other panel shell:
	// measured 2026-09-08, `whence -w nocorrect` is `reserved` where
	// `noglob` beside it is `builtin`, and the two behave accordingly —
	// `nocorrect x=1 echo hi` prints `hi` because the word was gone before
	// the assignment prefix was read, where `noglob x=1 echo a[b]c` is
	// `command not found: x=1`. Off in the other four, where the word is an
	// ordinary command name.
	d.ReservedPrecommands = map[string]bool{"nocorrect": true}
	d.FunctionKeywordParens = true
	// `()` is one token to this lexer, which shows in the one place a
	// diagnostic names the last token it read: `f()` with the input running
	// out reports as parse error near `()' rather than naming the closing
	// paren on its own.
	d.EmptyParensAreOneToken = true
	// When constructs nest and the input runs out inside them, this shell
	// names the *enclosing* one where the other three name the innermost:
	// `echo "$( echo hi` is `unmatched "` here, about a quote the script
	// did write, and `unexpected EOF while looking for matching )` in bash.
	// Measured with the nesting both ways round — see the flag (#1151).
	d.UnmatchedBlamesTheOutermost = true
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
	// `{ … } always { … }`, the try-always block. Measured 2026-09-07: the
	// word is positional and not reserved here — `always` alone is `command
	// not found`, `always() { :; }` defines a function and `echo always`
	// prints it — so what the flag adds is one production hanging off a brace
	// group. Nine files in a real `~/.zi` plugin tree are unparseable without
	// it, including zsh-autosuggestions, powerlevel10k's gitstatus and F-Sy-H
	// (#1216).
	d.TryAlways = true
	d.AnonymousFunction = true
	// The same reach: a body may have nothing in it — `{ }`, `( )`, `while
	// cond; do done`, and a condition too. Every shape, and this shell alone.
	d.EmptyCompoundBody = true
	// The same reach again, one level out: an and-or list may end with its
	// operator, so `{ : || ⏎ }` is `{ : ⏎ }`. Measured 2026-09-07 in every
	// closing context, and this shell alone — the other four name the closer
	// they met. `~/.zi/bin/lib/zsh/install.zsh` line 2048 ends with `|| \`
	// and the line after it closes the block, so a shell that refuses this
	// cannot read the file.
	d.OpenEndedAndOr = true
	// A `;` written where a command belongs is stepped over, as many as are
	// written and anywhere — `; b`, `a ; ; b`, `a & ; b`, `a && ; b` and
	// `a | ; b` all run here. The `;` is absorbed rather than standing in for
	// anything: `false || ; echo two` prints `two` and `true || ; echo two`
	// prints nothing, so the command after it really is the operator's
	// right-hand side.
	d.SeparatorWhereACommandBelongs = syntax.AnySeparatorWhereACommandBelongs
	// Floating point, which POSIX has not and these two do.
	d.ArithFloat = true
	// A double quote inside an arithmetic expression is stepped over
	// wherever a token may begin. Measured 2026-09-10 on zsh 5.9.2: with
	// `n=5`, `$(( "1" + 1 ))` is 2, `$(( "n" + 1 ))` is 6 and
	// `$(( 1 + "2" ))` is 3, while `$(( 1"0" ))` is `operator expected` —
	// the quote ends the number here as it does in ksh93, and does not
	// vanish from the text as it does in bash 5.3.
	//
	// The single quote is the opposite answer and is the row below: it is
	// refused outright here, where ksh93 reads it as a character constant
	// (#1223).
	d.ArithDoubleQuote = syntax.ArithDoubleQuoteSkipped
	// The bytes this shell's arithmetic reader refuses as part of no token at
	// all, so they are reported at the byte — `illegal character: @` — rather
	// than as a value it could not read. Measured 2026-09-07 against zsh
	// 5.9.2 by trying every ASCII punctuation byte in five positions: every
	// other one is either a math token here (`% & * = | : , < > / ? #`) or
	// can begin a value (`$` is the process id, `?` the last status, `#` the
	// character-code operator).
	//
	// Only where the expression could have stopped, which the parser decides:
	// `$((1+@))` gets the operand sentence from the same byte because an
	// operator has just been consumed. #698 has the whole table.
	//
	// `\` and `]` are in the table although `$(( ))` cannot put them in front
	// of the reader — a backslash escapes the closing paren, and `$((]))` is
	// re-read as `$( (]) )` — because `let` can, and there they answer the
	// same way: `let $'\134'` and `let 'a[1]]'` are both `illegal character`.
	// A backquote is refused by that reader too and is *not* here: nothing in
	// this shell reaches the reader with one, so it would be a row no
	// measurement of ours could hold honest.
	//
	// The control bytes are also not here. That shell refuses every one but
	// tab and newline, and renders them in caret notation — `illegal
	// character: ^A` — which is a second change: the caret reaches the
	// operand sentence as well, so ``operand expected at `^A'`` would have to
	// move with it. Recorded rather than half-done.
	d.ArithBytesRefusedOutright = "'@;{}\\]"
	// A bare `(a|b)` inside a pattern word, which makes `@(abc|xyz)` a
	// literal `@` followed by a group here rather than an extended pattern.
	d.PatternAlternation = true
	// And a `|` outside every group, which is the same alternation one level
	// out and is reachable only from a value: the written spelling is a parse
	// error here as it is everywhere else, so `L='a|b'; [[ a = ${~L} ]]` is
	// the only way to say it. Measured on zsh 5.9.2 in a condition, in a
	// `case` arm, under `setopt globsubst` and in a glob alike (#1497).
	d.PatternTopLevelAlternation = true
	// The same alternation reaching the `case` arm's own pattern list, where
	// one of the alternatives may be written as nothing: `(|https|git|ftp)`
	// matches one of those schemes or none at all. `~/.zi/bin/lib/zsh/install.zsh`
	// is written that way and the other four shells refuse the line.
	d.CasePatternMayBeEmpty = true
	// `;|` is this shell's spelling of bash's `;;&`: the arm runs and the
	// later patterns keep being tested. The two are mutually exclusive —
	// `;;&` is ``parse error near `&'`` here, which CaseContinue staying off
	// already gives us — so this is a flag of its own rather than a second
	// value of that one. `~/.zi/plugins/romkatv---powerlevel10k/gitstatus/mbuild`
	// is written with it and was the last file in that tree we could not
	// parse.
	d.CaseContinuePipe = true
	// And the same `|` from the other side: this shell reads a parenthesized
	// pattern list as one alternation word, so a newline in it is a character
	// of the pattern. `case a in (a|` newline `b)` runs the arm here for
	// subject `a`, and its second alternative is the two characters newline
	// and `b`; every other shell in the panel refuses the line.
	d.CasePatternListSpansNewlines = true
	// And a blank in the same place is a character of the pattern too, so
	// `(a b)` is one three-character pattern: `case 'a b' in (a b)` matches
	// here and is a syntax error in the other five. The paren is what
	// licenses it — without one this shell refuses the line as well — and the
	// blanks are verbatim, so `(a b)` misses `a  b`. Only where the pattern
	// continues, which is what keeps `(a | b)` two alternatives.
	//
	// Line 234 of `VCS_INFO_get_data_git` is `(''(x|exec) *)` — a group, a
	// blank, more pattern — so without this the git segment of every prompt
	// drawing one fails to autoload (#1744). See
	// syntax.Dialect.CasePatternListSpansBlanks.
	d.CasePatternListSpansBlanks = true
	// `<->` is a number and `<1-9>` a bounded one, where every other panel
	// shell reads the `<` as a redirection. Measured 2026-09-05 on zsh
	// 5.9.2: `[[ 1 = <-> ]]` is 0 here and a syntax error in bash 5.3, bash
	// 3.2, bash-as-sh and ksh93, and dash — which has no `[[ ]]` — tries to
	// open a file called `-`.
	d.NumericRangePattern = true
	// `=(cmd)` runs cmd to completion, writes its output into a regular file
	// and expands to that file's path — process substitution with a file
	// where the other two spellings have a pipe, which is what makes `diff
	// =(a) =(b)` seek and `vi =(cmd)` open. Measured 2026-09-11 on zsh
	// 5.9.2: `echo =(echo hi)` writes a path under `$TMPPREFIX` and the
	// other five columns refuse the `(`. See
	// syntax.Dialect.ProcessSubstitutionToFile for where it may stand.
	d.ProcessSubstitutionToFile = true
	// `[[ -v name ]]`, which asks whether a parameter is set. Measured on zsh
	// 5.9.2, which reads more kinds of name through it than the other two that
	// have it; see interp.Semantics.ParameterIsSetSeesSpecials.
	d.ParameterIsSetTest = true
	// A function definition's name is a *word*, so an expansion in one names
	// the function the expansion produces: `w=foo; _p_${w}() { … }` defines
	// `_p_foo`. This shell's alone — the other five refuse both spellings,
	// and say so as a complaint about a name rather than about the grammar.
	// It is how a plugin generates one function per widget.
	d.FunctionNameExpands = true
	// One body under several names: `function clipcopy clippaste { … }`,
	// where `$0` in the body is the name that was called. zsh alone —
	// syntax.Dialect.FunctionMultipleNames has the six answers, and the
	// construct is what an Oh-My-Zsh clipboard library ends with (#1680).
	d.FunctionMultipleNames = true
	// And a name list with no body after it, plus the `;` that may stand
	// between the names and a body it does have — see
	// syntax.Dialect.FunctionKeywordBodyIsOptional for the run that says a
	// bodyless declaration defines an empty function rather than an autoload
	// stub, and that `function a; echo B` binds `echo B` as the body (#1686).
	d.FunctionKeywordBodyIsOptional = true
	// After the keyword, the word is the name whatever is in it — the empty
	// string, a space, a semicolon, a dollar, all of it — and this shell is
	// the only one in the panel that reads it that way. The other four with
	// the keyword parse the line and refuse the *name* where it runs, two of
	// them without stopping; dash has no keyword to reach it with. See
	// syntax.Dialect.FunctionKeywordNameIsAnyWord for the six columns and for
	// the one group the flag does not carry, the names that shell matches
	// against the filesystem.
	//
	// Reached in the wild because a plugin manager builds definitions by
	// expansion and quotes the name with `${(q)…}`, so whatever the value it
	// meant to use came to arrives as a quoted word: an empty one (#1548) or
	// one holding a space (#1560).
	d.FunctionKeywordNameIsAnyWord = true
	// And the same of the `name()` spelling, which this parser had refusing a
	// quoted word while the keyword form took one — so `function a\ b { … }`
	// defined a function and `a\ b() { … }`, the same name, was a parse
	// error. Five of the six columns read the definition and four of those
	// refuse the *name* where it runs; only dash refuses to parse it. See
	// syntax.Dialect.FunctionNameIsAnyWord.
	//
	// Reached in the wild by every completion widget on the machine:
	// `zsh-autosuggestions` builds a wrapper per widget out of
	// `$widgets[$widget]`, which for a completion widget is
	// `user:_complete_help -C .complete-word _complete_help`, quotes the
	// blanks with backslashes and `eval`s a `name()` definition of it (#1743).
	d.FunctionNameIsAnyWord = true
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
	// A `^` in the same slot, which distributes the word the expansion stands
	// in over the elements it came to whatever RC_EXPAND_PARAM says: with
	// `a=(1 2)`, `x${^a}y` is the two words `x1y` and `x2y` where `x${a}y` is
	// `x1` and `2y`. This shell alone: measured 2026-09-08, bash 5.3, bash
	// 3.2, bash-as-sh and dash all answer `bad substitution` when the
	// expansion is reached and ksh93 refuses it while reading with the `^`
	// named. `~/.zi/bin/zi.zsh` builds the argv of every non-zsh plugin with
	// it — `${(s: :)^ICE[opts]}` — so a refusal stops the load of any plugin
	// carrying an `opts` ice.
	d.ParamRcExpandFlag = true
	d.ParamSetTestFlag = true
	// A subscript's own parenthesized flag group: `${a[(re)value]}`, the
	// first element equal to the operand. This shell alone — measured
	// 2026-09-06, bash 5.3, bash 3.2, bash-as-sh and ksh93 all read the same
	// text as arithmetic and fail there, and dash has no arrays to subscript
	// — and it is additive rather than a conflict because this shell reads a
	// group it does not recognize as arithmetic too. `~/.zi/bin/zi.zsh` has
	// sixty-four of them, six of which stand between the plugin manager and
	// its first definition.
	d.ArraySubscriptFlags = true
	// An assignment whose name is a number, which writes the positional
	// parameter that number names: `1=abc`. Measured 2026-09-07 with `set --
	// x; 1=abc; echo $1` — bash 5.3.15, that binary as `sh`, bash 3.2.57,
	// dash and ksh93 all read the word as a command name and answer
	// `1=abc: command not found` at 127, and this shell prints `abc`. So it
	// is set here and nowhere else.
	//
	// The plugin manager in `~/.zi` writes it twice, in
	// `.zi-any-to-user-plugin` and `.zi-formatter-pid`, and both run once per
	// plugin — twelve `no such file or directory: 1=username/reponame` lines
	// on a real startup, and no prompt (#1438).
	d.PositionalAssignment = true
	d.ParamElementSelection = true
	// `${a:/pat/repl}`, the same family's fourth operator: the elements the
	// pattern matches whole become the replacement. `compaudit` and
	// `compdump` both build their file list with `${^~fpath:/.}`, which is
	// this operator dropping `.` out of `fpath`, and p10k's `_p9k_must_init`
	// uses it over `$parameters` — so without it a real startup writes no
	// completion dump and draws no prompt (#1617).
	d.ParamWholeElementReplace = true
	// The length may carry an operator here, and it measures what the
	// operator *leaves*: `v=abc; echo ${#v#a}` is 2. bash, bash 3.2, bash as
	// `sh`, dash and ksh93 all call the same text a bad substitution, so
	// this is the grammar that has the construct rather than a different
	// arithmetic over one they share. Uniform across every operator this
	// shell has — the trims, the substring, the replacement, the four
	// conditionals and the element exclusion — measured 2026-09-06.
	d.ParamLengthTakesAnOperator = true
	// `${name::=word}`, the assignment that runs every time. Measured
	// 2026-09-07 on zsh 5.9.2 against the rest of the panel: bash 5.3, bash
	// 3.2 and bash as `sh` read the same text as a substring and answer
	// `=word: arithmetic syntax error: operand expected`, ksh93 answers
	// `:=word: arithmetic syntax error`, and dash — which has no substring —
	// says `Bad substitution`. So this shell alone, and the four refusals
	// are the evidence that it is additive rather than a shared syntax read
	// two ways.
	//
	// `~/.zi/bin/zi.zsh` writes it on the line its own message formatter is
	// built out of, which is why an unread `::=` printed eighteen arithmetic
	// errors and one line of raw markup on every startup (#1369).
	d.ParamAssignAlways = true
	// An expansion where a parameter name would be — `${${v}#a}`, which is
	// how this shell applies one expansion to the result of another and is
	// idiomatic here rather than a corner. Measured 2026-09-05 on zsh 5.9.2
	// against the rest of the panel: bash 3.2 and 5.3 answer `bad
	// substitution` and ksh93 a syntax error, so this shell alone.
	d.NestedParamExpansion = true
	// An expansion with no parameter name at all: `${:-abc}` is `abc`, `${}`
	// is the empty string, and `${%x}` trims a suffix off the nothing in
	// front of it. Measured 2026-09-08 on zsh 5.9.2 against the rest of the
	// panel: bash 5.3, bash as `sh`, bash 3.2 and dash all answer `bad
	// substitution`, and ksh93 refuses `:' while reading. This shell alone.
	//
	// `~/.zi/bin/zi.zsh` writes it inside the nested expansion that builds
	// the argv of every non-zsh plugin, so the two flags arrive together on
	// the same real line (#1529).
	d.NamelessParamExpansion = true
	// A bare `{` inside an unquoted `${…}` opens a nesting level here, so the
	// expansion ends at the brace that balances it. Measured 2026-09-10 with
	// `unset u; printf "[%s]" ${u:-{a,q}.z}`: this shell and ksh93 answer
	// `[a.z][q.z]`, two fields, where bash 5.3, bash 3.2 and dash answer the
	// single field `[{a,q.z}]` — the operand stopping at the first `}` and
	// `.z}` arriving as literal text. The comma is not what does it:
	// `${u:-a{b}c}` splits the panel the same way with no group in it at all.
	d.BareBraceNestsInExpansion = true
	// A parameter written without braces carries a subscript here, and `$#a`
	// is a count rather than `$#` with a letter after it. Measured 2026-09-05
	// on zsh 5.9.2: `a=(x y z); echo $a[1]` prints `x` and `echo $#a` prints
	// `3`, where bash 3.2, bash 5.3 and dash print `x[1]` and `0a`. This
	// shell alone, which is why it is set here and nowhere else.
	d.BareSubscript = true
	d.BareParamFlags = true
	// A braced expansion carries more than one subscript here, each reading
	// what the one before it named. Measured 2026-09-08 on zsh 5.9.2 with
	// `a=(one two three); echo ${a[1][2]}`: this shell prints `n`, bash 5.3
	// and that binary as `sh` both answer `bad substitution`, ksh93 prints
	// nothing, dash has no arrays, and bash 3.2 prints `two` — the second
	// subscript ignored rather than read. Five different answers to one text,
	// and this is the shell the flag describes.
	//
	// `~/.zi/bin/zi.zsh` writes `${ICE[atload][1]}` ten times, in the
	// function that sources a plugin (#1516).
	d.ChainedSubscript = true
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
	// And a name with a `(` touching it is a *math function* call —
	// `$(( mf(5) ))` — where `mf` was registered with `functions -M`. The
	// only shell in the panel with the construct; the other five read the
	// same text as a name with a leftover parenthesis after it and say so.
	// See interp/mathfunc.go for what running one means (#1493).
	d.ArithFunctionCall = true
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
	// The clobber-override marker generalized: `|` or `!`, after any of the
	// four write operators. Measured 2026-09-07 — all seven of `>!`, `>>|`,
	// `>>!`, `&>|`, `&>!`, `&>>|` and `&>>!` write the file they name here,
	// and none of the other five columns reads any of them. The `!` half is
	// the one to keep in mind: there `echo hi >! f` writes a file called `!`
	// holding `hi f` and reports success, so the spelling is not an error
	// elsewhere but a different meaning (#1247).
	d.ClobberOverrideMarker = true
	return d
}

// Semantics is what zsh means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	s.CommandNotFoundStatusIsNotFound = interp.No
	s.SetFTurnsOffGlobbing = interp.No
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
	// A job started with `&` reads the shell's own standard input here, where
	// the other five hand it an empty one — measured 2026-09-07,
	// `sh -c '/bin/cat & wait; echo ---; /bin/cat' < f` writes the file's line
	// *before* the marker in zsh 5.9.2 and after it in the other five. POSIX
	// XCU 2.9.3 specifies the majority; this is the divergence, and it is
	// this shell's to keep.
	s.BackgroundJobInput = interp.BackgroundJobInputIsTheShells
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
	// What `zsh --version` writes, on standard output at status 0 — measured
	// 2026-09-11, one line and no more.
	s.VersionOption = interp.VersionOption{Spellings: "--version", Text: versionLine()}
	// The parameter an `autoload`d name is looked up on, and the panel's only
	// one: `autoload -Uz is-at-least` finds its file on `$FPATH` and nothing
	// else does. Measured under `-f`, so it is the shell's own value and not a
	// startup file's — which is also what says the value survives the escape
	// hatch above. Named here and filled in by the front end, because which
	// directories is a fact about where a shell was installed; see
	// interp.Semantics.FunctionSearchVariable.
	s.FunctionSearchVariable = "FPATH"
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
	s.TestAndArithmeticUpdatePipelineStatus = interp.No
	// And where bash writes the record after negating one of those two, this
	// shell writes what the construct itself reported: `false | true;
	// ! [[ a = a ]]` leaves 0 here and 1 there. Measured 2026-09-11.
	s.NegatedTestRecordsThePostNegationStatus = interp.No
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
	// Every letter joins, kind letters included: `typeset -ax` writes every
	// array and every exported name, and `typeset +xr` every exported name
	// and every read-only one. Measured 2026-09-10, and the same answer
	// under both signs.
	s.DeclarationListingFilter = interp.DeclarationFilterAnyLetter
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
	s.ShiftNamesAreArrays = interp.Yes
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
	// A declaration may shadow a frozen name here: `typeset -r x=1; f() {
	// local x=2; echo $x; }` prints 2, the function runs on, and the outer
	// 1 is back — and frozen again — when it returns. bash refuses every
	// spelling of it.
	s.DeclarationMayShadowAReadonly = interp.Yes
	// And this is the one shell in the panel where `typeset +r` takes the
	// attribute back off: `typeset -r s=1; typeset +r s; s=9` leaves 9
	// there, silently and with status 0. `declare +r` is the same word. A
	// *special* parameter still refuses — `typeset +r EPOCHSECONDS` is
	// `can't change type of a special parameter` — which this engine has no
	// specials to reach.
	s.ReadonlyAttributeCanBeRemoved = interp.Yes
	s.DeclaredNameWithoutValueIsEmpty = interp.Yes
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
	// `readonly -a` declares the array as well as freezing the name:
	// `readonly -a a` lists as `typeset -ar a=(  )`.
	s.ReadonlyRecordsTheCompoundAttribute = interp.Yes
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
	// And an attribute added to a name that already holds a value re-reads
	// it at once, as ksh93 does: `FOO=bar; typeset -i FOO` stores 0 and
	// `d=MiXeD; typeset -u d` stores MIXED. A separate question from the
	// one above, which this shell happens to answer the same way — bash
	// answers both no and ksh93 answers them differently from each other.
	s.AttributeRereadsTheValueItFinds = interp.Yes
	s.InheritedValueSurvivesADeclaredType = interp.Yes
	s.CompoundElementsGoThroughTheAttribute = interp.No
	s.CompoundAttribute = interp.CompoundAttributeReplacesItWithAScalar
	// And the converse discards under both letters: `b=1; typeset -a b` is
	// `typeset -a b=( )` with `${#b[@]}` 0 and `$b` empty, and `a=1;
	// typeset -A a` is `typeset -A a=( )`. Measured 2026-09-08 against
	// 5.9.2 — the one column that throws the script's own value away, and
	// the one this implementation was giving every dialect.
	s.ScalarUnderAnArrayDeclaration = interp.ScalarUnderACompoundDiscardsIt
	s.ScalarUnderATableDeclaration = interp.ScalarUnderACompoundDiscardsIt
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
	s.ScalarAssignedOverACompoundReplacesTheName = interp.Yes
	s.ArrayLiteralAssignmentStartsTheNameOver = interp.No
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
	// A subscript is not a quoting context here — it is taken exactly as
	// written, with substitutions performed and every other character kept.
	// So `m["k"]=W` stores under the three characters `"k"` and `${m[k]}`
	// finds nothing, and `${q["$v"]}` looks up a key with two quote
	// characters in it where `${q[$v]}` looks up the value's own text. It is
	// one rule with the search operand's, which is why both go through
	// Runner.searchOperand.
	s.SubscriptIsAQuotingContext = interp.No
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
	s.BracketEscapeIsAlsoAMember = true
	// And the join this shell *does* perform: an unquoted `@` list reaching
	// a context that keeps no fields is joined on the first character of
	// IFS, so `IFS=-; a=(x y z); v=${a[@]}` is `x-y-z` where bash and ksh93
	// give `x y z`. With IFS set and empty it is `xy`, which is what says
	// the separator is read from IFS rather than defaulted to a space.
	s.UnsplitAtListJoinsOnIFS = interp.Yes
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
	s.GlobExpansionResults = interp.No
	s.GlobNoMatchIsError = interp.Yes
	// The one column that assigns: `set --; printf "<%s>" ${1:=abc}` is
	// `abc` at status 0 here and `$1` is `abc` afterwards, where the other
	// five refuse it fatally. `@` and `*` are refused here too — `not an
	// identifier` — so it is the positional alone that parts (#1541).
	s.AssignThroughExpansionMayNameAPositional = interp.Yes
	s.AssignmentPrefixPersistsOnSpecialBuiltin = interp.No
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
	// `${#s[@]}` on a scalar is the width of the value, so an empty one is
	// 0 and `a b` is 3 — where every other shell in the panel reads the name
	// as a list of one and answers 1 for both. Not moved by `ksharrays`,
	// measured: the three rows are 0, 1 and 3 under the option as well.
	s.WholeSubscriptOnAScalarMeasuresIt = interp.Yes
	s.FcEmptyHistoryIsAnError = interp.Yes
	s.JobControlAbsenceIsReportedFirst = interp.Yes
	// A stopped job holds the exit back: the shell says so and stays,
	// and the next attempt leaves. Measured through a pseudo-terminal for
	// `exit` and for ^D alike.
	s.StoppedJobsHoldTheExit = interp.Yes
	// And says only the sentence: measured, zsh writes `you have running
	// jobs.` and draws the next prompt, with no table under it, whichever way
	// `checkjobs` and `checkrunningjobs` are set.
	s.HeldExitListsTheJobs = interp.No
	// CDPATH moves in silence here.
	s.CdpathAnnouncesTheDirectory = interp.No
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
	s.EchoInterpretsEscapes = interp.Yes
	// echo reads -n, -e and -E, and -e wins over -E whatever the order.
	s.EchoOptions = "neE"
	s.EchoLastEscapeFlagWins = interp.No
	s.EchoExpandsHexEscapes = interp.Yes
	// `echo '\u0041'` and `echo '\U00000041'` are both `A` here, with no
	// `-e` needed, and a hexadecimal escape with no digit after it is a NUL
	// followed by whatever was there: `\xZ`, `\uZ` and a bare `\x` are all
	// `00` and then the rest, where bash leaves the two characters standing.
	s.EchoExpandsUnicodeEscapes = interp.Yes
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
	// Except for one refusal, which leaves 0 behind when the program came
	// from an argument rather than from a file. Measured on every
	// neighboring refusal too, and they all leave 1 — see the axis.
	s.SetArrayBadNameLeavesZeroFromCommandString = interp.Yes
	s.HeredocExpandsInTheCommandsProcess = interp.Yes
	s.RedirectTargetExpandsInTheCommandsProcess = interp.Yes
	s.ArithNameValueRecurses = interp.Yes
	// And an unset name found that way is a zero like any other unset name:
	// `x=abc; $((x+1))` is 1 and the script runs on. Measured 2026-09-11 —
	// ksh93 is the panel's holdout, where it is a fatal `parameter not set`.
	s.ArithRecursedNameMustBeSet = interp.No
	s.BraceExpansion = interp.Yes
	// Pads like bash — `{01..3}` is `01 02 03` — but a negative step
	// reverses the walk the endpoints chose: `{3..1..-1}` is `1 2 3` and
	// `{1..10..-4}` is `9 5 1`, bash's `1 5 9` backwards rather than the
	// `10 6 2` swapped endpoints would give.
	s.BraceRangePadsToEndpointWidth = interp.Yes
	s.BraceRangeStepSignHonored = interp.No
	s.BraceRangeNegativeStepReverses = interp.Yes
	// And the endpoints are read after the expansions in them: `n=3;
	// echo {1..$n}` is `1 2 3` where bash prints the literal `{1..3}`.
	s.BraceRangeEndpointsExpanded = interp.Yes
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
	// A condition operand that is not an expression abandons the input —
	// `echo one; [[ 1+ -eq 0 ]]; echo two` writes `one`, complains, and
	// never reaches `two`. Note it is `[[ ]]` alone: the same expression
	// in `(( ))` complains and the shell goes on.
	s.ConditionArithmeticErrorIsFatal = interp.Yes
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
	s.ArrayLiteralSubscriptIsAKey = interp.No
	// The FUNCTION_ARGZERO option, on by default and the reason this shell
	// alone moves `$0`: it names the function being run, or the file being
	// sourced, and goes back to the script's name when that call returns.
	s.DollarZeroNamesTheInnermostCall = interp.Yes
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
	// Except `${x?word}`, which this shell's own manual documents as printing
	// the word and *exiting the shell* — and measured to do exactly that from
	// inside a sourced file, where an unset parameter under `set -u` two
	// lines away is caught. So it is a request to stop rather than an error.
	s.ParamErrorIsAnExitRequest = interp.Yes
	s.DotMissingFileFatal = interp.No
	s.DotPassesArguments = interp.Yes
	// zsh opens a directory operand, reads no commands out of it and calls
	// that a script that did nothing: measured, `. ./` is silent at status
	// 0 and `. ./ && echo ok` prints `ok`. dash agrees; bash and ksh93 do
	// not. We reported `no such file or directory: ./` at 127 — the status
	// that says the path was never opened — four times in a real startup
	// (#1577).
	s.DotDirectoryOperandIsAnError = interp.No
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
	// The arm written first, which is this shell alone in the panel:
	// measured 2026-09-11 on 5.9.2, `x=abc`, `${x##(a|ab)}` is `bc` where
	// `${x##(ab|a)}` is `c`. The same order decides what a `(#b)` reports,
	// which the matcher already followed — see
	// Semantics.LongestPrefixTrimTakesTheWrittenArm.
	s.LongestPrefixTrimTakesTheWrittenArm = interp.Yes
	s.ExitTrapIsFunctionLocal = interp.Yes
	s.SignalHandlerSeesEarlierStatus = interp.Yes
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
	s.RedirectsUseEveryTarget = interp.Yes
	// The null-command hook: a command that is only redirections runs
	// `$NULLCMD`, or `$READNULLCMD` where its one redirection is a plain
	// `<file`. The names, not the values — a script reassigns them at will,
	// and the defaults live in the prelude where a script can see and change
	// them. `setopt cshnullcmd` and `setopt shnullcmd` move these two fields
	// rather than adding a branch; see nullcommand.go in this package.
	s.NullCommandVariable = nullCommandParameter
	s.ReadNullCommandVariable = readNullCommandParameter
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
	// `\u` and `\U` at both sites, and an empty digit run is a zero here
	// too: `printf 'a\uZ'` is an `a`, a NUL and a `Z`, where bash leaves the
	// escape standing and complains.
	s.PrintfUnicodeEscape = interp.PrintfUnicodeEscapeCodePointOrNul
	s.PrintfBUnicodeEscape = interp.PrintfUnicodeEscapeCodePointOrNul
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
	s.DollarSingleCaretMeta = interp.Yes
	s.GetoptsAssignmentRestartsWord = interp.No
	// OPTIND is local to a shell function here: the call starts at 1 and the
	// caller's position — words and the place inside a clustered word alike —
	// comes back on return. It is what lets this shell's own function
	// library parse options without resetting OPTIND by hand.
	s.GetoptsPositionIsFunctionLocal = interp.Yes
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
	// And fatal to `read` as well, which no other member of the panel is:
	// `read 1bad; echo after` prints the refusal and stops, in a function and
	// in a subshell alike. It is a field of its own because `read` is not a
	// special builtin anywhere, so this is not the special-builtin rule
	// reaching it — the two shells whose special builtins are fatal both
	// carry on past this one.
	s.BadNameToReadFatal = interp.Yes
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
	// ReadCountJudgesTheNamesAfterTheFirst is left unanswered here too.
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
	// The declaration builtins take one: `typeset a[1]=v` sets the element
	// and reports success, measured.
	s.TypesetTakesASubscript = interp.Yes
	s.UnsetTakesASubscript = interp.Yes
	// An element is not a name, so a declaration that would freeze, type or
	// localize the *array* refuses the operand instead of doing half of it —
	// and it ends the script. Three wordings for the three, measured
	// 2026-09-07, which is how a script tells which of them it ran into.
	// The same as ksh93 and for the same measurement, `unset` included: a
	// fatal `unset ":" ok1 ok2` removes ok1 and ok2 before it stops.
	s.BadNameDeclaresTheOperandsAfterIt = interp.Yes
	s.SubscriptedOperandTakesTheIntegerAttribute = interp.No
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
	// A `*` or `@` subscript inside an expression is the slice, joined and
	// then read as an expression: measured 2026-09-11 on 5.9.2, `typeset -A
	// m; m[k]=9; $(( m[*] ))` is 9 and `a=(1+1); $(( a[*] * 3 ))` is 6.
	s.ArithWholeArraySubscriptIsTheSlice = interp.Yes
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
	s.BadSubscriptToUnsetFatal = interp.No
	// A negative subscript past the first element places one in front of it
	// rather than being refused: `a=(p q); a[-3]=x` is three elements with
	// `x` at the head, and `-4` and `-5` land in the same place. What this
	// shell refuses is `a[0]`, which is below its first element rather than
	// counting back from the last.
	s.NegativeSubscriptPastTheStartInserts = interp.Yes
	// `not valid in this context: a+` — the append operator is not a
	// declaration operand here.
	s.DeclarationTakesAnAppendOperand = interp.No
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
	// The hooks. `chpwd` is the one whose site is a builtin rather than the
	// prompt loop — see repl.HookStyle for the two that are not — and the
	// suffix is shared by every hook this shell has, wherever it fires.
	s.HookListSuffix = "_functions"
	s.DirectoryChangeHook = "chpwd"
	s.CdLastPathOptionWins = interp.No
	s.BadSetOptionNameFatal = interp.Yes
	s.UnknownConditionOptionIsAStatus = interp.Yes
	s.ReturnOutsideAFunctionIsRefused = interp.No
	// And `break` with no loop around it stops the script here, which is the
	// opposite way round from the line above: measured, `echo t; break` on
	// one line and on two both end at status 1 with nothing after the
	// `break` running at all.
	s.LoopControlOutsideALoopIsFatal = interp.Yes
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
	s.DeclareOptions = "aAfFgHhilmpruUTxz"
	// `-z` and `+z` are taken and do nothing here, which is measured on both
	// signs and on both sides of the builtin: `typeset -z p q` leaves `p`
	// alone and declares an empty `q` exactly as a bare `typeset q` would,
	// `typeset +z p` is 0 with `p` unchanged, and `typeset -fz f` over a
	// defined function is 0 with the function still defined and still
	// listing as itself.
	//
	// The one spelling where the letter is not inert is `typeset -fu`, which
	// is that shell's second name for `autoload` — there `-z` picks the
	// autoloaded file's syntax and is recorded on the stub, `typeset -fuz n`
	// listing as `builtin autoload -Xz` exactly as `autoload -z n` does.
	// This engine's `typeset -fu` does not autoload at all yet (#1753), so
	// there is nothing for the letter to be recorded on and inert is the
	// whole truth for every spelling that works here. `autoload -z` itself
	// is unaffected: that word has its own table and already keeps the
	// letter.
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
	// `typeset +f` is the function *names*, one bare name a line and no
	// quoting — the shape a shell snapshot reads to find what to capture.
	// See Semantics.FunctionNamesUnderPlus for the two other answers.
	s.FunctionNamesUnderPlus = interp.Yes
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
	s.DeclareOptionsTakingANumber = "F"
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
	s.FunctionsOptions = "mM"
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
	// `E`, `L`, `R` and `t` stay out, and that is the honest half: zsh takes
	// them and this shell has not built them, `typeset -E` being `-E is not
	// implemented yet` in the same run. Claiming them here would move the
	// refusal from the letter to nowhere at all.
	s.LocalOptions = "aAFHhilpruUTx"
	// A bad `typeset` option is reported and the script goes on.
	s.TypesetBadOptionFatal = interp.No
	// `integer` here is `typeset` with the letter prepended rather than a
	// declaration of its own, and its letter set is *narrower* than
	// typeset's: measured 2026-09-06, `integer -a`, `-A`, `-f`, `-F`, `-T`
	// and `-U` are bad options where this shell's typeset takes all six. So
	// the set is its own field and not DeclareOptions over again — a shell
	// that reused them would accept `integer -A m`, which is an associative
	// array in no shell that has the word.
	s.IntegerOptions = "gHhilprux"
	// `-i16` and `-i 16` are an output base here — `integer -i 16 b=255` is
	// `16#FF` — and this engine has no base to keep, so it refuses by name.
	s.IntegerAttributeTakesABase = interp.Yes
	// The alphabet this shell counts an output base in, and its length is
	// the largest base it can spell — see Semantics.IntegerBaseDigits.
	s.IntegerBaseDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	s.IntegerBaseComesFromTheValueAssigned = interp.Yes
	s.IntegerBaseNegativeIsTwosComplement = interp.No
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
	// The name ends the options here: `set -A ff -x -y` stores the two dash
	// words as elements and `set -A dd -- 1 2` stores three, the `--` among
	// them. ksh93 keeps parsing and answers both the other way.
	s.SetArrayOptionsContinuePastTheName = interp.No
	// `set -A a` with no values leaves an array with no elements here —
	// `typeset -a a=(  )` — where ksh93 unsets the name outright. Both count
	// 0, so the difference shows only to a script that asks whether the name
	// is set at all.
	s.SetArrayWithNoValuesUnsetsTheName = interp.No
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

	// `[[ -v ]]` reads more kinds of name here than either of the other two
	// shells that have the operator: a positional, as bash does, and every
	// parameter spelled as one punctuation character — `?`, `#`, `$`, `!`,
	// `*` and `-` — which is this shell's alone. `@` is unset here as it is
	// everywhere, so it is excluded outright rather than by an axis.
	// Measured on zsh 5.9.2.
	s.ParameterIsSetSeesPositionals = true
	s.ParameterIsSetSeesSpecials = true
	return s
}

// Diagnostics is how zsh reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		// The four loops this shell has, and the builtin's name is stripped
		// back out of the front of it because this dialect puts it in the
		// location: `zsh:break:1: not in while, …`.
		LoopControlOutsideALoop: "%[1]s: not in while, until, select, or repeat loop",
		// zsh names itself, not the path it was invoked by. `/bin/zsh` and a
		// symlink called `myzsh` both say `zsh:`, and so does the shell run
		// as `exec -a weirdname /bin/zsh` — measured all three ways, because
		// the first alone looks like a base name rather than a fixed one.
		// The other three shells print argv[0] whole.
		SelfName:    "zsh",
		TypeKeyword: "%[1]s is a reserved word",
		// The only one that names itself in the line.
		TypeFunction: "%[1]s is a shell function from zsh",
		// A name declared with `autoload` and not yet called is not an
		// ordinary function here and does not say it is: measured,
		// `whence -v myfn` and `type myfn` both write this line, and the
		// same name after one call writes TypeFunction instead.
		TypeUndefinedFunction:  "%[1]s is an autoload shell function",
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
		BadArraySubscript:   "%[1]s: assignment to invalid subscript range",
		ArithEmptySubscript: "invalid subscript",
		// The same sentence at the parameter site, and its own field because
		// the two coincide here and do not in bash — see the field.
		EmptyParamSubscript: "invalid subscript",
		// The two ways a subscripted array literal has no span to land in.
		// Worded apart from each other here, and both without the subscript:
		// what is reported is the name's kind rather than the number.
		ArrayValueToNonArray:      "%[1]s: attempt to assign array value to non-array",
		SliceOfAnAssociativeArray: "%[1]s: attempt to set slice of associative array",
		// `+=` at a subscript of a numeric name. The name is not in the
		// sentence at all here, where it is in both of the two above — the
		// location carries it, as it does for the messages this shell raises
		// from an expansion.
		AppendToANumericSlice: "attempt to add to slice of a numeric variable",
		// The identical sentence from `unset`, and the builtin is *not* in
		// the location for it — the store is speaking rather than `unset` —
		// which is why the two routes need two fields even where one shell
		// words them alike.
		UnsetSubscriptBeforeTheFirstElement: "%[1]s: assignment to invalid subscript range",
		// Through a literal it is the subscript alone, and the literal is
		// named for what it is rather than by the variable it fills.
		BadArrayLiteralSubscript: "bad subscript for direct array assignment: %[2]s",
		DeclareNoSuchVariable:    "no such variable: %[1]s",
		// Measured: `typeset -i64 a=100` and `typeset -i1 f=5` are both
		// refused, the name is left with nothing, and the script carries on.
		IntegerBadBase:           "invalid base (must be 2 to 36 inclusive): %[2]s",
		LocationNamesTheFunction: true,
		SetInvalidOptionName:     "no such option: %[1]s",
		SetImmovableOptionName:   "can't change option: %[1]s",
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
		RunningJobsAtExit: "%[1]s: you have running jobs.",
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
		// Under `noclobber`, a name holding something that is not a regular
		// file is opened — `>/dev/null` writes — and where that open fails
		// for a reason of its own zsh says `file exists` about it anyway.
		// Measured: a directory, a socket, a dangling symlink and a
		// `/dev/tty` with no controlling terminal all answer `file exists`
		// here and answer with the open's own reason in the other three.
		NoclobberRefusalCoversAFailedOpen: true,
		// The null-command hook's own refusal, for a command that is only
		// redirections and a `NULLCMD` with nothing in it. No verbs, and the
		// same sentence whether the parameter was unset or emptied.
		RedirectionWithNoCommand: "redirection with no command",
		NotABuiltin:              "no such builtin: %[1]s",
		CommandStringParsedWhole: true,
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
		// An element a declaration will not create, in the three ways this
		// shell will not create one — measured 2026-09-07. The base and the
		// subscript are the verbs, so what is quoted back is `a[1]` and not
		// the whole operand.
		//
		// There is no BuiltinBadSubscript here any more. The two entries it
		// held were reachable only through a path that never ran: a
		// declaration's operand was glob-matched before the builtin saw it,
		// and `export a[0]=v`'s complaint turned out to belong to the array
		// store rather than to the name check — it is what a bare
		// `a[0]=v` says, since the first element is number one (#1203).
		ReadonlyElementRefusal: "%[1]s[%[2]s]: can't create readonly array elements",
		IntegerElementRefusal:  "%[1]s[%[2]s]: inconsistent array element or slice assignment",
		LocalElementRefusal:    "%[1]s[%[2]s]: can't create local array elements",
		// `set -A 1v q` does not name the builtin in its location where
		// `unset 1x` and `typeset 1w` from this same shell do.
		// `read` joins `set` in it: `zsh:1: not an identifier: 1bad` has no
		// `read:` in the location where `zsh:export:1:` has the builtin.
		BadNameRefusalHidesTheBuiltin: map[string]bool{"set": true, "read": true},
		// `set -t`, the one letter this shell has and refuses to move. It is
		// `singlecommand` under a borrowed spelling, one of the five options
		// about being interactive that this shell will not let a script
		// change — and it answers the letter exactly as it answers the name:
		// measured 2026-09-10, `set -t` is `can't change option: -t` at 1 and
		// stops the script, the same sentence, status and fatality as
		// `set -o singlecommand`, with the letter echoed back rather than the
		// option's own name.
		//
		// Before this it rode UnimplementedOptionLetters and said `-t is not
		// implemented yet`, which is this implementation confessing to
		// something the shell itself refuses (#1716).
		ImmovableOptionLetters: map[string]string{"set": "t"},
		UnimplementedOptionLetters: map[string]string{
			// zsh gives a single letter to far more of its options than the
			// rest of the panel does: measured 2026-09-05, it refuses only
			// b, c, j, q and z of the fifty-two, and has the other
			// forty-seven. These are the ones it has and this shell does
			// not, so a script asking for one is told it is missing rather
			// than told this shell knows better than zsh what zsh has.
			// `-A` has left this list: it assigns an array and is
			// implemented, in Semantics.SetArrayLetter.
			//
			// `-t` has left it for the opposite reason, and that is the
			// point of the pair: this shell will not move that option at
			// all, which is a different sentence from a letter we have not
			// built — see ImmovableOptionLetters below.
			"set": "dgiklprswyBDEFGHIJKLMNOPQRSTUVWXYZ",
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
			// `-H`, `-U`, `-T`, `-h` and `-m` have left this list — they are
			// implemented, in DeclareOptions above.
			"typeset": "bcEkLnRtZ",
			"type":    "mvwsS",
			// jobs' letters that are zsh's own: -d names the directory the
			// job was started in, and -z and -Z are about the process
			// title rather than about the job table.
			"jobs":    "dzZ",
			"declare": "bcEkLnRtZ",
			// The same list as `typeset` and `declare`, which is the point:
			// `-F` is one attribute and the three names declare it alike.
			// It was here and in neither of theirs, which is the same split
			// LocalOptions had — the letter refused under one name and
			// answered under the other (#1594).
			// `local`'s list is *not* `typeset`'s, and `m` is why: this
			// shell's `local` has no `-m` to be missing — `local -m q`
			// inside a function is `bad option: -m` there — so naming it
			// unimplemented told a script the letter was on its way when
			// nothing was coming. `L`, `R` and `Z` are the padding letters
			// `local` really does spell and this engine does not.
			"local": "bcEkLnRtZ",
			// `integer`'s own short list, and it is not typeset's: the
			// letters typeset is missing that `integer` refuses outright —
			// b, c, E and m — are bad options under this name and belong in
			// neither field, while `-t`, `-L`, `-R` and `-Z` are letters
			// this shell's `integer` really takes. `-h` was here too and is
			// implemented now, in IntegerOptions above.
			"integer": "LRZt",
			// `functions`' own letters, none of which is `typeset`'s: -u
			// and -U mark a name for autoloading, -k and -z pick which
			// shell the autoloaded file is read as, -t and -T trace, -x
			// sets the listing's indent, -c makes one function another's
			// copy and -W is the `zsh/parameter` writability flag. -m and
			// -M are implemented, in FunctionsOptions above.
			"functions": "ckstuxzTUW",
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
		TraceQuoting:      interp.QuoteShell,
		TraceStyle:        interp.TraceNameLine,
		TraceForHeader:    interp.TraceForAssign,
		TraceArrayLiteral: interp.TraceArraySpaced,
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
		ArithIllegalByte:          "bad math expression: illegal character: %[1]s",
		ArithOperandExpected:      "bad math expression: operand expected at `%[1]s'",
		ArithExpressionRanOut:     "bad math expression: operand expected at end of string",
		ArithOperatorExpected:     "bad math expression: operator expected at `%[1]s'",
		// The math functions, which only this shell has. The three at the
		// call stand alone rather than being wrapped as a bad math
		// expression — measured 2026-09-08, `zsh:1: unknown function:
		// nosuchmf` where an ordinary failure in the same place opens with
		// `bad math expression:`. The four at the registration are the
		// builtin's and name it in the location the way its option
		// complaints do: `zsh:functions:1: -M: too many arguments`.
		//
		// `no such function` names the *implementation*: a registration is
		// never checked when it is made, so `functions -M mf 1 1 nodef` is
		// status 0 and this arrives at the first call.
		MathFunctionUnknown:         "unknown function: %[1]s",
		MathFunctionArgumentCount:   "wrong number of arguments: %[1]s",
		MathFunctionMissingImpl:     "no such function: %[1]s",
		MathFunctionTooManyOperands: "%[1]s: -M: too many arguments",
		MathFunctionBadName:         "%[1]s: -M %[2]s: bad math function name",
		MathFunctionBadMinimum:      "%[1]s: -M: invalid min number of arguments: %[2]s",
		MathFunctionBadMaximum:      "%[1]s: -M: invalid max number of arguments: %[2]s",
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
		// This shell prints at most twenty bytes of the word and marks it,
		// and marks it at exactly twenty as well — see the field. The other
		// three print the whole word or none of it.
		UnmatchedNearMaxBytes: 20,
		// The word again, the same as for `$(` — and for `$[` too, which
		// this shell has and refuses the same way.
		UnmatchedArithSubst: "parse error near `%[3]s'",
		UnmatchedBraceSubst: "closing brace expected",
		SyntaxErrorStatus:   1,
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
		// A third operand to the substitution form, at this shell's ordinary
		// `cd` status rather than a usage one — which is where it parts
		// company with bash over the same sentence.
		CdTooManyOperands: "cd: too many arguments",
		// And `cd old new` where old is not in the current directory's path,
		// which names the operand where ksh93's does not.
		CdBadSubstitution: "cd: string not in pwd: %[1]s",
		PrintfBadVerb:     "%[2]s: invalid directive",
		PrintfMissingVerb: "%[1]s: invalid directive",
		UmaskBadMask:      "bad umask",
		// The builtin's name comes from the location, as everywhere in zsh.
		UnaliasNotFound:        "no such hash table element: %[2]s",
		UnaliasAllWithOperands: "-a: too many arguments",
		UnaliasUsage:           "not enough arguments",
		UnsetNoOperands:        "%[1]s: not enough arguments",
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
			// `set -A 1bad v` — the array letter's name operand. It gets the
			// leading-digit wording rather than this shell's usual "not
			// valid in this context", measured: `not an identifier: 1bad`.
			"set":      "not an identifier: %[2]s",
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
			// `integer` names itself in its own refusal here, where ksh93's
			// calls itself `typeset` — so it is an entry rather than a
			// rename. Measured: `integer 1x` is
			// `zsh:integer:1: not an identifier: 1x`.
			"integer": "not valid in this context: %[2]s",
			// `read` gets the leading-digit wording for every bad operand,
			// not just a numeric one: `read a-b` is `not an identifier: a-b`
			// where `export a-b` is `not valid in this context`. So there is
			// no BuiltinBadNameNumeric entry beside this — the two are the
			// same sentence here.
			"read": "not an identifier: %[2]s",
		},
		// An operand that starts with a digit is a different complaint, for
		// the two that have one. `unset` says the same to both.
		BuiltinBadNameNumeric: map[string]string{
			"export":   "not an identifier: %[2]s",
			"readonly": "not an identifier: %[2]s",
			"local":    "not an identifier: %[2]s",
			"typeset":  "not an identifier: %[2]s",
			"declare":  "not an identifier: %[2]s",
			"integer":  "not an identifier: %[2]s",
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
		// Except for a math complaint, which this shell writes as its own:
		// `let '1+'` is `zsh:1: bad math expression: …` where its `cd` is
		// `zsh:cd:1: …`, and `let` with no operand at all *is* the builtin's
		// and does say `zsh:let:1:`. See the field.
		ArithErrorNamesTheBuiltin: false,
		LowercaseReason:           true,
		DirectoryReason:           "Permission denied",
		HashNotFound:              "no such command: %[1]s",
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
	registerLocalOptions(r)
	// `**/` crosses directory levels here with no option asked for, and
	// there is no `setopt` name that turns it off — which is why this is a
	// state the dialect sets rather than a name registered above. Measured
	// 2026-09-07 against zsh 5.9.2 in a directory holding `ax`, `bx` and
	// `cx/dx/ax`: `**/a*` is `ax cx/ax cx/dx/ax`, so the component stands
	// for **zero or more** levels and the match in the starting directory is
	// part of the answer. Without this the component read as `*` and the
	// listing came back short at status 0 (#1339).
	//
	// Only the slashed form. Bare `**` is an ordinary pattern here — `echo
	// **` is `ax bx cx`, which is what `*` answers — so
	// StarStarAloneCrossesDirectories stays off, and `setopt globstarshort`,
	// which is the name that would move it, remains recorded rather than
	// acted on (see setopt.go).
	r.SetMatchOption(interp.StarStarCrossesDirectories, true)
	// $LINES and $COLUMNS follow the window here too, and — like `**/` above
	// — with no option name to ask for it: this shell simply does it, so
	// there is nothing in setopt.go for a script to turn off. Measured
	// 2026-09-08 through a pseudo-terminal against zsh 5.9.2 started with
	// `-f`, which reports `COLUMNS=80 LINES=24` at its first prompt and
	// `132`/`40` at the next prompt after a resize — the same two numbers
	// bash answers with `checkwinsize` on.
	//
	// The same core switch bash's `checkwinsize` moves. One capability with
	// two shells' worth of naming above it is exactly the shape a second copy
	// gets written into, so there is one: interp.Runner.TracksWindowSize.
	r.SetTracksWindowSize(true)
	// A job still running holds the exit here, where bash needs to be asked:
	// measured, zsh 5.9.2 started with `-f` answers `sleep 40 &` then `exit`
	// with `you have running jobs.` and stays, and bash 5.3.15 leaves. Both
	// shells spell the option `checkjobs`; only the defaults differ, and this
	// is where this one's is set. See interp.Runner.ChecksRunningJobsAtExit.
	r.SetChecksRunningJobsAtExit(true)
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
	// And the other half of it: defining what a key can be bound *to*. The
	// table was here without it, so an rc file that bound a plugin's own
	// widget bound a name nothing ever answered to. See zle.go.
	registerZle(r)
	// Timed commands, which is what a plugin manager's deferred loading is
	// built on. Independent of the line editor despite arriving with it. See
	// sched.go.
	registerSched(r)
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
	// And `zsh/datetime`'s four, all of which this shell can answer honestly:
	// three clock reads through Runner.Now and a formatter over the same
	// format language `printf '%(fmt)T'` writes. See datetime.go.
	registerDatetimeModule(r)
	// And `zsh/stat`'s one builtin under the one of its two names a shell
	// may safely hold always: `zstat`. See statmodule.go for why `stat` is
	// not registered beside it.
	registerStatModule(r)
	// And `zsh/files`' nine file operations, under the nine `zf_`-prefixed
	// names — the same argument again, and the same split. See
	// filesmodule.go.
	registerFilesModule(r)
	// And `zsh/net/socket`'s one: a Unix-domain socket opened into this
	// shell's own descriptor table. See socketmodule.go.
	registerSocketModule(r)
	// And `zsh/terminfo`'s and `zsh/termcap`'s one parameter each: the
	// terminal's capabilities under two name systems, over the values in
	// repl.TerminalCapabilities. See terminfo.go.
	registerTerminfoModules(r)
	// And `zsh/langinfo`'s one: the locale's own vocabulary, answered from
	// the variables that name the locale rather than from a fixed table. See
	// langinfo.go.
	registerLangInfoModule(r)
	// And `zsh/zleparameter`'s two: the line editor's widgets and keymaps,
	// read off the tables `zle` and `bindkey` already keep. See
	// zleparameter.go.
	registerZleParameterModule(r)
	// And `zsh/system`, all nine features of it: two parameters, a math
	// function and the six builtins — three that move bytes, a file lock and
	// the two small ones that close the module. See system.go, systemio.go,
	// systemlock.go and systemseek.go.
	registerSystemModule(r)
	// And `zsh/zselect`'s one: the wait on descriptors that the same prompt
	// theme's worker uses as its only sleep. See zselect.go.
	registerZselectModule(r)
	// And `zsh/mathfunc`'s forty-seven, which are the C math library under
	// names arithmetic can call. See mathmodule.go.
	registerMathFuncModule(r)
	// This shell's richer `echo`, and not ksh93's builtin of the same
	// spelling: different letters, a different escape set and different
	// wordings, all measured side by side. See print.go.
	registerPrint(r)
	// And the same escape set, reached from the other end: the `(p)`
	// expansion flag reads the argument of a `j` or `s` behind it the way
	// `print` reads an operand, so `${(pj:\n:)a}` joins on a real newline.
	// One function for both, because the set is one measurement — see
	// print.go, and interp.Runner.SetFlagArgumentEscapes for why the
	// substrate asks rather than answers.
	r.SetFlagArgumentEscapes(expandFlagArgumentEscapes)
	// And the same set a third time, reached from a third end: the `(g)`
	// expansion flag reads the escapes in a *value*, with its option letters
	// naming which parts of the set are live. One decoder for all three, for
	// the reason above — see print.go, and interp.Runner.SetExpansionEscapes.
	r.SetExpansionEscapes(expandExpansionFlagEscapes)
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
	// The login name for that same uid, for the `%n` prompt escape. Asked
	// here for the reason the uid above is: nothing a script does changes
	// it, two shells in one program genuinely have the same one, and it has
	// no other source. Measured as a fact about the *process* rather than
	// about the environment — `%n` ignores `$USER`, `$LOGNAME` and
	// `$USERNAME` however they are set — so it is not read out of a variable.
	//
	// Handed over as a *question* rather than as an answer, and that is the
	// whole of #1403's zsh column. `user.Current` measures 0.83-1.10 ms on
	// darwin — Directory Services, and no cheaper with cgo off — and Apply
	// runs on every invocation, so `zsh -c ':'` and every subshell paid a
	// millisecond to learn a name only a prompt can draw. Asked when `%n` is
	// drawn, the cost lands on the one route that wants it and the answer is
	// kept for the redraws. What is *not* different is who may ask: the core
	// still does not, this dialect still does, and the reasoning above about
	// uid, process and environment stands unchanged.
	//
	// The question itself is interp.LoginName rather than a closure written
	// here, and that is #1446: bash needs the same answer for `\u`, and while
	// this was the only asker the drawer grew a lookup of its own that read
	// `$USER` — which is empty under `env -i` and names the wrong person under
	// `env USER=someone-else`. One question, every asker.
	r.SetPromptUserFunc(interp.LoginName)
	// The machine's name, for `%m` and `%M`, asked the same way and from the
	// same place.
	r.SetPromptHostFunc(interp.MachineName)
	// And the prompt-escape table, which is the *same* value the prompt
	// drawer is handed — repl.PromptStyle is an alias for interp.PromptStyle,
	// not a copy. This is what makes `print -P '%F{196}…'` and a drawn prompt
	// one answer rather than two: before it, this shell drew `%F{196}` at a
	// prompt and refused it by name in a script (#1090).
	r.SetPromptStyle(PromptStyle())
	r.SetSpecial("EUID", strconv.Itoa(os.Geteuid()))
	r.SetDynamic("RANDOM", func(*interp.Runner) string { return interp.Randoms() })
	// `$ARGC`, this shell's name for `$#` — see argc.go for what was
	// measured and for the startup that could not run without it.
	registerARGC(r)
	r.SetDynamic("SECONDS", func(rr *interp.Runner) string {
		// Whole seconds unless the parameter carries the float attribute,
		// which is how a script asks this one to count in fractions:
		// `typeset -F 3 SECONDS=0` is what a plugin loader opens with and
		// what it reads back to time itself (#1453). Measured 2026-09-07 —
		// three places after `-F 3`, ten after a bare `-F`, and whole
		// seconds with neither.
		//
		// The producer asks rather than the attribute reaching it, because a
		// counted value never passes through the fold every stored name
		// does: there is nothing to fold until the moment of the read.
		if places, ok := rr.FloatPlacesOf("SECONDS"); ok {
			return strconv.FormatFloat(rr.SecondsFrom(), 'f', places, 64)
		}
		return strconv.Itoa(int(rr.SecondsFrom()))
	})
	// A NUL as well as the three whitespace characters, which is zsh's alone.
	r.SetSpecial("IFS", " \t\n\x00")
	// The two names the null-command options point at, which no script can
	// reach and which never move — see nullcommand.go in this package.
	registerNullCommandParameters(r)
	tieTheBuiltInPairs(r)
	// And the four scalar pairs that are one parameter under two names —
	// see promptnames.go, and the theme that could not draw without them.
	registerPromptNames(r)
	if dot, ok := r.Builtin("."); ok {
		r.Register("source", dot)
	}
	// `declare` is `typeset` under a second name rather than a second
	// implementation. ksh93 has only the older name and dash has neither, so
	// which names exist is a dialect's answer and not an axis.
	// The precommand modifiers this shell has and the core does not, as a
	// table of names — see interp.PrecommandModifier for what was measured
	// about each of them and why `command` is not among them.
	//
	// `noglob` is the one with an effect: the plugin manager on the rc file
	// this shell has to run reaches `.zi-set-m-func`, whose whole body is
	// `noglob unset functions[m]`, and an unmatched pattern is fatal here —
	// so the word being ignored ended that function rather than producing a
	// wrong one (#1526).
	//
	// `builtin` and `exec` are named only so the scan reads past them:
	// `builtin noglob echo a[b]c` and `exec noglob echo a[b]c` both print
	// the three characters, measured. They keep their own builtins and do
	// their own work.
	r.SetPrecommand("noglob", interp.PrecommandNoGlob)
	r.SetPrecommand("builtin", interp.PrecommandTransparent)
	r.SetPrecommand("exec", interp.PrecommandTransparent)
	// `integer` is `typeset` with the type already decided, and it is one of
	// the two shells that has the word — this shell's own `add-zsh-hook`
	// opens with `integer del list help`, so a startup file that installs a
	// hook cannot run without it. Registered rather than built here, so both
	// dialects get the *same* declaration — see interp/integerbuiltin.go.
	r.Register("integer", interp.IntegerBuiltin())
	// And the assignment rule follows the name the way it follows `declare`:
	// `integer n=5+2` is a declaration's operand and not a word to split.
	r.SetDeclaring("integer")
	// `functions` is `typeset -f` under this shell's own name and
	// `unfunction` is `unset -f` under its own — the same registration
	// `integer` gets and for the same reason, so that the listing and the
	// removal have one implementation between two words each. See
	// interp/functionsbuiltin.go.
	//
	// The plugin manager on the rc file this shell has to run is why they
	// exist: `unfunction` is on 47 lines of it, nine of them consecutive in
	// the path out of every plugin load, and a `command not found` there
	// leaves that loader's temporary stubs standing over `compdef`,
	// `autoload`, `source`, `bindkey`, `zstyle`, `alias` and `zle` for the
	// rest of the session (#1487).
	r.Register("functions", interp.FunctionsBuiltin())
	r.Register("unfunction", interp.UnfunctionBuiltin())
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
