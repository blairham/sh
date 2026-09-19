// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package ksh answers the substrate's questions the way ksh93 does.
package ksh

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what ksh93 parses.
func Dialect() syntax.Dialect {
	d := syntax.Core()
	// ksh93 expands them in a script too, on every route and with no option
	// to turn on.
	// A `:` written straight after the substring's own belongs to the offset
	// expression here, so `${x::2}` is the expression `:2` and a refusal
	// rather than an offset of nothing (#2818).
	d.ParamSubstringOffsetTakesALeadingColon = true
	d.AliasesExpandUnlessTold = true
	d.ExpandAliasesInProgramText = syntax.RouteOnEveryRoute
	// And a body's newlines are lines of the program: `$LINENO` after a
	// two-line body reads one more than the physical line.
	d.AliasBodyCountsLines = true
	// A backslash an alias body ends with reaches the newline after the
	// alias word, where it is an ordinary line continuation and joins the
	// next line to the word. zsh and bash 3.2 are the columns that do not.
	// See syntax.Dialect.AliasBodyBackslashJoinsTheNextLine (#2710).
	d.AliasBodyBackslashJoinsTheNextLine = true
	// A reserved word an alias supplied is still the reserved word behind an
	// assignment prefix, where a written-out one is an ordinary name there:
	// `alias g="{ :; }"; v=x g` quotes the `{` and `v=x { :; }` quotes the
	// `}`. See [syntax.Dialect.AliasedReservedWordStandsBehindAnAssignmentPrefix]
	// for the five rows (#2888).
	d.AliasedReservedWordStandsBehindAnAssignmentPrefix = true
	// A subscript written at command position runs to its matching `]`:
	// `m[foo bar]=v` is the element keyed `foo bar`, read back here as
	// `typeset -A m=(['foo bar']=v)`. Measured 2026-09-12 on 93u+ beside
	// bash, against zsh 5.9.2, which refuses the text. See
	// [syntax.Dialect.SubscriptSpansSeparators].
	d.SubscriptSpansSeparators = true
	// And every quoting construct holds a `]` back from ending a `${a[ … ]}`
	// subscript on the way back out, `$'…'` included — the widest row of the
	// panel, and the one cell where this shell and bash part. Measured
	// 2026-09-15 on 93u+ 2012 with `a=(9 8 7); echo "[${a[$'0]'+1]}]"`: an
	// arithmetic error naming `0]+1` here, and the text `[1]]` in bash, whose
	// subscript ended at the bracket inside the quotes. See
	// [syntax.Dialect.SubscriptQuoteProtectsTheClosingBracket] (#2299).
	d.SubscriptQuoteProtectsTheClosingBracket = syntax.SubscriptBackslashQuotes |
		syntax.SubscriptSingleQuotes | syntax.SubscriptDoubleQuotes |
		syntax.SubscriptDollarSingleQuotes
	// And a compound literal is one shape or the other rather than a
	// mixture, which is what says where a subscript may be read at all here:
	// `a=([1]=A [2]=B)` is subscripted throughout, `a=(p [1]=A)` is a word
	// list and keeps the brackets, and `a=([1]=A p)` is a syntax error
	// naming `p`. The first element decides. Measured 2026-09-14 on 93u+;
	// see [syntax.Dialect.ArrayLiteralShapeFollowsTheFirstElement] for the
	// whole table and for the store rule that rides beside it (#2505).
	d.ArrayLiteralShapeFollowsTheFirstElement = true
	// A name on the left of an assignment may carry more than one subscript,
	// and the later ones reach *into* what the earlier one named:
	// `a[1][2]=v` is `typeset -a a=([1]=([2]=v) )` with one element, which is
	// the same value `a[1]=([2]=v)` builds. Measured 2026-09-14 on 93u+; the
	// other columns refuse the operand or declare an empty array under the
	// base name, and this is the grammar for the one that nests (#2491).
	d.ChainedAssignSubscript = true
	// And the read half of the same nesting: `${a[1][2]}` was a parse error
	// here while the write built a value only `typeset -p` could show. The
	// grammar is shared with the other shell that writes the text and the
	// *reading* is not — see Semantics.ChainedSubscriptReadsANestedValue
	// (#2830).
	d.ChainedSubscript = true
	// `${a[lo..hi]}` is a range of elements rather than one arithmetic
	// subscript: `a=(a b c d e); ${a[1..3]}` is `b c d`, three fields when
	// quoted. bash and zsh hand the same text to the arithmetic and refuse
	// it. Measured 2026-09-16 on 93u+; the rules for what the range names are
	// in interp/dotrange.go (#3408).
	d.SubscriptDotRange = true
	// A backslash the input ends immediately after is kept only where it is
	// the first thing in the word: `printf "[%s]" \` is `[\]` here and
	// `printf "[%s]" x\` is `[x]`, where bash 5.3 keeps both and zsh drops
	// both. Measured 2026-09-13; see [syntax.EndOfInputBackslash] (#2680).
	d.BackslashAtEndOfInput = syntax.EndOfInputBackslashIsLiteralOnlyAtAWordStart
	// And in a redirection's target, where the panel splits three-to-one
	// against it: `> m[foo bar] echo hi` writes one file here and two words
	// in every bash. Measured 2026-09-13 on 93u+; see
	// [syntax.Dialect.SubscriptSpansSeparatorsInRedirect] for the positions
	// it does and does not reach — `echo hi > m[foo bar]` splits, because an
	// argument stood in front of the redirection.
	d.SubscriptSpansSeparatorsInRedirect = true
	// ksh93 has neither `local` nor `declare`, so `local a=(x)` is the same
	// syntax error there that `echo a=(x)` is — the rule follows the name
	// into the shell that has it.
	d.DeclarationUtilities = map[string]bool{
		"typeset": true, "export": true, "readonly": true,
	}
	// And the words that open a *compound variable's* body when they stand
	// first inside `c=( … )`. A superset of the utilities above, because this
	// shell spells three of `typeset`'s letters as words of their own —
	// `integer` is `typeset -l -i`, `float` is `typeset -l -E`, `compound` is
	// `typeset -C` — and `nameref` is `typeset -n`. `declare`, `local`, `set`
	// and every ordinary builtin are *not* in it: `c=(declare x=1)` is an
	// indexed array of the two words there, which is what says the set is
	// closed rather than "anything that looks like a command". Measured
	// 2026-09-13 on 93u+; the rows are in
	// [syntax.Dialect.CompoundVariableDeclarators] (#2620).
	d.CompoundVariableDeclarators = map[string]bool{
		"typeset": true, "export": true, "readonly": true,
		"integer": true, "float": true, "compound": true, "nameref": true,
	}
	// A `!` with no pipeline after it is a pipeline of its own here, and this
	// shell is the union of the other two answers: it reaches a statement
	// terminator and a `&` the way bash does, and every closer and an and-or
	// operator the way zsh does. Measured 2026-09-12 over all ten positions
	// (#948).
	d.BareNegationReach = syntax.BareNegationAtEitherPlace
	// And a second `!` inverts the first, as in bash: `! ! true` answers 0.
	d.RepeatedNegationToggles = true
	// A process substitution stands only where a command takes a word — an
	// argument, or a file redirection's target. In a `[[ ]]` operand, a
	// `case` subject or pattern, a loop header's list, an array literal or a
	// here-string it is `` `<(' unexpected `` while reading, before anything
	// in the input has run. See
	// syntax.Dialect.ProcessSubstitutionOnlyWhereACommandTakesAWord for the
	// rows, and for why this is a grammar flag and not the condition axis
	// that answers zsh (#930).
	d.ProcessSubstitutionOnlyWhereACommandTakesAWord = true
	d.ParamIndirection = true
	// A C-style `for` header may hold more than the two separators its three
	// expressions need, as in zsh: the leftover text belongs to the third
	// expression and is refused as arithmetic if the loop ever evaluates it,
	// so `for ((;;;)); do echo body; break; done` prints `body` here and
	// `for ((;;;)); do :; done` stops on its first pass with an arithmetic
	// complaint. bash refuses the whole script instead. Measured 2026-09-12
	// across the panel (#2225).
	d.ForArithExtraSeparators = true
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
	d.SeparatorWhereACommandBelongs = syntax.OneSeparatorExceptAfterABarOrBeforeACondition
	// And the one place that reading does not reach: the body of a `$( … )`
	// or a `${ …;}`, where only an and-or's missing operand still takes one.
	// Measured 2026-09-17 over fourteen shapes — `echo a; ;` runs at the top
	// level and inside backquotes and is `` `;' unexpected `` inside either
	// newer spelling, while `v=$(false || ; echo b)` runs and leaves `b`
	// (#3333).
	d.SubstitutionBodyRefusesASteppedOverSeparator = true
	// And where a separator was stepped over and nothing came after it at
	// all, an empty command stands there and succeeds: `false || ;` answers
	// 0 here and 1 in zsh, which drops the operator instead. The `&&` row
	// agrees in both — 1 either way, because the left-hand side failed — so
	// the `||` is the only shape that tells the two readings apart.
	d.AbsentAndOrOperandIsAnEmptyCommand = true
	// And where a separator was stepped over and the construct closed right
	// after it, that separator is the whole body: `{ ; }`, `( ; )`,
	// `if :; then ; fi`, `while :; do ; done` and the rest all run here. The
	// control is `{ }`, which is refused — so this is not zsh's
	// EmptyCompoundBody, which takes both — and `{ ; ; }`, refused by the
	// count above (#775, #2231).
	d.SteppedOverSeparatorIsABody = true
	// An `&` standing where a body or a condition must have something in it
	// is refused at the token *after* it, where the rest of the panel names
	// the `&`. Measured 2026-09-14 with `-n` over a script file: `if & then
	// :; fi` is `` `then' unexpected ``, `while & do :; done` is `` `do' ``,
	// `if & fi` is `` `fi' ``, `{ & }` is `` `}' ``, `( & )` is `` `)' ``,
	// `case x in x) & ;; esac` is `` `;;' `` and `if & ; then :; fi` is
	// `` `;' `` — the next token whatever kind it is, with nothing stepped
	// over. It is the reverse of what the same shell does with a `;` there,
	// which names the `;` (#2023), so whatever it does with an empty command
	// before a terminator is not one rule over both of them.
	//
	// The set is `&` alone. `|`, `|&`, `&&`, `||`, `;;` and `;&` in the same
	// positions all name themselves here (#2235).
	d.EmptyBodyBlame = syntax.BlameTheTokenAfterIt
	d.EmptyBodyBlamed = map[syntax.Kind]bool{syntax.TokAmp: true}
	// `esac` written straight after a `case`'s `in`, with no newline between
	// them, is the first arm's pattern here rather than the terminator:
	// `case esac in esac) echo hit;; esac` prints `hit`, and `case x in esac`
	// on one line is refused as a consequence. A newline gives the word back
	// its reservation — `case x in⏎esac` runs — which is what says this is
	// not "a case must have an arm" (#773).
	d.CaseTerminatorIsAPatternAfterTheHeader = true
	// A single `;` may end an array literal's element list: `a=( x; )` and
	// `a=( x y; )` run here, and so do `a=( x ⏎ ; )` and `a=( x; ⏎ )`. It is a
	// terminator rather than a separator, and every part of that is measured:
	// `a=( x; y )` is `` `y' unexpected ``, so it does not stand between two
	// elements; `a=( ; )` is `` `;' unexpected ``, so it needs one in front of
	// it; and `a=( x; ; )` is `` `;' unexpected ``, so it may be written once.
	// zsh takes all four, which is the reading the other value records (#1162).
	d.SemicolonInAnArrayLiteral = syntax.OneSemicolonEndsTheArrayElements
	// And this shell has the brace spelling of a `case` header too, with the
	// two words **paired**: `case x { … }` and `case x in … esac` run, while
	// `case x { … esac` and `case x in … }` are both `` `case' unmatched ``.
	// zsh takes all four, which is the split the spelling records. The other
	// three rows that differ between the two are other flags showing through:
	// `case x {x)` needs the blank here because OpenBraceNeedsNoBlank is off,
	// `case x { x) echo hit }` needs the `;;` because CloseBraceAlwaysReserved
	// is, and `case esac { esac) … }` prints `hit` because the rule above
	// follows the header's position rather than the word `in` (#1928).
	d.CaseBraceBody = syntax.CaseBraceBodyPairsWithItsOpener
	// A loop's variable may be written with quoting or an escape in it, and
	// the quoting comes off before the word is read as a name: `for "i" in a
	// b` binds `i` here and is refused by the other four. `for 'i'`,
	// `for i""`, `for "i"x` and `for \i` too, so the escape travels with the
	// quotes. A name out of an *expansion* is refused here as everywhere,
	// which is why this is the quoting half alone (#1076).
	d.ForNameMayBeQuoted = true
	// And the same removal in front of a function definition's parentheses:
	// `'q'() { echo q; }` defines and calls `q` here, where this parser
	// refused the line at the `(`. The quoting is the whole of it and the
	// name is read *expanded* — `a\ b() { :; }` is `a b:
	// invalid function name`, naming the two words and not the backslash, so
	// the check the definition then meets is the one it already had. Measured
	// 2026-09-10: zsh and this shell take the quoted name where the three
	// bash columns refuse the word as written and dash refuses to parse it,
	// which is a two-against-four split and not the one-against-five a name
	// holding a space gives. See syntax.Dialect.FunctionNameIsAnyWord for the
	// six columns (#1561).
	d.FunctionNameIsAnyWord = true
	// And a word that is not a name at all parses, the complaint coming when
	// the loop is reached — so `ksh -n` accepts a script this used to refuse.
	// interp.Semantics.ForNameWhenTheLoopRuns is what happens then (#1110).
	d.ForNameCheckedWhenTheLoopRuns = true
	// The same stage for a function definition's name: `function _p_${w} { …
	// }` parses and the complaint — `_p_${w}: invalid function name`, naming
	// the word as it was written — comes when the definition is reached, and
	// stops the script. interp.Semantics.FunctionNameWhenTheDefinitionRuns is
	// what happens then (#1296).
	d.FunctionNameCheckedWhenTheDefinitionRuns = true
	// And the same stage for a name this shell keeps for itself. A function
	// may not be named after a *special* builtin here — `export() { :; }` is
	// `export: invalid function name` and the script stops at 1 — and the
	// set is this shell's own rather than POSIX's list: `times` is a preset
	// alias here rather than a builtin and is an ordinary name, while
	// `alias`, `enum`, `hash`, `login`, `newgrp`, `typeset` and `unalias`
	// are special and are refused. Regular builtins are not: `true`, `read`
	// and `cd` all define. `.` and `:` are special too and are left out,
	// because PunctuatedFunctionNameIsRefused already refuses them and says
	// so in this shell's own two wordings — a dot names a discipline
	// function here, which is a different complaint. Both spellings of a
	// definition reach the set, and the stage is what
	// separates this from dash — the `printf` in front of the definition
	// runs, a definition in a branch nothing takes is never refused, and one
	// inside a subshell ends the subshell alone. Measured 2026-09-15 on all
	// three routes; see syntax.Dialect.FunctionNamesRefused (#2932).
	// A `]]` standing where a condition **term** belongs is an ordinary
	// word here, so the closer is only a closer once the condition has
	// something to close over. Measured 2026-09-18: `[[ ]] ]]` is 0, the
	// two-character word being non-empty; `[[ ]] == x ]]` is 1, comparing it
	// with `x`; and `[[ ]]` alone is `` `[[' unmatched `` at the end of the
	// input, the condition never having closed. bash and zsh refuse the
	// token in every one of those. An **operand**'s position is a separate
	// question and this shell answers it the way they do — `[[ -n ]]` and
	// `[[ x == ]]` are `` `]]' unexpected `` here too (#2964).
	d.ConditionCloserIsAWordWhereATermBegins = true
	d.FunctionNamesRefused = map[string]bool{
		"alias": true, "break": true, "continue": true, "enum": true,
		"eval": true, "exec": true, "exit": true, "export": true,
		"hash": true, "login": true, "newgrp": true, "readonly": true,
		"return": true, "set": true, "shift": true, "trap": true,
		"typeset": true, "unalias": true, "unset": true,
	}
	// And a definition keeps the characters it was written with, because this
	// shell's `typeset -f` says them back rather than laying the tree out:
	// `f(){    echo     a   ;   }` lists with every one of those blanks, a
	// comment inside the body survives, and the listing ends with the `;`
	// that ended the statement. See syntax.FuncDecl.SourceText for the
	// measurement and Diagnostics.FunctionListingIsSourceText for the half
	// that writes it (#2610).
	d.FunctionDefinitionIsSourceText = true
	// A colon written before a trim is ignored here: `${v:#hel*}` is
	// `${v#hel*}` and comes to `lo`, where zsh reads the same six characters
	// as an element exclusion and bash refuses them as arithmetic. All four
	// trims, measured — `:#`, `:##`, `:%` and `:%%` — and nothing else: the
	// replacement, the case changes and a colon with a space after it stay
	// arithmetic errors, and `${v:2}` is still an offset.
	d.ParamColonBeforeTrimIsIgnored = true
	// A bare `{` inside an unquoted `${…}` opens a nesting level here, the
	// same as in zsh and unlike the three bash-family columns. Measured
	// 2026-09-10 with `unset u; printf "[%s]" ${u:-{a,q}.z}`: this shell
	// answers `[a.z][q.z]`, so the operand ran to `.z` and the group was
	// then expanded, where bash 5.3, bash 3.2 and dash answer the single
	// field `[{a,q.z}]`.
	d.BareBraceNestsInExpansion = true
	// A line continuation inside `${ }` is removed only once a name has
	// begun: `${x\⏎}` is the value and `${\⏎x}`, `${#\⏎x}`, `${1\⏎}` and
	// `${@\⏎}` are ``syntax error at line 1: `\' unexpected`` here, where the
	// other four read them all. See the flag for the rows.
	d.ParamContinuationNeedsAName = true
	// A line continuation written between a `$` and what it introduces stops
	// the `$` at a bare parameter and at a parenthesis outside quotes, and at
	// everything inside double quotes. Measured 2026-09-16 from script files
	// with `x=5` and `set -- a b`: `$\⏎x` is the text `$x` here where the
	// other four expand it, `$\⏎(echo hi)` is `` `(' unexpected `` because
	// the `$` stayed behind as text and a `(` cannot begin a word, and
	// `"$\⏎{x}"` and `"$\⏎1"` are `${x}` and `$1`. The two it does not stop
	// at outside quotes are the brace and `$'…'`: `$\⏎{x}` is 5 and
	// `$\⏎'a\tb'` decodes, both as in the rest of the panel. `$[…]` is not
	// a form this shell has, so nothing here is a measurement of it.
	//
	// One shape is measured and deliberately not modeled: outside quotes a
	// *pattern* character standing earlier in the same word takes the stop
	// away, so `[$\⏎x]`, `{$\⏎x`, `*$\⏎x` and `?$\⏎x` all expand here
	// while `a$\⏎x` and `!$\⏎x` do not. Reading it would make the lexer's
	// `$` depend on glob characters it has already passed, which is a
	// question about the *word* and not about the delimiter this field is
	// for. See #3523.
	d.ContinuationStopsADollarAt = syntax.DollarBareParameter | syntax.DollarParens
	d.ContinuationStopsADollarAtInDoubleQuotes = syntax.EveryDollarForm
	// A line continuation between either pair of an arithmetic expansion's
	// delimiters parts them here, so the construct is a command substitution
	// holding a subshell. Measured 2026-09-16 from script files: both
	// `echo "[$(\⏎( 1 + 2 ))]"` and `echo "[$(( 1 + 2 )\⏎)]"` say `1: not
	// found` and expand to nothing, where bash 5.3, bash 3.2 and dash read
	// both as arithmetic and answer 3.
	d.ContinuationPartsTheArithmeticOpener = true
	d.ContinuationPartsTheArithmeticCloser = true
	// And a continuation directly behind the `((` of an arithmetic *command*
	// opens nothing at all here: the two parentheses and the pair are
	// consumed and no command is produced, so `echo pre⏎((\⏎echo hi⏎echo b`
	// prints all three lines at status 0 and `false⏎((\⏎echo "rc=$?"` prints
	// `rc=1`. See the flag for the refusals that shape produces, each of
	// which is the text that followed being read on its own.
	d.ContinuationEndsTheArithmeticCommandOpener = true
	// The scan that looks for an arithmetic command's `))` is blind to
	// quoting here too: `((echo "a)b"))` and `((echo 'a)b'))` print `a)b` as
	// two groupings. One row of that measurement is left standing —
	// `((echo "(" ))` is `` `"' unmatched `` at 3 in this shell and two
	// groupings printing `(` here — because it is a fact about where that
	// shell resumes after giving the reading up rather than about the scan.
	// See the flag for the five rows.
	d.ArithCommandScanIgnoresQuoting = true
	// A `name=( … )` operand keeps the array reading through *quoting* of the
	// command word here: `'typeset' a=(x y)`, `\typeset a=(x y)` and
	// `type"set" a=(x y)` all set the array, where bash 5.3 refuses each as a
	// syntax error and zsh 5.9.2 reads a glob qualifier on the word `a=`.
	// Measured 2026-09-16 from script files. An *expansion* takes it away
	// here too — `cmd=typeset; $cmd a=(x y)` is `` `(' unexpected `` — which
	// is the line between this reading and the core's.
	d.DeclarationArrayFromTheCommandWord = syntax.DeclarationArrayFromAWrittenWord
	// A function body that is not compound may carry no redirection here:
	// `f() echo hi` runs and `f() >out`, `f() echo hi >out` and `f() x=1
	// >out` are all a syntax error at the operator. A braced body is not
	// this rule — `f() { :; } >out` is accepted.
	d.FuncBodyTakesNoRedirection = true
	// The `function` keyword's body is a brace group here and nothing else.
	// Measured 2026-09-11 over a file holding `function a`, the body and a
	// call: `echo B` is ``syntax error at line 2: `echo' unexpected``,
	// `(( 1 ))` is the same sentence at `((`, `( echo B )` at `(` and a `for`
	// loop at `for`, all at status 3, where `{ echo B; }` runs. The
	// parenthesized form is the rule above and is *wider* than this one, so
	// the two spellings need two flags.
	d.FunctionKeywordBodyMustBeBraceGroup = true

	// Words after the keyword's name are a list of name references: they are
	// read and discarded, so `function a b { … }` defines `a` alone and
	// leaves `b` not found. See
	// syntax.Dialect.FunctionKeywordReferenceList (#2014).
	d.FunctionKeywordReferenceList = true
	// `$"..."`, the locale-translatable string: with no catalog it is a
	// plain double-quoted string with the `$` stripped. Not core, because
	// dash and zsh keep the `$` as a literal.
	d.DollarDoubleQuote = true
	// ksh93 alone refuses an unrecognized ${...} operator while reading the
	// script; the other three wait until the expansion is reached.
	d.BadSubstitutionAtParseTime = true
	// A `.` is a name character here, which is how `${.sh.version}` is
	// spelled and how a compound variable's member is addressed. One rule
	// covering both — see syntax.Dialect.DottedName for the probes (#2620).
	d.DottedName = true
	// `~(E)pat` and its family: a `(` after a `~` belongs to the word here,
	// which is what makes this shell's only spelling for a regular
	// expression writable. See syntax.Dialect.TildeGroup (#2621).
	d.TildeGroup = true
	// `${ cmd;}`, a command substitution that runs in the current shell
	// so that what it assigns survives. The space after the brace is the
	// whole of the grammar: `${x}` is a parameter and `${ x}` is not.
	d.CurrentShellSubstitution = true
	// And that body ends at a `}` which begins a *token*, in argument
	// position as well as command position, which is where this shell parts
	// from the other one that has the construct: `${ echo } ;}` is refused
	// here and hands `echo` a literal brace there. See
	// syntax.Dialect.BraceProgramBodyEnd for the seven rows (#2724).
	d.BraceProgramBodyEnd = syntax.BraceProgramBodyEndsAtATokenStart
	// `${(list)}`, whose body is the parenthesized subshell and whose `}` has
	// to sit directly behind the matching `)`. Not the form above with a `(`
	// for an opener: `${(echo a); echo b;}` is refused while reading here and
	// the blank spelling of the same body runs both commands. See
	// syntax.Dialect.SubshellSubstitution for the six rows (#2615).
	d.SubshellSubstitution = true

	// A flag group written for another shell is refused when the word
	// expands here, where every other unreadable expansion is refused while
	// the line is read. Measured 2026-09-18 on ksh93u+ 2012-08-01, a script
	// file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on
	// /dev/null, each body as `echo one; echo "[<body>]"; echo two`:
	// `${(f)x}`, `${(qq)x}` and `${(j:|:)x}` all write `one` first and then
	// refuse at 3, while `${%%%}`, `${~x}`, `${=x}` and `${+x}` refuse
	// before `one` runs. `if false; then echo "${(f)x}"; fi` writes nothing
	// and exits 0, where the same line with `${%%%}` in it is still refused
	// — which is what says the deferral reaches the *expansion* and not
	// merely the line. See syntax.Dialect.FlagGroupRefusedAtExpansion
	// (#3013).
	d.FlagGroupRefusedAtExpansion = true
	// `${((expr))}`, a second spelling for an arithmetic expansion — and not
	// the form above with a subshell inside it: `${((echo hi))}` is an
	// arithmetic syntax error here where a subshell body would have printed
	// `hi`, and one space, `${( (1+2) )}`, is what turns it back into a
	// command nobody has. See syntax.Dialect.BracedArithmeticExpansion (#2725).
	d.BracedArithmeticExpansion = true
	// A here-document inside parentheses that hold a program ends at the
	// closing one: `v=$(cat <<EOF` / `a` / `EOF)` is accepted and `v` is `a`.
	// dash and zsh read the body from the whole input instead, so the `)`
	// goes into it and the construct is never closed (#963).
	d.HeredocEndsAtClosingParen = true
	// And a body line that took a continuation is never the delimiter here,
	// however it joined — which is the core answer and is written out because
	// it is measured rather than inherited (#2430). The measured build also
	// declines to join at all when the text before the backslash is a
	// non-empty prefix of the delimiter, which is recorded in the corpus and
	// not modeled: it reads as an incremental matcher failing to back out
	// rather than as a rule, and it costs only the spelling of the body.
	d.HeredocDelimiterAcrossAContinuation = syntax.NoHeredocDelimiterAcrossAContinuation
	// A `<<-` delimiter written with leading tabs has them stripped the way
	// the body lines do, so `EOF` ends a `<tab>EOF` document. zsh agrees;
	// bash and dash do not. See syntax.HeredocDelimiterTabs.
	d.StrippedHeredocDelimiter = syntax.HeredocDelimiterTabsAreStrippedToo
	// The measured ksh93 is 93u+ 2012, which has no `&>`. Later ksh93u+m
	// does, twelve years apart under the same name — which is the divergence
	// syntax.Dialect's own comment names as the reason its fields are called
	// after constructs rather than after shells. A dialect built for the
	// newer build sets this back to true; the panel measures the older one.
	d.AmpersandRedirect = false
	// `>;` writes to a temporary file beside the target and renames it over
	// the target only if the command succeeded. Measured 2026-09-14 on
	// ksh93u+ 2012-08-01, where it is unique in the panel: `echo new >; f`
	// replaces `f`, and `{ printf X; false; } >; f` leaves the old contents
	// alone at status 1. bash 5.3.15, zsh 5.9.2 and dash all refuse the text
	// at the `;`.
	//
	// This is where `echo <->` comes from: with `>;` in the grammar, that
	// text is `echo` with `<-` and a `>;` whose target is the next word, and
	// `echo <->; echo done` therefore runs nothing and reaches nothing after
	// it (#918).
	d.RenameOnSuccessRedirect = true
	// `<#((expr))` and `>#((expr))`, the file-position redirections — the
	// other half of what this shell alone does to a descriptor. Measured
	// 2026-09-16 on ksh93u+ 2012-08-01 (#3034).
	d.SeekRedirect = true
	// `times` is a reserved word here, so `times foo` is a syntax error
	// rather than a builtin ignoring an argument — the one place in the panel
	// where which builtin a shell has changes what parses.
	d.TimesIsReserved = true
	// Floating point, which POSIX has not and these two do.
	d.ArithFloat = true
	// And C's hexadecimal spelling of one, which this shell alone reads:
	// `$(( 0x1p4 ))` is 16 and `$(( 0x1.8 ))` is 1.5, while `$(( 0x1e5 ))`
	// stays the integer 485 because `e` is a hexadecimal digit.
	d.ArithHexFloat = true
	// `name(args)` inside an expression is a call to a math function, and
	// this shell needs nothing registered for it: it ships sixty-one of them
	// built in, which is what mathfunc.go holds. Measured 2026-09-13 on
	// ksh93u+ 2012-08-01, `$(( sqrt(4) ))` is 2 and `$(( pow(2,10) ))` is
	// 1024, where a name it does not know is `nosuchmf(1) : unknown
	// function` — never a syntax complaint. The grammar and the table are
	// one change, because either alone answers worse than neither.
	d.ArithFunctionCall = true
	// A double quote inside an arithmetic expression is stepped over
	// wherever a token may begin. Measured 2026-09-10 on ksh93u+: with
	// `n=5`, `$(( "1" + 1 ))` is 2, `$(( "n" + 1 ))` is 6 and
	// `$(( 1 + "2" ))` is 3, while `$(( 1"0" ))` is an arithmetic syntax
	// error — the quote ends the number rather than vanishing from the
	// text, which is the line between this reading and bash's (#1223).
	d.ArithDoubleQuote = syntax.ArithDoubleQuoteSkipped
	// A quotation inside a subscript holds its brackets here too: measured
	// on ksh93u+ 2012-08-01, `typeset -A a; a[']']=5; (( r = a[']'] ))` is
	// 5, which is bash's answer and not zsh's. See
	// syntax.Dialect.ArithSubscriptQuoting (#3302).
	d.ArithSubscriptQuoting = true
	// `'c'` is the code of the character between the quotes, the way C
	// reads one, and this is the only shell in the panel with it: measured
	// 2026-09-10, `$(( '1' + 1 ))` is 50 and `$(( 'a' ))` is 97 where the
	// other five answer with a diagnostic. Nothing about the double quote
	// predicts it — three of the four shells that read through one refuse
	// this (#1223).
	d.ArithCharacterConstant = true
	// Extended patterns wherever a pattern may stand.
	d.ExtendedPattern = true
	// And inside `[[ ]]`, which is the only place bash reads them.
	d.ExtendedPatternInCondition = true
	// And a `|` standing outside every group is an alternation of the whole
	// pattern, **however it arrived** — which is a different reading from
	// the one zsh has, not a wider setting of the same one (#2528).
	//
	// Measured 2026-09-13, ksh93u+ 2012-08-01, `env -i PATH=/usr/bin:/bin`
	// over a script file, `v=abc` and `L='a|ab'`. `${v#a|ab}` and `${v#$L}`
	// both answer `bc`, so provenance is not asked; `w='a|b'; ${w#a|b}`
	// answers `|b`, the value declining to match its own text, which is what
	// says the bar is syntax here rather than one more character.
	//
	// It does not reach the filesystem, and that is measured against a
	// control rather than assumed: in a directory holding `aa` and `ab`,
	// `S='a*'` and `B='a?'` each expand to two fields while `P='aa|b'` stays
	// one, so it is the bar that stops at pathname expansion and not the
	// value failing to be a pattern. `[[ ab == $L ]]` is also `n`.
	d.PatternTopLevelAlternation = syntax.TopLevelAlternationWhereverWritten
	// A **bare** group is not one of them, and in an expansion's pattern
	// operand it is refused while reading: `${v#(a)}` is `syntax error at
	// line 1: ` + "`" + `(' unexpected` here and takes the script with it, where
	// `${v#@(a)}` — this shell's own spelling of a group — is read (#1430).
	d.GroupOpeningAPatternOperandIsRefused = true
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
	// The end of a *command string* closes a quote here: `ksh -c 'echo "abc'`
	// prints abc, and so does `eval "echo \"abc"`. The old backquote form
	// behaves the same way and the substitutions do not.
	//
	// The command string and nothing else. A script file, a file read by
	// `.`, and a program on standard input are all refused — ``syntax error
	// at line N: `"' unmatched`` — which is what the other five shells say
	// on every route including this one. Answering the whole shell yes let a
	// truncated script run under our ksh with status 0 and no diagnostic
	// (#1424).
	d.CloseQuotesAtEOF = syntax.RouteFromCommandString
	// `exec {a[1]}>&-`: the name inside the braces may be a subscripted one.
	// Measured — this shell closes the descriptor the element holds, as bash
	// does and zsh does not.
	d.FdVariableSubscript = true
	return d
}

