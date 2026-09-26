// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package zsh answers the substrate's questions the way zsh does.
package zsh

import (
	"context"
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
	// `in` is an ordinary command name here, where bash, dash and ksh93 all
	// refuse it wherever a command may begin. Measured 2026-09-22: `in` on
	// its own and `echo | in` are both `command not found` and `in() { :; }`
	// defines a function, against a syntax error naming the word in the
	// other three. See syntax.Dialect.InStandsAsACommandName (#4134).
	d.InStandsAsACommandName = true
	// A here-document body line joined out of two physical ones is compared
	// against the delimiter whole, as it is in bash: `A\` over `BC` ends an
	// `ABC` document. dash and BusyBox ash take only a join that began at
	// the start of a line, and ksh93 takes neither (#2430).
	d.HeredocDelimiterAcrossAContinuation = syntax.HeredocDelimiterOnTheJoinedLine
	// A `<<-` delimiter written with leading tabs has them stripped the way
	// the body lines do, so `EOF` ends a `<tab>EOF` document. ksh93 agrees;
	// bash and dash do not. See syntax.HeredocDelimiterTabs.
	d.StrippedHeredocDelimiter = syntax.HeredocDelimiterTabsAreStrippedToo
	// A backslash the input ends immediately after is dropped and the word
	// it was in stays: `printf "[%s][%s]" a \` prints `[a][]` here and
	// `[a][\]` in bash 5.3, dash and BusyBox ash. Measured 2026-09-13; see
	// [syntax.EndOfInputBackslash] (#2680).
	d.BackslashAtEndOfInput = syntax.EndOfInputBackslashIsDropped
	// `exec {1}>&-` closes the descriptor a *positional parameter* holds,
	// which is how a prompt theme's scheduler closes the one it was handed.
	// zsh alone — see the flag for what bash and ksh93 answer instead.
	d.FdVariablePositional = true
	// zsh expands with nobody asking; `unsetopt aliases` is how it is turned
	// off, and measured 2026-09-12 that option is the only gate on `eval`, a
	// command substitution, a sourced file and a trap body alike — under a
	// `-c` string as much as anywhere else.
	d.AliasesExpandUnlessTold = true
	// A function defined under a name the alias table holds is refused
	// outright here, before any parse of the body. Measured 2026-09-18,
	// script files under `env -i PATH=/usr/bin:/bin LC_ALL=C`: with
	// `alias zz='typeset -n'`, both `zz() { :; }` and `zz () { :; }` write
	// two lines and exit 1, and so does the same definition under
	// `alias zz=echo`, whose expansion would have been a legal definition —
	// which is what says it is the *name being an alias* that is refused
	// rather than anything about the value.
	//
	// This shell expanded and then read `typeset -n () { :; }` as a
	// multi-name definition, which is a legal shape here, so two functions
	// called `typeset` and `-n` were defined in silence at 0 (#3643).
	d.AliasAtAFunctionName = syntax.AliasRefusesAFunctionName
	// The shell's own program text is the one place the route matters, and
	// the reason is not aliases: a `-c` string is read *whole* here — see
	// Diagnostics.CommandStringParsedWhole — so the `alias` on line 1 has not
	// run when line 2 is parsed. Measured 2026-09-11, `zsh -fc $'printf
	// A\nif; then'` prints no A at all. This front end reads a command string
	// a line at a time, so the route set is what stands in for that.
	d.ExpandAliasesInProgramText = syntax.RouteFromScriptFile | syntax.RouteOnStandardInput
	// And a body's newlines are lines of the program, as they are in the two
	// that expand by every route.
	d.AliasBodyCountsLines = true
	// A reserved word may be aliased and the alias wins, as in bash: `alias
	// for=echo` on one line makes `for x in 1` on the next a command.
	// Measured 2026-09-13, zsh 5.9.2 from a script file. Invoking this shell
	// as `sh` takes it back, which is interp.Runner.SetPosixMode's half and
	// is why the value here is the shell's own and not the mode's.
	d.AliasesExpandReservedWords = true
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

	// The other half of the same reserved-word rule: a bare `{` where a
	// command may begin is the word by itself, so `a(){print A}` is a
	// definition here and a command called `{print` everywhere else. See
	// syntax.Dialect.OpenBraceNeedsNoBlank for the measurements (#1788).
	d.OpenBraceNeedsNoBlank = true
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
	// And a short body may be empty because a joining operator stands where
	// it would be: `for i in a b; | cat` pipes the loop rather than refusing
	// a missing body (#3898). Measured with the rest of the panel on
	// syntax.Dialect.ShortBodyEndsOnAJoiningOperator.
	d.ShortBodyEndsOnAJoiningOperator = true
	// A C-style `for` header may hold more than the two separators its three
	// expressions need: everything past the second `;` is part of the third
	// expression, semicolons and all, so `for ((;;;)); do echo body; break;
	// done` prints `body` here and is a whole-script syntax error in bash.
	// Measured 2026-09-12 against bash 5.3.15, bash 3.2.57, the same bash as
	// `sh`, and ksh93u+, which agrees with this shell. Fewer than two is not
	// this flag: `for ((i=0))` is refused by every column (#2225).
	d.ForArithExtraSeparators = true
	// `for key value ( a 1 b 2 ) { … }`: the loop takes one word per name on
	// every pass, and a short final pass leaves the names it did not reach
	// empty. Independent of the two flags above, measured over all four
	// combinations.
	d.ForMultipleNames = true
	d.Repeat = true
	d.Foreach = true
	// `case x { x) echo hit;; }`: the brace spelling of a `case` header, the
	// same shape as the short loop forms above. The opener and the closer are
	// independent here — `case x { … esac` and `case x in … }` both run —
	// which is what separates this answer from ksh93's, where the two are
	// paired and both mixtures are `` `case' unmatched ``. And `esac` stays
	// reserved after the `{`, so `case esac { esac) … }` is a parse error
	// here and prints `hit` there (#1928).
	d.CaseBraceBody = syntax.CaseBraceBodyMixesWithTheKeyword
	// A `-` or `?` behind a `${#` is the name here and stays the name when
	// text is left over behind it, where the other five re-read the `#` as
	// the parameter `$#`. With `set -- p q r`: `${#-w}` and `${#?w}` are bad
	// substitutions here and `3` there, and `${#-:-x}` is the length of
	// `${-:-x}` — 4 — where the five answer 3. A *bare* `${#-}` is the length
	// of `$-` in all six, which is why the flag is about the leftover rather
	// than about the name (#1242).
	d.ParamLengthOverASpecialNameIsFinal = true
	// And an operator behind that `${#` with nothing after it keeps the `#`
	// as the parameter here, where the other five scan the operator as a
	// name and refuse the whole expansion for not finding one: `${#+}` is
	// empty here and `${#=}` is `$#`, and both are bad substitutions in bash
	// 5.3, bash as `sh`, bash 3.2, dash and ksh93. With an operand every
	// column agrees — `${#+w}` is `w` and `${#=w}` is `$#` everywhere — so
	// the split is the empty word and not the operator (#4166).
	d.ParamLengthBareOperatorIsTheParameter = true
	// And a length over `$!` is not a shape this shell has at all: `${#!}` is
	// a bad substitution here where the other six answer `0`, the length of
	// an empty `$!`. Not because there is nothing to measure — `$!` reads
	// here and is `0` with no background job, where the six leave it empty —
	// and not a rule about special names, since `${#$}` and `${#?}` are
	// lengths here as everywhere. Deferred, not fatal: `${#!}` in a branch
	// never taken runs clean and `zsh -n` accepts the file (#2415).
	d.ParamLengthRefusesTheBangName = true
	// A `;` between the parentheses of an array literal stands exactly where a
	// newline already does: `a=( x; y )` holds two elements here, `a=( ; )` is
	// the empty array, and `a=( x; ; )` holds one. ksh93 takes only a single
	// one at the very end, which is the reading the other value records, and
	// every bash column refuses all four. `a=( x;; y )`, `a=( x & )` and
	// `a=( x && y )` are parse errors here as everywhere, which is what says
	// the `;` is specifically a separator rather than this shell being lenient
	// about operators (#1162).
	d.SemicolonInAnArrayLiteral = syntax.SemicolonSeparatesArrayElementsLikeANewline
	// `{ … } always { … }`, the try-always block. Measured 2026-09-07: the
	// word is positional and not reserved here — `always` alone is `command
	// not found`, `always() { :; }` defines a function and `echo always`
	// prints it — so what the flag adds is one production hanging off a brace
	// group. Nine files in a real `~/.zi` plugin tree are unparseable without
	// it, including zsh-autosuggestions, powerlevel10k's gitstatus and F-Sy-H
	// (#1216).
	d.TryAlways = true
	d.AnonymousFunction = true
	d.BareFunctionKeyword = true
	// A token no command could begin with, standing where an `if` or `elif`
	// clause's first command is due, is stepped over and the one after it is
	// refused: `if true; then ) echo X; fi` is `parse error near `echo''
	// here and names the parenthesis in the other four. See
	// syntax.Parser.clauseStepsOverWhatItCannotUse for the twelve rows.
	d.IfClauseStepsOverWhatItCannotUse = true
	// The same reach: a body may have nothing in it — `{ }`, `( )`, `while
	// cond; do done`, and a condition too. Every shape, and this shell alone.
	d.EmptyCompoundBody = true
	// A pipeline or and-or operator standing where a body or a condition
	// must begin is refused at a *reserved word*, where the rest of the
	// panel names the operator. Measured 2026-09-14 with `-n` over a script
	// file: a condition is named at the keyword that ends its header however
	// much stands in between — `if | :; then :; fi`, `if | :; :; then :; fi`
	// and `if | { :; }; then :; fi` are all `` `then' `` — and a body is
	// named at a keyword standing next and at the operator otherwise, so
	// `if :; then | fi` is `` `fi' `` and `for i in 1; do | :; done` is
	// `` `|' ``.
	//
	// `{ | }` is `` `|' `` — the closing brace is the one reserved word it
	// never names — and so are `( | )` and `case x in x) | ;; esac`.
	//
	// The set is the pipeline and and-or operators. An `&` there names
	// itself in this shell, which is the reverse of ksh93's answer and the
	// reason the two dialects need different sets (#2235).
	d.EmptyBodyBlame = syntax.BlameTheKeywordAfterIt
	d.EmptyBodyBlamed = map[syntax.Kind]bool{
		syntax.TokPipe:    true,
		syntax.TokPipeAmp: true,
		syntax.TokAndAnd:  true,
		syntax.TokOrOr:    true,
	}
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
	// `:` is a math token here wherever it stands, rather than text no
	// operator could be — see syntax.Dialect.ArithColonIsAToken.
	d.ArithColonIsAToken = true
	// And a numeral stops at the first character its base cannot use, where
	// the rest read every letter into the number and then refuse it — see
	// syntax.Dialect.ArithNumeralEndsAtABadDigit.
	d.ArithNumeralEndsAtABadDigit = true
	// And an underscore inside a numeral is a digit separator, skipped
	// rather than read: `$(( 1_0 ))` is 10 here where every other column
	// calls it a digit too great for base ten — see
	// syntax.Dialect.ArithDigitSeparator.
	d.ArithDigitSeparator = true
	// And the binary radix prefix, which this shell alone has: `0b101` is 5
	// here and an octal constant carrying a `b` everywhere else.
	d.ArithBinaryLiteral = true
	// And the operators do not bind in C's order. This shell's manual calls
	// the order below its *native mode* and documents C's beside it, with an
	// option — `c_precedences` — that picks between the two, which is as
	// explicit as a disagreement between shells gets.
	//
	// Measured 2026-09-15 from a script file, `env -i` with a scratch HOME,
	// against bash 5.3.15, ksh93u+ and dash, which agree with each other:
	//
	//	                 here   the other three
	//	1 << 2 + 1         5           8
	//	1 + 2 << 1         5           6
	//	1 << 2 * 2         8          16
	//	1 < 2 & 1          0           1
	//	6 | 1 + 1          8           6
	//	2 ** 1 | 3         8           3
	//
	// The last row is the one a reading of the first four would miss: `**`
	// is *looser* than the bitwise operators here, so `2 ** 1 | 3` is `2 **
	// (1 | 3)`. Parenthesized, every column agrees, which is what says this
	// is precedence and not a broken operator.
	//
	// See syntax.ArithPrecedenceShiftsAndBitwiseBindTighter for the whole
	// ladder.
	d.ArithPrecedence = syntax.ArithPrecedenceShiftsAndBitwiseBindTighter
	// `^^`, the logical exclusive-or, and the `^^=` beside it. This shell's
	// alone; see syntax.Dialect.ArithLogicalXor for where each sits and
	// docs/spec/grammar/arithmetic.md for the ladders.
	d.ArithLogicalXor = true
	// `**=`, exponentiation's compound assignment, which is this shell's
	// alone among the four columns that parse `**` at all. Measured
	// 2026-09-26 with `n=2; $(( n **= 3 ))`: 8 here, and an arithmetic
	// syntax error in bash 5.3, ksh93u+ and BusyBox ash 1.37.0 — so it is a
	// flag of its own rather than something syntax.Dialect.ArithExponent
	// implies. See syntax.Dialect.ArithExponentAssign (#4663).
	d.ArithExponentAssign = true
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
	// And a group the input runs out of is a *word* here rather than input
	// still to come: `print -r -- a(b` is `bad pattern: a(b` in zsh 5.9.2 and
	// the word itself under `unsetopt badpattern`, where bash with `extglob`
	// and ksh93 both answer their own spelling with an unmatched-parenthesis
	// parse error. The refusal has to be the matcher's for the option to be
	// able to withhold it (#4645).
	d.UnterminatedPatternGroupIsAWord = true
	// And a `|` outside every group, which is the same alternation one level
	// out, and only for a bar a value supplied: `L='a|b'; [[ a = ${~L} ]]`.
	// Measured on zsh 5.9.2 in a condition, in a `case` arm, under `setopt
	// globsubst` and in a glob alike (#1497). A bar *written* in a `${…}`
	// operand is an ordinary character here — `v=abc; ${v#a|ab}` is `abc`
	// (#2168) — which is not the parse error the other two spellings are.
	d.PatternTopLevelAlternation = syntax.TopLevelAlternationFromAValue
	// The same alternation reaching the `case` arm's own pattern list, where
	// one of the alternatives may be written as nothing: `(|https|git|ftp)`
	// matches one of those schemes or none at all. `~/.zi/bin/lib/zsh/install.zsh`
	// is written that way and the other four shells refuse the line.
	d.CasePatternMayBeEmpty = true
	// And inside the arm's parentheses the `|` is only ever the separator:
	// `(a|&b)` is blamed on the `&` there where every other shell with the
	// operator blames `|&` (#1111).
	d.CasePatternListPipeIsOnlyASeparator = true
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
	// A `;` may stand in the header wherever a newline may — `case x; in`
	// and `case x in;` both run here, and both are a syntax error in the
	// other six. Nine of the completion functions this shell ships open a
	// `case` that way, so without it they cannot be read at all. `;;` and
	// `&` are not this: both are refused in the same position by this shell
	// too. See syntax.Dialect.CaseHeaderSpansSeparators.
	d.CaseHeaderSpansSeparators = true
	// A loop's variable may be a positional parameter's number: `for 1 in a
	// b` sets `$1` on each pass. The other six columns refuse the header,
	// calling `1` an invalid identifier or a bad loop variable. Seven of the
	// completion functions this shell ships are written that way. See
	// syntax.Dialect.ForNameMayBeAPositionalParameter.
	d.ForNameMayBeAPositionalParameter = true
	// A `(` at the front of a condition's *third* word belongs to that word:
	// `[[ 9 -gt ( 1 + 2 ) ]]` is true here and refused in the other six, and
	// `[[ -prefix 1 (f|ht)tp:// ]]` is the completion system's own spelling.
	// See syntax.Dialect.ConditionOperandMayOpenWithAGroup.
	d.ConditionOperandMayOpenWithAGroup = true
	// A condition with no term in it is reported at the token **behind** the
	// `]]` rather than at the `]]`: the closer is consumed and whatever
	// stands after it is what the complaint names, at that token's own line.
	// Measured 2026-09-18 over `[[ ]]`, where bash names the `]]` at line 1
	// on every route and this shell names a newline at line 2 in a file that
	// has one, `echo` at its own line where a command follows, `;` where one
	// stands, and the `]]` itself where the input simply ends. It is not the
	// word reading ksh93 has — `[[ ]] ]]` and `[[ ]] == x ]]` are refusals
	// here and run there — only where the refusal lands (#2964).
	d.ConditionTermMissingBlamesTheTokenAfterTheCloser = true
	// A condition term's first word may be the last thing on its line, with
	// the operator or the `]]` that decides it written on the next. This
	// column alone: bash 5.3, bash 3.2 and ksh93 all name the newline there.
	// See syntax.Dialect.ConditionNewlineMayFollowATermsFirstWord (#3627).
	d.ConditionNewlineMayFollowATermsFirstWord = true
	// A written-out reserved word keeps its reading behind an assignment
	// prefix here, where bash 5.3, bash 3.2, ksh93, dash and BusyBox ash all
	// drop it and read the word as an ordinary command name — so the
	// complaint lands on the word rather than on whatever closes what it
	// opened. Three of the rows were worse than a
	// wording without it: `v=x time :` ran /usr/bin/time, `v=x !` was a
	// command that could not be found, and `v=x [[ -n a ]]` reached the
	// pattern matcher. See
	// syntax.Dialect.ReservedWordStandsBehindAnAssignmentPrefix (#3560).
	d.ReservedWordStandsBehindAnAssignmentPrefix = true
	// And a redirection written in *front* of a compound command belongs to
	// it, which is ordinary zsh and which this shell refused for every
	// compound it has. ksh93 takes one before a parenthesized command alone;
	// bash and dash take none. See
	// syntax.RedirectionBeforeACompoundPolicy (#3560).
	d.RedirectionBeforeACompound = syntax.RedirectionMayPrecedeAnyCompoundCommand
	// A `{ … }` written immediately after `$$` is a run of characters: a
	// blank, a newline or an operator inside is text, and the braces are a
	// brace list nowhere — though a range written straight into them still
	// expands, which syntax.Span.PidBrace records. The other six columns split the word at the blank and
	// refuse the `;`, and every one of them reads the `>` as a redirection.
	// One shipped completion function is written that way — with `$$` where
	// `$` was meant — and cannot be read at all without it. See
	// syntax.Dialect.PidBraceGroupIsText.
	d.PidBraceGroupIsText = true
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
	// `[[ -prefix … ]]` and `[[ -suffix … ]]`, the completion-context tests.
	// They are in the grammar unconditionally here and the restriction is on
	// where they may *run* — `condition can only be used in completion
	// function`, status 1, and fatal — which is why this is a parser flag and
	// not a builtin's refusal: a completion function is a file, and a file
	// that will not parse never gets as far as the restriction. Two files in
	// an ordinary `~/.zi` tree reach it (#1879).
	d.CompletionConditions = true
	// And a known conditional operator standing with the wrong number of
	// operands parses here and is refused when it runs: `echo pre; [[ -n x y
	// ]]` writes `pre` and then `unknown condition: -n`, where bash and
	// ksh93 name the offending token while reading and never run the `echo`
	// (#965).
	d.ConditionIsResolvedWhenItRuns = true
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
	// And a body that is not a brace group reaches to the end of the and-or
	// list: `function a; echo X && echo Y` binds both, where
	// `function a { echo X; } && echo Y` binds only the group. See
	// syntax.Dialect.FunctionKeywordBodyIsAnAndOrList (#1832).
	d.FunctionKeywordBodyIsAnAndOrList = true
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
	// And a name holding a bare `*`, `?` or `[` is the one group neither of
	// those two flags carried: this shell matches such a word against the
	// **filesystem** and defines one function per match, so the name is a
	// pattern rather than a name. Both spellings, measured 2026-09-25 in a
	// directory holding `ax` and `bx`: `?x() { :; }` and `function ?x { :; }`
	// each leave `ax` and `bx` defined, and a pattern matching nothing is
	// `no matches found: <word>` at 1 with the next line unrun.
	//
	// It was refused while reading here, which bought a visible answer at the
	// cost of a parse failure where this shell's own `-n` accepts — two of
	// the files the zsh column's static read refused were refused for it. See
	// syntax.Dialect.FunctionNameIsFilenameGenerated (#4437).
	d.FunctionNameIsFilenameGenerated = true
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
	d.ParamArrayZip = true
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
	// A `"` written inside a `${ }` operand opens a run whose escaping starts
	// over, so the brace that would close the expansion is not escapable in
	// it. This shell alone: measured 2026-09-10 with `u` unset,
	// `printf '[%s]' "${u-"A\}B"}"` is `[A\}B]` here and `[A}B]` in dash,
	// bash 5.3.15, that build as `sh`, bash 3.2.57, ksh93u+ and BusyBox ash —
	// six columns saying the whole body of the expansion is the escaping
	// context and this one saying the context resets at the quote.
	//
	// The engine gave this reading in every dialect before the flag existed,
	// arrived at without anyone choosing it, so what changed for the other
	// four is the answer and not the construct (#2001).
	d.NestedQuoteResetsOperandEscapes = true
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
	// A single quote written inside a double-quoted `${ … }` quotes nothing
	// here, in any operand, so the expansion always ends at the first `}`.
	// Measured 2026-09-12: `v=Vx}y; echo "[${v#'a}b'}]"` is `[Vx}yb'}]` here
	// where every other column answers `[Vx}y]` — this shell alone reads a
	// *pattern* operand's quote as an ordinary character too. See
	// Dialect.QuoteProtectsTheClosingBrace (#2399).
	d.QuoteProtectsTheClosingBrace = syntax.BraceQuoteProtectsNothing
	// A backslash holds a `]` back from ending a `${a[ … ]}` subscript here
	// and a quote does not — the narrowest row of the panel. Measured
	// 2026-09-15 on zsh 5.9.2: `typeset -A a; a[x\]y]=1; print -r --
	// "${a[x\]y]}"` is `1`, while `${a['0]'+1]}` stops at the quoted bracket
	// and reports a math error against `'0`, where bash and ksh93 read the
	// whole of `'0]'+1`. See
	// Dialect.SubscriptQuoteProtectsTheClosingBracket (#2299).
	d.SubscriptQuoteProtectsTheClosingBracket = syntax.SubscriptBackslashQuotes
	// A parameter written without braces carries a subscript here, and `$#a`
	// is a count rather than `$#` with a letter after it. Measured 2026-09-05
	// on zsh 5.9.2: `a=(x y z); echo $a[1]` prints `x` and `echo $#a` prints
	// `3`, where bash 3.2, bash 5.3 and dash print `x[1]` and `0a`. This
	// shell alone, which is why it is set here and nowhere else.
	d.BareSubscript = true
	d.BareParamFlags = true
	d.BareParamModifiers = true
	// A line continuation written between a `$` and what it introduces stops
	// the `$` at everything but a bare parameter inside double quotes, and at
	// nothing outside them. Measured 2026-09-16 from script files with `x=5`
	// and `set -- a b`: `"$\⏎x"` is 5 and `"$\⏎1"` is `a`, where
	// `"$\⏎(echo hi)"` is the text `$(echo hi)` and `"$\⏎[1+2]"` is
	// `$[1+2]`. Unquoted, every one of those reads through the pair as it
	// does in bash.
	//
	// The brace is the one it stops at *and* drops the `$` for: `"$\⏎{x}"`
	// is `{x}` here and `${x}` in the other shell that stops at it.
	d.ContinuationStopsADollarAtInDoubleQuotes = syntax.EveryDollarForm &^ syntax.DollarBareParameter
	d.DollarGoesWhenAContinuationStopsItAtABrace = true
	// A line continuation between the two `)` of an arithmetic expansion
	// parts them here, so `echo "[$(( 1 + 2 )\⏎)]"` is a command
	// substitution holding the subshell `( 1 + 2 )` — `command not found: 1`,
	// and the expansion empty — where bash 5.3, bash 3.2 and dash answer 3.
	// The *opener* is not this: `echo "[$(\⏎( 1 + 2 ))]"` is 3 here as it is
	// in bash, which is why the two ends are two fields.
	d.ContinuationPartsTheArithmeticCloser = true
	// The scan that looks for an arithmetic command's `))` is blind to
	// quoting here, so `((echo "a)b"))` and `((echo 'a)b'))` give the
	// arithmetic reading up and print `a)b` as two groupings, where the three
	// bash columns call each an arithmetic syntax error. Measured 2026-09-15;
	// a backslash and a command substitution are stepped over in every column
	// and are not this. See the flag for the five rows.
	d.ArithCommandScanIgnoresQuoting = true
	// And the same at the `$((` fallback, which is a second field because
	// the two scans were measured separately: `echo $(( '0)' + 1 ))` runs a
	// command named `0)` here where the bash columns refuse the expression
	// (#3530).
	d.ArithSubstScanIgnoresQuoting = true
	// A run of digits after an unbraced `$` is one positional parameter here.
	// Measured 2026-09-15 with `set -- 1 2 3 4 5 6 7 8 9 ten eleven`: `$10` is
	// `ten` and `$11` is `eleven` in this shell, where bash 5.3, bash 3.2,
	// bash as `sh`, ksh93, dash and BusyBox ash all answer `10` and `11` —
	// `$1` with the next digit left in the word. `${10}` is the tenth in all
	// seven, so the braces are what the other six need and this shell does
	// not. The run is read as a number, so `$01` is the first parameter and
	// `$00` is the shell's own name (#2879).
	d.MultiDigitPositional = true
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
	// And a bracketed specifier says what base the *result* is written in:
	// `$(( [#16] 255 ))` is `16#FF` and `$(( [##16] 255 ))` is `FF`, with the
	// second `#` dropping the `base#` in front. Measured 2026-09-12 against
	// the rest of the panel, every one of which reads the `[` as an operand
	// it cannot have. A prompt theme reaches it to render a color as hex,
	// where the refusal was not a wrong number but no value at all (#2095).
	d.ArithOutputFormat = true
	// `a |& b`, read exactly as bash 5.3 reads it: the left side's standard
	// error joins its standard output on the pipe. Not the ksh93 reading of
	// the same two characters, which is a coprocess and a different slot in
	// the grammar — measured, because ksh93 refuses `a | ; b` and accepts
	// `a |& ; b`, so its `|&` ends a command where this one joins two.
	// A `!` with no pipeline after it is a pipeline of its own here, and it
	// reaches wherever the *list* ends — `( ! )`, `{ ! }`, `case x in x) ! ;;`
	// and `! || echo two` all run — as well as a `;`, a newline and the end
	// of input. Not a `&`: `! & echo x` is a parse error here where bash
	// takes it, which is the same boundary OpenEndedAndOr has in this shell.
	d.BareNegationReach = syntax.BareNegationWhereAListEnds
	// And a second `!` is refused outright here, which is what makes the
	// toggle a separate question from the reach: this shell has one and not
	// the other (#948).
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

	// This shell reads a `#!` line itself rather than leaving it to the
	// kernel, which is two answers and not one.
	//
	// A line naming nothing is refused: measured 2026-09-25, `printf
	// '#!\necho hi\n' > empty; chmod +x empty; empty` is
	// `zsh:1: exec format error: empty` at 126 here and `hi` at 0 in the
	// other six columns — the same for a `#!` followed only by blanks. See
	// TestAnEmptyInterpreterLineIsRefused.
	s.EmptyInterpreterLineIsNotAScript = interp.Yes
	// And a line naming a word with no slash in it is looked up on PATH:
	// `printf '#!cat\necho hi\n' > tstcmd; chmod +x tstcmd; tstcmd` prints
	// the file's own two lines at 0 here, where the three bash columns say
	// `cat: bad interpreter` at 126 and dash, ksh93 and BusyBox ash say
	// `not found` at 127. See TestASlashlessInterpreterIsFoundOnPath.
	s.SlashlessInterpreterIsPathSearched = interp.Yes
	s.CommandNotFoundStatusIsNotFound = interp.No
	// A pathname operand is written back as typed, through `command -v`,
	// `type` and `whence -p` alike: `./bb/tool` is `./bb/tool`. Measured
	// 2026-09-14 against 5.9.2.
	s.APathnameOperandIsReportedAbsolute = interp.No
	// The command that named a `>(cmd)` waits for its body here, which is
	// what makes `printf x | tee >(sleep 3) >/dev/null` take three seconds
	// in this shell and none in bash and ksh93 (#2197).
	s.WritingSubstitutionIsWaitedForAtTheCommand = interp.Yes
	// And it names the path after /proc/self/fd where that directory is
	// there: measured 2026-09-21, `echo <(true)` is `/proc/self/fd/11` in
	// the pinned Linux image and `/dev/fd/11` on the panel machine, while
	// bash and ksh93 write `/dev/fd/N` in both. One image, one /dev/fd
	// symlink, and this shell alone writes the target (#3986).
	s.SubstitutionPathPrefersProcSelfFd = interp.Yes
	s.SetFTurnsOffGlobbing = interp.No
	// A numeric signal goes to `kill(2)` unchecked here, so `kill -99 $$` is
	// `kill <pid> failed: invalid argument` at 1 — the errno, printed —
	// rather than a word refused. Measured 2026-09-16; `kill -s 99` is still
	// `unknown signal: SIG99`, since `-s` takes a name.
	// `kill -n signum` is an option here, and `-s` with nothing after it is
	// an option missing its argument rather than a signal named `s`.
	s.KillReadsTheNumberOption = interp.Yes
	s.KillOptionWithNoArgumentIsASignalName = interp.No
	s.KillSendsASignalNumberItCannotName = interp.Yes
	// And `-s` really does take a name here: a word of digits after it is
	// the name it is not. Measured 2026-09-17 — `kill -s 9 999999` is
	// `unknown signal: SIG9` and the listing hint at 1, where bash, ksh93,
	// dash and BusyBox ash all reach a real send. The number *out of* range
	// is the axis above and this is the same position with one in it, which
	// is where the three that send part company with this column (#3544).
	s.KillNameOptionReadsANumber = interp.No
	// One name older than the table's own: `IOT` is signal 6 here, as it is
	// in ksh93 and as it is not in bash. Measured 2026-09-17 — `kill -l
	// IOT`, `kill -s IOT`, `kill -IOT` and `trap 'x' IOT` are all the ABRT
	// the table carries, and `kill -l 6` is `ABRT`, so this shell reads the
	// older spelling and never writes it (#3536).
	s.SignalNamesTheShellAlsoReads = "IOT=ABRT"
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
	// And what this shell does spend `-f` on: the startup files, whose name
	// in its own option namespace is `norcs`. The letter and the name are
	// one state, which is measured from both ends — `set -f` puts `norcs`
	// in a bare `setopt` listing and `f` in `$-`, `set -o norcs` puts the
	// letter there without the letter having been written, and `set +f` in a
	// `zsh -f` shell takes both away again (#1542). See setopt.go's `rcs`
	// entry, which is the same state read the other way up.
	s.SetFLetterOption = "norcs"
	// `set -h` is histignoredups here — a history option — not the command
	// tracking the letter abbreviates in bash and ksh93.
	s.SetHLetterTracksCommands = interp.No
	// And so nothing at this shell's startup spells command tracking, which
	// leaves its default to be said outright: it is **on**. Measured on zsh
	// 5.9.2, 2026-09-25, in `zsh -f -c` — `[[ -o hashcmds ]]` is 0,
	// `${options[hashcmds]}` and `${options[hashall]}` are both `on`, a bare
	// `unsetopt` names the row `nohashcmds`, and `set -o` writes
	// `nohashcmds            off`, which is the spelling and the state a
	// listing gives an option that is on by default.
	//
	// **The shell's own default is the noun, not what it does.** Both shells
	// hash a command they ran, in every state of the option, so a probe that
	// only ran `ls` and read `hash` back agrees under either reading — which
	// is why this was reported off for as long as it was. The pair that
	// discriminates holds the *hashing* fixed and moves the report:
	// `ls >/dev/null; hash` lists `ls` here and in zsh alike while
	// `[[ -o hashcmds ]]` parted, so the disagreement was never the hashing
	// (#4533). The option is not inert either — `unsetopt hashcmds; ls
	// >/dev/null; hash` is empty in both — so what was wrong was the state a
	// fresh shell starts in and nothing else.
	s.CommandTrackingStartsOn = interp.Yes
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
	// And on the other route too: measured 2026-09-12 on `-i -c` through a
	// pseudo-terminal, `[1] <pid>` as the job starts and `[1]  + done` after the
	// `wait` that reaps it.
	s.InteractiveCommandStringAnnouncesJobs = interp.Yes
	// Written where the job ended rather than where a prompt is drawn, which
	// the same run says: `-i -c` never draws one and zsh writes the row.
	s.FinishedJobNoticeNeedsAPrompt = interp.No
	// And the moment the job ends rather than before the next prompt, which
	// is `NOTIFY` — on out of the box here and off in the rest of the panel.
	// Measured 2026-09-25 on a pseudo-terminal: `sleep 0.4 &` writes `[1]
	// <pid>` and then `[1]  + done       sleep 0.4` about four tenths of a
	// second later, with nothing typed in between.
	//
	// The one axis here a running script can move, through `unsetopt NOTIFY`
	// — so the entry in setopt.go writes this rather than remembering the
	// request. Until #4524 it was remembered and ignored, and this shell
	// behaved as `nonotify` in every session (#4524).
	s.FinishedJobNoticeArrivesAtOnce = interp.Yes
	// A job started with `&` reads the shell's own standard input here, where
	// the other five hand it an empty one — measured 2026-09-07,
	// `sh -c '/bin/cat & wait; echo ---; /bin/cat' < f` writes the file's line
	// *before* the marker in zsh 5.9.2 and after it in the other five. POSIX
	// XCU 2.9.3 specifies the majority; this is the divergence, and it is
	// this shell's to keep.
	s.BackgroundJobInput = interp.BackgroundJobInputIsTheShells
	// unanswered BackgroundJobInputIsOnlyTheShellsOwn: this shell substitutes
	// nothing at all for a background job's input — the line above is the
	// statement of that — so which streams a substitution reaches has no
	// answer to observe here. Measured 2026-09-23 on 5.9.2: the job reads the
	// shell's own input, an `exec`-installed one, an enclosing loop's
	// redirection and a pipeline alike (#4153).
	// `$!` before any background command is `0` here and nothing in the other
	// five columns — a number nothing ever had. Measured,
	// `sh -c 'echo "[$!]"'` writes `[0]`, and that zero is *set*: `${!-unset}`
	// is `0` and `${!+set}` is `set`, so `set -u; echo "[$!]"; echo "st=$?"`
	// writes `[0]` then `st=0`.
	//
	// It is the quiet side of the `set -u` split with ksh93 and for a
	// different reason — ksh93 has nothing there and does not mind, zsh has a
	// value — and it is worth writing down beside this shell refusing an
	// unset `$1`, which it does.
	s.LastBackgroundPid = interp.LastBackgroundPidZero
	s.ProcessSubstitutionIsTheLastBackgroundJob = interp.No
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
	// The option namespace's own way of saying `-i`, and the shell prompts for
	// it: measured 2026-09-16 with a program on a pipe so nothing else could
	// make it interactive, `zsh -f -o interactive` draws a prompt, runs the
	// line and draws another, and reads `~/.zshrc` exactly as `-i` does.
	// `+o interactive` and `-o nointeractive` are the other direction and draw
	// none, on a pipe and — measured through a pseudo-terminal — on a terminal
	// too, where the shell would otherwise have prompted (#3195).
	s.InteractiveOptionName = "interactive"
	s.NonInteractiveOptionName = "nointeractive"
	// zsh has a history expander and starts a prompt with it on — measured
	// 2026-09-15 through a pseudo-terminal with a two-row prompt, where
	// `echo one two three` then `echo !!` echoes the expanded line and runs
	// it, with nothing configured.
	//
	// The letter is not `H` here and must not be: zsh's `set -H` is
	// `rmstarsilent`, measured by diffing `setopt` across it, and this
	// dialect's own letter table answers that before the core's `-H` is
	// reached. The spellings that reach the expander are `setopt banghist`
	// and the borrowed `set -o histexpand`.
	s.HistoryExpansion = interp.Yes
	// An event's words are the shell's own, a `${ }` included. See
	// Semantics.HistoryWords.
	s.HistoryWords = interp.HistoryWordsShellBraces
	// And a `:q` quotes where it is written. See
	// Semantics.HistoryQuoteModifierInPlace.
	s.HistoryQuoteModifierInPlace = interp.Yes
	// A quote or a backquote against the event character is ordinary text
	// here and part of the event's name in bash and ksh93, a second event
	// character closes the name it is in, and `!{…}` is the braced form
	// neither of the others has. Measured 2026-09-18 at a prompt: `echo
	// "T!'x E"` prints its own text, `X!ab!cdY` is `event not found: ab!`,
	// and `!{x}` is the event `x`. See
	// Semantics.HistoryQuoteEndsAnEventReference,
	// Semantics.HistoryEventCharClosesAnEventName and
	// Semantics.HistoryBracedEventReference.
	s.HistoryQuoteEndsAnEventReference = interp.Yes
	s.HistoryEventCharClosesAnEventName = interp.Yes
	s.HistoryBracedEventReference = interp.Yes
	// And a `$` does not end a word designator: `!!:$-3` is a range this
	// shell cannot make rather than the last word with `-3` written after
	// it, which is `no such word in event`. Measured the same day over
	// `echo a b c d e`. See Semantics.HistoryLastWordEndsTheDesignator.
	s.HistoryLastWordEndsTheDesignator = interp.No
	// No keyword option. The letter is not the option here: zsh spells
	// `interactivecomments` with `-k` and answers `no such option` to `set
	// -o keyword`, both measured 2026-09-16, so a `name=value` word after
	// the command name stays a positional and that is zsh's answer rather
	// than a gap. See Semantics.KeywordAssignments.
	s.KeywordAssignments = interp.No
	// unanswered ExportContainerLetterNeedsAValue: `export` takes no container
	// letter here, which Semantics.ExportOptions is the statement of — it is
	// empty in this dialect, so `export -A` is a bad option and the question has
	// no line to be about. Measured 2026-09-23.
	// unanswered RestrictedModeIsLeftByTheLetter: this shell *does* have the
	// letter and a restricted mode of its own — `set -r; cd /` is `cd:3:
	// restricted` in zsh 5.9.2, measured 2026-09-22 — and that mode is not
	// built here: the letter reaches a `setopt` name this dialect records and
	// nothing acts on. So there is no mode for the axis to move.
	// TestSetTakesTheRestrictedLetterAndDoesNothing pins what is there today,
	// and #4205 filed the mode rather than claiming it.
	// unanswered RestrictedFreezeIsAReadonly: the same — no mode here, so no
	// name is ever mode-frozen.
	// unanswered RestrictedBuiltinRefusalIsFatal: the same — none of the three
	// refusals exists here.
	// unanswered KeywordPromotesADeclarationsOperand: there is no keyword
	// option here to reach a declaration with. `-k` is this shell's
	// `interactivecomments` and `set -o keyword` is `no such option`, both
	// measured 2026-09-16, so the question cannot be put to it.

	// The seven declaration commands are reserved words here, and the
	// assignment rule for their operands belongs to the word as written:
	// under `setopt shwordsplit`, `$cmd a=$b`, `\typeset a=$b`, `'typeset'
	// a=$b`, `"typeset" a=$b`, `type'set' a=$b`, `noglob typeset a=$b` and
	// `builtin typeset a=$b` all split the value, where `typeset a=$b`, an
	// alias for it and `nocorrect typeset a=$b` keep it whole. Measured
	// 2026-09-16 on zsh 5.9.2, for typeset, declare, export, local, readonly,
	// float and integer. See #3315.
	s.DeclarationCommandWord = interp.DeclarationByUnquotedLiteralWord
	// And `command` in front of one does not keep the rule. The question can
	// only be put to this shell under `setopt posixbuiltins`, since `command
	// typeset` is `command not found` without it: `setopt shwordsplit
	// posixbuiltins; command typeset a=$b` is `[x]` there, and so is
	// `command -p typeset a=$b`. Measured 2026-09-16 on zsh 5.9.2. See #3341.
	s.CommandPrefixKeepsADeclaration = interp.No

	s.HistoryExpansionAtAPrompt = interp.Yes
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
	// And the system-wide file in front of each of those, which this shell
	// is the one in the panel with enough slots to pin the *position* of.
	// Measured 2026-09-12 with `setopt sourcetrace`, which names each file
	// as it is read: `zsh -o sourcetrace -l -i` writes `~/.zshenv`,
	// `/etc/zprofile`, `~/.zprofile`, `/etc/zshrc`, `~/.zshrc`, `~/.zlogin`.
	// So each system file comes first in its own slot rather than all of
	// them coming before all of the person's — a shell that hoisted them
	// would read `/etc/zshrc` before `~/.zprofile`, and `~/.zprofile` is
	// where a person sets `$PATH`.
	//
	// `zshenv` and `zlogin` are the manual's answer rather than a measured
	// one, and the difference is worth stating: neither `/etc/zshenv` nor
	// `/etc/zlogin` exists on the machine this was measured on, so no probe
	// can see them read. The slot each occupies *is* measured, because the
	// two files that do exist each land first in theirs.
	//
	// Both have since been *seen* read, on the other platform, which is the
	// half of the blind spot the sentence above could not close: a Debian
	// zsh keeps all four in `/etc/zsh` and traces `/etc/zsh/zshenv` on every
	// invocation. Names and not paths, so this is unchanged by that — see
	// [SystemStartupDirectory], which is where the directory differs (#3987).
	s.SystemStartupFiles = interp.SystemStartupFiles{
		Unconditional: "zshenv",
		Login:         "zprofile",
		Interactive:   "zshrc",
		LateLogin:     "zlogin",
	}
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
		// And one that drops root's files and keeps the person's, which no
		// other shell in the panel has. Measured 2026-09-12: `zsh -d -l -c`
		// still reads `.zshenv`, `.zprofile` and `.zlogin`, and every one of
		// them reports the inherited `${PATH%%:*}` rather than
		// `path_helper`'s, so `/etc/zprofile` did not run.
		SuppressSystem: "-d --no-globalrcs",
	}
	// What `zsh --version` writes, on standard output at status 0 — measured
	// 2026-09-11, one line and no more.
	s.VersionOption = interp.VersionOption{Spellings: "--version", Text: versionLine()}
	// And `--help`, which this shell answers with a usage block on standard
	// output at 0 — measured 2026-09-18 on zsh 5.9.2, and `no such option:
	// help` at 1 here until now, since the front end hands any unmatched
	// `--word` to the option table and this is not an option name (#3156).
	//
	// The whole answer is the **trailer**, which is unusual and is forced by
	// the measurement: Diagnostics.InvocationUsage is the block a *refused*
	// option gets as well as the one `--help` gets, and this shell prints no
	// block at all under a refusal — `zsh -o badname -c :` and `zsh --badname
	// -c :` are each one line, `no such option: badname`, with nothing after
	// them. So a block hung there would start appearing where the reference
	// prints none. The trailer is the one piece of the answer that is this
	// option's alone, and it is the piece that reads the shell's own name.
	//
	// Generated from this dialect's three option tables rather than
	// committed — see helpBlock, where the measurement that settles that
	// choice is.
	s.HelpOption = interp.HelpOption{Spellings: "--help", Trailer: helpBlock()}
	// And `--emulate MODE`, which is the `emulate` builtin run before a line
	// is read rather than a spelling of an option name. zsh alone on the
	// panel; measured 2026-09-18 on 5.9.2 under `env -i PATH=/usr/bin:/bin
	// LC_ALL=C`, and the rows are in interp.EmulationOption.
	//
	// Both refusals are the shell's own words at status 1, where a word this
	// front end cannot place exits 2 — `--emulate` with nothing after it is
	// `--emulate: argument required`, and `-x --emulate sh` is `--emulate:
	// must precede other options`. The mode itself is never judged here: an
	// unknown one is the builtin's silence, which is the same answer `emulate
	// fish` gives at a prompt.
	//
	// And the *name*, which asks the same question without an option word:
	// the first letter of argv[0]'s basename picks the mode this shell starts
	// in. Measured 2026-09-26 on 5.9.2 by copying the binary under each name
	// — `s`, `sx`, `shell`, `shx`, `b`, `bash` and `bsh` all report `sh`, `k`
	// and `ksh` report `ksh`, `c` and `csh` report `csh`, and `xsh`, `mysh`,
	// `ash`, `dash`, `fish` and `SH` report `zsh`. A leading dash is stripped
	// first, so the login spelling `-sh` is the name too, and then one
	// leading `r`: `rsh` is sh emulation, `rzsh` and `rr` are not. The rows
	// are in interp.EmulationOption.NameInitials.
	s.EmulationOption = interp.EmulationOption{
		Spellings:        "--emulate",
		Builtin:          "emulate",
		MissingArgument:  "%s: argument required",
		OutOfOrder:       "%s: must precede other options",
		Status:           1,
		NameInitials:     "s=sh b=sh k=ksh c=csh",
		NameDropsInitial: "r",
	}
	// `-b` ends the option reading at the end of the word it is written in,
	// so every word after that one is an operand — which is why `zsh -b -c
	// cmd` is a failure to open a file called `-c` rather than a command
	// that ran. The panel's only such letter; five of the other six spend
	// `b` on the option that reports a finished background job at once, and
	// this shell refuses it at `set` while reading it at invocation, which
	// is what says it is the invocation's alone. Measured 2026-09-19; the
	// rows are in interp.Semantics.EndOfOptionsInvocationLetter.
	s.EndOfOptionsInvocationLetter = "b"
	// The parameter an `autoload`d name is looked up on, and the panel's only
	// one: `autoload -Uz is-at-least` finds its file on `$FPATH` and nothing
	// else does. Measured under `-f`, so it is the shell's own value and not a
	// startup file's — which is also what says the value survives the escape
	// hatch above. Named here and filled in by the front end, because which
	// directories is a fact about where a shell was installed; see
	// interp.Semantics.FunctionSearchVariable.
	s.FunctionSearchVariable = "FPATH"
	// `+i` takes the prompt back, the letter's half of the name above.
	// Measured 2026-09-16 on zsh 5.9.2 with the program on a pipe: `zsh -i +i
	// -c 'echo $-'` reports `569X`, which is what a shell started with no
	// letter at all reports, and `zsh -i -c` reports it with `i` in front.
	s.PlusSignedInteractiveLetterStillPrompts = interp.No
	s.CommandStringShowsCInDollarDash = interp.No
	s.LoginShowsLInDollarDash = interp.Yes
	s.CommandStringShowsSInDollarDash = interp.No
	// And the order, which is the simplest in the panel and the only sorted
	// one: the whole string by byte, so the digits lead, the capitals follow
	// and the lowercase letters come last. Measured 2026-09-12 on zsh 5.9.2,
	// the interactive row through a pseudo-terminal:
	//
	//	set -f; set -u; set -e   569Xefu
	//	set -C                   569CX
	//	set -C -e                569CXe
	//	set -o noglob            569FX
	//	set -e -C, on stdin      569CXes
	//	-l -c                    569Xl
	//	-i -c, at a terminal     569XZim
	//
	// The `-C` rows are what make it a sort rather than an append: the
	// letter lands in front of a startup letter this shell already held.
	//
	// The string spells the sort out for **every** letter this shell can
	// show rather than only the handful the rows above name, and that is a
	// correction rather than padding: the thirty letters #2578 added were
	// refused when it was written, so a string naming only the letters that
	// could then be produced left each new one to "keeps its produced place
	// and follows the ones it does" — which put `D` after `X` where zsh puts
	// it before. Measured on the same binary, one run rather than thirty:
	//
	//	set -T -Q -D -u -e -w -y -h -p -G -M   569DGMQTXehpuwy
	//	set -Y -B -a -C -F                     569BCFXYa
	//
	// Both are the whole string by byte with nothing else to explain, which
	// is why writing the byte order out in full is the same claim the rows
	// make and not a wider one.
	s.DollarDashLetterOrder = "569" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz"
	// The panel's holdout: `echo hi >&-` is status 0 here and 1 in the other
	// three — the text is quietly lost and nothing is said about a simple
	// command's own closed stream. What zsh prints when the stream was
	// closed by `exec >&-` instead — a `write error` with the status still 0
	// — is measured in docs/spec/semantics.md and not reproduced.
	s.BuiltinWriteErrorFailsTheCommand = interp.No
	// And the opposite answer for the other errno, which is the half of the
	// pair this shell reverses. A builtin writing into a pipe nobody is
	// reading, with SIGPIPE disarmed, reports 1 here -- where `echo hi >&-`
	// reports 0 -- and says so twice: `zsh:echo:N: write error: broken pipe`
	// from the builtin, then `zsh:N: write error: broken pipe` from the
	// stream. ksh93 is the mirror image of both rows (#770).
	s.BrokenPipeWriteErrorFailsTheCommand = interp.Yes
	s.ArithIntegerOperatorRefusesFloat = interp.No
	// A numeral a double cannot hold saturates: `$((1e400))` is `Inf` and
	// `$((-1e400))` is `-Inf`, the same answers the arithmetic gives for a
	// value that overflowed while being computed.
	s.ArithFloatOverflowIsZero = interp.No
	// A negative exponent is a float answer here, not a refusal: `2**-1`
	// is 0.5.
	s.ArithNegativeExponentIsError = interp.No
	s.ArrayScalarIsTheWholeArray = interp.Yes
	// Off in a shell that has not asked for it: `x${a}y` on `a=(1 2)` is
	// the one word `x1 2y` here, and the two words `x1y x2y` only under
	// `setopt RC_EXPAND_PARAM`, which is the one option in the panel that
	// moves this. The setopt.go entry writes the axis rather than
	// remembering the request (#4549), so `(setopt rcexpandparam)` stays in
	// the subshell. The per-expansion `${^a}` spelling does not read this
	// field at all — its parity wins — which is what keeps the flag and the
	// option one mechanism.
	s.ParamExpansionDistributesOverTheWord = interp.No
	// GLOB_ASSIGN is **off**, so a plain assignment's right-hand side is
	// text: `a=*.txt` stores the six characters here as it does in every
	// other column. The setopt.go entry writes this axis rather than
	// remembering the request (#4638), so `(setopt globassign)` stays in the
	// subshell and `emulate -R` puts it back with the rest of the vector.
	//
	// The option is what the *assignment* consults and not what makes a word
	// a pattern: `a=(*.txt)` globs in either state, because an array
	// literal's elements are ordinary words.
	s.ScalarAssignmentValueIsGlobbed = interp.No
	// A traced array literal shows what its elements came to: `x="p q";
	// a=("$x" r)` is `a=( 'p q' r )` here. The unquoted `a=($x)` is
	// `a=( 'p q' )` — one element, because nothing here splits an unquoted
	// parameter — which is this column's own expansion in its own trace.
	s.TraceArrayLiteralShowsTheExpandedElements = interp.Yes
	// One line for a subscripted literal as much as for a plain one:
	// measured 2026-09-17, `typeset -A m; set -x; m=([k]=v [j]=w)` is a
	// single `m=( … )` line in zsh 5.9.2, with the shell's own separator
	// byte where the brackets were.
	s.TraceSubscriptedArrayLiteralIsElementAssignments = interp.No
	// The subscript is not resolved, though: `i=2; a[$i]=v` is `a[$i]=v`
	// here where ksh93 writes `a[2]=v`. That pair is why the two are
	// separate axes.
	s.TraceElementSubscriptIsEvaluated = interp.No
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
	// And where the other six columns let a `!` alone once `set -n` has
	// stopped anything running, this shell inverts the status it exits with
	// anyway: `zsh -n` over a file holding `! true` writes nothing on either
	// stream and exits 1, on 5.9 and 5.9.2 alike, where bash, ksh93, dash and
	// BusyBox ash all exit 0. Measured 2026-09-16 (#3179).
	s.UnrunNegationInvertsTheStatus = interp.Yes
	// And where the other six columns read nothing of a command `set -n`
	// will not run, this shell reads its words far enough to raise the
	// refusals a word's own reading makes: `zsh -n -c 'echo =nosuchcmd'`
	// writes `nosuchcmd not found` and exits 1. Measured 2026-09-19, with
	// the controls that say it is not an expansion — a command substitution
	// in the same position creates no file. See interp/noexecwords.go
	// (#3823).
	s.UnrunSimpleCommandReadsItsWords = interp.Yes
	// And whether a *compound* writes the record at all is decided by what
	// its body holds rather than by what ran: `if [[ a = b ]]; then :; fi`
	// replaces the record and `if [[ a = b ]]; then [[ b = b ]]; fi` leaves
	// it, where neither body runs. Measured 2026-09-11 (#1931).
	s.CompoundPipelineStatusRecord = interp.CompoundPipelineStatusFromTheBody
	// A construct this shell has refused still asks for another line at a
	// prompt, where the other three refuse it before the next line is read.
	// Measured 2026-09-11 with `printf 'echo one\nif; then\necho three\n'`
	// into `-i`: this shell draws PS2 twice and swallows the `echo three`,
	// which is what makes it worth an answer rather than a bug (#1893).
	s.PromptAsksAgainAfterARefusedToken = true
	// And a `#` typed at this shell's prompt is an ordinary character until
	// `interactivecomments` is set, where the other three open a comment with
	// it — bash through a `shopt` of the same name that is on by default, and
	// dash and ksh93 with no option in the matter at all. Measured 2026-09-12,
	// `printf 'echo a #b\n'` into `-f -i` on a pipe: `a #b` here and `a`
	// everywhere else.
	//
	// The prompt alone: this shell's `-c`, its `eval` and its `.` all read
	// the `#` as a comment with the option off, which is why this is the
	// front end's question and not a preset for [syntax.Dialect.Comments].
	s.PromptCommentsNeedTheOption = "interactivecomments"
	s.UnsetEndsTheProducedPipelineStatus = interp.Yes
	// `unset RANDOM; RANDOM=9; a=$RANDOM; b=$RANDOM` is two fresh numbers
	// here — measured 2026-09-12, 20191 and 4730 — where bash, ksh93, dash
	// and BusyBox ash all answer 9 twice: the assignment is a message to the
	// producer whether or not `unset` has been past (#2450). The 9 is the
	// discriminator, being one digit against the generator's five.
	s.AssignmentRestoresAnUnsetProducedParameter = interp.Yes
	s.SelectLayout = interp.SelectMenuColumns
	s.SelectPromptNeedsTerminal = interp.No
	s.AliasParsesOptions = interp.Yes
	s.AliasHasPrintOption = interp.No
	// And no `-x` either: `alias: bad option: -x`, at 1.
	s.AliasHasExportOption = interp.No
	// The two kinds nothing else in the panel has: a global alias expands
	// wherever a word stands, and a suffix alias is a second namespace
	// keyed on a command word's extension (#2081).
	s.GlobalAliases = interp.Yes
	s.SuffixAliases = interp.Yes
	s.AliasListsAsDefinitions = interp.Yes
	s.AliasRestrictsToRegularKind = interp.Yes
	s.AliasOperandsCanBePatterns = interp.Yes
	s.AliasPlusPrintsNamesOnly = interp.Yes
	s.TypeNamesAnAliasOnlyWhenExpanded = interp.No
	// The reverse of ksh93: silent about `alias nope` and not about
	// `unalias nope`.
	s.AliasReportsNotFound = interp.No
	s.UnaliasReportsNotFound = interp.Yes
	s.AliasNotFoundStatusCounts = interp.No
	// A removed alias leaves nothing behind: `unalias h` twice is 0 then
	// 1 here.
	s.AliasRemembersTheNamesItNames = interp.No
	s.AliasOptionEndsTheLookup = interp.No
	// There is no `-p` here for an operand to stand behind: `alias -p zz` is
	// `alias: bad option: -p` at 1, with or without a `--` after it.
	// Measured 2026-09-19 (#3701).
	s.AliasPrintOptionIgnoresItsOperands = interp.No
	// The two builtins that keep an assignment written in front of them here
	// without being special ones — `V=1 alias` and `V=1 hash` leave `V` set,
	// where `V=1 :` and `V=1 shift 0` do not. See
	// interp.Semantics.BuiltinsKeepingAnAssignmentPrefix for the rows and for
	// why this is not the specialness axis (#3313).
	s.BuiltinsKeepingAnAssignmentPrefix = "alias hash"
	// And a command word that expanded to exactly `-` is thrown away here,
	// where the other six columns look it up and report 127. Not the same
	// question as LoneDashIsAnOption below, which this shell also answers
	// yes: that one is a dash-word a builtin was handed (#3236).
	s.LoneDashInCommandPositionIsDiscarded = interp.Yes
	s.UnaliasAllRefusesOperands = interp.Yes
	s.AliasQuoting = interp.ListingQuoteWhenNeededRuns
	s.AliasListingQuotesTheName = interp.Yes
	s.TrapQuoting = interp.ListingQuoteWhenNeededPlain
	// `typeset -p` writes `typeset v=1`, an exported scalar as `export e=E`
	// — values in the alias style, keys in the trap one.
	s.DeclareListing = interp.DeclareListingExportSpelled
	// A listing with no operands writes the produced parameters with their
	// readings. Measured 2026-09-13, zsh 5.9.2, `env -i PATH=/usr/bin:/bin`
	// with a scratch HOME, over a script file: `typeset -i10 RANDOM=11798`
	// and `typeset -i10 SECONDS=0`, with no reference to either first, and no
	// `LINENO` row — that one is interp.ProducedDeclaration.Silent and is
	// answered per parameter rather than here.
	//
	// It re-reads the producer, which zsh does too: two listings a line
	// apart hold `RANDOM=22537` and then `RANDOM=21677`, so a listing here
	// differs from itself exactly as ksh93's does. Measure that with a
	// redirection — `typeset -p | grep` forks the listing and two forks off
	// one state draw the same number, which reads as a freeze that is not
	// there (#2518).
	s.ProducedParameterListing = interp.ProducedListingWithValue
	s.DeclareValueQuoting = interp.ListingQuoteWhenNeededRuns
	s.ListingControlEscape = interp.ControlEscapeCaret
	// A byte above ASCII is written as itself — `v=é`, the key `[é]`, and
	// `$'a\téb'` where a control byte opened the form — and `^` and `=` are
	// not: `'^'`, `'a^b'`, `'a=b'`, `'x=y=z'`. Measured 2026-09-14 over
	// `set`, a keyed `typeset -p` and an alias listing. This engine left `^`
	// bare and quoted the non-ASCII byte, which is ksh93's answer to both
	// and this shell's to neither (#2820).
	// `!` is bare here and in ksh93 and quoted in bash, which is a third
	// pairing again: `v=!`, `v=a!b` and the key `[!]` all unquoted.
	s.ListedBangIsOrdinary = interp.Yes
	s.ListedCaretIsOrdinary = interp.No
	s.ListedEqualsIsOrdinary = interp.No
	// And a `~` is quoted wherever it stands, which is bash's rule refused:
	// `v='a~b'` and `v='b~'` from a bare `set`, measured 2026-09-17 (#2298).
	s.ListedTildeIsBareWhereItCannotExpand = interp.No
	// And the subscript takes the characters: `m[~/k]` is the three-character
	// key here, where bash and ksh93 expand it. Measured 2026-09-17 on
	// 5.9.2, with `typeset -p` beside the store (#2298).
	s.SubscriptKeyExpandsALeadingTilde = interp.No
	// And a tilde prefix carrying a quote or an expansion still expands here,
	// which is this shell alone: the quotes come off and the name is looked
	// up, so `~\chet/bar` is an error about a user called `chet` where the
	// other six columns print the word as written, and `~"/bar"`, `~$x` and
	// `~+"/x"` all reach the directory. Measured 2026-09-22 against the panel
	// on eleven shapes; see Semantics.TildePrefixStopsAtAQuoteOrAnExpansion
	// (#4156).
	s.TildePrefixStopsAtAQuoteOrAnExpansion = interp.No
	// And an unset `HOME` answers a `~` the way an empty one does — `~` is
	// nothing and `~/x` is `/x` — rather than leaving the word as written.
	// Measured 2026-09-23 on 5.9.2 with `unset HOME`, which really does
	// unset it here (`${HOME-UNSET}` is `UNSET`), and with `v=a:~:b` on the
	// same line for the colon road. The startup row is not this question:
	// this shell seeds `HOME` from the password entry before any line runs,
	// so a bare `env -i zsh` has a home to read where bash has none. See
	// interp.TildeWithNoHomePolicy (#4179).
	s.TildeWithNoHome = interp.TildeWithNoHomeIsEmpty
	s.ListedNonAsciiIsOrdinary = interp.Yes
	s.ListedAssignmentPrefixIsBare = interp.No
	// unanswered OperatorAfterTheSubscriptListingIsBad: `${!name[@]}` is a
	// bad substitution here in the *bare* form too, so there is no listing
	// for an operator to come after. Measured 2026-09-14 (#2821).
	// `export -p` is that same form narrowed to the exported names, not a
	// listing that repeats its own command word: it writes the attribute
	// letters, and it picks `typeset` where `export` will not carry the
	// value. Measured 2026-09-12, `env -i` and `-f`, from a table holding
	// `typeset -ix n5=5`, `typeset -ax A5=(1 2); export A5` and a frozen
	// export:
	//
	//	export -i n5=5
	//	typeset -ax A5=( 1 2 )
	//	export -r rr=4
	//	export -T PATH path=( /usr/bin /bin )
	//	export -i10 SHLVL=1
	//
	// The command-word form this had wrote `export A5` and `export n5=5` —
	// the value right in every case and every letter absent, with an array
	// reduced to its name (#1061). dash and ksh93 keep that form, measured:
	// their `export -p` writes no letters at all.
	s.ExportListing = interp.DeclareListingExportSpelled
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
	s.DeclarePrintReportsAMissingFunctionName = interp.No
	// `typeset -p s=5` performs nothing there: it is `no such variable: s`
	// at 1 with `s` still unset, and `typeset -p e=(1 2)` answers the same
	// way — measured 2026-09-20 on zsh 5.9.2. See
	// interp/declareprintoperand.go.
	s.DeclarePrintPerformsItsOperand = interp.DeclarePrintOperandIsANameAlone
	// And `export -p` and `readonly -p` narrow to their operands here, which
	// is the one column where those two words list at all once a name is
	// written beside the letter: measured 2026-09-20 on zsh 5.9.2, `export
	// a1=1; export a2=2; export -p a1` writes `export a1=1` alone and
	// `readonly t=6; readonly -p t` writes `typeset -r t=6`. Nothing is
	// declared — `export -p w=8` is `no such variable: w` at 1 with `w`
	// still unset. See interp/exportprintoperand.go.
	s.ExportOrReadonlyPrintWithOperands = interp.ExportPrintNarrowsToTheOperands
	s.TrapBodyLine = interp.TrapBodyLineWhereItFired
	// Where it fired for a DEBUG or ERR body too: this shell names the
	// firing line for every line of every body, so there is no second
	// answer to give.
	s.CommandTrapBodyLine = interp.TrapBodyLineWhereItFired
	s.TrapActionIsParsedWhenSet = interp.Yes
	// The strict end of the symbolic mask: one operator per clause, a who
	// before `=`, and neither `s` nor `t`.
	s.SymbolicMaskTakesMoreThanOneOperator = interp.No
	s.SymbolicMaskWhoAloneSetsIt = interp.No
	s.SymbolicMaskTakesTheSetuidLetter = interp.No
	// Neither: `bad symbolic mode permission: u`, and the same for X.
	s.SymbolicMaskTakesAPermissionCopy = interp.No
	// unanswered UmaskPermissionCopyBesideLetters: a copy is refused here
	// before anything looks at what is beside it — the line above is that
	// refusal — so a clause holding a copy and a letter cannot reach the
	// question. TestUmaskRefusesAPermissionCopy pins the refusal.
	s.SymbolicMaskTakesTheConditionalExecuteLetter = interp.No
	s.SymbolicMaskTakesTheStickyLetter = interp.No
	// A dash word is an option unless it is all digits, which is what
	// separates this from ksh93: `shift -x` is a bad option and `shift -1` is
	// a count it refuses for being below zero.
	s.ShiftOptionWords = interp.ShiftOptionWordsNonNumeric
	s.ShiftDoubleDashEndsOptions = interp.Yes
	// The marker is taken in front of a numeric operand too, so
	// `break -- 1` ends the loop and `exit -- 3` exits 3.
	s.NumericOperandDoubleDashEndsOptions = interp.Yes
	// Refused, and nothing is given up: `for i in 1 2; do break 1 2; done`
	// complains once per pass and runs to the end of the list.
	s.ExtraNumericOperand = interp.ExtraNumericOperandRefused
	s.ShiftNamesAreArrays = interp.Yes
	s.ShiftNegativeIsOutOfRange = interp.Yes
	s.WaitReadsOptions = interp.No
	// Job specs by command text, a second match taken rather than refused;
	// `wait` complains about a spec that names nothing, has no -n, and
	// `disown` takes the job out of the table.
	s.JobSpecsByName = interp.Yes
	s.AmbiguousJobNameIsRefused = interp.No
	s.WaitReportsAMissingJob = interp.Yes
	// And a job it has already reported stays waitable by its process id:
	// `wait %1; wait "$p"` answers the job's status in 5.9.2.
	s.WaitRemembersAReapedJob = interp.Yes
	s.WaitReportsTheSignalThatEndedTheJob = interp.No
	s.WaitNextJob = interp.WaitNextJobAbsent
	// Nor `-p`: the word is a job spec there too, and `wait -p` is `job
	// not found: -p` at 127. Measured 2026-09-13.
	s.WaitPNamesTheFinishedJob = interp.No
	// A trapped signal cuts a `wait` short with 128 plus the signal, and the
	// form that names a job answers the same as the bare one.
	s.WaitForAJobFailsWhenInterrupted = interp.No
	s.DisownRemovesTheJob = interp.Yes
	// And it reports what it did, as bash does (#3187).
	s.DisownAlwaysFails = interp.No
	s.CommandRejectsUnknownOption = interp.No
	// Whether `command -v` answers for every name it was given, and what
	// decides the status when it found some of them. See
	// interp.Semantics.CommandReportsEveryOperand for the split.
	s.CommandReportsEveryOperand = interp.Yes
	s.CommandCountsAMissingOperand = interp.Yes
	// And whether a `command` reached through an expansion keeps the power
	// to run what it names (#3369).
	s.ExpandedCommandOnlyReports = interp.No
	// What a `command -p` search resolved is **not** remembered here: with
	// an unusable PATH, a `command -p ls` and a plain `ls` after it, the
	// second is 127 in this column and 0 in bash. See
	// interp.Semantics.DefaultPathSearchIsRemembered (#2975).
	s.DefaultPathSearchIsRemembered = interp.No
	// `command` here means an *external* program of that name and nothing
	// else: `command set -o …` is `command not found: set` at 127, not the
	// survivable spelling of a special builtin it is in the other four. The
	// state is not fixed — `posixbuiltins` moves it, and `emulate sh` and
	// `emulate ksh` turn that option on — so setopt.go reads and writes this
	// axis and the preset only says where a plain zsh starts.
	s.CommandReachesABuiltin = interp.No
	// And where it does reach one — `setopt posixbuiltins`, the only route
	// that can be asked at all — it draws no boundary. Measured 2026-09-13
	// with the option on: `eval 'set -Z; echo IN'` ends the subshell at 1 and
	// `command eval 'set -Z; echo IN'` ends it identically, as do `readonly
	// 1bad=x`, `unset 1bad`, `export 1bad=x` and `. /nonexistent/file` in the
	// same text. So the axis is a constant here where CommandReachesABuiltin
	// is not.
	s.FatalErrorEndsAtTheCommandWord = interp.No
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
	// unanswered DeclarationMayShadowAnEnclosingScopesReadonly: that
	// narrower question is asked only where the one above said no, and this
	// shell takes every shadow — a caller's frozen local included, measured
	// 2026-09-21 — so there is no route by which it is put here.
	// And this is the one shell in the panel where `typeset +r` takes the
	// attribute back off: `typeset -r s=1; typeset +r s; s=9` leaves 9
	// there, silently and with status 0. `declare +r` is the same word. A
	// *special* parameter still refuses — `typeset +r EPOCHSECONDS` is
	// `can't change type of a special parameter` — which this engine has no
	// specials to reach.
	s.ReadonlyAttributeCanBeRemoved = interp.Yes
	// And a declaration's own array literal replaces a frozen *scalar*
	// outright, freeze and all: `readonly q=1; typeset -g q=(b)` leaves
	// `typeset -ar q=( b )` at status 0 here, where bash refuses it. The
	// exemption stops at the container — a name already holding an array or
	// a table refuses the same operand — so it is the retype and not the
	// write. See the axis for the rows (#2250).
	s.ArrayLiteralOperandRetypesAFrozenScalar = interp.Yes
	// unanswered DeclarationRereadsAParenthesizedValue: this shell refuses
	// the assignment before anything asks what the text means. Measured
	// 2026-09-21 under `env -i` on zsh 5.9.2, `typeset -a a="(1 2)"` is
	// `a: inconsistent type for assignment` and `typeset -A m="([k]=v)"` is
	// the same sentence for `m`, so neither the array nor the table route
	// reaches the question (#2298).
	// The letter half of the same rule: a numeric type letter that moves the
	// name to a type it does not already hold carries out its own
	// assignment over the freeze, and the freeze stays on. Not one field
	// with the literal half, because the two disagree over the same frozen
	// array — `typeset -ar q=(a); typeset -gi q=4` is taken and `readonly
	// q=(a); typeset -g q=(b)` is refused. See the axis for the rows
	// (#2539).
	s.NumericTypeLetterRetypesAFrozenName = interp.Yes
	// And nothing else about a frozen name's attributes is refused here
	// either — the letter that is not a numeric type is taken just the same.
	// Measured 2026-09-13 under `env -i` on zsh 5.9.2, `readonly q=1;
	// typeset -u q` lists `typeset -ur q=1` at 0. What is still refused is a
	// *value*, which is a different question and reaches refuseReadonly
	// (#2561).
	s.AttributeOverAFrozenNameIsRefused = interp.No
	// And with no value either: measured 2026-09-20, `readonly c; typeset
	// -i c` is taken at 0 here, exactly as the same line over a frozen name
	// holding a value is (#3937).
	s.TypeLetterOverAFrozenNameWithNoValueIsRefused = interp.No
	// Nor a keyed one over a frozen name holding a value: measured
	// 2026-09-20, `c=1; readonly c; typeset -A c` is taken at 0 here
	// (#3965).
	s.KeyedLetterOverAFrozenNameHoldingAValueIsRefused = interp.No
	s.DeclaredNameWithoutValueIsEmpty = interp.Yes
	// unanswered PrefixListingNamesADeclaredOnlyCompound: there is no
	// `${!prefix@}` in this shell at all — `typeset -A q1; echo "${!q@}"` is
	// `bad substitution` — so no listing of that shape ever reaches the axis
	// to be asked. Measured 2026-09-16 on zsh 5.9.2.
	// A declaration without a value leaves the name holding the empty string
	// here, so `typeset xyz; typeset -p xyz` writes `typeset xyz=''` and the
	// record below is not what that shape reaches (#2999). It is reached
	// from the other side: `unset` of a local the running call declared
	// leaves the name unset with the shadow still standing, which is the
	// same state, and this shell writes **nothing** for it. Measured
	// 2026-09-21, `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME,
	// `v=global; f() { local v; unset v; typeset -p v; }; f` — no output, at
	// 0, with `${v-UNSET}` firing its default. bash writes `declare -- v`
	// there. Answered rather than left unanswered because that route is a
	// spelling this shell has.
	s.ValuelessDeclarationRecordsTheName = interp.No
	// The name is there and the listing writes nothing for it — the third
	// state the field above cannot spell. See Semantics.ValuelessRecordIsStillAName
	// for the rows (#4053).
	s.ValuelessRecordIsStillAName = interp.Yes
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
	// A valueless declaration sets the name here, so an exported name of a
	// numeric type is holding `0` already and there is no valueless one for
	// this to be about. What the one-command form hands a child is nothing —
	// `typeset -ix Z; env` is silent — and a second declaration is what
	// gives the name its value and the child its entry. See
	// Runner.declarationOwnsTheStandingEmpty.
	s.NumericTypeWithNoValueReachesAChildAsZero = interp.No
	// The integer and float letters make a name a *scalar* of that type
	// here, so a declaration that also assigns an array literal is asking
	// for two kinds at once and is refused fatally — `typeset -ia z=(1 2)`
	// is `typeset: z: inconsistent type for assignment` at 1. The array
	// letter is not what triggers it and the case letters do not: `typeset
	// -i z=(1 2)` is the same refusal and `typeset -ua q=(ab cd)` is taken.
	s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = interp.Yes
	// A numeric type letter takes the container letter off the same
	// declaration: `typeset -ia z` is `typeset -i z=0` here and `typeset -iA
	// m` is `typeset -i m=0`, a scalar of that type either way. The valued
	// form of the same pairing is the refusal above rather than a collapse.
	s.NumericAttributeReplacesTheArrayAttribute = interp.Yes
	// And an array literal over a name that is not an array re-creates it,
	// dropping the letters that say what its values are — assigned and
	// appended alike, which is where this shell parts from ksh93: `typeset
	// -i p=3; p+=(5+5)` is `typeset -a p=( 3 5+5 )` here and `typeset -a -i
	// p=(3 10)` there.
	s.ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = interp.Yes
	s.AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = interp.Yes
	// A numeric letter takes a case attribute off — `typeset -l z; typeset
	// -i z` is `typeset -i z=1` — and the case letter does not take the
	// numeric one off: `typeset -i y; typeset -l y` keeps both, `typeset -il
	// y=1`. The one column that answers the two directions differently,
	// which is why they are two axes.
	s.NumericAttributeReplacesTheCaseAttribute = interp.Yes
	// But within one declaration the two letters part company: `-l` records
	// beside a numeric type letter and `-u` records nothing. Measured
	// 2026-09-12, `typeset -li v=4` lists `typeset -il v=4` where `typeset
	// -ui v=4` lists `typeset -i v=4` — upper case being the rendering a
	// based integer already has here (#2541).
	s.UpperCaseLetterBesideANumericTypeLetterRecordsNothing = interp.Yes
	// Both case letters on one declaration cancel, and take off a standing
	// one with them. Measured 2026-09-12, `typeset -lu z=Ab` lists `typeset
	// z=Ab` with the value unfolded, and `typeset -l z=Ab; typeset -lu z=Cd`
	// lists `typeset z=Cd` (#2541).
	s.TwoCaseLettersOnOneDeclarationCancel = interp.Yes
	s.CaseAttributeReplacesTheNumericAttribute = interp.No
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
	// And the other half of that asymmetry: an array with no elements *is*
	// a set parameter here, so `e=(); echo "[${e[@]+S}]"` writes `[S]` and
	// `${e[@]-D}` substitutes nothing. bash 5.3.20, bash 3.2.57 and ksh93u+
	// all read it as unset. Measured 2026-09-16 on zsh 5.9.2 under
	// `env -i PATH=/usr/bin:/bin LC_ALL=C`, from a file (#2298).
	s.EmptyArrayIsSet = interp.Yes
	// And `[[ -v a[@] ]]`, which is a different question: the subscript names
	// an **element** here, and no array has one called `@` or `*`. Measured
	// 2026-09-18 from a script file — `f=(x); [[ -v f[@] ]]` is false with an
	// element in the array, `typeset -A n; n[k]=v; [[ -v n[@] ]]` is false
	// with a key in the table, and `s=plain; [[ -v s[@] ]]` is *true*,
	// because a scalar has no elements to name and answers for itself
	// (#3436).
	s.ConditionWholeArraySubscript = interp.ConditionWholeArraySubscriptNamesAnElement
	// `set -u` reaches two subscripted shapes here that it reaches in no
	// other column. The length of an element that is not there is a refusal
	// — `a=(x y z); echo "${#a[9]}"` is `a[9]: parameter not set` where bash
	// 5.3.20, bash 3.2.57 and ksh93u+ all answer `0` at 0 — and a
	// whole-array subscript on a name holding nothing at all is one too:
	// `${nope[@]}` and `${nope[*]}` are refused here and in bash 3.2.57,
	// against `[]` at 0 in bash 5.3.20 and ksh93u+. `b=(); ${b[@]}` is the
	// control for the second and is `[]` at 0 here, so what it refuses is
	// the absent name rather than the empty array. Measured 2026-09-18 on
	// zsh 5.9.2 under `env -i HOME=… PATH=/usr/bin:/bin LC_ALL=C`, from a
	// script file with each row in a subshell (#2980).
	s.LengthOfAMissingElementIsRefused = interp.Yes
	s.UnsetNameWithAWholeArraySubscriptIsRefused = interp.Yes
	// The **count** of a whole array is a third shape of the same option and
	// it is refused here too, for a name holding nothing at all: `${#a[@]}`
	// on a name that was never mentioned is `a[@]: parameter not set` and
	// ends the shell, where ksh93u+ answers `0`. A name holding a string is
	// not a name holding nothing — `x=abc; ${#x[@]}` is `3` here — which is
	// what separates this from bash 5.3.20's reading of the same line, where
	// a scalar is refused alongside the absent name. Measured 2026-09-18 on
	// zsh 5.9.2 under `env -i HOME=… PATH=/usr/bin:/bin LC_ALL=C`, from a
	// script file with `echo REACHED` on the line after (#3125).
	s.WholeArrayCount = interp.WholeArrayCountRefusesANameHoldingNothing
	// And the colon form of the same operator counts the elements under
	// `[@]` rather than reading the join, so a one-element array holding the
	// empty string is a **value**: `f=(""); "${f[@]:-x}"` is `[]` and
	// `"${f[@]:+y}"` is `[y]`, where bash 5.3.20 answers `[x]` and `[]`.
	// `"${f[*]:-x}"` is `[x]` in both columns, which is what keeps the two
	// spellings apart here and is why this is not simply the element count.
	// Measured 2026-09-18 on zsh 5.9.2 under `env -i HOME=…
	// PATH=/usr/bin:/bin LC_ALL=C`, from a script file (#3425).
	s.WholeArrayColonTest = interp.WholeArrayColonTestCountsTheElementsUnderAt
	// And an attribute added to a name that already holds a value re-reads
	// it at once, as ksh93 does: `FOO=bar; typeset -i FOO` stores 0 and
	// `d=MiXeD; typeset -u d` stores MIXED. A separate question from the
	// one above, which this shell happens to answer the same way — bash
	// answers both no and ksh93 answers them differently from each other.
	s.AttributeRereadsTheValueItFinds = interp.Yes
	s.InheritedValueSurvivesADeclaredType = interp.Yes
	s.CompoundElementsGoThroughTheAttribute = interp.No
	// The case attributes fold on every *read* here and never touch the
	// store: `typeset -l lo=AB` lists back as `typeset -l lo=AB`, and
	// `typeset +l lo` reveals `AB`. See Semantics.CaseAttributeFoldsWhenRead
	// for the panel (#1755).
	s.CaseAttributeFoldsWhenRead = interp.Yes
	s.CompoundAttribute = interp.CompoundAttributeReplacesItWithAScalar
	// And the converse discards under both letters: `b=1; typeset -a b` is
	// `typeset -a b=( )` with `${#b[@]}` 0 and `$b` empty, and `a=1;
	// typeset -A a` is `typeset -A a=( )`. Measured 2026-09-08 against
	// 5.9.2 — the one column that throws the script's own value away, and
	// the one this implementation was giving every dialect.
	s.ScalarUnderAnArrayDeclaration = interp.ScalarUnderACompoundDiscardsIt
	s.ScalarUnderATableDeclaration = interp.ScalarUnderACompoundDiscardsIt
	// A name holding a compound reaches no child, with bash and against
	// ksh93 (#1380).
	s.ExportedCompoundReachesAChildAsItsFirstValue = interp.No
	// And a **subscripted operand** records no attribute at all here:
	// measured 2026-09-12, `typeset -x a[1]=v` and `export a[1]=v` both list
	// `typeset -a a=( v )` with no `x`, where the whole-name `typeset -x
	// a=(p q)` lists `typeset -ax`. So it is the subscripted operand that
	// carries nothing, and not the letter (#1380).
	s.SubscriptedOperandCarriesTheAttributes = interp.No
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
	//
	// This one is the option's, too: under `setopt ksharrays` the same line
	// writes the array's base and leaves the rest standing — `a=(first
	// second); a=word` is `typeset -a a=( word second )`, which is what bash
	// and ksh93 do with nothing set. It is the eighth axis setKshArrays
	// moves, and it could not move before the line below existed, since a
	// *table* under the option refuses the store rather than taking its key
	// `0` (#4618).
	s.ScalarAssignedOverACompoundReplacesTheName = interp.Yes
	// And with `ksharrays` off nothing refuses the store first, which is what
	// makes the line above reachable for a table at all: `typeset -A h=(one
	// 1); h=string` is `typeset h=string` at 0, and so is the `h+=string`
	// spelling. Under the option both are `h: attempt to set associative
	// array to scalar` and the shell leaves — see setKshArrays, which is
	// where the seventh axis moves — and
	// Semantics.ScalarStoredOverATableIsRefused for the grid (#4617).
	s.ScalarStoredOverATableIsRefused = interp.No
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
	// And the boundary between two elements is a field break, as in ksh93
	// and unlike dash and BusyBox ash. Measured 2026-09-26 on zsh 5.9.2
	// under `shwordsplit`, which is where this shell splits a list at all:
	// `IFS=:; set -- b ':'; w x$@y` is `[xb] [] [y]`.
	s.UnquotedListBoundaryIsIFSWhitespace = interp.No
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
	s.BracketEscape = interp.BracketEscapeProtectsAndIsAMember
	// And the join this shell *does* perform: an unquoted `@` list reaching
	// a context that keeps no fields is joined on the first character of
	// IFS, so `IFS=-; a=(x y z); v=${a[@]}` is `x-y-z` where bash and ksh93
	// give `x y z`. With IFS set and empty it is `xy`, which is what says
	// the separator is read from IFS rather than defaulted to a space.
	s.UnsplitAtListJoinsOnIFS = interp.Yes
	// zsh 5.9.2 prints `e1: a:b:c` for `${e1?$*}` under `IFS=:`. Worth
	// stating that this shell answered neither reading before the axis: with
	// zsh's own no-split rule over the fields path it made `a:b c`.
	s.DiagnosticWordIsFields = interp.No
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
	// The whitespace half of IFS is POSIX's three and not `isspace`: with
	// `IFS` the vertical tab alone and `setopt shwordsplit`, `a\v\vb` is
	// three fields here where bash 5.3 and ksh93 give two.
	s.IFSWhitespaceIsEverySpaceCharacter = interp.No
	// A bare `read` trims the record the way a named operand's value is
	// trimmed: a line with two spaces at each end reaches REPLY with none of
	// them, which is ksh93's answer and not bash's.
	s.BareReadTakesTheLineWhole = interp.No
	s.GlobExpansionResults = interp.No
	s.GlobNoMatchIsError = interp.Yes
	// No parameter of GLOBIGNORE's kind, and `.` and `..` are not in what a
	// pattern may match: `echo .*` is `.dot` alone and `echo .*/` matches
	// nothing at all. Measured 2026-09-14 (#2748).
	s.GlobListsDotAndDotDot = interp.No
	// unanswered IgnoredNamesValueIsOnePattern: there is no parameter of
	// GLOBIGNORE's kind in this shell — measured 2026-09-14, setting
	// `GLOBIGNORE` and `FIGNORE` alike changes nothing about what `echo *`
	// produces — so the three questions about *how* its value is read
	// cannot be put to it. IgnoredNamesMatchTheLastComponent and
	// IgnoredNamesFollowTheParameter are unanswered beside it for the same
	// reason, and IgnoredNamesVariable being empty is what keeps any of the
	// three from ever being reached (#2748).
	// unanswered IgnoredNamesMatchTheLastComponent: see above.
	// unanswered IgnoredNamesFollowTheParameter: see above.
	// The one column that assigns: `set --; printf "<%s>" ${1:=abc}` is
	// `abc` at status 0 here and `$1` is `abc` afterwards, where the other
	// five refuse it fatally. `@` and `*` are refused here too — `not an
	// identifier` — so it is the positional alone that parts (#1541).
	s.AssignThroughExpansionMayNameAPositional = interp.Yes
	s.AssignmentPrefixPersistsOnSpecialBuiltin = interp.No
	// And `command` in front of one takes the persistence away where the
	// option puts it there. Reachable only under `setopt posixbuiltins`,
	// which is where the row above turns yes, and measured there:
	// 2026-09-18, `setopt posixbuiltins; s=base; s=C command :` leaves
	// `base` while the bare `s=P :` leaves `P` (#3448).
	s.CommandKeepsASpecialBuiltinsPrefix = interp.No
	// And a prefix to a function is transient here too, and exported while
	// the call runs: `f(){ echo "[$v]"; }; v=1; v=9 f` prints `[9]` with
	// `v=9` in a child's environment, and leaves `1` behind. Measured
	// 2026-09-12 in 5.9.2. `emulate sh` moves it to persisting, which is
	// the off-panel reading that keeps the two questions apart (#2407).
	s.AssignmentPrefixPersistsAfterAFunction = interp.No
	s.PrefixToAFunctionIsExported = interp.Yes
	// The same answer as bash, measured the same day on 5.9.2: `s+=5 kf` in
	// front of a `function`-form function shows the body `base5` (#3161).
	s.PrefixToAKeywordFunctionIsScopedToTheCall = interp.No
	// And it writes the binding the name already has rather than a cell of its
	// own, which is where this shell parts from bash. Measured 2026-09-21 on
	// 5.9.2 with `ff() { typeset -p foo; }`: `typeset -i foo=7; foo=bar ff`
	// lists `export -i foo=0` and `typeset -u foo=abc; foo=bar ff` lists
	// `export -u foo=BAR` — the letters still on, and the word read through
	// them. The array rows agree with bash's for a reason that is not this
	// question: a plain scalar assignment replaces an array here anyway, so
	// `foo=(asdf fdsa); foo=bar ff` lists `export foo=bar` under the overlay
	// too (#4087).
	s.AssignmentPrefixMakesAFreshCell = interp.No
	// A prefix to a *builtin* is the other question and this shell answers it
	// the other way: the attribute is left exactly where it was. `export z=1;
	// z=2 typeset -p z` lists `export z=2` and `c=1; c=2 typeset -p c` lists a
	// plain `typeset c=2`, so nothing is gained and nothing taken off.
	// Measured 2026-09-16 in 5.9.2 (#3437).
	// The redirections are opened first, so a substitution in a prefix's value
	// never runs when one fails: measured 2026-09-18, `w=$(echo S >&2) f >
	// /nope/x` writes the file complaint alone (#3449).
	s.PrefixExpandedBeforeTheRedirections = interp.PrefixExpandedBeforeRedirectionsNever
	// And yet the prefix is worked through **before** a declaration utility's
	// operand, which is why the two are separate axes: this shell opens the
	// redirections ahead of the prefix and still reaches the prefix ahead of
	// the operand. Measured 2026-09-19, `PRE=$(echo PRE >&2) export s=$(echo
	// OP >&2)` writes `PRE` and then `OP` (#3814).
	s.PrefixExpandedBeforeADeclarationsOperand = interp.Yes
	s.PrefixExportAtABuiltin = interp.PrefixExportAtABuiltinUnchanged
	// And a listing with no operand sees the prefix's entry as an ordinary
	// one: its value is written and the attribute tables say the rest, so
	// `m=1; m=9 export -p` writes nothing and `m=9 typeset -p` writes
	// `typeset m=9`. Measured 2026-09-18 (#3446).
	s.PrefixInAWholeTableListing = interp.PrefixInAWholeTableListingIsAnOrdinaryEntry
	// And a declaration keeps nothing: `b=7; b=8 readonly b` reads `7` back
	// and unfrozen, `d=8 export d` reads `7`, `y=2 typeset -r y` reads `1`.
	// The temporary the prefix made is what the attribute went on, and it
	// leaves with the command (#3437).
	s.DeclarationPromotesThePrefixEntry = interp.No
	// A subscripted name in a prefix writes the element, and **this shell
	// never gives it back** — which is the opposite of what it does with the
	// scalar beside it, since a scalar prefix here outlives nothing at all,
	// not a function and not `:`. `arr=(x y z); arr[1]=P read -r j
	// </dev/null` leaves `P`, and so do a function, `:` and `eval`; only a
	// command a child runs leaves `x`, and that row is no dialect's answer.
	// Measured 2026-09-16 on zsh 5.9.2 (#3433).
	s.SubscriptedAssignmentPrefix = interp.SubscriptedPrefixStoresTheElement
	s.SubscriptedPrefixIsTakenBack = interp.No
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
	// such command" to it — and so is a pathname it was handed. Measured
	// 2026-09-21: `hash /bin/ls` is `no such command: /bin/ls` at 1 with the
	// table left empty, where three of the panel say nothing at all and
	// ksh93 remembers the path as written.
	s.HashSearchesPathAlone = interp.Yes
	// Which is the same answer said from the other side: the operand is not
	// passed over in silence here, it is searched for the only way this
	// shell searches and reported when the search cannot have it (#4064).
	s.HashIgnoresAnOperandWithASlash = interp.No
	// `hash -d` here is not bash's "forget one name": it is the table of
	// **named directories** that `~name` reads back, written as an
	// assignment. `hash -d a=/tmp; print -r -- ~a` is `/tmp`, `hash -d`
	// lists the table sorted by name and `hash -dL` writes the command that
	// would put an entry back. A named directory wins over a *user* of the
	// same name — `hash -d root=/tmp; print -r -- ~root` is `/tmp` (#2191).
	s.HashDefinesANamedDirectory = interp.Yes
	// And lists it in name order, alone in the panel — the other three print
	// their own tables' bucket order, which is a fact about their hashing
	// rather than about the language.
	s.HashListingIsSorted = interp.Yes
	// And `unsetopt hashcmds` really stops the table being filled, which is
	// the same reading bash gives `set +h` and not the one ksh93 gives
	// `trackall`. Measured 2026-09-13: `unsetopt hashcmds; ls >/dev/null;
	// hash` lists nothing here and `setopt hashcmds` starts it again, where
	// ksh93 with `set +o trackall` goes on hashing. The letter `-h` is a
	// different option in this shell and does not reach it.
	s.HashObeysCommandTracking = interp.Yes
	// A `PATH=… cmd` prefix leaves the table alone, the way ksh93's does
	// and bash 5.3's, dash's and ash's do not: the new PATH reaches the
	// child and the search and never this shell's own PATH, so there is no
	// assignment to empty the table. Measured 2026-09-13 with two copies of
	// one name on PATH, the first hashed — the prefixed run takes the
	// second copy and the table still holds the first.
	s.APrefixedPathEmptiesTheCommandHash = interp.No
	s.TildePlusMinusExpands = interp.Yes
	// A colon closes nothing in an ordinary word here — `echo ~:x` is the
	// characters as written, as in dash and BusyBox ash — which is the
	// standard's reading and is inherited rather than set; it is named here
	// because this is the column that departs from those two on the
	// neighboring question above.
	s.UnderscoreTracksTheLastArgument = interp.Yes
	// The parameter exists before anything has put a value in it: `${_+x}`
	// is non-empty and `set -u` reads it, even though what it holds is the
	// empty string until the first command runs.
	s.UnderscoreIsAParameterAtAll = interp.Yes
	s.UnderscoreMovesOnlyBetweenInputCommands = interp.No
	// Alone in the panel, this shell writes the call's own last argument
	// into `$_` before the body runs: `: outer` then `f one two` reads `two`
	// on the first line of the body, where bash and ksh93 read `outer`.
	s.UnderscoreMovesBeforeAFunctionBody = interp.Yes
	// And zsh alone lets the text write through: the same probe leaves
	// `inner`, and a sourced file leaves its own last command's argument.
	// Measured 2026-09-23 on zsh 5.9 over a script file.
	s.UnderscoreHoldsTheCallAcrossEvalAndSource = interp.No
	// zsh reads it the way `return` reads everywhere else — the action's
	// own last command. Same probe, entered at 4, answers 123.
	s.TrapReturnStatus = interp.TrapReturnTakesTheHandlersLastStatus
	// And starts it empty regardless, alone in the panel: an exported `_`
	// is discarded rather than carried in, so the parameter says nothing
	// about the invocation until the first command has run.
	s.UnderscoreInheritsFromTheEnvironment = interp.No
	// Bases stop at 36 here, and the refusal says so.
	s.ArithBaseAbove36 = interp.No
	// A base is read in plain decimal and however long it is written:
	// `010#9` is 9 and `0010#5` is 5, so the leading zeros are padding
	// rather than the start of a constant — which they cannot be here in
	// any case, ArithLeadingZeroIsOctal being No.
	s.ArithBaseMayHaveALeadingZero = interp.Yes
	s.ArithBaseIsAtMostTwoDigits = interp.No
	// And zero is a base this shell reads through rather than refuses: the
	// digits after it are an ordinary constant, so `$(( 0#0x10 ))` is 16.
	s.ArithBaseZeroReadsTheDigitsAsWritten = interp.Yes
	// `$(( 0x ))` is 0 and `$(( 0x+1 ))` is 1, as in bash.
	s.ArithEmptyRadixDigitsAreZero = interp.Yes
	// The one column that reads part of an over-large numeral and says so,
	// on stderr, with the script carrying on:
	// `$(( 10000000000000000000 ))` writes `number truncated after 19
	// digits: 10000000000000000000` and answers 1000000000000000000 (#3202).
	s.ArithNumeralPastTheWord = interp.NumeralPastTheWordKeepsTheDigitsThatFit
	// And the same numeral out of a variable reads identically; only dash
	// parts the two.
	s.ArithStoredNumeralPastTheWordIsRefused = interp.No
	// ${#a} of an array counts elements, and a function's $LINENO counts
	// from the function.
	s.ArrayLengthWithoutSubscriptIsCount = interp.Yes
	// `${#s[@]}` on a scalar is the width of the value, so an empty one is
	// 0 and `a b` is 3 — where every other shell in the panel reads the name
	// as a list of one and answers 1 for both. Not moved by `ksharrays`,
	// measured: the three rows are 0, 1 and 3 under the option as well.
	s.WholeSubscriptOnAScalarMeasuresIt = interp.Yes
	s.FcEmptyHistoryIsAnError = interp.Yes
	// And an editor that emptied the file is an error here too, where bash
	// runs nothing and says nothing. Measured 2026-09-21 with a stand-in
	// editor that truncates what it is handed (#4017).
	s.FcEmptyEditIsAnError = interp.Yes
	// And an event this list cannot reach is refused rather than brought to
	// the nearest end, which is the other shell's answer: `fc -l 99` on five
	// entries is `fc: no such event: 99` at 1 where bash writes all five at
	// 0. Measured 2026-09-21 against zsh 5.9.2, `zsh -f` on a script file
	// under `env -i` with a scratch `HOME`, the list planted with `print -s`
	// (#4018).
	s.FcEventOutOfRangeIsAnError = interp.Yes
	// A relative operand is counted back from this shell's own event number,
	// and a shell reading a script has none — so `fc -l -1`, `fc -l -2` and
	// `fc -l -20` all write the whole list, on five entries and on thirty.
	// The same fact gives the default range: the newest seventeen entries of
	// the list rather than the sixteen events below a current one.
	s.FcRelativeEventNeedsTheShellsOwnEventNumber = interp.Yes
	// And the newest entry is the line this shell is standing on, so the
	// roads that *run* one stop below it and say why: `fc -s 5` on five
	// entries is refused, `fc -s 99` and `fc 99` are refused in the same
	// words rather than named as events, and `fc -s` on a list of one — or
	// of none — is refused too, where on two or more it runs the oldest
	// entry and says nothing. A range's far end is brought under the
	// threshold instead: `fc -e ed 1 5` edits 1 through 4. Measured
	// 2026-09-21 on the same lists as the axis above (#4058).
	s.FcNewestEntryIsTheCurrentLine = interp.Yes
	// And an operand's digits are read past whitespace and a `+`, where
	// bash wants the sign or the first digit at the front of the word:
	// `fc -l ' -1'` and `fc -l ' 2'` and `fc -l +3` are all events here and
	// all `fc: no command found` there. The digit *prefix* itself is not
	// this axis — both shells read `fc -l 2x` as entry 2, and the core does
	// that unasked.
	// And a range whose entries would run newest first is refused on the
	// editor road rather than run that way: `fc -e ed 3 1` and
	// `fc -r -e ed 1 3` are both refused where `fc -r -e ed 3 1` edits 1, 2,
	// 3 at 0, so it is the order they would run in that is judged. `fc -l
	// 3 1` still lists backwards at 0, so it is the running and not the
	// range (#4100).
	s.FcBackwardsRangeIsAnError = interp.Yes
	s.FcNumericOperandSkipsBlanksAndASign = interp.Yes
	// And the same looseness decides where the options end: `fc -l -1x` is
	// the operand `-1x` here and `fc: -1: invalid option` at 2 in bash,
	// which reads a word as an operand only when it is a dash and nothing
	// but digits.
	s.FcOptionsEndAtADashAndADigit = interp.Yes
	s.JobControlAbsenceIsReportedFirst = interp.Yes
	// And the monitor alone is what `fg` and `bg` need. Reachable only with
	// a terminal here, since `set -m` without one is fatal in this shell —
	// measured 2026-09-15 on a pseudo-terminal, where the job line is
	// printed and the status is 0 (#2720).
	// A chain counts through what the link before it named — characters of
	// one string, elements of a list — which is the reading
	// syntax.Dialect.ChainedSubscript was written for and is not ksh93's
	// walk into a nested compound. This shell has no nested compound to
	// walk into (#2830).
	s.ChainedSubscriptReadsANestedValue = interp.No
	s.MonitorAloneResumesAJob = interp.Yes
	// And the one column that *announces* on the monitor alone: a script
	// with `set -m` writes `[1] <pid>` for a `&` job with nobody at a
	// prompt, where the other four write nothing. Measured 2026-09-15 on a
	// pseudo-terminal, which is the only place this shell has a monitor at
	// all (#2838).
	s.MonitorAloneAnnouncesAJob = interp.Yes
	// And it is the one shell that accounts for its jobs on the way out with
	// no prompt anywhere. Measured 2026-09-25 against zsh 5.9.2 on a
	// pseudo-terminal, `zsh -fm` over a script holding `sleep 3 &`: the
	// error stream gets `you have running jobs.` and then
	// `warning: 1 jobs SIGHUPed`, both before the EXIT trap, and `zsh -f`
	// over the same script writes neither. The monitor and not the prompt —
	// an interactive session with `unsetopt monitor` writes neither too
	// (#4542).
	s.MonitorAloneAccountsForJobsAtExit = interp.Yes
	// A stopped job holds the exit back: the shell says so and stays,
	// and the next attempt leaves. Measured through a pseudo-terminal for
	// `exit` and for ^D alike.
	s.StoppedJobsHoldTheExit = interp.Yes
	// And says only the sentence: measured, zsh writes `you have running
	// jobs.` and draws the next prompt, with no table under it, whichever way
	// `checkjobs` and `checkrunningjobs` are set.
	s.HeldExitListsTheJobs = interp.No
	// `setopt hup` is on in a fresh zsh and needs no login shell: measured
	// 2026-09-25 on a pseudo-terminal with the shell started `-fiV +Z`, an
	// interactive session that is not a login shell hangs its running job up
	// and writes `zsh: warning: 1 jobs SIGHUPed`. A stopped job is left out
	// of the send — one running beside one stopped is `1 jobs SIGHUPed`, two
	// stopped is silence, and the stopped process reads `SN` afterwards
	// rather than `T`, so this shell continued it rather than hanging it up.
	// And all of it happens before the EXIT trap: with `trap 'echo
	// EXIT_TRAP' EXIT` the warning comes first. Each row is on its axis.
	s.HangupAtExitNeedsALoginShell = interp.No
	s.HangupAtExitSkipsStoppedJobs = interp.Yes
	s.HangupAtExitPrecedesTheExitTrap = interp.Yes
	// CDPATH moves in silence here.
	s.CdpathAnnouncesTheDirectory = interp.No
	// And CDPATH is a search beside the ordinary relative lookup, with the
	// fallback POSIX gives it (#2896).
	s.CdpathReplacesTheRelativeLookup = interp.No
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
	// A subscript's expanded text is handed back to the bracket scanner
	// here, so a key holding `]` or `[` is read as syntax rather than as
	// the string a key is: with `key='x],b['` already stored, `(( m[$key]++
	// ))` is `not an identifier: b[]` at status 2 and the element is left
	// where it was, where bash and ksh93 both increment it. The `b[` in
	// that sentence is the tail of the key being named as a second array.
	// Measured 2026-09-13 against zsh 5.9.2 (#2581).
	s.ArithSubscriptRereadsItsExpandedText = interp.Yes
	// And a subscript whose quotation never closes is still the key it
	// looks like: measured 2026-09-20 on 5.9.2, `typeset -A a; k="q'r";
	// a[$k]=4; let "++a[$k]"` leaves 5 under the three-character key, with
	// ksh93u+ and against bash 5.3.20, which calls the subscript bad.
	//
	// Unreachable from this preset and answered anyway: no quoting is read
	// inside a subscript here — syntax.Dialect.ArithSubscriptQuoting is off
	// — so no scan has a quotation to give up on. Answered for the reason
	// ArithWholeArraySubscriptIsReportedAsBad above is: an unanswered axis
	// is a refusal, and a reading this dialect cannot reach must not be
	// able to produce one (#3796).
	s.ArithSubscriptQuotationMustClose = interp.No
	// A `let` operand's subscript is not a quoting context here either, and
	// that is measured rather than inherited from the line above: on zsh
	// 5.9.2 with `typeset -A a; a[k]=1; a['"k"']=2`, `let '++a["k"]'`
	// leaves the bare `k` at 1 and the quoted key at 2, so it stored under
	// a third key spelled with the quotes — which is what
	// SubscriptIsAQuotingContext already answers for every other subscript
	// of this preset.
	//
	// It is a pin, exactly as the axis above is: with no subscript here
	// ever a quoting context, the builtin route cannot part from the
	// expression's, and the answer exists so that route cannot reach an
	// unanswered axis (#3871).
	s.ArrivedSubscriptIsAQuotingContext = interp.No
	// And an apostrophe written inside a subscript stops nothing here
	// either: measured 2026-09-20 on 5.9.2 from a script file,
	// `typeset -A m; kq=q; (( m['$kq'] = 42 ))` stores under `'q'` — the
	// expansion performed and the apostrophes kept, which is
	// SubscriptIsAQuotingContext's `no` showing through beside this one
	// (#3942).
	s.WrittenSubscriptQuotationStopsItsExpansion = interp.No
	// And the expansion being performed does not end the key: measured in
	// the same run, `(( m[q'$kq'z] = 42 ))` is `q'q'z` here — every
	// character kept, quotation included — where ksh93u+ 2012-08-01 keeps
	// only `q`. A reading of its own rather than a pin: this column
	// performs the expansion, so it reaches the question (#3968).
	s.SubscriptQuotationEndsTheKey = interp.No
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
	// And a code point the locale cannot hold is refused rather than written
	// back: this shell reports `character not in range`, writes what came
	// before the escape, and abandons the script with the status it already
	// had. Measured 2026-09-11 under `LC_ALL=C` (#1851).
	s.UnicodeEscapeOutsideTheLocale = interp.OutsideLocaleEscapeRefused
	// And the radix character, where this shell is the panel's third answer:
	// it writes the locale's radix and reads a number at **either** that or
	// the point. Measured on macOS 15 with the host's own `de_DE.UTF-8`,
	// `printf '%.4f' 1` is `1,0000` and `printf '%.2f' 1.5` is `1,50` where
	// bash and ksh93 call the operand invalid. See
	// interp.Semantics.NumberRadix.
	s.NumberRadix = interp.RadixIsTheLocalesOwnOrThePoint
	// A value above what six bytes hold is still encoded here, the arithmetic
	// overflowing into the lead byte: measured 2026-09-22 under
	// `LC_ALL=en_US.UTF-8`, `printf '%s' $'a\UFFFFFFFFb'` is
	// `61 ff bf bf bf bf bf 62` in 5.9.2, where bash and ksh93 write `a b`.
	// The lead byte is one no decoder will take back, which is the shape of
	// the disagreement rather than an accident of it.
	s.CodePointPastSixBytesIsEncoded = interp.Yes
	// And an *unset* locale is the C locale here, on every operator that
	// reads one: under `env -i`, 5.9.2 answers 6 for `s=héllo; echo ${#s}`,
	// leaves `${(U)s}` on `café` as `CAFé`, and refuses the escape above the
	// way it refuses it under `LC_ALL=C`. bash 5.3.15 answers 5, `CAFÉ` and
	// the encoded character to the same three (#2020).
	s.UnsetLocaleIsUnicodeAware = interp.No
	// And the decoder does not reach the reader that takes the program text:
	// this shell counts a multibyte character everywhere it measures a string
	// and still walks its input a byte at a time. Measured 2026-09-23 under
	// `LC_ALL=zh_TW.Big5` with the Big5 spelling of U+03B1, whose second byte
	// is a backslash — `x=α` is `command not found: x=`, the trail byte having
	// joined the next line, `x="α"` is `unmatched "`, and a here-document body
	// holding `α$v` writes `$v` as text. bash 5.3.20 and ksh93u+ take the
	// character in all three.
	//
	// `read` is the other way round here and is a separate axis: see
	// interp.Semantics.ReadTakesAMultibyteCharacterWhole, which this shell
	// answers Yes by inheriting the preset.
	s.MultibyteCharacterIsReadWhole = interp.No
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
	//
	// `-q` reads one key from the same terminal `-k` reads and answers
	// whether it was a yes — 0 for `y` or `Y`, 1 for anything else, with the
	// name left holding the normalized `y` or `n` rather than the key. It is
	// zsh's letter alone: measured 2026-09-26, bash 5.3.20 and 3.2.57 call it
	// `invalid option`, ksh93u+ `unknown option` and dash `Illegal option`,
	// all at 2, so nothing here needs an axis. interp/readkeys.go carries the
	// rest of what was measured.
	s.ReadOptions = "rsnpAd:t:u:k#q"
	// `unset -m` reads its operands as patterns, which is this shell's
	// alone; `-n` is not here, and that is measured rather than an
	// omission — `unset -n x` is `bad option: -n` in zsh 5.9.2 where bash
	// 5.3 and ksh93 take it.
	// unanswered NamerefCycleIsRefused: this shell has no name references to
	// make a cycle of, which is measured rather than assumed — #2553's report
	// named `typeset -n` among its three spellings and it is not one.
	// Measured 2026-09-15 on zsh 5.9.2, `env -i` with a scratch HOME, inside
	// a function so all three words are reachable: `typeset -n v=1`,
	// `declare -n v=1` and `local -n v=1` are each `bad option: -n`.
	// unanswered NamerefLetterStandsAlone: there is no `n` letter to write
	// beside another one. Measured 2026-09-18 on zsh 5.9.2, `typeset -ni
	// r=v` is `typeset: bad option: -n` at 1 — the complaint names the
	// missing letter and never reaches a question about the pair (#3171).
	// unanswered DeclarationThroughAReferenceNamesTheOperand: there is no
	// reference for a declaration to be redirected through (#3173).
	// unanswered ExportOrReadonlyTakesAReferenceToAnElement: the same missing
	// letter, and the `(P)` flag this shell does have is an expansion rather
	// than a name, so neither builtin can be handed a reference at all.
	// Measured 2026-09-20 on zsh 5.9.2, `a=(p q r); typeset -n b='a[1]'` is
	// `typeset: bad option: -n` at 1 (#3881).
	// unanswered NamerefTargetResolvedWhenAimed: the same missing letter —
	// there is no reference for a target to be settled on. Measured
	// 2026-09-18, `typeset -n r=a[1]` is `typeset: bad option: -n` at 1
	// (#3124).
	// unanswered ArrayLetterOverAnUnaimedReferenceDropsIt: the `n` letter is
	// the missing one again, so there is no unaimed reference for an array
	// letter to land on. Measured 2026-09-22, `typeset -n foo` is `typeset:
	// bad option: -n` at 1 and `typeset -a foo` after it is an ordinary
	// array (#4178).
	// unanswered NamerefArrayRefusal: the letter is the same missing one, so
	// there is no declaration for an array to be refused under. Measured
	// 2026-09-16, `r=(a b); typeset -n r=v` is `typeset: bad option: -n` at 1
	// and the array is untouched (#3103).
	// unanswered NamerefDroppedByASubscriptedOperand: the same missing
	// letter, so there is never a reference for a subscripted operand to
	// take an attribute off. Measured 2026-09-23, `typeset -n xref` is
	// `typeset: bad option: -n` at 1 and `typeset -a xref[1]=one` after it
	// writes an ordinary array (#4178).
	// unanswered UnsetReferenceLetterRemovesANonReference: `-n` is not one of
	// this shell's letters. Measured 2026-09-12, `unset -n x` is
	// `unset: bad option: -n` at 1 and `x` keeps its value (#932).
	s.UnsetOptions = "vfm"
	// `readonly` here is `typeset -r` under another name, so it takes far
	// more than these six: measured 2026-09-12, `-i`, `-x`, `-g`, `-l` and
	// `-u` are all taken at status 0 as well, and only `-n` and `-r` are
	// refused. The set stops at the letters this builtin does something
	// with, because a letter accepted and then ignored hands a script a
	// success it did not earn — see Semantics.ReadonlyOptions. `-t` joined
	// it in #3101, when the trace attribute became something this builtin
	// records: measured 2026-09-18, `readonly -t R=1` lists as `typeset -rt
	// R=1` here. Making `readonly` read DeclareOptions is the honest fix for
	// the rest and has its own measurements to make.
	//
	// unanswered ReadonlyReferenceLetter: `readonly -n` is refused here, so
	// the letter never reaches the axis. Measured 2026-09-18 under
	// `env -i PATH=/usr/bin:/bin LC_ALL=C`: `readonly -n zz` is
	// `readonly: bad option: -n` at 1. ReadonlyOptions has no `n`, which is
	// what keeps the question off this column rather than answered wrongly.
	s.ReadonlyOptions = "paAft"
	// And it is `typeset -r` in the other half too: a `readonly` written
	// inside a function declares a **local**, where every other shell in
	// the panel freezes the name the shell already has. Measured
	// 2026-09-15 over a script file, `b() { readonly B=1; }; b` leaves
	// `B` unset here and at 1 in bash, ksh93, dash and BusyBox ash —
	// including ksh93's keyword form, which is what keeps this apart from
	// TypesetLocalNeedsKeywordFunction. `export` is not the same word and
	// is not local here: it is `typeset -gx`, and the `g` is the whole
	// difference.
	s.ReadonlyDeclaresALocal = interp.Yes
	s.ReadZeroTimeout = interp.ReadZeroTimeoutFinishesWhatItStarted
	// `read -A` with no name fills the array `reply`, which is the array
	// neighbor of the `REPLY` a bare `read` fills and deliberately not the
	// same parameter: the shell is case-sensitive about which. Measured
	// 2026-09-26 under `-f`, with the other one pre-set on each row so that
	// neither reads back what it had a moment earlier — `reply=(x y); read
	// -A <<<'hello world'` leaves `reply=(hello world)` and never creates
	// `REPLY`, and `reply=scalar; read <<<'plain'` leaves `reply` alone and
	// puts the line in `REPLY`.
	s.ReadArrayDefault = interp.ReadArrayDefaultIsTheArrayReply
	s.ReadTimeoutKeepsWhatArrived = interp.No
	s.ReadTimeoutBoundsReadability = interp.Yes
	s.ArithLeadingZeroIsOctal = interp.No
	// Nothing makes a leading zero octal here, so `let` and `(( ))` read one
	// alike and the axis that parts them elsewhere has nothing to change.
	s.LetReadsALeadingZeroAsDecimal = interp.No
	// And the same for the assignment reader, which is reachable here only
	// under `setopt octal_zeroes` — the one switch in the panel that moves
	// the axis above, so this shell is the only column where the question is
	// asked of a *running* shell rather than of a preset. Measured 2026-09-17
	// on zsh 5.9.2 with the option set: `typeset -i d=010` and
	// `e=010; typeset -i e` both leave `8#10`, which is eight written in the
	// base it was read in, so the assignment goes through the expression
	// reader's octal rule rather than around it. ksh93 is the column that
	// answers otherwise, and this one does not join it (#2884).
	s.IntegerAssignmentReadsALeadingZeroAsDecimal = interp.No
	// A name an arithmetic assignment *creates* is an integer here, which
	// outlives the expression: `(( x = 5 )); x=2+3` is 5 where the same two
	// commands leave the three characters `2+3` everywhere else. Only a name
	// it creates — `x=3; (( x = 5 ))` leaves an ordinary scalar.
	s.ArithmeticAssignmentDeclaresANumber = interp.Yes
	s.FatalErrorStatusIsOne = interp.Yes
	// Except for one refusal, which leaves 0 behind when the program came
	// from an argument rather than from a file. Measured on every
	// neighboring refusal too, and they all leave 1 — see the axis.
	s.SetArrayBadNameLeavesZero = interp.Yes
	s.StoreRefusalOfADeclaredElementLeavesZero = interp.Yes
	// And a refused store through `printf -v`'s operand, which parts from
	// the identical refusal through `read`'s here: measured 2026-09-17,
	// `a=(1 2 3); ( printf -v "a[1/0]" X ); echo $?` is 0 and
	// `( read "a[1/0]" < in.txt )` is 1, by both routes, and the empty
	// subscript answers the same way at each.
	s.StoreRefusalThroughPrintfLeavesZero = interp.Yes
	s.HeredocExpandsInTheCommandsProcess = interp.Yes
	s.RedirectTargetExpandsInTheCommandsProcess = interp.Yes
	// And a subshell's, which is where this shell had it wrong: `( : ) >
	// "${u:=made}"` leaves `u` unset here, and `( echo RAN ) > $(( 1/0 ))`
	// writes one line, is caught by `||`, leaves 1 behind and runs the rest
	// of its own line, where a group with the same target ends the shell
	// (#4695).
	s.RedirectTargetOnASubshellExpandsInTheSubshell = interp.Yes
	// A *target* this shell expanded itself and could not is this shell's
	// own failed expansion rather than a failed redirection, which here is
	// fatal whatever it was written on — where `: < /nonexistent/f` and
	// `read x < /nonexistent/f` both carry on at 1. Measured 2026-09-26 on
	// zsh 5.9.2 under `-f` (#4689).
	s.RedirectTargetFailureIsTheRedirections = interp.No
	// And a body this shell expanded itself and could not is its own failed
	// expansion, which here is fatal: `: <<END`, `read x <<END`, a function
	// and a group all end the shell at 1 over a body of `$(( 1/0 ))`, where
	// a file that will not open on any of them carries on at 1 and is caught
	// by `||`. Measured 2026-09-26 on 5.9.2 under `-f` (#4684).
	s.HeredocBodyFailureIsTheRedirections = interp.No
	s.ArithNameValueRecurses = interp.Yes
	// And a fixed depth is what stops it: a chain of sixty distinct names
	// ending in a number is `math recursion limit exceeded`. Measured
	// 2026-09-18 (#3416).
	s.ArithRecursionBound = interp.ArithRecursionBoundedByDepth
	// `typeset +A` takes the attribute off and leaves the name an empty
	// scalar, at status 0 and with nothing said. See
	// interp.Semantics.ArrayAttributeRemoval (#4241).
	s.ArrayAttributeRemoval = interp.ArrayAttributeRemovalEmptiesTheName
	// A table literal mixing `[key]=` heads with bare words is refused whichever
	// way round it was written, and the script is given up. An empty key is a
	// key like any other here. See interp.Semantics.MixedTableLiteral and
	// .EmptyKeyInATableLiteral (#4241).
	s.MixedTableLiteral = interp.MixedTableLiteralRefused
	s.EmptyKeyInATableLiteral = interp.EmptyKeyInATableLiteralAccepted
	// And an unset name found that way is a zero like any other unset name:
	// `x=abc; $((x+1))` is 1 and the script runs on. Measured 2026-09-11 —
	// ksh93 is the panel's holdout, where it is a fatal `parameter not set`.
	s.ArithRecursedNameMustBeSet = interp.No
	s.ArithSubscriptNameMustBeSet = interp.No
	// With nounset on, though, an unset name an expression reads is refused
	// as it is in bash and ksh93: `set -u; : $((b))` is `b: parameter not
	// set`. Not fatal of itself — the construct answers, so `(( b ))` leaves
	// 2 and the line runs on where `$(( b ))` in a word stops the shell
	// (#3574).
	s.ArithUnsetNameUnderNounsetIsRefused = interp.Yes
	s.ArithNounsetRefusalIsFatal = interp.No
	s.BraceExpansion = interp.Yes
	// The fan copies the names and not the work: `i=0; echo {x,y,w}$((i++))`
	// is `x0 y0 w0` with `i` left at 1 on zsh 5.9.2, and `echo {x,y}$(echo
	// TICK >&2; echo z)` writes TICK once for the two names. The words are
	// the same either way, so the count and the variable are the whole of
	// the tell — see the panel on the axis (#4694).
	s.BraceFanExpandsEachNameOnItsOwn = interp.No
	// What the braces produced goes back into the word the parse cut rather
	// than being read again as text, so `var=baz; varx=vx; echo $var{x,y}`
	// is `bazx bazy` and `printf '[%s]' {Z..a}` keeps the backslash it
	// counted — bash answers `vx vy` and an empty element.
	s.BraceOutputRereadAsText = interp.No
	// Agrees with bash on where the scan resumes after a group that did not
	// expand: one byte past its open brace, so `{a{b,c}}` is `{ab} {ac}`
	// and `@{x}{a,b}@` is two words.
	s.BraceRescanEntersFailedGroup = interp.Yes
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
	// A range between two single characters spans whatever those characters
	// are — `{1..x}` is seventy-two words and `{α..γ}` is three — and takes
	// no step, so `{a..z..2}` is the word as written where bash counts
	// `a c e …`. And a body shaped like a numeric range that holds no range
	// loses its *braces* rather than standing whole: `{1..}` is `1..`,
	// `{..3}` is `..3`, and a written step of zero goes the same way.
	s.BraceCharRangeSpansAnyCharacter = interp.Yes
	s.BraceRangeMissingEndCountsFromZero = interp.No
	s.BraceRangeZeroStepCountsAsOne = interp.No
	s.BraceRangeNumberMayCarryAPlus = interp.No
	s.BraceRangeThatCannotBeCounted = interp.BraceRangeFailureDropsTheBraces
	s.BracketCaretNegates = interp.Yes
	s.EqualsExpansion = interp.Yes
	s.LastPipelineElementInCurrentShell = interp.Yes
	// A process substitution may *not* stand as a condition's operand. The
	// word is read and then refused, in a sentence of this shell's own and
	// at status 2 — and the command is not started, which is the half a
	// refusal that came after the expansion would get wrong. Measured
	// 2026-09-05: `[[ x == <(x) ]]` is `process substitution <(x) cannot be
	// used here` here and runs the command in bash.
	// An empty right operand is refused, as in bash, though with this
	// shell's own wording and status: `failed to compile regex: empty
	// (sub)expression`, status 1. It is the engine underneath showing
	// through — POSIX ERE has no empty expression — and it is visible in
	// the shipped `regexp-replace`, where an accepted empty pattern would
	// write the replacement between every pair of characters (#2043).
	s.EmptyRegexOperandIsAnError = interp.Yes
	// unanswered RegexMatchSurvivesAFailedMatch,
	// RegexMatchOmitsGroupsThatDidNotMatch,
	// PatternMatchWritesTheMatchRecord: this shell keeps its captures
	// through the reporting parameters a pattern flag fills rather than
	// through a named array, so the record those three axes are about is never
	// written here and none of the three questions is put.
	// TestARegexMatchReportsWhatItMatched pins the shape it does
	// have.
	s.ProcessSubstitutionInCondition = interp.No
	// A substitution's body reads what the *shell* is reading, and this shell
	// is alone in it. `printf "PIPE\n" | cat <(cat)` with the shell's own
	// input a file holding OUTER prints OUTER here and PIPE in bash and
	// ksh93 — the body takes the input the element's pipe replaced, because
	// that pipe is a redirection of the element and the words were expanded
	// before it was applied. `=(cat)` in the same place answers the same,
	// which is the whole reason #1933 was filed before the file form landed.
	s.ProcessSubstitutionBodyReadsTheShellsInput = interp.Yes
	// A condition operand that is not an expression abandons the input —
	// `echo one; [[ 1+ -eq 0 ]]; echo two` writes `one`, complains, and
	// never reaches `two`. Note it is `[[ ]]` alone: the same expression
	// in `(( ))` complains and the shell goes on.
	s.ConditionArithmeticErrorIsFatal = interp.Yes
	// And a C-style `for` header. Not `(( ))`, which zsh reports and carries
	// on from — the two are a field apart for exactly that reason. Measured
	// 2026-09-13; see [interp.Semantics.ForHeaderArithmeticErrorIsFatal].
	s.ForHeaderArithmeticErrorIsFatal = interp.Yes
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
	// unanswered ExpansionResultSuppliesGroupSyntax: read only where
	// GlobExpansionResults says yes, and zsh answers that one no — it does
	// not match the result of an expansion against the filesystem at all, so
	// there is no pattern here whose group syntax could count.
	// unanswered TableLetterReachesItsOwnOperandsSubscript: zsh refuses the
	// shape the axis is about rather than answering it — measured 2026-09-12,
	// `typeset -A m[k]=v` is `m[k]: inconsistent type for assignment` and
	// fatal, so there is no key and no evaluated subscript to choose between.
	// A keyed literal's bare element is an ordinary word here, and comes to
	// one field only because this shell does not split an unquoted expansion.
	// The empty word is the discriminator: `e=; typeset -A m=(p $e q)` is
	// `typeset -A m=( [p]=q )`, the null removed, where an assignment's value
	// would have kept it. See
	// Semantics.BareElementsInATableLiteralAreEachOneValue.
	s.BareElementsInATableLiteralAreEachOneValue = interp.No
	// And they must pair off: an odd number of fields is refused rather than
	// leaving the last one a key with nothing under it. Measured 2026-09-26,
	// `typeset -A h=(a 1 b)` is `bad set of key/value pairs for associative
	// array`, the shell exits 1 and nothing after it runs, where the even
	// `typeset -A h=(a 1 b 2)` and the repeated-key `typeset -A h=(a 1 a 2)`
	// are both taken. See Semantics.BareElementsInATableLiteralMustPairOff.
	s.BareElementsInATableLiteralMustPairOff = interp.Yes
	s.ArrayLiteralSubscriptIsAKey = interp.No
	// A `[k]+=` element joins what the literal has built, not the table it
	// replaced: `typeset -A m; m[k]=v; m=([k]+=x)` is `x`.
	s.KeyedLiteralAppendJoinsTheReplacedValue = interp.No
	// The FUNCTION_ARGZERO option, on by default and the reason this shell
	// alone moves `$0`: it names the function being run, or the file being
	// sourced, and goes back to the script's name when that call returns.
	s.DollarZeroNames = interp.DollarZeroIsTheInnermostCall
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
	// And a missing operand is `not enough arguments` at 1 with the script
	// running on, measured 2026-09-18 on zsh 5.9.2 under both spellings.
	s.DotWithNoOperandIsFatal = interp.No
	// And the mode moves nothing here either: measured 2026-09-19, this
	// shell called `sh` still writes `not enough arguments` at 1 and runs
	// the line after it, which is the row that keeps the companion from
	// being a constant inside the mode (#3818).
	s.DotWithNoOperandIsFatalInPosixMode = interp.No
	s.DotPassesArguments = interp.Yes
	// And the caller's come back over whatever the file did to them, `set`
	// included: measured 2026-09-21 on zsh 5.9.2, `set -- a b c; . ./g p q;
	// echo "$@"` with `set -- m n o p` in the file is `a b c`, where bash
	// lets the `set` stand (#4063).
	s.DotSetCancelsTheRestore = interp.No
	// A leading dash-word is the file here, so `. -p dir f` is a complaint
	// about a file called `-p` and not about an option.
	s.DotReadsOptions = interp.No
	// `eval` is the exception to the line above: it reads the `--` marker
	// and nothing else. `eval -- echo hi` prints `hi`, and `eval -q echo
	// hi` runs `-q` as a command at 127 rather than refusing the letter —
	// which `unset -q` here does refuse, so it is this builtin's answer
	// and not the shell's.
	s.EvalOptions = interp.EvalTakesTheEndMarkerOnly
	// `builtin -q` and `builtin --` are both `no such builtin`: the word is
	// a name. See interp.Semantics.BuiltinReadsOptions.
	s.BuiltinReadsOptions = interp.No
	s.DotTakesTheSearchPathOption = interp.No
	// zsh opens a directory operand, reads no commands out of it and calls
	// that a script that did nothing: measured, `. ./` is silent at status
	// 0 and `. ./ && echo ok` prints `ok`. dash agrees; bash and ksh93 do
	// not. We reported `no such file or directory: ./` at 127 — the status
	// that says the path was never opened — four times in a real startup
	// (#1577).
	s.DotDirectoryOperandIsAnError = interp.No
	// zsh and ksh93 drop the EXIT trap when an exec fails, whichever way it
	// failed; dash and BusyBox ash still run it either way, and bash runs it
	// for a PATH-search miss alone (#3983).
	s.ExecFailureRunsExitTrap = interp.No
	s.ExecFailureOnAPathnameRunsExitTrap = interp.No
	s.ExecTakesOptions = interp.Yes
	// Both letters, and `-a` wins over `-l` in either order.
	s.ExecTakesTheLoginLetter = interp.Yes
	s.ExecTakesTheEmptyEnvironmentLetter = interp.Yes
	s.ExecLoginPrefixesTheGivenName = interp.No
	// And the name a command is started under can be asked for by an exported
	// `ARGV0`, which no other column reads: the variable is spent on argv[0]
	// and the child never sees it. `exec -a` is the other spelling and wins
	// where both are written.
	s.ExportedArgv0NamesTheCommand = interp.Yes
	s.TestAcceptsDoubleEqual = interp.Yes
	// Of the operators past the three-word rules this shell has only `-N`:
	// `test -a f` and `test -o errexit` are `too many arguments` here, and
	// `<` and `>` are `condition expected`.
	s.TestHasTheModifiedSinceReadOperator = interp.Yes
	// And a file written and not read since — equal times, which is every
	// file a script has just created — is `-N` true here, where bash 5.3 and
	// ksh93 want the write to be strictly the later of the two. Measured
	// 2026-09-25 with `touch -t` setting both times, so the case holds still;
	// bash 3.2 is on this side of it too.
	s.TestModifiedSinceReadCountsAnEqualTime = interp.Yes
	// `set -p` is the short spelling of `privileged` here too.
	s.SetHasThePrivilegedLetter = interp.Yes
	// `-nt` and `-ot` want both files to exist, and `-t x` is a plain
	// false rather than an integer complaint.
	s.MissingFileIsOlder = interp.No
	s.TerminalTestRequiresANumber = interp.No
	// And a lone `-t` is `-t 1` rather than a non-empty string, the one
	// reading this shell shares with ksh93: `[ -t ] >/dev/null` is 1 here
	// and 0 in dash and bash.
	s.BareTerminalTestIsDescriptorOne = interp.Yes
	// And a trailing operator with nothing behind it is read as a word,
	// which is what that answer is then applied to: `test -n xx -a -f` is 0
	// and `test -n xx -a -t` is 1 — the word, and then `-t 1` about it.
	s.TestTrailingUnaryOperatorIsAWord = interp.Yes
	// And a connective with nothing behind it is the connective still, over
	// a right operand that is missing and therefore false — the reading this
	// shell shares with dash and with no other column. Measured 2026-09-18:
	// `[ x -a ]` is 1 here, `[ x -o ]` is 0, and `[ 1 -eq 1 -a ]` is 1,
	// silently in each case. The one shape the two columns split on is
	// `[ ! x -a ]`, 1 here and 0 in dash, which is where the rule is asked
	// rather than the rule; the axis records it (#2917).
	s.TestTrailingConnectiveTakesAMissingOperand = interp.Yes
	s.PipefailOption = interp.Yes
	// A substituted element keeps the status its death produced, 128 plus
	// the signal, the same as anywhere else.
	s.PipefailSubstitutesTheBareSignal = interp.No
	s.ErrexitSeesPipefailFailure = interp.Yes
	// The default state of `ERR_RETURN`, which is off — the shell that has
	// the option is still the shell that does not do it unasked. Measured
	// 2026-09-25 on zsh 5.9.2: `unsetopt errreturn; f() { false; print
	// notreached }; f; print "after f: status=$?"` writes `notreached` and
	// `after f: status=0` at 0, in the reference and here alike. `setopt
	// errreturn` moves this axis and nothing else does; see
	// dialect/zsh/setopt.go.
	s.FailureTakesAnImplicitReturn = interp.No
	// `time` changes nothing about the judging here either: `set -e; time
	// false` stops and the ERR trap fires, over every shape measured.
	s.TimedCommandIsJudged = interp.Yes
	// And a redirection that cannot be opened on a compound command is a
	// failure both judges see.
	s.CompoundRedirectionFailureIsJudged = interp.Yes
	// This shell writes two E for `true | ( false )` as well, and for a
	// different reason: its ERR trap runs inside subshells at all, so
	// `( false )` on its own is already two. Measured 2026-09-18, `true |
	// ( exit 3 )` — the row with no failing command inside the element —
	// writes one E here and two in bash, which is what separates the two
	// mechanisms.
	s.ASubshellAsTheLastPipelineElementJudgesItself = interp.No
	// Alone in refusing an argument to `times`; dash and bash ignore it.
	s.TimesRejectsArguments = interp.Yes
	// `[ ( -n x ) ]` is 0 here (#3419).
	s.TestGroupedUnaryAloneLosesTheClosingParen = interp.No
	// `[ ! ! -n x ]` is 0 here, so the four-word `!` negates the negation
	// behind it. Measured 2026-09-19 (#3700).
	s.TestFourWordsNegateANegationOnce = interp.No
	// And the connective before a leading `!` at three words: `[ ! -a / ]`
	// is 0 here where this shell has no unary `-a` at all — `[ -a / ]` is
	// `too many arguments` at 2 — so reading the negation first would refuse
	// and reading the connective first gives the both-set guard over `!` and
	// `/`. Measured 2026-09-19 (#3717).
	s.TestThreeWordsNegateBeforeAConnective = interp.No
	s.TestFailureInsideAnUnclosedGroupIsTheParen = interp.No
	// And a group with nothing in it is `argument expected` at 2 here,
	// which is a refusal rather than a false expression (#3687).
	s.TestEmptyGroupIsFalse = interp.No
	// unanswered LocalThroughCommandDeclaresNothing: `command local a=1` is
	// `command not found: local` at 127 in this shell, so the declaration
	// this axis is about never happens and there is nothing to measure.
	s.UnterminatedBracket = interp.BracketBadPattern
	// And the same question where a `[:name:]`, a `[.x.]` or a `[=x=]`
	// inside it is what left it open: not a pattern, the same as a bare `[`.
	s.UnterminatedBracketAfterASubExpression = interp.BracketBadPattern
	s.UnknownCharacterClass = interp.UnknownClassIsInert
	// The one column with neither construct: `[[.a.]]` is the three-member
	// set `[`, `.`, `a` followed by a literal `]`, so it matches `a]` where
	// four columns match `a` and one matches nothing at all. Measured
	// 2026-09-16 on 5.9.2.
	s.CollatingElements = interp.NoCollatingElements
	// `[[:]` is a bracket holding `[` and `:`, the same reading bash 5.3 gives it.
	// See interp.Semantics.UnterminatedCharacterClass (#1431).
	s.UnterminatedCharacterClass = interp.UnterminatedClassIsOrdinaryCharacters
	// This shell does not glob the result of an expansion, so the axis is
	// reached only through `${~spec}` and `setopt globsubst` — and it has an
	// answer there rather than no answer at all. Measured 2026-09-12 in a
	// directory holding `a\b` and `a*`: `v='a\*'; print -r -- ${~v}` is
	// `a\*`, so the `*` behind the backslash was not live (#1367).
	s.ValueBackslashInAPattern = interp.ValueBackslashDisarmsWhatFollows
	// The piece question never arises with that answer either — the backslash
	// stays a character of the pattern — and is answered for the reason ksh93's
	// is: an axis left open refuses the word the day the one above moves.
	s.ValueBackslashSurvivesAPatternPiece = interp.Yes
	// A bracket carrying a live `/` is a bracket that matches nothing here, and
	// this column needs no option to show it: a pattern matching nothing is an
	// error, and `print -r -- [a/b]` is `no matches found: [a/b]` on 5.9.2. See
	// interp.Semantics.BracketHoldingASlashIsStillABracket (#4158).
	s.BracketHoldingASlashIsStillABracket = interp.Yes
	// ksh93's answer for the trim at the end of a `read` value, measured the
	// same way: `printf 'a b\\ \n' | read x y` leaves `b` here (#1360).
	s.ReadTrailingEscapedSeparator = interp.ReadTrailingEscapedSeparatorTrimmed
	// The arm written first, which is this shell alone in the panel:
	// measured 2026-09-11 on 5.9.2, `x=abc`, `${x##(a|ab)}` is `bc` where
	// `${x##(ab|a)}` is `c`. The same order decides what a `(#b)` reports,
	// which the matcher already followed — and it decides a **substitution**
	// too, measured 2026-09-12: `${x//(a|ab)/X}` is `Xbc` and
	// `${x//(ab|a)/X}` is `Xc`. See
	// Semantics.LongestMatchTakesTheWrittenArm.
	s.LongestMatchTakesTheWrittenArm = interp.Yes
	// An empty pattern is an ordinary pattern that matches the empty string,
	// which is this shell alone: measured 2026-09-12 on 5.9.2, `v=abc`,
	// `${v///X}` is `XaXbXc` where bash and ksh93 leave the value alone.
	// The end of the value is not one of the positions, which is the general
	// rule below rather than anything about this pattern.
	s.EmptyReplacementPattern = interp.EmptyReplacementPatternMatchesEveryPosition
	// The anchors, and an empty pattern behind one still matches at that
	// end: `v=abcabc` gives `Xabcabc` for `${v/#/X}` and `abcabcX` for
	// `${v/%/X}`, which is bash's answer and not ksh93's (#3272).
	s.ReplacementAnchors = interp.Yes
	// And after the global `//` as well, which is this shell alone: the two
	// spellings are one construct here and two everywhere else. Measured
	// 2026-09-16 on 5.9.2 — `${v//#a/X}` on `abcabc` is `Xbcabc` where every
	// other column is `abcabc`, and the discriminating row, `${w//#a/Q}` on
	// `x#ay%bz`, is `x#ay%bz` here and `xQy%bz` there: this shell left the
	// value alone *because* it anchored (#3307).
	s.GlobalReplacementAnchors = interp.Yes
	s.AnchoredEmptyReplacementPattern = interp.Yes
	// The same empty match bash refuses, measured the same way:
	// `${v//(b|)/<>}` under extendedglob is `<>a<><>c`.
	s.ReplacementEmptyMatchDeclined = interp.EmptyMatchDeclinedAtTheEnd
	s.ExitTrapIsFunctionLocal = interp.Yes
	// And the shell runs no EXIT trap at all when it ends over an error it
	// reported with `set -e` on. Measured 2026-09-18 over twelve rows, each
	// written twice: a refused `set` option, a readonly reassignment, a
	// `break` outside a loop, an unset parameter under `set -u` and a
	// division by zero all lose the handler under the option and keep it
	// without; `exit 3`, a plain `false` under the option and `${x?word}` all
	// keep it. See interp.Runner.exitTrapSkippedByAFatalError (#2744).
	s.FatalErrorUnderErrexitSkipsTheExitTrap = interp.Yes
	// A *subshell* loses it for the same errors with no option at all, and
	// for two more besides: `${x?word}`, which keeps it at the top level
	// here, and a refused `set` option, which ksh93's subshell keeps.
	// Measured 2026-09-18 over nine rows in `( … )`, each written plain and
	// under `set -e` — identical both ways. See
	// interp.SubshellExitTrapPolicy (#3612).
	s.SubshellExitTrapAfterAGiveUp = interp.SubshellExitTrapSkippedByABuiltinsUsageToo
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
	s.ExitTrapRunsInsideTheExitingCall = interp.No
	s.QuitIgnoredWhenNotInteractive = interp.Yes
	// And zsh alone parts from bash on what `trap -` then means: it hands
	// SIGQUIT back its default action, so `trap - QUIT; kill -QUIT $$` kills
	// this shell where bash prints the next command. Measured without any
	// handler ever having been installed, so it is a disposition and not a
	// memory of what the script did — and `trap '' QUIT` afterwards brings
	// the ignore back.
	s.QuitResetRestoresTheDefault = interp.Yes
	// This shell alone: an untrapped SIGHUP ends the shell with 1 and runs
	// the EXIT trap, rather than killing it with 128 plus the number. The
	// EXIT trap is why it is not simply a different number — the line above
	// says dying does not run it, and here one runs.
	s.HangupIsAnOrderlyExit = interp.Yes
	// zsh alone: a bare `exit` there reports what the trap's own last
	// command did, so `trap "false; exit" 0` exits 1.
	s.ExitInTrapReportsEarlierStatus = interp.No
	s.KillListAcceptsName = interp.Yes
	// One subtraction, and what it cannot name it prints back — `kill -l
	// 160` is `160` here. No EXIT: this shell answers `kill -l 0` with 0.
	s.KillListReducesRepeatedly = interp.No
	s.KillListPrintsANumberItCannotName = interp.Yes
	s.KillListNamesZeroAsExit = interp.No
	s.SIGPrefixAccepted = interp.Yes
	s.RedirectsUseEveryTarget = interp.Yes
	// `exec {fd}< /etc/hosts; echo $fd` says 11 here and 10 in bash 5.3 and
	// ksh93 — measured 2026-09-12. The same number comes back from `zsocket`
	// in `$REPLY` and from `sysopen -u name` (#1752).
	s.FirstAllocatedDescriptor = interp.AllocateDescriptorsFromEleven
	// `exec 3<<X` puts the body in a temporary file here, so `/dev/fd/3` is
	// a regular file and the descriptor is seekable: `head -1 <&3` reads a
	// block and seeks back, and the `cat <&3` after it still gets the rest of
	// the document. bash 5.3.15 and dash use a pipe and lose it. Measured
	// 2026-09-14 (#2759).
	s.HeredocBody = interp.HeredocBodyInATemporaryFile
	// A read that fails after the open worked fails the substitution here:
	// `mkdir dir; v=$(<dir)` is status 1 with a sentence, where bash 5.3 and
	// ksh93 are status 0 in silence (#1778).
	s.ReadFailureInAFileSubstitutionFailsIt = interp.Yes
	// The null-command hook: a command that is only redirections runs
	// `$NULLCMD`, or `$READNULLCMD` where its one redirection is a plain
	// `<file`. The names, not the values — a script reassigns them at will,
	// and the defaults live in the prelude where a script can see and change
	// them. `setopt cshnullcmd` and `setopt shnullcmd` move these two fields
	// rather than adding a branch; see nullcommand.go in this package.
	s.NullCommandVariable = nullCommandParameter
	s.ReadNullCommandVariable = readNullCommandParameter
	s.KillStatus = interp.KillStatusFailureCount
	// A subshell sees the parent's job table when the parent's monitor is
	// on, and sees nothing when it is off — which is every script, so this
	// used to read Cleared and was right about every case a script can
	// reach. Measured 2026-09-25 against zsh 5.9.2: `zsh -f -c 'sleep 1 &
	// jobs; (jobs)'` writes the row once, and the same line under `-fm` on a
	// pseudo-terminal writes it twice. The monitor and not the terminal and
	// not the prompt: `zsh -fi` with `unsetopt monitor` writes it once
	// again (#4538). The rows and what the subshell may do with them are on
	// interp.SubshellJobsKeptUnderTheMonitor.
	s.SubshellJobTable = interp.SubshellJobsKeptUnderTheMonitor
	s.PrintfEmptyIsNotANumber = interp.No
	s.PrintfAbsentNumberIsAnEmptyOne = interp.No
	s.PrintfStarWithoutOperandIsRefused = interp.No
	s.PrintfStarComplaintCostsTheStatus = interp.Yes
	// zsh alone writes the bare word: `%G` of an infinity is `inf` and
	// `%10f` of one is `inf` unpadded.
	s.PrintfNonFiniteIsConverted = interp.No
	// The `'` flag, among the flags and nowhere else: `%15'd` is
	// `%15': invalid directive`.
	s.PrintfGroupingFlag = interp.Yes
	s.PrintfGroupingFlagAfterTheWidth = interp.No
	// A `*` beside a width's own digits is refused here too: `printf '%5*d' 4 42`
	// is a conversion character this shell does not have (#2824).
	s.PrintfStarBesideTheFieldDigits = interp.No

	// The width is stored in a C int and what is left is used, so
	// `printf '[%21474836470s]' x` — -10 as an int32 — is `[x         ]`
	// and `%4294967306s` is `[         x]`. Measured 2026-09-15.
	s.PrintfFieldBeyondAnInt = interp.PrintfFieldWrapsToAnInt

	// Wraps on both routes, which is what makes the literal spelling and
	// the star agree here. Measured 2026-09-15.
	s.PrintfStarBeyondAnInt = interp.PrintfStarWrapsToAnInt
	s.CaseSubjectKeepsThePreviousLine = interp.No
	// unanswered SubstringRangeThirdColonIsABadSubstitution: a third segment
	// is a *modifier list* here — `${x:1:5:t}` is `D` — so the question of
	// what to call the refusal never arises.
	s.PrintfReportsBadNumber = interp.No
	s.PrintfNumberOperand = interp.PrintfNumberArithmetic
	// Exact, though the reading is an expression: this shell's arithmetic is
	// an integer one and `printf '%d' 123456789012345678` is the operand
	// (#2907). It is ksh93 alone that rounds.
	// A flag past a field is no flag at all: the prefix ends there and the
	// byte arrives at the scan as the conversion character (#2910).
	// A division by zero ends the reading of the operand here, which is
	// what the zero beside `printf '%d' 1/0` says (#2912).
	s.ArithDivisionByZeroYieldsAValue = interp.No
	s.PrintfFlagAfterTheField = interp.No
	s.PrintfIntegerOperandGoesThroughTheFloatingType = interp.No
	// None of C99's three: `printf '%F' 1.5` is `%F: invalid directive` at
	// 1 in zsh 5.9.2, and `%a` and `%A` the same.
	s.PrintfC99FloatConversions = interp.No
	// unanswered PrintfHexFloatZeroFillPrecedesThePrefix: and so no fill to
	// place in it either.
	// unanswered PrintfHexFloatDefaultIsTwelveDigits: there is no `%a` here
	// to have a default precision.
	s.PrintfRefusedOperandKeepsItsLeadingNumber = interp.No
	// One complaint per operand whichever conversion asked: `printf '%f'
	// 42abc` writes one arithmetic line here, as `printf '%d' 42abc` does.
	s.PrintfFloatOperandIsEvaluatedTwice = interp.No
	s.PrintfBackslashC = interp.PrintfBackslashCStops
	s.PrintfUnfinishedConversionIsAPercent = interp.No
	// The same two digits bash reads, and an empty digit run is a zero
	// rather than an escape left standing: `printf 'a\xZ'` is a NUL here.
	// unanswered PrintfReportsAMissingHexDigit: an empty digit run is a zero
	// here too, by the reading beside this line, so the complaint's site is
	// unreachable. The two columns that do reach it split, and
	// TestPrintfMissingHexDigit pins the pair (#3239).
	s.PrintfHexEscape = interp.PrintfHexEscapeByteOrNul
	// A `%b` argument reads the same escape the same way, NUL and all.
	s.PrintfBHexEscape = interp.PrintfHexEscapeByteOrNul
	// `\u` and `\U` at both sites, and an empty digit run is a zero here
	// too: `printf 'a\uZ'` is an `a`, a NUL and a `Z`, where bash leaves the
	// escape standing and complains.
	s.PrintfUnicodeEscape = interp.PrintfUnicodeEscapeCodePointOrNul
	s.PrintfBUnicodeEscape = interp.PrintfUnicodeEscapeCodePointOrNul
	// The format site splits the same way this shell's `%b` does, and in
	// the other direction from ksh93's format: `printf 'a\eZ'` is
	// `61 1b 5a` and `printf 'a\EZ'` is `61 5c 45 5a` in 5.9.2 (#3225).
	s.PrintfEscEscape = interp.Yes
	s.PrintfCapitalEscEscape = interp.No
	// An escape a format does not define keeps its backslash — `printf
	// '[\q][\z][\8][\-]'` is `[\q][\z][\8][\-]` — and a floating
	// conversion's exact half goes to the even neighbor: `printf '%.0f
	// %.0f %.0f' 2.5 4.5 -2.5` is `2 4 -2` and `%.2f` of 0.125 is `0.12`.
	// Measured 2026-09-18 under `LC_ALL=C` from a script file; ksh93 is the
	// one column on the other side of both.
	s.PrintfUnknownEscapeDropsTheBackslash = interp.No
	s.PrintfFloatHalf = interp.PrintfFloatHalfToEven
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
	// Characters, where the locale has them: `printf '[%.2s|%7s]' αβγ αβγ`
	// is `[αβ|    αβγ]` under a UTF-8 locale and `[α| αβγ]` under C, zsh
	// 5.9.2, 2026-09-16 — the one column counting characters. The `l` is
	// ignored, so `%ls` is `%s` and `%lc` is `%c`'s byte.
	s.PrintfFieldCountsCharacters = interp.Yes
	s.PrintfLongModifierCountsCharacters = interp.No
	// No `%(fmt)T`: `%(` is a directive this shell does not have.
	s.PidListingFinishesWithAJob = interp.No
	s.PrintfTimeConversion = interp.No
	s.PrintfTimeOperandIsADateString = interp.No
	s.PrintfQuote = interp.PrintfQuoteAnsiCCharacter
	// C's `#` at a value of nought, as in bash: `printf '%#x' 0` is `0`.
	s.PrintfAlternateFormAsksTheValue = interp.Yes
	// And counts it against the width: `printf '%#05x' 7` is `0x007`,
	// five characters, on zsh 5.9.2.
	s.PrintfZeroFillCountsTheAlternatePrefix = interp.Yes
	// zsh is the one shell with `$'…'` and no `\c` in it, so `$'\cA'` is the
	// two characters `cA`; and its strings are counted rather than
	// terminated, so a decoded NUL is a byte like any other.
	s.DollarSingleBackslashC = interp.DollarSingleControlAbsent
	s.DollarSingleUnknownEscape = interp.DollarSingleUnknownDropsBackslash
	s.DollarSingleNul = interp.DollarSingleNulIsAByte
	// Two digits after `\x` and no more, as in bash — but a run with no
	// digit at all is a zero byte here, where bash keeps the two characters
	// it was written as. This shell keeps the zero, so `$'\xzz'` is three
	// bytes.
	s.DollarSingleHexReadsEveryDigit = interp.No
	s.DollarSingleDigitlessEscapeIsAZeroByte = interp.Yes
	// There is no braced spelling at all, so `$'a\x{41}b'` is the zero byte
	// the axis above gives a digitless `\x`, and then `{41}b` as ordinary
	// text. Measured 2026-09-22 under `LC_ALL=C` on zsh 5.9.2 (#4165).
	s.DollarSingleBracedHex = interp.DollarSingleBracedHexAbsent
	// An octal escape past 255 keeps the low byte: `$'\401'` is 01, and
	// `$'\400'` is the zero byte this shell holds as a character of the
	// text. Measured 2026-09-18 by `od` (#3415).
	s.DollarSingleOctalPastAByteDropsTheLastDigit = interp.No
	// `\C-X` is a control character and `\M-X` the same byte with the high
	// bit set, the dash optional in both and either able to take the other as
	// its argument. ksh93 writes `\C-A` too and means `m` then `A` by it, so
	// the value names the reading rather than saying yes (#2345).
	s.DollarSingleCaretMeta = interp.DollarSingleCaretMetaMaskedWithAnOptionalDash
	// `\e` and `\E` are both the escape character here, which is **not** the
	// split this shell's `echo -e` has — there it takes `\e` and leaves `\E`
	// standing. One site's answer is not the other's, and measuring both is
	// what #3270 was about. `\?` and the two Unicode spellings are read too:
	// `3f`, and `41` from $'\u0041' and from $'\U00000041'.
	s.DollarSingleEscEscape = interp.Yes
	s.DollarSingleQuestionEscape = interp.Yes
	s.DollarSingleUnicodeEscapes = interp.Yes
	s.GetoptsAssignmentRestartsWord = interp.No
	// unanswered GetoptsRefusedNameStillScans: a refused name operand ends
	// the shell here, so nothing downstream can read OPTIND back and say
	// whether the scan ran. Measured 2026-09-21, `set -- -a; getopts a
	// opt-var; echo "rc=$? OPTIND=$OPTIND"` writes `zsh:1: not an
	// identifier: opt-var` and stops — the echo never runs, under `emulate
	// sh` as well. The fatality itself is BadNameToGetoptsFatal's and is
	// answered there.
	// `kill %1` reaches the job's process. dash aims at the group.
	s.KillJobSpecAimsAtTheGroup = interp.No
	// This shell continues a stopped job before it sends, which is what
	// makes the signal land rather than sit pending: measured 2026-09-25
	// through a pseudo-terminal, `sleep 300 &` then `kill -STOP %1` reads
	// `TN` from `ps -o stat=`, and `kill -0 %1` — which delivers nothing at
	// all — reads `SN` afterwards, as does `kill -WINCH %1`. Every other
	// column leaves both at `T`. The `%` word is what decides: `kill -0
	// "$!"` on the same job in the same session leaves it `TN`, so the
	// number is a number here even though this shell knows whose it is. Not
	// for a signal that stops — STOP, TSTP, TTIN and TTOU all leave it `TN`
	// — and it makes no difference whether `^Z` or an explicit `kill -STOP`
	// put it there.
	s.KillJobSpecContinuesAStoppedJob = interp.Yes
	// zsh 5.9.2 sends it: `kill -0 -- -1` is 0.
	s.KillRefusesTheAllProcessesTarget = interp.No
	// zsh 5.9.2 leaves the name unset.
	s.ArrayOperandIsStoredPastAFailedOpen = interp.No
	// zsh 5.9.2 reaches the target behind either spelling.
	s.KillTakesEndOfOptionsAfterTheSignal = interp.Yes
	// Measured 2026-09-26 against zsh 5.9.2: `kill a b c` writes three
	// `illegal pid` lines and reports 3, `kill a b c d` four and 4. The
	// count is this shell's KillStatus; the four lines are this axis.
	s.KillKeepsGoingPastAnOperandThatIsNotAPid = interp.Yes
	// A trim on `$@` runs over each field, as it does in bash.
	s.OperatorDistributesOverTheFieldList = interp.Yes
	// OPTIND names the word until its last letter has been read.
	s.GetoptsCountsTheWordAtItsFirstLetter = interp.No
	// The other end of that question: the word is not counted when its last
	// letter is read either. OPTIND stays on it and the *next* call moves
	// the count, so the parameter lags a word behind the scan all the way
	// along and catches up only where the options run out. An option that
	// took an argument is the exception and counts as it does everywhere
	// else (#3275).
	s.GetoptsCountsTheWordOnTheNextCall = interp.Yes
	// And the run that reports "no more options" leaves the name holding
	// whatever it held, where the other six write `?`. After a scan that
	// read something that is the last letter still standing, which is how it
	// reads as a written value and is not one.
	s.GetoptsEndOfOptionsNamesIt = interp.No
	// A `+`-prefixed word is an option word here, and this shell is alone in
	// it: `getopts a o +a` is 0 with `+a` in the name, the sign kept so a
	// script can tell it from `-a`. `+ab` clusters, `+a val` and `+aval` take
	// the argument an `a:` letter takes, and a letter the string does not
	// have is `bad option: +z` with `+z` in OPTARG under the silent form. A
	// lone `+` is still an operand and `++` is the option `+` rather than an
	// end-of-options word. Measured 2026-09-17 on 5.9.2.
	s.GetoptsTakesAPlusPrefixedOption = interp.Yes
	// OPTIND is local to a shell function here: the call starts at 1 and the
	// caller's position — words and the place inside a clustered word alike —
	// comes back on return. It is what lets this shell's own function
	// library parse options without resetting OPTIND by hand.
	s.GetoptsFunctionPosition = interp.GetoptsFunctionPositionIsLocal
	// The same answer by a second route, and recorded rather than left to
	// the axis above: every call here has its own cursor whether or not
	// anything was declared, so a declaration inside one cannot be the
	// thing that loses the caller's place.
	s.GetoptsLocalOptindRestoresTheCursor = interp.Yes
	// `#` in an option string is another option letter here. Measured
	// 2026-09-16, zsh 5.9.2 (#2947).
	s.GetoptsOptionStringHasANumericType = interp.No
	// OPTERR is an ordinary variable here: measured 2026-09-17 on 5.9.2, a bad
	// option is one line on stderr with it set to 0 as with it set to 1.
	s.GetoptsOptErrSilencesTheComplaint = interp.No
	s.GetoptsClearsOptarg = interp.Yes
	// And the same for an option that simply takes none, which is the other
	// axis and the one this shell shares with dash and BusyBox ash.
	s.GetoptsEmptiesOptargForAnArgumentlessOption = interp.Yes
	// OPTARG and OPTIND are written through a freeze without a word, as in
	// ksh93 — measured 2026-09-16 on 5.9.2.
	// The last option's argument stands after the scan runs out, as in dash.
	s.GetoptsUnsetsOptargAtEndOfOptions = interp.No
	// The clearings this shell does make write an empty string through the
	// freeze rather than removing the name, so the freeze is still on
	// afterwards.
	s.GetoptsClearingOptargIsARealUnset = interp.No
	s.GetoptsOwnParametersIgnoreAFreeze = interp.Yes
	s.GetoptsRefusedWriteEndsTheBuiltin = interp.No
	// Unreachable while GetoptsEndOfOptionsNamesIt is no: there is no write
	// at the end of the options for a freeze to refuse, so `N=kept; readonly
	// N; set -- x; getopts ab N` is silent at 1 — measured 2026-09-17 on
	// 5.9.2. Answered so that nothing reports an axis this shell cannot be
	// asked.
	s.GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal = interp.No
	// Neither `read` axis is reachable here: this is the one column whose
	// refused write inside a builtin ends the script, and that is asked
	// first. Answered so that nothing reports an axis this shell cannot be
	// asked, the way the getopts twin above is.
	s.ReadRefusedWriteEndsTheBuiltin = interp.No
	s.ReadRefusedWriteIsOneOnTheLastName = interp.No
	// The freeze on the *name* is reached, and this is the one shell where a
	// builtin's refused write ends the script: `readonly o; getopts a: o;
	// echo reached` prints neither `reached` nor anything after it.
	s.ReadonlyRefusalInABuiltinIsFatal = interp.Yes
	// Measured 2026-09-12: `OLDPWD=/usr zsh -c 'echo $OLDPWD'` answers `$PWD`,
	// so an inherited value never arrives at all — the name is this shell's own
	// record of where it has been, and it starts where the shell started. That
	// is why `OLDPWD=/nonexistent zsh -c 'cd -'` says nothing and reports 0:
	// there is no unusable value to refuse.
	s.InheritedOldpwd = interp.InheritedOldpwdIgnored
	// The depth, counted and told to every child, with no ceiling: measured
	// 2026-09-16, zsh 5.9.2 takes an inherited 9999 to 10000 and says
	// nothing. See interp.ShellLevelPolicy.
	s.ShellLevel = interp.ShellLevelCounted
	// And a shell that replaces this process stands in its place: measured
	// 2026-09-18, `exec /usr/bin/env` hands over `SHLVL=0` where this shell
	// holds 1. With no floor, so an inherited `-1` reaches the replacement as
	// `-1` and it reads 0 — which is where this column parts from bash's
	// answer to the same question.
	s.ShellLevelExec = interp.ShellLevelExecNotCounted
	// And the starting directory is named by what the kernel reports, however
	// the parent spelled it.
	s.StartupPwdName = interp.StartupPwdNameFromTheKernel
	s.CdWithoutHomeIsAnError = interp.No
	s.CdDashPrintsTheDirectory = interp.No
	// The same as bash, and it is the chunk `B01cd.ztst` ends on. Measured
	// 2026-09-26 on zsh 5.9.2 (`-f`): with `d` renamed to `e`, `cd .` is 0
	// and `$PWD` becomes `…/e` while a bare `pwd` goes on printing `…/d` —
	// the kernel's name reaches `$PWD` at the `cd` and nowhere else. See
	// Semantics.CdDestinationIsNotThere.
	s.CdDestinationIsNotThere = interp.CdDestinationNotThereEntersAndTakesTheKernelsName
	s.PrintfAssignsWithV = interp.Yes
	s.PrintfRejectsUnknownOption = interp.No
	// zsh reads no options here: `trap -p` sets a trap whose action is the
	// word `-p`, and the failure surfaces when it fires.
	s.TrapParsesOptions = interp.No
	s.TrapOneArgumentIsACondition = interp.Yes
	// And the one column that refuses a number the table cannot name.
	// Measured 2026-09-19 on linux/arm64, zsh 5.9 in Debian bookworm, the
	// script file `trap 'echo CAUGHT' $1; kill -$1 $$; echo SURVIVED`: 15
	// prints CAUGHT then SURVIVED, and 40 and 64 are `undefined signal: N`
	// with the shell then dying of the signal at 128 + N. Written here rather
	// than left to the preset, because the value a preset happens to hold is
	// not a measurement and the next reader cannot tell the two apart.
	s.TrapTakesASignalNumberItCannotName = interp.No
	s.TrapReportsAnUnknownSingleCondition = interp.No
	// ERR and DEBUG but not RETURN, and both follow the script everywhere:
	// into functions, and — alone in the panel — into subshells and command
	// substitutions, where the handler's output is captured with the rest.
	s.TrapHasErrCondition = interp.Yes
	s.TrapHasDebugCondition = interp.Yes
	s.TrapHasReturnCondition = interp.No
	// And ERR answers to `ZERR` as well, which is the name zsh's own
	// documentation gives it and the one a startup file writes. One slot
	// under two names: `trap 'echo E' ERR; trap - ZERR; false` fires nothing
	// (#2771).
	s.TrapErrConditionIsAlsoZERR = interp.Yes
	// zsh alone reads a *function* name as a condition: `TRAPZERR() { … }`
	// installs the ZERR handler with no `trap` command anywhere, and
	// `TRAPINT`, `TRAPEXIT` and `TRAPDEBUG` do the same for theirs. It is
	// the spelling that fails silently in a shell without it, because a
	// declaration parses everywhere — see interp/trapfunction.go.
	s.TrapIsNamedByAFunction = interp.Yes
	s.ErrTrapRunsInsideFunctions = interp.Yes
	s.ErrTrapRunsInSubshells = interp.Yes
	// One failure, one firing, however deeply the command that failed was
	// called: `f(){ g; }; g(){ h; }; h(){ false; }; f` writes a single E,
	// and with the action printing `${funcstack[*]}` it reads `h g f` — so
	// the firing is inside the frame that failed and none of the three
	// calls is an event of its own. Read from the frame rather than counted:
	// a `return 1` judged at the call instead would write the same one E,
	// and only the action's view of the locals, the arguments and the stack
	// says which happened. A subshell still fires on both sides, which is
	// the axis above and not this one.
	s.ErrTrapRefiresForTheCommandItFiredInside = interp.ErrTrapFiresOnceForTheFailure
	// A pipeline whose last element ran here is judged once, by its status,
	// unless that element is a compound whose body judged itself: `true | {
	// false && true; }` carries on under `set -e`.
	s.FailingPipelineWhoseLastElementRanHere = interp.PipelineJudgedUnlessItsLastElementJudgedItself
	s.DebugTrapRunsInsideCalls = interp.Yes
	s.DebugTrapRefiresOnEnteringAFunction = interp.No
	// Every compound head, once each — `if`, `while`, a group, a subshell,
	// `repeat` and a function *definition* among them — and a loop's passes
	// are inside that once rather than beside it.
	s.DebugTrapCompoundHeads = interp.DebugTrapHeadsEveryCompound
	// A pipeline is one statement here and fires once, in the shell running
	// it, however many elements it has — and no element fires a head of its
	// own. Measured: `trap 'echo d' DEBUG; echo a | tr a-z A-Z` writes
	// `d A`, and a four-element pipeline still writes one. Commands nested
	// *inside* an element are commands in their own right and go on firing:
	// `echo A | { cat; }` writes one for the pipeline and one for the
	// `cat`.
	s.DebugTrapPipelines = interp.DebugTrapPipelineOnceForThePipeline
	// And once for a whole `&&`/`||` list, which is the same reading one
	// layer further out: measured 2026-09-25 on 5.9.2 under `-f`, `print x
	// && print y` writes one firing where bash and ksh93 write two, and
	// `$ZSH_DEBUG_CMD` at it reads the whole list back rather than the first
	// operand (#4556). An operand that is a compound fires no head of its
	// own either; what is inside one fires as usual.
	s.DebugTrapSublists = interp.DebugTrapSublistOnceForTheList
	s.DebugTrapRunsInSubshells = interp.Yes
	// Ahead of the command, which is `DEBUG_BEFORE_CMD` on — zsh's own
	// default, measured 2026-09-25 on 5.9.2 under `-f`. This is the only one
	// of these axes a script can move: `unsetopt DEBUG_BEFORE_CMD` puts
	// every firing behind the command it would have preceded, so the entry
	// in setopt.go writes this axis rather than remembering the request
	// (#4473). See Semantics.DebugTrapRunsBeforeTheCommand for the grid.
	s.DebugTrapRunsBeforeTheCommand = interp.Yes
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
	// An empty operand is a limit of nought: `ulimit -n ""` is silent at 0
	// and leaves `ulimit -n` answering `0` (#3064). Measured 2026-09-18.
	s.UlimitEmptyOperandIsZero = interp.Yes
	s.UlimitSetsBothLimits = interp.No
	// zsh takes `hard` and refuses `soft`, which is why the two words are
	// two axes: `ulimit -n soft` here is `invalid number: soft`.
	s.UlimitTakesHardKeyword = interp.Yes
	s.UlimitTakesSoftKeyword = interp.No
	s.UlimitOperandIsArithmetic = interp.No
	s.BadOptionToSpecialBuiltinFatal = interp.No
	// And this shell's POSIX mode does not move it, which is the departure
	// from the standard's preset and from every other dialect here. The mode
	// has one door in this shell — there is no `set -o posix`, `set -o posix`
	// is `no such option` at 1 — so it is the name that reaches it, and a
	// symlinked `sh` answers `bad option: -x` and then `alive` at 0, exactly
	// as this shell does under its own name. Measured 2026-09-13 for both
	// `shift -x` and `export -q`.
	//
	// Not a claim that the mode does nothing here: the same shell under the
	// same name *does* move RedirectErrorOnSpecialBuiltinFatal, stopping on
	// `export x > /nonexistent/dir/f` where it carries on as `zsh`. Two axes,
	// one mode, opposite answers — which is why this is a field a dialect
	// fills in rather than a constant in SetPosixMode (#2583).
	s.BadOptionToSpecialBuiltinFatalInPosixMode = interp.No
	// `alias` is no more special here than POSIX makes it: the complaint is
	// said and the next command runs. Measured with `alias -g x`.
	s.AliasBadOptionFatal = interp.No
	// unanswered AliasNameCheckReachesALookup, AliasInvalidNameFatal: this
	// shell checks no alias name at all, so AliasNameRefusedCharacters is
	// empty and neither question is ever reached. Measured 2026-09-12:
	// `alias 'a b'=echo` is accepted in silence here and the name is listed
	// back, where the two shells that check refuse it (#2413).
	s.EarlierDeclarationLetterBlocksALaterPlus = interp.No
	// Nor does a redirection that cannot be made end anything: the message
	// is printed and the script runs on. The starting value only — `emulate
	// sh` and `emulate ksh` move it to the POSIX answer, and `emulate zsh`
	// puts it back.
	// Read as a number like any other; nothing about the width is
	// refused.
	s.MultiDigitDuplicationTargetIsAnError = interp.No
	s.RedirectErrorOnSpecialBuiltinFatal = interp.No
	// A substitution's body does hold `-e`, which is the neighboring axis
	// answering the other way: measured 2026-09-15,
	// `set -e; echo "end[$(false; echo no)]"` is `end[]` in zsh 5.9.2 under
	// every `emulate`, and under the name `sh` too. So nothing moves it and
	// emulate.go leaves it alone.
	s.ErrExitEntersACommandSubstitution = interp.Yes
	// The same csh spelling, and one step further: a word that expanded to
	// nothing is a name too, so `>&""` opens the empty path and fails on it
	// rather than complaining about a descriptor.
	s.GreatAmpTarget = interp.GreatAmpTargetNamesAnyFile
	// zsh has no move operator, and the absence shows in two different
	// answers rather than one refusal. `<&` wants a number and says `file
	// number expected` at 1; `>&5-` is a word that names no descriptor, so
	// GreatAmpTarget above sends it to a *file* called `5-` — measured, and
	// the file really appears.
	s.FdMove = interp.FdMoveIsNotAnOperator
	// And the reading side ends the shell, on a builtin alone.
	s.DuplicationTargetError = interp.DuplicationTargetErrorEndsTheShellOnABuiltin
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
	// And `printf -v` ends it too: measured 2026-09-17,
	// `printf -v '1x' %s Q` is `not an identifier: 1x` and nothing after
	// it runs, the same as `read '1x'`. `a-b` gets that sentence as well,
	// which is why there is no BuiltinBadNameNumeric row beside printf's.
	s.BadNameToPrintfFatal = interp.Yes
	// And at `getopts`, measured 2026-09-18: `echo A; getopts x 1bad -x;
	// echo B` writes A and `not an identifier: 1bad` and never reaches B,
	// where the other six columns all run it.
	s.BadNameToGetoptsFatal = interp.Yes
	// A subscripted operand is a name here, as it is at `read` — measured
	// 2026-09-18, `getopts x 'o[1]' -x` is 0 with the letter in `$o`. The
	// `o[0]` spelling is `o: assignment to invalid subscript range`, which
	// is this shell's one-based arrays refusing the *position*; asking only
	// that spelling would have read this column as a refusal (#3555).
	s.GetoptsOperandTakesASubscript = interp.Yes
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
	// The element is written and the array's own attributes are left
	// alone: measured 2026-09-17, `a=(1 2 3); export 'a[1]'=v` leaves
	// `typeset -a a=( v 2 3 )` with no `x` on it and nothing in a child's
	// environment, and `export 'a[1]'` with no value agrees.
	s.ExportThroughASubscriptedOperandRecordsTheLetter = interp.No
	// The declaration builtins take one: `typeset a[1]=v` sets the element
	// and reports success, measured.
	s.TypesetTakesASubscript = interp.Yes
	s.UnsetTakesASubscript = interp.Yes
	// The same here: one element removed, the rest standing.
	s.UnsetElementEmptiesAnUnwrittenArrayInASubshell = interp.No
	// `read 'a[2]'` fills the element, measured 2026-09-10 on `a=(x y z)`.
	s.StoreOperandTakesASubscript = interp.Yes
	// The brackets name the whole array and the operand fills it, silently
	// and at 0: measured 2026-09-17, `r=(1 2 3); read 'r[@]'` on `Y`
	// leaves `r` as the one element `Y`. Not a split — `read 'r[@]' b` on
	// `X Y` leaves `r` as the one element `X` and fills `b` with `Y` — so
	// the operand takes its field like any other name and the spelling is
	// what makes the array one element, exactly as `r[@]=X` does.
	s.StoreOperandWholeArraySubscript = interp.StoreOperandWholeArraySubscriptNamesEveryElement
	// Over a **table** it is refused by name and the input ends: measured
	// 2026-09-17, `typeset -A m=(k v); read 'm[@]'` is `m: attempt to set
	// slice of associative array` and nothing after it runs. So this shell
	// takes the array and refuses the table, the other way round from bash
	// — the same swap the bare assignment makes.
	s.StoreOperandWholeArraySubscriptOverATable = interp.WholeArraySubscriptIsASliceOfATable
	// An element is not a name, so a declaration that would freeze, type or
	// localize the *array* refuses the operand instead of doing half of it —
	// and it ends the script. Three wordings for the three, measured
	// 2026-09-07, which is how a script tells which of them it ran into.
	// The same as ksh93 and for the same measurement, `unset` included: a
	// fatal `unset ":" ok1 ok2` removes ok1 and ok2 before it stops.
	s.BadNameDeclaresTheOperandsAfterIt = interp.Yes
	s.SubscriptedOperandTakesTheIntegerAttribute = interp.No
	// And the container letter, refused in the same breath — `typeset -A
	// m[k]=v` and `typeset -a n[2]=v` alike are `inconsistent type for
	// assignment`, and so is the letter over a name already a table of that
	// very kind, so it is the letter beside the subscript that is refused
	// rather than anything the letter would change.
	s.SubscriptedOperandTakesTheContainerAttribute = interp.No
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
	// An empty key is a key here too, though `m[""]` is not the way to reach
	// one: the subscript is not a quoting context, so those brackets hold a
	// two-character key and `m[$w]` with an empty `$w` holds the empty one.
	// Measured 2026-09-12, both store and the table ends with two elements
	// (#1938).
	s.EmptyAssociativeKeyIsAnError = interp.No
	// Either letter takes a name that is already the other kind, and the
	// elements are gone: measured 2026-09-12, `typeset -A h; h[k]=v;
	// typeset -a h` is `typeset -a h=(  )` at status 0 and the reverse is
	// `typeset -A a=( )`. The one column that converts in both directions,
	// and the only one that loses the values doing it (#1375).
	s.TableUnderAnArrayDeclaration = interp.CompoundKindChangeEmptiesTheName
	s.ArrayUnderATableDeclaration = interp.CompoundKindChangeEmptiesTheName
	// The literal form converts and empties in both directions too, the same
	// as the valueless one. Measured 2026-09-12, `typeset -A h; h[k]=v;
	// typeset -a h=(x)` lists `typeset -a h=( x )` at 0 and `typeset -a a=(x
	// y); typeset -A a=([k]=v)` lists `typeset -A a=( [k]=v )` at 0 (#2287).
	s.TableUnderAnArrayLiteralDeclaration = interp.CompoundKindChangeEmptiesTheName
	s.ArrayUnderATableLiteralDeclaration = interp.CompoundKindChangeEmptiesTheName
	// `a[@]=Z` names every element: the name comes out holding the one value
	// whatever it held before, and `a[@]+=Z` adds one at the end. Over a
	// table the same spelling is refused by name and the input ends.
	// Measured 2026-09-12, `x=(p q); x[@]=Z` leaves `${#x[@]}` at 1, `v=s;
	// v[@]=Z` and a bare `u[@]=Z` both leave a one-element array, and
	// `typeset -A m; m[k]=v; m[@]=Z` is `m: attempt to set slice of
	// associative array` under both separators (#2285).
	s.WholeArraySubscriptAssigningAnArray = interp.WholeArraySubscriptNamesEveryElement
	s.WholeArraySubscriptAssigningATable = interp.WholeArraySubscriptIsASliceOfATable
	// And the `(P)` route answers it the same way, which is the column that
	// does *not* swap sides between the two.
	s.WholeArraySubscriptThroughAReferenceToATable = interp.WholeArraySubscriptIsASliceOfATable
	// Nor is reading one reported: measured 2026-09-12, `typeset -A m;
	// m[k]=v; w=; ${m[$w]}` is the empty string at status 0 and silent
	// (#1972).
	s.EmptyAssociativeKeyIsReportedWhenRead = interp.No
	// Nor is its *length* refused: measured 2026-09-12, `typeset -A m;
	// m[k]=v; w=; echo "[${#m[$w]}]"; echo after` is `[0]`, `after` and
	// status 0, where bash stops the script on the same line (#2286).
	s.EmptyAssociativeKeyRefusesTheLength = interp.No
	// An empty positional list is a **set** parameter here, with dash and
	// against the four bash-and-ksh columns: measured 2026-09-12, `set --;
	// "${@-word}"` is empty and `"${@+word}"` is `word` (#1941).
	s.PositionalListWithNoneIsSet = interp.Yes
	// The third reading of what an empty `$@` takes with it, and the one that
	// makes the axis three-valued: only what stands *before* the list goes with
	// it, so `"$e$@"` is no argument and `"$@$e"` is one.
	s.EmptyListTakesTheWord = interp.EmptyListReachWhatStandsBeforeIt
	// The expand-first order, with dash and ksh93 (#1943).
	s.PrefixToAFrozenNameIsCheckedFirst = interp.FrozenPrefixCheckedWithTheCommand
	// A `*` or `@` subscript inside an expression is the slice, joined and
	// then read as an expression: measured 2026-09-11 on 5.9.2, `typeset -A
	// m; m[k]=9; $(( m[*] ))` is 9 and `a=(1+1); $(( a[*] * 3 ))` is 6.
	s.ArithWholeArraySubscriptIsTheSlice = interp.Yes
	// So the question below it is never reached here: the slice is the
	// answer before anything can call the subscript bad. Answered rather
	// than left open because an unanswered axis is a refusal, and a reading
	// this dialect cannot reach must not be able to produce one (#1978).
	s.ArithWholeArraySubscriptIsReportedAsBad = interp.No
	// A subscript whose text expanded to nothing is not read as an
	// expression here at all: measured 2026-09-11 on 5.9.2, `a=(5 6 7); w=;
	// ${a[$w]}` is `bad math expression: empty string` and the shell ends,
	// where `${a[ ]}` is the expression running out. The written `${a[]}`
	// above is a third sentence again, which is why they are two axes.
	s.EmptySubscriptTextIsAMathError = interp.Yes
	// A subscript's expression stops at the first top-level `,` or `;` and
	// the rest of the text is discarded, unevaluated: measured 2026-09-12,
	// `a=(p q r s); i="2,3"; ${a[$i]}` is `q` where the comma operator would
	// give `r`, and `i="2,n=9"` leaves `n` at 0. The other half of the rule
	// that a comma has to have been *written* to separate a range: what
	// reaches an expression with one still in it arrived through a
	// substitution, and it separates nothing (#2160).
	s.SubscriptExpressionStopsAtASeparator = interp.Yes
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
	s.BadSubscriptToUnset = interp.BadSubscriptReported
	// A *store* through an operand is the opposite, and this is the column
	// the two fields exist for: measured 2026-09-17, `read 'r[1/0]' <<< Y`
	// in a script file writes `division by zero` and the script ends at 1,
	// where the identical expression handed to `unset` one line earlier only
	// left a failed builtin behind.
	s.BadSubscriptToAnOutputOperand = interp.BadSubscriptEndsTheScript
	// And a declaration ends it too, which is the site where zsh and ksh93
	// agree with each other and both part from bash: measured 2026-09-17,
	// `typeset "a[b c]"=v` in a script file writes `bad math expression` and
	// the script ends at 1, with nothing on a later line reached — from the
	// top level, a function, an `&&` list, an `if` condition and a loop body
	// alike (#3495).
	// The construct catches it and answers with its own status: measured
	// 2026-09-17, `(( a[b c] )); echo "same=$?"` prints `same=2` and the
	// line carries on, by both routes. The expansion `$(( a[b c] ))` in the
	// same shell ends the input, so the two constructs really do part here.
	s.BadSubscriptEscapesAnArithmeticCommand = interp.No
	s.BadSubscriptToADeclaration = interp.BadSubscriptEndsTheScript
	// And a plain `a[]=6` ends it too, which is this column's answer
	// wherever a write walks into brackets it cannot use. Measured
	// 2026-09-20 on 5.9.2 from a script file: `a=(1 2 3); a[]=6; echo
	// same-line` writes `not an identifier: a[]` and exits 1 with nothing
	// after it reached, from the top level and from inside a function alike;
	// a `( … )` around it takes the refusal and the outer script lives to
	// read the array back untouched (#3949).
	s.EmptySubscriptToAnAssignment = interp.BadSubscriptEndsTheScript
	// And a valueless subscripted operand is the empty-value form under
	// another spelling, which is measured and not inferred: `a=(1 2 3);
	// typeset 'a[1]'` and `typeset 'a[1]='` both leave `( '' 2 3 )`,
	// `typeset 'a[9]'` grows the array to nine, `typeset 'a[0]'` is
	// `assignment to invalid subscript range`, and `typeset -A m; typeset
	// 'm[b c]'` puts the key in with an empty value. So the brackets are
	// read here, and a subscript that will not evaluate ends the script
	// through the axis above (#3501).
	s.ValuelessSubscriptedOperand = interp.ValuelessSubscriptedOperandWritesTheElement
	// And a name it has never heard of is left alone with its brackets
	// unread: `unset a "a[x+]"` is silent at 0, and `i=0; unset
	// "nodecl[i++]"` leaves i at 0. What counts as heard of is set-ness and
	// not emptiness — `v=` and an empty declared array both evaluate.
	s.UnsetSubscriptSkippedWhenNameUnset = interp.Yes
	// The status is the last *subscript's* and not the last operand's: with
	// an earlier `a[x+]` failing, `unset "a[x+]" "a[1]"` is 0 and `unset
	// "a[x+]" nope` is still 1, because a plain name carries no status.
	s.UnsetStatusIsTheLastSubscripts = interp.Yes
	// A negative subscript past the first element places one in front of it
	// rather than being refused: `a=(p q); a[-3]=x` is three elements with
	// `x` at the head, and `-4` and `-5` land in the same place. What this
	// shell refuses is `a[0]`, which is below its first element rather than
	// counting back from the last.
	s.NegativeSubscriptPastTheStartInserts = interp.Yes
	s.SubscriptBeforeTheFirstElementRead = interp.SubscriptBeforeStartIsNothing
	s.SubscriptBeforeTheFirstElementNeedsAnElement = interp.No
	// And the length is answered exactly as the read is — silently, at `0`.
	// Measured 2026-09-18 (#3591).
	s.SubscriptBeforeTheFirstElementRefusesTheLength = interp.No
	s.OperandSubscriptQuoting = interp.OperandSubscriptQuotesNothing
	s.ArithmeticOnlyBodyIsAnArithmeticExpansion = interp.No
	// `not valid in this context: a+` — the append operator is not a
	// declaration operand here.
	s.DeclarationTakesAnAppendOperand = interp.No
	// Nor with a subscript on the name: `typeset a[1]+=q` is
	// `not an identifier: a[1]+` at 1, which is the same refusal at a
	// different wording rather than a different answer.
	s.DeclarationTakesASubscriptedAppendOperand = interp.No
	// And the same operator on an *array literal* operand, which this shell
	// does not take: `typeset u+=(3 4)` is `not valid in this context: u+`
	// and the script ends.
	s.DeclarationTakesAnAppendingArrayOperand = interp.No
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.No
	// A stopped job keeps the current-job marker: measured 2026-09-12
	// through a pseudo-terminal with a scratch home directory, `sleep 40`
	// stopped with ^Z and then `sleep 41 &` lists `[1]  + suspended` and
	// `[2]  - running`, and `jobs %+` names the suspended one. #1563
	// recorded the opposite for this shell and it does not reproduce —
	// re-measured on zsh 5.9.2 with `-f`, this column agrees with bash.
	s.StoppedJobTakesTheCurrentJobMarker = interp.Yes
	s.JobsListFinishedJobs = interp.No
	s.EndedJobIsListedAsRunningWithoutTheMonitor = interp.No
	// Off by default, and the one column in the panel where a script can
	// turn it on: `setopt LONG_LIST_JOBS` puts the job's pid in every notice
	// this shell writes, and the entry in setopt.go moves this axis rather
	// than remembering the request (#4491). Measured 2026-09-25 on 5.9.2
	// under `-fiV +Z` on a pseudo-terminal: `[1]  + done       :` with the
	// option off and `[1]  + 96801 done       :` with it on.
	s.JobNoticeNamesThePID = interp.No

	// `jobs`' letters: POSIX's pair, the state filters, `-d` — the directory
	// each job was started in, which is this shell's alone — and `-z` and
	// `-Z`, which are about the process title and ride
	// UnimplementedOptionLetters. `-n` and `-x`, which bash has, are bad
	// options here.
	s.JobsOptions = "dlprs"
	// The split this issue was about. zsh reads `-p` as "put the job's
	// process *group* id in the listing" and prints its ordinary rows,
	// where the other three print the ids and nothing else — so
	// `kill $(jobs -p)` is a bash idiom rather than a portable one.
	// unanswered JobsListsWhatChangedSinceTheLastReport: `jobs -n` is not a
	// letter this shell has — it is not in JobsOptions — so the axis is
	// never consulted here. ksh93 is the one column with the letter, and
	// bash's letter of the same name is a different question that stays
	// unimplemented (#3390).
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
	// The hole gets refilled, and here it is a real hole first: measured
	// 2026-09-12, `jobs %2` is 127 after the middle job is killed and reaped,
	// 0 once a fourth job is started, and `%4` is 127.
	s.NextJobNumberRefillsAHole = interp.Yes
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
	s.CdHasSymlinkFreeOption = interp.Yes
	// The hooks. `chpwd` is the one whose site is a builtin rather than the
	// prompt loop — see repl.HookStyle for the two that are not — and the
	// suffix is shared by every hook this shell has, wherever it fires.
	s.HookListSuffix = "_functions"
	s.DirectoryChangeHook = "chpwd"
	// Where this shell keeps the directories it has pushed, newest first and
	// **without the one it is standing in** — `dirs` puts `$PWD` in front of
	// this array rather than reading it out of it. Measured 2026-09-26 on zsh
	// 5.9.2 under `-f`: after one `cd` with `AUTO_PUSHD` on, `dirs -v` numbers
	// two entries and `$#dirstack` is **1**, not the 2 #4592 predicted.
	//
	// A plain array and not a produced view: this shell lets a script assign
	// to it, and the assignment *is* the new stack — `dirstack=(/tmp /usr)`
	// then `popd` arrives in `/tmp`. So the prelude's `pushd`, `popd` and
	// `dirs` keep it under this name and there is one stack rather than two.
	s.PushedDirectoriesParameter = "dirstack"
	// The default state of `AUTO_PUSHD`, which is off — `setopt autopushd` is
	// what moves this axis and nothing else does; see dialect/zsh/setopt.go.
	// Measured 2026-09-26: `unsetopt autopushd; cd sub; print $#dirstack`
	// writes 0, in the reference and here alike.
	s.CdPushesTheDirectoryItLeaves = interp.No
	// The other hook whose site is not the prompt loop: this one fires as the
	// shell ends, which is where a plugin tears down what it started. #2111.
	s.ExitHook = "zshexit"
	s.CdLastPathOptionWins = interp.No
	s.BadSetOptionNameFatal = interp.Yes
	// And the letter too, at zsh's own 1: `set -Z; echo one` prints
	// nothing and exits 1.
	s.BadSetOptionLetterFatal = interp.Yes
	// `set -o posix` is `no such option: posix` here, so the `sh` name is
	// the only door — and this is the shell whose `sh` emulation changes the
	// most, which is why it was asked rather than assumed. Measured 2026-09-14
	// through a symlink named `sh`: both spellings still stop at 1. The mode
	// moves neither, and this preset says so for the same reason it declines
	// the special-builtin move above (#2641).
	s.BadSetOptionNameFatalInPosixMode = interp.Yes
	s.BadSetOptionLetterFatalInPosixMode = interp.Yes
	// The other welding column, and it agrees with ksh93 word for word on
	// behavior: `set -oerrexit zzznosuch` is errexit with `zzznosuch` as $1,
	// and `set -oe` is `no such option: e`.
	s.SetOLetterAttachesItsName = interp.Yes
	// The option loop goes on *applying* the words behind one it refused,
	// which is the other half of the same loop from SetReportsEveryBadOption
	// and splits the panel the other way. Measured 2026-09-18 from a script
	// file under `env -i`: `set -Z -x -o` writes one refusal, then the whole
	// option table with `xtrace on` in it, and `set -e -Z -o` writes the
	// table with `errexit on`. The table is what the applying loop prints, so
	// a shell that stops at the bad word never writes one — bash, dash and
	// BusyBox ash do not, and ksh93 does not either although it is the column
	// that reports every bad word. The refusal is still as fatal as it was;
	// what carries on is the loop and not the script.
	s.SetAppliesTheWordsAfterARefusedOption = interp.Yes
	// Every `setopt` name is an invocation word here too, and reaching it
	// costs nothing beyond saying so: the resolver, the folding and the
	// refusal are this dialect's option table, which `-o NAME` already goes
	// through. Measured 2026-09-16 against zsh 5.9.2 — `--xtrace` traces,
	// `--noxtrace` does not, `--NO_GLOB` and `--no-glob` and `--no_glob` are
	// all `noglob`, `--shwordsplit` really splits, and `--zzznosuch` is
	// `no such option: zzznosuch` at 1 with the word as it was written.
	//
	// ksh93's `no` fallback cannot reach this column and that is what makes
	// one axis enough for both: Runner.hasSetOptionName takes a dialect with
	// an option table of its own at its word, so the substrate never strips a
	// second `no` off a name this table has already folded. It is the
	// difference between `--nonomatch`, which zsh takes (`nomatch` is a
	// canonical name), and `--nonoglob` and `--no_no_glob`, which it refuses
	// — the canonical name is `glob`, so `noglob` is already the negation and
	// there is no second one. All three answer the same here and there.
	s.LongOptionNamesASetOption = interp.Yes
	// The `set` builtin is a different question in this column, which is why
	// it is a different axis. Measured the same day: `set --zzz q` is status
	// 0 with an empty standard error and `q` as `$1`, and `set --xtrace q`
	// traces nothing — the word is swallowed whole rather than applied,
	// refused, or left to be a positional parameter. #3129.
	s.SetLongOptionWord = interp.LongOptionWordIsDiscarded
	// And the `--name` spelling takes hyphens out of the name, which is that
	// spelling's rule here and not this shell's namespace: `--no-glob` is
	// `noglob` and `-o no-glob`, `setopt no-glob` and `set -o no-glob` are
	// all `no such option: no-glob`, in one shell in one run.
	s.LongOptionNameIgnoresHyphens = interp.Yes
	// unanswered OptionNamespaceIgnoresSeparators, OptionNamespaceTakesANoPrefix:
	// this shell installs an option namespace of its own (Runner.SetOptionTable),
	// which answers every spelling before the substrate's roster is reached —
	// including both of these, since `setopt no_glob` and `setopt noglob` are
	// one name there. So the two axes govern a table this column never asks,
	// and there is no site here to put the question to (#3155, #3254).
	// unanswered LongOptionValueIsANumber: measured 2026-09-16, `--xtrace=1`
	// here is `no such option: xtrace=1` at 1 — the whole word including the
	// `=1` is looked up and refused, so this column reads no value at all and
	// the axis has nothing to answer. ksh93's reading is the only one.
	// Where the `-o` does stand alone it takes the next word regardless of
	// how it is spelled, which is the half of the panel ksh93 leaves it on
	// here: `set -o -e` is `no such option: -e` at 1.
	s.SetODeclinesADashWord = interp.No
	s.SetListsOptionsOnceAtTheEnd = interp.No
	// And it applies as it goes, asked with an action rather than a state
	// because its refusal ends a `-c` script whatever stands around it:
	// `set -e -Z -o` writes the whole option table with `errexit on` in it,
	// and `set -Z -e -o` does too — this column does not even stop at the
	// bad word.
	s.SetValidatesOptionLettersFirst = interp.No
	s.BadSetOptionNameAtInvocationExitsZero = interp.No
	s.UnknownConditionOptionIsAStatus = interp.Yes
	s.ReturnOutsideAFunctionIsRefused = interp.No
	// And `break` with no loop around it stops the script here, which is the
	// opposite way round from the line above: measured, `echo t; break` on
	// one line and on two both end at status 1 with nothing after the
	// `break` running at all.
	s.LoopControlOutsideALoopIsFatal = interp.Yes
	// The count is read first: `break abc` outside a loop is `argument is
	// not positive: 0` and not the `not in while, until, select, or repeat
	// loop` this shell writes for a bare `break` in the same place.
	s.LoopControlPlaceIsJudgedBeforeTheCount = interp.No
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
	// `unset -f` is the only spelling that reaches a function here.
	s.UnsetReachesTheFunctionTable = interp.No

	// Whether a redirection target is expanded as an ordinary word.
	s.RedirectTargetIsAnOrdinaryWord = interp.No
	// And it matches a pattern the script *wrote* into the target, which is
	// the half POSIX forbids and this column keeps: `cat < only-*.txt` reads
	// the one file that matches and `printf X > only-*.txt` truncates it.
	// Measured 2026-09-16 (#3207).
	s.RedirectTargetTakesPathnameExpansion = interp.Yes

	// Whether `type --` ends the options.
	// zsh 5.9.2 has one wording for every builtin it has: `whence -v .` is
	// `. is a shell builtin`. Its `export` is a third answer — a reserved
	// word — and belongs to the grammar rather than to this axis.
	// zsh 5.9.2 ends the script from inside a subshell too, at its own
	// syntax status of 1 (#3274).
	s.SubstitutionParseErrorEscapesASubshell = interp.Yes
	// And from a here-document body as well: measured 2026-09-17, `cat <<END`
	// over a body holding `$(echo hi; for)` prints `start` and exits 1 here,
	// where bash 5.3.20 and ksh93u+ carry the script on (#3318).
	s.SubstitutionParseFailureInAHeredocBodyEndsTheShell = interp.Yes
	// Never reached: this column catches the failure at a `.` and at an
	// `eval` — Semantics.FatalErrorEndsBorrowedTextOnly — and it has no
	// here-document row to carry on from. Answered anyway, because an
	// unanswered axis is a refusal in front of a reader (#3319).
	s.SubstitutionParseFailureCarriesTheFatalStatus = interp.No
	s.TypeDistinguishesSpecialBuiltins = interp.No
	s.TypePrintsFunctionBody = interp.No
	s.TypeEndsOptionsWithDashDash = interp.Yes
	// whence -v under another name, so the letters answer in sentences:
	// -p searches PATH past the shell's own answer and words the hit the
	// way plain type does, and -f *prints* a function rather than skipping
	// it. The completion-system letters ride as unimplemented.
	// `w` names the kind — `ls: command` — which is how a syntax
	// highlighter classifies every word on the line. See interp's typeKind,
	// where its six words are measured against the other dialect's `-t`
	// (#2512). `-t` itself is a bad option here and is absent from the
	// letters, which is the whole of how that is said since #2180.
	s.TypeOptions = "afpw"
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
	s.DeclareOptions = "aAEfFgHhiLlmpRrtuUTxZz"
	// And what the `m` of that set *means*: the operands are patterns and
	// every other letter on the line decides what a match is then used
	// for. ksh93 spells the same letter and moves a parameter with it, so
	// this is a disagreement about identical syntax rather than a letter
	// one dialect has — see interp.DeclareMatchingLetterPolicy.
	s.DeclareMatchingLetter = interp.DeclareMatchingLetterSelects
	// And the `M` of `functions`: it registers a shell function as a math
	// function here, where ksh93's `M` names a character mapping. The same
	// shape as the `m` letter above and for the same reason.
	s.DeclareMappingLetter = interp.DeclareMappingLetterRegistersAMathFunction
	// And the `T` of `typeset`: it ties a scalar to an array here — see
	// interp/tiedscalar.go — where ksh93's names a type. The third letter of
	// that shape and the third for the same reason.
	s.DeclareTypeLetter = interp.DeclareTypeLetterTiesAScalarAndAnArray
	// And the `H`, which is the fourth: it withholds a name's *value* from a
	// listing here — `typeset -H h=hid` lists back as `typeset h` — where
	// ksh93 records an inert attribute and writes the letter and the value
	// alike. Measured 2026-09-13; interp/declarehide.go holds it.
	s.DeclareHideValueLetter = interp.DeclareHideValueLetterHidesTheValue
	// And the lower-case letter hides a special name's *specialness* from a
	// local declaration, taking no argument — where ksh93 reads a string
	// after it. See interp/hideinscope.go.
	s.DeclareHideInScopeLetter = interp.DeclareHideInScopeLetterHidesInScope
	// `export` is this word's declaration under another name, and it takes
	// the same letters bar six. Measured 2026-09-12, a letter at a time
	// against `export -X q=4`: `-A`, `-g`, `-m`, `-x` and `-z` are
	// `bad option` and `-f` is `invalid option(s)`, and every other letter
	// above is read. See Semantics.ExportOptions for why the refused six are
	// the six they are.
	s.ExportOptions = "aFHhiLlpRrtuUTZ"
	// `-z` and `+z` are taken and do nothing here, which is measured on both
	// signs and on both sides of the builtin: `typeset -z p q` leaves `p`
	// alone and declares an empty `q` exactly as a bare `typeset q` would,
	// `typeset +z p` is 0 with `p` unchanged, and `typeset -fz f` over a
	// defined function is 0 with the function still defined and still
	// listing as itself.
	//
	// The one spelling where the letter is not inert is `typeset -fu`, which
	// is that shell's second name for `autoload`: there `-z` picks the
	// autoloaded file's syntax and is recorded on the stub, `typeset -fuz n`
	// listing as `builtin autoload -Xz` exactly as `autoload -z n` does.
	// That spelling is built now (#1753), and the letter reaches the stub
	// through declareFlags.letters rather than through this table — being
	// listed here means "no attribute", not "unread", which is why the two
	// can both be true of one letter. `autoload -z` itself has always been
	// unaffected: that word has its own table and keeps the letter.
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
	// And `export` and `readonly` read it too, which is the half ksh93 does
	// not have — see Semantics.SignAloneIsAnOptionWordToExport. Measured
	// 2026-09-12: `export +` writes the exported names and `readonly +` the
	// frozen ones, both bare and both at 0, where this engine answered
	// `not valid in this context: +` at 1 (#1756).
	s.SignAloneIsAnOptionWordToExport = interp.Yes
	// `typeset +f` is the function *names*, one bare name a line and no
	// quoting — the shape a shell snapshot reads to find what to capture.
	// See Semantics.FunctionNamesUnderPlus for the two other answers.
	s.FunctionNamesUnderPlus = interp.Yes
	// `typeset -f` written with `-u` or `-U` and names is not a listing: it
	// is `autoload` under a second word. `z` rides along and is recorded on
	// the stub without being one of these — `typeset -fz nm` marks nobody.
	// See dialect/zsh's autoloadFromDeclaration for the measurements (#1753).
	s.FunctionLettersThatMarkUndefined = "uU"
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
	// And the three width letters take one the same way: `-L`, `-R` and `-Z`
	// each name how many characters the value is presented in, under the
	// detached spelling and the attached one alike — `typeset -L 5 a=ab` and
	// `typeset -L5 a=ab` are both `[ab   ]`. See interp/fieldwidth.go for
	// the rule and for why ksh93 does not have them here (#1461).
	// `-E` joins the four here rather than standing apart: it takes a number
	// in exactly the shape `-F` does, attached or detached, and what it does
	// with the number is FloatFormatLetterE's question and not this one.
	s.DeclareOptionsTakingANumber = "EFLRZ"
	// And the letter is a **format**: `%.*e` with n−1 places, so `typeset -E
	// 3 a=3.14159` is `3.14e+00` where ksh93's same line is `3.14`. See
	// interp/floatformat.go for the measurements (#2559).
	s.FloatFormatLetterE = interp.FloatFormatExponentWithPlaces
	// The two numeric type letters may stand together here and the **first
	// written** wins, which the parse settles: measured 2026-09-15,
	// `typeset -iE 3 a=1.5` lists as `typeset -i3 a=1` and `typeset -Ei 3
	// a=1.5` as `typeset -E a=1.50e+00`. ksh93 refuses the pair outright.
	s.NumericTypeLettersAreExclusive = interp.No
	// And the letter read *first* wins, which is the order the parse falls
	// into on its own: `typeset -iF 3 a=1.5` is `typeset -i3 a=1` and
	// `-Fi 3` is `typeset -F a=1.500`.
	s.NumericTypeLetterPrecedence = interp.NumericLetterFirstWrittenWins
	// `Z` is a **justification of its own** here and the third member of an
	// exclusive set with `L` and `R`, where ksh93 reads it as a fill riding
	// on one of the other two. Measured 2026-09-18 on zsh 5.9.2: `typeset
	// -ZL 5 q=7` lists as `typeset -Z5 q=7` and is `00007`, and `typeset -LZ
	// 5 r=7` as `typeset -L5 r=7` and is `7    ` — one letter written back
	// for a pair, and the value the surviving letter's. See
	// interp/fieldwidth.go for ksh93's pair of letters (#2859).
	s.DeclareZeroFillLetter = interp.DeclareZeroFillLetterIsAJustificationOfItsOwn
	// And which of them survives is the numeric letters' rule again, with
	// the same answer: the **first written** wins. Measured in the same run
	// — `typeset -LR 5 t=7` lists as `typeset -L5 t=7`, `-RL` as `typeset
	// -R5`, `-ZRL` as `typeset -Z5` and `-LRZ` as `typeset -L5`. ksh93 takes
	// the last.
	s.WidthJustificationPrecedence = interp.WidthJustificationFirstWrittenWins
	// And a width letter may stand beside the integer one, the first written
	// winning there too: measured 2026-09-18, `typeset -iL 5 a=7` lists as
	// `typeset -i5 a=7` — the `5` a base — and `typeset -Li 5 a=7` as
	// `typeset -L5 a=7`. ksh93 answers both with typeset's usage block and
	// ends the script.
	s.WidthLettersExcludeTheIntegerLetter = interp.No
	// A bare `-F` or `-E` over a name that already has a precision keeps it
	// here — measured, `typeset -F 3 x=1.5; typeset -F x` still reads
	// `1.500` — where ksh93 resets to the letter's default.
	s.BareFloatLetterResetsThePrecision = interp.No
	// And a detached number reaches its letter wherever the letter stands:
	// `typeset -El 3 a=1.5` is `1.50e+00` here and `3: invalid variable
	// name` in ksh93, which is where the two part.
	s.DeclareNumberDetachedOnlyAtTheWordEnd = interp.No
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
	// `-u` and `-U` are the marking letters, and under this word they are
	// what they are under `typeset -f`: with operands a declaration, with
	// none the listing a bare `autoload` writes. Measured 2026-09-12 on zsh
	// 5.9.2 with `f1` autoloaded plainly and `f2` with `-U`, `functions -u`
	// is byte for byte what `autoload` with no operands writes, and
	// `functions -U` is that listing narrowed to `f2` (#1996). See
	// Semantics.FunctionLettersThatMarkUndefined, which already named them
	// for the declaration word, and Diagnostics.MarkingLettersUnderPlus for
	// the half of `-u` this shell refuses.
	s.FunctionsOptions = "mMuU"
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
	s.LocalOptions = "aAFHhiLlpRrtuUTxZ"
	// A bad `typeset` option is reported and the script goes on.
	s.TypesetBadOptionFatal = interp.No
	// `integer` here is `typeset` with the letter prepended rather than a
	// declaration of its own, and its letter set is *narrower* than
	// typeset's: measured 2026-09-06, `integer -a`, `-A`, `-f`, `-F`, `-T`
	// and `-U` are bad options where this shell's typeset takes all six. So
	// the set is its own field and not DeclareOptions over again — a shell
	// that reused them would accept `integer -A m`, which is an associative
	// array in no shell that has the word.
	s.IntegerOptions = "gHhilprtux"
	// And `float` is the same word one letter along, with the same set
	// narrowed the same way: measured 2026-09-16 under `-f`, a letter at a
	// time against `float -X zz=1.5`, this shell's `float` refuses `-a`,
	// `-A`, `-G`, `-i`, `-m`, `-T`, `-U` and `-z` and takes the rest. So it
	// is `integer`'s letters with the integer one traded for the two float
	// ones, which is what the two words are.
	s.FloatOptions = "EFgHhlprtux"
	// `-i16` and `-i 16` are an output base here — `integer -i 16 b=255` is
	// `16#FF` — and this engine has no base to keep, so it refuses by name.
	s.IntegerAttributeTakesABase = interp.Yes
	// The alphabet this shell counts an output base in, and its length is
	// the largest base it can spell — see Semantics.IntegerBaseDigits.
	s.IntegerBaseDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	s.IntegerBaseComesFromTheValueAssigned = interp.Yes
	s.IntegerBaseNegativeIsTwosComplement = interp.No
	// `16#FF` by default, and `0xFF` under `setopt C_BASES` — the one
	// option in the panel that moves this, so the entry in setopt.go writes
	// the axis rather than remembering the request (#4502). Off here
	// because `C_BASES` is off in a shell that has not set it.
	s.IntegerBaseMarkIsCSpelled = interp.No
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
	// `-` is a parameter here — the option letters in effect — so `local -`
	// is a declaration of it and not the save the other three read it as.
	// Measured 2026-09-15: the options a body sets are still set after it
	// returns.
	s.LocalDashSavesTheShellOptions = interp.No
	s.BareLocalListing = interp.BareLocalListsEveryParameter
	// And `local -p name` lists whatever the name reaches, marking a global
	// as one: measured 2026-09-21, `string=global; f() { local -p string; }`
	// writes `typeset -g string=global` at 0 here where bash refuses it.
	s.LocalListingIsTheRunningCallsOwn = interp.No
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
	// The ends are numbered where any other allocation goes rather than at
	// the top of the table, which a shell publishing no array still shows in
	// what is left for it to hand out. Measured 2026-09-13: `exec
	// {a}>/dev/null {b}>/dev/null {c}>/dev/null` answers `11 12 13` with no
	// coprocess running and `12 13 15` with one (#2596).
	s.CoprocessEndPlacement = interp.CoprocEndsWhereAnyDescriptorGoes
	// And a process substitution's far end goes there too, which is the
	// reading that makes 11 the base rather than a number of its own:
	// measured 2026-09-21, `echo <(true) <(true) <(true)` is `/dev/fd/11
	// /dev/fd/12 /dev/fd/13`, and an `exec {a}</dev/null` first moves the
	// substitution to 12.
	s.SubstitutionEndPlacement = interp.SubstitutionEndsWhereAnyDescriptorGoes
	// And nothing is taken back when the coprocess ends: both ends stay open
	// and stay reachable by their letters. Measured 2026-09-12 — a `print -p`
	// after the coprocess has gone writes into a pipe with no reader and the
	// shell dies on SIGPIPE at 141, where the other two shells with a
	// coprocess refuse the write instead. The death itself is #770's, not
	// this axis's: a builtin's broken-pipe write is answered here with the
	// status rather than the signal, so this column reports 1 where zsh
	// reports 141 until that lands (#2411).
	s.ReapedCoprocessEnds = interp.CoprocEndsSurviveTheCoprocess
	// `p` after `>&` or `<&` is the coprocess here too, which is the same
	// facility the `-p` letters are: `coproc cat; exec 3>&p; print -u3 x`
	// reaches the coprocess through a 3. It **lends** the end rather than
	// handing it over — measured 2026-09-16, `print -p` after that `exec`
	// still writes to the coprocess, where ksh93's letter finds none. The
	// refusal with none running names the facility rather than the word,
	// `coprocess: bad file descriptor`, which is what the diagnostic below
	// carries (#2345).
	s.CoprocessNamedByARedirection = interp.CoprocessRedirectionDuplicatesTheEnd
	s.SetListingQuoting = interp.ListingQuoteWhenNeededRuns

	// A descriptor number the process cannot hold is not checked here: with
	// `ulimit -n 6`, `exec 8>f` reports success and prints nothing, where
	// bash and ksh93 hand the kernel's refusal back. Rarely reachable, since
	// a number this shell reads is one digit and the shell picks its own for
	// `{name}>f`.
	s.FdNumberBoundedByOpenFileLimit = interp.No
	// No descriptor-number ceiling of this shell's own (#3210), and `read
	// -t` takes no argument here at all, so the word after it is an operand
	// and never an expression (#3209). The `-v` echo of a file's last line
	// writes no newline the input did not have (#3130) — measured on a file
	// with no final newline; the `-c` route is a different mechanism there
	// and is named in the axis.
	s.DescriptorNumberCeiling = interp.NoDescriptorNumberCeiling
	s.VerboseEchoAddsAMissingNewline = interp.No
	s.ReadTimeoutOperandIsArithmetic = interp.No

	// `[[ -v ]]` reads more kinds of name here than either of the other two
	// shells that have the operator: a positional, as bash does, and every
	// parameter spelled as one punctuation character — `?`, `#`, `$`, `!`,
	// `*` and `-` — which is this shell's alone. `@` is unset here as it is
	// everywhere, so it is excluded outright rather than by an axis.
	// Measured on zsh 5.9.2.
	s.ParameterIsSetSeesPositionals = true
	s.ParameterIsSetSeesSpecials = true

	// A subscript the condition receives as *text* is expanded again before
	// it is looked up, and this shell writes no bracket of its own into that
	// text: it has no written-subscript reading, so `k='x y'; kk='$k'` makes
	// `[[ -v m[$kk] ]]` set here where bash stops at the key `$k`. Measured
	// 2026-09-19 on zsh 5.9.2, and the same answer for the quoted operand
	// and for one that arrives out of a value. See
	// interp.Semantics.ConditionIsSetExpandsAFlatSubscript (#3298).
	s.ConditionIsSetExpandsAFlatSubscript = interp.Yes

	// And a declaration's operand expands its subscript: `typeset 'd[$k]'=Q`
	// writes the key `x y`, and `typeset 'arr[$i]'=Q` the element `$i`
	// counts to. See
	// interp.Semantics.DeclarationOperandExpandsItsSubscript (#3298).
	s.DeclarationOperandExpandsItsSubscript = interp.Yes

	// The store through a builtin's name operand rounds too, and `test -v`
	// with it. `unset` is where this shell parts from the one that has a
	// name for the whole group, and it is the only surface in it where the
	// two disagree — which is why the three are three fields rather than
	// one. Measured 2026-09-19 on zsh 5.9.2, from a script file with
	// standard input on /dev/null, over `k='x y'` and a table holding one
	// element under the key `x y`:
	//
	//	unset -v 'a[$k]'      the element **survives**
	//	unset 'a[$k]'         the element **survives**
	//	printf -v 'c[$k]' P   the key `x y`
	//	read 'b[$k]' <<<Z     the key `x y`
	//	test -v 'g[$k]'       true
	//	[ -v 'g[$k]' ]        true
	//
	// The control for the first two rows is the same operand with nothing in
	// it to expand: `unset 'a[plain]'` takes the element away here, so the
	// survival above is the round not happening rather than the quoted
	// brackets not reaching `unset`. See
	// interp.Semantics.UnsetExpandsAFlatSubscript (#3298).
	s.UnsetExpandsAFlatSubscript = interp.No
	s.OutputOperandExpandsAFlatSubscript = interp.Yes
	s.TestIsSetExpandsAFlatSubscript = interp.Yes
	return s
}

