// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"github.com/blairham/sh/syntax"
)

// Dialect is what zsh parses.
func Dialect() syntax.Dialect {
	d := syntax.Core()
	// A replacement's `/` is found without regard to quotes. See the field.
	d.ReplacementSlashIgnoresQuotes = true
	// `$[expr]`, zsh's other arithmetic spelling and the older one.
	// Measured 2026-09-06: `echo $[1+1]` is 2, and the same text is
	// the literal `$[1+1]` in ksh93 and dash. Reading it as a glob is
	// what made the failure `no matches found`, which points a
	// person at globbing rather than at arithmetic (#900).
	d.DollarBracketArith = true
	// `${!name}` is refused **past the parse** here, which is where this
	// shell refuses every other expansion it cannot read: measured with
	// `set -n`, `echo "${!x}"` is `bad substitution` and
	// `{ echo "${!x}"; }` is silent at 0, exactly as `${9nope}` is in both
	// positions. Without this the refusal ended the parse and a `${!x}` in
	// a branch nothing takes was a dead script. See
	// syntax.Dialect.IndirectionRefusedAtExpansion, and
	// emulateindirection.go for the mode that gives the construct a meaning
	// rather than a refusal.
	d.IndirectionRefusedAtExpansion = true
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
	// And blanks may stand between the `{name}` and its operator, among a
	// command's arguments: `echo {v} >/dev/null` allocates (#6587). See
	// syntax.Dialect.FdVariableAcrossBlanks.
	d.FdVariableAcrossBlanks = true
	// And in front of `&>` and its family, and `>>(` / `<<(` are a
	// one-character operator before a process substitution, measured
	// 2026-10-15 on 5.9.2 (#6665). See syntax.Dialect.FdVariableBeforeBothStreams
	// and syntax.Dialect.DoubledAngleOpensAProcessSubstitution.
	d.FdVariableBeforeBothStreams = true
	d.DoubledAngleOpensAProcessSubstitution = true
	// A plain redirection onto a process substitution running the other
	// way is refused while reading: `echo hi > <(cat)` is ``parse error near
	// `<(cat)'`` (#6812). See
	// syntax.Dialect.ProcessSubstitutionAgainstAPlainRedirectionIsRefused.
	d.ProcessSubstitutionAgainstAPlainRedirectionIsRefused = true
	// And refuses the construct after a compound command, as a parse error
	// naming either the brace or the whole word by the compound's kind. See
	// the flag for the table.
	d.FdVariableRefusedAfterACompound = true
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
	// zsh has all five, like bash, and a sixth the others have not.
	//
	// `private` is a declaration word in the grammar and not only at the
	// store: measured 2026-09-27 on zsh 5.9.2 from a script file under
	// `env -i PATH=/usr/bin:/bin`, behind `zmodload zsh/param/private`,
	//
	//	private q=(1 2)        array-local-hide-special, two elements
	//	private -a q=(1 2)     the same
	//	private -A m=(k v)     association-local-hide-special, ${m[k]} is v
	//	private -a q=()        array-local-hide-special, empty
	//	local -P q=(1 2)       array-local-hide-special, two elements
	//	private topq=(1 2)     array, at the top level
	//	'private' q=(1 2)      unknown file attribute: 1 — a glob qualifier,
	//	                       exactly as `'typeset' a=(x y)` is
	//
	// The last row is why the word belongs in this map rather than in a
	// rule of its own: it answers
	// [syntax.Dialect.DeclarationArrayFromTheCommandWord] the way every
	// other declaration word in this shell answers it.
	//
	// **Unconditional, where the reference gates it on the module.** Without
	// `zmodload zsh/param/private` the reference reads `private q=(1 2)` as
	// a glob qualifier too, because the word is not reserved until the
	// module is loaded. That is the same one-state-for-two divergence
	// interp/privatebuiltin.go records for `whence -w private` and for
	// `local -P`: this shell registers the word from the start and does not
	// autoload, so the grammar cannot be gated on a load that never happens.
	// A script that writes the `zmodload` — which is every script the word
	// was measured through — sees the same shell either way.
	//
	// And the two numeric words, which take an array literal as an operand
	// exactly as `typeset` does: measured 2026-10-15, `integer v=(1+1 2)` and
	// `float v=(1)` are `inconsistent type for assignment` — the literal
	// read and refused — where reading `(1 2)` as a scalar wrote `bad math
	// expression` (#6566).
	d.DeclarationUtilities = map[string]bool{
		"declare": true, "typeset": true, "local": true,
		"export": true, "readonly": true, "private": true,
		"integer": true, "float": true,
	}
	// The reserved-word declarations, which `private` is not: it is the
	// module's builtin. Measured 2026-10-02 through `which` (#5138).
	d.DeclarationReservedWords = map[string]bool{
		"declare": true, "export": true, "float": true, "integer": true,
		"local": true, "readonly": true, "typeset": true,
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
	// The lexical half of the same rule, which `ignoreclosebraces` keeps
	// after taking the reserved reading away: a `}` that ends a word ends it,
	// so `print a}` is two arguments under that option rather than one. See
	// syntax.Dialect.CloseBraceEndsAWord (#5011).
	d.CloseBraceEndsAWord = true

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
	// And the two body spellings that are not a brace group — one command, or
	// none at all. They are a flag of their own because `shortloops` moves
	// them and leaves the brace group alone: with the option off `while
	// (( 0 )) { :; }` still parses here and `while (( 0 )) :` does not,
	// measured 2026-09-27 under `set -n` in all three emulations. On at
	// construction because the option is on by default; see setopt.go, where
	// the option is what moves it.
	d.ShortFormBody = true
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
	// And the end of a `-c` string or a script file ends one too, where a
	// prompt asks for more. Measured 2026-10-03. See
	// syntax.Dialect.OpenEndedAndOrAtTheEndOfInput, which says why standard
	// input is not in the set.
	d.OpenEndedAndOrAtTheEndOfInput = syntax.RouteFromCommandString | syntax.RouteFromScriptFile
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
	// So are the control bytes, every one but tab and newline, which are
	// blanks to the reader. Measured 2026-10-16 on zsh 5.9.2 through `let`
	// and `[[ … -eq … ]]`: `let $'1 \x01'`, `$'1\x01'` and `$'\x01'` are
	// `illegal character: ^A`, and the same for `\x7f` (`^?`), `\r`, `\x0b`
	// and `\x1b`; `let $'1+\x01'` keeps ``operand expected at `^A'`` and
	// `let $'(1\x01)'` is `')' expected`, the two positions every byte here
	// answers that way at (#6723). The caret spelling is the diagnostic's
	// visible rendering, which reaches this sentence like any other.
	d.ArithBytesRefusedOutright = "'@;{}\\]" +
		"\x01\x02\x03\x04\x05\x06\x07\x08\x0b\x0c\x0d\x0e\x0f" +
		"\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x7f"
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
	// And a `$( … )` body the grammar refuses **at a token** settles the read:
	// no closing parenthesis is looked for, so the commands written before the
	// substitution on the same line never run.
	//
	// Measured 2026-09-27 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
	// `/opt/homebrew/bin/zsh`, `-f` over a script file holding
	// `echo b; v=$(X); echo a`, under `set -n` and run:
	//
	//	X          refused with the line   `b` written first
	//	&&         yes                     no
	//	;;         yes                     no
	//	fi         yes                     no
	//	done       yes                     no
	//	esac       yes                     no
	//	echo hi    no                      yes — nothing is wrong with it
	//
	// This shell wrote `b` for all six: the body was read when the
	// substitution was expanded, which is after the commands in front of it
	// on that line have run. The wording and the status were already this
	// shell's — both complaints, at both lines — so what was wrong was only
	// **when** the refusal was charged.
	//
	// It is not [syntax.Dialect.SubstitutionBodyRead] moving with it. That
	// answers when the body is *parsed*, and a body with nothing wrong in it
	// is still parsed here when it runs: `$(if)` writes `b` and then fails at
	// the substitution in the reference, which is the zsh column the corpus
	// records for `subst/a-body-that-will-not-parse-stops-the-line`.
	d.SubstitutionBodyRefusalEndsTheRead = true
	// And the one shape carved back out of the closer's own carve-out: a body
	// whose read stopped at the closing parenthesis with an `if` still short
	// of its `then`. This shell's brace-bodied `if` is what closes the
	// construct there, so with that option on the parenthesis is the
	// substitution's and the body waits for the moment it runs — and with the
	// option off the same body settles the read. `$(if)` parses here and
	// `$(if true; then)` does not, measured 2026-09-27 under `set -n`; the
	// whole grid is on the field.
	//
	// Off at construction because `shortloops` is **on** by default, which is
	// the state this places: see setopt.go, where the option is what moves it.
	d.SubstitutionBodyRefusesAnUnfinishedCondition = false
	// And a *regular expression's* group is the same construct: a `;`, `<`,
	// `>` or `&` inside `(…)` stops the group here where the four shells with
	// `=~` all keep it. Measured 2026-09-27 on zsh 5.9.2 under `set -n` —
	// `[[ "a<b" =~ (a<b) ]]` and the rows for the other three are refused,
	// and `(a b)` and `(a|b)` are taken, which is the same pair of exceptions
	// the line above records for a pattern group. Until this the corpus row
	// `cond/a-regex-operands-group-keeps-its-operators` was answered here
	// with the reading the other four have.
	d.RegexGroupEndsAtAShellOperator = true
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
	// And the whole list written as nothing, which is a separate answer: the
	// option that takes this one away leaves the empty alternative standing.
	// See syntax.Dialect.CasePatternListMayBeEmpty.
	d.CasePatternListMayBeEmpty = true
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
	// And where an operand belongs: `[[ -n x` and `-z "" ]]` on the next line
	// is one term with three operands, refused when it runs as `unknown
	// condition: -n`. See syntax.Dialect.ConditionNewlineMayPrecedeAnOperand.
	d.ConditionNewlineMayPrecedeAnOperand = true
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
	// And the keyword spelling's own list, which the option that moves the
	// one above leaves alone — see syntax.Dialect.FunctionKeywordNameList.
	d.FunctionKeywordNameList = true
	// And a name list with no body after it, plus the `;` that may stand
	// between the names and a body it does have — see
	// syntax.Dialect.FunctionKeywordBodyIsOptional for the run that says a
	// bodyless declaration defines an empty function rather than an autoload
	// stub, and that `function a; echo B` binds `echo B` as the body (#1686).
	d.FunctionKeywordBodyIsOptional = true
	// And the separator in front of a body, which the option that takes the
	// bodyless declaration away leaves standing — see
	// syntax.Dialect.FunctionKeywordSeparatorBeforeBody.
	d.FunctionKeywordSeparatorBeforeBody = true
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
	// And the square brackets have to balance at the `))`, or it is two
	// subshells. See Dialect.ArithBracketsMustBalance (#5378).
	d.ArithBracketsMustBalance = true
	// An operand where a conditional's `:` belonged is blamed as an
	// operand. See Dialect.ArithConditionalThenLeftoverIsAnOperand (#5378).
	d.ArithConditionalThenLeftoverIsAnOperand = true
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
	d.ArithSpecialParameterOperands = true
	// Its operand is one character, and `\C` and `\M` reach past themselves
	// to take it: measured 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0)
	// run `-f`, `$(( ##\C-a ))` is 1, `$(( ##\M-a ))` is 225 and
	// `$(( ##\M-\C-a ))` is 129 — the separating `-` optional in both and
	// either able to take the other as its argument. The span is what decides
	// whether the operand is complete at all, which is why it is written here
	// and not only where the escape is decoded: reading `\C` alone left `-a`
	// standing and answered 0 (#4607).
	d.ArithCharacterEscapes = syntax.ArithCharacterEscapesMaskedCaretMeta
	// And *both* branches of `c ? t : e` are read at the conditional level,
	// so a bare assignment cannot begin either: measured 2026-09-26 on zsh
	// 5.9.2, `$(( 1 ? x = 2 : 3 ))` is `':' expected` at 1 and
	// `$(( 1 ? 2 : x = 3 ))` is `lvalue required`, where
	// `$(( 1 ? (x = 2) : 3 ))` is 2. bash 5.3.20 and ksh93u+ answer 2 to the
	// first, which is why this is one column's grammar and not a shared
	// refusal (#4680).
	//
	// The then level is this column's alone, and the sequence operator is
	// what shows it: `$(( 1 ? 2 , 3 : 4 ))` is `':' expected` here and 3 in
	// bash, ksh93 and BusyBox ash, which read that branch at the loosest
	// level C's grammar gives it. Measured 2026-09-27 (#4776).
	d.ArithConditionalThenLevel = syntax.ArithConditionalBranchConditional
	d.ArithConditionalElseLevel = syntax.ArithConditionalBranchConditional
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
	// And the same operators spelled with the ampersand last: `>>&` appends
	// both streams, and `>&|`, `>&!`, `>>&|` and `>>&!` are the marked
	// forms. Each is its `&>` twin row for row, measured; what tells the
	// family from `>&` is that the target is always a name, so `>>&2` writes
	// a file called `2` where `>&2` duplicates the descriptor.
	d.ReversedAmpersandRedirect = true
	return d
}