// Semantics is what ksh93 means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	// The command that named a `>(cmd)` does not wait for its body, which is
	// bash's answer and not zsh's — measured as an ordering, `AFTER[PIPE]`
	// against zsh's `[PIPE]AFTER` (#2197).
	s.WritingSubstitutionIsWaitedForAtTheCommand = interp.No
	// A builtin's write into a pipe nobody is reading, with SIGPIPE
	// disarmed, leaves the command at status 0 here -- silently, where the
	// same shell reports 1 just as silently for `echo hi >&-`. The two errnos
	// are one answer in five of the panel, and this shell and zsh split them
	// in opposite directions (#770).
	s.BrokenPipeWriteErrorFailsTheCommand = interp.No
	// An unquoted list is its elements taken one at a time, never their
	// join: `IFS=:; set -- "x:" y; printf "[%s]" $@` is `[x][y]` here and
	// `[x][][y]` in bash, which joins to `x::y` first.
	s.UnquotedListJoinsOnIFS = interp.No
	// And a quoted empty word written behind a separator in the word a `-`
	// or `+` substitutes is not a field here: `set -- a b; v=x; printf
	// "[%s]" ${v:+"$@" ""}` is `[a][b]`, where bash 5.3.20, dash and BusyBox
	// ash all write a third, empty one. See
	// interp.Semantics.EmptyQuotesAfterASeparatorAreAField.
	s.EmptyQuotesAfterASeparatorAreAField = interp.No
	// `${1:=abc}` is `${1:=abc}: bad substitution` — the whole expansion
	// refused, exactly as `${@:=abc}` is, so the positional is not a name
	// an assignment through an expansion may reach (#1541).
	s.AssignThroughExpansionMayNameAPositional = interp.No
	// The one shell with `=~` that takes an empty right operand: `[[ abc =~
	// "" ]]` reports a match and exits 0, where bash and zsh both refuse it
	// as an empty subexpression. It is not that a regex which will not
	// compile is quietly a non-match here — `[[ "a[" =~ [ ]]` is status 1
	// and no match, so an unbalanced bracket does fail — the empty pattern
	// simply is not one of the failures (#2043).
	s.EmptyRegexOperandIsAnError = interp.No
	// A failed `=~` leaves the record of the last match alone here, and a
	// group that took no part is left out of it entirely, so the groups
	// after it move down a place: `[[ abcd =~ (b)(z)?(c) ]]` is three
	// elements here and four in bash (#2916). Both measured 2026-09-18.
	s.RegexMatchSurvivesAFailedMatch = interp.Yes
	s.RegexMatchOmitsGroupsThatDidNotMatch = interp.Yes
	// An associative array's subscript is a quoting context here, as it is
	// in bash: `m["k"]=W` stores under `k`.
	s.SubscriptIsAQuotingContext = interp.Yes
	// bash's answer on the other join, against its own on the one above:
	// `IFS=-; a=(x y z); v=${a[@]}` is `x y z` here where zsh gives
	// `x-y-z`. The two axes partition the panel differently, which is why
	// neither can stand in for the other.
	s.UnsplitAtListJoinsOnIFS = interp.No
	s.CommandNotFoundStatusIsNotFound = interp.No
	// The one shell in the panel that does what POSIX asks of `command -v`
	// for a pathname operand: `command -v ./bb/tool` is the working directory
	// with the operand stuck on the end. Measured 2026-09-14 against 93u+ —
	// the `./` survives in the middle, `cd /` gives `//./bin/ls`, and
	// `command -v /bin/./ls` comes back untouched, so the rule is a prefix
	// join and never a cleaning. `whence -p` and `whence -a` answer the same
	// way, which is why the shaping is in one place (#2931).
	//
	// Written out rather than left to PosixSemantics, which already holds
	// Yes: this is the column the axis was measured on, and a reader looking
	// for the measurement should find it beside the shell that makes it.
	s.APathnameOperandIsReportedAbsolute = interp.Yes
	s.SetFTurnsOffGlobbing = interp.Yes
	// Neither editing mode is selected on its own, interactive or not.
	// Measured 2026-09-11 in a session at a real terminal: `set -o` reports
	// `emacs off` and `vi off` there, with `viraw on` beside them.
	s.InteractiveSelectsEmacs = interp.No
	// And `-B` is this shell's `braceexpand` too — measured 2026-09-11,
	// `set +B; echo {a,b}` writes `{a,b}` and `$-` loses the letter (#1856).
	s.SetBTurnsOffBraceExpansion = interp.Yes
	// `set -t`, and the letter is the whole of it here: this shell has no
	// long name for the option at all, so `set -o onecmd` is `bad option(s)`
	// where the letter is taken. Measured on a script file, on standard input
	// and at the invocation.
	s.SetHasTheTLetter = interp.Yes
	// And it reaches the command string too, which bash's does not: a two-line
	// `-c` string that turns the option on writes nothing after it.
	s.OneCommandStopsACommandString = interp.Yes
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
	// ksh93 has a history expander and starts with it **off**, at a prompt as
	// well as in a script — the one shell in the panel that does, and the
	// reason the second half of this is an axis rather than a constant.
	// Measured 2026-09-15 through a pseudo-terminal with a two-row prompt:
	// `echo !!` at a fresh prompt prints the two characters, `set -H` is
	// taken silently, and `echo !!` after it expands and echoes the expanded
	// line. So the letter is not in InteractiveOptionLetters above either.
	s.HistoryExpansion = interp.Yes
	// And a `^` where a range's **end** is written stays ordinary text here
	// rather than naming word one: measured 2026-09-18 at the same prompt
	// over `echo a b c d e`, `!!:1-^` is `a b c d^` — the range `1-` and
	// then a character nothing read — where bash and zsh both answer `a`.
	// See Semantics.HistoryFirstWordEndsARange.
	s.HistoryFirstWordEndsARange = interp.No
	// `set -k` and `set -o keyword`, and this is the column that leaves a
	// declaration's own operand where it stands: measured 2026-09-16, `set
	// -k; export E1=e1` exports `E1` here where bash lists and exports
	// nothing. ksh93 also decides the option at *parse* time, which
	// keywordassign.go records rather than models.
	s.KeywordAssignments = interp.Yes
	s.KeywordPromotesADeclarationsOperand = interp.No
	// A declaration utility is recognized from the command word as it was
	// written, quoting and all — `\typeset v=$b`, `'typeset' v=$b`,
	// `"typeset" v=$b` and `typese't' v=$b` all keep the value whole — but
	// not from a word an expansion produced: `cmd=typeset; $cmd v=$b` and
	// `e=; $e typeset v=$b` split it. Measured 2026-09-16 on ksh93u+
	// 2012-08-01 with `b='x y'`, for typeset, export and readonly. See #3340.
	s.DeclarationCommandWord = interp.DeclarationByWrittenWord
	// `command typeset v=$b` keeps the value whole, and so do `\command
	// typeset v=$b` and `command command typeset v=$b`. `command -p typeset
	// v=$b` splits it, which the reading above accounts for: the flag is not
	// the prefix as written. Measured the same day. See #3341.
	s.CommandPrefixKeepsADeclaration = interp.Yes
	s.HistoryExpansionAtAPrompt = interp.No
	// Login-ness written out, in both spellings. Measured 2026-09-05 with a
	// scratch home directory: `ksh -l -c cmd` and `ksh --login -c cmd` each
	// read `~/.profile`, which is the file the POSIX preset already names.
	// The interactive file stays `$ENV` and the escape hatches stay empty —
	// this shell has none, so a startup file that breaks it is escaped by
	// moving the file, which is measured and is what it does.
	s.StartupFileOptions = interp.StartupFileOptions{Login: "-l --login"}
	// And Semantics.SystemStartupFiles stays at the POSIX preset's
	// `/etc/profile`, measured for this shell too: with a dashed argv[0] and
	// a command string, `~/.profile` reports `path_helper`'s `${PATH%%:*}`
	// rather than the inherited one, so the system file ran in front of it.
	// The panel's odd one out: this shell names its version on standard
	// error and exits 2 for having been asked. Measured 2026-09-11.
	s.VersionOption = interp.VersionOption{
		Spellings:       "--version",
		Text:            versionLine,
		ToStandardError: true,
		Status:          2,
	}
	// `ksh -c 'echo $-'` reports `chsB` — both route letters, where bash
	// shows `c` alone and dash and zsh show neither. Read down its rows and
	// ksh93's rule for `s` is "no script file was named" where the other
	// three's is "the program came from standard input"; this is the one
	// invocation where those two differ.
	// `+i` takes the prompt back. Measured 2026-09-16 on ksh93u+ 2012-08-01
	// with the program on a pipe: `ksh -i +i -c 'echo $-'` and `ksh +i -c
	// 'echo $-'` both report `chsB`, where `ksh -i -c` reports `icmsBE`.
	s.PlusSignedInteractiveLetterStillPrompts = interp.No
	s.CommandStringShowsCInDollarDash = interp.Yes
	s.LoginShowsLInDollarDash = interp.Yes
	s.CommandStringShowsSInDollarDash = interp.Yes
	// And the order: `i` and `c` lead, the rest of the lowercase letters are
	// sorted, then the uppercase ones, and `l` comes last of all. Measured
	// 2026-09-12, the interactive rows through a pseudo-terminal with a
	// scratch home directory:
	//
	//	set -f; set -u; set -e   cefhsuB
	//	set -C                   chsBC
	//	set -a                   cahsB
	//	-l -c                    chsBl
	//	-i -c                    icmsBE
	//	-il -c                   icmsBEl
	//	-i -Cc                   icmsBCE
	//
	// Two of those are why this is not simply "sorted, capitals last": `i`
	// stands in front of a `c` it sorts after, and `l` stands behind
	// capitals it sorts before. Both are stable across runs and both differ
	// from every other member of the panel.
	s.DollarDashLetterOrder = "icaefhkmnstuvxBCEHTl"
	s.ArithIntegerOperatorRefusesFloat = interp.Yes
	// A numeral a double cannot hold is lost rather than saturated:
	// `$((1e400))` is `-0` here where the same value *computed*,
	// `$((1e300*1e300))`, is `inf`. The zero is the negative one and the
	// unary minus applies to it, so `$((-1e400))` is `0`.
	s.ArithFloatOverflowIsZero = interp.Yes
	// A negative exponent is a float answer here, not a refusal: `2**-1`
	// is 0.5.
	s.ArithNegativeExponentIsError = interp.No
	s.ArrayScalarIsTheWholeArray = interp.No
	// And the one element a plain `$m` on a keyed table gives is the one
	// keyed `0`, which is nothing at all where no such key was written.
	s.KeyedTableScalarIsTheFirstValue = interp.No
	s.ArrayNameWithoutSubscriptIsTheList = interp.No
	// A subscript inside a literal is the text between the brackets, and a
	// literal written with one declares a keyed array: `typeset -p` answers
	// `-A` and `${a[2]}` does not find what `[1+1]=c` stored.
	s.ArrayLiteralSubscriptIsAKey = interp.Yes
	// A `[k]+=` element joins what the literal has built, not the table it
	// replaced: `typeset -A m; m[k]=v; m=([k]+=x)` is `x`.
	s.KeyedLiteralAppendJoinsTheReplacedValue = interp.No
	// `typeset -A m[k]=v` — see Semantics.TableLetterReachesItsOwnOperandsSubscript.
	s.TableLetterReachesItsOwnOperandsSubscript = interp.No
	// ksh93 globs a result but will not let one build a group — see
	// Semantics.ExpansionResultSuppliesGroupSyntax.
	s.ExpansionResultSuppliesGroupSyntax = interp.No
	s.SelectLayout = interp.SelectMenuVertical
	s.SelectPromptNeedsTerminal = interp.Yes
	s.AliasParsesOptions = interp.Yes
	s.AliasHasPrintOption = interp.Yes
	// And the other letter of its own usage line, `Usage: alias [-ptx]
	// [name[=value]...]`: `-x` marks an entry and narrows a listing to
	// the marked ones. It was refused here while that line advertised it,
	// which is the paired tables apart (#2927).
	s.AliasHasExportOption = interp.Yes
	// Its extra letters are `-t` and `-x`, not these two: `alias -g` is
	// `unknown option` on ksh93u+.
	s.GlobalAliases = interp.No
	s.SuffixAliases = interp.No
	s.AliasListsAsDefinitions = interp.No
	s.AliasRestrictsToRegularKind = interp.No
	s.AliasOperandsCanBePatterns = interp.No
	s.AliasPlusPrintsNamesOnly = interp.No
	s.TypeNamesAnAliasOnlyWhenExpanded = interp.No
	// It complains about `alias nope` and says nothing about `unalias nope`,
	// which is why these are two questions.
	s.AliasReportsNotFound = interp.Yes
	s.UnaliasReportsNotFound = interp.No
	s.AliasNotFoundStatusCounts = interp.Yes
	// Alone in the panel, a name this shell's `alias` has *named* stays
	// in the table with no value, so `unalias h` succeeds a second and a
	// third time where the other four report there was nothing to
	// remove. Naming is enough — a failed `alias z` leaves the name
	// behind — and `unalias -a` clears them (#2926).
	s.AliasRemembersTheNamesItNames = interp.Yes
	s.AliasSeparatorEndsTheLookup = interp.Yes
	// A command word that is exactly `-` is a command name here and is
	// reported as one: `- echo hi` is `command not found` at 127 and the
	// script carries on. zsh is the column that throws the word away (#3236).
	s.LoneDashInCommandPositionIsDiscarded = interp.No
	s.UnaliasAllRefusesOperands = interp.No
	s.AliasQuoting = interp.ListingQuoteWhenNeededDollar
	s.AliasListingQuotesTheName = interp.No
	s.TrapQuoting = interp.ListingQuoteWhenNeededDollar
	// `typeset -p` writes `typeset -x -r n=5` — separate flags — and a name
	// with no attributes as a bare `v=1`; a missing name is passed over in
	// silence, status 0, which is measured rather than a shortcut.
	s.DeclareListing = interp.DeclareListingBareAssignments
	// A listing with no operands writes the produced parameters with their
	// readings, and re-reads the producer to get them. Measured 2026-09-13,
	// ksh93u+ 2012-08-01, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
	// over a script file: `typeset -i LINENO=1`, `typeset -i RANDOM=18168`,
	// `typeset -F 3 SECONDS=0.003`, with no reference to any of them first.
	//
	// It re-reads rather than repeating a stored one, and that is observable
	// rather than inferred — two listings a line apart in the same shell hold
	// `RANDOM=9218` then `RANDOM=14475`, and `SECONDS=0.009` then
	// `SECONDS=0.011`. So a listing here differs from itself, which is the
	// hazard interp/localbuiltin.go warns about and is this column's real
	// behavior rather than a defect to design around (#2518).
	s.ProducedParameterListing = interp.ProducedListingWithValue
	s.DeclareValueQuoting = interp.ListingQuoteWhenNeededDollar
	// And a `#` in a listed value is left bare unless a name stands in front
	// of the first one: `16#ff`, `99#zz` and `1a#b` all list unquoted here
	// and `a#b` and `#lead` do not (#1271).
	s.ListedHashIsBareAfterANonName = interp.Yes
	// And not the weaker position rule bash has: `a#b` is quoted here,
	// which is what says the leading text is judged and not the offset.
	s.ListedHashIsBareUnlessItOpensTheValue = interp.No
	// And neither of bash's position rules applies to a `~`: `a~b`, `b~`,
	// `~b` and the keys `['a~b']` and `['b~']` are all quoted here.
	// Measured 2026-09-17 over `set` and a keyed `typeset -p` (#2298).
	s.ListedTildeIsBareWhereItCannotExpand = interp.No
	// The subscript's leading tilde *is* expanded here, as it is in bash:
	// `typeset -A m; m[~/k]=v` stores under `$HOME/k` and `${m[~/k]}` reads
	// it back. Measured 2026-09-17 on ksh93u+ 2012-08-01 (#2298).
	s.SubscriptKeyExpandsALeadingTilde = interp.Yes
	s.ListingControlEscape = interp.ControlEscapeHex
	// The column that leaves `^` bare, and the only one: `v=^` and `v=a^b`
	// list unquoted here where bash and zsh write `'^'` and `'a^b'`.
	// Measured 2026-09-14 over `set`, a keyed `typeset -p` and an alias
	// listing, which agree (#2820).
	// And `!`, which zsh leaves bare too and bash does not.
	s.ListedBangIsOrdinary = interp.Yes
	s.ListedCaretIsOrdinary = interp.Yes
	// And the column that spells a byte above ASCII out: `$'\xc3\xa9'` for
	// a value, for a key and for an alias body alike, where bash and zsh
	// write the character. The same answer decides both halves — whether
	// such a byte is bare, and whether it survives inside `$'...'`.
	s.ListedNonAsciiIsOrdinary = interp.No
	// `=` is not an ordinary byte here. What is bare is a leading `name=`,
	// with the rest quoted on its own: `a=b` bare, `a='b c'`, `x='y=z'`,
	// `'=x'` and `'1=2'`, keys included — `[a=b]` and `[x='y=z']`.
	s.ListedEqualsIsOrdinary = interp.No
	s.ListedAssignmentPrefixIsBare = interp.Yes
	s.ExportListing = interp.DeclareListingCommandWord
	s.ReadonlyListing = interp.DeclareListingCommandWord
	// And the bare form is not the `-p` form here: the command word goes and
	// what is left is a plain assignment — `export` writes `V='a b'` where
	// `export -p` writes `export V='a b'`. Measured; zsh does the same.
	s.BareDeclarationListing = interp.DeclareListingPlainAssignment
	// The opposite of the other two: every letter must hold. Measured
	// 2026-09-10 — `typeset -xi` and `typeset +xi` alike write the one name
	// that is both exported and an integer, and `typeset -ir` writes nothing
	// where the same table gives bash ten rows.
	s.DeclarationListingFilter = interp.DeclarationFilterEveryLetter
	s.DeclarePrintReportsAMissingName = interp.No
	s.TrapActionIsParsedWhenSet = interp.No
	s.TrapParseFailureNamesWhereItFired = interp.Yes
	s.SymbolicMaskTakesMoreThanOneOperator = interp.Yes
	s.SymbolicMaskWhoAloneSetsIt = interp.Yes
	s.SymbolicMaskTakesTheSetuidLetter = interp.Yes
	// The letter but not the copy, which is why the two are two axes:
	// `umask g=u` is `bad format` here and `umask u=X` is taken.
	s.SymbolicMaskTakesAPermissionCopy = interp.No
	// unanswered UmaskPermissionCopyBesideLetters: a copy is `bad format`
	// here whatever is beside it — the line above is that refusal — so a
	// clause holding a copy and a letter never reaches the question.
	// TestUmaskRefusesAPermissionCopy pins the refusal.
	s.SymbolicMaskTakesTheConditionalExecuteLetter = interp.Yes
	s.SymbolicMaskTakesTheStickyLetter = interp.Yes
	// Every dash word is an option here, digits and all: `shift -1` and
	// `shift -0` are both refused as options this shell does not have, which
	// is why a negative count is only reachable after the marker.
	s.ShiftOptionWords = interp.ShiftOptionWordsAny
	s.ShiftDoubleDashEndsOptions = interp.Yes
	// The marker is taken in front of a numeric operand too, so
	// `break -- 1` ends the loop and `return -- 3` returns 3.
	s.NumericOperandDoubleDashEndsOptions = interp.Yes
	// The count is taken and the rest of the line is not read.
	s.ExtraNumericOperand = interp.ExtraNumericOperandIgnored
	s.ShiftNamesAreArrays = interp.No
	s.ShiftNegativeIsOutOfRange = interp.Yes
	s.WaitReadsOptions = interp.Yes
	// Job specs by command text, a second match taken rather than refused.
	// A `wait` whose spec names nothing says nothing at all and reports 0;
	// there is no -n, and disown only shields a job from a HUP this engine
	// never forwards, so the listing keeps it.
	s.JobSpecsByName = interp.Yes
	s.AmbiguousJobNameIsRefused = interp.No
	s.WaitReportsAMissingJob = interp.No
	// And it is the one column that keeps nothing once a job is reported:
	// `wait %1; wait "$p"` is 127 there where it is the job's status in
	// every other column measured.
	s.WaitRemembersAReapedJob = interp.No
	// And it says so in a sentence of its own — see
	// Diagnostics.WaitSignalNotice.
	s.WaitReportsTheSignalThatEndedTheJob = interp.Yes
	s.WaitNextJob = interp.WaitNextJobAbsent
	// `wait -p` is not ksh93's either: `wait: -p: unknown option`, beside
	// its own usage line. Measured 2026-09-13.
	s.WaitPNamesTheFinishedJob = interp.No
	// The lone divergence on an interrupted wait: a bare one reports this
	// shell's own 256 plus the signal, and one that names a job — `wait $!`
	// or `wait %1` — reports a plain 1 instead.
	s.WaitForAJobFailsWhenInterrupted = interp.Yes
	s.DisownRemovesTheJob = interp.No
	// And every call answers 1 and says nothing, found or not: six
	// spellings, six silent 1s, with and without job control, and the job
	// it can list is still listed afterwards (#3187). Measured 2026-09-18.
	s.DisownAlwaysFails = interp.Yes
	// Both Yes, re-measured with a letter ksh93 does not own (-q): the first
	// probes used -x and -a, which are real ksh93 options, and recorded No
	// off ksh93's own features.
	s.CommandRejectsUnknownOption = interp.Yes
	// Whether `command -v` answers for every name it was given, and what
	// decides the status when it found some of them. See
	// interp.Semantics.CommandReportsEveryOperand for the split.
	s.CommandReportsEveryOperand = interp.Yes
	s.CommandCountsAMissingOperand = interp.Yes
	// And whether a `command` reached through an expansion keeps the power
	// to run what it names (#3369).
	s.ExpandedCommandOnlyReports = interp.Yes
	// What a `command -p` search resolved is **not** remembered here: with
	// an unusable PATH, a `command -p ls` and a plain `ls` after it, the
	// second is 127 in this column and 0 in bash. See
	// interp.Semantics.DefaultPathSearchIsRemembered (#2975).
	s.DefaultPathSearchIsRemembered = interp.No
	// And the word is a boundary: `eval 'export -q; echo INNER'` stops the
	// script here, and `command eval '…'` abandons the `eval`'s text, reports
	// 2 and carries on. The special-builtin row is what answers for this
	// shell — the three expansion producers cannot, because its `eval` is
	// already a boundary for those (FatalErrorEndsBorrowedTextOnly).
	s.FatalErrorEndsAtTheCommandWord = interp.Yes
	s.GetoptsRejectsUnknownOption = interp.Yes
	s.ShiftCountIsArithmetic = interp.Yes
	s.TrapBodyRunsWhatParsed = interp.No
	s.ReportsAKilledCommandInACommandSubstitution = interp.Yes
	// A substitution's body is a subshell environment and holds the
	// shell's options with it, `-e` included: measured 2026-09-15,
	// `set -e; echo "end[$(false; echo no)]"` is `end[]` here, and `$-`
	// inside the body still carries `e`.
	s.ErrExitEntersACommandSubstitution = interp.Yes
	// And the shared-state spelling is a boundary for a *stop* even though
	// it is not a subshell. Measured 2026-09-16 from a script file: `x=${
	// echo pre; exit 7; }` is `[pre]` at status 7 with the next line of the
	// script still run here, where bash 5.3.20 — the only other column with
	// the construct — ends the shell at 7. It holds for an error the shell
	// reported too (`q: is read only`, `shift: 99: bad number`, `nope:
	// parameter not set`), and however deep the raise is: a function called
	// from the body that runs `exit 4` leaves the substitution at 4 and no
	// more. Only `break` and `continue` still leave, which they do in bash
	// as well.
	s.CurrentShellSubstitutionBoundsAnUnwind = interp.Yes
	// And a body that is nothing but `<file` is that file, exactly as
	// `$(<file)` is. Measured 2026-09-16: `${ <f ;}` is the file's text here
	// and the empty string in bash 5.3.20, with `${ <f ; echo t; }` and
	// `${ echo h; <f ; }` answering alike in both — so what this shell reads
	// is the whole body being the one redirection.
	s.CurrentShellSubstitutionReadsAFile = interp.Yes
	// And the body is *not* a variable scope here, which is the half of the
	// spelling the two columns that have it split on. Measured 2026-09-16:
	// `x=outer; v=${ typeset x=in; printf %s "$x"; }` leaves `x` as `in`
	// here and as `outer` in bash 5.3.20 — the declaration writes the
	// shell's own name, which is what "the body runs in this shell" means
	// taken all the way. Said out loud rather than inherited, because this
	// is one of only two shells that can be asked.
	s.CurrentShellSubstitutionBodyIsAScope = interp.No
	// The listing runs the other way here: descending by signal number,
	// which puts EXIT last where the other six put it first.
	s.TrapListingOrder = interp.TrapListingHighestFirst
	s.TrapBodyLine = interp.TrapBodyLineOffsetFromWhereItFired
	// And the same for the two conditions that fire at a command: this
	// shell counts every trap body from where it fired, so the second
	// question has the same answer as the first.
	s.CommandTrapBodyLine = interp.TrapBodyLineOffsetFromWhereItFired
	s.ExitTrapFiresPastTheEnd = interp.No
	// A call of `function g { … }` is a scope for the trap table and a call
	// of `g() { … }` is not, which is the third answer to the locality
	// question and the only one keyed on how the function was *written*. It
	// takes the EXIT trap with it: an EXIT trap set inside a `function` body
	// fires at that call's return and the caller's comes back, where the
	// POSIX-form call beside it replaces the shell's outright.
	//
	// Measured 2026-09-16 on AT&T 93u+ 2012-08-01 — `trap 'echo O' USR1;
	// function g { trap 'echo I' USR1; }; g; kill -USR1 $$` is `O` and the
	// same body written `g() { … }` is `I` (#2345).
	s.FunctionLocalTraps = interp.TrapsGoBackAtTheReturnOfAKeywordFunction
	// The option table is the same word's third scope, and it is a *restore*
	// where the trap table is a reset: the body is handed the caller's
	// options exactly as they stand and the whole table goes back at the
	// return. Measured 2026-09-16 on AT&T 93u+ 2012-08-01 — `set -o noglob;
	// function g { set +o noglob; }; g` leaves noglob **on**, and the same
	// body written `g() { … }` leaves it off (#3308).
	s.FunctionLocalOptions = interp.OptionsGoBackAtTheReturnOfAKeywordFunction
	s.SelectEofEndsPromptLine = interp.No
	s.SelectEofIsSuccess = interp.No
	s.SelectTakesUnterminatedReply = interp.No
	s.SelectEofPrintsNewline = interp.No
	// hash is an alias for `alias -t` here, and a name that resolves to
	// nothing is a silent success.
	s.HashReportsAMissingName = interp.No
	// And a `PATH=… cmd` prefix leaves the table alone: the new PATH goes
	// to the child and to the search this shell makes, and never to the
	// shell's own PATH, so nothing empties the table. Measured 2026-09-13
	// with two copies of one name on PATH, the first hashed: the prefixed
	// run takes the second copy and the table still holds the first.
	s.APrefixedPathEmptiesTheCommandHash = interp.No
	// And the entry does not stand in front of the search. A copy that has
	// appeared in a directory earlier on PATH is found on the next call with
	// no assignment to PATH in between, and the entry is repointed at it —
	// measured 2026-09-15 and ksh93's alone in a panel of six. See
	// [interp.Semantics.HashedPathShadowsAnEarlierDirectory] (#2936).
	s.HashedPathShadowsAnEarlierDirectory = interp.No
	// Which failed candidate a PATH search names, which a probe with one PATH
	// entry could not see: this shell keeps the failure of the **last entry it
	// really searched** rather than the first interesting one, so the same two
	// entries in the two orders answer differently — `$dir:$empty` is `zzcmd:
	// not found` at 127 and `$empty:$dir` is `cannot execute [Is a directory]`
	// at 126. See [interp.PathCandidateReport] for the seven rows (#3249).
	s.PathCandidateReported = interp.LastSearchedEntry
	s.TildePlusMinusExpands = interp.Yes
	// This shell has `$_`, and the row that said it did not was measured
	// through a `;`-list — the one shape where a shell with the parameter
	// and a shell without it give the same empty answer. `echo one two`
	// and `echo "[$_]"` on their own lines read `two` here (#3134).
	s.UnderscoreTracksTheLastArgument = interp.Yes
	// The same reading one step earlier: the parameter is there before any
	// command has written it, so `${_+x}` is non-empty and `set -u` reads an
	// empty string rather than stopping. That is what separates this column
	// from dash and BusyBox ash, which the `;`-list probe could not.
	s.UnderscoreIsAParameterAtAll = interp.Yes
	// And it moves under a rule nothing else in the panel has: only a simple
	// command standing alone on a line at the top level of the input writes
	// it. A `;`-list, an `&&` chain, a pipeline, a backgrounded command and
	// everything inside a loop, a branch, a group, a subshell, a function
	// body or an `eval` all leave it where it stood — and so does a bare
	// assignment, which in the other two empties it.
	s.UnderscoreMovesOnlyBetweenInputCommands = interp.Yes
	// A function body opens with what the caller had, as bash's does. It
	// then stays there for the whole body, which the rule above is what
	// says: nothing inside a body is at the input level.
	s.UnderscoreMovesBeforeAFunctionBody = interp.No
	// A defined f-g stops the script; a.b is an invalid discipline function.
	s.PunctuatedFunctionNameIsRefused = interp.Yes
	// And `a.get` is not: a dotted name whose suffix is one of this shell's
	// four variable events defines a *hook* on the variable in front of the
	// dot. Measured 2026-09-15 against AT&T ksh93u+ 2012-08-01 — `function
	// g.get`, `g.set`, `g.append` and `g.unset` all define, whether or not
	// `g` exists yet, where `function ns.thing` is refused exactly as it is
	// here. The refusal and its fatality were already right; the set of
	// names they fired on was four suffixes too wide (#3033). See
	// interp/discipline.go for what each event carries.
	s.DisciplineFunctionIsAVariableHook = interp.Yes
	// Every arithmetic value is a C double here — `$(( 9007199254740993 ))`
	// is 9007199254740992 and `$(( big * 2 ))` is 1.84467440737096e+19 — and a
	// value that names another variable is chased until it is a number.
	//
	// This was `ArithOverflowSaturates`, recorded from `$(( big + 1 ))` alone.
	// That row is the maximum under either reading, so it could not tell a
	// clamp from a double; `$(( big * 2 ))` can, and it is not a clamp.
	s.ArithValuesAreCarriedInADouble = interp.Yes
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
	// `${!ab@}` and `${!ab*}` list the names *extending* the prefix here:
	// with `ab=1; abc=2; abd=3` the answer is `abc abd`, and `ab` — which
	// is set, and which every other column with the operator lists — is
	// left out. Measured 2026-09-13 against 93u+ 2012-08-01 (#2619).
	// A substring's offset and length have their pattern characters
	// protected before the range is read, so every one of them is an
	// arithmetic syntax error here: `${s:(-2)}` is `\(-2\): arithmetic
	// syntax error` where bash and zsh both slice the last two characters.
	// Measured 2026-09-13 against 93u+ 2012-08-01 (#2618).
	s.SubstringRangeQuotesPatternCharacters = interp.Yes
	s.NamePrefixListingExcludesTheExactName = interp.Yes
	// `${s[@]:off:len}` on a name holding one string slices a *list of one*
	// here, alone in the panel: measured 2026-09-11 against 93u+
	// 2012-08-01, `h="a b"; ${h[@]:0:1}` is `a b` and `${h[@]:1}` is
	// nothing, where bash 3.2, bash 5.3 and zsh all answer with the value's
	// characters. The length is not the same split — `${#h[@]}` is 1 here
	// and in both bashes, and 3 only in zsh — which is why the two are
	// separate axes.
	s.WholeSubscriptOnAScalarSlicesIt = interp.No
	// `[ n -eq 5 ]` holds with `n=5` here, and so does `[ 1+1 -eq 2 ]`: the
	// single-bracket builtin reads its comparison operands as arithmetic the
	// way `[[ ]]` does, which is what makes `[ a -eq 1 ]` a silent false
	// rather than an integer complaint — the name is zero.
	s.TestBuiltinComparisonOperandsAreArithmetic = interp.Yes
	// `f -nt missing` holds when f exists, and `-t x` is a plain false
	// rather than dash's and bash's integer complaint.
	s.MissingFileIsOlder = interp.Yes
	s.TerminalTestRequiresANumber = interp.No
	// A `-t` descriptor is read at the width of a C `int` — `[ -t 4294967296 ]`
	// asks about descriptor 0 — and -1, which is where every conversion too
	// wide to hold lands, holds whatever the shell is holding.
	s.TerminalTestDescriptorNarrowsToThirtyTwoBits = interp.Yes
	s.TerminalTestMinusOneIsATerminal = interp.Yes
	// And a lone `-t` is `-t 1` rather than a non-empty string: `[ -t ]
	// >/dev/null` is 1 here and 0 in dash and bash, while the same line on a
	// pseudo-terminal is 0 in all six.
	s.BareTerminalTestIsDescriptorOne = interp.Yes
	// A name-shaped value is looked up in turn here as it is in bash and
	// zsh — `y=5; x=y; $((x+1))` is 6, and `y=z; z=7; x=y` is 8 — measured
	// 2026-09-11 against 93u+ 2012-08-01. The axis comment in interp said
	// this shell errored instead, which was a mistake about *this* field
	// rather than about the preset; see #1629 and the next line, which is
	// the difference it was describing.
	s.ArithNameValueRecurses = interp.Yes
	// And a fixed depth is what stops it, as in bash: a chain of sixty
	// distinct names ending in a number is `recursion too deep`. Measured
	// 2026-09-18 (#3416).
	s.ArithRecursionBound = interp.ArithRecursionBoundedByDepth
	// And an unset name reached that way is a refusal rather than a zero:
	// `x=abc; $((x+1))` is `abc: parameter not set` at status 1 and the
	// script stops, with nounset off. A name written in the expression
	// itself is still zero — `$((nosuch+1))` is 1 here as everywhere.
	s.ArithRecursedNameMustBeSet = interp.Yes
	// A name inside a subscript is a *parameter* here and an unset one is
	// refused, where the same name outside the brackets is zero (#2817).
	s.ArithSubscriptNameMustBeSet = interp.Yes
	// And with nounset on the same refusal reaches every name an expression
	// reads, subscript or not: `set -u; : $((b))` is `b: parameter not set`.
	// Not fatal of itself — it is the expression's failure, and the
	// construct holding it answers, which is why `(( b ))` ends the input
	// here (ArithCommandErrorIsFatal) and `let "x=b"` reports and returns 1
	// (#3574).
	s.ArithUnsetNameUnderNounsetIsRefused = interp.Yes
	s.ArithNounsetRefusalIsFatal = interp.No
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
	// And the keyword is what carries `$0` too. Measured 2026-09-15 from a
	// script, and each line is a separate claim:
	//
	//	function kf { echo "$0"; . ./inc.sh; }   → kf, then kf again
	//	pf() { echo "$0"; }                      → the script
	//	. ./inc.sh at the top level              → the script
	//	function outer { pf; }                   → outer, from inside pf
	//
	// So a sourced file never answers here, a `name()` function never
	// answers here, and neither of them hides the keyword function that
	// called it — which is a different frame from the innermost call zsh
	// names, not the same rule with a filter on it. The axis was an Answer
	// until #2345 and its doc said this shell keeps the script's name inside
	// a function, which is what the `name()` spelling alone had been asked.
	s.DollarZeroNames = interp.DollarZeroIsTheInnermostKeywordFunction
	// An assignment prefix to a frozen name is answered by the kind of
	// command it stands in front of, and by the kind the word *resolves*
	// to: `command` is transparent here. Measured 2026-09-11 with
	// `readonly x=1` and `x=2 <cmd>; echo after`:
	//
	//	/bin/echo RAN      complains, never runs it, 1, reaches `after`
	//	true, echo E       says nothing at all, runs it, 0
	//	command true       says nothing at all, 0
	//	command /bin/echo  complains, never runs it, 1
	//	an alias for true  says nothing at all, 0
	//	: and a function   complains and ends the script at 1
	//
	// So the silence is the *regular builtin*'s, whichever way the word
	// reached one, and the fatality is the two kinds that would have kept
	// the assignment. An earlier reading of this held `true` fixed
	// throughout and got five of those rows backwards (#1219).
	s.PrefixToARegularBuiltinIsRefused = interp.No
	// The panel's holdout on both halves of a prefix to a *function*, and it
	// is the same departure twice: the assignment is made to this shell
	// rather than handed to the call, so it is still there afterwards and no
	// child is ever told about it. Measured 2026-09-12 in 93u+ 2012-08-01:
	// `f(){ env | grep "^v="; }; v=1; v=9 f; echo "[$v]"` prints nothing
	// from the child and `[9]` after, where the other six print `v=9` and
	// `[1]`. It holds for a body that assigns over the name as well —
	// whatever the function leaves is what the caller reads. And the
	// attribute is taken *off* rather than merely withheld: `export v=1;
	// f(){ typeset -p v; }; v=9 f` prints a plain `v=9` here against bash's
	// `declare -x v="9"`, so a name the script had exported reaches no child
	// during the call and none after it either (#2407).
	// And `command` in front of a special builtin does **not** take that
	// persistence away here, which is where this shell parts from the other
	// two that persist anything. Measured 2026-09-18 from a script file under
	// `env -i PATH=/usr/bin:/bin LC_ALL=C`: `s=base; s=C command :` leaves
	// `C`, `s=E command eval :` leaves `E`, and the subscripted spelling
	// `arr=(x y z); arr[1]=A command :` leaves `A`. `s=Q command true` is
	// `base`, which is the control: a regular builtin's prefix never
	// persisted, so the word is being looked through rather than ignored
	// (#3448).
	s.CommandKeepsASpecialBuiltinsPrefix = interp.Yes
	s.AssignmentPrefixPersistsAfterAFunction = interp.Yes
	s.PrefixToAFunctionIsExported = interp.No
	// And a prefix in front of a `function`-form function belongs to the
	// call: measured 2026-09-18, `s=base; function kf { print "[$s]"; };
	// s+=5 kf` shows the body `5` — a fresh cell with nothing to append to
	// — and leaves `base` behind, where the POSIX-form `pf` above keeps the
	// 5. The same split this shell makes for a `typeset` inside the body,
	// read from the prefix's side (#3161).
	s.PrefixToAKeywordFunctionIsScopedToTheCall = interp.Yes
	// The same direction at a builtin, and the same reading: a prefix is an
	// ordinary assignment to this shell, so a name that had the export
	// attribute *loses* it for the length of the command. `export z=1; z=2
	// typeset -p z` reads a bare `z=2` here against bash's `declare -x z="2"`,
	// and the child of `z=2 eval env` is told nothing. Measured 2026-09-16 in
	// 93u+ 2012-08-01 (#3437).
	// The prefix is worked through before the redirections are opened, as in
	// bash: measured 2026-09-18, `w=$(echo S >&2) f > /nope/x` writes `S` and
	// then `cannot create` (#3449).
	// …and only where the assignment is a real store: an external command and
	// a regular builtin open the redirections first here, which is exactly
	// the set whose prefix this shell does not keep. Measured 2026-09-18 —
	// `f` and `eval :` write their letter, `/usr/bin/true`, `true` and
	// `print` write none.
	s.PrefixExpandedBeforeTheRedirections = interp.PrefixExpandedBeforeRedirectionsWhereItPersists
	s.PrefixExportAtABuiltin = interp.PrefixExportAtABuiltinOff
	// And a listing with no operand walks the **environment** the command
	// was handed, so the prefix's entry is in it and counts as exported
	// whatever that row just did to the attribute: `m=1; m=9 export -p`
	// writes `export m=9` for a name nothing ever exported, on the same line
	// whose `m=9 typeset -p m` writes the unexported `m=9`. Measured
	// 2026-09-18 (#3446).
	s.PrefixInAWholeTableListing = interp.PrefixInAWholeTableListingIsTheCommandsEnvironment
	// Nothing to keep: a prefix persists on a special builtin here by the
	// axis above, and `typeset` keeps it too — `x=1; x=2 typeset x` reads `2`
	// and so does `i=2 typeset -i i`, whatever letters are written. So every
	// row the promoting shell is measured on already reads the prefix's value
	// here, for a reason that is not this one (#3437).
	s.DeclarationPromotesThePrefixEntry = interp.No
	// A subscripted name in a prefix writes the element, and the write is
	// given back exactly where a scalar prefix's is: `arr=(x y z); arr[1]=P
	// read -r j </dev/null` leaves `x` here and `P` in zsh, while a function,
	// `:` and `eval` keep it in both. Measured 2026-09-16 on ksh93u+
	// 2012-08-01 (#3433).
	s.SubscriptedAssignmentPrefix = interp.SubscriptedPrefixStoresTheElement
	s.SubscriptedPrefixIsTakenBack = interp.Yes
	s.PrefixRefusalFatality = interp.PrefixRefusalFatalOnASpecialBuiltinOrFunction
	s.PrefixRefusalCostsTheCommand = interp.Yes
	// The attribute cannot come off, though, which is where this shell parts
	// from zsh and why the two are separate questions: `typeset +r x` on a
	// frozen name is `typeset: x: is read only` and ends the script.
	s.ReadonlyAttributeCanBeRemoved = interp.No
	s.DeclaredNameWithoutValueIsEmpty = interp.No
	// And nothing is recorded either: `typeset xyz; typeset -p xyz` writes
	// nothing at 0 and a bare `typeset` does not name it. An *attributed*
	// valueless operand does list — `typeset -i xyz` comes back as itself —
	// so this is the unattributed one alone. Measured 2026-09-15 on ksh93u+
	// under `env -i PATH=/usr/bin:/bin` (#2999).
	s.ValuelessDeclarationRecordsTheName = interp.No
	// A table the letters merely declared *is* among the names a prefix
	// listing comes to: `typeset -A q1; typeset -a q2; echo "[${!q@}]"` is
	// `[q1 q2]` here, where bash 5.3 answers `[]`. Measured 2026-09-16 on
	// ksh93u+ under `env -i PATH=/usr/bin:/bin LC_ALL=C`, from a file.
	s.PrefixListingNamesADeclaredOnlyCompound = interp.Yes
	// And an array with no elements is unset to the `-`/`+` test, as it is
	// in bash: `set -A e; echo "[${e[@]+S}]"` is `[]` and `${e[@]-D}` is
	// `[D]`. Measured with `set -A` rather than `e=()`, which this shell
	// reads as a compound variable — the confound
	// Semantics.UnsetNameAtIsOneEmptyField records. 2026-09-16 on ksh93u+
	// (#2298).
	s.EmptyArrayIsSet = interp.No
	// And `[[ -v a[@] ]]`, set from this shell's *table* row: measured
	// 2026-09-18, `typeset -A n; n[k]=v; [[ -v n[@] ]]` is false, so the
	// subscript names an element and the key `@` is not one. The indexed rows
	// are not modeled — this shell evaluates an array subscript
	// arithmetically and `[[ -v e[@] ]]` is `@: arithmetic syntax error`,
	// which ends the script — so what is answered here is the half that can
	// be asked (#3436).
	s.ConditionWholeArraySubscript = interp.ConditionWholeArraySubscriptNamesAnElement
	// The colon-less `${a[i]?word}` reaches only the element a bare read of
	// the name means here: with `a=(x y z)`, `${a[9]?m}`, `${nope[1]?m}` and
	// `${m[q]?m}` on a declared table are all `[]` at 0, where bash 5.3.20
	// and zsh 5.9.2 refuse all three. `${nope?m}` and `${nope[0]?m}` are the
	// controls and refuse here too — so the operator is aimed rather than
	// switched off by brackets — and `${a[9]-D}` and `${a[9]+S}` are the
	// sharper ones: those two do see the missing element, in this column as
	// in the others. Measured 2026-09-18 on ksh93u+ 2012-08-01 under
	// `env -i HOME=… PATH=/usr/bin:/bin LC_ALL=C`, from a script file with
	// each row in a subshell (#3241).
	s.ErrorOperatorSeesOnlyTheBareElement = interp.Yes
	// And the **colon** form of the same operator tests the array's first
	// element alone, however many non-empty elements follow it:
	// `a=("" c); "${a[@]:-x}"` is `[x]` and `"${a[@]:+y}"` is `[]`, where
	// bash 5.3.20 and zsh 5.9.2 both keep the two elements. `b=(c "")` is
	// the control and is `[c][]` in every column. The `[*]` spelling answers
	// with the `[@]` one here, which is where it parts from zsh. Measured
	// 2026-09-18 on ksh93u+ 2012-08-01 under `env -i HOME=…
	// PATH=/usr/bin:/bin LC_ALL=C`, from a script file (#3425).
	s.WholeArrayColonTest = interp.WholeArrayColonTestReadsTheFirstElement
	// A keyword-defined function's `typeset -x` is local like any other
	// declaration; the POSIX-style function that leaks it has no scope to
	// leak out of, which TypesetLocalNeedsKeywordFunction already answers.
	s.ExportLetterDeclaresAGlobal = interp.No
	// A valueless `typeset` of a standing name is silent here.
	s.ValuelessDeclarationOfAHeldNameListsIt = interp.No
	// And a plain word over a name holding an array is taken.
	s.ScalarOverACompoundIsAnInconsistentType = interp.No
	// A frozen name refuses a declaration's array literal here, which is the
	// answer every column but one gives: measured 2026-09-12, `readonly q=1;
	// typeset -g q=(b)` is `q: is read only` at 1, where zsh replaces the
	// scalar with an array and carries on.
	s.ArrayLiteralOperandRetypesAFrozenScalar = interp.No
	// The letter half of the same rule, and the same answer — which is the
	// column #2539 was filed not knowing, since the word `integer` is this
	// shell's own and nothing had asked it. Measured 2026-09-12 under
	// `env -i` on ksh93u+, `readonly q=1; typeset -i q=4` is `q: is read
	// only` and so are the `-F` and `export -i` spellings, with the name
	// left at 1 (#2539).
	s.NumericTypeLetterRetypesAFrozenName = interp.No
	// The valueless half is the other way, and that is the split: a letter
	// alone over a frozen name is taken here, silently and at 0. Measured
	// 2026-09-13 under `env -i` on ksh93u+, `readonly q=1; typeset -i q`
	// lists `typeset -r -i q=1` and `typeset -u q` lists `typeset -r -u
	// q=1`, where bash 5.3 refuses both (#2561).
	s.AttributeOverAFrozenNameIsRefused = interp.No
	// An exported name whose declaration named a numeric type reaches a
	// child as `0`, even though the shell itself reads the name as unset:
	// `typeset -ix Z; env` hands over `Z=0` where `${Z+set}` is empty.
	s.NumericTypeWithNoValueReachesAChildAsZero = interp.Yes
	// So is a type letter with an array literal: `typeset -ia z=(1 2)` is
	// `typeset -a -i z=(1 2)`, and `typeset -Fa z=(1 2)` is the float
	// spelling of the same line.
	s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = interp.No
	// A container letter and a numeric one stand together: `typeset -ia z` is
	// `typeset -a -i z`, an array of integers.
	s.NumericAttributeReplacesTheArrayAttribute = interp.No
	// But an array literal *assigned* to a name the array letter was never
	// written for re-creates it, dropping the letter and storing the words
	// unread: `typeset -i a; a=(5+5 6+6)` is `typeset -a a=(5+5 6+6)`, where
	// `typeset -a -i c; c=(5+5 6+6)` is `typeset -a -i c=(10 12)`.
	s.ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = interp.Yes
	// An **append** over the same name keeps it, which is the half that
	// makes the two separate axes: `typeset -i p=3; p+=(5+5)` is `typeset -a
	// -i p=(3 10)`, the scalar promoted and the join evaluated.
	s.AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = interp.No
	// A name carries one letter saying what its values are, and the last one
	// written speaks: `typeset -l z; typeset -i z` is `typeset -i z=1`, and
	// `typeset -i y; typeset -l y` is `typeset -l y=1`. This is the one
	// column that answers both directions yes.
	s.NumericAttributeReplacesTheCaseAttribute = interp.Yes
	// Within one declaration both case letters record beside the numeric one
	// alike. Measured 2026-09-12, `typeset -li v=4` lists `typeset -l -i
	// v=4` and `typeset -ui v=4` lists `typeset -u -i v=4` — which is also
	// what this shell's own `integer` word lists as, since it is `typeset
	// -li` (#2541).
	s.UpperCaseLetterBesideANumericTypeLetterRecordsNothing = interp.No
	// And two case letters on one declaration do not cancel here: the later
	// one wins and folds. Measured 2026-09-12, `typeset -lu z=Ab` reads `AB`
	// and `typeset -ul z=Ab` reads `ab` (#2541).
	s.TwoCaseLettersOnOneDeclarationCancel = interp.No
	s.CaseAttributeReplacesTheNumericAttribute = interp.Yes
	// But an attribute added to a name that already holds a value re-reads
	// that value at once: `FOO=bar; typeset -i FOO` stores 0 over the text,
	// and `d=MiXeD; typeset -u d` stores MIXED. bash waits for the next
	// assignment.
	s.AttributeRereadsTheValueItFinds = interp.Yes
	s.InheritedValueSurvivesADeclaredType = interp.No
	s.CompoundElementsGoThroughTheAttribute = interp.Yes
	// The case attributes fold on the way in here as they do in bash, and
	// the listing writes what the store holds: `typeset -l lo=AB` is
	// `typeset -l lo=ab`.
	s.CaseAttributeFoldsWhenRead = interp.No
	s.CompoundAttribute = interp.CompoundAttributeFoldsEveryElement
	// The two array letters answer the converse differently here, which is
	// the whole reason it is two axes. `b=1; typeset -a b` converts nothing
	// and records nothing — `typeset -p b` is `b=1`, and a bare `typeset -a`,
	// which lists every name carrying the attribute, prints nothing — while
	// `a=1; typeset -A a` is `typeset -A a=([0]=1)`. Measured 2026-09-08.
	//
	// Staying a scalar is invisible to `${b[0]}` and `${#b[@]}` in this
	// shell, because a scalar answers both as an array of one would; the
	// listing is what tells it from bash's promotion, and a one-element array
	// of this shell's own making does carry the letter — `b=(1)` lists as
	// `typeset -a b=(1)`.
	s.ScalarUnderAnArrayDeclaration = interp.ScalarUnderACompoundStaysAScalar
	s.ScalarUnderATableDeclaration = interp.ScalarUnderACompoundBecomesTheFirstElement
	// A name holding a compound reaches a child as its **first value**:
	// measured 2026-09-12, `typeset -x a=(p q)` puts `a=p` in a child's
	// environment and an exported table puts its first value there. The
	// only column that hands a child anything for a compound (#1380).
	s.ExportedCompoundReachesAChildAsItsFirstValue = interp.Yes
	// And a subscripted operand's letters land on the name: `typeset -x
	// a[1]=v` lists `typeset -x -a a=([1]=v)` here (#1380).
	s.SubscriptedOperandCarriesTheAttributes = interp.Yes
	// `a=(1 2); a+=x` is `typeset -a a=(1x 2)`: the first element joined, the
	// rest standing, two elements.
	s.ScalarAppendedToAnArrayBecomesANewElement = interp.No
	// `a=(1 2 3); a=x` is `typeset -a a=(x 2 3)`: the first element
	// written and the array kept, and `m=x` over a table is
	// `typeset -A m=([0]=x [k]=v)`.
	s.ScalarAssignedOverACompoundReplacesTheName = interp.No
	s.ArrayLiteralAssignmentStartsTheNameOver = interp.Yes
	// echo reads -n and -e; a word carrying -E is an operand. \e expands,
	// \x does not.
	s.EchoOptions = "ne"
	// `\E` is the escape character here and `\e` is two characters — the
	// opposite of zsh, which is why one axis could not answer for both.
	// No `\u` and no `\U`: `echo 'a\u0041Z'` writes the characters as they
	// stand in 93u+, where the two shells that have the escape write `aAZ`.
	s.EchoExpandsUnicodeEscapes = interp.No
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
	// `readonly` keeps POSIX's single letter, and this shell says so
	// itself: measured 2026-09-12, `readonly -a zz` is `readonly: -a:
	// unknown option` followed by `Usage: readonly [-p] [name[=value]...]`.
	// `-A`, `-f` and `-n` are refused the same way. It is the reason
	// ReadonlyRecordsTheCompoundAttribute is not answered here — the letter
	// that raises the question does not exist, so the axis cannot be
	// reached rather than being left undecided (#2277).
	// unanswered ReadonlyReferenceLetter: `readonly -n` is refused here, so
	// the letter never reaches the axis. Measured 2026-09-18 under
	// `env -i PATH=/usr/bin:/bin LC_ALL=C`: `readonly -n zz` is `readonly: -n: unknown option`
	// with the builtin's usage block.
	//
	// ReadonlyOptions has no `n`, which is what keeps the question off
	// this column rather than answered wrongly.
	s.ReadonlyOptions = "p"
	// Measured 2026-09-12: `x=1; unset -n x` leaves `x` gone at 0, which is
	// what `unset x` does — the letter changes nothing for a name that is not
	// a reference, where bash removes nothing at all (#932).
	s.UnsetReferenceLetterRemovesANonReference = interp.Yes
	// A name reference that would reach itself is refused at the
	// *declaration* here, however long the loop. Measured 2026-09-15 on
	// ksh93u+, `env -i` with a scratch HOME: `typeset -n r=r` and the pair
	// `typeset -n a=b; typeset -n b=a` are both `typeset: …: invalid self
	// reference` at 1 — the second on the declaration that closes the loop —
	// where bash makes the pair and warns at the read. See
	// Semantics.NamerefCycleIsRefused.
	s.NamerefCycleIsRefused = interp.Yes
	// And the array refusal comes **before** both of those, on a name that
	// really holds an array rather than one merely carrying the attribute:
	// measured 2026-09-16 on ksh93u+, `r=(a b); typeset -n r='not a name'`
	// and `r=(a b); typeset -n r=r` are both `r: reference variable cannot
	// be an array`, while a bare `typeset -a r` — which this shell's own
	// listing writes with no `=` after it — takes `typeset -n r=v` at 0.
	// The associative attribute does make an object, `typeset -A m=()`, and
	// is refused (#3103).
	s.NamerefArrayRefusal = interp.NamerefArrayCheckedFirstOnTheContents
	// And the `n` letter is read **alone** or not at all. Measured
	// 2026-09-18 on ksh93u+ 2012-08-01, `env -i` with a scratch HOME, from
	// a file: `typeset -n r=v` is the reference at 0, while `-rn`, `-ni`,
	// `-nx`, `-nu`, `-na`, `-nl`, `-nt`, the two-word `-n -i` and `-i -n`,
	// and even `+n -i` are each the builtin's bare usage block at 2 with
	// the script ending there. `typeset -Q r=v` is the control: an option
	// this shell has not got writes `typeset: -Q: unknown option` in front
	// of the same block, and the pair writes no such line — the parser is
	// refusing the company rather than a letter (#3171).
	s.NamerefLetterStandsAlone = interp.Yes
	// And the target is settled **at the declaration**: measured 2026-09-18,
	// `a=(x y z); i=2; typeset -n r=a[i]` lists `typeset -n r='a[2]'` and
	// goes on reading `z` after `i=0`, `typeset -n s=u; typeset -n s2=s`
	// lists `s2=u` and stays there when `s` is re-aimed, and `typeset -n
	// r=a[@]` is `@: arithmetic syntax error` with the script ending —
	// because `@` is not an expression and this shell has no target text to
	// keep (#3124, #3172).
	s.NamerefTargetResolvedWhenAimed = interp.Yes
	// And a refusal through a reference names the cell the write would have
	// landed in, whether or not the declaration carried a value: measured
	// 2026-09-18, `u=1; readonly u; typeset -n s=u; typeset s=9` is `u: is
	// read only` here (#3173).
	s.DeclarationThroughAReferenceNamesTheOperand = interp.No
	s.ReadZeroTimeout = interp.ReadZeroTimeoutTakesWhatIsWaiting
	s.ReadPartialCountSucceeds = interp.Yes
	s.ReadExactCountKeepsPartial = interp.No
	s.ReadTimeoutKeepsWhatArrived = interp.No
	s.ReadTimeoutBoundsReadability = interp.No
	// typeset in a keyword function hides the caller's value, as bash's
	// local does.
	s.ValuelessDeclarationHidesTheOuterValue = interp.Yes
	s.TypesetLocalNeedsKeywordFunction = interp.Yes
	// And such a body is scoped *statically*: the name it declared is
	// visible in it and nowhere else, so a function it calls reads the
	// shell's own name and a write there changes the shell's own. ksh93's
	// alone in a panel of six. See
	// [interp.Semantics.CallerLocalsReachTheCallee] (#2865).
	s.CallerLocalsReachTheCallee = interp.No
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
	// And a definition's name ends the script too, at 1 — without the
	// redirection exception the loop has, which is measured rather than
	// assumed: `function _p_${w} { :; } > mf; echo after` stops here where
	// the same shape on a `for` clause carries on.
	s.FunctionNameWhenTheDefinitionRuns = interp.FuncNameEndsTheScript
	// The one shell in the panel that reports *every* option word `set`
	// cannot use before it gives up, with a single usage block after them
	// all. The other four stop at the first, which three of them do because
	// the refusal is fatal there and the loop never reaches the second — so
	// this is a rule of its own here rather than a consequence of the
	// fatality, since this shell's refusal is fatal too and it still prints
	// both lines first (#1170).
	s.SetReportsEveryBadOption = interp.Yes
	// And not only `set`: every builtin here names each letter of a bundle
	// it does not have, with one usage block under the lot. `typeset -Uz f`
	// — which is what `autoload -Uz f` becomes through this shell's own
	// alias — says `-U` and then `-z` (#2345).
	s.BuiltinReportsEveryBadOption = interp.Yes
	// `alias` and `unalias` end the script over an option they do not have,
	// the way a special builtin does, though POSIX marks neither.
	s.AliasBadOptionFatal = interp.Yes
	// The same set bash refuses plus the pattern characters `* ? [ { }`,
	// swept over every printable ASCII character on 2026-09-12. `]` is in
	// neither set, which is what says the extra five are the pattern
	// characters rather than a bracket rule (#2413).
	s.AliasNameRefusedCharacters = "\t\n \"$&'()*/;<>?[\\`{|}"
	// And the check reaches a bare lookup too: `alias 'a$b'` is `invalid
	// alias name` here where bash answers `not found`.
	s.AliasNameCheckReachesALookup = interp.Yes
	// The script ends over it, as it does over an option this builtin does
	// not have — a separate axis, because that one is about a letter and
	// this is a different complaint with a wording of its own.
	s.AliasInvalidNameFatal = interp.Yes
	s.EarlierDeclarationLetterBlocksALaterPlus = interp.Yes
	s.HeredocExpandsInTheCommandsProcess = interp.Yes
	s.RedirectTargetExpandsInTheCommandsProcess = interp.Yes
	s.ArithInvalidOctalDigitIsError = interp.No
	// The integer attribute has a reader of its own, and it is not the
	// arithmetic one: `$((010))` is 8 here and `typeset -i d=010` is 10.
	s.IntegerAssignmentReadsALeadingZeroAsDecimal = interp.Yes
	// And a value read *out of a name* inside an expression is the same
	// second reader: `k=010; $((k))` is 10 where `$((010))` is 8.
	s.ArithStoredValueReadsALeadingZeroAsDecimal = interp.Yes
	// A base is read in plain decimal here, padding and all — `08#7` is 7 —
	// but only two characters of it, which is as many as base 64 needs. That
	// cap is what refuses `010#5`, and it refuses `0002#11` where `02#11` is
	// 3, so it is a length rule and not one about the zero.
	s.ArithBaseMayHaveALeadingZero = interp.Yes
	s.ArithBaseIsAtMostTwoDigits = interp.Yes
	s.ArithBaseZeroReadsTheDigitsAsWritten = interp.No
	// And a radix prefix wants a digit after it: `$(( 0x ))` is refused.
	s.ArithEmptyRadixDigitsAreZero = interp.No
	// And a third site with a third answer: `let` reads *every* numeral in
	// its word in decimal, so `let "x=1+010"` is 11 where the same text
	// stored in a name is 9 and written as a literal is 9.
	s.LetReadsALeadingZeroAsDecimal = interp.Yes
	s.ArithmeticAssignmentDeclaresAnInteger = interp.No
	s.IndirectionYieldsName = interp.Yes
	// And an operator after `${!name[@]}` is a bad substitution here rather
	// than either reading: measured 2026-09-14, `${!w[@]#H}`, `${!w[@]:1:2}`,
	// `${!w[@]/L/x}` and `${!w[@]+SET}` all end the script at 1 where the
	// bare `${!w[@]}` answers the key. The written subscript is what is
	// refused: `${!b#o}` on an array answers `b`, this shell's ordinary
	// reading of `${!x}` (#2821).
	s.OperatorAfterTheSubscriptListingIsBad = interp.Yes
	s.BraceExpansion = interp.Yes
	// A group that does not expand does not end the word — `@{x}{a,b}@` is
	// `@{x}a@ @{x}b@` here too — but the scan resumes past that group's
	// *close* brace rather than past its open, so a list nested inside a
	// failed group is never reached: `{a{b,c}}` and `{a}{b{c,d}}` are left
	// whole where bash and zsh expand them, while `{a{b,c}}{d,e}`, whose
	// list is outside the failed group, still gives `{a{b,c}}d {a{b,c}}e`.
	// An unclosed `{` has no close to step over and ends the scan.
	s.BraceRescanEntersFailedGroup = interp.No
	// The one shell that strips a range endpoint's zeros — `{01..3}` is
	// `1 2 3` — and takes a written step's sign at its word, so `{10..1..3}`
	// is `10` alone and `{1..10..-3}` is `1`. A negative step that agrees
	// with the endpoints keeps their order: `{3..1..-1}` is `3 2 1`.
	s.BraceRangePadsToEndpointWidth = interp.No
	s.BraceRangeStepSignHonored = interp.Yes
	s.BraceRangeNegativeStepReverses = interp.No
	// It agrees with zsh on the one thing bash does not do at all: a
	// range's endpoints are read after the expansions written in them, so
	// `n=3; echo {1..$n}` is `1 2 3`.
	s.BraceRangeEndpointsExpanded = interp.Yes
	// A character range is letters only here as well, and takes a step. The
	// two readings past that are ksh93's alone: a *missing second endpoint*
	// counts from zero, so `{1..}` is `1 0` and `{5..}` is `5 4 3 2 1 0`,
	// while a missing first endpoint or a missing step is no range at all
	// and leaves the word as written. A step of zero is not read as one
	// either — `{1..2..0}` stays whole where bash counts `1 2`.
	s.BraceCharRangeSpansAnyCharacter = interp.No
	s.BraceRangeMissingEndCountsFromZero = interp.Yes
	s.BraceRangeZeroStepCountsAsOne = interp.No
	s.BraceRangeNumberMayCarryAPlus = interp.Yes
	s.BraceRangeThatCannotBeCounted = interp.BraceRangeFailureKeepsTheWord
	s.BracketCaretNegates = interp.Yes
	s.LastPipelineElementInCurrentShell = interp.Yes
	// Nor here. ksh93 refuses `[[ $v == <(cmd) ]]` earlier still — while
	// reading, as `` `<(' unexpected `` — so the answer is the same no and
	// only the moment differs. What this dialect does not yet reproduce is
	// that moment: it reads the word and refuses it at the run.
	s.ProcessSubstitutionInCondition = interp.No
	// With bash and not with zsh on where a substitution's body reads from:
	// `printf "PIPE\n" | cat <(cat)` prints the pipe's PIPE here, not the
	// OUTER the shell itself is reading.
	s.ProcessSubstitutionBodyReadsTheShellsInput = interp.No
	// As zsh: `more tokens expected` and the input is abandoned.
	s.ConditionArithmeticErrorIsFatal = interp.Yes
	// And the parenthesized spelling is fatal too, which is the half the
	// condition axis could not carry: zsh abandons the condition and stays
	// for `(( ))`, so the two constructs do not group.
	s.ArithCommandErrorIsFatal = interp.Yes
	// And a clean zero result is not a failure at all: `set -e; (( 0 ));
	// echo survived` carries on and fires no ERR, where a call whose body
	// ended in one is judged at the call (#3348).
	s.ArithCommandZeroIsAFailure = interp.No
	// And a C-style `for` header, which zsh gives up too where it stays for
	// `(( ))` — see [interp.Semantics.ForHeaderArithmeticErrorIsFatal].
	s.ForHeaderArithmeticErrorIsFatal = interp.Yes
	s.UnterminatedBracket = interp.BracketLiteral
	// And the same question where a `[:name:]`, a `[.x.]` or a `[=x=]`
	// inside it is what left it open: the column that moves: a bare `[` is a literal `[` here and `[[:alpha:]` matches nothing at all.
	s.UnterminatedBracketAfterASubExpression = interp.BracketNoMatch
	// And inside a bracket expression the backslash protects the character
	// behind it and puts nothing of its own in the set: `[\)]` is the
	// one-character set `)`, and `[a\-z]` is the three members a, `-` and z,
	// the escape being what stops the dash reading as the range operator.
	// zsh adds the backslash to the set as well and BusyBox ash protects
	// nothing, so this value is what five of the seven columns share (#3271).
	s.BracketEscape = interp.BracketEscapeProtectsTheMember
	// The parameter whose patterns take names back out of a pathname
	// expansion, spelled `FIGNORE` here and `GLOBIGNORE` in bash. The
	// facility is the same and the model is not: measured on ksh93u+,
	// 2026-09-14, in a directory holding `a.txt`, `b.txt`, `c.log`, `.dot`,
	// `.hid.txt` and `sub`, it is **one pattern** rather than a
	// colon-separated list — `FIGNORE='*.txt:*.log'` takes nothing out —
	// matched against the *entry in the directory* rather than the word, so
	// `FIGNORE='*.txt'` reaches `sub/x.txt` through `sub/*`; and it is read
	// off the parameter where it stands, so a value inherited from the
	// environment works and a null value still shows the hidden names.
	//
	// The hidden names it shows include `.` and `..`, which is the row
	// #2748 was filed on — and that is GlobListsDotAndDotDot below rather
	// than a second rule about this parameter: this shell lists the two
	// names whether or not `FIGNORE` is set, and the leading-period rule is
	// what keeps them out of an ordinary `*`.
	s.IgnoredNamesVariable = "FIGNORE"
	s.IgnoredNamesRevealHiddenNames = true
	s.IgnoredNamesValueIsOnePattern = interp.Yes
	s.IgnoredNamesMatchTheLastComponent = interp.Yes
	s.IgnoredNamesFollowTheParameter = interp.Yes
	// `echo .*` is `. .. .dot` here, where bash 5.3 and zsh answer `.dot`.
	// bash 3.2 and dash agree with this shell, so it is three of the panel
	// against two.
	s.GlobListsDotAndDotDot = interp.Yes
	s.UnknownCharacterClass = interp.UnknownClassEmptiesTheBracket
	// `[[.a.]]` and `[[=a=]]` are the collating element and the equivalence
	// class, matching `a`. One character is the whole of an element here:
	// measured 2026-09-18 under `LC_ALL=C`, `[[.hyphen.]]` matches no `-`,
	// where bash reads that body as a name. A body this shell cannot read
	// empties the bracket the way an unknown class name does: measured
	// 2026-09-16, `[a[.nosuch.]b]` matches neither a nor b.
	s.CollatingElements = interp.OneCharacterIsACollatingElement
	// `[[:]` makes the whole bracket match nothing, wherever the `[:` stands — the same shape this shell gives a class name it has not got.
	// See interp.Semantics.UnterminatedCharacterClass (#1431).
	s.UnterminatedCharacterClass = interp.UnterminatedClassEmptiesTheBracket
	// A value's backslash is **data**, and the metacharacter behind it stays
	// live — this shell against the other five. Measured 2026-09-12 in a
	// directory holding `a\b` and `a*`, `v='a\*'; set -- $v` matches `a\b`
	// here and is left as the word `a\*` everywhere else. Both files are
	// present because a directory holding neither prints the same word under
	// either reading, which is what makes the arrangement discriminating
	// (#1367).
	s.ValueBackslashInAPattern = interp.ValueBackslashIsData
	// The trim ignores the mask and reaches the last name's value however it
	// was arrived at. Measured 2026-09-12: `printf 'a b\\ \n' | read x y`
	// leaves `b` here and `b ` in dash and the three bashes — one field per
	// name, so bash has no remainder to trim and this shell trims anyway
	// (#1360).
	s.ReadTrailingEscapedSeparator = interp.ReadTrailingEscapedSeparatorTrimmed
	// The longest arm, as in the bash column: measured 2026-09-11 on
	// ksh93u+, `x=abc`, `${x##@(a|ab)}` and `${x##@(ab|a)}` are both `c`.
	s.LongestMatchTakesTheWrittenArm = interp.No
	// An empty pattern matches only where there is nothing to scan: measured
	// 2026-09-12 on ksh93u+, `v=abc` and `e=`, `${v///X}` is `abc` and
	// `${e///X}` is `X`, against a control `${e//x/X}` that stays empty. This
	// is the one column of the three that neither declines the pattern nor
	// treats it as an ordinary one.
	s.EmptyReplacementPattern = interp.EmptyReplacementPatternMatchesAnEmptyValue
	// The anchors are here — `v=abcabc` gives `Xbcabc` for `${v/#a/X}` and
	// `abcabX` for `${v/%c/X}` — and an **empty** pattern behind one is
	// declined, which is this shell alone: `${v/#/X}` and `${v/%/X}` are both
	// `abcabc` here where bash and zsh write the `X`. The axis above has
	// recorded that in prose since #1857 and nothing read it until #3272.
	s.ReplacementAnchors = interp.Yes
	// And only after a single `/`: `${v//#a/X}` on `abcabc` is `abcabc`
	// here, and `${w//#a/Q}` on `x#ay%bz` is `xQy%bz` — the `#a` found
	// inside the value, which is what says the character was the pattern's
	// and not an anchor. Measured 2026-09-16; bash and bash 3.2 agree and
	// zsh does not (#3307).
	s.GlobalReplacementAnchors = interp.No
	s.AnchoredEmptyReplacementPattern = interp.No
	// And the empty match it refuses is the one where the match before it
	// ended, which is the classic global-replace rule: `${v//@(b|)/<>}` is
	// `<>a<>c<>` here against `<>a<><>c` in the other two — no replacement
	// between `b` and `c`, and one after the last unit.
	s.ReplacementEmptyMatchDeclined = interp.EmptyMatchDeclinedAfterAMatch
	// Leading digits and no further: `return 3abc` is 3 and `return r` is 0
	// whatever `r` holds. Not arithmetic, which the leading zero settles —
	// `return 010` is 10 here while `$((010))` is 8, so the operand is plainly
	// not going through the arithmetic reader that zsh's does.
	s.StatusArgument = interp.StatusArgLeadingDigits
	s.UnsetPositionalIsAllowed = interp.Yes
	s.TraceShowsItsOwnDisabling = interp.No
	// A traced array literal shows what its elements came to: `x="p q";
	// a=("$x" r)` is `a=( 'p q' r )` here, and the unquoted `a=($x)` is
	// `a=( p q )` — this column's own field splitting, visible in its own
	// trace.
	s.TraceArrayLiteralShowsTheExpandedElements = interp.Yes
	// And a literal whose elements name where their values go is traced as
	// those writes, one line each — see the axis for the table.
	s.TraceSubscriptedArrayLiteralIsElementAssignments = interp.Yes
	// And the subscript is the one the assignment resolved: `i=2; a[$i]=v`
	// is `a[2]=v`, `a[i]=v` is too, and a table's `m[k$x]=v` is `m[k]=v`.
	// The only column that does.
	s.TraceElementSubscriptIsEvaluated = interp.Yes
	s.TraceAssignmentsSeparately = interp.Yes
	// An unparseable `eval` is reported and survived here, unlike dash, but a
	// file `.` cannot open still ends the script — so the two halves of the
	// POSIX "a special builtin's failure is fatal" rule are answered
	// differently, which is why they are two axes.
	s.BuiltinSyntaxErrorFatal = interp.No
	// The only column that reads both through before running either: neither
	// `eval` nor a sourced file leaves anything behind from the lines before
	// the one that will not parse.
	s.EvalRunsWhatItParsed = interp.No
	s.SourcedFileRunsWhatItParsed = interp.No
	// An error inside a file `.` read ends that file and nothing above it:
	// measured, `.` reports 1 and the sourcing file runs the command after
	// it — for an unset parameter under `set -u`, a readonly assignment, a
	// division by zero and a bad substitution alike.
	s.FatalErrorEndsBorrowedTextOnly = interp.Yes
	// But not a builtin's complaint about how it was *called*, which is the
	// one kind that goes straight out: measured 2026-09-15, `eval 'alias -g
	// x=1'` inside a subshell prints nothing and leaves 2, exactly as the
	// bare call does, where `eval 'r=2'` against a readonly `r` reports and
	// lets the next command in the subshell run. The same split holds for a
	// dot script, and it is the same fourteen calls either way — see
	// [interp.Semantics.BuiltinUsageErrorEscapesBorrowedText] for the two
	// lists (#2950).
	s.BuiltinUsageErrorEscapesBorrowedText = interp.Yes
	// And `${x?word}` is one of those errors here rather than a request to
	// stop: `.` reports 1 for it too and the sourcing file carries on, which
	// is the half of this zsh answers the other way.
	s.ParamErrorIsAnExitRequest = interp.No
	s.DotPassesArguments = interp.Yes
	// Reads options and has none to read, which is a different answer from
	// reading the word as a filename: `unknown option` and a usage line.
	s.DotReadsOptions = interp.Yes
	// And `eval` reads one too — which #3216 expected it not to. Measured
	// on ksh93u+ 2012-08-01: `eval -q echo hi` is `eval: -q: unknown
	// option` with `Usage: eval [ options ] [arg...]` under it, at 2, and
	// the script ends there because a special builtin's usage error is
	// fatal here. `eval -- echo hi` prints `hi`.
	s.EvalOptions = interp.EvalReadsOptions
	s.DotTakesTheSearchPathOption = interp.No
	// A directory operand is an error, and a fatal one through
	// DotMissingFileFatal: measured, `. ./` is `.: ./: cannot open [Is a
	// directory]` and the script ends there. No wording of its own — that
	// is the one sentence ksh93 uses for every `.` failure, with the reason
	// filling the bracket, so DotCannotOpen already carries it.
	s.DotDirectoryOperandIsAnError = interp.Yes
	// And a missing operand ends it too, at 2 rather than at this shell's
	// usual fatal 1 — the usage line and nothing after it, measured
	// 2026-09-18 on ksh93u+ 2012-08-01 from a script file. The one column in
	// the panel that stops here. Only the `.` spelling reaches it: `source`
	// is an alias for `command .` in this shell, and the word takes the
	// failure, so `source` with no operand writes the same usage line and the
	// script runs on (#3473).
	s.DotWithNoOperandIsFatal = interp.Yes
	s.ExecFailureRunsExitTrap = interp.No
	s.ExecTakesOptions = interp.Yes
	// `-a` and `-c`, and no `-l`: this shell reports the letter as an
	// option it does not know and ends the script, `exec` being special.
	s.ExecTakesTheLoginLetter = interp.No
	s.ExecTakesTheEmptyEnvironmentLetter = interp.Yes
	s.ExecLoginPrefixesTheGivenName = interp.No
	// A file the kernel would not start is run as a script here as it is
	// everywhere, and `$0` inside it is the word that was typed rather than
	// the path the PATH search resolved. Measured 2026-09-13: with the file
	// on PATH, `ne.scr` reports `ne.scr` here and the resolved path in the
	// other six columns.
	s.ScriptImageSeesTheResolvedPath = interp.No
	s.TestAcceptsDoubleEqual = interp.Yes
	// The same three unary operators bash has past the three-word rules.
	// `set -p` is the short spelling of `set -o privileged`, which this
	// shell lists; `umask -p` it has not got.
	s.SetHasThePrivilegedLetter = interp.Yes
	// `set -s` sorts the operands, or the positional parameters when there
	// are none; see Semantics.SetSLetterSortsTheOperands.
	s.SetSLetterSortsTheOperands = interp.Yes
	s.TestHasTheFileExistsLetter = interp.Yes
	s.TestHasTheShellOptionOperator = interp.Yes
	s.TestHasTheModifiedSinceReadOperator = interp.Yes
	// The column with no operand count: `test x = x y` is 0 here and a
	// refusal in the other six. See
	// Semantics.TestReadsOneExpressionOffTheOperands for the seam that makes
	// it a rule rather than a leniency, and #2959.
	s.TestReadsOneExpressionOffTheOperands = interp.Yes
	// And `>` alone: `test b '<' a` here is `test: <: unknown operator` at 2
	// while `test b '>' a` is 0, which is the split the enum exists for.
	s.TestStringOrder = interp.TestStringOrderGreaterOnly
	s.SignalDeathStatusIsTwoFiftySix = interp.Yes
	s.PipefailOption = interp.Yes
	// And the one place ksh93's 256-plus-the-signal convention stops: an
	// element pipefail substitutes for the last one, and which a signal
	// killed, is reported as the signal's number alone — 13 and 15, not 269
	// and 271.
	s.PipefailSubstitutesTheBareSignal = interp.Yes
	s.ErrexitSeesPipefailFailure = interp.No
	// `time` times its command in a context where neither `set -e` nor the
	// ERR trap judges anything — not the timed command, not what it calls,
	// and not the clause. Measured 2026-09-18: `set -e; time { false; echo
	// in; }; echo survived` writes both lines here and stops at the `false`
	// in bash and zsh, and `trap 'printf E' ERR; time false` writes no E.
	// The status is untouched — `time false` leaves 1 in `$?` — so it is the
	// judging alone.
	s.TimedCommandIsJudged = interp.No
	// And a redirection this shell cannot open on a *compound* command is a
	// failure neither judge sees: the diagnostic is written, the body does
	// not run, 1 is left behind and the script carries on. Measured the same
	// day over a group, a `for`, a `while`, an `if`, a `case`, a subshell and
	// an input redirection; a *simple* command stops here as it does
	// everywhere, which is the control.
	s.CompoundRedirectionFailureIsJudged = interp.No
	// A subshell written as the last element of a pipeline is judged once
	// here — `trap 'printf E' ERR; true | ( false )` writes one E, where bash
	// writes two.
	s.ASubshellAsTheLastPipelineElementJudgesItself = interp.No
	// Alone in the panel: `PATH=` finds nothing here, where dash, bash and
	// zsh still search the current directory.
	s.EmptyPathIsTheCurrentDirectory = interp.No
	s.ExitTrapRunsOnSignalDeath = interp.Yes
	s.QuitIgnoredWhenNotInteractive = interp.No
	// unanswered BuiltinReadsOptions: ksh93's `builtin` is another command,
	// one that registers builtins from a library, and is not the one this
	// axis is asked in (#3217).
	//
	// unanswered QuitResetRestoresTheDefault: that axis is what a reset does
	// to the *ignore* above, and this shell has no ignore to take away — an
	// untrapped QUIT kills it whether or not `trap - QUIT` has been run, so
	// both readings run every script identically and there is nothing to
	// measure a preference from.
	s.HangupIsAnOrderlyExit = interp.No
	s.ExitInTrapReportsEarlierStatus = interp.Yes
	// A subshell that the shell reported an error and gave up on runs no
	// EXIT trap of its own, with or without `set -e` — a readonly
	// reassignment, an unset name under `set -u`, a division by zero,
	// `${x?word}` and a command substitution that would not parse all lose
	// the handler. A builtin's complaint about how it was *called* keeps it:
	// `( trap … EXIT; set -Z )` runs the handler here and does not in zsh,
	// which is the row that makes this three values rather than two. The top
	// level keeps the handler on every one of those rows, which is what makes
	// this a second axis rather than FatalErrorUnderErrexitSkipsTheExitTrap
	// reaching further. Measured 2026-09-18 on ksh93u+ 2012-08-01 (#3612).
	s.SubshellExitTrapAfterAGiveUp = interp.SubshellExitTrapSkippedByAReportedError
	// `kill -n signum` is an option here, and `-s` with nothing after it is
	// an option missing its argument rather than a signal named `s`.
	s.KillReadsTheNumberOption = interp.Yes
	s.KillOptionWithNoArgumentIsASignalName = interp.No
	s.KillListAcceptsName = interp.Yes
	// Subtracts while the number is still 128 or more, so `kill -l 257` is
	// HUP and `kill -l 300` is 44, and prints back what it cannot name.
	s.KillListReducesRepeatedly = interp.Yes
	s.KillListPrintsANumberItCannotName = interp.Yes
	s.KillListNamesZeroAsExit = interp.Yes
	// This shell's signal table is one name short of the machine's on macOS:
	// signal 29 is INFO to every other column and nothing at all here, so
	// `kill -l 29` is `29`, `kill -l INFO` is `INFO: unknown signal name`
	// and `trap 'x' INFO` is `bad trap` — while `trap 'x' 29` is 0 and
	// `kill -29` sends, because a number is the kernel's. Measured
	// 2026-09-17 on ksh93 93u+ 2012-08-01.
	s.SignalNamesTheShellLacks = "INFO"
	// And a signal written onto the option with no space: `kill -n9` and
	// `kill -sKILL` both send. Measured 2026-09-12. This shell is looser
	// still — it takes `kill -s9` too, which the axis records and does not
	// follow (#2227).
	s.KillReadsASignalJoinedToItsOption = interp.Yes
	// A numeric signal goes to `kill(2)` unchecked here, so `kill -99 $$` is
	// `kill: <pid>: no such process` at 1 — the sentence this shell gives
	// every failed send — rather than a word refused. Measured 2026-09-16;
	// `kill -s 99` is still `kill: 99: unknown signal name`, since `-s`
	// takes a name.
	s.KillSendsASignalNumberItCannotName = interp.Yes
	s.SIGPrefixAccepted = interp.Yes
	s.RedirectsUseEveryTarget = interp.No
	s.KillStatus = interp.KillStatusAnyFailure
	s.SubshellJobTable = interp.SubshellJobsKept
	s.PrintfOutputPrecedesComplaint = interp.Yes
	s.PrintfEmptyIsNotANumber = interp.No
	s.PrintfAbsentNumberIsAnEmptyOne = interp.No
	s.PrintfStarWithoutOperandIsRefused = interp.Yes
	s.PrintfStarComplaintCostsTheStatus = interp.Yes
	// Unmeasurable rather than measured — ksh93 reads `inf` through its
	// arithmetic evaluator and never reaches the conversion — so it takes
	// C's reading, which is what its field and its flags already do.
	s.PrintfNonFiniteIsConverted = interp.Yes
	// The `'` flag, and this shell alone reads it wherever it is written in
	// the prefix: `%15'd` and `%.5'd` are accepted here and refused by the
	// other two that have the flag at all.
	s.PrintfGroupingFlag = interp.Yes
	s.PrintfGroupingFlagAfterTheWidth = interp.Yes
	// And a `*` may stand beside a width's own digits, where it wins:
	// `printf '[%5*d]' 4 42` is `[  42]` here (#2824).
	s.PrintfStarBesideTheFieldDigits = interp.Yes

	// Wraps into a C int exactly as zsh does, both spellings measured
	// 2026-09-15.
	s.PrintfFieldBeyondAnInt = interp.PrintfFieldWrapsToAnInt

	// Wraps on both routes, as zsh does. Measured 2026-09-15.
	s.PrintfStarBeyondAnInt = interp.PrintfStarWrapsToAnInt
	// A `case` does not advance the line until its subject is expanded, so
	// the subject reads the line of the command in front of it (#2818).
	s.CaseSubjectKeepsThePreviousLine = interp.Yes
	// A third segment in a substring range is a bad substitution here, named
	// after the whole word: `${x:1:5:t}` (#2818).
	s.SubstringRangeThirdColonIsABadSubstitution = interp.Yes
	s.PrintfReportsBadNumber = interp.No
	s.PrintfNumberOperand = interp.PrintfNumberArithmetic
	// C99's three, with a default precision of its own: `printf '%a' 1.5`
	// is `0x1.800000000000p+0` in ksh93u+ where bash and dash write
	// `0x1.8p+0`, and `printf '%a' 0.1` is `0x1.99999999999ap-4` — twelve
	// digits, the thirteenth rounded away rather than padded.
	s.PrintfC99FloatConversions = interp.Yes
	s.PrintfHexFloatDefaultIsTwelveDigits = interp.Yes
	// And a `%a`'s zero fill goes in front of the `0x` here, where the two
	// columns that share this shell's alternate-prefix arithmetic put it
	// between the prefix and the digits.
	s.PrintfHexFloatZeroFillPrecedesThePrefix = interp.Yes
	// An integer operand is carried in this shell's floating type, so one
	// past 2^53 comes back rounded: `printf '%d' 123456789012345678` is
	// `123456789012345680` (#2907).
	// A flag written past a field restarts the scan, the same grammar the
	// `'` already has here -- and a `-` past a *precision* clears the
	// precision instead of becoming a flag (#2910).
	// A division by zero leaves a value behind and the evaluation carries
	// on with it: 0 for `/`, the dividend for `%` (#2912).
	s.ArithDivisionByZeroYieldsAValue = interp.Yes
	s.PrintfFlagAfterTheField = interp.Yes
	s.PrintfIntegerOperandGoesThroughTheFloatingType = interp.Yes
	s.PrintfRefusedOperandKeepsItsLeadingNumber = interp.Yes
	// A floating conversion evaluates its operand twice and the complaint
	// escapes both times: `printf '%f' 42abc` writes the arithmetic line
	// twice where `printf '%d' 42abc` writes it once (#2823).
	s.PrintfFloatOperandIsEvaluatedTwice = interp.Yes
	s.PrintfBackslashC = interp.PrintfBackslashCControl
	// `printf 'a%5'` is `a%` here and reports success: the unfinished
	// conversion becomes one literal character and the prefix is dropped.
	s.PrintfUnfinishedConversionIsAPercent = interp.Yes
	// Every digit that follows, and more than two of them make the value a
	// code point rather than a byte: `\xff` is one byte and `\x0ff` is
	// U+00FF in UTF-8. An empty digit run is a zero.
	// unanswered PrintfReportsAMissingHexDigit: an empty digit run is a zero
	// here and the escape never stands, so there is nothing for the complaint
	// to be about. The two columns that do reach it split, and
	// TestPrintfMissingHexDigit pins the pair (#3239).
	s.PrintfHexEscape = interp.PrintfHexEscapeCodePoint
	// And no `\x` at all in a `%b` argument, which is this shell alone and
	// the reason the two sites are two axes: `printf '%b' 'a\x41Z'` is the
	// six characters as written where the same escape in a format is an `A`.
	s.PrintfBHexEscape = interp.PrintfHexEscapeAbsent
	// `\u` and `\U` in a format, and an empty digit run ends that pass over
	// it — this shell alone, and the reading no `\x` anywhere in the panel
	// has: `printf '[%s]\uZ' x y` is `[x][y]`, so the pass ends and the loop
	// over the operands does not.
	s.PrintfUnicodeEscape = interp.PrintfUnicodeEscapeCodePointOrTruncate
	// And no `\u` at all in a `%b`, the same split this shell's `\x` makes:
	// `printf '%b' 'a\u0041Z'` is the ten characters as written.
	s.PrintfBUnicodeEscape = interp.PrintfUnicodeEscapeAbsent
	// A code point the locale's encoding cannot hold is written all the
	// same: this shell never consults a locale for the escape, which is a
	// third answer beside writing the escape back and refusing it. Measured
	// 2026-09-11 under `LC_ALL=C`, bytes read with `od`: `printf 'a\u00e9Z'`
	// and `$'a\u00e9Z'` are `61 c3 a9 5a` there and in a UTF-8 locale alike,
	// where bash leaves `a\u00E9Z` standing and zsh refuses both. It is
	// reachable at those two sites only, since this shell reads no `\u` in
	// `echo`, in `print` or in a `%b` (#2021).
	s.UnicodeEscapeOutsideTheLocale = interp.OutsideLocaleEscapeEncoded
	// In a *format* this shell takes both letters: `printf 'a\eZ'` and
	// `printf 'a\EZ'` are each `61 1b 5a` in 93u+ 2012-08-01 (#3225). Its
	// two sites disagree, which is what says the format's pair is not the
	// `%b` pair under another name — one field could not hold `\e` yes here
	// and no there.
	s.PrintfEscEscape = interp.Yes
	s.PrintfCapitalEscEscape = interp.Yes
	// An escape this shell's format does not define is written without its
	// backslash: `printf '[\q][\z][\8][\-]'` is `[q][z][8][-]`, where
	// the other five columns write the backslash too. The `%b` site is not
	// this one and keeps it — `printf '%b' '[\q]'` is `[\q]` here as it is
	// everywhere.
	s.PrintfUnknownEscapeDropsTheBackslash = interp.Yes
	// And a floating conversion's exact half goes away from zero rather
	// than to the even neighbor — `printf '%.0f %.0f %.0f' 2.5 4.5 -2.5`
	// is `3 5 -3` here against `2 4 -2` in the other five, and `%.2f` of
	// 0.125 is `0.13` against `0.12` — but only above a tenth: `%.3f` of
	// 0.0125 is `0.012` here and `0.013` there, and `%.4f` of 0.09375 is
	// `0.0937` against `0.0938`. Both arms are this one constant because
	// they are one shell's one rule. Measured 2026-09-18 under `LC_ALL=C`
	// from a script file.
	s.PrintfFloatHalf = interp.PrintfFloatHalfAwayFromZeroAboveATenth
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
	// Bytes, with or without an `l`: `printf '[%.2s|%.2ls]' αβγ αβγ` is
	// `[α|α]` under a UTF-8 locale, ksh93u+, 2026-09-16. Its `%c` taking a
	// character there is a different question and is not modeled.
	s.PrintfFieldCountsCharacters = interp.No
	s.PrintfLongModifierCountsCharacters = interp.No
	// ksh93's `%T` is a different conversion under the same letter: its
	// operand is a date *string* and a number earns a warning and the current
	// time. Not the one bash has, and not modeled — see
	// docs/spec/semantics.md.
	s.PidListingFinishesWithAJob = interp.Yes
	s.PrintfTimeConversion = interp.Yes
	s.PrintfTimeOperandIsADateString = interp.Yes
	s.PrintfQuote = interp.PrintfQuoteSingle
	// The one column that does not read C's `#` off the value: `printf
	// '%#x' 0` is `0x0` here and `0` in the other six, and `printf
	// '%#.0o' 0` is nothing here and `0` in the other six. The prefix
	// follows the digits instead — written wherever there are digits.
	s.PrintfAlternateFormAsksTheValue = interp.No
	// And the one column that does not count that prefix against the
	// width either: `printf '%#05x' 7` is `0x00007` here, seven
	// characters, where the other six write `0x007` (#3066). The width
	// is the digits' alone and the `0x` is written past it.
	s.PrintfZeroFillCountsTheAlternatePrefix = interp.No
	// And the `0` flag survives a precision here, where C ignores it: the
	// precision is applied and the field is then filled with `0` rather
	// than with blanks. See the axis for the table.
	s.PrintfZeroFlagSurvivesAPrecision = interp.Yes
	// The same `\c` as the printf format, and the arithmetic is bit 6
	// toggled rather than bash's five-bit mask: `$'\c1'` is `q`, not 0x11.
	s.DollarSingleBackslashC = interp.DollarSingleControlToggled
	// `\C` is the same arithmetic under a second spelling, and it takes **no
	// dash**: `$'\CA'` is 01, `$'\Ca'` is 01 too, and `$'\C-A'` is the
	// escape applied to `-` — `m` — followed by a literal `A`. zsh writes
	// those same five characters and means 01 by them, so a yes/no field
	// could not hold both and this shell sat unanswered instead. `\M` is not
	// an escape by itself and `\M-` is the escape byte, taking nothing after
	// it: `$'\M-x'` is 1b then `x`. Measured 2026-09-13 through `od -c`
	// (#2345).
	s.DollarSingleCaretMeta = interp.DollarSingleCaretMetaFoldedWithNoDash
	// The plain C escapes `\e`, `\E` and `\?` are all here — `1b`, `1b` and
	// `3f` — and so are both Unicode spellings: $'\u0041' and $'\U00000041'
	// are `41`. Measured 2026-09-16 by `od` (#3270). BusyBox ash is the one
	// column that has none of the five, which is what made them axes.
	s.DollarSingleEscEscape = interp.Yes
	s.DollarSingleQuestionEscape = interp.Yes
	s.DollarSingleUnicodeEscapes = interp.Yes
	s.DollarSingleUnknownEscape = interp.DollarSingleUnknownDropsBackslash
	s.DollarSingleNul = interp.DollarSingleNulEndsTheSpan
	// `\x` here takes every hexadecimal digit that follows and a run past
	// two is a code point, so `$'\x00b'` is the one byte 0x0b where the
	// other shells read `\x00` and truncate. A run with no digit at all is
	// a zero byte, which this shell's truncation then makes into nothing.
	s.DollarSingleHexReadsEveryDigit = interp.Yes
	s.DollarSingleDigitlessEscapeIsAZeroByte = interp.Yes
	// An octal escape past 255 keeps the low byte, as in bash: `$'\401'` is
	// 01. Measured 2026-09-18 by `od` (#3415).
	s.DollarSingleOctalPastAByteDropsTheLastDigit = interp.No
	s.GetoptsAssignmentRestartsWord = interp.Yes
	// `kill %1` reaches the job's process. dash aims at the group.
	s.KillJobSpecAimsAtTheGroup = interp.No
	// A trim on `$@` runs over each field, as it does in bash.
	s.OperatorDistributesOverTheFieldList = interp.Yes
	// bash's answer here: OPTIND names the word until its last letter.
	s.GetoptsCountsTheWordAtItsFirstLetter = interp.No
	// Counted at its last letter here too, and the name gets a `?` when the
	// scan runs out (#3275).
	s.GetoptsCountsTheWordOnTheNextCall = interp.No
	s.GetoptsEndOfOptionsNamesIt = interp.Yes
	// A `+`-prefixed word is an operand and ends the scan: measured
	// 2026-09-17 on ksh93u+ 2012-08-01, `getopts a o +a` is 1 with the name
	// `?` — which is worth measuring rather than inheriting, this shell's own
	// `set` and `typeset` reading `+x` as the sense of `-x`.
	s.GetoptsTakesAPlusPrefixedOption = interp.No
	// The `letter#` numeric type, which is this shell's alone: `getopts 'n#' o`
	// reads `-n 5` and `-n5` as the number 5 where the other three read `#` as a
	// second option letter. Measured 2026-09-16 on ksh93u+ 2012-08-01 (#2947).
	s.GetoptsOptionStringHasANumericType = interp.Yes
	// OPTERR is an ordinary variable here: measured 2026-09-17 on ksh93u+
	// 2012-08-01, a bad option is one line on stderr with it set to 0 as with
	// it set to 1.
	s.GetoptsOptErrSilencesTheComplaint = interp.No
	s.GetoptsClearsOptarg = interp.No
	s.GetoptsEmptiesOptargForAnArgumentlessOption = interp.No
	// OPTARG and OPTIND are the builtin's own here: `readonly OPTARG;
	// getopts a: o` says nothing at all and leaves `val` in OPTARG.
	// Measured 2026-09-16 on ksh93u+ 2012-08-01.
	// The end of the options unsets OPTARG here too, and the freeze stays on:
	// `OPTARG=written` after the loop is still refused. It reaches that
	// through the answer below rather than through an unset of its own —
	// OPTARG is the builtin's, so the clearing is not refused, and the
	// attribute is nobody's to remove. Measured 2026-09-16 on ksh93u+
	// 2012-08-01.
	s.GetoptsUnsetsOptargAtEndOfOptions = interp.Yes
	s.GetoptsClearingOptargIsARealUnset = interp.No
	s.GetoptsOwnParametersIgnoreAFreeze = interp.Yes
	// Unreachable while the answer above is yes — answered so that nothing
	// reports an axis this shell cannot be asked.
	s.GetoptsRefusedWriteEndsTheBuiltin = interp.No
	// The **name** is not the builtin's, so a freeze does refuse it — and
	// this shell is the one column that answers the refusal by which path
	// reached it. `N=kept; readonly N; getopts a: N` reports and returns 2
	// where the scan found `-a`, and ends the script at 2 where it had run
	// out. Measured 2026-09-17 over a script file, through `-c` and through
	// standard input, and for all three ways of running out — a non-option
	// word, a `--` and no words at all — with the silent form answering the
	// same.
	s.GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal = interp.Yes
	// And `read` does not stop either: every frozen name is reported and
	// every other name is filled — `readonly a; printf 'x y\n' | { read a b; }`
	// leaves b holding `y` here, where bash, dash and BusyBox ash leave it
	// alone. The builtin reports 1 for it, whatever the read itself did, and
	// reports it once however many names were frozen (#3208).
	s.ReadRefusedWriteEndsTheBuiltin = interp.No
	// Unreachable while the answer above is no: the builtin never stops, so
	// there is no early stop to part from a failed write. Answered so that
	// nothing reports an axis this shell cannot be asked.
	s.ReadRefusedWriteIsOneOnTheLastName = interp.No
	// A frozen *name* is refused, and the refusal is not fatal: `read`
	// writes `warning: x: is read only` and the script runs on.
	s.ReadonlyRefusalInABuiltinIsFatal = interp.No
	// The scan position is the call's own — OPTIND and the place inside a
	// word both, reset on the way in and the caller's put back at the
	// return — for a function written with the `function` word, and shared
	// with the caller for one written `name() { … }`, which is why a
	// POSIX-form option-parsing helper here has to reset OPTIND itself
	// (#3321).
	s.GetoptsFunctionPosition = interp.GetoptsFunctionPositionIsLocalToAKeywordFunction
	// Reached through `typeset` in a function defined with the `function`
	// word, this shell having no `local`: the caller's position inside a
	// clustered word comes back with the number.
	s.GetoptsLocalOptindRestoresTheCursor = interp.Yes
	// Measured 2026-09-12: `OLDPWD=/nonexistent ksh -c 'echo $OLDPWD'` answers
	// the path it was given, and `cd -` then names it — this shell judges the
	// value when something tries to use it and not before.
	s.InheritedOldpwd = interp.InheritedOldpwdTaken
	// The depth, counted and told to every child, with no ceiling —
	// ksh93u+ 2012-08-01 answers as zsh does. It additionally gives the
	// name the integer attribute, which is #3099's row and not this one.
	// See interp.ShellLevelPolicy.
	s.ShellLevel = interp.ShellLevelCounted
	// And a shell that replaces this process counts one deeper, which is the
	// other side of that split: measured 2026-09-18, `exec /usr/bin/env`
	// hands over `SHLVL=1` untouched and the shell it starts reads 2.
	s.ShellLevelExec = interp.ShellLevelExecCounted
	// And the name this shell gives the directory it starts in: a `PWD` it
	// was handed is kept whatever it says, and with none handed over the
	// directory is named under `$HOME` where it sits there — where three of
	// the panel simply ask the kernel. Measured 2026-09-13 under the macOS
	// `TMPDIR`, which is reached through a symbolic link.
	s.StartupPwdName = interp.StartupPwdNameFromTheEnvironmentOrHome
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
	// And every call is a second place it fires, whatever the trap was
	// doing when the call began: with the trap carried into functions as
	// well, `f(){ g; }; g(){ h; }; h(){ false; }; f` writes four E lines —
	// the failure and each of the three calls — where bash writes one and
	// zsh writes one.
	s.ErrTrapRefiresForTheCommandItFiredInside = interp.ErrTrapAlwaysRefires
	// A pipeline whose last element ran here is judged by that element's own
	// status, and a builtin or a function there is judged twice: `true |
	// false` writes EE and `true | /usr/bin/false` writes E, and `set -o
	// pipefail; false | true` writes nothing (#2921).
	s.FailingPipelineWhoseLastElementRanHere = interp.PipelineJudgedAsItsLastElement
	s.DebugTrapRunsInsideCalls = interp.Yes
	s.DebugTrapRefiresOnEnteringAFunction = interp.No
	// The same heads as the bash columns, and two measured departures: the
	// menu loop's head repeats with its replies the way the list loop's
	// repeats with its words, and an arithmetic `for`'s initializer or step
	// that the script did not write fires nothing.
	s.DebugTrapCompoundHeads = interp.DebugTrapHeadsEveryPassAndWrittenParts
	// And a pipeline has no rule of its own here: the trap is carried into
	// a subshell, so each element fires wherever it runs, with that
	// element's redirections already in place. Measured: `trap 'echo d'
	// DEBUG; echo a | tr a-z A-Z` writes `d D A`, and the upper-case `D`
	// is the first element's own action written down the pipe.
	s.DebugTrapPipelines = interp.DebugTrapPipelineInEachElement
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
	// An empty operand is a limit of nought, silently, and leaves the limit
	// at 0 afterwards (#3064). Measured 2026-09-18.
	s.UlimitEmptyOperandIsZero = interp.Yes
	// CDPATH is the **whole** of how a relative operand is resolved here: a
	// search that misses is `cd: target: [No such file or directory]` at 1
	// with the directory right there, and a `./` operand is searched too
	// rather than exempted (#2896). Measured 2026-09-18.
	s.CdpathReplacesTheRelativeLookup = interp.Yes
	s.UlimitHasResidentSet = interp.Yes
	s.UlimitHasProcessCount = interp.Yes
	s.UlimitSetsBothLimits = interp.Yes
	// No keyword either way here; the operand is an expression instead,
	// so `hard` is a parameter and the complaint is that it is not set.
	s.UlimitTakesHardKeyword = interp.No
	s.UlimitTakesSoftKeyword = interp.No
	s.UlimitOperandIsArithmetic = interp.Yes
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
	// ksh93 has the move operator too and reads it as one operation: the
	// descriptor changes number. So both halves are the command's redirection
	// and both are taken back — `exec 5<f; true 6<&5-` leaves 5 open, where
	// bash leaves it closed — and the number the source gives up is the one
	// `{v}<&$w-` receives, so the name answers with `$w`'s own number.
	s.FdMove = interp.FdMoveRelocates
	s.DuplicationTargetError = interp.DuplicationTargetErrorCarriesOn
	// Fatal to `export` and `readonly` and not to `unset`, which prints the
	// same kind of complaint, returns 1 and carries on. Not `unset` being
	// less special: a bad *option* to it is fatal, just above.
	s.BadNameToDeclarationFatal = interp.Yes
	s.BadNameToUnsetFatal = interp.No
	// And not to `read`, which is not a special builtin in any shell.
	// unanswered BadNameToPrintfFatal: this shell's `printf` has no `-v` —
	// measured 2026-09-17, `printf -v '1x' %s Q` is `printf: -v: unknown
	// option` and a usage line at 2 — so it never reaches an operand to
	// judge.
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
	// A count stops it judging past the first operand, though:
	// `read -n 3 a 1bad` is quiet here and `read a 1bad` is not, measured
	// with both count letters. The first operand is still judged —
	// `read -N 3 1bad` is refused — so it is the names after it that a
	// count releases.
	s.ReadCountJudgesTheNamesAfterTheFirst = interp.No
	s.DeclarationTakesASubscript = interp.Yes
	// And the letter lands on the array: measured 2026-09-17,
	// `a=(1 2 3); export 'a[1]'=v` leaves `typeset -x -a a` and puts `a=1`
	// in a child's environment, and the valueless spelling agrees.
	s.ExportThroughASubscriptedOperandRecordsTheLetter = interp.Yes
	// The declaration builtins take one as well — `typeset a[1]=v` creates
	// the element — so this shell gives the two the same answer where bash
	// splits them.
	s.TypesetTakesASubscript = interp.Yes
	s.UnsetTakesASubscript = interp.Yes
	// And an element removed from an array a subshell has only inherited
	// takes the whole array with it, for the rest of that subshell:
	// measured 2026-09-17, `a=(1 2 3); ( unset "a[1]"; echo "[${a[*]}]" )`
	// prints `[]` here and `[1 3]` in the other two columns, with the
	// parent's array whole in all three. A write in front of it makes the
	// unset ordinary, and an array the subshell made itself is ordinary.
	s.UnsetElementEmptiesAnUnwrittenArrayInASubshell = interp.Yes
	// `read 'a[2]'` fills the element, measured 2026-09-10 on `a=(x y z)`.
	s.StoreOperandTakesASubscript = interp.Yes
	// This column really does evaluate the brackets, and `@` is not an
	// operand: measured 2026-09-17, `r=(1 2 3); read 'r[@]'` is
	// `read: @: arithmetic syntax error` at 1 with the array whole and the
	// rest of the line still running. `r[*]` answers the same way. That is
	// not what the same shell says to the *assignment* `r[@]=Z`, which is
	// `@: invalid subscript in assignment` and ends the input — the reason
	// this is a field of its own rather than the assignment's read twice.
	s.StoreOperandWholeArraySubscript = interp.StoreOperandWholeArraySubscriptIsAnExpression
	// And over a **table** the brackets are a key rather than an
	// expression, so the operand stores one: measured 2026-09-17,
	// `typeset -A m; m[k]=v; read 'm[@]'` on `Z` is status 0 with the keys
	// `@` and `k` standing. This column swaps sides between the operand and
	// the assignment — `m[@]=Z` is `@: invalid subscript in assignment` and
	// ends the input — which is why the operand has a field of its own.
	s.StoreOperandWholeArraySubscriptOverATable = interp.WholeArraySubscriptIsAnOrdinaryKey
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
	// And the container letter, taken here too — what this shell then does
	// with the subscript is TableLetterReachesItsOwnOperandsSubscript and
	// not this question.
	s.SubscriptedOperandTakesTheContainerAttribute = interp.Yes
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
	// Brackets with nothing between them hold an expression that is empty,
	// and an empty expression is zero — so the operand is element zero and
	// not the number zero. Measured 2026-09-10: `a=(5 6 7); $(( a[] ))` is 5,
	// `typeset -A m; m[""]=4; $(( m[] ))` is 4, and `(( a[]++ ))` steps the
	// first element. The stream stays clean and the status stays 0, which is
	// the only one of the three answers the panel gives that says nothing.
	s.EmptyArithSubscript = interp.EmptyArithSubscriptIsTheEmptyExpression
	// The one column that reads it at the parameter site too, and by the same
	// rule: the brackets hold an expression that happens to be empty, so
	// `a=(5 6 7); ${a[]}` is `5`, `s=hi; ${s[]}` is `hi` and `${m[]}` is the
	// value under the empty key (#1763).
	s.EmptyParamSubscriptIsAnError = interp.No
	// An empty key is a key: measured 2026-09-12, `typeset -A m; m[""]=4`
	// leaves one element under the empty key here and is refused in bash
	// (#1938).
	s.EmptyAssociativeKeyIsAnError = interp.No
	// The two directions part here, which is why they are two axes. Measured
	// 2026-09-12: `typeset -A h; h[k]=v; typeset -a h` is `typeset: cannot
	// change associative array h to index array` and the **script ends**,
	// where `typeset -a a=(x y); typeset -A a` converts and carries the
	// elements over as the keys `0` and `1` — `typeset -A a=([0]=x [1]=y)`,
	// with `${a[0]}` reading `x` afterwards (#1375).
	s.TableUnderAnArrayDeclaration = interp.CompoundKindChangeEndsTheScript
	s.ArrayUnderATableDeclaration = interp.CompoundKindChangeKeepsTheElements
	// And the literal form converts in both directions without a word, which
	// is what makes it a second pair of fields rather than a widening of the
	// first: this shell ends the script over the valueless array letter and
	// takes the identical letter carrying a literal. Measured 2026-09-12,
	// `typeset -A h; h[k]=v; typeset -a h=(x)` lists `typeset -a h=(x)` at 0
	// and `typeset -a a=(x y); typeset -A a=([k]=v)` lists `typeset -A
	// a=([k]=v)` at 0 -- the old elements gone in both, where the valueless
	// table letter carries them across (#2287).
	s.TableUnderAnArrayLiteralDeclaration = interp.CompoundKindChangeEmptiesTheName
	s.ArrayUnderATableLiteralDeclaration = interp.CompoundKindChangeEmptiesTheName
	// A literal of **bare words** on a table is a third question again, and
	// the one this shell answers alone: it reads the parentheses as an index
	// array's value and will not put one in a table. Measured 2026-09-13,
	// `typeset -A m=(alpha one)` is `cannot append index array to associative
	// array m` and **the input ends**, where bash and zsh both pair the words
	// off and list `[alpha]=one`. The same for `m=(alpha one)` on a name
	// already *declared and empty* and for the `+=` spelling; a literal with
	// no element written in it — `typeset -A m=()` — is taken, and a
	// replacing literal onto a table that **has** an element converts the
	// name instead of complaining. The axis carries those rows (#2611).
	s.BareElementsInATableLiteralEndTheScript = true
	// `a[@]=Z` is refused for either kind of name, in a sentence about the
	// *subscript* rather than about the name, and the input ends under both
	// separators. The only column that answers the two questions alike.
	// Measured 2026-09-12, `x=(p q); x[@]=Z` and `typeset -A m; m[k]=v;
	// m[@]=Z` are both `@: invalid subscript in assignment` (#2285).
	s.WholeArraySubscriptAssigningAnArray = interp.WholeArraySubscriptIsInvalidInAnAssignment
	s.WholeArraySubscriptAssigningATable = interp.WholeArraySubscriptIsInvalidInAnAssignment
	// And reading one says nothing either: measured 2026-09-12, `typeset -A
	// m; m[k]=v; w=; ${m[$w]}` is the empty string at status 0 with no
	// diagnostic, where bash names the table (#1972).
	s.EmptyAssociativeKeyIsReportedWhenRead = interp.No
	// And its *length* is the `0` an absent element gives, silently:
	// measured 2026-09-12, `typeset -A m; m[k]=v; w=; echo "[${#m[$w]}]";
	// echo after` is `[0]`, `after` and status 0, with zsh and against bash
	// (#2286).
	s.EmptyAssociativeKeyRefusesTheLength = interp.No
	// The bash column's answer for whether an empty positional list is a set
	// parameter: measured 2026-09-12, `set --; "${@-word}"` is `word` here
	// and `${@=abc}` is `${@=abc}: bad substitution` because the operator
	// fires at all (#1941).
	s.PositionalListWithNoneIsSet = interp.No
	// This shell's own order for a frozen name in a prefix, and it is
	// neither of the two this axis began with: the name is checked ahead of
	// everything the command does in front of a **function** and a **special
	// builtin**, and the redirections are opened first in front of a regular
	// builtin and an external (#1943, #3314).
	//
	// That is the same split Semantics.PrefixExpandedBeforeTheRedirections
	// draws below and the same set this shell keeps a prefix for, which is
	// what says it is one persistence rule read twice rather than two rules
	// that agree. Measured 2026-09-18, a script file under `env -i
	// PATH=/usr/bin:/bin LC_ALL=C` with standard input on /dev/null, each
	// line its own `( … )` with `readonly V=0` in front and `> /nope/f`
	// behind: `:`, `typeset q=2`, `command eval :`, `command :` and a
	// function all write `V: is read only` and never the file's complaint,
	// where `print hi` and a name nothing answers for write the file's
	// alone at 1.
	//
	// The issue's case is the first of those through a redirection that
	// *succeeds*: `V=1 export > /dev/null 2>&1` writes the complaint here
	// because the check happens before the `2>&1` is in force.
	s.PrefixToAFrozenNameIsCheckedFirst = interp.FrozenPrefixCheckedFirstWhereItPersists
	// The same as the bash column: measured 2026-09-11 on ksh93u+,
	// `typeset -A m; m[k]=9; $(( m[*] ))` is 0.
	s.ArithWholeArraySubscriptIsTheSlice = interp.No
	// And it is not reported either: the brackets hold a text that is no
	// expression and the arithmetic says so, abandoning it. Measured
	// 2026-09-12, `a=(3 4 5); $(( a[*] ))` is `*: arithmetic syntax error`
	// at status 1, where bash reports and answers zero (#1978).
	s.ArithWholeArraySubscriptIsReportedAsBad = interp.No
	// And the same for a subscript that expanded to nothing, which this
	// shell reads as the empty expression exactly as it reads the written
	// `${a[]}`: measured 2026-09-11 on ksh93u+, both are element zero.
	s.EmptySubscriptTextIsAMathError = interp.No
	// The comma is the operator here too, and there are no ranges for the
	// question to be about (#2160).
	s.SubscriptExpressionStopsAtASeparator = interp.No
	// And so is whitespace between them, which is the same reading one text
	// further along: measured 2026-09-10, `a=(1 2 3); echo $(( a[ ] ))` is
	// `1` and `(( a[ ] = 9 ))` writes element zero. The two texts coincide
	// here, and they do not in bash, which is why they are two axes (#1762).
	s.BlankArithSubscriptIsTheEmptyExpression = interp.Yes
	// And the complaint is the builtin's: `unset` reports 1 and the script
	// goes on, which is what makes `unset a[@]` survivable here.
	s.BadSubscriptToUnset = interp.BadSubscriptReported
	// The store behind `read 'r[…]'` answers the same way, and it is the one
	// column where the two questions have the same answer as bash's pair do:
	// measured 2026-09-17, `read 'r[1/0]' <<< Y; echo same-line` writes the
	// complaint, 1, and then `same-line`.
	s.BadSubscriptToAnOutputOperand = interp.BadSubscriptReported
	// A *declaration* is the one of the three that ksh93 does not survive,
	// and it is why the declaration is a field of its own: measured
	// 2026-09-17, `typeset "a[b c]"=v` in a script file writes `typeset: b c:
	// arithmetic syntax error` and the script ends at 1, where the identical
	// expression handed to `unset` or to `read` two lines earlier left a
	// failed builtin behind and ran the very next thing. `readonly` and
	// `export` take a subscript here as well and answer the same (#3495).
	// The construct catches it and this shell's `(( ))` failure ends the
	// input, which is what it does for any other expression it cannot
	// evaluate: measured 2026-09-17, `(( a[b c] ))` is `b c: arithmetic
	// syntax error` and exit 1 with nothing after it, by both routes.
	s.BadSubscriptEscapesAnArithmeticCommand = interp.No
	s.BadSubscriptToADeclaration = interp.BadSubscriptEndsTheScript
	// And this is the third answer to what a *valueless* subscripted operand
	// does: the brackets are read and no element is written. Measured
	// 2026-09-17, `a=(1 2 3); typeset 'a[1]'` leaves the array exactly as it
	// found it where `typeset 'a[1]='` replaces an element, and yet `typeset
	// 'a[b c]'` ends the script and `i=0; typeset 'a[i++]'` moves i. bash
	// reads nothing and zsh writes the element, so ksh93 is neither (#3501).
	s.ValuelessSubscriptedOperand = interp.ValuelessSubscriptedOperandReadsTheSubscript
	// And it reads the brackets whether or not it has the name: `unset a
	// "a[x+]"` complains and reports 1 where bash and zsh are silent at 0,
	// and `i=0; unset "nodecl[i++]"` leaves i at 1.
	s.UnsetSubscriptSkippedWhenNameUnset = interp.No
	// The failure is kept: `unset "a[x+]" "a[1]"` is 1 here and 0 in zsh.
	s.UnsetStatusIsTheLastSubscripts = interp.No
	// A negative subscript is read against the elements the name has, and a
	// name holding a string has none: `a=abc; a[-1]=x` is `subscript out of
	// range` where the write over a non-negative subscript promotes.
	s.NegativeSubscriptCountsOverAPromotedScalar = interp.No
	// A negative subscript past the first element is refused here too, and
	// the refusal ends the script.
	s.NegativeSubscriptPastTheStartInserts = interp.No
	s.SubscriptBeforeTheFirstElementRead = interp.SubscriptBeforeStartEndsTheScript
	s.SubscriptBeforeTheFirstElementNeedsAnElement = interp.Yes
	s.OperandSubscriptQuoting = interp.OperandSubscriptBackslashQuotes
	s.ArithmeticOnlyBodyIsAnArithmeticExpansion = interp.Yes
	s.BareExitReportsTheUnitsOwnStatus = interp.Yes
	// `a[1]=(p q)` makes the element an array of its own — the array keeps
	// its length and the element stops being a string. The one dialect that
	// does; bash refuses the line and zsh splices the words in.
	s.SubscriptedArrayLiteral = interp.SubscriptedArrayLiteralNests
	// `typeset: a+: invalid variable name` — the append operator is not a
	// declaration operand here.
	s.DeclarationTakesAnAppendOperand = interp.No
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.Yes
	// `fg` and `bg` in a script are refused for want of job control before
	// the operand is read, exactly as in bash and zsh — this shell is only
	// the one that says nothing while doing it, and silence is what made it
	// look like the other half of the axis (#2657). The probe that tells
	// them apart: `sleep 0 & fg %2` names a job that does not exist and is
	// silent at 1, where `jobs %2` on the same line answers `jobs: no such
	// job`. A shell that read the operand first would have said so for both.
	// Measured 2026-09-13 from a script with no terminal.
	s.JobControlAbsenceIsReportedFirst = interp.Yes
	// And this is the column where the monitor is *not* enough: `set -m` is
	// granted in a script with no terminal, and `fg` still answers 1 without
	// a word — on a pseudo-terminal too, which is what says the missing
	// thing is a person and not a terminal (#2720).
	// `${a[1][2]}` reads the nested compound `a[1][2]=v` builds, rather than
	// counting characters the way the other shell with the grammar does:
	// measured 2026-09-15, `a=(one two); ${a[0][1]}` is empty here and `n`
	// there. The write half landed in #2491 and the value it built was
	// reachable only through `typeset -p` until this (#2830).
	s.ChainedSubscriptReadsANestedValue = interp.Yes
	s.MonitorAloneResumesAJob = interp.No
	// And it does not announce one on the monitor alone either, which is the
	// same answer for a different reason: this column will not resume a job
	// without a person, and it will not report one starting without a person
	// either. Measured 2026-09-15 on a pseudo-terminal, a script file
	// (#2838).
	s.MonitorAloneAnnouncesAJob = interp.No
	// The panel's lone dissent on the current-job marker: it goes to the
	// newest job here rather than staying with one that stopped. Measured
	// 2026-09-12 through a pseudo-terminal, `sleep 40` stopped with ^Z and
	// then `sleep 41 &` lists `[2] +  Running` and `[1] - Stopped`, and
	// `jobs %+` names the background job where the other five name the
	// stopped one. A job that stops still *takes* the marker — with two
	// background jobs, `kill -TSTP %1` moves the `+` onto the older one
	// here too — so what differs is only whether a later `&` takes it back.
	s.StoppedJobTakesTheCurrentJobMarker = interp.No
	s.JobsListFinishedJobs = interp.Yes
	// A `&` job is reaped under the monitor and nowhere else, so with no
	// monitor the listing goes on saying the job is running — after a
	// `wait` that reaped it, too.
	s.EndedJobIsListedAsRunningWithoutTheMonitor = interp.Yes

	// `jobs`' letters, as its own usage line gives them: `-lnp`. The state
	// filters `-r` and `-s` are unknown options here.
	s.JobsOptions = "lnp"
	// And `-n` means the jobs whose state has moved since this shell last
	// said anything about them, which in a script is none — see the axis for
	// the measurement, and note bash's letter of the same name is a different
	// question and stays unimplemented there.
	s.JobsListsWhatChangedSinceTheLastReport = interp.Yes
	s.JobsPidsOnlyOption = interp.Yes

	// Whether a `&` job's command appears in a `jobs` listing.
	s.JobsShowBackgroundCommand = interp.No

	// Whether a backgrounded job is announced to whoever is typing.
	// Whether `export -f` carries a function to a child.
	// Whether an unassigned subscript is an element.
	s.ArraysAreSparse = interp.Yes
	// An operator on `${a[*]}` trims each element before the join here.
	s.OperatorDistributesOverStarSubscript = interp.Yes
	// A replacement operand's quotes quote and are removed, as in bash 5.3:
	// measured 2026-09-07, `"${s/a/'$v'}"` on `s=xay` and `v=VAL` is `x$vy`
	// (#1209).
	s.ReplacementOperandTakesTheEnclosingQuoting = interp.No
	s.ExportCarriesFunctions = interp.No
	// No `-n` either: measured, `export: -n: unknown option` with the usage
	// line under it, and the script ends there.
	s.ExportTakesTheAttributeOff = interp.No
	s.AnnouncesBackgroundJob = interp.Yes
	// And with the monitor off as well, as bash does — measured 2026-09-10,
	// `[1]\t<pid>` with `set +m` in force (#1738).
	s.AnnouncesBackgroundJobWithoutTheMonitor = interp.Yes
	// The hole gets refilled, and here it is a real hole first: measured
	// 2026-09-12, `jobs %2` is no-such-job after the middle job is killed and
	// reaped, 0 once a fourth job is started, and there is no `%4`.
	s.NextJobNumberRefillsAHole = interp.Yes
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
	// And on the other route too: measured 2026-09-12 on `-i -c` through a
	// pseudo-terminal, `[1]\t<pid>` as the job starts and the `Done` row after
	// the `wait` that reaps it.
	s.InteractiveCommandStringAnnouncesJobs = interp.Yes
	// The row is written where the job ended and not where a prompt is drawn,
	// which the `-i -c` run above is the whole evidence for: there is no prompt
	// on that route and ksh93 writes it anyway.
	s.FinishedJobNoticeNeedsAPrompt = interp.No
	// `$!` before any background command is *unset* here, and `set -u` still
	// has nothing to say about it — the combination no other column has.
	// Measured 2026-09-18 from a script file under `env -i`: `${!-unset}`
	// takes its default and `${!+set}` is empty, so every operator that can
	// see the difference reads the parameter as absent, while
	// `set -u; echo "[$!]"; echo "st=$?"` writes `[]` and then `st=0`.
	//
	// It read as set and empty here until #3011, because the question was two
	// Answer fields whose four combinations had no spelling for this one.
	//
	// Note this is *not* UnsetPositionalIsAllowed reached from another route.
	// ksh93 does let an unset `$1` be empty, and it is quiet about `$!` too,
	// but the two are separate answers: bash refuses both and dash refuses
	// both, while zsh refuses `$1` and not `$!`.
	s.LastBackgroundPid = interp.LastBackgroundPidUnsetButNotRefused
	// A job started with `&` reads an empty standard input, not the shell's:
	// measured 2026-09-07, `ksh -c '/bin/cat & wait; echo ---; /bin/cat' < f`
	// writes `---` and then the file's line on ksh93u+. POSIX XCU 2.9.3.
	//
	// `UnlessClosed` and not the plain answer, which is this column's alone
	// among the four that substitute: `exec 0<&-; /bin/cat & wait` is silent
	// at 0 in dash and bash and `cat: stdin: Bad file descriptor` here. What
	// it cannot dup it leaves alone, so a closed fd 0 reaches the job closed.
	s.BackgroundJobInput = interp.BackgroundJobInputEmptyUnlessClosed
	s.ProcessSubstitutionIsTheLastBackgroundJob = interp.No
	s.ReportsACommandKilledBySignal = interp.Yes
	s.ReportsAnyKilledPipelineElement = interp.No
	s.ChildInterruptEndsTheScript = interp.Yes
	// One sentence for every empty destination: `cd ""`, `HOME=; cd` and a
	// HOME that is absent all say `cd: bad directory` at 1. The first two
	// are these axes and the third is CdWithoutHomeIsAnError.
	s.CdEmptyOperandIsAnError = interp.Yes
	s.CdEmptyHomeIsAnError = interp.Yes
	// `cd old new` rewrites the current directory's path here, and prints
	// where it went. Measured from `…/x/alpha`: `cd alpha beta` writes
	// `…/x/beta` on standard output and moves there.
	s.CdSubstitutesTheOperands = interp.Yes
	s.CdSubstitutionPrintsTheDirectory = interp.Yes
	// Never asked, the form above having answered for two operands and
	// refused three; recorded so that nothing is left unanswered.
	s.CdRefusesExtraOperands = interp.Yes
	s.CdRefusesUnknownOption = interp.Yes
	s.CdHasQuietOption = interp.No
	s.CdHasSymlinkFreeOption = interp.No
	s.CdLastPathOptionWins = interp.Yes
	s.BadSetOptionNameFatal = interp.Yes
	// And the letter too: `set -Z; echo one` prints nothing and exits 2.
	s.BadSetOptionLetterFatal = interp.Yes
	// No POSIX mode here either — `set -o posix` is `posix: bad option(s)` —
	// so the measurement is through the `sh` name, which is the door the
	// core's mode has: both spellings stop at 2 there as they do under
	// `ksh`. The mode moves neither (#2641).
	s.BadSetOptionNameFatalInPosixMode = interp.Yes
	s.BadSetOptionLetterFatalInPosixMode = interp.Yes
	// One of the two columns that weld: `set -oerrexit zzznosuch` turns
	// errexit on and leaves `zzznosuch` as $1, and `set -oe` refuses `e` as
	// an option *name* rather than reading it as a letter. Its own usage
	// line spells the form, `[-o[option]]`.
	s.SetOLetterAttachesItsName = interp.Yes
	s.LongOptionNamesASetOption = interp.Yes
	// The second column with a route split inside one shell: the three names
	// Apply declares immovable are refused to a script and taken on the
	// command line that started it. Measured 2026-09-16 on ksh93u+
	// 2012-08-01, the program on a pipe so that nothing but the invocation
	// could make the shell interactive — `ksh -o interactive` prompts, runs
	// the line and prompts again at status 0, and `ksh -o rc` and `ksh -o
	// login_shell` are 0 and silent, while `set -o interactive`, `set -o rc`
	// and `set -o login_shell` are all `bad option(s)` in both directions
	// from inside that same shell.
	//
	// The axis governs the refusal and not the applying, so this moves the
	// one of the three that has something to move: `interactive` writes the
	// field `$-`'s `i` and the prompt decision read, and `rc` and
	// `login_shell` are rows the table records without acting on and keep
	// the refusal they had. See the reading of ksh93's own `set -o` listing
	// under those two names in Apply (#3221).
	s.ImmovableOptionsSetAtInvocation = interp.Yes
	// The namespace's own way of saying `-i`, in both senses. Measured
	// 2026-09-16 with the program on a pipe: `ksh -o interactive` and `ksh
	// +o nointeractive` each draw a prompt around the line, and `ksh +o
	// interactive` and `ksh -o nointeractive` each draw none, all four at
	// status 0 — the same composition of sign and sense zsh has. The
	// negative spelling is not a row of the listing here: `set -o` names
	// only `interactive`, and the `no` half is the shell's own prefix over
	// it (#3221).
	s.InteractiveOptionName = "interactive"
	s.NonInteractiveOptionName = "nointeractive"
	// And the `set` builtin reads such a word the same way, which is this
	// column and not zsh's: `set --xtrace q` here traces and leaves `q` as
	// `$1`, where zsh swallows the word whole. Two axes since #3129, because
	// the two shells with the invocation spelling disagree about the builtin.
	s.SetLongOptionWord = interp.LongOptionWordIsAnOptionName
	// And two of those words are not option names at all: `--state` writes
	// what `set +o` writes and `--default` puts every option back to its
	// compiled-in default. This shell prints both in the usage line it shows
	// when it refuses something else — `Usage: set [--default] [--state]
	// [arg ...]` — so refusing them was declining what the diagnostic had
	// just offered, and it is the reason `eval "$(set +o)"` still did not
	// round-trip with every roster name moving: that line opens with
	// `--default` (#3153).
	s.SetHasTheStateAndDefaultWords = interp.Yes
	s.LongOptionValueIsANumber = interp.Yes
	// This shell folds hyphens **and** underscores, and does it on every
	// route to an option name rather than in the `--name` spelling alone —
	// `-o err-exit`, `set -o err_exit` and `--glob-star` all name the
	// option, measured 2026-09-16. So its fold is the *namespace's*, and
	// LongOptionNameIgnoresHyphens — which asks about the spelling, and is
	// zsh's because there the hyphen means two things in one shell — is not
	// the question to answer here. It stays unanswered and the two below
	// carry this column instead (#3155, #3254).
	//
	// The fold reaches every roster name and the refusals bound it in both
	// directions: case is not folded (`--ERREXIT`, `-o RC`), and a word the
	// fold leaves unrecognizable is still refused (`--no_profile`). Measured
	// 2026-09-18 on ksh93u+ 2012-08-01.
	s.OptionNamespaceIgnoresSeparators = interp.Yes
	// And `no` in front of any roster name is that name off, on every route:
	// `set -o noerrexit` is 0 with `errexit off` listed, `set +o noerrexit`
	// puts it back, and the listing never grows a `no` row. Measured
	// 2026-09-18. The five rows AddNegatedSetOptions declares below are a
	// separate, listing-shaped fact, and `set -o nonoclobber` is what parts
	// them: the `no` comes off once and `noclobber` is not a name this
	// shell's listing holds, so the word is refused even though `noclobber`
	// alone is taken (#3254).
	s.OptionNamespaceTakesANoPrefix = interp.Yes
	// Where the `-o` does stand alone, the word behind it is taken only if
	// it does not look like options: `set -o -e` lists and turns errexit
	// on, which is bash's reading and not zsh's. What this column still
	// does differently is *when* it lists — once, at the end of the option
	// parse, in a form the last `-o`/`+o` decides — and that is a listing
	// mechanism rather than another answer here, #2698. Measured on the
	// field.
	s.SetODeclinesADashWord = interp.Yes
	// And it lists **once**, at the end of the option parse, in the form the
	// last `-o`/`+o` decided — measured across ten shapes on ksh93u+ 2012-08-01
	// (#2698). The other five columns list where they stand.
	s.SetListsOptionsOnceAtTheEnd = interp.Yes
	// A bare `-` turns `-x` and `-v` off here and so does a bare `+`, which is this column alone (#2699).
	s.BareOptionWord = interp.BareEitherSignClearsTraceAndVerbose
	// unanswered SetValidatesOptionLettersFirst: this column does agree with
	// bash about what a refusal leaves behind — `command set -e -Z` is
	// errexit off, and `set -o -Z`, where a bare `-o` takes no next word,
	// lists nothing at all — but it gets there by a different reading. It
	// takes in every option word, reports every bad one in the order they
	// were written, names and letters alike, and applies none of them:
	// `set -o nosuch -z` draws both sentences and one usage line, and
	// `command set -u -o zzznosuch` is nounset off where bash's is on. That
	// is SetReportsEveryBadOption carried one step further, and folding it
	// into an axis about *letters* would either lose the name reports or put
	// bash's errexit off where the measurement says it is on. So it is the
	// second half of SetReportsEveryBadOption above, which this column is the
	// only one to answer yes (#2670), and the builtin reads it there rather
	// than putting the letters question to a dialect that reports every bad
	// option — the unanswered field is never reached rather than quietly
	// defaulting.
	s.BadSetOptionNameAtInvocationExitsZero = interp.No
	s.UnknownConditionOptionIsAStatus = interp.No
	s.ReturnOutsideAFunctionIsRefused = interp.No
	// And a `break` with no loop around it is ignored, silently: measured on
	// 93u+ 2012-08-01, `echo t; break; echo after` prints both and ends at
	// 0. No wording goes with it, which is dash's answer too.
	s.LoopControlOutsideALoopIsFatal = interp.No
	// The count is read first — `break abc` outside a loop is `break: abc:
	// label not implemented` — and the place draws no sentence here.
	s.LoopControlPlaceIsJudgedBeforeTheCount = interp.No
	// The same pairing as dash: a call stops a `break` and the parentheses
	// do not.
	s.FunctionCallIsALoopControlBoundary = interp.Yes
	s.SubshellIsALoopControlBoundary = interp.No
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
	s.UnsetReachesTheFunctionTable = interp.No

	// Whether a redirection target is expanded as an ordinary word.
	s.RedirectTargetIsAnOrdinaryWord = interp.No
	// And no pathname expansion either, which is POSIX's own rule and the
	// one this column follows: `cat < only-*.txt` is `only-*.txt: cannot
	// open [No such file or directory]` with a file of that name sitting
	// there, and `printf X > only-*.txt` creates a file called `only-*.txt`
	// rather than truncating the match. Measured 2026-09-16 (#3207).
	s.RedirectTargetTakesPathnameExpansion = interp.No

	// Whether `type --` ends the options.
	// `whence -v .` and `type .` alike are `. is a special shell builtin` in
	// ksh93u+ 2012-08-01, where `echo` is a plain one.
	// The one column that contains it. `( v=$(echo hi; for) ); printf 'after
	// st=%s\n' "$?"` prints `after st=3` in ksh93u+ 2012-08-01 and the script
	// runs on, where the other five end it; and it contains it however deep,
	// so a pipeline element and an enclosing `$( … )` answer the same (#3274).
	s.SubstitutionParseErrorEscapesASubshell = interp.No
	// It carries on from a here-document body too, and it is one of the two
	// columns that does: measured 2026-09-17, `cat <<END` over a body holding
	// `$(echo hi; for)` prints `start`, never runs `cat`, and reaches the
	// next command with `after st=3` (#3318).
	s.SubstitutionParseFailureInAHeredocBodyEndsTheShell = interp.No
	// The 3 it leaves there is the refusal's own syntax status surviving and
	// not a fatal error's number, which is 1 here — a plain failed
	// redirection leaves 1 in this column, so the two are told apart by the
	// measurement rather than by their arithmetic (#3319).
	s.SubstitutionParseFailureCarriesTheFatalStatus = interp.No
	s.TypeDistinguishesSpecialBuiltins = interp.Yes
	// And this shell's special side is three names longer than POSIX's.
	// Measured 2026-09-16 on ksh93u+ 2012-08-01, each probe with its
	// control: `type alias` is `alias is a special shell builtin` where
	// `type echo` is plain; `V=1 alias` leaves V at 1 where `V2=1 cd .`
	// leaves V2 unset; `alias -Z` ends the script at 2 where `cd -Z` writes
	// its usage and carries on; and `set -x; V=1 alias` traces `+ V=1`
	// before `+ alias`, the order this shell keeps for `export` and not for
	// `cd`. `unalias` and `typeset` answer all four the same way — `typeset
	// -Q` is the usage error that reaches the fatality, since `typeset
	// -Z9Z` is accepted here.
	//
	// `newgrp` is ksh93's fourth, and it is here now that the builtin behind
	// it exists — it is `exec newgrp` under a name, which is what lets the
	// word be special at all. Two of the three consequences apply: the
	// sentence `type` writes and a prefixed assignment persisting. The third
	// is unreachable rather than absent, and that is a fact about `exec`:
	// measured 2026-09-18, `newgrp -Z` there writes `newgrp: illegal option
	// -- Z` and the program's own usage line, so no option ever reaches the
	// shell to be fatal about. See newgrp.go (#3316).
	s.SpecialBuiltinsBeyondPosix = "alias unalias typeset newgrp"
	s.TypePrintsFunctionBody = interp.No
	s.TypeEndsOptionsWithDashDash = interp.Yes
	// whence -v's letters, and no `-t` among them: that letter is refused
	// the way `whence` refuses any option it does not have, usage line and
	// all. -a lists every resolution, -p and -f are
	// the PATH search and the function skip — and -p searches past the
	// shell's own answer here, naming the bare path.
	s.TypeOptions = "afp"
	s.TypePSearchesPathPastTheShell = interp.Yes
	s.TypePathAnswerIsASentence = interp.No
	s.TypeFSaysTheFunctionBack = interp.No

	// The letters `typeset` reads here. `-g` it simply does not have, and
	// there is no `local` (see Register), so LocalOptions stays empty.
	//
	// unanswered LocalDashSavesTheShellOptions: `local -` is an operand of a
	// builtin this shell does not have, so there is no spelling of the
	// question to put to it. Measured 2026-09-15: `local -` here is
	// `local: not found` and the option the body set survives, which is what
	// a missing command does rather than an answer about the operand.
	//
	// `-f` is here now (#1494), and it says the function back *verbatim*:
	// the parser keeps the definition's own characters and the listing
	// writes them, terminator and all — see
	// syntax.Dialect.FunctionDefinitionIsSourceText and FunctionLayout for
	// what is left of the layout (#2610). What the listing must get right
	// beyond the spacing is the `function` keyword, because `typeset`
	// declares a local in a keyword body here and the global in the other,
	// so a listing that dropped the word would hand back a program whose
	// variables leak — and the source text keeps it because it was written.
	// `-E` is here since #2559: it is the float **format**, `%.*g` with n
	// significant digits, where zsh's same letter is `%.*e` with n−1 places.
	// See interp/floatformat.go.
	// `-t` is here since #2192: on a `-f` line it traces the function, and
	// on a variable line this shell has no meaning for it — which is why it
	// stays in Diagnostics.UnimplementedOptionLettersOnAFunctionLine's
	// opposite number rather than here alone. See fpath.go.
	//
	// `-F` joins it since #2419 and is the same *attribute* `-E` already
	// had under a different rendering — n decimal places rather than n
	// significant digits — so the two letters share every rule below. `-h`
	// joins it too, and it is not zsh's letter of the same spelling: here it
	// takes a string and records nothing, `[-h string]` in this shell's own
	// usage block.
	s.DeclareOptions = "aACEFfHhilmMnprtTux"
	// And the number the float letters take, under this shell's own rule for
	// a detached one: only where the letter ends its option word. `typeset
	// -Ex 3 a=3.14159` is `3: is not an identifier` here and the float in
	// zsh, and `typeset -Fx 3 a=1.5` is the same refusal. That rule is what
	// #1461 and #2559 both deferred; it is the axis below.
	s.DeclareOptionsTakingANumber = "EF"
	s.DeclareNumberDetachedOnlyAtTheWordEnd = interp.Yes
	s.FloatFormatLetterE = interp.FloatFormatSignificantDigits
	// And a bare float letter over a name that already has a precision
	// resets it to the letter's default here, where zsh keeps it: measured
	// 2026-09-15, `typeset -E 3 a=1.23456789; typeset -E a` reads `1.23` and
	// then `1.23456789`, and measured again 2026-09-14 at the other letter,
	// `typeset -F 3 e=1.5; typeset -F e` reads `1.500` and then
	// `1.5000000000`.
	s.BareFloatLetterResetsThePrecision = interp.Yes
	// And the integer letter cannot stand beside a float one: measured,
	// `typeset -iE 3 a=1.5` and `typeset -Ei 3 a=1.5` are both typeset's
	// usage block at 2, and the script ends there. zsh takes the pair and
	// lets the first letter written win.
	s.NumericTypeLettersAreExclusive = interp.Yes
	// And where a pair *is* taken, the letters have a fixed rank rather than
	// an order: `E` over `F` over `i`, whichever way round they stand and
	// whether they share a word or not. `typeset -iF 3 a=1.5` and
	// `typeset -Fi 3 a=1.5` both list `typeset -F 3 a=1.500` here, where zsh
	// answers them differently (#2419).
	s.NumericTypeLetterPrecedence = interp.NumericLetterFloatOutranksTheInteger
	// The letters a `-f` line marks a function with, both built since #2192:
	// `-u` says the body is read from `$FPATH` at the first call and `-t`
	// traces the function. See dialect/ksh/fpath.go, where the measurements
	// are, and note that `t` names a mark that is not "undefined" — which is
	// why interp.Runner.SetFunctionMarkedUndefined is handed the letters and
	// not a digested request.
	s.FunctionLettersThatMarkUndefined = "tu"
	// `-m` is here now, and it is not the letter zsh spells the same way:
	// it *moves* a parameter — `typeset -m new=old` — where the other
	// shell selects several by pattern. Measured 2026-09-13 on ksh93u+;
	// interp/declaremove.go holds the whole measurement and
	// interp.DeclareMatchingLetterPolicy is the axis.
	s.DeclareMatchingLetter = interp.DeclareMatchingLetterMoves
	// A bare `typeset` is a listing here, and a third one: not every
	// parameter and not the running scope's, but every name that carries an
	// **attribute**, in this shell's own vocabulary and with no value on the
	// line — `export ex`, `integer n`, `toupper up`, and nothing at all for
	// a name that was only assigned. Measured 2026-09-13 on ksh93u+ with
	// `env -i`; interp.BareLocalListsAttributedNames holds the measurement
	// and Runner.attributePhraseHead the words (#2345).
	//
	// There is no `local` here to give the other half of the pair to, which
	// is why only this field moves: `local` is not a builtin in this shell
	// and the word is a command that was not found.
	s.BareTypesetListing = interp.BareLocalListsAttributedNames
	// `-M` is here too, and it is the same shape: the letter names a
	// *character mapping* — `typeset -M tolower v`, which lists back as
	// `typeset -l v` — where zsh's `functions -M` registers a math function.
	// Measured 2026-09-13; interp/declaremapping.go holds it.
	s.DeclareMappingLetter = interp.DeclareMappingLetterNamesACharacterMapping
	// `-T` is the third letter of that shape: it names a **type** here —
	// `typeset -T Pt=(…)` — where zsh's ties a scalar to an array, and the
	// two share no operand grammar. Measured 2026-09-13;
	// interp/declaretype.go holds the whole measurement and
	// interp.DeclareTypeLetterPolicy is the axis. A type is a compound value
	// and this engine has none, so every form but the definition is the
	// silence that shell answers with and the definition goes on saying the
	// letter is missing (#2620).
	s.DeclareTypeLetter = interp.DeclareTypeLetterNamesAType
	// `-H` is the fourth of that shape, and the cheapest: it is an inert
	// attribute here — recorded, and written back as `typeset -H h=hid` with
	// the value and all — where zsh's letter of the same spelling is what
	// *withholds* a value from a listing. On a UNIX ksh93 it maps nothing and
	// changes nothing a script can see but the listing. Measured 2026-09-13;
	// interp/declarehide.go holds the whole measurement and
	// interp.DeclareHideValueLetterPolicy is the axis.
	s.DeclareHideValueLetter = interp.DeclareHideValueLetterIsAnInertAttribute
	// And the lower-case letter is not the same question: here it takes a
	// *string* — `[-h string]` in this shell's own usage block — and records
	// nothing at all. `typeset -hx s q=1` leaves `q` unexported, the `x`
	// having been the argument (#2419).
	s.DeclareHideInScopeLetter = interp.DeclareHideInScopeLetterTakesAString
	// A lone `-` or `+` is an option word to *this* builtin: measured
	// 2026-09-10, `typeset +` names every parameter and `typeset -` writes
	// the same table with values, where bash calls the sign an identifier
	// and refuses it. It is not this shell's answer everywhere — `export +`
	// is `+: is not an identifier` here — but `export` does not read its
	// options through this parser, so the narrower reading is the one that
	// gets to be true (#1576).
	s.SignAloneIsAnOptionWord = interp.Yes
	// Not on `export` and `readonly`, which is the one place these two
	// questions come apart: measured 2026-09-12 on ksh93u+, `export +` is
	// `+: is not an identifier` and `readonly +` is `+: invalid variable
	// name`, both at 1, while `typeset +` on the same line lists.
	s.SignAloneIsAnOptionWordToExport = interp.No
	// typeset is one of this shell's own special builtins, so any of its
	// failures ends the script — a bad option included.
	s.TypesetBadOptionFatal = interp.Yes
	// `typeset +f` names the functions here as it does in zsh, and the
	// *spelling* is this shell's own — see Diagnostics.FunctionNameListing.
	// Measured 2026-09-12: `f() { :; }; function g { :; }; typeset +f`
	// writes `f()` and then `g`.
	s.FunctionNamesUnderPlus = interp.Yes
	// `functions` is a word here as well as in zsh, and it is `typeset -f`
	// under a second name — same listing, same status, same silence for a
	// name nobody defined. The letters are narrower than `typeset`'s: this
	// engine implements `-f` and `-p`, which reach the listing, and `-t`
	// and `-u`, which are the two marks a `-f` line puts on a function
	// (#2192). ksh93u+ answers the usage line for `-F`, `-m` and `-M`,
	// which is what an unimplemented letter gets here too.
	s.FunctionsOptions = "fMptu"
	// `integer` is the same declaration under a second name, and this shell
	// hands it typeset's whole letter grammar — measured 2026-09-06, every
	// letter typeset takes is either accepted by `integer` or refused by it
	// for conflicting with the type, and none is refused as unknown. So the
	// set is typeset's and so is Diagnostics.UnimplementedOptionLetters for
	// it, which is why `integer -f` here says the letter is missing rather
	// than claiming this shell has never heard of it.
	//
	// `-f` is *not* here, and that is measured rather than an oversight:
	// `integer -f nm` answers with the usage line on ksh93u+ — and
	// fatally, since these are special builtins there — where `typeset -f
	// nm` on the same line lists. The word carries a type and a function
	// has none.
	s.IntegerOptions = "aACEFHhilmMnprtTux"
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
	s.SetArrayBadNameLeavesZero = interp.No
	s.StoreRefusalOfADeclaredElementLeavesZero = interp.No
	// No `-v` here — `printf -v x %s Q` is `printf: -v: unknown option` at
	// 2 — so the store is never reached through that builtin.
	s.StoreRefusalThroughPrintfLeavesZero = interp.No
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

	// The coprocess `|&` starts is half taken back when it ends: the end this
	// shell writes goes, so `print -p` is `no query process` at 1; the end it
	// reads stays, so whatever the coprocess left in the pipe is still there
	// for a `read -p` to answer with. What happens when that read finds
	// nothing left is not this axis — both shells with the letters forget the
	// whole coprocess there, reaped or not. Measured 2026-09-12 (#2411).
	s.ReapedCoprocessEnds = interp.CoprocWriteEndGoesWithTheCoprocess
	// And `p` is a duplication target that names the ends the letters reach,
	// so a script may park them on numbers of its own: `cat |&; exec 3>&p`
	// then writes to the coprocess through a 3. It **takes** the end rather
	// than lending it — `print -p` after that `exec` is the same `no query
	// process` it gives before any coprocess was started, while a `read -p`
	// still answers, so the two ends move one at a time. Measured
	// 2026-09-16; with no coprocess running the word is refused as the
	// duplication's own failure, `p: cannot open [Bad file descriptor]`, and
	// a *file* called `p` in the directory does not make it an open (#2345).
	s.CoprocessNamedByARedirection = interp.CoprocessRedirectionMovesTheEnd
	// The ends are numbered where any other allocation goes rather than at
	// the top of the table. Measured 2026-09-13 through the numbers left over
	// for the shell to hand out: `exec {a}>/dev/null; exec {b}>/dev/null;
	// exec {c}>/dev/null` answers `10 11 12` with no coprocess and `11 12 13`
	// with one, so the coprocess is in that region and bash's is not (#2596).
	s.CoprocessEndPlacement = interp.CoprocEndsWhereAnyDescriptorGoes

	// A `{name}>f` descriptor goes back with the command's other
	// redirections, and closing through a name that holds nothing is not
	// worth a word here.
	s.FdVariableOutlivesTheCommand = interp.No
	// `exec 3<<X` puts the body in a temporary file here, so `/dev/fd/3` is
	// a regular file and the descriptor is seekable: `head -1 <&3` reads a
	// block and seeks back, and the `cat <&3` after it still gets the rest of
	// the document. bash 5.3.15 and dash use a pipe and lose it. Measured
	// 2026-09-14 (#2759).
	s.HeredocBody = interp.HeredocBodyInATemporaryFile
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
	// A duplication's descriptor number stops at 64 here, above which the
	// refusal is about the number and carries no errno: `echo x >&63` is
	// `cannot open [Bad file descriptor]` and `echo x >&64` is `bad file
	// unit number` (#3210). The `-v` echo writes a line as it was read, so
	// the last line of a `-c` string with no newline is echoed without one
	// (#3130). And `read -t` takes an expression, as `ulimit`'s operand
	// does here — `read -t abc` is a timeout of nought in silence (#3209).
	// All measured 2026-09-18.
	s.DescriptorNumberCeiling = interp.DescriptorNumbersStopAtSixtyFour
	s.VerboseEchoAddsAMissingNewline = interp.No
	s.ReadTimeoutOperandIsArithmetic = interp.Yes

	return s
}