// Diagnostics is how zsh reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		// A table literal mixing `[key]=` heads with bare words, refused
		// whichever way round it was written and naming neither the variable nor
		// the element: measured 2026-09-23, `typeset -A a; a=([zero]=5 four)` is
		// `bad [key]=value syntax for associative array` and the script stops.
		// See interp.Semantics.MixedTableLiteral (#4241).
		MixedTableLiteralRefusal: "bad [key]=value syntax for associative array",
		// A scalar store over a name holding a table, in the option state
		// that refuses one. Measured 2026-09-26 under `setopt ksharrays`,
		// `typeset -A h=(one 1); h=string` is `h: attempt to set associative
		// array to scalar` on standard error and the shell leaves at 1; the
		// `h+=string` spelling earns the same sentence, and from inside a
		// function the function's name goes in front of it where the file and
		// line otherwise do. The name is the one verb.
		// See interp.Semantics.ScalarStoredOverATableIsRefused (#4617).
		ScalarStoredOverATable: "%s: attempt to set associative array to scalar",
		// A table literal whose bare elements came to an odd number of
		// fields, which this shell will not pair off. Measured 2026-09-26,
		// `typeset -A h=(a 1 b)` is `bad set of key/value pairs for
		// associative array` on standard error and the shell leaves at 1; from
		// inside a function the function's name is what goes in front of it.
		// See interp.Semantics.BareElementsInATableLiteralMustPairOff (#4594).
		UnpairedTableLiteralElements: "bad set of key/value pairs for associative array",
		// The four loops this shell has, and the builtin's name is stripped
		// back out of the front of it because this dialect puts it in the
		// location: `zsh:break:1: not in while, …`.
		LoopControlOutsideALoop: "%[1]s: not in while, until, select, or repeat loop",
		// The count is judged by its *sign* rather than by whether it could
		// be read, and the sentence quotes the number it read and not the
		// word the script wrote: `break abc` is `argument is not positive:
		// 0` and `break -1` is `argument is not positive: -1`. Measured
		// 2026-09-14, and the script ends in both (#2800).
		LoopControlCount:               "%[1]s: argument is not positive: %[2]s",
		LoopControlCountNamesTheNumber: true,
		LoopControlCountStatus:         1,
		// No name in the sentence: zsh's location already carries it —
		// `./s.sh:break:1: too many arguments`.
		NumericOperandTooMany: "too many arguments",
		// zsh names itself, not the path it was invoked by. `/bin/zsh` and a
		// symlink called `myzsh` both say `zsh:`, and so does the shell run
		// as `exec -a weirdname /bin/zsh` — measured all three ways, because
		// the first alone looks like a base name rather than a fixed one.
		// The other three shells print argv[0] whole.
		SelfName: "zsh",
		// With one exception, and it goes the other way from ksh93's: an
		// option refused at an invocation names the **whole word** the shell
		// was started by, where SelfName above would have written `zsh`.
		// Measured 2026-09-16 invoking it as `/opt/homebrew/bin/zsh` —
		// `-o zzznosuch` and `--zzznosuch` both write the path, and an
		// unset parameter under `-c` writes `zsh:1:`. The route is the
		// split, not the sentence.
		InvocationOptionRefusalNamesTheInvocation: true,
		// Read part of a numeral too large for the machine word, and say so —
		// the only column that says anything at all. The count is of the digits
		// it got through and the second verb is the digit run as written, with
		// any radix prefix off it: `$(( 0xffffffffffffffff ))` is `number
		// truncated after 15 digits: ffffffffffffffff`. Not an error: stderr,
		// status 0, and the script carries on (#3202).
		ArithNumberTruncated: "number truncated after %[1]d digits: %[2]s",
		TypeKeyword:          "%[1]s is a reserved word",
		// The only one that names where the function came from. The second
		// verb is the file it was defined in, or the shell's own name where
		// the shell itself defined it — see Diagnostics.TypeFunctionFrom.
		// It was the fixed string `… from zsh` until #1706, which is the
		// answer for one of the three cases given to all of them.
		TypeFunctionFrom: "%[1]s is a shell function from %[2]s",
		// And the clause-less form, for the one route with no origin: a
		// program on standard input.
		TypeFunction: "%[1]s is a shell function",
		// The three kinds, each with its own sentence. A suffix alias names
		// the *extension*: `whence -v p.txt` is `txt is a suffix alias for
		// cat`.
		TypeAlias:       "%[1]s is an alias for %[2]s",
		TypeGlobalAlias: "%[1]s is a global alias for %[2]s",
		TypeSuffixAlias: "%[1]s is a suffix alias for %[2]s",
		CommandVAlias:   "alias %[1]s=%[2]s",
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
		// lower case in a column nine wide with two spaces after it. zsh
		// never lists a finished job, so it needs no word for one.
		//
		// Nine-and-two rather than a flat eleven, which is the same for
		// every word that fits and is not the same for one that does not.
		// Measured 2026-09-25 on 5.9.2: `hangup` is followed by five
		// spaces and `interrupt` by two — eleven either way — while
		// `terminated` is followed by **two**, for twelve, and
		// `broken pipe` and `segmentation fault` by two as well. A flat
		// eleven would have written `terminated ` and run
		// `segmentation fault` straight into the command. Nothing moves for
		// the words this shell wrote before the signal ones arrived:
		// `done`, `running`, `suspended`, `continued` and `exit N` are all
		// nine or fewer (#4508).
		JobLine: "[%[1]d]  %[2]s %-9[3]s  %[4]s",
		// `jobs -l`, and `jobs -p` too: the process id after the marker,
		// with the same state column after it.
		JobLineLong: "[%[1]d]  %[2]s %[3]d %-9[4]s  %[5]s",
		// `jobs -d`, under the row rather than in it. Spaces either side of
		// the colon, and the directory with the home directory written `~`.
		// Measured 2026-09-25 on 5.9.2: `(pwd : /tmp/jobsdir/a)` from `/tmp`
		// and `(pwd : ~/x)` from the home directory (#4507).
		JobDirectoryLine: "(pwd : %[1]s)",
		// The builtin's name is in the location here rather than in the
		// sentence, which is this shell's rule for every message.
		// About its table rather than about the function, and the builtin
		// is named in the location as it is for every message here.
		UnsetFunctionNotFound: "no such hash table element: %[1]s",
		// The array alone, and a sentence about the assignment rather than
		// about the subscript.
		BadArraySubscript:         "%[1]s: assignment to invalid subscript range",
		ArithEmptySubscript:       "invalid subscript",
		ArithEmptySubscriptTarget: "not an identifier: %[1]s[]",
		// A plain `a[]=6` gets that same sentence, which is measured and
		// not inferred: 5.9.2 answers the assignment and the arithmetic
		// write with one line, where bash words the two apart (#3949).
		AssignEmptySubscript: "not an identifier: %[1]s[]",
		// One sentence either way, which is the measurement rather than a
		// copy: 2026-09-17, a script file, `typeset 'a[]'=v` and `typeset
		// 'a[]'` both write `./f.sh:2: not an identifier: a[]` and the script
		// ends at 1 — the same words the write of an empty subscript gets in
		// an expression, and no builtin in the location, where zsh's own
		// bad-name refusal writes one.
		DeclarationEmptySubscript:          "not an identifier: %[1]s[]",
		ValuelessDeclarationEmptySubscript: "not an identifier: %[1]s[]",
		// The same sentence at the parameter site, and its own field because
		// the two coincide here and do not in bash — see the field.
		EmptyParamSubscript: "invalid subscript",
		// And the same sentence for a `[` the word never closed, which is a
		// different construct wearing this shell's one complaint. See
		// interp.Diagnostics.BareSubscriptUnclosed.
		BareSubscriptUnclosed: "invalid subscript",
		// The subscript machinery's other complaint with the same words and
		// a different construct: `${(k)x[1,2]}` and `${a[(i)q,2]}` are each
		// a reading that needs one index over a subscript that names a span.
		SubscriptIsAnIndexAndARange: "invalid subscript",
		// And the sentence for a subscript that expanded to nothing, which
		// is the arithmetic reader's rather than the subscript machinery's:
		// measured, `zsh:1: bad math expression: empty string`, where the
		// written `${a[]}` on the line above is `invalid subscript`.
		EmptySubscriptTextExpanded: "bad math expression: empty string",
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
		// The one shell that answers 1 for both, which is what separates it
		// from the two that answer 1 for one spelling and 2 for the other.
		SetInvalidOptionNameStatus:   1,
		SetInvalidOptionLetterStatus: 1,
		// A denied `set -m` echoes the spelling it was asked with — `-m` or
		// `monitor` — and fails at 1, fatally like every `set` failure here.
		MonitorDenied:       "can't change option: %[1]s",
		MonitorDeniedStatus: 1,
		// `[[ -o nosuchoption ]]`. The same words `setopt` uses for the same
		// mistake, and measured to be: one message, said at the condition's
		// own location rather than a builtin's. The status is 3, which is
		// neither of the two a condition otherwise gives.
		UnknownConditionOption: "no such option: %[1]s",
		// A known conditional operator with the wrong number of operands,
		// which this shell's grammar accepts and this refuses when it runs.
		// The operator is named and the surplus word is not — `[[ -n ]]`,
		// `[[ -n x y ]]` and `[[ -n x -z "" ]]` are one sentence (#965).
		UnknownCondition: "unknown condition: %[1]s",
		// And what a condition this shell could not *read* says, which is a
		// pair of sentences rather than one: the word named moves with how
		// many words the group holds, and the second word chooses between
		// the two lead-ins. Measured 2026-09-15 — `[[ p q r ]]` is
		// `condition expected: q` and `[[ p q ]]` is `parse error:
		// condition expected: p`. See Diagnostics.ConditionExpected (#2846).
		ConditionExpected:            "condition expected: %[1]s",
		ConditionExpectedPrefixed:    "parse error: condition expected: %[1]s",
		UnknownConditionOptionStatus: 3,
		// `[[ -prefix … ]]` and `[[ -suffix … ]]` reached outside a
		// completion function. No verbs: the sentence names neither the
		// operator nor the operand, measured over all three operand shapes.
		CompletionConditionOutsideCompletion: "condition can only be used in completion function",
		// zsh knows `-f` — it means functions to its own typeset — so what
		// it refuses is the combination, and it says so without naming the
		// letter it names in every other refusal.
		ExportFunctionOptionRefused: "invalid option(s)",
		JobRunning:                  "running",
		SignalDescriptions:          signalDescriptions(),
		// Only ever seen in a completion notice: zsh's listing never
		// mentions a job that has ended.
		JobDone: "done",
		// A job a signal ended is named for the signal rather than called
		// done, in this shell's own words for it — see signalDescriptions,
		// which is why the number the second verb offers is not taken:
		// `[1]  + terminated  sleep 30`, where the host's C library would
		// have written `Terminated: 15` (#4508).
		JobSignaled: "%[1]s",
		JobStopped:  "suspended",
		// ^Z is a sentence rather than a listing row here, and the shell
		// names itself in it: `zsh: suspended  sleep 40`, two spaces, no job
		// number. Under a newline of its own, as bash's is.
		JobStoppedNotice:           "%[3]s: suspended  %[4]s",
		JobStoppedNoticeOnANewLine: true,
		// `fg` and `bg` both print a listing row, and the state in it is
		// JobLine's 11-wide column with the resume's own word in it. Not
		// always `continued`: measured 2026-09-15 on a pseudo-terminal,
		// `fg` on a job that is *running* prints `[1]  + running    sleep 1`
		// — the row `jobs` prints — and only a job the shell had to send a
		// continue to is called continued. So the state is a verb here and
		// `continued` is JobContinued, beside JobRunning and JobStopped
		// where a state word belongs (#2838).
		JobResumedInForeground: "[%[1]d]  %[2]s %-9[4]s  %[3]s",
		// `bg` never sees the other state: a job that is already running is
		// refused below rather than resumed, so the only row this prints is
		// the continued one. Spelled with the literal word for that reason —
		// the format that takes the verb is the one that has two answers.
		JobResumedInBackground: "[%[1]d]  %[2]s continued  %[3]s",
		JobContinued:           "continued",
		// `bg` on a job that is not stopped is refused, and the sentence
		// names neither the builtin nor the job — this shell's location
		// prefix already carries the builtin, as `bg: %1: no such job` does.
		// Status 1, where bash complains about the same thing and still
		// reports success. Measured 2026-09-15 from a script with `set -m`
		// on a pseudo-terminal and again at an interactive prompt (#2838).
		JobAlreadyInBackground:       "job already in background",
		JobAlreadyInBackgroundStatus: 1,
		// The shell names itself here too, and the held `exit` reports
		// nothing — measured, `echo $?` after the refusal says 0, where
		// bash's says 1.
		StoppedJobsAtExit: "%[1]s: you have suspended jobs.",
		RunningJobsAtExit: "%[1]s: you have running jobs.",
		// And what it says once it goes anyway, having sent the signal
		// `setopt hup` asks for. The shell names itself here too; `jobs`
		// however many, including one, which is the reference's own wording
		// and not a plural this rendering chose. Measured 2026-09-25 through
		// a pseudo-terminal: `setopt no_check_jobs`, `sleep 3 &`, `exit`
		// writes `zsh: warning: 1 jobs SIGHUPed` on the error stream and the
		// job is gone. See interp.Runner.SendsHangupToJobsAtExit.
		JobsHUPedAtExit: "%[1]s: warning: %[2]d jobs SIGHUPed",
		// The reason first and the name after it, which is zsh's shape and
		// nobody else's. Lowercased, which LowercaseReason already says.
		// Same either way — zsh does not distinguish opening from creating.
		CannotOpen: "%[2]s: %[1]s",
		// zsh names nobody: `cat <&qq` and `cat <&""` are both `file number
		// expected`, so the wording uses neither verb and the empty case has
		// nothing of its own to say. The writing side never reaches this —
		// GreatAmpTarget sends it to the file.
		DuplicationTargetIsNotADescriptor: "file number expected",
		// A refused `>&p` names the facility rather than the word:
		// `coprocess: bad file descriptor`, measured 2026-09-16 with no
		// coprocess running. ksh93 quotes the `p` the script wrote.
		CoprocessDuplicationTargetName: "coprocess",
		CannotCreate:                   "%[2]s: %[1]s",
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
		ScriptNotFound: "can't open input file: %[1]s",
		// `zsh -c` with nothing behind it names the option last, which is
		// why this is a verb rather than a fixed sentence. Measured
		// 2026-09-22 on zsh 5.9.2.
		InvocationMissingOptionArgument: "string expected after %[1]s",
		ScriptNotFoundStatus:            127,
		ScriptNotReadableStatus:         127,
		Location:                        interp.LocationTightLine,
		// And no line at a prompt, which is the same answer this shell gives
		// a program on standard input: measured 2026-09-11 under `-i`,
		// `if; then` then end of input is `zsh: parse error near `\n'` where
		// a script file is `s.sh:3: parse error near `\n'`.
		PromptLocation: interp.LocationNameOnly,
		// And a builtin's complaint at a prompt is the *builtin's* name
		// alone — no shell, no line — which is the same answer this shell
		// gives a program on standard input and a different one from the
		// shell's own messages above: measured 2026-09-11 under `-i`,
		// `cd /nope` is `cd: no such file or directory: /nope` where
		// `nosuchcmd` is `zsh: command not found: nosuchcmd` (#2024).
		PromptBuiltinLocation: interp.LocationBuiltinNameOnly,
		BadSubstitution:       "bad substitution",
		// The position is 1-based and counts from the `$`: `${(Y)x}` errors
		// at 4, and a group that runs out of text errors just past the end.
		ExpansionFlagsError: "error in flags near position %[1]d in '%[2]s'",
		BadPattern:          "bad pattern: %s",
		// No verbs: this shell names neither the escape nor the value it
		// refused. Measured 2026-09-11 under `LC_ALL=C`, the whole line is
		// `zsh:1: character not in range` (#1851).
		CodePointOutsideTheLocale: "character not in range",
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
		// The container letter's is the sentence a whole-name kind change
		// gets rather than one of its own, which is what makes it worth
		// having beside the three above: only the wording tells a script
		// which refusal it ran into.
		ContainerElementRefusal: "%[1]s[%[2]s]: inconsistent type for assignment",
		// `set -A 1v q` does not name the builtin in its location where
		// `unset 1x` and `typeset 1w` from this same shell do.
		// `read` joins `set` in it: `zsh:1: not an identifier: 1bad` has no
		// `read:` in the location where `zsh:export:1:` has the builtin.
		// `printf` joins the two: measured 2026-09-17, its refusal is
		// `./f.sh:1: not an identifier: 1x` with no builtin in the location,
		// where `typeset 1x` is `./f.sh:typeset:1: …`.
		BadNameRefusalHidesTheBuiltin: map[string]bool{
			"set": true, "read": true, "printf": true,
			// And `getopts`, measured 2026-09-18: `getopts x 1bad -x` is
			// `<file>:N: not an identifier: 1bad` with no `getopts` in the
			// location (#3555).
			"getopts": true,
		},
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
		// History expansion's three complaints, and zsh words all three
		// unlike anybody else — measured 2026-09-15 through a
		// pseudo-terminal. The reference goes at the *end* and loses the `!`
		// that introduced it, and a failed substitution names nothing at all.
		HistoryEventNotFound:          "event not found: %[2]s",
		HistorySubstitutionFailed:     "substitution failed",
		HistoryBadWordSpecifier:       "no such word in event",
		HistoryNoPreviousSubstitution: "no previous substitution",
		// The trace attribute is a **variable** letter this engine records
		// and lists (#3101) and a **function** mark it does not. This shell
		// writes the mark inside the body — `f () {` then a `# traced`
		// comment line then the statements — where the one other shell with
		// the letter writes a `declare -ft f` row after the body, so the two
		// are not one rendering and this column's half is not built. Named
		// as missing on a `-f` line alone, which is the narrowest true
		// statement: `typeset -t v=1` is a declaration here and only
		// `typeset -ft f` and the bare `typeset -ft` are refused.
		UnimplementedOptionLettersOnAFunctionLine: map[string]string{
			"typeset": "t",
			"declare": "t",
			"local":   "t",
		},
		UnimplementedOptionLetters: map[string]string{
			// **`set` has left this table entirely**, and the emptiness is
			// the measurement. zsh gives a single letter to far more of its
			// options than the rest of the panel does — measured 2026-09-05,
			// it refuses only b, c, j, q and z of the fifty-two — and
			// thirty-three of the ones it has were listed here as letters
			// this shell had not built, while `set -o <name>` already took
			// and moved every one of the options behind them. The letters
			// were the only thing missing, so they are a table of names now
			// rather than a table of refusals: see setLetterOptions in
			// setopt.go, which #2578 measured a letter at a time.
			//
			// The three reasons a letter is in neither table are worth
			// keeping, because each is a different sentence:
			//
			//   - `-A` assigns an array and is implemented, in
			//     Semantics.SetArrayLetter.
			//   - `-t`, `-i` and `-Z` are options this shell will not
			//     *move*, which zsh words differently from a letter nobody
			//     has: `can't change option: -i`. `-t` reaches that through
			//     ImmovableOptionLetters above and the other two through
			//     setLetterOptions, since the names they abbreviate already
			//     refuse the same way.
			//   - `-b`, `-c`, `-j`, `-q` and `-z` are letters zsh itself has
			//     not got, so the substrate's own bad-letter refusal is
			//     already zsh's answer.
			// zsh's hash past `-r`: `-d` is the *named directory* table
			// rather than a forgetting, `-f` hashes every command on PATH
			// at once, `-m` reads the operands as patterns, `-v` reports
			// each entry as it is made and `-L` lists the table as `hash`
			// commands. Measured 2026-09-13 by asking zsh 5.9.2 for every
			// letter of the alphabet: it takes these six and refuses the
			// rest, so a script asking for one is told it is missing here
			// rather than told zsh has not got it.
			"hash": "dfmvL",
			// read's letters about a terminal or the line editor —
			// -e/-E echoing, -z and the zle pair -c/-l. The -p
			// coprocess is implemented as its measured refusal — see
			// ReadNoCoprocess. zsh's read also says nothing at all about a
			// dead -u descriptor and reports 1, which is why no
			// ReadBadFileDescriptor wording appears here.
			//
			// `-k` has left this list and joined ReadOptions above, spelled
			// `k#` — a number, optional — which is the one letter in the
			// panel with that shape. It reads characters from the terminal;
			// interp/readkeys.go carries what was measured. `-q` has left it
			// the same way and for the same reason: it is that read with a
			// yes-or-no verdict on the end, and the two letters compose. The
			// two tables move together on purpose: a letter in the accepted
			// set and still named here is refused as missing while it works,
			// and a letter in neither is `bad option` for something zsh has.
			"read": "eEzcl",
			// typeset's letters this engine does not hold: the float
			// format (-E), the key read (-k) and tracing (-t).
			// `-H`, `-U`, `-T`, `-h` and `-m` have left this list — they are
			// implemented, in DeclareOptions above.
			// `L`, `R` and `Z` have left this list: they are the width
			// attributes and are implemented, in DeclareOptions and
			// DeclareOptionsTakingANumber above.
			//
			// **`-b`, `-c` and `-n` have left it because this shell has not
			// got them**, which is the other way a letter leaves: naming one
			// here says "zsh has it and we do not", and refuses it as
			// missing where zsh refuses it as unknown. Measured 2026-09-15
			// on zsh 5.9.2, `env -i` with a scratch HOME, in a function so
			// all three words are reachable — `typeset -b v=1`, `-c` and
			// `-n` are each `bad option` under `typeset`, `declare` and
			// `local` alike, while `-E`, `-k` and `-t` are silently taken.
			// `-n` is the one that was load-bearing: #2553 built the name
			// reference for bash and ksh93 on the premise that this shell
			// spelled it too, and it does not.
			"typeset": "k",
			// `w` has left this list and joined TypeOptions above, in the same
			// change: a letter in both is refused as missing while it works,
			// and a letter in neither is `bad option` for something this
			// shell has. TestNoLetterIsBothAcceptedAndCalledMissing is the
			// invariant.
			"type": "mvsS",
			// jobs' letters that are zsh's own: -d names the directory the
			// job was started in, and -z and -Z are about the process
			// title rather than about the job table.
			// `-d` came off this list in #4507: the directory line it
			// writes needed the job table to record where each job
			// started, which nothing here did before.
			"jobs":    "zZ",
			"declare": "k",
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
			//
			// `-k` was gone for the same reason `-b`, `-c` and `-n` are:
			// measured in the same run, `local -k v=1` inside a function is
			// `bad option: -k` where `typeset -k` and `declare -k` are taken
			// — so this list was one letter shorter than theirs and not the
			// same list after all. `-t` was the last of it and left with
			// #3101, so `local` has no row here at all now.
			// `integer`'s own short list, and it is not typeset's: the
			// letters typeset is missing that `integer` refuses outright —
			// b, c, E and m — are bad options under this name and belong in
			// neither field, while `-t`, `-L`, `-R` and `-Z` are letters
			// this shell's `integer` really takes. `-h` was here too and is
			// implemented now, in IntegerOptions above.
			"integer": "LRZ",
			// `functions`' own letters, none of which is `typeset`'s: -u
			// and -U mark a name for autoloading, -k and -z pick which
			// shell the autoloaded file is read as, -t and -T trace, -x
			// sets the listing's indent, -c makes one function another's
			// copy and -W is the `zsh/parameter` writability flag. -m and
			// -M are implemented, in FunctionsOptions above, and so are
			// -u and -U: they mark, and with no operands they are the
			// listing a bare `autoload` writes (#1996).
			"functions": "ckstxzTW",
		},
		// `u` alone, and that is the measurement rather than an omission:
		// `functions +u` and `typeset +fu` are `invalid option(s)` at 1
		// where `functions +U` and `typeset +fU` are a listing at 0, though
		// both letters mark. So the refused set is strictly narrower than
		// Semantics.FunctionLettersThatMarkUndefined and cannot be derived
		// from it (#1996).
		MarkingLettersUnderPlus: "u",
		// `functions`' own wording and not `autoload`'s `bad option: -Q`,
		// because the line never reached a marking: `zsh:functions:1:
		// invalid option(s)` and `zsh:typeset:1: invalid option(s)`, each
		// naming the word that was written.
		MarkingUnderPlusRefusal: "invalid option(s)",
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
		// The sixteen rows zsh 5.9 prints on Linux. This shell lists in the
		// *kernel's* order rather than one of its own — see
		// UlimitListingInKernelOrder — so the sequence here is a reading
		// order and not the output: on a kernel that numbers nine limits
		// this prints nine, in that kernel's sequence.
		//
		// Measured 2026-09-18 on zsh 5.9.2 (macOS arm64) and 5.9 in the
		// panel's Alpine image. macOS numbers the resident set and the
		// address space alike and this shell prints the address-space row
		// for it, which is why it has no `-m` there and refuses the letter
		// while bash, listing its own table, reads it (#2806).
		//
		// `-N 15` is the one row with no letter behind it: the option form
		// is `-N <number>`, which nothing else in the panel has, so the row
		// prints and the letter is not read.
		UlimitListing: []interp.UlimitListingRow{
			{Prefix: "-t: cpu time (seconds)              ", Letter: 't', Res: interp.ResourceCPUTime, Scale: 1},
			{Prefix: "-f: file size (blocks)              ", Letter: 'f', Res: interp.ResourceFileSize},
			{Prefix: "-d: data seg size (kbytes)          ", Letter: 'd', Res: interp.ResourceData, Scale: 1024},
			{Prefix: "-s: stack size (kbytes)             ", Letter: 's', Res: interp.ResourceStack, Scale: 1024},
			{Prefix: "-c: core file size (blocks)         ", Letter: 'c', Res: interp.ResourceCore},
			{Prefix: "-m: resident set size (kbytes)      ", Letter: 'm', Res: interp.ResourceResidentSet, Scale: 1024},
			{Prefix: "-u: processes                       ", Letter: 'u', Res: interp.ResourceProcesses, Scale: 1},
			{Prefix: "-n: file descriptors                ", Letter: 'n', Res: interp.ResourceOpenFiles, Scale: 1},
			{Prefix: "-l: locked-in-memory size (kbytes)  ", Letter: 'l', Res: interp.ResourceLockedMemory, Scale: 1024},
			{Prefix: "-v: address space (kbytes)          ", Letter: 'v', Res: interp.ResourceAddressSpace, Scale: 1024},
			{Prefix: "-x: file locks                      ", Letter: 'x', Res: interp.ResourceFileLocks, Scale: 1},
			{Prefix: "-i: pending signals                 ", Letter: 'i', Res: interp.ResourcePendingSignals, Scale: 1},
			{Prefix: "-q: bytes in POSIX msg queues       ", Letter: 'q', Res: interp.ResourceMessageQueues, Scale: 1},
			{Prefix: "-e: max nice                        ", Letter: 'e', Res: interp.ResourceSchedulingPriority, Scale: 1},
			{Prefix: "-r: max rt priority                 ", Letter: 'r', Res: interp.ResourceRealtimePriority, Scale: 1},
			{Prefix: "-N 15: rt cpu time (microseconds)   ", Res: interp.ResourceRealtimeTime, Scale: 1},
		},
		UlimitListingInKernelOrder: true,
		// `-N <number>`: the one option in the panel whose *operand* names
		// the resource, by the kernel's own number. It exists because this
		// shell has a row its letters do not cover — `-N 15: rt cpu time
		// (microseconds)` — and the number is the platform's, which is what
		// `ulimit -N 7` says: 10666 processes on macOS and 1024 open files
		// on Linux, from the same shell. Measured 2026-09-18 on zsh 5.9.2
		// (macOS arm64) and zsh 5.9 in the pinned Alpine image (#3667).
		UlimitNumberedOption: interp.UlimitNumberedOption{
			Letter:      'N',
			NeedsNumber: "number required after -%[1]s",
			BadNumber:   "invalid number: %[1]s",
			OutOfRange:  "can't read limit: invalid argument",
		},
		// Lazy rather than QuoteShell: this shell drops the empty `''`
		// segments that closing and reopening leaves, so `ab'` traces as
		// `'ab'\\'` where bash writes `'ab'\\'''`. Measured 2026-09-13 against
		// bash 5.3.15; the two shared a value until #2695 because the corpus
		// row used `it's`, the one shape they agree on.
		TraceQuoting: interp.QuoteShellLazy,
		TraceEscape:  interp.TraceEscapeControlNotation,
		// The path inside a `type`, `command -V` or `whence -v` sentence is
		// written back the way this shell writes a word — the same spelling,
		// against the same alphabet below — while `command -v`, `whence`,
		// `where` and `which` write the resolved path plain. Measured
		// 2026-09-19 with a directory on PATH holding an executable called
		// `a b`: `type 'a b'` is `a b is '/…/bb/a b'` and `command -v 'a b'`
		// is `/…/bb/a b`. The *name* is bare in both, which is what keeps
		// this apart from NameReportQuoting (#3702).
		TypeSentencePathQuoting: interp.QuoteShellLazy,
		// No position rule at all: every character it quotes it quotes
		// anywhere, which is what makes the *shape* of this field necessary
		// rather than a longer string in one shared set. It is the only one
		// of the three that quotes `=`, and it quotes `^` with bash and
		// leaves `!` bare with ksh93. Measured 2026-09-12.
		TraceMetacharacters: interp.TraceMetacharacters{
			Anywhere: "*?[]{}~#=^",
		},
		// The `[` that opens a test is bare and the `]` that closes it is an
		// argument like any other: `[ 1 -lt 2 ']'`.
		TraceBareBracket: interp.TraceBracketCommandWordBare,
		// A declaration utility's operand is written as an assignment —
		// the target bare and only the value quoted — where bash quotes
		// the whole word. Measured 2026-09-18: `typeset x="a b"` is
		// `typeset x='a b'` here and `typeset 'x=a b'` there, and
		// `typeset x=a=b` is `typeset x='a=b'` here, where that shell
		// has no reason to quote at all. The same word after `echo` is
		// one quoted word in both, which is what says it is the command
		// in front of it that decides (#3654).
		TraceAssignmentOperand: interp.TraceAssignmentOperandValue,
		TraceStyle:             interp.TraceNameLine,
		TraceForHeader:         interp.TraceForAssign,
		TraceArrayLiteral:      interp.TraceArraySpaced,
		// The prefix goes on the command's own line, with the trace prefix
		// written a second time between the assignments and the words when
		// the command is one this shell runs itself: `+x.sh:3> A=3 +x.sh:3>
		// f zz` for a function and `+x.sh:4> B=4 /bin/echo c` for an
		// external.
		TracePrefixAssignment: interp.TracePrefixOnTheCommandLineRepeatingThePrefix,
		// The subject and the arm's patterns, once per arm it tries, where
		// bash prints the header as written once and the other two print
		// nothing. The line count is the information: it says how far down
		// the arms the subject got.
		TraceCaseHeader: interp.TraceCaseArm,
		// One line for the whole condition rather than one per primary, and
		// the operands quoted: `[[ -n a && -n b ]]` is one line here and two
		// in bash and ksh93.
		TraceCondition:        interp.TraceCondWhole,
		TraceConditionQuoting: interp.QuoteShell,
		// The two arithmetic sites disagree in this shell alone: a `(( ))`
		// command is wrapped in spaced parentheses and the three parts of a
		// `for ((;;))` header are written bare — `i=0`, `i<2`, `i++`.
		TraceArithForPart: interp.TraceArithBare,
		// zsh names the last token it read and nothing else.
		EvalNaming:       interp.SourceReplacesShell,
		SourceFileNaming: interp.SourceReplacesShell,
		EvalSourceName:   "(eval)",
		// A runtime failure at the top level of a sourced file names the
		// file — `./inc.sh:1: command not found: nosuch` — while one inside
		// a function still names the function, which the answer above wins.
		LocationNamesTheCurrentFile: true,
		// A dot script's failure is diagnosed under the name it could not
		// open, which is the same rename DollarZeroNames makes on the path
		// where the file does open (#2958).
		DotFailureNamesTheFileItCouldNotOpen: true,
		// And text `eval` is running is named for itself over both — see
		// Diagnostics.LocationNamesTheEvalText, where the nine rows are.
		LocationNamesTheEvalText: true,
		// zsh does not quote the expression, where the other three do.
		ArithError:          "%[2]s",
		ArithInvalidBase:    "invalid base (must be 2 to 36 inclusive): %[1]s",
		ArithRecursionLimit: "math recursion limit exceeded: %[1]s",
		// And it is the name the expression was written with, not the one the
		// bound stopped on: `a=b; b=a` is blamed on `a` here and on `b` in the
		// two shells above.
		ArithRecursionBlamesTheWrittenName: true,
		OptionListingWidth:                 optionListingWidth,
		KillListing:                        interp.KillListingSpaceJoined,
		// The event a refused range names, which is not always the `1` an
		// empty list used to be the only route to: `fc -l -1` on that same
		// empty list says `no such event: 0`, and `fc -l 99` says 99.
		FcNoSuchEvent: "no such event: %[1]d",
		// Its pair, for a refused range whose two ends are different
		// numbers — `fc -l 6 7` on five entries — where there is no one
		// event to name.
		FcNoEventsInRange: "no events in that range",
		// And the operand that named nothing is named here, where bash says
		// only that it found no command: `fc -l zzz` is `event not found:
		// zzz`, and so is `fc -l -0`, which this shell does not read as a
		// relative operand at all.
		FcNoCommandFound: "event not found: %[1]s",
		// The line this shell is standing on, which `fc -s` and the editor
		// road may not reach. A sentence about what running it would do,
		// and it names no operand — `fc -s 5`, `fc -s 99` and a bare
		// `fc -s` on a one-entry list all come to these words.
		FcCurrentLineRecurses: "current history line would recurse endlessly, aborted",
		// And the range this shell will not run in the order it was asked
		// for, which also names no operand.
		FcBackwardsRange: "history events can't be executed backwards, aborted",
		// The editor's file left empty, named by the path the person never
		// saw.
		FcEmptyEdit:  "read error on %[1]s",
		NoJobControl: "no job control in this shell.",
		// The refusal the verbs that would *act* on a job give in a subshell
		// holding nothing but the parent's. No verb in the sentence: the
		// builtin's name is in the location, which is this shell's rule for
		// every message. Measured 2026-09-25 under `-fm` on a
		// pseudo-terminal — `<script>:wait:6: can't manipulate jobs in
		// subshell`, at 1, and `disown` the same (#4538).
		JobsNotManipulableInASubshell: "can't manipulate jobs in subshell",
		FdVariableWithoutADescriptor:  "parameter %[1]s does not contain a file descriptor",
		// `mkdir dir; v=$(<dir)` — the read after a successful open, which
		// this shell alone words. The name is the word as it expanded, not
		// the path the working directory made of it (#1778).
		FileSubstitutionReadError: "error when reading %[1]s: %[2]s",
		// A failed write to a stream this command did not itself close.
		// Neither the builtin nor the number is named, so the format has one
		// verb where the others have two (#1363).
		InheritedClosedStreamWriteError: "write error: %[2]s",
		// The builtin's own complaint, reached only where the write failed
		// with EPIPE -- the closed-descriptor axis answers No above and
		// returns before this is read, which is why one wording serves both
		// errnos here without making the quiet route speak. The builtin is
		// named by the location rather than by a verb, so the format uses
		// only the reason, exactly as the sentence above it does (#770).
		BuiltinWriteError: "write error: %[2]s",
		// A conditional with no `:` is its own sentence; a conditional
		// missing either value is the ordinary end of input, so
		// ArithConditionalThen and ArithConditionalElse stay empty and fall
		// through to it.
		ArithConditionalColon: "bad math expression: ':' expected",
		// And a `:` with no `?` in front of it, which is a complaint only a
		// reader that took the colon and then read a second value can make.
		ArithColonWithoutQuestion: "bad math expression: ':' without '?'",
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
		// The same family's sentence for `++` on something that cannot be
		// assigned to, and it names neither the operator nor the operand:
		// `$(( 1++ ))` is `bad math expression: lvalue required`. Measured
		// 2026-09-14 (#2420).
		ArithIncrementNeedsAPlace: "bad math expression: lvalue required",
		ArithBadFloatConstant:     "bad floating point constant",
		ArithExpressionRanOut:     "bad math expression: operand expected at end of string",
		ArithOperatorExpected:     "bad math expression: operator expected at `%[1]s'",
		// The output format's own two, neither of which opens with `bad math
		// expression:` and neither of which names the text it refused —
		// measured 2026-09-12, `zsh:1: bad output format specification` for
		// `$(( [# 16] 1 ))` and `zsh:1: bad base syntax` for `$(( [16] 1 ))`.
		// One character apart and worded apart, which is why the parser tells
		// them apart rather than calling both a bad specifier.
		ArithBadOutputFormat: "bad output format specification",
		ArithBadBaseSyntax:   "bad base syntax",
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
		// The operator that backgrounds a command and lets go of it is
		// written two ways here and named by one of them — `&!` in a
		// refusal comes back `&|`. It is the only pair this shell folds;
		// see interp.Diagnostics.SyntaxUnexpectedNamesTheDisowningOperatorWithAPipe
		// for the measurement and for the pairs that keep their spelling.
		SyntaxUnexpectedNamesTheDisowningOperatorWithAPipe: true,
		// The sentence this shell writes in front of that one when the
		// function being defined is named by an alias. Two lines, and the
		// second is the parse failure above located at the parentheses —
		// which is why this is a remark rather than the error. See
		// syntax.Dialect.AliasAtAFunctionName (#3643).
		FunctionNameIsAnAlias: "defining function based on alias `%[2]s'",
		// The newline is spelled `\n` here rather than by name, and is
		// blamed on the line it ends (#1364).
		SyntaxUnexpectedNewline:          "parse error near `\\n'",
		UnexpectedNewlineIsOnTheNextLine: true,
		// The same echo bash gives, in this shell's sentence: `"zzz"` and
		// `'a b'` come back with their quotes on (#1239).
		UnexpectedWordNaming: interp.UnexpectedWordIsSourceText,
		// zsh numbers a function body that never began from the parentheses
		// it was due after rather than from the top of the input, and writes
		// no line at all where the two stand together. `f() ;` reports as
		// zsh: parse error near `;', `foo()` on line 3 of a file is still
		// s.sh:1:, and `if true` — an input that ran out just as much —
		// reports the file's own line, as zsh:1: parse error near `true'.
		MissingFuncBodyCountsFromItsParens: true,
		// zsh lets the body of a `$( … )` that never closed say what was
		// wrong with it before complaining about the substitution, and puts
		// both messages on that refusal's line. `v=$(echo hi; for` is
		// `:2: parse error near `\n'` and then the quote at `:2:`, where
		// `v=$(echo hi` — a body that parses — is the quote alone.
		UnterminatedSubstitutionWritesItsBodysRefusal: true,
		ForName: "parse error near `%[1]s'",
		// A C-style `for` header with fewer than two separators, named by
		// the text of its last part and by nothing where that part is blank.
		// Measured 2026-09-12: `for ((i=0))` is `parse error near `i=0'`,
		// `for ((;2))` names `2`, and `for ((;))`, `for ((1;))` and
		// `for (( x ; ))` are a bare `parse error`. More than two separators
		// is not refused here at all — see
		// syntax.Dialect.ForArithExtraSeparators (#2225).
		ForArithHeader:       "parse error near `%[1]s'",
		ForArithHeaderNoPart: "parse error",
		Unterminated:         "parse error near `%[5]s'",
		UnmatchedQuote:       "unmatched %[1]s",
		UnmatchedCmdSubst:    "parse error near `%[3]s'",
		// This shell prints at most twenty bytes of the word and marks it,
		// and marks it at exactly twenty as well — see the field. The other
		// three print the whole word or none of it.
		UnmatchedNearMaxBytes: 20,
		// And a control character in that text is written as an escape
		// rather than sent to the terminal — a tab as `\t`, a `\001` as
		// `^A`. Measured 2026-09-17 one byte at a time; see
		// Diagnostics.NearTextEscapesControlCharacters for the seven rows
		// and for where the high half stops (#3563).
		NearTextEscapesControlCharacters: true,
		// And the same cut on the second message a substitution body refused
		// at expansion time is given — see the field.
		SubstitutionParseFailureQuotesTheWord: true,
		SubstitutionParseFailureSentence:      "parse error in command substitution",
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
		SourcedFatalStatus: 126,
		// The builtin the script wrote rather than a literal `.`, because
		// this shell has two names for it and says the one it was given.
		// Measured 2026-09-12 on zsh 5.9.2 with nothing of that name
		// anywhere: `source nope.sh` is `zsh:source:1: no such file or
		// directory: nope.sh` and `. nope.sh` is `zsh:.:1: …`. The name is
		// in the location either way — see NamesBuiltinInLocation — and
		// spelling it here lets the rule that strips a duplicated name reach
		// the second one too. A literal `.` survived it and made every
		// `source` failure read `zsh:source:1: .: no such file …`.
		DotCannotOpen:       "%[3]s: no such file or directory: %[1]s",
		DotCannotOpenStatus: 127,
		DotNoOperand:        "%[1]s: not enough arguments",
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
		// A `#!` line naming an interpreter that is not there, which this
		// shell reads the file to find out about — and words the other way
		// round from bash, with the complaint before the name it could not
		// find. Measured 2026-09-25 on 5.9.2: `./bad` whose first line is
		// `#!/nonexistent/interp` is `zsh:1: ./bad: bad interpreter:
		// /nonexistent/interp: no such file or directory`, keeping the line
		// bash drops and numbering it 127 where bash says 126. A script
		// testing `$? -eq 127` for "not found" is reading that number.
		BadInterpreter:       "%[1]s: bad interpreter: %[2]s: %[3]s",
		BadInterpreterStatus: 127,
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
		// The sign is the wording's second verb here, where the other four
		// dialects spell a `-` into the format: this is the one shell that
		// reads a `+`-prefixed word as an option, so it is the one shell
		// whose complaint has two signs to write. `getopts a o +z` is `bad
		// option: +z` and `getopts a: o +a` is `argument expected after +a
		// option`. Measured 2026-09-17 on 5.9.2.
		GetoptsBadOption:       "bad option: %[2]s%[1]s",
		GetoptsMissingArgument: "argument expected after %[2]s%[1]s option",
		// Not a usage line at all here — this shell counts the operands and
		// says so, and it is the same sentence several of its builtins write
		// for too few words. Measured 2026-09-14: `<script>:getopts:1: not
		// enough arguments`, status 1 rather than the 2 the rest report, and
		// the script carries on (#2801).
		BuiltinUsage: map[string]string{
			"getopts": "getopts: not enough arguments",
		},
		GetoptsUsageStatus: 1,
		CdCannotChange:     "%[2]s: %[1]s",
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
		UnaliasNotFound: "no such hash table element: %[2]s",
		// `zsh:unalias:1: not enough arguments` — the location carries the
		// builtin's name, so the sentence does not.
		UnaliasNoPattern:       "not enough arguments",
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
			// `printf -v` says exactly what `read` says, and says it to
			// `a-b` as well as to `1x` — measured 2026-09-17,
			// `printf -v 'a-b' %s Q` is `not an identifier: a-b`. So there
			// is no BuiltinBadNameNumeric row beside this one either.
			"printf": "not an identifier: %[2]s",
			// And `getopts`, which says the same to `1bad` and to `a-b`
			// alike — measured 2026-09-18, and the script ends on it, which
			// is Semantics.BadNameToGetoptsFatal (#3555).
			"getopts": "not an identifier: %[2]s",
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
		// A word holding a bracket that never closes is judged by the
		// identifier rule rather than the context one, and the location
		// leaves the builtin out — see Diagnostics.BuiltinBadNameBracketed.
		// Measured 2026-09-12: `typeset 'm[a]b]'=v` is `<shell>:1: not an
		// identifier: m[a]b]` where `typeset 'a]'=v`, with no bracket to
		// open one, is `<shell>:typeset:1: not valid in this context: a]`.
		//
		// `export` is on the same side, measured 2026-09-19: `export
		// 'm[a]b]'=v` and `export 'a[1]+'=v` are both `<shell>:1: not an
		// identifier: <name>` where `export 'a]'=v` and `export 'x+'=v` keep
		// the builtin and the other wording. `readonly` is **not** — it
		// answers a bracketed name with `<name>: can't create readonly array
		// elements`, which is the element refusal rather than a name one, and
		// is filed on its own rather than spelled as a third wording here.
		BuiltinBadNameBracketed: map[string]string{
			"typeset": "not an identifier: %[2]s",
			"declare": "not an identifier: %[2]s",
			"local":   "not an identifier: %[2]s",
			"integer": "not an identifier: %[2]s",
			"export":  "not an identifier: %[2]s",
		},
		BuiltinBadOptionStatus: 1,
		PrintfUsage:            "not enough arguments",
		PrintfUsageStatus:      1,
		TrapBadSignal:          "undefined signal: %[1]s",
		KillNoSuchProcess:      "kill %[1]s failed: no such process",
		// The same sentence with the errno in it, for the sends this shell
		// makes that the two above do not cover: `kill -99 $$` is `kill
		// <pid> failed: invalid argument`, measured. The other two are this
		// wording with ESRCH and EPERM written out, which is why all three
		// read alike.
		KillSendFailed:            "kill %[1]s failed: %[4]s",
		KillNotPermitted:          "kill %[1]s failed: operation not permitted",
		KillInvalidSignal:         "unknown signal: %[3]s",
		KillIllegalOption:         "unknown signal: %[3]s",
		KillNotAPid:               "illegal pid: %[1]s",
		KillMissingSignalArgument: "%[1]s: argument expected",
		// The third route a signal spec takes here, and it is a wording of
		// its own rather than a spelling of either other one: a word written
		// where a number goes and that is not one. Verb 5 is the operand as
		// the script wrote it, which is what carries the dash for `kill -9x`
		// and leaves it off for `kill -n 9x`. Measured 2026-09-17 on zsh
		// 5.9.2 — and the listing hint does *not* follow this one, where it
		// follows both of the others (#3167).
		KillInvalidSignalNumber: "invalid signal number: %[5]s",
		KillUsage:               "not enough arguments",
		// -L, capitalized, and measured against the zsh the panel resolves:
		// 5.9.2 from Homebrew says -L where Apple's /bin/zsh 5.9 says -l.
		// Probing whichever zsh came first on PATH is how the lowercase one
		// got here.
		KillUnknownSignalHint: "type kill -L for a list of signals",
		// zsh is the one dialect that does not treat "nothing to signal" as a
		// usage error worth a different number from any other failure.
		KillUsageStatus: 1,
		// The only dialect that reads the word as an operator and says so:
		// `[ -Q x -a -n x ]` is `zsh:[:1: unknown condition: -Q` (#1290).
		TestUnknownLongOperator: interp.TestUnknownOperatorNamed,
		TestUnaryExpected:       "unknown condition: %[1]s",
		TestBinaryExpected:      "condition expected: %[1]s",
		TestIntegerExpected:     "integer expression expected: %[1]s",
		// The whole substitution as it was written, not its inside.
		ProcessSubstitutionNotInCondition: "process substitution %[1]s cannot be used here",
		// An empty `=~` right operand, which this shell refuses in its
		// engine's words and leaves the condition **false** over — 1, the
		// status a match that did not happen gives, where bash calls the
		// same operand a failure of the construct and ends at 2. Measured
		// 2026-09-18, `[[ abc =~ "" ]]` in a script file (#3279).
		EmptyRegexOperand:       "failed to compile regex: empty (sub)expression",
		EmptyRegexOperandStatus: 1,
		TestTooManyArguments:    "too many arguments",
		// `-a` and `-o` are words this shell knows — as the connectives —
		// so one standing where a unary operator belongs is a string with a
		// word left over rather than an operator it has never heard of.
		// Measured: `test -a f` is `too many arguments` and `test -Q f` is
		// `unknown condition: -Q`.
		TestConnectiveIsALeftoverWord: true,
		TestOperandExpected:           "argument expected",
		TestMissingBracket:            "']' expected",
		TimesDecimals:                 2,
		TimesArguments:                "times: too many arguments",
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
		// line: `zsh:shift:1:`. A rule rather than a handful of cases, and
		// the first shell in the panel measured to do it — BusyBox ash is the
		// second, with a space after the colon rather than none, which is the
		// location style's own punctuation and not a second axis (#2761).
		NamesBuiltinInLocation: true,
		// Two refusals this shell locates as its own rather than the
		// builtin's, where the other shell that names builtins keeps the
		// name. `readonly r=1; unset r` is `zsh:2: read-only variable: r` and
		// `exec nosuchcmd` is `zsh:1: command not found: nosuchcmd` — the
		// second word for word what a bare command word reports. Both were
		// rules in the substrate until a dialect disagreed with them; see the
		// fields for the rows.
		UnsetReadonlyIsTheShellsOwn: true,
		ExecNotFoundIsTheShellsOwn:  true,
		// Except for a math complaint, which this shell writes as its own:
		// `let '1+'` is `zsh:1: bad math expression: …` where its `cd` is
		// `zsh:cd:1: …`, and `let` with no operand at all *is* the builtin's
		// and does say `zsh:let:1:`. See the field.
		ArithErrorNamesTheBuiltin: false,
		LowercaseReason:           true,
		DirectoryReason:           "Permission denied",
		HashNotFound:              "no such command: %[1]s",
		HashNamedDirNotFound:      "hash: no such directory name: %[1]s",
		// `rehash` and `unhash` are this shell's own names for the two
		// halves of the table, and each refuses under its own name.
		// Measured 2026-09-18 on zsh 5.9.2: `rehash foo` is `too many
		// arguments`, `unhash` alone is `not enough arguments`, and every
		// table answers a name it has not got with the identical `no such
		// hash table element` — all three at 1 (#3109).
		RehashTooManyArguments: "too many arguments",
		UnhashNoOperands:       "not enough arguments",
		UnhashElementNotFound:  "no such hash table element: %[2]s",
		HashNamedDirBadName:    "hash: invalid character in directory name: %[1]s",
		// `ls=/bin/ls`, the shape an assignment would have — zsh and ksh93
		// both write the table that way.
		HashListing: interp.HashListingNameEqualsPath,
	}
}

