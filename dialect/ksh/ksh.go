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
	d.AliasesExpandUnlessTold = true
	d.ExpandAliasesInProgramText = syntax.RouteOnEveryRoute
	// And a body's newlines are lines of the program: `$LINENO` after a
	// two-line body reads one more than the physical line.
	d.AliasBodyCountsLines = true
	// A subscript written at command position runs to its matching `]`:
	// `m[foo bar]=v` is the element keyed `foo bar`, read back here as
	// `typeset -A m=(['foo bar']=v)`. Measured 2026-09-12 on 93u+ beside
	// bash, against zsh 5.9.2, which refuses the text. See
	// [syntax.Dialect.SubscriptSpansSeparators].
	d.SubscriptSpansSeparators = true
	// ksh93 has neither `local` nor `declare`, so `local a=(x)` is the same
	// syntax error there that `echo a=(x)` is — the rule follows the name
	// into the shell that has it.
	d.DeclarationUtilities = map[string]bool{
		"typeset": true, "export": true, "readonly": true,
	}
	// A `!` with no pipeline after it is a pipeline of its own here, and this
	// shell is the union of the other two answers: it reaches a statement
	// terminator and a `&` the way bash does, and every closer and an and-or
	// operator the way zsh does. Measured 2026-09-12 over all ten positions
	// (#948).
	d.BareNegationReach = syntax.BareNegationAtEitherPlace
	// And a second `!` inverts the first, as in bash: `! ! true` answers 0.
	d.RepeatedNegationToggles = true
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
	// `${ cmd;}`, a command substitution that runs in the current shell
	// so that what it assigns survives. The space after the brace is the
	// whole of the grammar: `${x}` is a parameter and `${ x}` is not.
	d.CurrentShellSubstitution = true
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
	// A double quote inside an arithmetic expression is stepped over
	// wherever a token may begin. Measured 2026-09-10 on ksh93u+: with
	// `n=5`, `$(( "1" + 1 ))` is 2, `$(( "n" + 1 ))` is 6 and
	// `$(( 1 + "2" ))` is 3, while `$(( 1"0" ))` is an arithmetic syntax
	// error — the quote ends the number rather than vanishing from the
	// text, which is the line between this reading and bash's (#1223).
	d.ArithDoubleQuote = syntax.ArithDoubleQuoteSkipped
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
	s.DollarDashLetterOrder = "icaefhmnstuvxBCEHTl"
	s.ArithIntegerOperatorRefusesFloat = interp.Yes
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
	s.UnaliasAllRefusesOperands = interp.No
	s.AliasQuoting = interp.ListingQuoteWhenNeededDollar
	s.TrapQuoting = interp.ListingQuoteWhenNeededDollar
	// `typeset -p` writes `typeset -x -r n=5` — separate flags — and a name
	// with no attributes as a bare `v=1`; a missing name is passed over in
	// silence, status 0, which is measured rather than a shortcut.
	s.DeclareListing = interp.DeclareListingBareAssignments
	s.DeclareValueQuoting = interp.ListingQuoteWhenNeededDollar
	// And a `#` in a listed value is left bare unless a name stands in front
	// of the first one: `16#ff`, `99#zz` and `1a#b` all list unquoted here
	// and `a#b` and `#lead` do not (#1271).
	s.ListedHashIsBareAfterANonName = interp.Yes
	// And not the weaker position rule bash has: `a#b` is quoted here,
	// which is what says the leading text is judged and not the offset.
	s.ListedHashIsBareUnlessItOpensTheValue = interp.No
	s.ListingControlEscape = interp.ControlEscapeHex
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
	s.SymbolicMaskTakesTheStickyLetter = interp.Yes
	// Every dash word is an option here, digits and all: `shift -1` and
	// `shift -0` are both refused as options this shell does not have, which
	// is why a negative count is only reachable after the marker.
	s.ShiftOptionWords = interp.ShiftOptionWordsAny
	s.ShiftDoubleDashEndsOptions = interp.Yes
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
	// The listing runs the other way here: descending by signal number,
	// which puts EXIT last where the other six put it first.
	s.TrapListingOrder = interp.TrapListingHighestFirst
	s.TrapBodyLine = interp.TrapBodyLineOffsetFromWhereItFired
	// And the same for the two conditions that fire at a command: this
	// shell counts every trap body from where it fired, so the second
	// question has the same answer as the first.
	s.CommandTrapBodyLine = interp.TrapBodyLineOffsetFromWhereItFired
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
	// And an unset name reached that way is a refusal rather than a zero:
	// `x=abc; $((x+1))` is `abc: parameter not set` at status 1 and the
	// script stops, with nounset off. A name written in the expression
	// itself is still zero — `$((nosuch+1))` is 1 here as everywhere.
	s.ArithRecursedNameMustBeSet = interp.Yes
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
	s.AssignmentPrefixPersistsAfterAFunction = interp.Yes
	s.PrefixToAFunctionIsExported = interp.No
	s.PrefixRefusalFatality = interp.PrefixRefusalFatalOnASpecialBuiltinOrFunction
	s.PrefixRefusalCostsTheCommand = interp.Yes
	// The attribute cannot come off, though, which is where this shell parts
	// from zsh and why the two are separate questions: `typeset +r x` on a
	// frozen name is `typeset: x: is read only` and ends the script.
	s.ReadonlyAttributeCanBeRemoved = interp.No
	s.DeclaredNameWithoutValueIsEmpty = interp.No
	// A keyword-defined function's `typeset -x` is local like any other
	// declaration; the POSIX-style function that leaks it has no scope to
	// leak out of, which TypesetLocalNeedsKeywordFunction already answers.
	s.ExportLetterDeclaresAGlobal = interp.No
	// A valueless `typeset` of a standing name is silent here.
	s.ValuelessDeclarationOfAHeldNameListsIt = interp.No
	// And a plain word over a name holding an array is taken.
	s.ScalarOverACompoundIsAnInconsistentType = interp.No
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
	// Measured 2026-09-12: `x=1; unset -n x` leaves `x` gone at 0, which is
	// what `unset x` does — the letter changes nothing for a name that is not
	// a reference, where bash removes nothing at all (#932).
	s.UnsetReferenceLetterRemovesANonReference = interp.Yes
	s.ReadZeroTimeout = interp.ReadZeroTimeoutTakesWhatIsWaiting
	s.ReadPartialCountSucceeds = interp.Yes
	s.ReadExactCountKeepsPartial = interp.No
	s.ReadTimeoutKeepsWhatArrived = interp.No
	s.ReadTimeoutBoundsReadability = interp.No
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
	s.UnterminatedBracket = interp.BracketLiteral
	s.UnknownCharacterClass = interp.UnknownClassEmptiesTheBracket
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
	// And `${x?word}` is one of those errors here rather than a request to
	// stop: `.` reports 1 for it too and the sourcing file carries on, which
	// is the half of this zsh answers the other way.
	s.ParamErrorIsAnExitRequest = interp.No
	s.DotPassesArguments = interp.Yes
	// A directory operand is an error, and a fatal one through
	// DotMissingFileFatal: measured, `. ./` is `.: ./: cannot open [Is a
	// directory]` and the script ends there. No wording of its own — that
	// is the one sentence ksh93 uses for every `.` failure, with the reason
	// filling the bracket, so DotCannotOpen already carries it.
	s.DotDirectoryOperandIsAnError = interp.Yes
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
	// unanswered QuitResetRestoresTheDefault: that axis is what a reset does
	// to the *ignore* above, and this shell has no ignore to take away — an
	// untrapped QUIT kills it whether or not `trap - QUIT` has been run, so
	// both readings run every script identically and there is nothing to
	// measure a preference from.
	s.HangupIsAnOrderlyExit = interp.No
	s.ExitInTrapReportsEarlierStatus = interp.Yes
	s.KillListAcceptsName = interp.Yes
	// And a signal written onto the option with no space: `kill -n9` and
	// `kill -sKILL` both send. Measured 2026-09-12. This shell is looser
	// still — it takes `kill -s9` too, which the axis records and does not
	// follow (#2227).
	s.KillReadsASignalJoinedToItsOption = interp.Yes
	s.SIGPrefixAccepted = interp.Yes
	s.RedirectsUseEveryTarget = interp.No
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
	s.PidListingFinishesWithAJob = interp.Yes
	s.PrintfTimeConversion = interp.Yes
	s.PrintfTimeOperandIsADateString = interp.Yes
	s.PrintfQuote = interp.PrintfQuoteSingle
	// The same `\c` as the printf format, and the arithmetic is bit 6
	// toggled rather than bash's five-bit mask: `$'\c1'` is `q`, not 0x11.
	s.DollarSingleBackslashC = interp.DollarSingleControlToggled
	s.DollarSingleUnknownEscape = interp.DollarSingleUnknownDropsBackslash
	s.DollarSingleNulTruncates = interp.Yes
	// `\x` here takes every hexadecimal digit that follows and a run past
	// two is a code point, so `$'\x00b'` is the one byte 0x0b where the
	// other shells read `\x00` and truncate. A run with no digit at all is
	// a zero byte, which this shell's truncation then makes into nothing.
	s.DollarSingleHexReadsEveryDigit = interp.Yes
	s.DollarSingleDigitlessEscapeIsAZeroByte = interp.Yes
	s.GetoptsAssignmentRestartsWord = interp.Yes
	s.GetoptsClearsOptarg = interp.No
	// Reached through `typeset` in a function defined with the `function`
	// word, this shell having no `local`: the caller's position inside a
	// clustered word comes back with the number.
	s.GetoptsLocalOptindRestoresTheCursor = interp.Yes
	// Measured 2026-09-12: `OLDPWD=/nonexistent ksh -c 'echo $OLDPWD'` answers
	// the path it was given, and `cd -` then names it — this shell judges the
	// value when something tries to use it and not before.
	s.InheritedOldpwd = interp.InheritedOldpwdTaken
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
	// ksh93 has the move operator too and reads it as one operation: the
	// descriptor changes number. So both halves are the command's redirection
	// and both are taken back — `exec 5<f; true 6<&5-` leaves 5 open, where
	// bash leaves it closed — and the number the source gives up is the one
	// `{v}<&$w-` receives, so the name answers with `$w`'s own number.
	s.FdMove = interp.FdMoveRelocates
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
	// A count stops it judging past the first operand, though:
	// `read -n 3 a 1bad` is quiet here and `read a 1bad` is not, measured
	// with both count letters. The first operand is still judged —
	// `read -N 3 1bad` is refused — so it is the names after it that a
	// count releases.
	s.ReadCountJudgesTheNamesAfterTheFirst = interp.No
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
	// And reading one says nothing either: measured 2026-09-12, `typeset -A
	// m; m[k]=v; w=; ${m[$w]}` is the empty string at status 0 with no
	// diagnostic, where bash names the table (#1972).
	s.EmptyAssociativeKeyIsReportedWhenRead = interp.No
	// The bash column's answer for whether an empty positional list is a set
	// parameter: measured 2026-09-12, `set --; "${@-word}"` is `word` here
	// and `${@=abc}` is `${@=abc}: bad substitution` because the operator
	// fires at all (#1941).
	s.PositionalListWithNoneIsSet = interp.No
	// dash's and zsh's order for a frozen name in a prefix: whatever the
	// command was going to do first happens first, and only bash checks the
	// name ahead of it (#1943).
	s.PrefixToAFrozenNameIsCheckedFirst = interp.No
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
	s.BadSubscriptToUnsetFatal = interp.No
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
	// `typeset: a+: invalid variable name` — the append operator is not a
	// declaration operand here.
	s.DeclarationTakesAnAppendOperand = interp.No
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.Yes
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
	s.UnknownConditionOptionIsAStatus = interp.No
	s.ReturnOutsideAFunctionIsRefused = interp.No
	// And a `break` with no loop around it is ignored, silently: measured on
	// 93u+ 2012-08-01, `echo t; break; echo after` prints both and ends at
	// 0. No wording goes with it, which is dash's answer too.
	s.LoopControlOutsideALoopIsFatal = interp.No
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

	// The letters `typeset` reads here. `-g` it simply does not have, and
	// there is no `local` (see Register), so LocalOptions stays empty.
	//
	// `-f` is here now (#1494). The real shell prints a function back
	// *verbatim* — it keeps the source text and this engine keeps a tree —
	// so the listing is a layout that reproduces that text for a definition
	// written the way anybody writes one; see FunctionLayout for what that
	// does and does not promise. What the listing must get right is not the
	// spacing: it is the `function` keyword, because `typeset` declares a
	// local in a keyword body here and the global in the other, so a
	// listing that dropped the word would hand back a program whose
	// variables leak.
	s.DeclareOptions = "aAfilprux"
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
	// engine implements `-f` and `-p`, which are the two that reach the
	// listing. Measured 2026-09-12, ksh93u+ also takes `-t` and `-u` there
	// — tracing and autoloading from FPATH, neither of which this shell
	// does — and answers the usage line for `-F`, `-m` and `-M`, which is
	// what an unimplemented letter gets here too. See #2192 for the two
	// that are missing.
	s.FunctionsOptions = "fp"
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

	// The coprocess `|&` starts is half taken back when it ends: the end this
	// shell writes goes, so `print -p` is `no query process` at 1; the end it
	// reads stays, so whatever the coprocess left in the pipe is still there
	// for a `read -p` to answer with. What happens when that read finds
	// nothing left is not this axis — both shells with the letters forget the
	// whole coprocess there, reaped or not. Measured 2026-09-12 (#2411).
	s.ReapedCoprocessEnds = interp.CoprocWriteEndGoesWithTheCoprocess

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
	d := interp.Diagnostics{
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
		TypeFunction: "%[1]s is a function",
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
		BadArraySubscript:         "%[1]s: subscript out of range",
		CannotConvertTableToArray: "%[2]s: cannot change associative array %[1]s to index array",
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
		// A duplication whose source is not open takes the same shape as an
		// open that failed here — `cat <&10` is `10: cannot open [Bad file
		// descriptor]`, measured 2026-09-12 — but it cannot share the field:
		// CannotOpen is worded reason-first in one of the other dialects
		// (#734).
		DuplicationSourceNotOpen: "%[1]s: cannot open [%[2]s]",
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
		// The same quoting inside a condition as outside it, which is where
		// this shell parts from bash: `[[ "a b" == "a b" ]]` traces
		// `[[ 'a b' == 'a b' ]]` here and `[[ a b == a b ]]` there.
		TraceConditionQuoting: interp.QuoteDollar,
		// `((n))` and `(( n + 1 ))`: the text between the parentheses, with
		// nothing added. bash and zsh add a space on each side.
		TraceArithCommand: interp.TraceArithTight,
		TraceArithForPart: interp.TraceArithTight,
		ScriptLocation:    interp.LocationLineWord,
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
		// It said `parameter not set` until #1629, which is the sentence a
		// *name-shaped* value earns — and it earns it by being looked up and
		// found unset, not by failing to be a number. Now that the lookup
		// really happens (Semantics.ArithRecursedNameMustBeSet) the stand-in
		// is not merely unnecessary: it was answering the wrong sentence for
		// every value that is no name at all.
		InvalidNumber: "%[1]s: arithmetic syntax error",
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
			// restricted, -s sorting the positional parameters, and -B -G
			// -H, its brace expansion, globstar and history-expansion
			// switches. Measured
			// 2026-09-05 by asking ksh93 for every letter of the alphabet
			// in both cases and both signs.
			// `-A` has left this list: it assigns an array and is
			// implemented, in Semantics.SetArrayLetter.
			// `-t` is not here: this shell really does stop after one
			// command, and the letter is the only spelling it has for the
			// option — see Semantics.SetHasTheTLetter.
			"set": "bkprsGH",
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
			// typeset's letters this engine does not hold: the floats'
			// -F, namerefs, padding and alignment, mappings and the rest of
			// its usage line.
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
			// `$FPATH`. So `typeset -fu nm` reads as a listing of a
			// function that is not there, silent at 1, where ksh93 marks
			// the name and is 0. That half is #2192; the seam it needs is
			// Semantics.FunctionLettersThatMarkUndefined, and what is
			// missing is an FPATH search for this dialect to put behind it.
			"typeset": "-bFhmnstCEHLMRSTXZ",
			// `functions` is `typeset -f` under a second name, so the
			// letters it is missing are read off its own set: `-t` traces a
			// function and `-u` marks one to be read from `$FPATH`, both of
			// which ksh93 takes there and this shell does not do, and `-M`
			// is a character mapping rather than zsh's math facility. `-F`
			// and `-m` are deliberately absent: measured, `functions -F`
			// and `functions -m` are the usage line on ksh93u+, so that
			// shell has not got them either and "unknown" is the truth.
			// See #2192.
			"functions": "-tuM",
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
			"integer": "-bFhmnstCEHLMRSTXZ",
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
		// And the names-only listing keeps the same distinction with
		// punctuation instead of a word: measured, `f() { :; }; function g
		// { :; }; typeset +f` writes `f()` and then `g`.
		FunctionNameListing:        "%[1]s()",
		FunctionNameListingKeyword: "%[1]s",
		BuiltinComplaintName: map[string]string{
			"type": "whence", "integer": "typeset", "functions": "typeset",
		},
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
		KillIllegalOption:         "kill: -%[1]s: unknown option\n" + kshKillUsage,
		KillMissingSignalArgument: "kill: %[1]s: signame argument expected\n" + kshKillUsage,
		KillUsageStatus:           2,
		KillBadOptionStatus:       2,
		KillUsageUnprefixed:       true,
		KillTargetUnprefixed:      true,
		// `[ -Q x -a -n x ]` is `[: x: unknown operator` here: the dash word
		// is an operand like any other, and the complaint names the second of
		// the two the primary is left with (#1290).
		TestUnknownLongOperator: interp.TestUnknownOperatorLeavesAnOperand,
		TestUnaryExpected:       "%[2]s: %[1]s: unknown operator",
		TestBinaryExpected:      "%[2]s: %[1]s: unknown operator",
		TestIntegerExpected:     "%[2]s: %[1]s: integer expected",
		TestTooManyArguments:    "%[2]s: too many arguments",
		TestOperandExpected:     "%[2]s: argument expected",
		TestMissingBracket:      "[: ']' missing",
		OptionListingHeader:     "Current option settings",
		OptionListingWidth:      25,
		PlusOListsActive:        true,
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
			for i, a := range args {
				if a == "--" || !strings.HasPrefix(a, "-") || a == "-" {
					break
				}
				if !strings.ContainsRune(a[1:], 't') {
					continue
				}
				rest := append(append([]string{}, args[:i]...), args[i+1:]...)
				rest = trimTrackedSeparator(rest, strings.Replace(a, "t", "", 1))
				hash, ok := rr.Builtin("hash")
				if !ok {
					break
				}
				return hash(rr, ctx, rest)
			}
			return alias(rr, ctx, args)
		})
	}
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
	// How this shell arranges a function it says back, stated rather than
	// left to the zero value — see FunctionLayout. The same layout for a
	// function written into the environment, because this shell does not
	// put functions there at all and a second arrangement would be a claim
	// about nothing.
	r.SetFunctionLayout(FunctionLayout(), FunctionLayout())
}

// trimTrackedSeparator puts back the option word `-t` was taken out of, and
// drops the `--` that `hash` has no letters to need.
//
// `hash` is `alias -t --` and `hash -r` is `alias -t -- -r`, so the operands
// reaching the cache are everything after the separator — including a word
// that looks like an option, which is why the separator goes rather than
// being handed on.
func trimTrackedSeparator(args []string, remainder string) []string {
	if remainder != "-" && remainder != "+" {
		args = append([]string{remainder}, args...)
	}
	for i, a := range args {
		if a == "--" {
			return args[i+1:]
		}
	}
	return args
}