// Diagnostics is how ksh93 reports failure.
// kshKillUsage is printed on its own for `kill` with no operands, and again
// after an unknown option — with no shell name or line in front of it either
// time, which is not how ksh93 prints anything else.
const kshKillUsage = "Usage: kill [-lL] [-n signum] [-s signame] job ...\n" +
	"   Or: kill [ options ] -l [arg ...]"

func Diagnostics() interp.Diagnostics {
	d := interp.Diagnostics{
		// A bare array name refused by `set -u` is named as its first
		// *element* here: `a=(x y z); unset "a[0]"; set -u; echo "$a"` is
		// `a[0]: parameter not set` where bash says `a` (#2818).
		UnboundBareArrayNamesElementZero: true,
		// And a subscript that named no element is refused by the value the
		// brackets came to rather than by the text in them: `i=9; a=(x);
		// set -u; echo "${a[$i]}"` is `a[9]: parameter not set` here where
		// bash 5.3 and zsh 5.9.2 both write `a[$i]` back (#2911).
		UnboundElementNamesTheSubscriptsValue: true,
		// And the `?` operator's refusal names the array where `set -u`'s
		// names the element: `a=(x y z); echo "${a[9]:?m}"` is `a: m` here,
		// against `a[9]: m` in bash 5.3.20 and zsh 5.9.2 alike, while the
		// same shell's `set -u` on `${a[9]}` writes `a[9]`. The same holds
		// for a table's key and for a name that is not there at all, and
		// `${nope:?m}` — with nothing to leave out — is the control that
		// agrees with the other two columns. Measured 2026-09-18 (#3241).
		ParamErrorNamesTheArray: true,
		// A leading numeral that met a second point straight after its
		// first has a sentence of its own here, naming the character:
		// `$(( 1..2 ))` is `.: invalid character in expression -  1..2 `
		// (#2817).
		ArithDoubledPointInTheNumeral: ".: invalid character in expression - %[1]s",
		// Every backquote substitution this shell reads draws a remark, and
		// it draws it only when the shell is not going to run the program:
		// `ksh -n bq.sh` writes one line per backquote and `ksh bq.sh`
		// writes nothing, measured 2026-09-12 on 93u+ 2012-08-01 over the
		// same file. It accompanies a refusal too, warning first.
		BackquoteObsolete: "warning: line %[1]d: `...` obsolete, use $(...)",
		// `-n` is a **lint mode with rules** in this shell rather than a
		// parse check, and this is the second of them: two operators written
		// with no blank between them draw a line about the layout, on the
		// same route and under the same restriction. `(:);(:)` and
		// `echo a&;b` draw it and then run at status 0; `if |; then :; fi`
		// draws it and is then refused, warning first. Writing the operators
		// apart silences it and changes nothing else. Measured 2026-09-12 on
		// 93u+ 2012-08-01.
		OperatorsNotSeparated: "warning: line %[1]d: use space or tab to " +
			"separate operators %[2]s and %[3]s",
		// And the line is inside that sentence rather than in the location,
		// which is the same shape this shell's syntax errors take.
		RemarkNamesItsOwnLine: true,
		// A math complaint raised by a builtin names it, as bash's does:
		// `let '1+'` is `ksh: let: 1+: more tokens expected`.
		ArithErrorNamesTheBuiltin: true,
		TypeKeyword:               "%[1]s is a keyword",
		// ksh93's `type` is `whence -v`, and the message says so.
		TypeExternal: "%[1]s is a tracked alias for %[2]s",
		// And the plain sentence for an operand that was written with a
		// slash in it, which this shell never searched PATH for: `command -V
		// ./bb/tool` is `./bb/tool is <dir>/./bb/tool` where `command -V ls`
		// is the tracked alias. Measured 2026-09-14 and again 2026-09-18;
		// the discriminator is the slash and not the hash table (#2953).
		TypePathnameOperand: "%[1]s is %[2]s",
		TypeFunction:        "%[1]s is a function",
		// The body is quoted the way this shell's own listing quotes it, and
		// `command -v` writes that body with no `alias name=` in front of it.
		TypeAlias:               "%[1]s is an alias for %[2]s",
		TypeAliasQuotesValue:    true,
		CommandVAlias:           "%[2]s",
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
		BadArraySubscript:                  "%[1]s: subscript out of range",
		SubscriptBeforeTheFirstElementRead: "%[1]s: subscript out of range",
		CannotConvertTableToArray:          "%[2]s: cannot change associative array %[1]s to index array",
		// The subscript is the verb, not the name — see
		// Semantics.WholeArraySubscriptAssigningAnArray.
		InvalidSubscriptInAssignment: "%s: invalid subscript in assignment",
		// `append` even for a plain `=`: the sentence is about the two kinds
		// and not about the operator — see
		// Semantics.BareElementsInATableLiteralEndTheScript.
		IndexArrayIntoATable: "cannot append index array to associative array %[1]s",
		// And the fourth kind is not exportable at all — the letter is
		// refused rather than the value being flattened. The builtin names
		// itself here, which `export` and `typeset` both do.
		CompoundIsNotExportable: "%[2]s: %[1]s: only simple variables can be exported",
		// The same sentence from `unset`, with the builtin named in front of
		// it as this shell names it in front of the arithmetic one below.
		UnsetSubscriptBeforeTheFirstElement: "unset: %[1]s: subscript out of range",
		UnsetBadFunctionName:                "unset: %[1]s: invalid function name",
		// The builtin names itself in front of the arithmetic sentence, which
		// it does not do for the identical failure in an expansion.
		UnsetBadSubscript: "unset: %[1]s",
		// And the store an operand's brackets walk into names the builtin
		// that was handed the operand: `read 'r[1/0]'` is `read: 1/0: divide
		// by zero` here, where bash and zsh write the sentence alone.
		StoreOperandBadSubscript: "%[1]s: %[2]s",
		// And a declaration's operand names the builtin it was handed to:
		// measured 2026-09-17, `a=(1 2 3); typeset 'a[b c]'=v` is
		// `typeset: b c: arithmetic syntax error`, with `readonly` and
		// `export` naming themselves and `integer` calling itself `typeset`.
		DeclarationBadSubscript: "%[1]s: %[2]s",
		// All three of those keep the *builtin's* location while the sentence
		// stays the language's, which the bare `$(( b c ))` is the control
		// for: it is `./k.sh: line 2:` where the three builtin routes are
		// `./k.sh[2]:`. See the field for the measurement.
		BadSubscriptKeepsTheBuiltinsLocation: true,
		SetInvalidOptionName:                 "set: %[1]s: bad option(s)",
		// The letter as the script spelled it: `set +q` is refused as `+q`
		// here, as it is in bash.
		SetInvalidOptionLetter: "set: %[1]s: unknown option",
		// Both 2, and both written down: see bash's pair for why a shell
		// that answers the two spellings alike still says so (#2629).
		SetInvalidOptionNameStatus:   2,
		SetInvalidOptionLetterStatus: 2,
		// The one shell in the panel that repeats `set`'s usage line under
		// a bad option *name* as well as under a bad letter.
		SetInvalidOptionNameUsage: true,
		// At an invocation ksh93 prints its own usage instead, and names
		// itself by the last element of the word it was invoked by — where
		// bash spells the whole path. Measured through a link named
		// `myksh`, which is what it called itself.
		InvocationUsage: "Usage: %[2]s [-cilrsDEabefhkmnprtuvxBCGH] [-R file] [-o[option]] [arg ...]",
		// The block a `--name` refusal gets instead, which names no
		// letters because the spelling that was refused has none.
		InvocationLongOptionUsage: "Usage: %[2]s [ options ] [arg ...]",
		SetLongOptionUsage:        "Usage: set [--default] [--state] [arg ...]",
		// And the sentence above it names the shell the same way, which
		// nothing but an option refusal does here.
		InvocationOptionRefusalNamesTheBase: true,
		SignalDescriptions:                  signalDescriptions(),
		JobRunning:                          " Running",
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
		// A job that has ended is `Done`, and `Done(N)` where it ended with
		// a status. The leading space is ksh's: see JobLine below.
		//
		// This used to be spelt ` Running`, on the measurement that ksh93
		// lists an ended job as running and says so even after a `wait`.
		// That measurement was made with the monitor off, where it holds —
		// and it is the shell not having reaped the job rather than the word
		// it writes, which `set -m` shows: the same listing is `Done(7)`
		// there. The blind spot is
		// Semantics.EndedJobIsListedAsRunningWithoutTheMonitor now, and
		// these are the words for a job this shell knows has ended (#3537).
		JobDone:   " Done",
		JobExited: " Done(%[1]d)",
		// The name first, then the verb with the OS string bracketed after
		// it — the same shape ksh93 uses for `.`, which DotCannotOpen
		// already says.
		CannotOpen: "%[1]s: cannot open [%[2]s]",
		// A duplication whose source is not open takes the same shape as an
		// open that failed here — `cat <&10` is `10: cannot open [Bad file
		// descriptor]`, measured 2026-09-12 — but it cannot share the field:
		// CannotOpen is worded reason-first in one of the other dialects
		// (#734).
		DuplicationSourceNotOpen: "%[1]s: cannot open [%[2]s]",
		// And past this shell's own ceiling of 64 the sentence is about the
		// number and carries no errno at all: `echo x >&63` is `cannot open
		// [Bad file descriptor]` and `echo x >&64` is this (#3210).
		FdNumberOverCeiling:     "%[1]s: bad file unit number",
		SeekDescriptorNotOpen:   "%[1]s: bad file unit number [%[2]s]",
		SeekOffsetRefused:       "%[1]s: invalid seek offset",
		SeekStreamHasNoPosition: "%[1]s: not seekable",
		// And quotes the move's suffix with it: `exec 6<&5-` with nothing
		// open at 5 is `5-: cannot open [Bad file descriptor]`, where bash
		// names the number alone.
		NamesTheMoveSuffixInTheTarget: true,
		// No BuiltinWriteError: `echo hi >&-` reports 1 here and says
		// nothing, which is the semantics axis answering and the wording
		// staying empty. A broken pipe is silent as well, and there the
		// status is 0 -- see BrokenPipeWriteErrorFailsTheCommand, the axis
		// this shell and zsh answer the opposite way round.
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
		ScriptNotFound:          "%[1]s: not found",
		ScriptNotFoundStatus:    127,
		ScriptNotReadable:       "%[1]s: cannot open [%[2]s]",
		ScriptNotReadableStatus: 126,
		Location:                interp.LocationLineWordAfterFirst,
		TraceQuoting:            interp.QuoteDollar,
		// The tilde and the hash count anywhere here rather than only at the
		// front, `=` counts only at the front, and `^` and `!` do not count
		// at all — three splits from bash in one set. Measured 2026-09-12.
		TraceMetacharacters: interp.TraceMetacharacters{
			Anywhere: "*?[]{}~#",
			Leading:  "=",
		},
		// Both ends of a test are printed bare: `[ 1 -lt 2 ]`, where bash
		// quotes both and zsh quotes the closer. Only the final operand —
		// `[ -n "]" ]` is `[ -n ']' ]`.
		TraceBareBracket:  interp.TraceBracketPairBare,
		TraceArrayLiteral: interp.TraceArraySpaced,
		// The assignment written in front of a command is traced *after* the
		// command's own line — `+ /bin/echo c` then `+ C=3` — except in front
		// of a special builtin or a function, where it comes first. See
		// interp/xtraceprefix.go for the nine rows that says.
		TracePrefixAssignment: interp.TracePrefixOwnLineAfter,
		// The same quoting inside a condition as outside it, which is where
		// this shell parts from bash: `[[ "a b" == "a b" ]]` traces
		// `[[ 'a b' == 'a b' ]]` here and `[[ a b == a b ]]` there.
		TraceConditionQuoting: interp.QuoteDollar,
		// `((n))` and `(( n + 1 ))`: the text between the parentheses, with
		// nothing added. bash and zsh add a space on each side.
		TraceArithCommand: interp.TraceArithTight,
		TraceArithForPart: interp.TraceArithTight,
		// And the text those parentheses go round is the part *this* shell
		// kept, which is neither the part as written nor the part with its
		// leading blanks off: the first two give up their trailing blanks
		// and the third gives up its leading ones, so one header traces
		// `((  i=0))` and `((i++  ))`. The same string a complaint blames —
		// ` i=1/0: divide by zero` against the step's `i=1/0 : divide by
		// zero` — which is why it is one answer and not two. Measured
		// 2026-09-13; interp/arithforpart.go holds it (#2420).
		ArithForPartText: interp.ArithForPartTextLosesOneEnd,
		ScriptLocation:   interp.LocationLineWord,
		// At a prompt this shell names no line at all — see
		// withPromptWordings, where the sentences it writes there are
		// measured. The location is already name-only for line 1 under `-c`;
		// this says so for every line, since a construct typed over three
		// lines is still `ksh: syntax error: …` there.
		PromptLocation:              interp.LocationNameOnly,
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
		ShiftNegativeCount: "shift: %[2]s: bad number",
		// A `break`'s count is a *label* here, because this shell's `break`
		// takes one — so a word it cannot read is not called a bad number at
		// all. Measured 2026-09-14: `break abc`, `break 0` and `break " 1 "`
		// are all `break: <word>: label not implemented` at status 1, with
		// the script ended. `break -1` is the option path instead (`unknown
		// option`, with the usage line under it), which this does not reach
		// and which is where a leading dash goes in every builtin here
		// (#2800).
		LoopControlCount:       "%[1]s: %[2]s: label not implemented",
		LoopControlCountStatus: 1,
		StdinBuiltinLocation:   interp.LocationBracketLine,
		ArithError:             "%[1]s: %[2]s",
		DivisionByZero:         "divide by zero",
		// ksh93 names the innermost keyword still awaiting a partner: `if`
		// on its own, and the `then` inside it once that has been consumed.
		EvalNaming:             interp.SourceBeforeLocation,
		SourceFileNaming:       interp.SourceBeforeLocation,
		SourceFileIsTheBuiltin: true,
		// And the whole chain of borrowed texts in front of it, each with
		// the line that entered the next: `./n.sh[2]: .[2]: .: line 3: …`.
		// Measured 2026-09-12 over six arrangements, which are one rule —
		// see the field. It is what the two naming fields above could not
		// say on their own: turning BorrowedTextIsNamedAtRunTime on here
		// without the brackets writes `./s.sh: .: line 3:`, which is closer
		// and still wrong (#2461).
		BorrowedTextRendersTheCallStack: true,
		// A failing offset is blamed together with what follows it in the
		// range: `${x:1+:2}` names `1+:2`. A failing length has nothing after
		// it and is named on its own.
		SubstringErrorNamesTheWholeRange: true,
		// An operand failure is two sentences here, and which one is said
		// turns on whether the expression ran out or found something it
		// could not use: `$((1+))` against `$((%))`.
		ArithOperandExpected: "arithmetic syntax error",
		// `++` on something that cannot be assigned to is about the
		// assignment rather than about the operator, and the operator is not
		// in the sentence. Measured 2026-09-14: `$(( 1++ ))`, `$(( 1-- ))`
		// and `$(( ++1 ))` all draw it, where bash and dash answer the last
		// of those with 1 (#2420).
		ArithIncrementNeedsAPlace: "assignment requires lvalue",
		ArithExpressionRanOut:     "more tokens expected",
		ArithOperatorExpected:     "arithmetic syntax error",
		// A stray `:` is the one construct this shell writes back to front:
		// the byte, the reason, and then the expression after a ` - `, where
		// every other math complaint it makes is `<expression>: <reason>`.
		// Measured 2026-09-12 over every printable byte — `:` is the only
		// one (#2224).
		ArithColonWithoutQuestionLine: ":: invalid character in expression - %[1]s",
		// A conditional with no `:` says so, and so does one with no value
		// after the `?` — this shell reports the colon it is still waiting
		// for rather than the value. A missing *else* is the ordinary end of
		// input, so ArithConditionalElse stays empty.
		ArithConditionalColon: "':' expected for '?' operator",
		ArithConditionalThen:  "':' expected for '?' operator",
		// A digit the base does not have is the same sentence.
		DigitTooGreatForBase: "arithmetic syntax error",
		// The same sentence for a base outside 2..64: `$(( 1#0 ))` is
		// ` 1#0 : arithmetic syntax error` here, as everything else is.
		ArithInvalidBase:    "arithmetic syntax error",
		ArithRecursionLimit: "recursion too deep",
		// The two math-function sentences, and they blame different extents
		// — which is the whole of why each takes the verb it does. Measured
		// 2026-09-13 on ksh93u+ 2012-08-01:
		//
		//	$(( nosuchmf(1) ))           nosuchmf(1) : unknown function
		//	$(( 1 + nosuchmf(1) + 2 ))   nosuchmf(1) + 2 : unknown function
		//	$((nosuchmf(1)))             nosuchmf(1): unknown function
		//	$(( 1 ? nosuchmf(1) : 2 ))   nosuchmf(1) : 2 : unknown function
		//	$(( atan(1,2) ))              atan(1,2) : function has wrong
		//	                             number of arguments
		//	$((atan(1,2)))               atan(1,2): function has wrong number
		//	                             of arguments
		//
		// A name it does not know is blamed from that name to the *end of
		// the expression*, so what stands in front of the call is cut off
		// and the blank before the `))` is kept. A count it will not take is
		// blamed on the expression entire, blanks at both ends included.
		// Both are complete sentences of their own: the `: ` here is this
		// shell's, not ArithError's, because neither reaches that wrapper.
		//
		// Every other math complaint this shell makes goes the ordinary way
		// — ` sqrt() : arithmetic syntax error`, ` sqrt(1/0) : divide by
		// zero` — so the pair above is the exception and not the rule.
		MathFunctionUnknown:                  "%[2]s: unknown function",
		MathFunctionArgumentCount:            "%[2]s: function has wrong number of arguments",
		MathFunctionNoArgumentIsASyntaxError: true,
		// Except for the `@` operator family, the one bad substitution ksh93
		// defers to run time — measured, `${x@Q}` in a branch never taken is
		// silent — and when reached it is reported as a bad substitution
		// after all, not with the parse wording above.
		// The whole word as it was written, quotes and all: `echo
		// "[${x@QQ}]"` is refused as `"[${x@QQ}]": bad substitution` and
		// `echo pre${x@QQ}post` as `pre${x@QQ}post: bad substitution`, so
		// what is named is the source of the word rather than the `${…}`.
		BadSubstitutionAtRun: "%[1]s: bad substitution",
		// This shell names neither the parameter nor the operator: the whole
		// word is blamed, `${@:=abc}: bad substitution`. Measured on the
		// shapes that separate a word from an expansion — `x${@:=abc}y`,
		// `"${@:=abc}"` and `a"${@}"b"${@:=abc}"c` are each named entire,
		// which is the same subject BadSubstitutionNames records for the
		// sentence beside it (#1541).
		AssignThroughExpansionBadName: "%[2]s: bad substitution",
		BadSubstitutionNames:          interp.NamesTheWholeWord,
		FunctionNameInvalid:           "%[1]s: invalid function name",
		FunctionNameDiscipline:        "%[1]s: invalid discipline function",
		// A parse failure by every other measure, and 1 rather than this
		// dialect's syntax-error status.
		ForNameStatus:     1,
		ForName:           "%[1]s: invalid variable name",
		SyntaxErrorStatus: 3,
		// The status is never reached — a file `.` cannot open ends the script
		// here — but the wording is, and it names the operand and the reason in
		// brackets rather than after a colon.
		DotCannotOpen:          ".: %[1]s: cannot open [%[2]s]",
		DotNoOperandUnprefixed: true,
		// The reason this shell gives for text that is not a number, and it
		// is the same one it gives for an expression it cannot parse:
		// measured 2026-09-11, `x=1abc; $((x+1))` and `x="1 2"; $((x+1))`
		// are both `arithmetic syntax error`.
		//
		// A reason and nothing else, which is the field's contract:
		// ArithError above supplies the text being blamed, and this sentence
		// carrying a `%[1]s` of its own named it a second time — `$((1e3abc))`
		// was `1e3abc: 1e3abc: arithmetic syntax error` against ksh93's one
		// naming (#2767). Only a numeral whose *leading* part reads and whose
		// tail does not reaches this field here, which is why the doubling
		// took a shape as particular as `1e3abc` to see: every other
		// unreadable literal is worded through DigitTooGreatForBase.
		//
		// It said `parameter not set` until #1629, which is the sentence a
		// *name-shaped* value earns — and it earns it by being looked up and
		// found unset, not by failing to be a number. Now that the lookup
		// really happens (Semantics.ArithRecursedNameMustBeSet) the stand-in
		// is not merely unnecessary: it was answering the wrong sentence for
		// every value that is no name at all.
		InvalidNumber: "arithmetic syntax error",
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
		GetoptsNumericArgument: "-%[1]s: numeric argument expected",
		// `getopts` is the one builtin whose complaint here carries no line.
		// Every other diagnostic this shell writes names a location — either
		// `file: line N:` or `file[N]:` — and these three write the script's
		// path and then the message, on the script route and on `-c` and
		// standard input alike, where the name is the shell's own.
		GetoptsNamesNoLine: true,
		CdCannotChange:     "cd: %[1]s: [%[2]s]",
		// One message for both, where bash names which variable was missing.
		CdHomeNotSet: "cd: bad directory",
		// The same sentence for an empty operand and for a HOME set to
		// nothing, which is what makes this shell's answer one rule.
		CdEmptyOperand: "cd: bad directory",
		// A third operand to the substitution form writes the usage block
		// and no sentence of its own, at the usage status.
		CdTooManyOperandsShowsUsage: true,
		CdTooManyOperandsStatus:     2,
		// `cd old new` where old is not in the current directory's path.
		// The operand is not named.
		CdBadSubstitution: "cd: bad substitution",
		CdOldpwdNotSet:    "cd: bad directory",
		// A warning and not a refusal: `%T` still writes the current time
		// after it, and only the status says anything went wrong.
		PrintfBadDateOperand:      "printf: warning: invalid argument of type T",
		PrintfBadVerb:             "printf: %[1]s: unknown format specifier",
		PrintfArithOperandFailure: "printf: %[1]s",
		// And a second line after it, naming the conversion character
		// rather than the operand — with the status split that comes with
		// it: a reading that failed reports 1 and an expression that
		// failed reports 0 (#2765). See Diagnostics.PrintfArithArgumentType
		// for the seventeen operands this was measured over and the two
		// that do not follow.
		PrintfArithArgumentType: "printf: warning: invalid argument of type %[1]s",
		// And a number the *conversion* cannot hold, which is a separate
		// complaint from a reading that overflowed: `printf '%f'` of the
		// same operand is silent here.
		PrintfIntegerOverflow:     "printf: warning: %[1]s: overflow exception",
		PrintfBadOption:           "printf: %[1]s: unknown option",
		TrapConditionRequired:     "trap: condition(s) required",
		PrintfBadOptionShowsUsage: true,
		UmaskBadMask:              "umask: %[1]s: bad number",
		// The name leads and the builtin follows it, which is the reverse of
		// everyone else — and unprefixed.
		AliasNotFound:              "%[2]s: %[1]s not found",
		AliasNotFoundUnprefixed:    true,
		AliasInvalidName:           "%[1]s: %[3]s: invalid alias name",
		AliasInvalidNameUnprefixed: true,
		UnaliasUsage:               "Usage: unalias [-a] name...",
		UnaliasUsageUnprefixed:     true,
		UmaskBadSymbolicMode:       "umask: %[1]s: bad format",
		UmaskBadOption:             "umask: %[1]s: unknown option",
		UmaskUsage:                 "Usage: umask [-S] [mask]",
		UmaskUsageUnprefixed:       true,
		LetNoExpression:            "Usage: let [ options ] [expr ...]",
		LetNoExpressionStatus:      2,
		LetNoExpressionUnprefixed:  true,
		// `ulimit -Z` is `ulimit: -Z: unknown option` here, with the usage
		// line under it and status 2 — the same shape every other bad
		// option in this dialect takes. It had been the string `not
		// supported`, which is the text this shell writes in the *-a
		// listing* for a resource the platform has no limit for; the two
		// are different sentences about different things and the listing's
		// had been pasted here. Nothing caught it because the field takes
		// the letter as its one verb and that copy had no verb in it at
		// all, so the letter the reader has to change was never printed.
		UlimitBadOption: "ulimit: -%[1]s: unknown option",
		UlimitBadNumber: "ulimit: %[1]s: parameter not set",
		// ksh93 names the operand as written and brackets the reason.
		UlimitCannotChange: "ulimit: %[2]s: limit exceeded [%[3]s]",
		BuiltinBadOption:   "%[1]s: %[2]s: unknown option",
		BadOptionNaming:    interp.BadOptionWholeWord,
		// `ls=/bin/ls`: ksh93's hash is `alias -t`, so its listing is the
		// alias shape — which is zsh's too, by a different road.
		HashListing: interp.HashListingNameEqualsPath,
		// `wait` names the builtin and the process id when the child it
		// reaped was ended by a signal — its own sentence, and not the
		// general one this shell writes for a foreground command a signal
		// killed. Measured 2026-09-17: `wait: <pid>: Terminated` then 271,
		// and `wait: <pid>: User signal 1` then 286 (#3392).
		WaitSignalNotice: "wait: %[1]d: %[2]s",
		WaitBadJob:       "wait: %[1]s: Arguments must be %%job, process ids, or job pool names",
		WaitBadJobStatus: 1,
		// ksh93 has `--version` here, which this shell does not.
		UnimplementedOptionLetters: map[string]string{
			// `set` letters ksh93 has and this shell does not: -b job
			// notices, -k assignment-anywhere, -r restricted, and -s
			// sorting the positional parameters. Measured 2026-09-05 by
			// asking ksh93 for every letter of the alphabet in both cases
			// and both signs.
			//
			// `-B` and `-p` have left this sentence without leaving the
			// string, the drift #3088 swept for: brace expansion is built
			// and `set -B` is silently 0, and `-p` is refused under its
			// *name* rather than through this table. A comment naming a
			// letter as missing is read the way the string is.
			// `-A` has left this list: it assigns an array and is
			// implemented, in Semantics.SetArrayLetter.
			// `-t` is not here: this shell really does stop after one
			// command, and the letter is the only spelling it has for the
			// option — see Semantics.SetHasTheTLetter.
			// `H` left this list in #3093, when the expander behind it was
			// built: ksh93's `-H` is `set -o histexpand`, measured — `set -o`
			// there lists `histexpand off` and the letter moves that row —
			// and a letter refused while the name is wired would refuse what
			// the name grants.
			// `G` left it in #3152 for the same reason and by the other
			// road: it is `set -o globstar`, and the name is now wired to the
			// walk, so the letter goes through the dialect's own letter table
			// above. A letter left here while its name moves is the paired
			// tables drifting apart, which is the drift #3088 swept for.
			// `s` left it when the sort was built: Semantics.
			// SetSLetterSortsTheOperands.
			"set": "br",
			// ksh93 answers --version on most builtins, and has its own
			// letters for these two.
			"wait": "-",
			// What is left of read's letters: compound -C, -S's csv
			// splitting and -v's default text. `-p` is implemented: it
			// reads the coprocess `cmd |&` started, and is
			// ReadNoCoprocess's wording when none is running.
			"read": "-CSv",
			"type": "-qv",
			// `jobs -n` has left this list: the record of what the shell
			// has already reported is on the job now, and the letter is in
			// Semantics.JobsOptions with
			// JobsListsWhatChangedSinceTheLastReport deciding what it means
			// (#3390).
			// typeset's letters this engine does not hold: the floats'
			// -F, padding and alignment, mappings and the rest of its usage
			// line.
			//
			// `-n` has left this list: the **name reference** is built
			// (#2553), under this word and under `nameref`, and
			// interp/nameref.go is the whole of it.
			//
			// `-f` has left this list — the function listing is built
			// (#1494). `-t` has not: on a `-f` line it *traces* a function,
			// which this shell does not do, and a letter taken and dropped
			// would read as one that worked — `typeset -ft f` writing the
			// body is the opposite of the silence ksh93 answers with.
			//
			// `-u` is not here and cannot be: it is the upper-case
			// attribute, which this shell has, and only its meaning *on a
			// `-f` line* is missing — there it marks a name to be read from
			// `$FPATH`. It is refused in that position alone, through
			// UnimplementedOptionLettersOnAFunctionLine below.
			//
			// `-T` has left this list too, and only half of it is built:
			// the letter names a type here, and every form of it but the
			// *definition* is the silence that shell answers with — see
			// Semantics.DeclareTypeLetter. The definition is a compound
			// value (#2620), and interp/declaretype.go goes on saying the
			// letter is missing for that one shape, in this table's own
			// sentence.
			//
			// `-H` has left it outright, and it is *not* the letter zsh
			// spells the same way: here it is an inert attribute that is
			// recorded and said back — `typeset -H h=hid` lists as `typeset
			// -H h=hid`, value and all — where zsh's withholds the value.
			// See Semantics.DeclareHideValueLetter and
			// interp/declarehide.go. The lower-case `-h` stays: it takes a
			// *string* argument here (`[-h string]` in the usage block) and
			// is neither zsh's hide-in-scope nor anything this shell does.
			//
			// `-C` has left it as well: the compound-variable letter is
			// built, and it is the declaration half of the kind `c=(a=1)`
			// makes — see interp/compoundvariable.go (#2620).
			"typeset": "-bsLRSXZ",
			// `functions` is `typeset -f` under a second name, so the
			// letters it is missing are read off its own set: `-t` traces a
			// function and `-u` marks one to be read from `$FPATH`, both of
			// which ksh93 takes there and this shell does not do, and `-M`
			// is a character mapping rather than zsh's math facility.
			//
			// The alias is what a script actually meets, so this entry is
			// reached only by the registered builtin behind it — `functions`
			// written after `unalias functions`, from a route where the
			// alias never expanded. The line a script writes is
			// `typeset -f -u`, and that is refused by the two tables under
			// `typeset`. Both are kept: a name that can be reached has to
			// answer, and the two must not disagree. `-F`
			// and `-m` are deliberately absent: measured, `functions -F`
			// and `functions -m` are the usage line on ksh93u+, so that
			// shell has not got them either and "unknown" is the truth.
			// See #2192.
			// `-t` and `-u` have left this list: they are the two marks a
			// `-f` line puts on a function and are built since #2192 — see
			// fpath.go. `-M` is a character mapping rather than zsh's math
			// facility and is what is left, along with the letters below.
			"functions": "",
			// `integer` reads typeset's letters, so it is missing exactly
			// the ones typeset is missing — including the `--version` this
			// shell answers on both names.
			// `integer` is missing exactly what `typeset` is missing, with
			// one letter of its own: `-f` is not on this list because it is
			// not *unimplemented* — the function listing is built (#1494)
			// and `typeset -f` uses it. It is simply not a letter this word
			// takes. Measured 2026-09-12, `integer -f w=1` is the only
			// letter of typeset's grammar that ksh93u+ refuses under the
			// second name, and it refuses it with the usage line alone.
			//
			// `-T` is the second of those, found when the letter was
			// measured for `typeset`: `integer -T TS ts` is that same usage
			// block at 2 where `typeset -T TS ts` is a silent 0, because the
			// word already carries a type and the letter has nothing left to
			// name. So it is not *unimplemented* under this name and leaves
			// this list with the one above it.
			//
			// `-H` leaves it with the one above too, and by the same route:
			// the letter is built, and `integer -H n=5` is the usage block
			// at 2 because the word already carries the integer attribute
			// and the two are exclusive — measured, `typeset -iH n=5` is
			// that same block. Refused for the conflict rather than for the
			// letter, which is what interp/declarehide.go says.
			//
			// `-C` leaves this list with the one above it, for the reason
			// every letter here shares one: `integer` reads typeset's whole
			// grammar.
			"integer": "-bsLRSXZ",
		},
		// `-u` on a `-f` line, which is the one letter that cannot go in
		// the list above: it is also the upper-case attribute, and this
		// shell has that — `typeset -u v=abc` is `ABC`. Only its meaning
		// under `-f` is missing, where it marks a name whose body is read
		// from `$FPATH` the first time it is called.
		//
		// Refused rather than left to fall through, because falling through
		// is not silence: the line reached the *listing* instead, so
		// `typeset -fu nm` was a silent status 1 — a listing of a function
		// that is not there — where ksh93 marks the name and is 0. A wrong
		// status in silence is the worst of the three answers available,
		// and an honest refusal is the best one until the search exists.
		// `autoload` is the same line under an alias (`alias
		// autoload='typeset -fu'`), so it is refused too and by the same
		// sentence.
		//
		// `-t` is not here: it is in the list above, under `typeset`, and
		// belongs there because this shell has no tracing on a variable
		// line either.
		//
		// The remaining half of #2192 is the `$FPATH` search itself, plus
		// the ksh93 rendering of a marked name — `typeset -f nm` writes
		// `typeset -fu nm`, a declaration with no body, where the
		// Runner.SetUndefinedFunctions seam hands back a *block*.
		// Nothing is left here: `-u` reads a body from `$FPATH` and `-t`
		// traces, and both are built since #2192. The table stays because
		// the *position* is the distinction it exists for — a letter this
		// shell has on a variable line and not on a function one — and the
		// next such letter goes here rather than into a second table.
		UnimplementedOptionLettersOnAFunctionLine: map[string]string{},
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
		// And `read`, which names itself in its builtin location as `set`
		// does — and then calls itself a warning, which nothing else in the
		// panel does. See ReadonlyVariableInRead.
		ReadonlyRefusalNamesBuiltin: map[string]bool{"set": true, "read": true},
		ReadonlyVariableInRead:      "%[2]s: warning: %[1]s: is read only",
		// A failed history substitution names what was *typed* here, where
		// bash names the modifier it rewrote the line into: measured
		// 2026-09-15 through a pseudo-terminal, `^hello^goodbye^` against a
		// line without `hello` is `ksh: ^hello^goodbye^: substitution
		// failed`. The event-not-found wording is bash's and stays at the
		// fallback.
		HistorySubstitutionFailed: "%[2]s: substitution failed",
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
		// A function said back, in the two spellings this shell tells
		// apart. The body starts at its own `{`, laid out by FunctionLayout,
		// so the header is the name and the join: `f(){ :; }` and `function
		// g { typeset x=1; }`. Measured 2026-09-12 on ksh93u+ through
		// `od -c`.
		//
		// Keeping the keyword is not cosmetic here. `typeset` declares a
		// local in a keyword body and assigns the global in a parenthesised
		// one — Semantics.TypesetLocalNeedsKeywordFunction — so the two
		// spellings are two programs, and `eval "$(functions g)"` has to
		// give back the one it was handed (#1494, #1406).
		FunctionListingHeader:        "%[1]s()%[2]s",
		FunctionListingKeywordHeader: "function %[1]s %[2]s",
		// And `whence -v` says what a name still waiting for its body is,
		// which is neither a function nor nothing: measured, `typeset -fu
		// zz; whence -v zz` is `zz is an undefined function`.
		TypeUndefinedFunction: "%[1]s is an undefined function",
		// And a name still waiting for a body is a *declaration* and not a
		// listing at all: measured, `typeset -fu nm; typeset -f nm` is
		// `typeset -fu nm` with no header and no braces. See fpath.go.
		UndefinedFunctionListing: "typeset -fu %[1]s",
		// Both headers are the fallback rather than the ordinary path now:
		// what this shell really writes is the definition's own source text,
		// terminator and all, and the parser keeps it — see
		// syntax.Dialect.FunctionDefinitionIsSourceText. The headers still
		// answer for a function whose declaration carries no text: one built
		// by an embedder, and one this shell is still waiting to read a body
		// for (#2610).
		FunctionListingIsSourceText: true,
		// And the names-only listing keeps the same distinction with
		// punctuation instead of a word: measured, `f() { :; }; function g
		// { :; }; typeset +f` writes `f()` and then `g`.
		FunctionNameListing:        "%[1]s()",
		FunctionNameListingKeyword: "%[1]s",
		BuiltinComplaintName: map[string]string{
			"type": "whence", "integer": "typeset", "functions": "typeset",
			// `nameref` is `typeset -n` under a second word, and its
			// complaints say so: measured 2026-09-15, `nameref 1x=v` is
			// `typeset: 1x=v: is not an identifier` — the *other* word's
			// name, which is what says the spelling is a front rather than a
			// builtin of its own.
			"nameref": "typeset",
		},
		// Two wordings, split between `export` and the other two, and the
		// operand quoted back as given.
		// `typeset -h` with nothing behind it, which is the one refusal that
		// letter has here.
		DeclareHideStringMissing: "%[1]s: -h: string argument expected",
		// And the export *letter* brings export's sentence with it:
		// `typeset -x 3` is `is not an identifier` where `typeset -r 3` is
		// `invalid variable name`. See
		// Diagnostics.ExportLetterTakesExportsBadName.
		ExportLetterTakesExportsBadName: true,
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
			// And the second spelling of that word takes the same wording,
			// keyed by what the script wrote rather than by what the
			// complaint calls it: the table is read before the rename above
			// is applied, so a missing entry here would fall back to the
			// substrate's sentence under ksh93's name.
			//
			// The real shell is one word apart on this line — `typeset -n
			// 1x=v` is `is not an identifier` there where the plain `typeset
			// 1x=v` is `invalid variable name`, a wording the `n` letter has
			// to itself. That is recorded rather than modeled: it would be a
			// per-letter bad-name wording, which is a table of its own for
			// one row.
			"nameref": "%[1]s: %[2]s: invalid variable name",
			// `read` takes readonly's wording too, and it names the part in
			// front of a prompt `?`: `read "1bad?p"` is `1bad`, and
			// `read "?p"` is the empty word the split left.
			"read": "%[1]s: %[2]s: invalid variable name",
		},
		// The `M` letter's two refusals — see
		// interp/declaremapping.go, where the letter names a character
		// mapping rather than zsh's math facility.
		DeclareUnknownMapping:    "%[1]s: %[2]s: unknown mapping name",
		DeclareMappingNeedsAName: "%[1]s: -M requires argument when operands are specified",
		// A name reference's two refusals. Measured 2026-09-15 on ksh93u+,
		// `env -i` with a scratch HOME:
		//
		//	typeset -n r=1bad    typeset: 1bad: invalid variable name
		//	typeset -n r=r       typeset: r: invalid self reference
		//
		// The first is this shell's ordinary bad-name sentence rather than
		// one of the letter's own, which is where it parts from bash — so it
		// is written out here rather than left to fall back, because the
		// substrate's fallback is bash's wording.
		NamerefBadTarget:     "%[1]s: invalid variable name",
		NamerefSelfReference: "%[1]s: invalid self reference",
		// A `${!r}` over a reference with nothing to point at, which is the
		// one indirection refusal this shell makes: IndirectionYieldsName
		// sends the rest of that expansion somewhere else here, so the two
		// sentences beside this one in Diagnostics are bash's alone.
		// Measured 2026-09-17 on ksh93u+ 2012-08-01 from a script file under
		// `env -i`: `typeset -n u; echo "[${!u}]"` writes `u: no reference
		// name` and the script is over, at 1.
		IndirectionUnaimedReference: "%[1]s: no reference name",
		// The one nameref sentence this shell and bash write identically,
		// measured on both: `r: reference variable cannot be an array`.
		NamerefCannotBeAnArray: "%[1]s: reference variable cannot be an array",
		// NamerefCircularWarning is deliberately empty and is not a gap: a
		// cycle is refused at the declaration here — see
		// Semantics.NamerefCycleIsRefused — so there is never a read through
		// one for this shell to warn about.
		BuiltinBadNameKeepsValue: true,
		BuiltinUsageUnprefixed:   true,
		// Three of this shell's builtins write their complaint bare, where
		// every other one of them carries the shell's name. Measured over
		// the whole set on ksh93u+ 2012-08-01; nothing the three have in
		// common explains it, so the list is the measurement (#2345).
		BuiltinComplaintUnprefixed: map[string]bool{
			"alias": true, "builtin": true, "pwd": true,
		},
		BuiltinUsage: map[string]string{
			"set": "Usage: set [-sabefhkmnprtuvxBCGH] [-A name] [-o[option]] [arg ...]",
			// The same usage block under the array letter, which is what
			// this shell prints for `set -A` with no name after it.
			"export":   "Usage: export [-p] [name[=value]...]",
			"readonly": "Usage: readonly [-p] [name[=value]...]",
			"read": "Usage: read [-ACprsSv] [-d delim] [-u fd] [-t timeout] [-n count] [-N count]\n" +
				"            [var?prompt] [var ...]",
			"trap": "Usage: trap [-p] [action condition ...]",
			// Two lines: the ordinary form and the substitution form, which
			// is this shell announcing that `cd old new` is a form rather
			// than a mistake.
			"cd": "Usage: cd [-LP] [directory]\n   Or: cd [ options ] old new",
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
			// And under `functions`, which is `typeset -f` again: measured,
			// `functions -Q` on ksh93u+ answers `typeset: -Q: unknown
			// option` with these same three lines under it, so both the
			// name in the complaint and the usage belong to the builtin
			// rather than to the word that reached it.
			"functions": "Usage: typeset [-bflmnprstuxACHS] [-a[type]] [-i[base]] [-E[n]] [-F[n]] [-L[n]]\n" +
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
			// The rest of the table, harvested a builtin at a time with a
			// letter none of them has. A builtin with no entry here printed
			// its complaint with nothing under it, which is the shape 51
			// corpus rows were failing on (#2345).
			"alias":    "Usage: alias [-ptx] [name[=value]...]",
			"unalias":  "Usage: unalias [-a] name...",
			"command":  "Usage: command [-pvxV] [command [arg ...]]",
			"builtin":  "Usage: builtin [-dls] [-f lib] [pathname ...]",
			"whence":   "Usage: whence [-afpqv] name  ...",
			"print":    "Usage: print [-enprsvC] [-f format] [-u fd] [string ...]",
			"umask":    "Usage: umask [-S] [mask]",
			"pwd":      "Usage: pwd [-LP]",
			"let":      "Usage: let [ options ] [expr ...]",
			"eval":     "Usage: eval [ options ] [arg...]",
			"exec":     "Usage: exec [-c] [-a name] [command [arg ...]]",
			"exit":     "Usage: exit [ options ] [n]",
			"return":   "Usage: return [ options ] [n]",
			"break":    "Usage: break [ options ] [n]",
			"continue": "Usage: continue [ options ] [n]",
			"getopts":  "Usage: getopts [-a name] opstring name [args...]",
			"hist":     "Usage: hist [-lnprs] [-e editor] [-N num] [first [last] ]",
			".":        "Usage: . [ options ] name [arg ...]",
			"bg":       "Usage: bg [ options ] [job ...]",
			"fg":       "Usage: fg [ options ] [job ...]",
			"disown":   "Usage: disown [ options ] [job ...]",
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
		// One complaint per *character* of the word, then the usage block
		// once. That is this shell's option parser rather than anything
		// about `kill`: `kill -NOPE` is `-N`, `-O`, `-P`, `-E` and then the
		// block, at 2, and `kill -99x` repeats the `-9`. Measured
		// 2026-09-17 on ksh93u+ 2012-08-01 (#3167). The block left the
		// wording because it is written once however many lines came
		// before it.
		KillIllegalOption:          "kill: -%[1]s: unknown option",
		KillIllegalOptionPerLetter: true,
		KillIllegalOptionUsage:     kshKillUsage,
		KillMissingSignalArgument:  "kill: %[1]s: signame argument expected\n" + kshKillUsage,
		KillUsageStatus:            2,
		KillBadOptionStatus:        2,
		KillUsageUnprefixed:        true,
		KillTargetUnprefixed:       true,
		// `[ -Q x -a -n x ]` is `[: x: unknown operator` here: the dash word
		// is an operand like any other, and the complaint names the second of
		// the two the primary is left with (#1290).
		TestUnknownLongOperator: interp.TestUnknownOperatorLeavesAnOperand,
		TestUnaryExpected:       "%[2]s: %[1]s: unknown operator",
		TestBinaryExpected:      "%[2]s: %[1]s: unknown operator",
		TestIntegerExpected:     "%[2]s: %[1]s: integer expected",
		// No TestTooManyArguments: this shell has no such sentence, and the
		// reading is why — it drops the words behind an expression instead
		// of counting them (Semantics.TestReadsOneExpressionOffTheOperands).
		// What it says about a list the grammar could not finish is below.
		TestIncorrectSyntax: "%[2]s: incorrect syntax",
		TestOperandExpected: "%[2]s: argument expected",
		TestMissingBracket:  "[: ']' missing",
		OptionListingHeader: "Current option settings",
		OptionListingWidth:  25,
		PlusOListsActive:    true,
		// Labeled lines, one figure each, and no children's times at all —
		// genuinely less information than the other three report.
		TimesLayout:   interp.TimesUserAndSystem,
		TimesDecimals: 2,
		// The `time` keyword's report is bash's shape with two decimals —
		// and a bare `time` reports the shell's own user and sys, no real,
		// where bash reports a run of nothing.
		TimeDecimals: 2,
		// The same variable and the same vocabulary as bash, byte for
		// byte; only the complaint about an unknown directive differs,
		// and it names the character alone.
		TimeFormatVariable:     "TIMEFORMAT",
		TimeFormatBadDirective: "%[2]s: bad format character in time format",
		TimeBare:               interp.TimeBareShellUserSys,
	}
	return withPromptWordings(d)
}