// Apply adds what zsh has and the substrate does not.
//
// `source` is the second name for `.`, which dash does not have at all — so
// the name is a dialect's answer, and it is the same function under a second
// name rather than a second implementation.
//
// It is not a *synonym*, which this comment used to say and which is true only
// of bash and ksh93. Measured 2026-09-12 on zsh 5.9.2, in a directory holding
// `plain.sh`:
//
//	source plain.sh   `sourced`, status 0
//	. plain.sh        `no such file or directory`, status 127
//
// The second row is what makes the measurement discriminating: a shell that
// simply searched the current directory for both would agree with the first
// and break the second. See where the name is registered below.
func Apply(r *interp.Runner) {
	// This shell has an `enable`, but a different one: it works on hash
	// tables and takes none of bash's options — `enable -n` is a bad option
	// there. Claiming a bash-shaped one would be worse than not having it.
	// Not removed but replaced: zsh has an `enable`, and it is a different
	// builtin from the one the core carries. See enable.go.
	registerEnable(r)
	// `log` is in the table for `disable log` to reach: macOS's `/etc/zshrc`
	// writes that line to keep the builtin out of the way of `/usr/bin/log`,
	// and a shell that reads the system-wide files meets it before the first
	// prompt (#2325). See log.go for what it does and does not do.
	r.Register("log", logBuiltin)
	// How zsh scripts actually change options, and how they change shells.
	// See setopt.go and emulate.go.
	registerSetopt(r)
	registerZcompile(r)
	registerCompctl(r)
	// And the new completion system's own two, which are what a `zle -C`
	// widget's function is written in. See compsys.go for the context they
	// refuse outside of, and #2776 for the rest of the module they belong to.
	registerCompadd(r)
	registerCompset(r)
	// And the module's other four features, which are conditions rather than
	// builtins: `[[ -prefix … ]]` and its three neighbors are the tests
	// `compset` performs without the move. See completioncondition.go, and
	// #3042 for the measurement that says loading the module is not what
	// makes them exist.
	registerCompletionConditions(r)
	// And `zsh/computil`'s eight, which is what the completion system zsh
	// *ships* is written in: `_arguments`, `_describe`, `_tags` and `_values`
	// are shell functions whose working parts are these. See computil.go.
	registerComputil(r)
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
	// And StarStarSeesLinkedDirectories stays off, which is the third
	// question about the same component and is measured separately rather
	// than assumed from the second. In a directory holding `r/x` and a
	// symlink `s` to `r`, `echo **/` is `r/` here and `r/ s/` in bash 5.3.15
	// under `shopt -s globstar`: the component stands for the levels the
	// walk crossed, and a link is not one of them. Neither shell **enters**
	// the link — `echo **/x` is `r/x` in both — so what splits is what `**/`
	// lists and not where it goes (#2360).
	//
	// `***/`, which does follow links, is a construct of its own rather than
	// this option turned on: pointed at a tree holding a link to its own
	// ancestor it walks until the kernel refuses, and zsh reports `too many
	// levels of symbolic links` once per link per level. That is the
	// distinction being deliberate, written down where somebody would
	// otherwise read the refusal above as an oversight.
	//
	// The fourth and fifth questions about the same component, and this
	// shell answers both the way bash does. A `**` that matched zero levels
	// is the directory the walk stood in — `d/**/` is `[d/][d/e/]` here and
	// in bash, against `[d/e/]` in ksh93 — and a walk may begin inside a
	// directory the pattern reached through a link: `s/**` is `[s/x]` here,
	// `[s/][s/x]` in bash, and no match at all in ksh93. Measured 2026-09-16
	// (#3152).
	r.SetMatchOption(interp.StarStarZeroLevelIsTheDirectoryItStartsFrom, true)
	r.SetMatchOption(interp.StarStarPatternsReadLinkedDirectories, true)
	// What `=~` matched is readable here, and under the *same* parameters a
	// reporting pattern fills rather than under a record of its own: `$MATCH`
	// is the whole match, `$match` the groups alone, and `$MBEGIN`/`$MEND`
	// with `$mbegin`/`$mend` where each began and ended. Measured 2026-09-11
	// against zsh 5.9.2 with `[[ 'hello world' =~ 'o (w[a-z]+)d' ]]`:
	// `MATCH=o world MBEGIN=5 MEND=11`, `match=(worl) mbegin=(7) mend=(10)`.
	//
	// A failing match leaves all of them alone — measured, a `zzz` that
	// matches nothing still reads back the match before it — which is the
	// reporting pattern's own rule and the opposite of the dense array the
	// other shell keeps. See interp/regexmatch.go.
	//
	// It is what makes `regexp-replace` possible: a global search and replace
	// in shell needs the *extent* of each match and not only that there was
	// one, and this is the only thing that reports it (#1968).
	r.SetRegexCaptureReport()
	// $LINES and $COLUMNS follow the window here too, and — like `**/` above
	// — with no option name to ask for it: this shell simply does it, so
	// there is nothing in setopt.go for a script to turn off. Measured
	// 2026-09-08 through a pseudo-terminal against zsh 5.9.2 started with
	// `-f`, which reports `COLUMNS=80 LINES=24` at its first prompt and
	// `132`/`40` at the next prompt after a resize.
	//
	// **Not the switch bash's `checkwinsize` moves**, which is what this line
	// used to be and what #2107 is. That switch is permission for the front
	// end to *assign* two variables once per prompt, and it is right for
	// bash — measured, bash has the pair only when it is interactive and
	// leaves both unset otherwise. This shell's are parameters of the
	// language: present under plain `-c` with no prompt anywhere, `0` rather
	// than unset where there is no terminal at all, `integer-special` to
	// `${(t)}`, and following a window that changes while a script runs. A
	// prompt theme measures itself against them — powerlevel10k saves
	// `$COLUMNS`, forces 1024, and shortens the directory to what is left —
	// so with the pair merely unset the arithmetic ran on an empty word and
	// the path came out at full length. See interp.Runner.ProvideWindowSize,
	// which holds the four answers and the rule about assignment.
	r.ProvideWindowSize()
	// The two integers the try-always block reports through, and recovers
	// through. The mechanism is the core's and only the spelling is here —
	// see interp.Runner.SetAlwaysBlockStatus for the measurements (#1234).
	r.SetAlwaysBlockStatus("TRY_BLOCK_ERROR", "TRY_BLOCK_INTERRUPT")
	// A job still running holds the exit here, where bash needs to be asked:
	// measured, zsh 5.9.2 started with `-f` answers `sleep 40 &` then `exit`
	// with `you have running jobs.` and stays, and bash 5.3.15 leaves. Both
	// shells spell the option `checkjobs`; only the defaults differ, and this
	// is where this one's is set. See interp.Runner.ChecksRunningJobsAtExit.
	r.SetChecksRunningJobsAtExit(true)
	// And the jobs that are still running when the session goes anyway are
	// sent SIGHUP, which is `setopt hup` and is **on** in a fresh zsh — the
	// switch the core starts off and bash only reaches through `shopt -s
	// huponexit`. Independent of `checkjobs` above, which is measured:
	// `unsetopt checkjobs` leaves at the first `exit` and still hangs the
	// jobs up and still says so. See interp.Runner.SendsHangupToJobsAtExit
	// and the three HangupAtExit axes for what this shell does differently
	// once the switch is on.
	r.SetSendsHangupToJobsAtExit(true)
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
	//
	// #1405 asked for it beside `vared` and `zcompile` and said to order the
	// three by evidence. Swept over `~/.zi` and the installed function
	// directories on this machine: `zcompile` in 25 files, `vared` in 3,
	// `zregexparse` in **none**. It is absent because the rest of completion
	// is — #1282 recorded that `compinit` is not close, the file being
	// refused and `zmodload zsh/complete` refusing by name — so the one
	// parser would be reachable by nothing.
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
	// Stopping the shell itself, which is two halves: a refusal a script sees
	// and a stop only a binary that owns the process may make. Not bash's
	// builtin under the same spelling — job control decides nothing here and
	// the statuses differ. See suspend.go.
	registerSuspend(r)
	// Editing a variable with the line editor: everything a script outside a
	// session sees, which is all of it without a terminal. See vared.go.
	registerVared(r)
	// The module loader, which answers per module rather than pretending to
	// load anything. See zmodload.go.
	registerZmodload(r)
	// And the other half of a module system: a name defined from `$fpath`
	// the first time it is called. See autoload.go.
	registerAutoload(r)
	// Eleven of `zsh/parameter`'s thirty-three: this shell's own tables read
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
	// And `zsh/mapfile`'s one: the filesystem as an association, where a key
	// is a path and the value is that file's bytes. See mapfile.go.
	registerMapfileModule(r)
	// And `fc`'s three file letters over a history list this dialect keeps,
	// which is what `print -s` fills. The core `fc` stays the answer for
	// every other letter — this registration replaces it and delegates. See
	// fchistory.go.
	registerFcHistory(r)
	// And `zsh/terminfo`'s and `zsh/termcap`'s one parameter each: the
	// terminal's capabilities under two name systems, read out of the
	// description `$TERM` names by repl.TerminalCapabilities. See
	// terminfo.go.
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
	// And `zsh/zpty`'s one: a command run on a pseudo-terminal of its own,
	// which is the last module of #1320's table and the first thing in this
	// dialect that needed a new seam on the substrate before it could exist
	// at all. See zpty.go and interp.Runner.StartConcurrent.
	registerZptyModule(r)
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
	r.Unregister("compopt")
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
		// The state behind this shell's own `emacs` option, which its table
		// reads and writes through Runner.NamedOption exactly as `stdin`
		// below does. It was in the substrate's common table until #3366
		// took it out — BusyBox ash has no such name — so it is declared
		// here for the same reason and with the same effect: this list
		// reaches nothing but the two named-option calls setopt.go makes.
		"emacs",
		"hashall",
		"histexpand",
		"histignoredups",
		"onecmd",
		"physical",
		"pipefail",
		"privileged",
		// The state behind the `s` in `$-`, which this dialect's `shinstdin`
		// is read and written through. It is not in this shell's own `set -o`
		// roster and does not appear in one — zsh installs an option table of
		// its own, so this list reaches nothing but Runner.NamedOption and
		// Runner.ApplyNamedOption, which is exactly what setopt.go's
		// shinStdinOption calls (#3154).
		"stdin",
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
	// And the *group* id beside it, which is this shell's and not bash's.
	//
	// Measured 2026-09-25, `-f -c`, against zsh 5.9.2 on this machine, where
	// the uid and the gid differ — `uid=501 gid=20`, which is what makes the
	// row evidence rather than a coincidence:
	//
	//	zsh 5.9.2    UID=[501] EUID=[501] GID=[20] EGID=[20]
	//	bash 5.3.20  UID=[501] EUID=[501] GID=[]   EGID=[]
	//	bash 3.2.57  UID=[501] EUID=[501] GID=[]   EGID=[]
	//
	// So the pair is gated to this dialect, measured rather than assumed:
	// dialect/bash sets `UID` and `EUID` on the two lines this one models and
	// must not grow these, and `env GID=999 bash -c 'echo $GID'` writes 999
	// there because the name is nothing to that shell.
	//
	// `SetSpecial` rather than a plain store for the reason `$UID` uses it:
	// measured, `env GID=999 zsh -f -c 'print $GID'` and `env EGID=999 …`
	// both write 20, so this is a name the environment may not supply. It is
	// the same read-of-the-process the comment above blesses — a gid is
	// per-process, no script changes it, and two Runners in one program
	// genuinely have the same one.
	//
	// **Unset is not a refusal**, which is what the issue that asked for this
	// was about (#4476): a word that expands to nothing is dropped from the
	// command line rather than complained about, so `chgrp $EGID file` ran as
	// `chgrp file` and the *system's* `chgrp` wrote the `usage:` line. The
	// symptom was a third-party program's message and none of ours.
	//
	// Not marked integer or readonly, and that is measured too rather than
	// left out. `${(t)GID}` is `integer-special` there against a bare
	// `scalar` here — but so is `${(t)UID}`, and `${(t)IFS}` is
	// `scalar-special` against `scalar`, so the missing mark belongs to
	// `SetSpecial` as a whole and is filed against all four rather than
	// grown for the new pair alone. Assignment is the other half and is
	// deliberately absent: `GID=999` in an unprivileged shell is `failed to
	// change group ID: operation not permitted` and stops the shell, while
	// `GID=20` — the gid it already has — is taken at 0, so the rule is "the
	// setgid call happened", not a flat refusal. A library may not make that
	// call (see the purity rule above), and a privileged path is not
	// testable from here, so neither is guessed at.
	r.SetSpecial("GID", strconv.Itoa(os.Getgid()))
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
	// The machine's name, for `%m` and `%M`, from the same place — but by a
	// different road, because this shell puts it in a *parameter* first.
	//
	// Measured 2026-09-13 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin`, `-c`:
	// `$HOST` is the machine's name at startup, and `HOST=a.b.c.d; print -rP
	// '%m|%2m|%M'` writes `a|a.b|a.b.c.d`. So the escapes read the parameter
	// rather than asking the system each time, and the two facts are
	// separable: a shell that set `$HOST` and still asked the system would
	// answer the machine's own name to all three of those. That is what
	// SetPromptHostParameter says and what SetPromptHostFunc could not —
	// the latter keeps its first answer, which an assignable name must not.
	//
	// An inherited `HOST` wins, which is the half `SetSpecial` alone does not
	// give: it guards on the *stored* table, and the environment is a layer
	// under that one, so the name it found unstored was a name the shell had
	// been handed. Measured, `env HOST=injected.example zsh -c 'print -rP
	// "$HOST|%m"'` writes `injected.example|injected`, and this shell wrote
	// the machine's own name for both until the guard read the environment
	// too. It is an ordinary exported scalar then and an ordinary unexported
	// one otherwise — `export HOST=…` against `typeset HOST=…` — and
	// `${(t)HOST}` is a bare `scalar` in both, with none of the `special`
	// that `$UID` and `$IFS` carry. `unset HOST` therefore removes it
	// outright and `%m` draws nothing at 0.
	//
	// Not folded into SetSpecial, because the other users of that hook want
	// what it does: measured, `env UID=999 zsh -c 'print $UID'` writes this
	// uid and not 999, so `$UID` is precisely the parameter the environment
	// may not name. `HOST` is the one that may.
	//
	// Asked eagerly, where the login name beside it is deferred, and the
	// asymmetry is the point rather than an oversight. A parameter has to
	// exist before the first `$HOST` is read and there is no `%m` to hang the
	// question on, so the choice is between paying it and not having the
	// parameter. It is affordable where `user.Current` was not: `os.Hostname`
	// measures 1.5 µs on this machine against 0.83-1.10 ms for the login
	// name, three orders of magnitude apart, and #1403's zsh column was about
	// the millisecond. The issue that asked for this (#2576) quoted the
	// millisecond for the host name too; that figure is the login name's.
	if _, inherited := r.GetVar("HOST"); !inherited {
		r.SetSpecial("HOST", interp.MachineName())
	}
	r.SetPromptHostParameter("HOST")
	// And the prompt-escape table, which is the *same* value the prompt
	// drawer is handed — repl.PromptStyle is an alias for interp.PromptStyle,
	// not a copy. This is what makes `print -P '%F{196}…'` and a drawn prompt
	// one answer rather than two: before it, this shell drew `%F{196}` at a
	// prompt and refused it by name in a script (#1090).
	r.SetPromptStyle(PromptStyle())
	r.SetSpecial("EUID", strconv.Itoa(os.Geteuid()))
	// And the effective gid, the fourth of the identity parameters this shell
	// starts with. See the `$GID` line above for what was measured and for
	// why the pair stops at the value.
	r.SetSpecial("EGID", strconv.Itoa(os.Getegid()))
	r.SetDynamic("RANDOM", func(rr *interp.Runner) string { return rr.Randoms() })
	// And an assignment seeds it, which is what makes a script that uses
	// `RANDOM` reproducible: measured 2026-09-14, `RANDOM=42` twice in one
	// shell gives the same pair of numbers both times here, in ksh93u+ and
	// in zsh 5.9.2. Without the writer the assignment was heard and stored
	// for the producer to find, and the producer had no state to find it
	// with (#2827).
	r.SetDynamicWriter("RANDOM", func(rr *interp.Runner, value string) { rr.SeedRandoms(value) })
	// And the sequence a seeded `RANDOM` answers, which differs from bash's in
	// one bit of the state and from ksh93's in both halves. See random.go, and
	// `docs/spec/random.md` (#4240).
	registerRandoms(r)
	// `typeset -p RANDOM` is `typeset -i10 RANDOM=13859` here — the base
	// rides on the letter in this shell's listing form, and both are facts
	// the parameter has to be told, having no attribute record of its own
	// (#2451).
	r.SetDynamicDeclaration("RANDOM", interp.ProducedDeclaration{Integer: true, Base: 10})
	// And the third answer, which is neither a row nor a refusal: measured
	// 2026-09-12, `typeset -p LINENO` writes nothing at all and reports 0,
	// where the same probe is a row in bash and ksh93 and was
	// `LINENO: not found` here.
	//
	// The integer letter and its base ride along, because the silence is the
	// `-p` word's and the forms that do write the name want them: `typeset
	// -r` writes `integer 10 readonly LINENO=…` and `${(t)LINENO}` is
	// `integer-readonly-special` (#2552).
	r.SetDynamicDeclaration("LINENO", interp.ProducedDeclaration{Integer: true, Base: 10, Silent: true})
	// And it is read-only here, which no other column in the panel says.
	// Measured 2026-09-12, zsh 5.9.2, `env -i PATH=/usr/bin:/bin` with a
	// scratch HOME, over a script file:
	//
	//	unset LINENO       ./b.sh:2: read-only variable: LINENO, and the
	//	                   shell stops
	//	LINENO=9           ./c.sh:1: read-only variable: LINENO
	//	${(t)LINENO}       integer-readonly-special
	//
	// where bash 5.3, bash 3.2, ksh93, dash and BusyBox ash all take both
	// and leave `$LINENO` empty after the `unset` (#2519). It was an
	// ordinary produced parameter here, so a script could remove the name
	// this shell keeps counting into.
	//
	// The mark is deliberately no wider than the two writes above, and that
	// was measured before it was written rather than assumed: a *local*
	// declaration with no value is still allowed — `f() { typeset LINENO;
	// echo $LINENO; }` prints 0 in zsh 5.9.2 and the outer count is intact
	// afterwards — so a script that shadows the name in a function must go
	// on working. `local LINENO=5` is refused, which is the assignment and
	// not the declaration.
	r.MarkReadonly("LINENO")
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
	// `typeset -i10 SECONDS=0`, the same shape RANDOM lists in. The float
	// attribute a script puts on the name with `typeset -F 3 SECONDS=0` is
	// the script's own record and lists from the attribute tables as it
	// always did; this is the shape of the name nobody has declared.
	r.SetDynamicDeclaration("SECONDS", interp.ProducedDeclaration{Integer: true, Base: 10})
	// A NUL as well as the three whitespace characters, which is zsh's alone.
	r.SetSpecial("IFS", " \t\n\x00")
	// The two names the null-command options point at, which no script can
	// reach and which never move — see nullcommand.go in this package.
	registerNullCommandParameters(r)
	tieTheBuiltInPairs(r)
	// And the four scalar pairs that are one parameter under two names —
	// see promptnames.go, and the theme that could not draw without them.
	registerPromptNames(r)
	// A fifth pair, and the one whose *value* was the fault rather than the
	// tie: `histchars` and `HISTCHARS` — see histchars.go, and the pattern
	// that matched every word once the parameter behind it was empty.
	registerHistoryCharacters(r)
	if dot, ok := r.Builtin("."); ok {
		// The same function under a second name — and *not* a synonym, which
		// is the one row this shell does not share with bash. `source` looks
		// in the current directory before `$path` and `.` never looks there
		// at all, so the second name is the same builtin with one thing asked
		// of it. See interp.Runner.DotLooksInCurrentDirectoryFirst for what
		// was measured and why the difference cannot be an axis.
		r.Register("source", func(rr *interp.Runner, ctx context.Context, args []string) int {
			restore := rr.DotLooksInCurrentDirectoryFirst()
			defer restore()
			return dot(rr, ctx, args)
		})
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
	// the two shells that has the word — `add-zsh-hook` declares integers
	// before it does anything else, so a startup file that installs a
	// hook cannot run without it. Registered rather than built here, so both
	// dialects get the *same* declaration — see interp/integerbuiltin.go.
	r.Register("integer", interp.IntegerBuiltin())
	// And the assignment rule follows the name the way it follows `declare`:
	// `integer n=5+2` is a declaration's operand and not a word to split.
	r.SetDeclaring("integer")
	// `float` is the same thing one letter along, and it is this shell's
	// alone: ksh93 has the word as the preset alias `typeset -lE` and the
	// other three answer `float: not found`. It was missing outright until
	// #3291 — `float c=1.5` was `command not found` here, in a shell whose
	// own `$reswords` names the word — so `add-zsh-hook`'s neighbors in a
	// startup file had one declaration keyword of the seven that did not
	// exist.
	r.Register("float", interp.FloatBuiltin())
	r.SetDeclaring("float")
	// And the classification this shell's grammar gives all seven of them.
	// zsh's reserved-word table is not its parser's in the sense the other
	// four dialects' are: the seven declaration commands and four words the
	// rest of the panel has no construct for are in it, and `in` and `]]` —
	// which the POSIX grammar's table holds — are not. zshReservedWords is
	// that table, measured twice and already relied on by `$reswords`, so
	// the classification reads the one list rather than a second copy of it.
	r.SetReservedWords(zshReserves)
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
	// `rehash` and `unhash`, which are this shell's names for clearing the
	// command hash and for taking one entry out of a table. The behavior was
	// here — `hash -r` empties the table — and the *names* were not, so a
	// startup file that added a directory to `$path` and called `rehash`
	// died on that line at 127, which reads to a script exactly like a typo.
	// Registered rather than written as prelude functions because each
	// refuses under its own name: `rehash -x` is `bad option: -x` naming
	// `rehash`, where a function calling `hash -r` would have named `hash`.
	// See interp/rehashbuiltin.go (#3109).
	r.Register("rehash", interp.RehashBuiltin())
	r.Register("unhash", interp.UnhashBuiltin())
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