// withPromptWordings fills in what this shell writes about a parse failure at
// a **prompt**, which is each of the sentences above with the line taken out
// of it.
//
// Measured 2026-09-11 under `-i`, the shell's own path normalized:
//
//	if; then          ksh: syntax error: `;' unexpected
//	if true; then     ksh: syntax error: `then' unmatched
//	x='never closed   ksh: syntax error: `'' unmatched
//	v=$(echo hi       ksh: syntax error: `(' unmatched
//	echo one &&       ksh: syntax error: `end of file' unexpected
//	echo $((1+        ksh: syntax error: `(' unmatched
//	echo ${x@         ksh: syntax error: `newline' unexpected
//
// Every one of them names the line in a script and none of them do here, and
// this shell is the only one in the panel that carries the line *inside* the
// sentence — the other three put it in the location, where one field answers
// for all of their wordings at once. See interp.Diagnostics.PromptLocation.
//
// Each pair is built by one call so the two halves cannot drift: a prompt
// sentence written out beside its script sentence is a second copy to keep in
// step, and the rule relating them is exactly one clause.
func withPromptWordings(d interp.Diagnostics) interp.Diagnostics {
	d.SyntaxUnexpected, d.PromptSyntaxUnexpected = parseWording(3, "`%[1]s' unexpected")
	// An arithmetic command in a position the grammar has none for is named
	// by the `((` that opened it here, where bash and zsh name the expression
	// it held — see Diagnostics.SyntaxUnexpectedNamesTheOpener.
	d.SyntaxUnexpectedNamesTheOpener = true
	// A refused word comes back with its quoting off — `"zzz"` is `zzz` —
	// unless it holds an expansion, and then every quote in it stays:
	// `"$x"` is `"$x"` and `"a"~` is `a~`. See UnexpectedWordNaming.
	d.UnexpectedWordNaming = interp.UnexpectedWordIsSourceTextWhenItExpands
	// A refused newline is named on the line it ends rather than on the line
	// it was written at the end of, which is dash and zsh's answer too and
	// leaves bash the odd one out (#1364).
	d.UnexpectedNewlineIsOnTheNextLine = true
	d.Unterminated, d.PromptUnterminated = parseWording(6, "`%[3]s' unmatched")
	// Nothing is unmatched when nothing was open, so the end of input is
	// named as the thing that was unexpected instead.
	d.UnterminatedNoConstruct, d.PromptUnterminatedNoConstruct = parseWording(6, "`end of file' unexpected")
	// Only the substitutions can go unmatched here — a quote the input runs
	// out inside is closed and run, which is the grammar flag. The `"` case
	// is a `${` that began inside a double quote, and it is worded as the
	// quote character standing where it should not.
	d.UnmatchedQuote, d.PromptUnmatchedQuote = parseWording(4, "`%[1]s' unmatched")
	d.UnmatchedCmdSubst, d.PromptUnmatchedCmdSubst = parseWording(4, "`(' unmatched")
	// A process substitution reaches a different *diagnosis* here, not only a
	// different sentence: `$(` is an unmatched parenthesis named at the
	// opener's line, and `<(` is the end of the file named at the line the
	// file ended on. Measured — `cat <(echo hi` in a script answers "syntax
	// error at line 2: `end of file' unexpected" where `v=$(echo hi` answers
	// "syntax error at line 1: `(' unmatched".
	d.UnmatchedProcSubst, d.PromptUnmatchedProcSubst = parseWording(5, "`end of file' unexpected")
	// Exactly what it says for `$(`: this shell blames the parenthesis and
	// the opener's line and does not distinguish the two constructs. Written
	// as its own call rather than shared with UnmatchedCmdSubst so that a
	// later change to one cannot silently move the other.
	d.UnmatchedArithSubst, d.PromptUnmatchedArithSubst = parseWording(4, "`(' unmatched")
	// An expansion the input ran out inside is the brace unmatched, and it is
	// the brace for the command form `${ echo hi` as much as for a `${x` the
	// input simply stopped after. It said `${x{: bad substitution` until
	// #2232 — a runtime sentence borrowed for a parse failure, with the
	// opener's line missing and the next construct's brace stuck on the end
	// of the quoted text.
	d.UnmatchedBraceSubst, d.PromptUnmatchedBraceSubst = parseWording(4, "`{' unmatched")
	// A parameter form a character stopped is that character standing where
	// it should not, at the line the `${` is on: `echo ${x` and a newline is
	// ``syntax error at line 1: `newline' unexpected``, and `echo ${x ` names
	// the space. See Diagnostics.UnmatchedBraceSubstAtStop.
	d.UnmatchedBraceSubstAtStop, d.PromptUnmatchedBraceSubstAtStop = parseWording(4, "`%[6]s' unexpected")
	// ksh93 does not call this a bad substitution: it is a syntax error
	// naming the character it could not read.
	d.BadSubstitution, d.PromptBadSubstitution = parseWording(2, "`%[1]s' unexpected")
	// And a refused flag group is quoted back as the rest of the word rather
	// than as the `(` — see Diagnostics.FlagGroupNamesTheWordTail.
	d.FlagGroupNamesTheWordTail = true
	// And a `${` standing where the expansion's name belonged is blamed on a
	// `!` — a character nothing in the input holds. `echo ${${v}}` is
	// ``syntax error at line 1: `!' unexpected`` here where `echo ${x${v}}`
	// with the same characters in it names the `$`. See
	// Diagnostics.NestedNameIsBlamedOnTheBang, where the measurement is.
	d.NestedNameIsBlamedOnTheBang = true
	// A C-style `for` header with fewer than two separators is an unexpected
	// closer here, whatever the header held: measured 2026-09-12, `for (())`,
	// `for (( ))`, `for ((1;2))` and `for ((i=0))` are each `syntax error at
	// line 1: `))' unexpected`, so the sentence names neither the header nor
	// the part the way the other two columns do. More than two separators is
	// accepted — see syntax.Dialect.ForArithExtraSeparators (#2225).
	d.ForArithHeader, d.PromptForArithHeader = parseWording(3, "`))' unexpected")
	// A refusal with no wording of its own — the substrate's own sentence,
	// which this shell still puts its line in front of. Measured from a
	// script: `function a 1b { :; }` is `syntax error at line 1: invalid
	// reference list`, where the other three print the sentence alone.
	d.SyntaxError, d.PromptSyntaxError = parseWording(2, "%[1]s")
	return d
}

// parseWording is one sentence about a parse failure in both of its shapes:
// with the line for a script and without it at a prompt.
//
// lineArg is the numbered argument that carries the line, which differs per
// sentence because the arguments a wording is handed differ per failure.
func parseWording(lineArg int, rest string) (inAScript, atAPrompt string) {
	return fmt.Sprintf("syntax error at line %%[%d]d: %s", lineArg, rest),
		"syntax error: " + rest
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
	// The prompt-escape table, so the escape language travels with the
	// dialect rather than only with the front end. ksh93's is a language of
	// one rule — the backslash goes and the letter stands — and a `!` that is
	// the history number, and a runner that was never told either of those
	// draws both as text. #1455.
	//
	// `driver.Shell.PromptStyle` hands the same value to the same runner on
	// the way to the prompt drawer; the two agree because they are the same
	// function's result. An embedder who applies this dialect to a Runner of
	// their own never builds a Shell, and is the caller this is for.
	r.SetPromptStyle(PromptStyle())
	// What the last `=~` matched, under this shell's name for it. The
	// captures are the only way to read what an ERE matched here — there is
	// no second spelling — so without this the operator's whole point was
	// out of reach in this dialect (#2916). The name is a dotted one, which
	// reads and subscripts like any other since #2620 and #3347.
	//
	// The two questions the record raises beyond its name are axes on the
	// vector rather than anything set here: a failed match leaves it alone,
	// and a group that took no part is left out of it.
	r.SetRegexMatch(".sh.match")
	// The C math library under the names arithmetic calls it by, all
	// sixty-nine of them. This shell has no `zmodload` and nothing to load:
	// they are simply there, which is what `$(( sqrt(4) ))` needing no
	// preamble means. See mathfunc.go.
	registerMathFuncs(r)
	// The three other questions `set -o globstar` has to answer here, set
	// beside the walk for the same reason dialect/bash does it: the walk
	// asks them only where that option has already said `**` crosses levels
	// at all, and `globstar` is the only name this shell has for any of
	// them. Measured 2026-09-16 on ksh93u+ 2012-08-01, each rendered one
	// word per `[…]` because `echo` joins with spaces and hid exactly this
	// kind of difference all night:
	//
	//	set -o globstar; printf '[%s]' d/**     every level, not `d/e` alone
	//	                 printf '[%s]' '**/'    names a symlinked directory
	//	                 printf '[%s]' '**/**/' one component, not a cross
	//	                                        product — 6 words in a tree
	//	                                        where zsh answers 48
	//
	// So all three are bash's answers too, and `**/a*`, `**`, `**/` and
	// `**/x` agree byte for byte between the two shells (#3152).
	r.SetMatchOption(interp.StarStarAloneCrossesDirectories, true)
	r.SetMatchOption(interp.StarStarSeesLinkedDirectories, true)
	r.SetMatchOption(interp.RepeatedStarStarIsOneComponent, true)
	// And the two this shell answers **differently** from bash and zsh, both
	// of which are left off. A `**` that matched zero levels is not the
	// directory the walk stood in — `printf '[%s]' d/**` is `[d/e][d/e/f]`
	// here against bash's `[d/][d/e][d/e/f]` — and a walk will not begin
	// inside a directory reached through a symbolic link, so `s/**` with `s`
	// a link to `r` is no match at all where bash lists `[s/][s/x]`. See
	// interp.StarStarZeroLevelIsTheDirectoryItStartsFrom and
	// interp.StarStarDescendsFromALinkedStart for the tables (#3152).

	// `set -G` is this shell's letter for `globstar`, and the letter and the
	// name are one request with one answer: `set -G; set -o` reports
	// `globstar on` and `$-` gains a `G`, measured. Declared through the
	// dialect's letter table rather than the shared one because `G` is a
	// letter two shells spell different options with — zsh's `-G` is
	// `nullglob` — which is exactly what that table is for. See
	// interp.Runner.SetOptionLetterNames.
	r.SetOptionLetterNames(map[rune]string{'G': "globstar"})
	// The `set -o` names beyond the ones every shell has. Measured 2026-09-15
	// against ksh93u+ 2012-08-01 by diffing the whole listing: 32 rows there
	// against 20 here, and five of the twenty spelled the other way round
	// (#2925). The listing is a capture surface — a script saves and
	// restores option state with `eval "$(set +o)"` — so a name missing from
	// it is a state that save cannot carry.
	r.AddSetOptions(
		"bgnice",
		"braceexpand",
		// `emacs` and `nolog` are declared here since #3366, when BusyBox ash
		// was asked for them and had neither and they left the substrate's
		// common table. This shell lists `emacs` as itself and `nolog` as the
		// positive `log`, which is the negated map below reading this name.
		"emacs",
		"nolog",
		"globstar",
		"gmacs",
		"histexpand",
		"interactive",
		"keyword",
		"letoctal",
		"login_shell",
		"markdirs",
		"multiline",
		"pipefail",
		"privileged",
		"rc",
		"restricted",
		"showme",
		"trackall",
		"viraw",
	)
	// And the five this shell spells as the positive. `set -o` here writes
	// `clobber on` where the other four columns write `noclobber off`; both
	// spellings are still taken, and only the roster changes. See
	// interp.Runner.AddNegatedSetOptions for the three measurements that
	// shape it.
	r.AddNegatedSetOptions(map[string]string{
		"clobber": "noclobber",
		"exec":    "noexec",
		"glob":    "noglob",
		"log":     "nolog",
		"unset":   "nounset",
	})
	// Three of the names above are listed and refused: `set -o interactive`,
	// `set -o login_shell` and `set -o rc` are `bad option(s)` in both
	// directions here, word for word with a name this shell has never heard
	// of, while all three rows move with how the shell was started. The
	// substrate has `interactive` as a movable name for the shell that does
	// let a script write it, so this says otherwise for this one.
	// Three names a *script* may not move, and the command line that started
	// the shell may — see Semantics.ImmovableOptionsSetAtInvocation.
	r.AddImmovableSetOptions("interactive", "login_shell", "rc")
	// And one that is listed, *taken* in both directions, and never moves.
	// Measured 2026-09-16 on ksh93u+: `set -o privileged` is 0 with nothing
	// on stderr and the listing still says `privileged off` afterwards, and
	// `ksh -p` reports it off too — so the request is granted and the state
	// is out of a script's reach, which is neither a refusal nor a move.
	// bash takes the same word and does move it, which is why this is the
	// dialect's to say. See Runner.AddInertSetOptions (#3128).
	r.AddInertSetOptions("privileged")
	// The six rows this shell's **compiled default** has on, which is what
	// `set --default` puts back and is not the state the shell starts in.
	// Measured 2026-09-16 on ksh93u+ 2012-08-01 by reading the whole listing
	// after `set -o errexit; set --default`: exactly these six are on, and
	// `braceexpand`, `multiline` and `trackall` — all three on in a stock
	// shell — are off. The reset reaches behavior and not only the listing:
	// `set --default; echo {a,b}` prints `{a,b}`, and `$-` goes from `chsB`
	// to `cs` as the brace and tracking letters leave with their options.
	//
	// Five of them are this shell's positive spelling of a state the
	// substrate stores negated, and the reset goes through the same seam
	// `set -o clobber` does, so the inversion is written once. See
	// Runner.AddDefaultOnSetOptions.
	r.AddDefaultOnSetOptions("clobber", "exec", "glob", "log", "unset", "viraw")
	// ksh93 has a `builtin` of its own and it is a different command: it
	// *registers* builtins rather than running one. With no operands it
	// lists the table; each operand is a name to add, and one that is not
	// already a builtin here is not found, at 1 — with the builtin's own
	// name as the whole prefix, measured.
	// `-t` is the *tracked* alias table, which is a command cache and not an
	// alias table at all: `hash` is spelled `alias -t --` here, and this
	// shell ships that spelling as one of its preset aliases. So the letter
	// is answered by the builtin that already models the cache rather than
	// by a second implementation of it — measured, `alias -t x` and
	// `alias -t -- -r` are silent successes for any operand, which is what
	// `hash` answers in this dialect.
	//
	// What is not modeled either way is the *population* of the table by
	// running a command: real ksh93 lists `ls=/bin/ls` after `ls` has run
	// and this shell lists nothing. That gap is the `hash` builtin's and
	// predates the letter reaching it.
	if alias, ok := r.Builtin("alias"); ok {
		r.Register("alias", func(rr *interp.Runner, ctx context.Context, args []string) int {
			call := readTrackedAliasCall(args)
			if !call.tracked {
				return alias(rr, ctx, args)
			}
			if foreignAliasLetters(call.letters) != "" {
				// A letter this builtin has not got, beside the one that
				// reroutes. The refusal is `alias`'s and it names the letter
				// `alias` lacks rather than the one it has: measured,
				// `alias -tq`, `alias -qt` and `alias -t -q` are all
				// `alias: -q: unknown option` with `alias`'s usage line. So
				// the call reaches the builtin with the tracked letter
				// already gone, and the builtin does the refusing (#3036).
				return alias(rr, ctx, call.untracked)
			}
			hash, ok := rr.Builtin("hash")
			if !ok {
				return alias(rr, ctx, args)
			}
			operands := call.operands
			for i, w := range operands {
				if w == "--" {
					operands = operands[i+1:]
					break
				}
				// `-r` is the one letter the cache has here — `hash -r` is
				// `alias -t -- -r` — and it is the only word starting with a
				// dash this reading accepts. Every other one is refused
				// whole, including `-` alone and a `--name`: measured
				// 2026-09-15 on ksh93u+ 2012-08-01 over
				// `-t -p -x -r -d -a -q -tp -z - +t --help --r -rz -R`, where
				// `-r` and `+t` pass and the rest are `bad option(s)`.
				if w == "-r" {
					continue
				}
				if !strings.HasPrefix(w, "-") {
					break
				}
				// `bad option(s)` rather than `unknown option`, no usage
				// line, and fatal at 1 — all three unlike the refusal two
				// lines up, and all three `alias`'s, because `alias` is what
				// the script wrote. `hash` is not a command in this dialect
				// and naming it sent the reader to options that do not
				// exist here (#3036).
				rr.RefuseBuiltinUsagef("alias", "alias: %s: bad option(s)\n", w)
				return 1
			}
			if len(operands) == 0 {
				// Two of `alias`'s own letters still mean something beside
				// the tracked one, and neither is a letter the cache builtin
				// has. Measured with `ls` hashed: `alias -xt` writes nothing
				// — a tracked alias is never an exported one — and
				// `alias -pt` writes `alias ls=/bin/ls`, the bare listing
				// with the word that would define it back in front. With an
				// operand both are the plain `alias -t name`.
				if strings.ContainsRune(call.letters, 'x') {
					return 0
				}
				if strings.ContainsRune(call.letters, 'p') {
					for _, name := range rr.HashedCommandNames() {
						path, ok := rr.HashedCommandPath(name)
						if !ok {
							continue
						}
						_, _ = fmt.Fprintf(rr.Stdout, "alias %s=%s\n", name, path)
					}
					return 0
				}
			}
			return hash(rr, ctx, operands)
		})
	}
	r.Register("builtin", func(rr *interp.Runner, _ context.Context, args []string) int {
		// And it reads four letters and a `--`, which is what the usage line
		// this dialect already prints for it has said all along — `Usage:
		// builtin [-dls] [-f lib] [pathname ...]`. Measured 2026-09-18 on
		// ksh93u+ 2012-08-01, one probe at a time, under
		// `env -i PATH=/usr/bin:/bin LC_ALL=C`:
		//
		//	builtin -q            -q: unknown option, the usage line, 2
		//	builtin -lq           the same, naming only the bad letter
		//	builtin -f            -f: lib argument expected, the usage, 2
		//	builtin -- echo hi    builtin: hi: not found, 1
		//	builtin --            the whole listing, 0
		//	builtin +d            builtin: +d: not found, 1
		//
		// The plus sign is the row that says these are options and not a
		// general dash-word reading: `+d` is looked up as a *name* and is not
		// found, which is what this command already answered (#3473).
		args, opts, optArg, code := rr.BuiltinOptions("builtin", args, "dlsf:")
		if code != 0 {
			return code
		}
		switch {
		case strings.ContainsRune(opts, 's'):
			// The special builtins alone, and `-s` beats `-l` where both are
			// written: `builtin -ls` writes the same twenty-one rows `-s`
			// does there. What it lists is *this* shell's special roster
			// rather than that shell's, exactly as the bare listing lists
			// this shell's builtins — the two rosters differ because the
			// shells do, and a listing that named names we do not have would
			// be the worse answer.
			for _, name := range rr.BuiltinNames() {
				if rr.IsSpecialBuiltinHere(name) {
					_, _ = fmt.Fprintln(rr.Stdout, name)
				}
			}
			return 0
		case strings.ContainsRune(opts, 'd'):
			// `-d` removes a builtin that was *registered* by an earlier
			// `builtin -f`, and nothing in this shell can be. Measured
			// silent at 0 there for every shape probed — `builtin -d`,
			// `builtin -d echo` (with `echo` still working afterwards) and
			// `builtin -d nosuchzz` — so the operands are read and no name
			// is judged, which is what a delete of nothing does.
			return 0
		case strings.ContainsRune(opts, 'f'):
			// A shared library to load builtins out of, which this shell has
			// no loader for. ksh93 reports the dynamic loader's own sentence
			// at 1; the status is the shell's and the words are not, so this
			// says the library is not found in the voice this command
			// already uses for a name it cannot find.
			_, _ = fmt.Fprintf(rr.Stderr, "builtin: %s: not found\n", optArg['f'])
			return 1
		}
		if len(args) == 0 {
			// `-l` and a bare call write the same listing, which is measured:
			// `builtin` and `builtin -l` are byte-for-byte one output there,
			// and so is `builtin --` with nothing behind it.
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
	r.Unregister("compopt")
	r.Unregister("complete")
	// And neither `mapfile` nor its other name; both are bash's alone.
	r.Unregister("mapfile")
	r.Unregister("readarray")
	// This shell has no `enable`.
	r.Unregister("enable")
	// `${.sh.version}` is `$KSH_VERSION` under the name this shell's own
	// namespace gives it — measured on ksh93u+ 2012-08-01, 2026-09-13, the
	// two are the same sentence there. Produced rather than stored, because
	// a stored one would be listed: real ksh93 answers `set | grep '^\.sh'`
	// with nothing at all, where every ordinary dotted name it holds *is*
	// listed (`.foo=1; set` shows `.foo=1`). A producer is how this shell
	// already keeps `LINENO` and `RANDOM` out of that listing.
	//
	// The rest of the `.sh` namespace is mostly absent still. Measured on
	// the same build with `-c`, `.sh.match`, `.sh.pid`, `.sh.file`,
	// `.sh.command`, `.sh.edchar` and `.sh.sig` expand to the empty string
	// at status 0 — which is what an unset dotted name already does here, so
	// naming them would add a claim without adding an answer. `.sh.lineno`
	// and `.sh.subshell` answer `0` there and are genuinely dynamic; they
	// are left unset rather than pinned to a number that would be wrong as
	// soon as a script had two lines.
	//
	// `.sh.name`, `.sh.subscript` and `.sh.value` are not absent either. They
	// are the parameters a *discipline* is entered with, they exist only
	// while one is running, and they are ordinary dotted variables the rest
	// of the time — which is exactly what the empty answer above measured,
	// because the probe asked outside a discipline. See
	// interp/discipline.go (#3033).
	//
	// `.sh.fun` and `.sh.level` were measured the same way and were the same
	// mistake: asked at the top level they really are empty and `0`, and the
	// probe could not tell "this shell has no such name" from "the answer
	// here is nothing". Inside a function they are the function's name and
	// its depth — `function outer { … }` reads `outer`/`1`, and an `inner`
	// it calls reads `inner`/`2`. Produced rather than stored for the reason
	// `.sh.version` is: real ksh93 lists nothing from this namespace, and no
	// stored value could follow a call stack anyway.
	r.SetDynamic(".sh.version", func(*interp.Runner) string { return kshVersion })
	//
	// And the pair is **writable**, which is the half #3090 measured: an
	// assignment to `.sh.level` selects a frame and `${.sh.fun}` then answers
	// for that one, which is how a debug trap looks at its caller. Produced
	// parameters with no writer hear an assignment and drop it in silence —
	// the exact failure SetDynamicWriter's own documentation warns about — so
	// before this, `.sh.level=1` inside a function two deep went nowhere and
	// `${.sh.fun}` went on naming the function that wrote it. What the
	// selection does, what an out-of-range level does, and how long it lasts
	// are all measured; see interp/callstack.go, which holds the rules.
	r.SetDynamic(".sh.fun", func(rr *interp.Runner) string {
		// Empty at the top level, and empty rather than absent: measured,
		// `${.sh.fun}` there writes nothing at status 0. A sourced file is
		// transparent — `function f { . ./inc; }` reads `f` from inside the
		// file — which is what counting only function frames gives.
		_, name := rr.SelectedCallFrame()
		return name
	})
	r.SetDynamicWriter(".sh.fun", func(rr *interp.Runner, value string) {
		// A plain string for as long as the frame lasts: measured,
		// `function f { .sh.fun=zzz; }` reads `zzz` inside `f` and the top
		// level is empty again afterwards.
		rr.NameSelectedCallFrame(value)
	})
	r.SetDynamic(".sh.level", func(rr *interp.Runner) string {
		// `0` at the top level rather than nothing, which is the half that
		// makes the name distinguishable from a shell that does not have it
		// — both were empty here before (#3033). A `name()` function counts
		// the same as a `function` one, measured.
		level, _ := rr.SelectedCallFrame()
		return strconv.Itoa(level)
	})
	r.SetDynamicWriter(".sh.level", func(rr *interp.Runner, value string) {
		rr.SelectCallFrame(value)
	})
	// And it is not there at all until the first call has been made, which
	// the depth cannot say: the `0` above is what a read answers *after* one
	// has returned, and before one this shell has no such parameter.
	// Measured 2026-09-18, a script file under `env -i PATH=/usr/bin:/bin
	// LC_ALL=C` with standard input from /dev/null:
	//
	//	echo "[${.sh.level}] [${.sh.level-U}] [${.sh.level+S}]"
	//	    at the top of the script   []  [U]  []
	//	function g { :; }; g; the same line
	//	                           [0] [0]  [S]
	//
	// so the discriminating pair says unset and not empty, and the second
	// row says the `0` really is a *starting* value rather than an
	// off-by-one — after a call returns, both shells say 0. The parameter is
	// written on the way out of the first call and was never written before
	// it (#3310).
	//
	// A sourced file is the one shape this reading does not cover, and that
	// is deliberate: a dot script makes the parameter set there, at a number
	// that is a leftover rather than a depth. See #3117, where the
	// measurement is written down and the divergence is taken on purpose.
	r.SetDynamicPresence(".sh.level", func(rr *interp.Runner) bool {
		return rr.HasEnteredAFunction()
	})
	r.SetDynamic("RANDOM", func(rr *interp.Runner) string { return rr.Randoms() })
	// And an assignment seeds it, which is what makes a script that uses
	// `RANDOM` reproducible: measured 2026-09-14, `RANDOM=42` twice in one
	// shell gives the same pair of numbers both times here, in ksh93u+ and
	// in zsh 5.9.2. Without the writer the assignment was heard and stored
	// for the producer to find, and the producer had no state to find it
	// with (#2827).
	r.SetDynamicWriter("RANDOM", func(rr *interp.Runner, value string) { rr.SeedRandoms(value) })
	// `typeset -i RANDOM=7000`, measured — where this shell answered
	// `RANDOM: not found` from a name it had just expanded a number for
	// (#2451). `LINENO` lists the same way here and does not in bash 5.3,
	// which is why the letters are per dialect rather than per parameter.
	r.SetDynamicDeclaration("RANDOM", interp.ProducedDeclaration{Integer: true})
	r.SetDynamicDeclaration("LINENO", interp.ProducedDeclaration{Integer: true})
	// Three decimal places, where the two other shells that have SECONDS
	// report whole seconds.
	r.SetDynamic("SECONDS", func(rr *interp.Runner) string {
		return strconv.FormatFloat(rr.SecondsFrom(), 'f', 3, 64)
	})
	// Deliberately no declaration for it, and the reason is a listing this
	// engine cannot yet write rather than an oversight: ksh93 lists it
	// `typeset -F 3 SECONDS=0.001`, with the places as a word of their own,
	// and no listing form here writes that number (#1461). Registering the
	// bare `-F` would put out `typeset -F SECONDS=0.001` — closer than the
	// `not found` it says today and still not what the shell writes, and a
	// corpus row cannot tell "closer" from "right". So the row stays wrong
	// in the way it already was until the places can be written (#2451).
	r.Unregister("local")
	if dot, ok := r.Builtin("."); ok {
		r.Register("source", dot)
	}
	// `integer` is `typeset` with the type already decided, and it is one of
	// the two shells that has the word: zsh's own `add-zsh-hook` declares
	// integers before it does anything else — run it under a shell without
	// the word and it fails there — which is why a shell claiming to be
	// either of them needs it. Registered rather than built here, so that both
	// dialects get the *same* declaration — see interp/integerbuiltin.go.
	r.Register("integer", interp.IntegerBuiltin())
	// And the assignment rule follows the name the way it follows `declare`:
	// `integer n=5+2` is a declaration's operand and not a word to split.
	r.SetDeclaring("integer")
	// ksh93's own spellings of "what would this run" and "write this out",
	// both pervasive in real ksh scripts. See whence.go and print.go.
	registerWhence(r)
	registerPrint(r)
	// `functions` is `typeset -f` under a second name — registered rather
	// than written here, so the two words reach one listing and cannot come
	// to disagree about the header, the layout, or the status a name nobody
	// defined leaves behind. See interp/functionsbuiltin.go.
	r.Register("functions", interp.FunctionsBuiltin())
	// `newgrp` is `exec newgrp` under a builtin's name, which is what lets it
	// be a *special* builtin here. See newgrp.go.
	registerNewgrp(r)
	// And `nameref` is `typeset -n` under a second word, on the same terms
	// and for the same reason. See interp/namerefbuiltin.go.
	r.Register("nameref", interp.NamerefBuiltin())
	// The two marks a `-f` line puts on a function: `-u` reads the body from
	// `$FPATH` at the first call, and `-t` traces it. See fpath.go.
	registerFPath(r)
	// The assignment rule follows this name too: `nameref r=v` is a
	// declaration's operand and not a word to split.
	r.SetDeclaring("nameref")
	// How this shell arranges a function it says back, stated rather than
	// left to the zero value — see FunctionLayout. The same layout for a
	// function written into the environment, because this shell does not
	// put functions there at all and a second arrangement would be a claim
	// about nothing.
	r.SetFunctionLayout(FunctionLayout(), FunctionLayout())
}

// aliasCall is a call to `alias` read the way this dialect has to route it.
type aliasCall struct {
	// letters are every option letter of the leading option words, in the
	// order they were written.
	letters string
	// operands are the words past those, with the `--` that ended them gone.
	operands []string
	// untracked is the same call with every `t` taken out of those words,
	// which is what a refusal is raised from — see the use in Extend.
	untracked []string
	// tracked is whether `t` was among the letters at all. Only then is any
	// of this the cache's business.
	tracked bool
}

// readTrackedAliasCall splits a call to `alias` at the end of its options.
//
// `hash` is `alias -t --` in this dialect and `hash -r` is `alias -t -- -r`,
// so the operands reaching the cache are everything past the separator —
// including a word that looks like an option, which is why the separator goes
// rather than being handed on. The scan ends at the first word that is not an
// option, the way option parsing does everywhere: a `--` after an operand is
// an operand.
func readTrackedAliasCall(args []string) aliasCall {
	call := aliasCall{untracked: make([]string, 0, len(args))}
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			call.untracked = append(call.untracked, a)
			i++
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			break
		}
		call.letters += a[1:]
		if rest := strings.ReplaceAll(a, "t", ""); rest != "-" {
			call.untracked = append(call.untracked, rest)
		}
	}
	call.operands = args[i:]
	call.untracked = append(call.untracked, args[i:]...)
	call.tracked = strings.ContainsRune(call.letters, 't')
	return call
}

// foreignAliasLetters is the letters of a word that `alias` has not got here.
//
// The set it does have is `ptx`, which is what its own usage line says. `t`
// is in it even though the substrate's `alias` does not implement the letter:
// the reroute above is this dialect's implementation of it, so a call naming
// it is well formed and one naming anything else is not.
func foreignAliasLetters(letters string) string {
	foreign := ""
	for _, c := range letters {
		if !strings.ContainsRune("ptx", c) {
			foreign += string(c)
		}
	}
	return foreign
}
