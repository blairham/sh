// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package ash answers the substrate's questions the way BusyBox ash does.
//
// # What was measured, and what checks it
//
// Every answer below was measured by running BusyBox v1.37.0's `ash` — the
// `/bin/ash` of the `alpine:3` image — over the whole oracle corpus and over
// hand-written probes, on 2026-09-12. No BusyBox source was read; see
// CLEANROOM.md, and docs/spec/ash.md for the method and the numbers.
//
// For its first day this was the one dialect in the tree **no instrument
// graded**, because the oracle located its shells with exec.LookPath and
// there is no ash binary on a macOS machine. That is over: the panel reaches
// a member by a *route* now, and ash's is a container of a digest-pinned
// alpine image, so it has a column in the golden record, a row in `make
// conformance-dialects` and a target in `make axis-sweep` (#2263).
//
// The day it did not have them cost exactly what it looked like it would.
// An axis was added to interp.Semantics with no answer here, `read -t`
// started being refused by a shell that used to take it, and `go test ./...`
// stayed green throughout (#2272). The column is what says so now.
//
// One thing `make check` does notice, since #2340: an axis this dialect has
// no value for *at all*. That is absence rather than drift, it costs no shell
// processes, and it is the shape #2272 shipped — so `make axis-coverage`
// asks it of every dialect and the same check fails in `go test ./...`.
//
// Read a comment that cites a measurement as evidence; read the absence of
// one as an unanswered question rather than as agreement with dash.
package ash

import (
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what ash parses.
//
// It starts from POSIX like every preset — not from dash, whose file this one
// must not be read as a delta against. The two shells are both NetBSD ash
// descendants, and the issue that asked for this dialect predicted the
// grammars would nearly coincide. Measured, they do not: this shell takes
// `$'…'`, `[[ ]]`, `<(…)`, `function`, `${v:1:2}`, `${v/a/b}`, `**`,
// `base#digits`, `++`, `&>` and `>|`, every one of which dash refuses.
func Dialect() syntax.Dialect {
	d := syntax.POSIX()
	// Nine constructs beyond POSIX, each run and watched rather than read
	// about. `echo $'a\tb'` writes a tab; `[[ a = a ]]` succeeds where dash
	// answers `[[: not found`; `cat <(echo hi)` writes hi; `function f {
	// …; }` and `function f() { …; }` both define; `${v:1:3}` of `abcdef` is
	// `bcd`; `${v/b/X}` of `abc` is `aXc`.
	d.DollarSingleQuote = true
	// `[[` is here, but as `test` with a closing word rather than as the
	// conditional every other shell with the spelling has: `type '[['` is
	// `[[ is a shell builtin`, `[[ abc == "a*" ]]` is 0 because the quotes
	// are gone before the builtin sees the pattern, and `s="two words"; [[
	// $s == "two words" ]]` is `words: unknown operand` at 2. The keyword
	// reading was what this preset held until #3409.
	d.DoubleBracketIsACommand = true
	d.ProcessSubstitution = true
	d.FunctionKeyword = true
	d.FunctionKeywordParens = true
	d.ParamSubstring = true
	d.ParamSubstitution = true
	// `$((2**3))` is 8 and `$((10#08))` is 8, where dash refuses both; `i=1;
	// echo $((i++)) $i` is `1 2`; `$((1,2))` is 2.
	d.ArithExponent = true
	d.ArithExplicitBase = true
	d.ArithIncDec = true
	d.ArithComma = true
	// The two redirection spellings: `echo x &>/dev/null` is silent, and
	// `set -C; echo x >| f` overwrites.
	d.AmpersandRedirect = true
	d.ClobberOverrideMarker = true
	// A here-document body line that joined *before* any text of it was
	// written still reaches the delimiter, as in dash; one that joined after
	// text does not (#2430).
	d.HeredocDelimiterAcrossAContinuation = syntax.HeredocDelimiterAfterALeadingContinuation
	// There is no `time` keyword here, and the thing that made it look like
	// one is a program. `time echo hi` prints a three-row summary because
	// `/usr/bin/time` is a BusyBox applet on this image, so that line runs an
	// ordinary command — and `type time` says so, answering `time is
	// /usr/bin/time` rather than naming a keyword. The discriminator is a
	// compound command, which only a keyword can take: measured 2026-09-18 in
	// the pinned image, `time for i in 1; do :; done` is `syntax error:
	// unexpected "do"` and `time ( : )` is `syntax error: unexpected word
	// (expecting ")")`, both at 2. This preset had the flag set from the
	// start, inherited rather than measured (#2982).
	d.TimeKeyword = false
	// A name followed by `(` is a function definition, whatever comes next:
	// `f(x) { :; }` is refused for the word rather than for the parenthesis,
	// which is what decides the token a malformed definition is blamed on.
	d.FuncDefAtParen = true
	// And a name with punctuation in it is a name: `a.b() { echo hi; }; a.b`
	// prints hi.
	d.FunctionNamePunctuation = true
	// A bare word is a name whatever is in it, and a word that was not
	// written bare is a definition that binds nothing: `a*b() { echo d; };
	// a*b` prints `d` here, and `'f'() { echo p; }; f` is `f: not found` at
	// 127 even though `f` is a name nobody could object to. So it is the
	// spelling and not the characters, which is the third answer to a
	// question the other six columns split three ways — see
	// syntax.Dialect.FunctionNameIsAnyBareWord for the measurements, and
	// note that both spellings of a definition answer alike here where the
	// panels for the two flags either side of that one differ (#2590).
	d.FunctionNameIsAnyBareWord = true
	// The word is carried to the definition rather than refused while
	// reading, which is what the run below has to say anything about: the
	// body is parsed like any other, `'h'() { if; }` being `syntax error:
	// unexpected ";"` at status 2.
	d.FunctionNameCheckedWhenTheDefinitionRuns = true
	// One operator may stand where a case pattern belongs: `case a in ;)
	// echo x;; *) echo def;; esac` reaches the default arm rather than
	// failing to parse.
	d.CasePatternAcceptsOperator = true
	// The loop-variable position is read as a name whatever stands there, so
	// `for ; do :; done` and `for "i" in a; …` are both `bad for loop
	// variable`.
	d.ForNonWordIsANameError = true
	// Aliases expand with nobody asking, as they do in dash: `alias foo=echo`
	// on one line and `foo` on the next expands in a file and on standard
	// input, and there is no option to turn on.
	d.AliasesExpandUnlessTold = true
	// And they expand on every route, `-c` included, which is dash's answer
	// and not zsh's. Measured 2026-09-13 in the pinned image, BusyBox
	// v1.37.0: `ash -c $'alias foo=echo\nfoo hi'` prints `hi`.
	//
	// This value was `RouteFromScriptFile | RouteOnStandardInput` until
	// #2338, and the probe that put it there could not have found anything
	// else. It was `ash -c 'alias foo=echo; foo'` — **one line** — and an
	// alias never expands on the line that defines it, so every shell in the
	// panel answers `not found`, dash included, and dash expands on every
	// route. The one-liner was measuring the same-line rule and reading it as
	// a route rule. Two lines, or nothing is being measured.
	d.ExpandAliasesInProgramText = syntax.RouteOnEveryRoute
	// And a body's newlines are input lines, the answer three of the four
	// existing dialects give.
	d.AliasBodyCountsLines = true
	// A backslash an alias body ends with reaches the newline after the
	// alias word, where it is an ordinary line continuation and joins the
	// next line to the word. zsh and bash 3.2 are the columns that do not.
	// See syntax.Dialect.AliasBodyBackslashJoinsTheNextLine (#2710).
	d.AliasBodyBackslashJoinsTheNextLine = true
	// `exec 10>f` names descriptor ten, as it does in bash. Measured
	// 2026-09-17 in the pinned image, BusyBox v1.37.0: `exec 10>f10` is 0,
	// `echo hi >&10` puts three bytes in the file, and `exec 100>f100` is 0
	// too, so the width is not two. A bare `10` on a line of its own is
	// still `10: not found` at 127, which is the control that says the
	// digits are a descriptor only in front of a redirection operator
	// (#3238). The target side is the other question and splits the panel
	// the other way — see Semantics.MultiDigitDuplicationTargetIsAnError,
	// already measured for this column.
	d.MultiDigitFdNumber = true
	// Deliberately absent, each refused when run: arrays (`a=(1 2 3)` is
	// `unexpected "("`, `${a[0]}` is `bad substitution`), the C-style `for`,
	// `select`, `<<<`, `(( ))`, `$[…]`, floating-point arithmetic, `${v^^}`,
	// `${!x}`, `${v@Q}`, `|&`, `;;&`, `$(<f)`, `{fd}>f`, `@(…)`, brace
	// expansion and `+=` — `v+=b` is a command this shell cannot find.
	return d
}

// Semantics is what ash means where the shells conflict.
//
// The measured shape is the finding the issue behind this package asked for,
// and it is not the one the issue predicted: ash sides with bash rather than
// with dash on `[^x]`, on `echo` needing `-e`, on a valueless `local`, on
// `**` and on `base#digits`. It is dash's sibling by ancestry and not by
// behavior.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	// As dash, and measured on BusyBox 1.37.0 in the pinned image the same
	// day: `echo "[$( (( 1+1 )) )]"` is `1+1: not found` and `[]` at 0 — a
	// nested subshell running a command named `1+1`, since this shell has no
	// arithmetic command either (#3364).
	s.ArithmeticOnlyBodyIsAnArithmeticExpansion = interp.No
	// unanswered WritingSubstitutionIsWaitedForAtTheCommand: this shell has
	// no process substitution, so there is no `>(cmd)` body for a command to
	// wait for or not. `echo >(:)` is the two characters as written (#2197).

	// ---- expansion and words ----

	// `export a+=2` is `a+: bad variable name`, so the append operator is not
	// an operand this shell's declarations take.
	// BusyBox does not look before it leaps: a file the kernel refused is
	// read as a shell script whatever is in it. Measured 2026-09-13 in the
	// pinned alpine image — a Mach-O header, a file beginning with a NUL and
	// a file with a NUL mid-line all run here, where the other six columns
	// answer 126.
	s.BinaryContentIsNotRunAsAScript = interp.No

	s.DeclarationTakesAnAppendOperand = interp.No
	// A prefix to a function, both halves with its sibling: visible and
	// exported for the length of the call, gone afterwards. Measured
	// 2026-09-12 in the pinned alpine image (#2407).
	// And `command` in front of a special builtin takes its persistence away,
	// as it does in dash. Measured 2026-09-18 in the digest-pinned image:
	// `s=base; s=C command :` leaves `base` where the bare `s=P :` leaves
	// `P` (#3448).
	s.CommandKeepsASpecialBuiltinsPrefix = interp.No
	s.AssignmentPrefixPersistsAfterAFunction = interp.No
	s.PrefixToAFunctionIsExported = interp.Yes
	// And not at a builtin, measured 2026-09-16 in the pinned alpine image:
	// `v=1; v=9 eval 'env | grep "^v="'` shows the child nothing, and the
	// attribute is left where it was. Its sibling's two lines, for the same
	// reason — `readonly` and `export` are the only declaration words and
	// their prefix persists here already (#3437).
	// unanswered PrefixInAWholeTableListing: as in dash — there is no
	// `typeset` here, `export -p` is the only whole-table declaration
	// listing, and `export` is a special builtin whose prefix persists, so
	// there is no second table for the axis to tell apart from the first
	// (#3446).
	// The redirections are opened first, as in dash: measured 2026-09-18 in
	// the digest-pinned image, `w=$(echo S >&2) f > /nope/x` writes `can't
	// create /nope/x` alone (#3449).
	s.PrefixExpandedBeforeTheRedirections = interp.PrefixExpandedBeforeRedirectionsNever
	s.PrefixExportAtABuiltin = interp.PrefixExportAtABuiltinUnchanged
	s.DeclarationPromotesThePrefixEntry = interp.No
	// unanswered SubscriptedAssignmentPrefix, SubscriptedPrefixIsTakenBack:
	// a subscripted word is no assignment in this grammar, so `a[1]=v f` is
	// the *command* `a[1]=v` and the complaint is `a[1]=v: not found`. The
	// word never becomes an assignment prefix and neither axis is reached.
	// Measured 2026-09-16 on BusyBox ash 1.37.0 (#3433).
	// POSIX makes an unquoted `$@` behave as `$*` where nothing is split, and
	// this shell complies: `IFS=-; set -- x y z; v=${@}` is `x-y-z`.
	s.UnsplitAtListJoinsOnIFS = interp.Yes
	// An empty positional list is a set parameter: `set --; "${@-word}"` is
	// empty and `"${@+word}"` is `word`.
	// unanswered ConditionWholeArraySubscript: this applet has no `-v`
	// operator for a subscript to be read by. Measured 2026-09-18 in the
	// digest-pinned image, `test -v x` is `x: unknown operand` at 2, and
	// there is no `[[ … ]]` here either — so the axis is unreachable in this
	// column rather than unanswered.
	s.PositionalListWithNoneIsSet = interp.Yes
	// `${#@}` with three parameters is 5 — the width of `a b c` — rather than
	// the count.
	s.LengthOfSpecialIsCount = interp.No
	// And a substring of `$@` or `$*` is a substring of that same joined
	// string: `set -- one two three four; "${*:1:2}"` is `ne` here and
	// `one two` in bash, zsh and ksh93.
	s.SubstringOfPositionalsSlicesTheList = interp.No
	// The panel's dash column is the one with no multibyte decoder; this
	// shell has one and does not need a locale to use it. Measured with the
	// harness's fixed `LC_ALL=C`: `s=héllo; echo ${#s}` is 5 here and 6 in
	// dash.
	s.MultibyteEncodingIsHonored = interp.Yes
	// A value's backslash quotes what follows it rather than standing as a
	// character of the pattern: `v='a\*'; set -- $v` is `a\*`.
	s.ValueBackslashInAPattern = interp.ValueBackslashQuotesWhatFollows
	// `case x in [^a]) echo negates ;; *) echo plain ;; esac` reaches the
	// first arm. dash is the panel's sole holdout on reading the caret as a
	// negation, and this shell — its sibling — is not with it, so the "dash
	// alone" argument in interp/semantics.go survives ash rather than losing
	// to it. That prediction is the one #497 was filed on and is the one it
	// refuted.
	s.BracketCaretNegates = interp.Yes
	// An unterminated bracket in a pattern matches nothing rather than being
	// read as a literal: `case a in [a) echo one;; *) echo def;; esac`
	// reaches the default arm.
	s.UnterminatedBracket = interp.BracketNoMatch
	// And the same question where a `[:name:]`, a `[.x.]` or a `[=x=]`
	// inside it is what left it open: unmeasured — no BusyBox was reachable; this keeps the answer the shell already gave, which is dash's (#3379).
	s.UnterminatedBracketAfterASubExpression = interp.BracketNoMatch
	// But inside a bracket expression it escapes nothing at all: the
	// backslash is an ordinary member of the set and the character behind it
	// keeps whatever meaning it has there. `case 'a]c' in a[\]]c)` reaches
	// the arm in the other six columns and not here — `[\]` is the
	// one-character set `\` and the second `]` is a literal — while
	// `a[a\-z]c` matches `abc` and `a\c` and does **not** match `a-c`,
	// because the `-` behind the backslash is still the range operator and
	// the backslash is its left bound. A shell that merely refused the
	// pattern would answer no to all three, which is what makes this a
	// reading rather than a refusal.
	//
	// Measured 2026-09-16 in the pinned alpine image under `--init`, over 37
	// values of X in `a[\X]c` and both routes a bracket can be written on.
	// `bet\a` matches `beta` here as everywhere, which is the control: the
	// escape is spent outside a bracket in this shell too, so this is about
	// the bracket and not about patterns in general (#3271).
	s.BracketEscape = interp.BracketEscapeIsOnlyAMember
	// `[[ x =~ "" ]]` matches rather than being refused, which is ksh93's
	// answer among the shells that have the operator and not bash's or
	// zsh's. Measured 2026-09-16 in the pinned alpine image: status 0 here,
	// 2 in bash 5.3, bash-as-`sh` and bash 3.2, and 1 in zsh, with `[[ abc
	// =~ b ]]` at 0 and `[[ abc =~ x ]]` at 1 in all four as the controls
	// that say the operator works at all. This dialect has `[[ ]]` and took
	// the preset's `Yes` — the refusal — because nothing asked (#3248's
	// class).
	s.EmptyRegexOperandIsAnError = interp.No
	// unanswered RegexMatchSurvivesAFailedMatch,
	// RegexMatchOmitsGroupsThatDidNotMatch: there is no `=~` here — the
	// line above is the empty-operand question this shell cannot be asked
	// either — so no evaluation ever reaches a record.
	// A `<(cmd)` may stand as a condition's operand and is performed there,
	// as it is in bash. Measured 2026-09-16 in the pinned alpine image:
	// `[[ "<(echo x)" == <(echo x) ]]` is **1**, because the right side
	// became a `/dev/fd` name and the left is the text — the row that tells
	// "performed" from "read as a literal word", which the `-e` form cannot.
	// zsh refuses at 1 with a sentence of its own and ksh93 and dash refuse
	// while reading, so the preset's `No` was three shells' answer and not
	// this one's (#3248's class).
	s.ProcessSubstitutionInCondition = interp.Yes
	// `echo .*` lists `.` and `..` beside the hidden names, which is the
	// answer dash and ksh93 give and bash 5.3 does not. Recorded from the
	// corpus run inside the container rather than guessed at, since there
	// is no BusyBox on this machine — see `glob/dot-and-dotdot-in-a-listing`
	// (#2748).
	s.GlobListsDotAndDotDot = interp.Yes
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
	s.UnknownCharacterClass = interp.UnknownClassIsInert
	// The delimiters are read as a sub-expression and no body is ever an
	// element, which is this column alone and is neither of the two readings
	// the axis had while it was a boolean. Measured 2026-09-18 in BusyBox
	// v1.37.0 in the pinned image, `LC_ALL=C`: `[[.a.]]` matches nothing at
	// all — not `a`, which is what the four columns that read an element
	// answer, and not `a]`, which is what the column with no construct
	// answers — while `[[.a.]x]` matches `x`, so the `]` inside the
	// delimiters did not end the bracket. Every body is then the unknown
	// body, and the axis above answers it inert here exactly as it answers
	// an unknown `[:name:]`: `[a[.nosuch.]b]` matches a and b (#3379).
	//
	// Borrowing dash's value would have been wrong in a way nothing would
	// have caught, which is what #3368 cost the neighboring axes.
	s.CollatingElements = interp.CollatingElementsHoldNothing
	// `[[:]` takes the `]` as part of the name it is still looking for, so the bracket never ends and UnterminatedBracket decides: `${w#[[:]}` on `[:y` is `y` here, a literal `[` and then `[:]` matching the colon, where every other column matches nothing.
	// See interp.Semantics.UnterminatedCharacterClass (#1431).
	s.UnterminatedCharacterClass = interp.UnterminatedClassSwallowsTheClosingBracket
	// `$(( ))` with nothing in it is 0 at status 0, where dash wants a
	// primary and stops the script.
	s.EmptyArithExpressionIsAnError = interp.No

	// The lines of `eval`'s text continue the line the `eval` is written on,
	// and `$LINENO` moves with them — this shell with bash rather than with
	// dash, measured 2026-09-12 in the pinned alpine image with the whole
	// `eval` on one physical line, which is the only arrangement that tells
	// this from the physical reading (#2462).
	s.EvalTextContinuesTheCallersLines = interp.Yes

	// ---- invocation and options ----

	s.CommandNotFoundStatusIsNotFound = interp.Yes
	// A pathname operand is written back to `command -v` and `type` exactly as
	// it was typed, POSIX's "shall be written as absolute pathnames"
	// notwithstanding. Measured 2026-09-14 in the pinned alpine image:
	// `command -v ./bb/tool` is `./bb/tool` and `command -v bb/tool` is
	// `bb/tool`, with ksh93 the only column in the panel that joins.
	s.APathnameOperandIsReportedAbsolute = interp.No
	s.SetFTurnsOffGlobbing = interp.Yes
	// No braces to expand and no `-B` to turn them off: `set -B` is `illegal
	// option -B` and ends the script.
	s.SetBTurnsOffBraceExpansion = interp.No
	// Nor the `-h` POSIX names: `set -h` is refused the same way.
	s.SetHasTheHLetter = interp.No
	// `set -E` is taken here and `set -T` is not, which is why those two are
	// separate fields since #3366 — one answer could only have given this
	// column both letters or neither. Measured 2026-09-17 in the pinned
	// alpine image, BusyBox v1.37.0: `set -E` and `set -o errtrace` are 0,
	// `set -T` is `set: illegal option -T` at 2, and `set -o functrace` is
	// `set: illegal option -o functrace` at 1. There is no ERR trap in this
	// shell for the carriage to be about, so what the option moves is
	// nothing and what a script can see is the letter, the name and the
	// listing row.
	s.SetHasTheErrtraceLetter = interp.Yes
	// And no keyword option either. Measured 2026-09-16 in the pinned
	// alpine image, BusyBox v1.37.0: `set -k` is `set: illegal option -k`
	// at 2 and the file ends there. The refusal is this shell's answer
	// rather than a gap.
	s.KeywordAssignments = interp.No
	// A declaration utility is recognized by the name of the utility that
	// runs, however the word was written: `cmd=export; $cmd v=$b`, `\export
	// v=$b`, `'export' v=$b` and `e=; $e export v=$b` all keep `x y` whole,
	// where zsh, bash and ksh93 split at least one of them. The zero value,
	// stated here because it is measured and not inherited — 2026-09-16 on
	// BusyBox ash 1.37.0, for export, readonly and local.
	s.DeclarationCommandWord = interp.DeclarationByUtilityName
	// And `command export v=$b` keeps it whole too, in every spelling of the
	// prefix and with `-p` as well. See #3341.
	s.CommandPrefixKeepsADeclaration = interp.Yes

	// unanswered KeywordPromotesADeclarationsOperand: there is no keyword
	// option here to reach a declaration with — `set -k` is `illegal option
	// -k` and ends the file — so the question cannot be put to this shell.
	// `$-` under `-c` is `c`, and `set -e -u` makes it `uce` — so the letter
	// is shown, where dash shows nothing at all on that route.

	// The one column that does not read the sign: `+i` asks for a prompt
	// here exactly as `-i` does. Measured 2026-09-16 on BusyBox ash 1.37.0 in
	// the pinned Alpine image, the program on a pipe — `ash +i -c 'echo $-'`,
	// `ash -i +i -c` and `ash +i -i -c` each write `can't access tty; job
	// control turned off` and report `ci`, byte for byte with `ash -i -c`,
	// while a shell started with neither letter reports `c` and says nothing.
	// So the letter is a request that only ever arrives (#3221).
	s.PlusSignedInteractiveLetterStillPrompts = interp.Yes
	s.CommandStringShowsCInDollarDash = interp.Yes
	s.LoginShowsLInDollarDash = interp.No
	s.CommandStringShowsSInDollarDash = interp.No
	// And the order the letters come out in, which is this shell's own and
	// not a sort and not the order they were set in (#3256). Measured
	// 2026-09-17 in the pinned image, BusyBox v1.37.0, one letter at a time
	// and then every settable letter at once: `set -EubaCvxIfe` is
	// `EubaCvxcIfe` under `-c`, the same run with `-i` is `EubaCvxciIfe`,
	// and the same again on a terminal with `-m` is `EubaCvxcmiIfe`. The
	// `-s` route puts its letter where `c` stands on the other one —
	// `set -Eubax` over a pipe is `Eubaxs`.
	//
	// `n` is the one letter nothing can observe: `set -n` stops the `echo`
	// that would read `$-`, on every route including an interactive one, so
	// its place here is dash's and is a guess about a letter no script can
	// see. Every other letter in the string was run.
	s.DollarDashLetterOrder = "EubaCvxsncmiIfe"
	// Both spellings of the login option are taken — `ash -l -c cmd` and
	// `ash --login -c cmd` each run the command — where dash refuses the long
	// one outright.
	s.StartupFileOptions = interp.StartupFileOptions{Login: "-l --login"}
	// Semantics.SystemStartupFiles is left at what the POSIX preset gives it
	// — `/etc/profile` in the login slot — and that is an **unanswered
	// question rather than a measurement**, in the sense the package comment
	// above sets out. There is no BusyBox on the machine this was measured
	// on and the field was added by a change that could not run one, so the
	// standard's preset carries it the way it carries `.profile` itself.
	// #2263 is the follow-on that would let it be asked.
	// VersionOption is left at its zero deliberately: `ash --version` is `bad
	// option '--version'`. This shell will not name its version through an
	// option, which is the same answer dash gives and reached the same way.
	//
	// Job control wants the tty, and says so rather than failing: `set -m`
	// with no terminal remarks `can't access tty; job control turned off` and
	// still reports 0.
	s.MonitorNeedsATerminal = interp.Yes
	// The monitor alone is what `fg` and `bg` need, which here means with a
	// terminal: the monitor is denied without one, so the gate answers no
	// through the line above. Measured 2026-09-15 on a pseudo-terminal —
	// `set -m; sleep 0 &; fg` prints `sleep 0` at 0 — and off one, where the
	// denial leaves `fg: job (null) not created under job control` at 2
	// (#2720).
	s.MonitorAloneResumesAJob = interp.Yes
	// And announces nothing on the monitor alone, as dash does not.
	// Measured 2026-09-15 in the pinned image, a script file on a
	// pseudo-terminal (#2838).
	s.MonitorAloneAnnouncesAJob = interp.No
	s.InteractiveMonitorNeedsATerminal = interp.Yes

	// ---- builtins ----

	// A bare `read` fills REPLY, where dash wants a name.
	s.ReadRequiresAVariableName = interp.No
	// `read` takes rather more than dash's pair: `-r`, `-p`, `-t` and `-n`
	// were each run and each accepted, and so were `-d`, `-s` and `-u`,
	// which this list did not have until 2026-09-16 — `read -d ';' x` was
	// `illegal option -d` at 2 here and 0 with the text before the `;` in
	// BusyBox v1.37.0, the answer bash, zsh and ksh93 give too. `-a`, `-e`,
	// `-i` and `-N` are refused there, and are refused here.
	s.ReadOptions = "rsd:p:t:n:u:"
	// `unset` has the two POSIX letters and calls anything else illegal:
	// `unset -q x` is `illegal option -q`.
	// unanswered EmptyAssociativeKeyRefusesTheLength: there is no keyed table
	// to take the length of an element of. Measured 2026-09-12 in a
	// container, `w=; typeset -A m; ${#m[$w]}` is `typeset: not found` and
	// then `syntax error: bad substitution` at 2 — no subscript reaches a
	// parameter expansion here at all, the same wall dash meets one construct
	// earlier than this axis (#2286).
	// unanswered DeclareMatchingLetter: BusyBox ash has no declaration
	// utility either, so the letter has nowhere to be written. Measured
	// 2026-09-13 in the pinned image, `typeset -m x` is `typeset: not found`
	// at 127 (#2345).
	// unanswered AttributeOverAFrozenNameIsRefused: the axis is about a type
	// **letter** meeting a frozen name, and BusyBox ash has no letters to
	// bring. Measured 2026-09-13 in the pinned image, `typeset -i q=1` is
	// `typeset: not found` at 127, so the declaration the axis asks about
	// never happens. `export` over a `readonly` name — the only attribute
	// this shell does have — is taken at 0, which is a different question
	// and not this one (#2561).
	// unanswered OperandSubscriptQuoting: there are no arrays, so no operand
	// of this shell's carries a subscript for a quote to be written inside.
	// Measured 2026-09-18 on BusyBox 1.37.0 in the pinned image, `unset
	// "a[x]"` is `unset: line 1: a[x]: bad variable name` at 2 — the brackets
	// are part of a name and are refused as one, quoted or not (#3049).
	// unanswered SubscriptBeforeTheFirstElementRead,
	// unanswered SubscriptBeforeTheFirstElementNeedsAnElement and
	// unanswered SubscriptBeforeTheFirstElementRefusesTheLength: there is no
	// array to count back through, and no subscript on the right of a name
	// either. Measured 2026-09-18 on BusyBox 1.37.0 in the pinned image,
	// `a=(x y z)` is `syntax error: unexpected "("` at 2 and `a=x; echo
	// "${a[-1]}"` is `syntax error: bad substitution` at 2, so the brackets
	// are refused before anything asks where they reach (#3406).
	// unanswered ArrayLiteralOperandRetypesAFrozenScalar: there is no array
	// literal to be the operand. Measured 2026-09-13 in the pinned image,
	// `q=(a b)` is `syntax error: unexpected "("`, so the shell refuses the
	// spelling before any frozen name is consulted.
	// unanswered DeclareMappingLetter: the same, for `typeset -M x`.
	// unanswered DeclareTypeLetter: no declaration utility here either.
	// Measured 2026-09-13 in the pinned image, `typeset -T q=1` is
	// `typeset: not found` at 127, so neither the tie nor the type reading
	// of the letter can be put to this shell (#2419).
	// unanswered DeclareHideValueLetter: the same, for `typeset -H q=1`.
	// unanswered ProducedParameterListing: the axis is what a listing with no
	// operands writes for a produced parameter, and there is no listing.
	// Measured 2026-09-13 in the pinned image, `typeset -p` and `declare -p`
	// are both `not found` at 127 — and unlike dash this shell *does* have a
	// `$RANDOM`, so the wall is the missing builtin and not the missing
	// parameter (#2518).
	// unanswered ExpansionResultSuppliesGroupSyntax: BusyBox ash has no
	// pattern groups either, for the same reason dash has none.
	// unanswered TableLetterReachesItsOwnOperandsSubscript: BusyBox ash has
	// no arrays and no table letter, for the same reason dash has neither.
	// unanswered KeyedLiteralAppendJoinsTheReplacedValue: no keyed table and
	// no literal here either. Measured 2026-09-12 on BusyBox v1.37.0,
	// `m=([k]+=x)` is `syntax error: unexpected "("` (#2405).
	// unanswered FloatFormatLetterE: there is no declaration command to write
	// the letter on. Measured 2026-09-15 on BusyBox v1.37.0 in a container,
	// `typeset` is `not found` — the same wall dash meets (#2559).
	// unanswered DeclareNumberDetachedOnlyAtTheWordEnd: the same wall, so no
	// option word for a number to stand at the end of.
	// unanswered BareFloatLetterResetsThePrecision: nor any float attribute
	// to have a precision.
	// unanswered NumericTypeLetterPrecedence: nor a rank between them, for
	// the same reason — a declaration here cannot write two numeric letters
	// because it cannot write one (#2419).
	// unanswered DeclareHideInScopeLetter: no `typeset` here either, so the
	// lower-case `h` cannot be put to this shell under either reading.
	// unanswered NumericTypeLettersAreExclusive: nor a pair of numeric
	// letters to write together.
	// unanswered PrefixToAKeywordFunctionIsScopedToTheCall: the same wall —
	// BusyBox ash has no `function` keyword, so there is only one function
	// spelling and the prefix axis beside this one answers for it (#3161).
	// unanswered NamerefCycleIsRefused: there are no name references to make
	// a cycle of. Measured 2026-09-15 on BusyBox v1.37.0, `typeset` is not a
	// command here at all — `typeset: not found` at 127 — so neither
	// spelling of the declaration can be put to this shell (#2553).
	// unanswered NamerefLetterStandsAlone: the same wall — there is no
	// `typeset` to write the letters on (#3171).
	// unanswered DeclarationThroughAReferenceNamesTheOperand: the same wall
	// again — no references and no `typeset` (#3173).
	// unanswered NamerefTargetResolvedWhenAimed: the same wall — no
	// `typeset`, so there is no declaration for a target to be settled at
	// (#3124).
	// unanswered NamerefArrayRefusal: the same wall, and there are no arrays
	// here either for a reference to be refused over (#3103).
	// unanswered UnsetReferenceLetterRemovesANonReference: no `-n` on `unset`
	// here either. Measured 2026-09-12 on BusyBox v1.37.0, `unset -n x` is
	// `unset: line 0: illegal option -n`, and the shell ends at 2 without
	// reading the operand (#932).
	// unanswered UnsetSubscriptSkippedWhenNameUnset: there is no subscript
	// for `unset` to skip or read. Measured 2026-09-12, `unset "nope[x+]"`
	// is `nope[x+]: bad variable name` and ends the script, so the operand
	// never reaches a base name and a bracket (#2373).
	// unanswered UnsetStatusIsTheLastSubscripts: no operand here can fail
	// and be outlived by a later one. A bad variable name, a bracketed name
	// and a readonly all end the script, measured, so there is never a
	// status left behind for a following operand to overwrite or keep
	// (#2373).
	s.UnsetOptions = "vf"
	// `readonly` keeps POSIX's single letter. Measured 2026-09-12,
	// BusyBox v1.37.0: `readonly -a zz` is `readonly: line 0: illegal
	// option -a`, and so are `-A` and `-f`. There is no `typeset` here at
	// all, so even the listing that would show the attribute is missing —
	// which is why ReadonlyRecordsTheCompoundAttribute is unreachable in
	// this column rather than unanswered (#2277).
	//
	// `-n` is the exception and is the one letter beyond POSIX's: measured
	// 2026-09-18 in the digest-pinned image, `readonly -n r=v` is 0 where
	// `-a`, `-A` and `-f` are refused. It buys a script nothing but that
	// status — the name is frozen regardless, and `r=5` after it is `r: is
	// read only` — which is the other reading of the letter and why it is an
	// axis rather than a fact. See Semantics.ReadonlyReferenceLetter.
	s.ReadonlyOptions = "pn"
	s.ReadonlyReferenceLetter = interp.ReadonlyReferenceLetterIsInert
	// `export -n` is accepted and reports 0, which dash refuses outright.
	s.ExportTakesTheAttributeOff = interp.Yes
	// `export -f` is `illegal option -f`, so a function does not travel.
	s.ExportCarriesFunctions = interp.No
	s.ExportListing = interp.DeclareListingCommandWord
	s.ReadonlyListing = interp.DeclareListingCommandWord
	// Every listed value is single-quoted with an embedded quote doubled out:
	// `v="quo'te"; set` writes `v='quo'"'"'te'`, and `alias` writes its
	// bodies the same way.
	s.DeclareValueQuoting = interp.ListingQuoteAlwaysDoubled
	// unanswered ChainedSubscriptReadsANestedValue: the grammar for a chained
	// subscript is not this shell's — `${a[1][2]}` is a bad substitution or
	// a pattern here — so there is no chain for a reading to be about.
	// Measured 2026-09-15 (#2830).
	// unanswered ListedBangIsOrdinary: every listing style this shell uses
	// quotes whatever it is given, so `'!'`, `'^'`, `'a=b'` and `'é'` say
	// nothing about which bytes a listing may leave bare. Measured
	// 2026-09-14 with a bare `set` over all four (#2820).
	// unanswered ListedCaretIsOrdinary: the same always-quoting style.
	// unanswered ListedEqualsIsOrdinary: the same always-quoting style.
	// unanswered ListedNonAsciiIsOrdinary: the same always-quoting style,
	// and no `$'...'` listing here for the escaped half of the question.
	// unanswered ListedAssignmentPrefixIsBare: a bare head is only visible
	// in a listing that leaves anything bare, and this one leaves nothing.
	// unanswered OperatorAfterTheSubscriptListingIsBad: `${!name[@]}` is a
	// bad substitution here in the *bare* form too, so there is no listing
	// for an operator to come after. Measured 2026-09-14 (#2821).
	s.SetListing = interp.SetListingAssignments
	s.SetListingQuoting = interp.ListingQuoteAlwaysDoubled
	s.AliasQuoting = interp.ListingQuoteAlwaysDoubled
	s.AliasListingQuotesTheName = interp.No
	s.TrapQuoting = interp.ListingQuoteAlwaysDoubled
	// A bare `local` in a function writes nothing.
	// `local -` saves the `set` table, as in dash: measured 2026-09-15 in
	// the pinned Alpine image, `f() { local -; set -f; }; f; echo $-` comes
	// back without the letter.
	s.LocalDashSavesTheShellOptions = interp.Yes
	s.BareLocalListing = interp.BareLocalListsNothing
	// `local` reads no options: `local -r x` declares a variable named `-r`
	// and then refuses it as the bad name it is, so LocalOptions stays empty.
	//
	// A valueless declaration leaves the name unset and hides the outer
	// value: `x=outer; f(){ local x; echo "[$x]"; }; f` is `[]` here and
	// `[outer]` in dash. That pairing is bash's, and it is the sharpest of
	// the five places this shell takes bash's side over its sibling's.
	s.DeclaredNameWithoutValueIsEmpty = interp.No
	// And no record of it, for dash's reason rather than bash's: `local` is
	// the only declaration word here and there is no listing to read a
	// record back with. Answered rather than left unanswered because `local
	// x` reaches the axis (#2999).
	s.ValuelessDeclarationRecordsTheName = interp.No
	// unanswered PrefixListingNamesADeclaredOnlyCompound: dash's reason
	// exactly — no `${!prefix@}` and no compound for a declaration to bring
	// into being, so the axis is never asked.
	s.ValuelessDeclarationHidesTheOuterValue = interp.Yes
	// `local` outside a function is refused and the refusal is fatal:
	// `local x=1` at the top level is `local: not in a function` and the
	// script ends at 2.
	s.LocalOutsideAFunctionIsAnError = interp.Yes
	s.LocalOutsideAFunctionIsFatal = interp.Yes
	// No `typeset` and no `declare` — both are `not found` — so neither can
	// be asked about a subscript or a readonly.
	// unanswered ExportThroughASubscriptedOperandRecordsTheLetter: no
	// arrays and no subscripted operand, so nothing reaches it.
	s.DeclarationTakesASubscript = interp.No
	s.TypesetTakesASubscript = interp.No
	// unanswered UnsetElementEmptiesAnUnwrittenArrayInASubshell: no
	// arrays, so no element for a subshell to remove.
	s.UnsetTakesASubscript = interp.No
	// No arrays here either, so the same rule: a bracketed `read` operand
	// is a bad variable name and not a subscript. See dash, whose sentence
	// and status this column shares.
	//
	// unanswered StoreOperandWholeArraySubscript: the operand is refused as
	// a name before any subscript is read.
	// unanswered StoreOperandWholeArraySubscriptOverATable: no tables
	// either, so the keyed half is one further out of reach again.
	s.StoreOperandTakesASubscript = interp.No
	// unanswered BadSubscriptToUnset: there is no subscript to evaluate here,
	// so the arithmetic the axis is about is never reached. Measured
	// 2026-09-17 in the pinned image, BusyBox v1.37.0: `q=1; unset 'q[b c]'`
	// is `unset: q[b c]: bad variable name` and the shell ends at 2 — the
	// name is refused one complaint earlier.
	//
	// unanswered BadSubscriptToAnOutputOperand: the same wall one builtin
	// over. `echo Y | read 'q[b c]'` is `read: 'q[b c]': bad variable name`
	// at 1, so the store that walks the brackets is never reached either.
	//
	// unanswered BadSubscriptToADeclaration: and the third site does not
	// exist at all. Measured 2026-09-17 in the pinned image, BusyBox
	// v1.37.0: there is no `declare` or `typeset` (`declare: not found`,
	// 127), and `readonly 'q[b c]'=v` ends the script at 2 with `q[b c]: bad
	// variable name` — a name refused, with no subscript read. `export` is
	// the same complaint at the same status.
	//
	// unanswered BadSubscriptEscapesAnArithmeticCommand: no `(( ))`
	// grammar here either, and no subscripts to fail inside one.
	//
	// unanswered ValuelessSubscriptedOperand: the same wall, one operand
	// shape over. `readonly 'q[b c]'` with no value is the same refusal at
	// the same status, so there is no subscripted operand here at all.
	s.DeclarationNameOperands = interp.PlainNamesOnly
	s.UnsetNameOperands = interp.PlainNamesOnly
	s.ReadNameOperands = interp.PlainNamesOnly
	// No prompt operand: `read "v?p"` names one variable spelled `v?p`.
	s.ReadPromptOperand = interp.ReadOperandIsAllName
	// A special builtin's failure ends the script, and a bad name and a bad
	// option are both failures: `unset 1bad`, `export 1bad`, `unset r` over a
	// readonly and `export -Z` each stop at 2.
	s.BadNameToDeclarationFatal = interp.Yes
	s.BadNameToUnsetFatal = interp.Yes
	s.UnsetReadonlyFatal = interp.Yes
	s.BadOptionToSpecialBuiltinFatal = interp.Yes
	// `alias` is no more special here than POSIX makes it: the complaint is
	// said and the next command runs. Measured with `alias -g x`.
	s.AliasBadOptionFatal = interp.No
	// unanswered AliasNameCheckReachesALookup, AliasInvalidNameFatal: this
	// shell checks no alias name at all, so AliasNameRefusedCharacters is
	// empty and neither question is ever reached. Measured 2026-09-12:
	// `alias 'a b'=echo` is accepted in silence here and the name is listed
	// back, where the two shells that check refuse it (#2413).
	// The two spellings part company here, and this shell is the only one
	// in the panel where they do. Measured 2026-09-13, BusyBox v1.37.0:
	// `set -o zzznosuch; echo one; set -Z; echo two` writes a complaint,
	// prints `one`, writes a second complaint and stops at 2 — so the
	// unknown *name* is survivable and the unknown *letter* is not.
	//
	// This said Yes for both from the day the dialect was written, sitting
	// among the assignments that record a special builtin's failure being
	// fatal, and with no comment of its own. It was inherited from the
	// six-column measurement in #483, taken before this column existed
	// (#2272), and it made `set -o posix` — a name this shell has not —
	// end scripts that really carry on (#2629).
	s.BadSetOptionNameFatal = interp.No
	s.BadSetOptionLetterFatal = interp.Yes
	// And the column that decides the POSIX-mode pair has to be a pair of
	// fields rather than a constant in the mode. BusyBox has no `set -o
	// posix` and its `sh` applet is its `ash` applet, so nothing here moves
	// under either door: the name still reports 1 and carries on, the letter
	// still stops at 2. The core's mode is entered by *every* dialect
	// invoked as `sh`, so a written-in `Yes` for the name would end a script
	// this shell runs to the end — the same shape as #2629's bug, arriving
	// through the mode instead of through the preset (#2641).
	s.BadSetOptionNameFatalInPosixMode = interp.No
	s.BadSetOptionLetterFatalInPosixMode = interp.Yes
	// `set -ozzznosuch` here is a bare `-o` — the whole option table on
	// standard output — and then the letters of `zzznosuch`, so it stops at
	// 2 on `z` where `set -o zzznosuch` reports 1 and carries on. Which is
	// also the probe that showed the seam above is the *spelling* refused
	// rather than the `-o` route (#2629).
	s.SetOLetterAttachesItsName = interp.No
	// Measured 2026-09-16 on BusyBox 1.37.0: `sh --xtrace -c 'echo ran'`
	// prints `ran` untraced at 0, and so does `--zzznosuch`. The word is
	// neither read as a name nor refused — it is dropped — so the answer to
	// *this* axis is no. What it does with the word instead is a separate
	// question and is not claimed here.
	s.LongOptionNamesASetOption = interp.No
	// unanswered LongOptionNameIgnoresHyphens: the fold is a rule of the
	// `--name` spelling, and the axis above says this shell has no such
	// spelling. There is no site here to put the question to.
	// An option name is read exactly as it is written here. Measured
	// 2026-09-18 in the digest-pinned alpine image, BusyBox v1.37.0:
	// `set -o err-exit`, `set -o no_err_exit` and `set -o noerrexit` are each
	// `set: illegal option -o <word>` at 1, where `set -o errexit` beside them
	// is 0 (#3155, #3254).
	s.OptionNamespaceIgnoresSeparators = interp.No
	s.OptionNamespaceTakesANoPrefix = interp.No
	// unanswered LongOptionValueIsANumber: the `=value` it reads rides on a
	// `--name` option word, and the axis above says this shell has no such
	// word. There is no site here to put the question to.
	// And as in dash the next word is taken whatever it looks like:
	// `set -o -e` is `illegal option -o -e` at 1, errexit left off.
	s.SetODeclinesADashWord = interp.No
	s.SetListsOptionsOnceAtTheEnd = interp.No
	// A bare `-` turns `-x` and `-v` off here, and a bare `+` is consumed with no effect (#2699).
	s.BareOptionWord = interp.BareDashClearsTraceAndVerbose
	// And as in dash it applies as it goes: `command set -e -Z` leaves
	// errexit on, and the unguarded form ends the script for that reason
	// rather than because the refusal is fatal on its own.
	s.SetValidatesOptionLettersFirst = interp.No
	// And the refusal of a `set -o` name is not a failure when it arrives on
	// the command line: `ash -o zzznosuch -c "echo after"` writes the
	// complaint, runs nothing, and exits 0 — where the same name inside a
	// script reports 1 and the same *letter* on the same route reports 2.
	s.BadSetOptionNameAtInvocationExitsZero = interp.Yes
	// `read` is not one of the three: `printf 'x\n' | read 1bad` reports at 1
	// — not dash's 2 — and the script carries on.
	// unanswered BadNameToPrintfFatal: no `-v` here either, so no output
	// operand is judged.
	s.BadNameToReadFatal = interp.No
	// `echo` needs `-e` to interpret an escape, and has `-E` and `-n` beside
	// it: `echo "a\tb"` writes the backslash, `echo -e "a\tb"` writes a tab,
	// and `echo -E "a\tb"` writes the backslash again. dash is the shell that
	// interprets without asking, so this is the second bash-ward answer.
	s.EchoInterpretsEscapes = interp.No
	s.EchoOptions = "neE"
	// And the set behind that `-e` is bash's rather than the XSI one, which
	// is the half that was read off the wrong probe (#3226). `echo -e
	// 'A\x41B'` is `AAB` here, and `echo -e 'a\eZ:a\EZ'` is `61 1b 5a 3a 61
	// 5c 45 5a` — the escape character under `\e` and the two characters
	// under `\E`, which is zsh's split and not ksh93's opposite one.
	//
	// The probe that has to be used is the one with the `-e` on it. Without
	// the letter this shell interprets nothing, so `echo 'A\x41B'` writes
	// `A\x41B` — which is *also* what a shell with no `\x` in its set
	// writes. The two readings coincide on every probe that leaves the
	// letter off, so a bare `echo` cannot tell them apart and three of these
	// values were set from one that could not.
	s.EchoExpandsHexEscapes = interp.Yes
	s.EchoExpandsEscEscape = interp.Yes
	s.EchoExpandsCapitalEscEscape = interp.No
	// `\u` and `\U` are in neither set: `echo -e 'a\u0041Z'` writes the
	// eight characters as they stand.
	s.EchoExpandsUnicodeEscapes = interp.No
	// And a `\x` that runs out of digits stands rather than reading as a
	// zero: `echo -e 'a\xZb'` is `61 5c 78 5a 62`, the same answer bash
	// gives and not zsh's NUL. Set here rather than left to the POSIX base
	// because the axis is only reachable at all once the hex escape above is
	// live, and a default that happens to be right is not a measurement.
	s.EchoEmptyHexDigitRunIsNul = interp.No
	// `printf 'a\x41Z'` is `aAZ`, so the hex escape is here where dash has
	// none at all, and `%b` takes it too. `\u` is not: `printf 'a\u0041Z'` is
	// the text as written. The digit rule is the same at both sites and the
	// same as `echo -e`'s: two digits at most, so `printf 'a\x4142b'` is
	// `aA42b`.
	// And it says nothing when the digit run is empty, where bash — the only
	// other column that reaches the question — writes a complaint. Measured
	// 2026-09-17 in the pinned image, BusyBox v1.37.0: `printf 'a\xZb'`
	// writes `a\xZb` at 0 with an empty standard error, and `printf '%b'
	// 'a\xZb'` is the same. The escape standing and the status were already
	// right; the extra sentence was bash's, inherited because an empty
	// wording means the substrate's own rather than silence (#3239).
	s.PrintfReportsAMissingHexDigit = interp.No
	s.PrintfHexEscape = interp.PrintfHexEscapeByte
	s.PrintfBHexEscape = interp.PrintfHexEscapeByte
	s.PrintfUnicodeEscape = interp.PrintfUnicodeEscapeAbsent
	s.PrintfBUnicodeEscape = interp.PrintfUnicodeEscapeAbsent
	// `\e` in a *format* too, and `\E` at neither site: this column's split
	// is zsh's rather than dash's silence. Measured 2026-09-16 in BusyBox
	// 1.37.0 inside the pinned Alpine image, `od -An -c`: `printf 'a\eZ'` is
	// `a 033 Z` and `printf 'a\EZ'` is `a \ E Z` (#3225).
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
	// And `%b` splits the two spellings of the escape character exactly as
	// `echo -e` does: `printf '%b' 'a\eZ:a\EZ'` is ` 61 1b 5a 3a 61 5c 45
	// 5a`. Measured at this site rather than borrowed from the one above,
	// because ksh93 is the shell whose two sites disagree.
	s.PrintfBEscEscape = interp.Yes
	s.PrintfBCapitalEscEscape = interp.No
	// `printf '%b\n' 'a\101b'` is `aAb`, so the octal needs no `\0`.
	s.PrintfBOctalWithoutZero = interp.Yes
	// And what it stopped is not padded out: `printf "%b|" "a\c"` is `a` and
	// nothing after it.
	s.PrintfBStopIsPadded = interp.No
	// `[ a == a ]` is 0, so `test` takes the doubled operator beside `=`.
	s.TestAcceptsDoubleEqual = interp.Yes
	// Both string-ordering operators, and none of the three unary additions
	// bash and ksh93 have past the three-word rules — measured through the
	// container, where `test -a f` is `unknown operand` at 2. On this
	// question BusyBox sides with dash rather than with bash.
	s.TestStringOrder = interp.TestStringOrderBoth
	// `set -o` lists `pipefail`, which dash's does not have.
	s.PipefailOption = interp.Yes
	// A frozen name in a command prefix is refused before anything else
	// happens: `unset u; readonly x=1; x=${u:=set} /bin/true` is `x: is read
	// only` and leaves `u` unset, so neither the value nor the command was
	// reached. dash answers the other way (#1943).
	s.PrefixToAFrozenNameIsCheckedFirst = interp.Yes
	// `printf 'a\cb'` writes `a` and stops, where dash writes the letter.
	s.PrintfBackslashC = interp.PrintfBackslashCStops
	// `printf '%ld\n' 5` is 5, so the length modifiers are read rather than
	// refused as conversions.
	// A run of `h`, `l`, `z` and `L`, which is neither of the two answers the
	// other columns hold. Measured 2026-09-17 in the pinned image: `%zd`,
	// `%zs`, `%zc`, `%ld`, `%hd`, `%Ld`, `%hhd` and `%lld` each write their
	// operand, while `%jd` and `%td` are `%jd: invalid format` — so the run
	// is C99's minus the two letters naming an integer type rather than a
	// width, and it is a run rather than one letter (#3141).
	s.PrintfLengthModifiers = interp.PrintfLengthModifiersC99ExceptJAndT
	// Bytes, with or without an `l`, under `LC_ALL=C.UTF-8` as under C:
	// `printf '[%.2s|%.2ls|%lc]' αβγ αβγ αβγ` is `[α|α|` and the byte 0xce,
	// BusyBox ash 1.37.0 in the pinned Alpine image, 2026-09-16.
	s.PrintfFieldCountsCharacters = interp.No
	s.PrintfLongModifierCountsCharacters = interp.No
	// No `%q` and no `%(fmt)T`: both are `invalid format`.
	s.PrintfQuote = interp.PrintfQuoteAbsent
	// C's `#` at a value of nought. Measured inside the pinned Alpine
	// image 2026-09-15: `printf '%#x' 0` is `0` and `printf '%#.0o' 0`
	// is `0`, which is musl reading the same C the BSD columns do.
	s.PrintfAlternateFormAsksTheValue = interp.Yes
	// And counts it against the width: `printf '%#05x' 7` is `0x007`,
	// five characters, on BusyBox 1.37.0 in the pinned Alpine image.
	s.PrintfZeroFillCountsTheAlternatePrefix = interp.Yes
	s.PrintfTimeConversion = interp.No
	// An empty operand to a numeric conversion is reported, where dash reads
	// it as a zero and says nothing.
	s.PrintfEmptyIsNotANumber = interp.Yes
	s.PrintfAbsentNumberIsAnEmptyOne = interp.Yes
	s.PrintfStarWithoutOperandIsRefused = interp.No
	s.PrintfStarComplaintCostsTheStatus = interp.No
	s.PrintfNonFiniteIsConverted = interp.Yes
	// No `'` flag, as dash has none: `%'d` is `invalid format` at 1.
	s.PrintfGroupingFlag = interp.No
	s.PrintfGroupingFlagAfterTheWidth = interp.No
	// A `*` beside a width's own digits is refused here too: `printf '%5*d' 4 42`
	// is a conversion character this shell does not have (#2824).
	s.PrintfStarBesideTheFieldDigits = interp.No

	// The field is dropped, operand and all, and the builtin carries on
	// reporting 1: `printf '[%21474836470s]' a b` is `[][]` at 1. Only
	// above INT_MAX — `%2147483647s` is laid out in full here.
	// Measured 2026-09-15.
	s.PrintfFieldBeyondAnInt = interp.PrintfFieldEmpty

	// A number this shell will not read rather than one it read and cannot
	// hold, which is the reading and not only the wording: the complaint is
	// the bad-number sentence — Diagnostics.PrintfNumberOutOfRange is empty
	// here — and what it leaves behind is a zero, so
	// `printf '[%.*f]' 21474836470 1` is `[1]`, identical to this column's
	// answer for `abc`. bash parts from it on exactly that, leaving the
	// field absent and writing `[1.000000]`. It costs nothing, since
	// PrintfStarComplaintCostsTheStatus is No, so
	// `printf 'A[%*s]B' 21474836470 x` is `A[x]B` at 0. Measured 2026-09-15.
	s.PrintfStarBeyondAnInt = interp.PrintfStarIsNotANumber
	s.CaseSubjectKeepsThePreviousLine = interp.No
	s.SubstringRangeThirdColonIsABadSubstitution = interp.No
	s.PrintfReportsBadNumber = interp.Yes
	s.PrintfNumberOperand = interp.PrintfNumberWholeOperand
	// Exact: `printf '%d' 9007199254740993` is itself in BusyBox v1.37.0,
	// so the reader behind it is an integer one (#2907).
	// A flag past a field is no flag at all: the prefix ends there and the
	// byte arrives at the scan as the conversion character (#2910).
	// unanswered ArithDivisionByZeroYieldsAValue: the value a division by
	// zero leaves behind is only visible through a `printf` operand, and
	// this shell's printf evaluates no operand -- so the question has no
	// site here. Every other expression abandons the command, here as in
	// the two columns that do evaluate.
	s.PrintfFlagAfterTheField = interp.No
	s.PrintfIntegerOperandGoesThroughTheFloatingType = interp.No
	// None of C99's three: `printf '%F' 1.5` is `%F]: invalid format` at 1
	// in BusyBox ash 1.37.
	s.PrintfC99FloatConversions = interp.No
	// unanswered PrintfHexFloatZeroFillPrecedesThePrefix: and so no fill to
	// place in it either.
	// unanswered PrintfHexFloatDefaultIsTwelveDigits: there is no `%a` here
	// to have a default precision.
	// unanswered PrintfRefusedOperandKeepsItsLeadingNumber: this shell keeps
	// no partial number at all — an operand its reader cannot finish is a
	// zero — and it evaluates nothing, so the question has no site here.
	// unanswered PrintfFloatOperandIsEvaluatedTwice: for the same reason.
	s.PrintfUnfinishedConversionIsAPercent = interp.No
	// `printf -v x '%s' hi` assigns nothing: `-v` is read as the format.
	s.PrintfAssignsWithV = interp.No
	s.PrintfRejectsUnknownOption = interp.No
	// `cd ""` and `HOME=; cd` both say nothing and report 0; `cd alpha beta`
	// goes to alpha and says nothing; `cd -` prints where it went.
	// Measured 2026-09-12 on BusyBox v1.37.0: `OLDPWD=/nonexistent ash -c 'echo
	// $OLDPWD'` answers the path it was given, and `cd -` then answers `can't cd
	// to /nonexistent: No such file or directory` at 2.
	s.InheritedOldpwd = interp.InheritedOldpwdTaken
	// The depth, counted and told to every child. This is the column the
	// issue's own table did not have: BusyBox ash 1.37.0 sets `SHLVL=1`
	// under `env -i` and exports it, which puts ash on bash's side of this
	// and leaves dash alone on the other. See interp.ShellLevelPolicy.
	s.ShellLevel = interp.ShellLevelCounted
	// And a shell that replaces this process counts one deeper, as ksh93
	// does: measured 2026-09-18 in the pinned image, `exec /bin/busybox ash
	// -c 'echo $SHLVL'` reads 2 where this shell holds 1.
	s.ShellLevelExec = interp.ShellLevelExecCounted
	// A handed-in `PWD` names the starting directory here too, but only where
	// it really is that directory: measured 2026-09-13 in the pinned image,
	// `PWD=/link/d` under a symbolic link survives and `PWD=/usr` in a
	// directory that is not `/usr` is dropped for the real path. ksh93 keeps
	// that second one, and also looks at `$HOME` where this shell does not.
	s.StartupPwdName = interp.StartupPwdNameFromTheEnvironmentWhenItFits
	s.CdWithoutHomeIsAnError = interp.No
	s.CdEmptyOperandIsAnError = interp.No
	s.CdEmptyHomeIsAnError = interp.No
	s.CdSubstitutesTheOperands = interp.No
	s.CdSubstitutionPrintsTheDirectory = interp.No
	s.CdRefusesExtraOperands = interp.No
	s.CdRefusesUnknownOption = interp.Yes
	s.CdHasQuietOption = interp.No
	s.CdHasSymlinkFreeOption = interp.No
	s.CdLastPathOptionWins = interp.Yes
	s.CdDashPrintsTheDirectory = interp.Yes
	// `umask` prints four digits, and `umask -S` prints the symbolic form.
	s.UmaskPrintsFourDigits = interp.Yes
	s.UmaskSetWithSPrints = interp.No
	s.SymbolicMaskTakesMoreThanOneOperator = interp.Yes
	s.SymbolicMaskWhoAloneSetsIt = interp.No
	// The two bits a umask has no room for split this shell the opposite way
	// from dash, and both were inherited rather than measured until #3237.
	// Measured 2026-09-17 in the pinned image, BusyBox v1.37.0, with
	// `umask u+r` beside each as the control: `umask u+s` is
	// `umask: illegal mode: u+s` at 2 — the whole clause named, not the
	// letter — and `umask u+t` and `umask o+t` are both taken at 0. So the
	// refusal is the setuid letter's alone, and a clause carrying it is
	// refused whole: `umask u=rwXs` is refused where `umask u+X` is taken.
	s.SymbolicMaskTakesTheSetuidLetter = interp.No
	s.SymbolicMaskTakesAPermissionCopy = interp.Yes
	// And a copy is taken **alone**: beside a permission letter, in either
	// order, the whole operand is `illegal mode` at 2 — `umask -S g=uw` and
	// `umask -S g=wu` both, where the copy on its own is fine (#3074).
	// Measured 2026-09-18 in the digest-pinned alpine image.
	s.UmaskPermissionCopyBesideLetters = interp.UmaskPermissionCopyRefusesTheMixture
	s.SymbolicMaskTakesTheConditionalExecuteLetter = interp.Yes
	s.SymbolicMaskTakesTheStickyLetter = interp.Yes
	// `shift -1` is `Illegal number: -1`, so every dash word is read as the
	// count and there is no `--` to end options with.
	s.ShiftOptionWords = interp.ShiftOptionWordsNone
	s.ShiftDoubleDashEndsOptions = interp.No
	// BusyBox answers as dash does: `break -- 1` is
	// `break: Illegal number: --`, measured in the pinned alpine image.
	s.NumericOperandDoubleDashEndsOptions = interp.No
	// The count is taken and the rest of the line is not read.
	s.ExtraNumericOperand = interp.ExtraNumericOperandIgnored
	s.ShiftNamesAreArrays = interp.No
	s.ShiftNegativeIsOutOfRange = interp.No
	s.ShiftCountIsArithmetic = interp.No
	// `shift` past the end is **not** fatal here, which is BusyBox siding
	// with bash rather than with the dash this preset starts from: `shift 5`
	// with nothing to shift says nothing at all, leaves 1 behind and leaves
	// `$#` alone, and the next command runs. Measured 2026-09-12 in the
	// pinned alpine image, and `axis/shift-past-end` has recorded `survived`
	// in the ash column since that column existed.
	//
	// Inherited from PosixSemantics as Yes and never overridden, so nothing
	// objected: no corpus case reaches this dialect's answer, which is the
	// blind spot #2441 is about. It was found by grading the preset against
	// the golden record rather than by running anything.
	s.ShiftPastEndFatal = interp.No
	// `command -Z true` is `illegal option -Z` at 2.
	s.CommandRejectsUnknownOption = interp.Yes
	// Whether `command -v` answers for every name it was given, and what
	// decides the status when it found some of them. See
	// interp.Semantics.CommandReportsEveryOperand for the split.
	s.CommandReportsEveryOperand = interp.No
	s.CommandCountsAMissingOperand = interp.No
	// And whether a `command` reached through an expansion keeps the power
	// to run what it names (#3369).
	s.ExpandedCommandOnlyReports = interp.No
	// What a `command -p` search resolved is **not** remembered here: with
	// an unusable PATH, a `command -p ls` and a plain `ls` after it, the
	// second is 127 in this column and 0 in bash. See
	// interp.Semantics.DefaultPathSearchIsRemembered (#2975).
	s.DefaultPathSearchIsRemembered = interp.No
	// And the word is a boundary around everything it runs. Measured
	// 2026-09-13 inside the pinned alpine image: `eval 'export -q; echo
	// INNER'` stops the script, `command eval '…'` reports 1 and carries on,
	// and the same split holds for `${NOPE?bad}`, a readonly reassignment and
	// an unset name under `set -u` — alive on all three with the word, gone
	// on all three without it.
	s.FatalErrorEndsAtTheCommandWord = interp.Yes
	s.GetoptsRejectsUnknownOption = interp.No
	s.GetoptsAssignmentRestartsWord = interp.Yes
	// `kill %1` reaches the job's process here, as it does in bash.
	s.KillJobSpecAimsAtTheGroup = interp.No
	// A trim on `$@` runs over the whole list once, as it does in dash.
	s.OperatorDistributesOverTheFieldList = interp.No
	// And a **non-global** replacement over that joined list ends it at the
	// parameter it replaced in: `set -- xb alpha beta; n "${@/a/-}"` is
	// `2:<xb><-lpha>` here, where the join reading alone would keep `beta`
	// and bash 5.3.20, zsh 5.9.2 and ksh93u+ all replace in every parameter.
	// A pattern that matches nothing keeps all three, the global `//` cuts
	// nothing, and the unquoted spelling is cut identically — so it is the
	// first match and not the quoting. dash 0.5.12, the other column that
	// joins, has no `${x/pat/rep}` at all. Measured 2026-09-18 on BusyBox
	// ash 1.37.0 in the digest-pinned Alpine image, with `cmd/ash`
	// cross-compiled into the same container (#3413).
	s.ReplacementEndsTheJoinedFieldList = interp.Yes
	// And nor does it over `"$*"`, which is dash's and zsh's answer:
	// `"${*#a}"` over `aa ab ba` is `a ab ba`. Measured 2026-09-16, and
	// unanswered until now, so the construct was a refusal.
	s.OperatorDistributesOverStarSubscript = interp.No
	// And it counts a clustered word at its first letter, which is dash's
	// answer and not bash's: `-abc` reads `a` with OPTIND already 2.
	s.GetoptsCountsTheWordAtItsFirstLetter = interp.Yes
	// Counting a word early is the opposite end of the same question from
	// counting it late, and this shell is at the early end — measured here
	// rather than assumed from dash (#3275).
	s.GetoptsCountsTheWordOnTheNextCall = interp.No
	s.GetoptsEndOfOptionsNamesIt = interp.Yes
	// A `+`-prefixed word is an operand and ends the scan: measured
	// 2026-09-17 in the pinned alpine image on BusyBox v1.37.0, `getopts a o
	// +a` is 1 with the name `?`.
	s.GetoptsTakesAPlusPrefixedOption = interp.No
	// A shell function call gets a `getopts` scan of its own here, while
	// OPTIND itself stays the shell's: a helper called twice reads its
	// arguments twice, and `$OPTIND` inside the call is still the caller's
	// number. Measured 2026-09-15 — `g() { while getopts ab o "$@"; do :;
	// done; }` called twice on `-a -b` sees both options both times, where
	// bash 5.3 and ksh93u+ see them once. BusyBox ash answers both of these
	// exactly as dash does (#2944).
	s.GetoptsFunctionPosition = interp.GetoptsFunctionPositionIsTheCallsOwn
	// `#` in an option string is another option letter here. Measured
	// 2026-09-16, BusyBox ash 1.37.0 (#2947).
	s.GetoptsOptionStringHasANumericType = interp.No
	// OPTERR is an ordinary variable here: measured 2026-09-17 in the pinned
	// alpine image on BusyBox v1.37.0, a bad option is one line on stderr with
	// it set to 0 as with it set to 1.
	s.GetoptsOptErrSilencesTheComplaint = interp.No
	s.GetoptsClearsOptarg = interp.No
	// But OPTARG is *emptied* rather than unset when the option that was read
	// is one the string has and takes no argument, which `${OPTARG-…}` and
	// `${OPTARG+…}` tell apart — the two spellings a careful script uses to
	// ask whether the option it just read carried a value. zsh agrees here
	// and disagrees on the row above, which is why the two are two axes.
	s.GetoptsEmptiesOptargForAnArgumentlessOption = interp.Yes
	// The same three answers dash gives, measured the same day in the pinned
	// alpine image on BusyBox v1.37.0: `getopts: OPTARG: is read only` at
	// status 2, OPTIND still 1, and the next command on the line still runs.
	// The end of the options unsets OPTARG, which is where this shell parts
	// from dash for the third time in one builtin. It is an ordinary write,
	// so a frozen OPTARG refuses it: measured 2026-09-16 in the pinned image,
	// `OPTARG=PRESET; readonly OPTARG; set -- -- x; getopts a: o` is
	// `getopts: OPTARG: is read only` at 2 with `PRESET` standing, the name
	// unwritten and OPTIND still 1 — so the clearing happens before the word
	// count moves.
	s.GetoptsUnsetsOptargAtEndOfOptions = interp.Yes
	s.GetoptsClearingOptargIsARealUnset = interp.No
	s.GetoptsOwnParametersIgnoreAFreeze = interp.No
	s.GetoptsRefusedWriteEndsTheBuiltin = interp.Yes
	// And a freeze on the **name** costs the same whichever path reached it:
	// measured 2026-09-17 in the pinned image, `readonly N` with `set -- -a v`
	// and with `set -- x` both write `getopts: line 1: N: is read only` at 2
	// and the script runs on.
	s.GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal = interp.No
	// And `read` the same, at 2 wherever the frozen name stood. Measured in
	// the pinned 1.37.0 image, 2026-09-16 (#3208).
	s.ReadRefusedWriteEndsTheBuiltin = interp.Yes
	s.ReadRefusedWriteIsOneOnTheLastName = interp.No
	s.ReadonlyRefusalInABuiltinIsFatal = interp.No
	// `alias` reads no options at all, so `-p` is a name it cannot find and
	// `-g` and `-s` are neither kinds nor letters.
	s.AliasParsesOptions = interp.No
	s.AliasHasPrintOption = interp.No
	// Nor an `-x`: `alias` reads no options here either.
	s.AliasHasExportOption = interp.No
	s.GlobalAliases = interp.No
	s.SuffixAliases = interp.No
	s.AliasListsAsDefinitions = interp.No
	s.AliasRestrictsToRegularKind = interp.No
	s.AliasOperandsCanBePatterns = interp.No
	s.AliasPlusPrintsNamesOnly = interp.No
	s.TypeNamesAnAliasOnlyWhenExpanded = interp.No
	s.AliasReportsNotFound = interp.Yes
	s.UnaliasReportsNotFound = interp.Yes
	s.AliasNotFoundStatusCounts = interp.No
	// A removed alias leaves nothing behind. Measured 2026-09-16 in the
	// pinned Alpine image, BusyBox 1.37: `alias h=1; unalias h; unalias
	// h` is 0 then 1, and a name only looked up is not remembered either.
	s.AliasRemembersTheNamesItNames = interp.No
	s.AliasSeparatorEndsTheLookup = interp.No
	// A command word that is exactly `-` is a command name here and is
	// reported as one: `- echo hi` is `command not found` at 127 and the
	// script carries on. zsh is the column that throws the word away (#3236).
	s.LoneDashInCommandPositionIsDiscarded = interp.No
	s.UnaliasAllRefusesOperands = interp.No
	// `type -- cd` reads the `--` as a name rather than as the end of
	// options, so no letter of its own is reachable — `-t` included.
	// BusyBox ash 1.37.0 draws the distinction exactly as dash does, so this
	// is the preset's value measured rather than inherited (#3248's shape).
	// BusyBox ash 1.37.0 ends the script here exactly as dash does — `start`
	// and 2, measured in the container rather than assumed from the preset,
	// which is #3248's shape (#3274).
	s.SubstitutionParseErrorEscapesASubshell = interp.Yes
	// And from a here-document body, measured 2026-09-17 in the pinned Alpine
	// image rather than inherited from the preset: BusyBox ash 1.37.0 prints
	// `start` and exits 2 there, as dash does (#3318).
	s.SubstitutionParseFailureInAHeredocBodyEndsTheShell = interp.Yes
	// And the status is the refusal's own, measured the same way (#3319).
	s.SubstitutionParseFailureCarriesTheFatalStatus = interp.No
	s.TypeDistinguishesSpecialBuiltins = interp.Yes
	// And it draws the *membership* as dash does too, `local` included:
	// measured 2026-09-16 in the digest-pinned Alpine image under `--init`,
	// `type local` is `local is a special shell builtin` where `type echo`
	// is plain, `local qq` outside a function ends the script at 2, and
	// `LV=1 local x` inside one leaves LV at 1 where `CV=1 command true`
	// leaves CV unset. `source` is the one name where the two columns part
	// — BusyBox has it and marks it special, dash has no such builtin — and
	// it is already in the substrate's own list, so it is not repeated here.
	s.SpecialBuiltinsBeyondPosix = "local"
	s.TypePrintsFunctionBody = interp.No
	s.TypeEndsOptionsWithDashDash = interp.No
	// `ulimit -a` is laid out with bash's labels and letters rather than
	// dash's; the block unit and the resources it knows were read off that
	// listing.
	s.UlimitBlockIsKilobyte = interp.No
	// An empty operand is refused — `invalid number ''` at 1, with the limit
	// untouched — which is the answer bash 5.3 gives and not dash's (#3064).
	// Measured 2026-09-18 in the digest-pinned alpine image.
	s.UlimitEmptyOperandIsZero = interp.No
	// And CDPATH is a search beside the ordinary relative lookup, with the
	// fallback POSIX gives it (#2896).
	s.CdpathReplacesTheRelativeLookup = interp.No
	s.UlimitHasResidentSet = interp.Yes
	s.UlimitHasProcessCount = interp.Yes
	s.UlimitSetsBothLimits = interp.Yes
	s.UlimitTakesHardKeyword = interp.No
	s.UlimitTakesSoftKeyword = interp.No
	s.UlimitOperandIsArithmetic = interp.No

	// ---- traps, signals and jobs ----

	// `trap 'echo t' SIGINT` is accepted, where dash takes only the bare
	// name.
	s.SIGPrefixAccepted = interp.Yes
	// `trap 'echo e' ERR` is accepted too — the sole holdout on the
	// pseudo-conditions is dash, and this shell is not with it.
	s.TrapHasErrCondition = interp.Yes
	// And a command that ran the failure fires it again whatever the trap
	// was doing when the command began, which is ksh93's answer and not
	// bash's: measured in BusyBox 1.37, `g() { trap 'echo I' ERR; false; }`
	// writes two I lines where bash writes one. The count stays at one for
	// a trap set at the top — `f(){ g; }; g(){ h; }; h(){ false; }; f`
	// writes a single E — because this shell does not carry the trap into a
	// call at all, so only the outermost of the three is a place it can
	// fire.
	s.ErrTrapRefiresForTheCommandItFiredInside = interp.ErrTrapAlwaysRefires
	// unanswered FailingPipelineWhoseLastElementRanHere: every element of a
	// pipeline is a subshell here and nothing moves one into this shell, so a
	// pipeline is judged once by its status and the question is never put.
	s.TrapHasDebugCondition = interp.No
	// No DEBUG condition, so no head to fire one at. Not an unanswered
	// axis — DebugTrapHeads has no unspecified value, because a head
	// either fires or does not and there is no third thing for a
	// dialect to be silent about.
	s.DebugTrapCompoundHeads = interp.DebugTrapHeadsNone
	// And a pipeline fires nothing, for the reason the line above says
	// nothing fires: BusyBox refuses `trap … DEBUG` outright. The value is
	// the absence of a pipeline rule rather than a reading of one.
	s.DebugTrapPipelines = interp.DebugTrapPipelineInEachElement
	s.TrapHasReturnCondition = interp.No
	// `trap` reads options and has none of them: `-p` and `-l` are both
	// `illegal option`.
	s.TrapParsesOptions = interp.Yes
	s.TrapPrintsWithP = interp.No
	s.TrapPrintsBareWithConditions = interp.No
	s.TrapPrintsBareWithP = interp.No
	s.TrapListsSignalsWithL = interp.No
	s.TrapOneArgumentIsACondition = interp.Yes
	s.TrapReportsAnUnknownSingleCondition = interp.Yes
	s.TrapSingleUnknownConditionIsUsage = interp.No
	s.TrapActionIsParsedWhenSet = interp.No
	s.TrapParseFailureNamesWhereItFired = interp.No
	s.TrapBodyRunsWhatParsed = interp.Yes
	// A subshell's listing shows only what survived the entry, which is the
	// POSIX answer and is measured rather than assumed.
	s.SubshellKeepsTrapListing = interp.No
	s.PipelineElementKeepsTrapListing = interp.No
	s.BackgroundJobKeepsTrapListing = interp.No
	s.SubshellHidesInheritedIgnoredTraps = interp.No
	// An EXIT trap does not run when a signal kills the shell: `trap 'echo
	// bye' EXIT; kill -TERM $$` writes nothing and dies at 143.
	s.ExitTrapRunsOnSignalDeath = interp.No
	// BusyBox ash sides with bash here rather than with dash, which is what
	// this line said until it was measured: `kill -QUIT $$; echo after`
	// prints after and exits 0, an external SIGQUIT is survived too, and
	// `kill -TERM $$` in the same shell dies at 143 — so it is this signal's
	// disposition and not a `kill` that delivers nothing. The oracle's own
	// record of `signal-death/quit-is-not-fatal-in-every-shell` has said so
	// for the whole of the ash column's life.
	s.QuitIgnoredWhenNotInteractive = interp.Yes
	// And the ignore survives `trap -`, as it does in bash.
	s.QuitResetRestoresTheDefault = interp.No
	s.HangupIsAnOrderlyExit = interp.No
	s.ExitInTrapReportsEarlierStatus = interp.Yes
	// `-n` is not an option here either, and this shell reads what is left
	// as a signal rather than as an option: `kill -n 99 <pid>` is `bad
	// signal name 'n'` at 1, the same sentence `kill -Q` draws. `kill -s`
	// with nothing after it is `bad signal name 's'` for the same reason —
	// nothing is missing as far as this applet is concerned, the letter is
	// the spec. Measured 2026-09-17 against BusyBox v1.37.0 (#3165).
	s.KillReadsTheNumberOption = interp.No
	s.KillOptionWithNoArgumentIsASignalName = interp.Yes
	s.KillListAcceptsName = interp.Yes
	// ksh93's reading of the number, measured in the container: `kill -l
	// 256` is EXIT, `257` is HUP, `300` is 44 and `160` is 32.
	s.KillListReducesRepeatedly = interp.Yes
	s.KillListPrintsANumberItCannotName = interp.Yes
	s.KillListNamesZeroAsExit = interp.Yes
	// `exec -a name` is BusyBox's too, which the ash preset had inherited a
	// No for: `exec -a NAME /bin/echo` was `-a: not found` here and runs the
	// applet named NAME there. The other two letters are not — `-l` and `-c`
	// are `illegal option` at the same door `-x` is, and this shell's `exec`
	// is special, so the script ends there (#3056).
	s.ExecTakesOptions = interp.Yes
	s.ExecTakesTheLoginLetter = interp.No
	s.ExecTakesTheEmptyEnvironmentLetter = interp.No
	s.ExecLoginPrefixesTheGivenName = interp.No
	s.KillStatus = interp.KillStatusAnyFailure
	s.SubshellJobTable = interp.SubshellJobsCleared
	// A job started with `&` reads an empty standard input: `ash -c
	// '/bin/cat & wait; echo ---' < f` writes `---` and nothing else.
	s.BackgroundJobInput = interp.BackgroundJobInputEmpty
	// `$!` before any background command is unset rather than zero: measured
	// 2026-09-18 in the pinned image, `${!-unset}` takes its default,
	// `${!+set}` is empty, and `set -u; echo "[$!]"` is `!: parameter not
	// set` at 2.
	s.LastBackgroundPid = interp.LastBackgroundPidUnset
	s.ProcessSubstitutionIsTheLastBackgroundJob = interp.No
	// A `jobs` listing keeps a job that has already ended, and shows the
	// `&`-started command's own text.
	s.JobsListNewestFirst = interp.Yes
	// StoppedJobTakesTheCurrentJobMarker is deliberately left unanswered.
	// The measurement wants a ^Z at a pseudo-terminal, which is exactly what
	// the container this shell was measured in could not give it, so there
	// is nothing recorded to put here — and the package comment above says
	// to read that silence as an unanswered question rather than as
	// agreement with dash. The axis is read and never asked, so an
	// unanswered dialect gets the answer five of the six measured columns
	// give rather than a refusal (#1563).
	// unanswered JobsListsWhatChangedSinceTheLastReport: `jobs -n` is not a
	// letter this shell has — it is not in JobsOptions — so the axis is
	// never consulted here. ksh93 is the one column with the letter, and
	// bash's letter of the same name is a different question that stays
	// unimplemented (#3390).
	s.JobsListFinishedJobs = interp.Yes
	// Every ended job is listed as ended, monitor or no monitor.
	s.EndedJobIsListedAsRunningWithoutTheMonitor = interp.No
	s.JobsOptions = "lp"
	s.JobsPidsOnlyOption = interp.Yes
	s.JobsShowBackgroundCommand = interp.No
	s.AnnouncesBackgroundJob = interp.No
	s.AnnouncesBackgroundJobWithoutTheMonitor = interp.No
	// The hole gets refilled: measured 2026-09-12 in the container, three jobs
	// with the middle one killed and reaped and then a fourth started, `jobs
	// %2` answers 0 and there is no `%4`. It shares dash's other half too —
	// `jobs %2` is 0 *before* the new job as well, so the dead job kept the
	// slot until something reused it.
	s.NextJobNumberRefillsAHole = interp.Yes
	// It has somebody to tell on the named-script route and tells them exactly
	// one thing, which is dash's shape as well: measured 2026-09-12 in the
	// container on `-i script.sh` with a job held open on a fifo, `[1]+  Done`
	// as it ends and nothing at all as it starts. The start is
	// AnnouncesBackgroundJob, answered No just above.
	s.InteractiveScriptAnnouncesJobs = interp.Yes
	// And nothing whatever on the other interactive route. The same program
	// under `-i -c` writes neither line, where bash, ksh93 and zsh all write at
	// least the start — and it is a real answer rather than an absent one,
	// since `$-` there is `cmi`, interactive with the monitor running.
	s.InteractiveCommandStringAnnouncesJobs = interp.No
	// Where it does speak it speaks at once: the `Done` row above is written
	// between the commands of a script, on a route that draws no prompt.
	s.FinishedJobNoticeNeedsAPrompt = interp.No
	s.ReportsACommandKilledBySignal = interp.Yes
	s.ReportsAnyKilledPipelineElement = interp.Yes
	s.ReportsAKilledCommandInACommandSubstitution = interp.Yes
	// And the one place BusyBox ash leaves the standard's reading to stand
	// with bash rather than with dash, which is what keeps this axis from
	// being "bash against the rest". Measured 2026-09-15 on BusyBox v1.37.0,
	// `set -e; echo "end[$(false; echo no)]"` is `end[no]`, and `$-` inside
	// the body carries no `e`. There is no posix mode and no option name
	// here, so this is where the shell stays.
	s.ErrExitEntersACommandSubstitution = interp.No
	s.ChildInterruptEndsTheScript = interp.No
	s.SubshellRunsOnAfterSignalingTheShell = interp.Yes
	// Only numbers and the `%%`, `%+`, `%-` forms resolve; `%name` is a job
	// that is not there, and `wait`, `jobs`, `fg` and `bg` each say so.
	s.JobSpecsByName = interp.No
	s.WaitReadsOptions = interp.Yes
	s.WaitReportsAMissingJob = interp.Yes
	// And a job it has already reported stays waitable by its process id.
	s.WaitRemembersAReapedJob = interp.Yes
	// `sh -c 'kill -TERM $$' &` then `wait $!` writes `Terminated` here,
	// which is the sentence a killed foreground command earns.
	s.WaitReportsTheSignalThatEndedTheJob = interp.Yes
	// `wait -n` is taken here and is not bash's `-n`. Measured 2026-09-17 in
	// the pinned image, BusyBox v1.37.0, with the jobs' exit statuses as the
	// discriminator — a probe using bare `sleep` jobs cannot tell the two
	// readings apart, which is the blind probe #3226 is about:
	//
	//	jobs 0 then 7    0 after 1s     bash: 0 after 1s
	//	jobs 7 then 0    0 after 2s     bash: 7 after 1s
	//	one job, 7       129 after 1s   bash: 7 after 1s
	//	jobs 0, 0, 7     0 after 1s
	//	no jobs at all   0              bash: 127
	//
	// So the jobs are waited out in order and the first to exit **0** ends
	// the wait; running out without one is 129, a constant rather than any
	// job's status. With operands the letter changes nothing: `wait -n p1
	// p2` waits both out at the last one's status, exactly as `wait p1 p2`
	// does. `-n` is also the only letter this `wait` takes — `-q` and `-zz`
	// are `illegal option` at 2, and so is a bare word (#3245).
	s.WaitNextJob = interp.WaitNextJobFirstToSucceed
	// Nor `-p`: `wait: illegal option -p`, BusyBox v1.37.0 in the pinned
	// image. Measured 2026-09-13.
	s.WaitPNamesTheFinishedJob = interp.No
	s.WaitForAJobFailsWhenInterrupted = interp.No

	// ---- control flow and redirection ----

	// `break` in a function reaches the caller's loop: `f(){ break; }; for i
	// in 1 2; do f; echo "i=$i"; done` writes nothing at all, where dash
	// writes both rounds. A call is not a boundary here and a subshell is,
	// which is the opposite pairing from dash's on the first half.
	s.FunctionCallIsALoopControlBoundary = interp.No
	s.SubshellIsALoopControlBoundary = interp.No
	// A definition whose name was not written bare binds nothing and says
	// nothing about it: `'f'() { echo body; }` is status 0 with an empty
	// stderr, `f` after it is `f: not found` at 127, and `g() { echo old;
	// }; 'g'() { echo new; }; g` still prints `old`, so an existing
	// definition is not replaced either. Measured 2026-09-13 in the pinned
	// alpine image; syntax.Dialect.FunctionNameIsAnyBareWord above is which
	// words reach this and interp.FuncNameDefinesNothing is the answer.
	s.FunctionNameWhenTheDefinitionRuns = interp.FuncNameDefinesNothing
	// `break` with no loop around it is ignored, silently: `echo t; break;
	// echo after` prints both and ends at 0.
	s.LoopControlOutsideALoopIsFatal = interp.No
	// The count is read first, as in dash: `break abc` outside a loop is
	// `Illegal number: abc`, measured in the pinned alpine image.
	s.LoopControlPlaceIsJudgedBeforeTheCount = interp.No
	s.ReturnOutsideAFunctionIsRefused = interp.No
	s.StartupFileReturnCarriesItsArgument = interp.Yes
	// `. ` with no operand is refused at 2, where dash does nothing and
	// reports success. A directory operand is no error in either.
	s.DotWithNoOperandIsAnError = interp.Yes
	// And it costs the builtin 2 and the script nothing. Measured 2026-09-18
	// in the digest-pinned alpine image, BusyBox v1.37.0, from a script file
	// with a line after it: `.` and `source` with no operand each leave 2
	// behind, write nothing at all, and the next line runs — where the same
	// shell's `.` on a file it cannot open does end the script. So the two
	// fatalities are two questions in this column, and this site was reading
	// the other one (#3277).
	s.DotWithNoOperandIsFatal = interp.No
	s.DotReadsOptions = interp.Yes
	// `eval` reads none, unlike `.` above — measured against BusyBox
	// 1.37.0, where `eval -- echo hi` is `eval: --: not found` at 127.
	s.EvalOptions = interp.EvalReadsNoOptions
	s.DotTakesTheSearchPathOption = interp.No
	s.DotDirectoryOperandIsAnError = interp.No
	// An operand with no slash that PATH does not have is looked for in the
	// current directory, which bash does and dash, zsh and ksh93 do not.
	// Measured 2026-09-16 in the pinned alpine image with the same basename
	// in both places: PATH wins where it has the name — that half is
	// unanimous — and where it has not, `. cwdlib.sh` runs the copy beside
	// the script here and is `not found` in dash. Inherited from the preset
	// as `No` until now (#3248's class).
	s.DotFallsBackToCurrentDirectory = interp.Yes
	// Words after the filename become the sourced file's own positional
	// parameters, and the caller's come back afterwards. This is the panel's
	// six-to-one split rather than its sibling's answer: dash alone ignores
	// them, and taking the preset's `No` here put this shell on the wrong
	// side of it (#3248). Measured 2026-09-16 in the pinned alpine image,
	// BusyBox v1.37.0: with `set -- outer1 outer2 outer3` and a body of
	// `echo "[$1][$2][$#]"`, `. ./sub.sh arg` writes `[arg][][1]` and the
	// line after it writes `[outer1][outer2][3]`. dash writes the caller's
	// three at both sites.
	s.DotPassesArguments = interp.Yes
	// Only the last of several targets is used: `echo hi >a >b` leaves a
	// empty.
	s.RedirectsUseEveryTarget = interp.No
	// A redirection a special builtin cannot make ends the script: `exec
	// 3>/nope/x` stops at 1.
	s.RedirectErrorOnSpecialBuiltinFatal = interp.Yes
	s.DuplicationTargetError = interp.DuplicationTargetErrorEndsTheShell
	// A duplication target wider than one digit is not refused while
	// parsing: `echo hi >&10` reaches the kernel and comes back `dup2(10,1):
	// Bad file descriptor`, where dash refuses the word outright.
	s.MultiDigitDuplicationTargetIsAnError = interp.No
	// And `>&word` names a file rather than being refused: `f=/tmp/gw; echo
	// hi >&$f` writes the file. dash is the panel's holdout on this.
	s.GreatAmpTarget = interp.GreatAmpTargetNamesAFile
	// BusyBox ash has no move operator either, and answers `redir error` at
	// 2 — its own sentence for any word after `<&` that is not a descriptor.
	s.FdMove = interp.FdMoveIsNotAnOperator
	s.RedirectTargetIsAnOrdinaryWord = interp.No
	// And no pathname expansion, as in dash and ksh93: `cat < only-*.txt`
	// is `can't open only-*.txt: no such file` with the match sitting there.
	// Measured in the pinned 1.37.0 image, 2026-09-16 (#3207).
	s.RedirectTargetTakesPathnameExpansion = interp.No
	// A here-document body and a redirection target are both expanded in the
	// shell rather than in the process the redirection is for, so what they
	// assign is still there afterwards: `cat /dev/null > "${u:=made}"` leaves
	// `u` set.
	s.HeredocExpandsInTheCommandsProcess = interp.No
	s.RedirectTargetExpandsInTheCommandsProcess = interp.No
	// A descriptor number the process cannot hold is not checked before the
	// open.
	s.FdNumberBoundedByOpenFileLimit = interp.No
	// No descriptor-number ceiling of this shell's own (#3210), a `-t` that
	// is refused rather than evaluated — `read: invalid timeout` at 2
	// (#3209) — and a `-v` echo that writes a line as it was read (#3130).
	s.DescriptorNumberCeiling = interp.NoDescriptorNumberCeiling
	s.VerboseEchoAddsAMissingNewline = interp.No
	s.ReadTimeoutOperandIsArithmetic = interp.No
	// `[[ ]]` is a builtin here rather than a keyword — `type '[['` answers
	// `[[ is a shell builtin` — so an unknown option inside it is a status
	// rather than a parse failure.
	s.UnknownConditionOptionIsAStatus = interp.Yes
	s.UnsetFunctionChecksTheName = interp.No
	s.UnsetFunctionReportsMissing = interp.No
	s.UnsetReachesTheFunctionTable = interp.No
	s.StdinProgramReadInBlocks = false
	s.StdinOptionNamesTheOperands = interp.No
	s.LoneDashIsAnOption = interp.No
	// A lone `+` is a name to `export`: `export +` is `+: bad variable name`.
	s.SignAloneIsAnOptionWordToExport = interp.No
	s.ReadTrailingEscapedSeparator = interp.ReadTrailingEscapedSeparatorTrimmed
	s.ReadTrailingWhitespaceEndsAField = interp.No
	s.ReadNoFieldsIsOneEmptyElement = interp.No
	s.ReadRefusesABadNameBeforeReading = interp.No
	s.BadNameDeclaresTheOperandsAfterIt = interp.No
	s.InteractiveSelectsEmacs = interp.No

	// ---- axes this dialect did not answer, swept for and measured ----
	//
	// #2272 was reported as one row — `read -t` refusing in the shipped
	// binary — and the row was the symptom of a shape. A `Semantics` axis
	// added after a dialect is written is unanswered there, and an
	// unanswered axis *refuses at run time* while `go test ./...` stays
	// green, so the whole set is swept rather than the one that was
	// noticed: every corpus snippet was run through this shell's binary and
	// every "no dialect was chosen" collected, which found nineteen sites
	// and not one — and then twenty, because answering one uncovers the
	// next question on the same path, so the sweep was re-run until it
	// stopped moving.
	//
	// Measured the same way the rest of this file was: BusyBox v1.37.0's
	// `/bin/ash` in the `alpine:3` image, 2026-09-12, each probe being the
	// corpus case that reaches the axis. The three the sweep found and this
	// block does *not* answer are named at the end, with what they answered
	// and why a value cannot be written yet — read those as open questions,
	// which is what docs/spec/ash.md says the absence of a measurement
	// means here.

	// `echo -e -E 'm\tn'` writes a tab, and so does `echo -E -e 'm\tn'`:
	// `-e` wins whichever way round the two are written, so the last flag
	// does not decide. bash is the panel's shell that lets it (#2272).
	s.EchoLastEscapeFlagWins = interp.No

	// `read -t 0` polls. `printf "a\nb\n" > f; exec < f; read -t 0 v` is
	// status 0 with `v` empty and the *next* `read` still finds `a`, so it
	// answered whether input was waiting and consumed none of it; on an
	// empty file it is status 0 as well, the end of a stream being ready to
	// a shell that only asks. bash's reading (#2272).
	s.ReadZeroTimeout = interp.ReadZeroTimeoutPolls
	// And a non-zero `-t` bounds the whole read rather than the wait for the
	// first byte. Measured with the case the axis was written for: a byte
	// into a fifo at once and the rest of the line 0.4s later, under `read
	// -t 0.1`, is status 1 with the variable left alone — where zsh answers
	// 0 with the whole line. This is the row #2272 was filed on (#644).
	s.ReadTimeoutBoundsReadability = interp.No
	// An expired `-t` touches no name, so the variable keeps what it held —
	// and this axis only became reachable once the one above was answered,
	// which is the layering the sweep had to be re-run to see. Measured with
	// the probe ReadTimeoutKeepsWhatArrived documents, half a line and then a
	// stall: `{ printf part; sleep 0.5; printf 'ial\n'; } | { v=old; read -t
	// 0.2 v; echo "$? [$v]"; }` is `1 [old]` here, against bash 5.3's `142
	// [part]`. ksh93's row (#2272).
	s.ReadTimeoutKeepsWhatArrived = interp.No
	// A count does not stop `read` judging the names after the first:
	// `printf 'XYZW\n' | read -n 3 a 1bad b` complains `read: '1bad': bad
	// variable name` at 1, exactly as it does without the count, and `-n 3`
	// really is a count here — `read -n 3 v` of `abcdef` leaves `abc`. ksh93
	// is the shell a count quiets (#2272).
	s.ReadCountJudgesTheNamesAfterTheFirst = interp.Yes

	// `set -e` stops for a failure only pipefail saw: `set -eo pipefail;
	// false | true; echo reached` reaches nothing and the shell ends at 1,
	// where a plain `set -e; false | true` reaches the echo. ksh93 is the
	// column that runs on (#2272).
	s.ErrexitSeesPipefailFailure = interp.Yes
	// `time` changes nothing about the judging here: measured 2026-09-18 in
	// the pinned image, `set -e; time false; printf survived` stops and
	// `trap 'printf E' ERR; time false` writes E.
	s.TimedCommandIsJudged = interp.Yes
	// unanswered CompoundRedirectionFailureIsJudged: BusyBox ash 1.37.0 gives
	// no rule to record. Measured 2026-09-18 in the pinned image, one script
	// file per row with `> /nonexistent/x`, it survives for a group, an `if`
	// and a `case` and stops for a `for`, a `while` and a subshell, and its
	// ERR rows do not agree with its `set -e` rows either. That is the
	// absence of an answer rather than a third one, and a dialect must not be
	// given a rule its reference does not follow. Unanswered judges, which is
	// what the other three columns with the construct do.
	// A subshell as the last element of a pipeline is judged once, as it is
	// everywhere but bash.
	s.ASubshellAsTheLastPipelineElementJudgesItself = interp.No
	// The status pipefail hands back for an element a signal killed is the
	// ordinary 128-plus-the-signal and not the bare number: a `{ echo "$v";
	// } | true` over a value grown past the pipe buffer is 141, and so is
	// `yes | head -1`. ksh93's 13 is the other answer (#2272).
	s.PipefailSubstitutesTheBareSignal = interp.No

	// `${1:=abc}` does not assign to a positional: after `set --` it is `1:
	// bad variable name` and the script ends. zsh alone assigns (#2272).
	s.AssignThroughExpansionMayNameAPositional = interp.No
	// A quoted replacement operand's own quotes quote, and are removed:
	// `s=xay; v=VAL; echo "${s/a/'$v'}"` is `x$vy` rather than `x'VAL'y`, so
	// the single quotes kept `$v` from expanding and then went. The
	// backslash row agrees — `\q`, `\{`, `\\`, `\"`, `\}` and `\$v` all lose
	// the backslash — which is bash 5.3's and ksh93's reading (#2272).
	s.ReplacementOperandTakesTheEnclosingQuoting = interp.No
	// An empty pattern in a span replacement matches nothing, whatever the
	// value holds: `v=abc; e=` makes `${v///X}`, `${v//$e/X}`, `${e///X}`
	// and `${e//x/X}` come to `abc`, `abc`, empty and empty. ksh93 replaces
	// in the third and zsh in every position (#2272).
	s.EmptyReplacementPattern = interp.EmptyReplacementPatternMatchesNothing
	// And no anchors at all behind the `/`, which is the half
	// syntax.Dialect.ParamSubstitution's comment calls one feature with the
	// replacement: `#` and `%` written there are the pattern's own first
	// character. Measured 2026-09-16 in the pinned alpine image under
	// `--init`, with `w='x#ay%bz'`: `${w/#a/Q}` is `xQy%bz` — the `#a` found
	// *inside* the value — and `${w/%b/Q}` is `x#ayQz`, where the other six
	// columns leave both alone. `${v/b/X}` and `${v//b/X}` are the controls
	// and are right here, so this is "no anchor" and not "no replacement"
	// (#3272).
	//
	// unanswered AnchoredEmptyReplacementPattern: that axis is what an
	// *anchored* empty pattern matches, and no anchor is ever read here, so
	// nothing in this shell can reach it.
	//
	// unanswered GlobalReplacementAnchors: that axis is whether the anchor
	// is read after the global `//` as well, and it is asked only of a
	// column that reads one after a single `/` — which the line below says
	// this shell does not. Measured 2026-09-16 in the pinned alpine image
	// under `--init`: `${w//#a/Q}` on `x#ay%bz` is `xQy%bz`, the same
	// reading the single spelling gives here (#3307).
	s.ReplacementAnchors = interp.No

	// `$'\q\8'` keeps both characters, which is bash's answer and not the
	// dropping one ksh93 and zsh share (#2272).
	s.DollarSingleUnknownEscape = interp.DollarSingleUnknownKeepsBackslash
	// There is no `\c` inside `$'…'` at all: `$'\cA\cz'` is the six
	// characters as written. zsh is the other column with none (#2272).
	s.DollarSingleBackslashC = interp.DollarSingleControlAbsent
	// And no caret or meta spelling either: `$'\C-A'`, `$'\CA'`, `$'\M-x'`
	// and `$'\M-\C-?'` are all kept as written, so the escape vocabulary
	// this shell's `$'…'` has is neither zsh's nor ksh93's (#2272).
	s.DollarSingleCaretMeta = interp.DollarSingleCaretMetaAbsent
	// And three more escapes this shell's `$'…'` does not have, where the
	// other six columns do. Measured 2026-09-16 by `od` in the pinned alpine
	// image, under `--init`: `$'\e'` is `5c 65`, `$'\E'` is `5c 45`, `$'\?'`
	// is `5c 3f`, and both $'\u0041' and $'\U00000041' keep the backslash and every
	// digit — where `$'\41'` is `41` and `$'\t'` is `09`, the controls that
	// say the construct works here at all.
	//
	// The `\e` row is the one to be careful with, because **this shell
	// answers the two sites differently**: `echo -e 'a\eZ'` writes an escape
	// character here (#3226, EchoExpandsEscEscape = Yes) and `$'a\eZ'` does
	// not. A probe at one site decides nothing at the other, which is the
	// same warning the `printf %b` comment above carries about ksh93 (#3270).
	s.DollarSingleEscEscape = interp.No
	s.DollarSingleQuestionEscape = interp.No
	s.DollarSingleUnicodeEscapes = interp.No

	// `$((2**-1))` is `exponent less than 0` — no float answer, which is
	// bash's side of the split (#2272).
	// A name-shaped value is re-read as an expression, and it recurses as
	// far as the values lead: `y=5; x=y` makes `$((x+1))` 6, and `y=z; z=7;
	// x=y` makes it 8. dash is the panel's only holdout — `Illegal number:
	// y` there — and taking the preset's `No` put this shell beside it
	// (#3248's class). Measured 2026-09-16 in the pinned alpine image,
	// BusyBox v1.37.0, both depths.
	s.ArithNameValueRecurses = interp.Yes
	// And what stops it is a *cycle* rather than a depth: a chain of sixty
	// names ending in a number is that number here, and a chain of three
	// hundred is too, where the other three columns refuse both. Only a name
	// that comes back on itself is refused, and the sentence names nothing.
	// Measured 2026-09-18 in the pinned image (#3416).
	s.ArithRecursionBound = interp.ArithRecursionBoundedByACycle
	// And a name reached that way which is unset is a zero, as it is in bash
	// and zsh; ksh93 refuses it with the `set -u` sentence. Left unanswered
	// when the recursion above was answered, so every such expansion was
	// refused as a question no dialect had chosen — `x=abc; $((x+1))` with
	// `abc` unset stopped the script where BusyBox prints 1. Measured
	// 2026-09-16 in the pinned alpine image, BusyBox v1.37.0: `$((x))` 0,
	// `$((x + 1))` 1, `$(((x) * 2))` 0, and two names deep 0.
	s.ArithRecursedNameMustBeSet = interp.No
	// And `set -u` does not reach arithmetic at all: `set -u; : $((b))` with
	// `b` unset is zero and the script runs on, where the four big shells
	// refuse it. Measured 2026-09-18 in the pinned alpine image, BusyBox
	// v1.37.0 — `$((b))` prints 0 at status 0 with the option on. dash is
	// the other column that reads it this way (#3574).
	s.ArithUnsetNameUnderNounsetIsRefused = interp.No
	s.ArithNegativeExponentIsError = interp.Yes

	// unanswered TraceArrayLiteralShowsTheExpandedElements: no array literal
	// here either — `a=(1 2)` is a syntax error — so nothing of this shape
	// is ever traced (#1959).
	// unanswered TraceSubscriptedArrayLiteralIsElementAssignments: the
	// subscripted shape is the same literal and does not parse here either.
	// unanswered TraceElementSubscriptIsEvaluated: and no subscript, so
	// `a[1]=v` names a variable spelled `a[1]` and there is nothing to
	// resolve.
	// unanswered ArithFloatOverflowIsZero: no floats here either, so
	// `$((1e400))` is a syntax error rather than a number out of range and
	// the axis is unreachable.

	// The right operand of `&&` and `||` is evaluated even when the left has
	// already decided the answer, so an assignment written there takes
	// effect: `x=0; $((0 && (x=9)))` leaves x at 9 and `y=0; $((1 || (y=8)))`
	// leaves y at 8. The value is still the operator's — 0 and 1 — because it
	// cannot be anything else.
	//
	// The sole holdout in the panel, and the first thing the ash column of
	// `make suite` found on the day it could run at all (#2605). Its
	// conditional does *not* do this — `w=5; $((0 ? (w=1) : 2))` leaves w at
	// 5 — so this is a fact about the two logical operators rather than about
	// this shell evaluating everything.
	s.ArithShortCircuitEvaluatesTheRightOperand = interp.Yes
	// BusyBox answers an over-large numeral as bash does, by letting the
	// unsigned word go round: `$(( 10000000000000000000 ))` is
	// -8446744073709551616 and `$(( 0xffffffffffffffff ))` is -1. Measured in
	// the pinned 1.37.0 image (#3202) — the column that is easiest to assume
	// follows dash here, and does not.
	s.ArithNumeralPastTheWord = interp.NumeralPastTheWordWraps
	// And the same numeral out of a variable reads identically; only dash
	// parts the two.
	s.ArithStoredNumeralPastTheWordIsRefused = interp.No

	// A declaration does not shadow a readonly: `readonly x=1; f() { local
	// x=2; }; f` is `local: line 1: x: is read only` and the script ends,
	// which is dash's and bash's answer rather than ksh93's and zsh's
	// (#2272).
	s.DeclarationMayShadowAReadonly = interp.No
	// A valueless declaration of a name its own scope already holds lists
	// nothing: `f() { local v=1; local v; }` is silent at 0, and the value
	// stays — `local FOO=x; local FOO` still reads `x`. zsh is the column
	// that lists (#2272).
	s.ValuelessDeclarationOfAHeldNameListsIt = interp.No

	// `jobs -p` does not finish a job the way a state listing does: after a
	// background `sleep` has ended, `jobs -p` prints the id and the next
	// bare `jobs` still reports it `Done`. ksh93 is the column that forgets
	// it there (#2272). The plain `jobs -p` in the issue answered 0 for
	// this reason — the axis is only reached once there is a finished job to
	// list.
	s.PidListingFinishesWithAJob = interp.No

	// The NUL an escape produced is dropped: `x=$'a\0b'` leaves `ab` at
	// length 2, so the byte is neither the end of the span (bash and ksh93,
	// length 1) nor a character of it (zsh, length 3). The octal and hex
	// spellings agree — `$'a\000b'` and `$'a\x00b'` are 2 as well — and the
	// rest of the span still follows, `printf '[%s]' $'a\0b'ccc` being
	// `[abccc]`. Measured 2026-09-12 in the pinned alpine image.
	//
	// This is the value that had nowhere to go while the axis was an
	// `Answer`, which is what #2276 widened. It is worth reading as a
	// warning about axis *types* rather than about this shell: two columns
	// were measured, "does the NUL truncate" looked like the question, and
	// the third column could then only be recorded by being wrong.
	s.DollarSingleNul = interp.DollarSingleNulIsDropped

	// The two hexadecimal-escape axes, measured 2026-09-12 in the pinned
	// alpine image and with probes chosen so the readings cannot agree.
	// They were left unanswered for want of a binary — the note that said
	// so is gone with them — and the probe that reads like the obvious one
	// is the one to avoid: `$'a\x00b'` is length 2 under **both** readings
	// here, because a run of three digits read short gives a NUL this shell
	// drops and read long gives U+000B, one byte either way.
	//
	//	printf '[%s]' $'\x414'   [A4]      two digits and the rest is text
	//	printf '[%s]' $'\xzz'    [\xzz]    kept as it was written
	//	printf '[%s]' $'\x'      [\x]
	//	printf '[%s]' $'\uZ'     [\uZ]
	//
	// So both are bash's answer and neither is ksh93's, which takes every
	// digit and reads a code point, or zsh's, which reads a zero byte from
	// a digitless escape. The three corpus rows that reach them already
	// record this column, so the values are graded by rows that exist
	// (#554).
	s.DollarSingleHexReadsEveryDigit = interp.No
	s.DollarSingleDigitlessEscapeIsAZeroByte = interp.No
	// A three-digit octal escape past 255 is the first two digits' byte here
	// and the third digit is dropped: `$'\401'` is a space rather than 01,
	// and `$'\4001'` is a space then a `1`. Measured 2026-09-18 by `od`
	// (#3415).
	s.DollarSingleOctalPastAByteDropsTheLastDigit = interp.Yes

	// The ones the sweep reached and this file deliberately leaves unanswered,
	// each with what BusyBox answered and what stands in the way of writing
	// it down. None is a guess deferred; each is a measurement the vector
	// cannot yet hold.
	//
	// Each is written as an `unanswered <axis>:` line, which is the
	// spelling internal/axissweep reads back (#2340). The coverage check
	// prints them under the entry they answer, so what this dialect has not
	// measured is stated by the instrument rather than only here — and a
	// value quietly appearing for one of them, copied from a neighbor to
	// quiet the refusal, fails that check instead of passing quietly.
	//
	// unanswered BuiltinReadsOptions: BusyBox ash has no `builtin` for a
	// dash-word to reach (#3217).
	//
	// unanswered BraceRescanEntersFailedGroup: this shell has no brace
	// expansion either, so nothing ever resumes a scan — `@{x}{a,b}@` is
	// one word, and the nine `BraceRange…` and `BraceCharRange…` axes are
	// unanswered beside it for the same reason.
	//
	// unanswered StoreRefusalThroughPrintfLeavesZero: this shell's
	// `printf` has no `-v`, so no store is reached through it.
	//
	// unanswered StoreRefusalOfADeclaredElementLeavesZero:
	// no declaration utility and no array literal here either, so neither
	// route reaches a store that could refuse an element. Measured
	// 2026-09-14 in the pinned image, `a=(x y); typeset "a[0]"=v` is `syntax
	// error: unexpected "("` at 2 by both routes — the same wall dash meets
	// (#1770).
	// unanswered ArrayLiteralOperandRetypesAFrozenScalar: no declaration
	// utility and no array literal here either. Measured 2026-09-12 on
	// BusyBox, `readonly q=1; typeset -g q=(b)` is `syntax error: unexpected
	// "("` at 2 — the same wall dash meets, and for the same reason (#2250).
	// unanswered NumericTypeLetterRetypesAFrozenName: no numeric type letter
	// either. Measured 2026-09-12 on BusyBox in a container, `typeset` is
	// `not found` and `export -i q=4` is `illegal option -i` — the same two
	// walls dash meets (#2539).
	// unanswered AttributeOverAFrozenNameIsRefused: no declaration command
	// here either. Measured 2026-09-12 on BusyBox in a container, `typeset`
	// is `not found` — the same wall dash meets (#2561).
	// unanswered UpperCaseLetterBesideANumericTypeLetterRecordsNothing and
	// unanswered TwoCaseLettersOnOneDeclarationCancel: no declaration
	// command here either. Measured 2026-09-12 on BusyBox in a container,
	// `typeset -lu z=Ab` is `typeset: not found` (#2541).
	// unanswered TableUnderAnArrayLiteralDeclaration and
	// unanswered ArrayUnderATableLiteralDeclaration: no arrays and no
	// declaration word here either, so neither half of the question can be
	// put. Measured 2026-09-12 on BusyBox, `typeset -A h` is `typeset: not
	// found` and `h=(x)` is `syntax error: unexpected "("` (#2287).
	// unanswered WholeArraySubscriptAssigningAnArray and
	// unanswered WholeArraySubscriptAssigningATable: no arrays here either,
	// so the same wall. Measured 2026-09-12 on BusyBox, `x=(p q)` is `syntax
	// error: unexpected "("` and `x[@]=Z` alone is `x[@]=Z: not found`
	// (#2285).
	//
	// unanswered EarlierDeclarationLetterBlocksALaterPlus: there is no
	// declaration command to write the letter on. `typeset` is not a
	// builtin here and `integer` is not a word, so neither sign of `-i`
	// can be put to this shell at all (#2345).
	// unanswered ReadonlyRecordsTheCompoundAttribute: this shell has no
	// letter to ask it with, which is a different thing from having no
	// answer and is the whole of #2277. `readonly -a a` is `readonly:
	// illegal option -a` and there is no `typeset` at all, so the question
	// cannot be put here rather than being left open. Semantics.
	// ReadonlyOptions is `p` in this dialect now, so the refusal a script
	// meets is the option's and not an axis's.
	//
	// Diagnostics.UlimitListing was the third of these and is no longer one
	// (#2278). It was held open because five of BusyBox's fifteen rows name
	// limits only Linux has, and a table measured on Linux could not say
	// what a macOS build should print. The answer turned out not to need a
	// BusyBox on a BSD, which does not exist to be measured: bash, zsh and
	// dash on macOS each print their own Linux table without the rows for
	// limits this kernel lacks, byte-identical otherwise, and the field
	// width is a literal rather than one computed from the rows present. So
	// the row a kernel cannot answer is dropped and nothing else moves —
	// which is Runner.HasRlimit, and a fact about the platform rather than a
	// layout anybody invented. The reasoning is written out where the table
	// is.

	return s
}

// Diagnostics is how ash reports failure.
//
// Two shapes run through the whole table and are worth reading first. A
// message the shell itself speaks carries no line under `-c` and a `line N`
// in a script — `ash: nosuchcmd: not found` against `s.sh: line 3:
// nosuchcmd: not found` — which is Location and ScriptLocation below. And the
// wordings are lower-case where dash's are capitalized: `syntax error:
// unexpected "("` against `Syntax error: "(" unexpected`, which is the single
// largest family of differences between the two siblings and accounts for
// 458 rows of the corpus on its own.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		// An assignment written in front of a command is traced on the
		// command's own line, exactly as the script spells it: `+ A=3 f zz`,
		// where bash writes two lines and ksh93 writes the command first.
		TracePrefixAssignment: interp.TracePrefixOnTheCommandLine,
		// Measured on `-c`, on a script file and on standard input: only the
		// script route names a line. `ash -c 'echo a; nosuchcmd'` is `ash:
		// nosuchcmd: not found` however deep the failure is, and the same two
		// lines in a file are `s.sh: line 2: nosuchcmd: not found`.
		Location:       interp.LocationNameOnly,
		ScriptLocation: interp.LocationLineWord,
		StdinLocation:  interp.LocationNameOnly,
		// A builtin names itself between the script and the line, and on
		// every route rather than only where the shell's own message carries
		// a line. Measured 2026-09-14, BusyBox v1.37.0 in the pinned image:
		//
		//	a script     /s.sh: export: line 1: illegal option -q
		//	-c           /bin/ash: export: line 0: illegal option -q
		//	standard in  /bin/ash: shift: line 1: Illegal number: -1
		//	a prompt     /bin/ash: shift: Illegal number: -1
		//
		// so the name is unconditional and the *line* is what the route
		// decides — which is why the three builtin locations below say
		// something the plain ones do not: `-c` and standard input name no
		// line for the shell's own failures and both name one here, and the
		// prompt names neither.
		//
		// It is a rule rather than a list, and the same rule zsh follows:
		// `export`, `set`, `unset`, `readonly`, `trap`, `shift`, `.`,
		// `return`, `break`, `continue`, `exit`, `cd`, `read`, `eval`,
		// `umask`, `getopts`, `hash`, `local`, `wait` and `command` were
		// measured and every one names itself. What stays bare is equally
		// consistent: a command that was not found, a parse failure the
		// shell reads for itself, an unset parameter, a division by zero, a
		// redirection that would not open, and an assignment to a readonly
		// name. Those are the shell's own failures and not a builtin's
		// (#2761).
		NamesBuiltinInLocation: true,
		// The three routes where a builtin's location is not the shell's.
		// LocationNone would mean "the same as Location", which is this
		// shell's name and no line — and a builtin does carry one on the two
		// routes where the shell does not.
		BuiltinLocation:       interp.LocationLineWord,
		StdinBuiltinLocation:  interp.LocationLineWord,
		PromptBuiltinLocation: interp.LocationNameOnly,
		// And the line belongs to the builtin's own complaint rather than to
		// anything a builtin was merely involved in: `ash -c 'read x <
		// /nofile'` is `ash: can't open /nofile: no such file`, with neither
		// the builtin's name nor a line, where `ash -c 'shift -1'` has both.
		// See interp.Diagnostics.BuiltinLocationIsTheSpeakersOnly.
		BuiltinLocationIsTheSpeakersOnly: true,
		// And the `-c` route counts its lines from 0, which the two
		// locations above are the only way to see: the shell's own
		// diagnostics carry no line there at all. `$LINENO` moves with it,
		// so it is the counter and not the rendering — see the field for the
		// panel's table (#2799).
		CommandStringLinesFromZero: true,
		// A sourced file and an `eval` are both named between the shell and
		// the line, which is bash's and ksh93's placing rather than dash's:
		// `ash: ./p.sh: line 3: NOPE: parameter not set`, and `ash: eval:
		// line 0: nosuchcmd: not found`.
		SourceFileNaming: interp.SourceBeforeLocation,
		EvalNaming:       interp.SourceBeforeLocation,
		// And the name is written while the text is *running* and not only
		// when it fails to parse, which is the fourth arrangement of these
		// fields: dash names the text after the location, bash and zsh put
		// it where the shell's own name goes, ksh93 renders the whole chain,
		// and this shell writes one name in front of the location. Measured
		// 2026-09-12, BusyBox v1.37.0, `p.sh` holding `echo one` and
		// `echo $NOPE`:
		//
		//	ash -c 'set -u; . ./p.sh'    ash: ./p.sh: line 2: NOPE: …
		//	the same from a script       ./s.sh: ./p.sh: line 2: NOPE: …
		//	the dot inside a function    ./s.sh: ./p.sh: line 2: NOPE: …
		//	an eval from a script        ./s.sh: eval: line 11: NOPE: …
		//	a function defined in a
		//	  sourced file, called after ./s.sh: line 1: NOPE: …
		//
		// The last row is why this is the innermost text still being read
		// rather than a rule about function frames — see
		// interp.Runner.borrowedNameBefore, which is dash's rule with this
		// shell's placement.
		BorrowedTextIsNamedAtRunTime: true,
		// The line beside that name is written on every route, where the
		// shell's own text carries one only from a file. See
		// interp.Diagnostics.BorrowedLocation for the five rows.
		BorrowedLocation: interp.LocationLineWord,

		// The parse failures, in this shell's own order: the complaint first
		// and the token after it, all lower case.
		SyntaxUnexpected:         `syntax error: unexpected "%[1]s"`,
		SyntaxUnexpectedWord:     "syntax error: unexpected word",
		SyntaxUnexpectedNewline:  "syntax error: unexpected newline",
		SyntaxRedirectUnexpected: "syntax error: unexpected redirection",
		SyntaxExpecting:          ` (expecting "%[1]s")`,
		SyntaxError:              "syntax error: %[1]s",
		SyntaxErrorStatus:        2,
		ForName:                  "syntax error: bad for loop variable",
		Unterminated:             `syntax error: unexpected end of file (expecting "%[4]s")`,
		UnterminatedNoConstruct:  "syntax error: unexpected end of file",
		UnmatchedQuote:           "syntax error: unterminated quoted string",
		UnmatchedBackquote:       "syntax error: unterminated quoted string",
		UnmatchedCmdSubst:        `syntax error: unexpected end of file (expecting ")")`,
		// Backticks alone, exactly as dash does it: this shell numbers a
		// `$( … )` body from the file and a backquoted one from one.
		// Measured 2026-09-18 on BusyBox 1.37.0 in the pinned image, both
		// routes to the body and a script file each time —
		// `echo one` / ``echo `if; then :; fi` `` / `echo two` is
		// `line 1: syntax error: unexpected ";"` against `line 2` for the
		// `$( … )` spelling, and `x=`nosuchcmd`` is `line 1: nosuchcmd: not
		// found` against `line 2`. Both were `line 2` here (#2471).
		BackquotedSubstitutionRestartsLines: true,
		UnmatchedBraceSubst:                 "syntax error: missing '}'",
		UnmatchedArithSubst:                 "syntax error: missing '))'",
		BadSubstitution:                     "syntax error: bad substitution",

		// Arithmetic says one thing about every way an expression can be
		// wrong: `$((1 2))`, `$((1+))`, `$((08))`, `$(('a'))` and `$((0b101))`
		// are all `arithmetic syntax error`. So the reason is the whole of the
		// message and the expression is never quoted back, which is what the
		// bare `%[2]s` says.
		ArithError:            "%[2]s",
		ArithOperandExpected:  "arithmetic syntax error",
		ArithOperatorExpected: "arithmetic syntax error",
		DigitTooGreatForBase:  "arithmetic syntax error",
		ArithConditionalColon: "arithmetic syntax error",
		// The one arithmetic reason that is not `arithmetic syntax error`,
		// and it is `divide` where the substrate and three of the panel
		// write `division`. Measured 2026-09-14, `: $((1/0))` and `: $((1%0))`
		// alike: `/t.sh: line 1: divide by zero`, and the script ends at 2
		// (#2801).
		DivisionByZero: "divide by zero",
		// And the second reason that is not `arithmetic syntax error`. A name
		// resolved through its own value is refused only where the chain comes
		// back on itself — Semantics.ArithRecursionBound — and the sentence
		// names nothing, where the three columns that count frames each blame
		// a name. Measured 2026-09-18 in the pinned image, `x=x; echo $((x))`,
		// `a=b; b=a; echo $((a+1))` and `x=x; echo $((0 && x))` alike:
		// `/t.sh: line 1: expression recursion loop detected`, and the script
		// ends at 2 (#3416).
		ArithRecursionLimit: "expression recursion loop detected",

		// The command-resolution family. Neither a name nor a line in front
		// of the `not found`, which is the shape ksh93 uses too.
		// `trap` words a condition it does not know as bash does rather than
		// as dash does, which is the half of #2761 that is not a location:
		// `/s.sh: trap: line 1: NOSUCHSIG: invalid signal specification`
		// against dash's `bad trap`, and the same sentence for a number out
		// of range. Measured 2026-09-14 over `NOSUCHSIG`, `99` and a second
		// condition after a good one; the status is 1 in every row.
		TrapBadSignal: "trap: %[1]s: invalid signal specification",

		TypeKeyword:            "%[1]s is a shell keyword",
		TypeFunction:           "%[1]s is a function",
		TypeAlias:              "%[1]s is an alias for %[2]s",
		CommandVAlias:          "alias %[1]s=%[2]s",
		TypeNotFound:           "%[1]s: not found",
		TypeNotFoundUnprefixed: true,
		TypeNotFoundOnStdout:   true,
		CommandVNotFound:       "%[1]s: not found",
		TypeNotFoundStatus:     127,
		CannotExecute:          "%[1]s: %[2]s",
		ExecCannotExecute:      "%[1]s: %[2]s",
		ExecNotFound:           "%[1]s: not found",
		DirectoryReason:        "Permission denied",
		DirectoryOnPathStatus:  127,

		// `.` and the files it reads. The quotes around the name are this
		// shell's and are not decoration: `. nosuchfile` is `ash: .: line 0:
		// can't open 'nosuchfile': No such file or directory`, with the OS's
		// own text after the colon rather than dash's truncation of it.
		// No verb in either sentence: this shell puts the word the builtin
		// was invoked by in the *location* — `case.sh: source: line 1:` —
		// and writes nothing in front of the message. The `.` these two
		// opened with used to be taken back out again by
		// NamesBuiltinInLocation's strip, which matches the name the builtin
		// was invoked by, so it came off for `.` and stayed for `source`:
		// `source ./nope.sh` wrote `case.sh: source: line 1: .: can't open
		// …`. Measured 2026-09-18 in the digest-pinned alpine image, BusyBox
		// v1.37.0, under both spellings (#3277).
		//
		// DotNotFound is reached here and in that shell never is: BusyBox
		// does no PATH search for an operand with no slash — `. nosuchzz`
		// there is `can't open 'nosuchzz'` — where this shell searches and
		// then says the name was not found. That is a separate row and is
		// not this change.
		DotCannotOpen: "can't open '%[1]s': %[2]s",
		DotNotFound:   "%[1]s: not found",
		// And a missing operand is an error with no sentence for it, which
		// is the one place these two questions come apart in the panel. See
		// Diagnostics.DotNoOperandSilent.
		DotNoOperandSilent: true,

		// Redirection. One verb each way, and the OS's reason reworded: an
		// open that finds nothing is `no such file`, a create that cannot
		// make one is `nonexistent directory`.
		CannotOpen:            "can't open %[1]s: %[2]s",
		CannotCreate:          "can't create %[1]s: %[2]s",
		FileNotFound:          "no such file",
		DirectoryNotFound:     "nonexistent directory",
		RedirectFailureStatus: 1,

		// A word after `<&` or `>&` that names no descriptor, and this shell
		// has two sentences for it rather than one — the same split bash and
		// ksh93 make, the other way round. A word that came to something is
		// `redir error`, three words with nothing of the script in them; a
		// word that came to nothing is `syntax error: bad fd number`, which
		// is a parse failure's wording on a parse that succeeded. Measured
		// 2026-09-13: `echo A; echo hi 2>&qq` prints `A` and then the first,
		// and `<&""`, `>&""` and `<&$UNSET` all print the second.
		DuplicationTargetIsNotADescriptor: "redir error",
		EmptyDuplicationTarget:            "syntax error: bad fd number",

		// The declarations. One wording for all of them, naming the part in
		// front of any `=`.
		BuiltinBadName: map[string]string{
			"export":   "%[2]s: bad variable name",
			"readonly": "%[2]s: bad variable name",
			"unset":    "%[2]s: bad variable name",
			"local":    "%[2]s: bad variable name",
			"read":     "read: '%[2]s': bad variable name",
		},
		BuiltinBadNameStatus: 2,
		// `read` is the exception on both counts. Measured 2026-09-17 in the
		// pinned image: `read 1bad` is `ash: read: '1bad': bad variable
		// name` at **1**, where `unset 'a[0]'`, `export 'e[0]=x'`,
		// `readonly`, `local` and `getopts` each write `<file>: <builtin>:
		// line N: a[0]: bad variable name` at 2. So this one complaint is
		// the applet's — the shell's own name and nothing else, with the
		// builtin kept in the sentence — and it counts differently as well
		// (#3367).
		BuiltinBadNameNamesTheShellAlone: map[string]bool{"read": true},
		BuiltinBadNameStatusFor:          map[string]int{"read": 1},
		ReadonlyVariable:                 "%s: is read only",
		ReadonlyVariableInDeclaration:    "%[1]s: is read only",
		// `getopts` names itself for a refused write to one of the three
		// names it fills in: measured 2026-09-16 in the pinned image,
		// `readonly OPTARG; getopts a: o` is `ash: getopts: line 4: OPTARG:
		// is read only`.
		// And `read`, which names itself in the location the same way:
		// measured 2026-09-16 in the pinned image, `a=A; readonly a; printf
		// 'x\n' | read a` is `ash: read: line N: a: is read only`.
		ReadonlyRefusalNamesBuiltin: map[string]bool{"getopts": true, "read": true},
		UnsetReadonly:               "%s: is read only",
		LocalOutsideAFunction:       "not in a function",

		// The option refusals: lower case, and the letter alone.
		SetInvalidOptionName:   "illegal option -o %[1]s",
		SetInvalidOptionLetter: "illegal option -%[2]s",
		// A `--word` this front end could not place, and it is **not** the
		// letter's sentence one word over: measured 2026-09-16 on BusyBox ash
		// 1.37.0 in the pinned Alpine image, `ash -q` is `illegal option -q`
		// and `ash --badopt` is `bad option '--badopt'` — a different verb and
		// quotes around the word, both at status 2. `--xyz`, `--a` and
		// `--login=x` each echo whole, so the word is never trimmed.
		InvocationBadLongOption: "bad option '%[1]s'",
		// The split this pair of fields exists for. Measured 2026-09-13,
		// BusyBox v1.37.0: `set -o zzznosuch; echo "st=$?"` writes the
		// complaint, then `st=1`, and the script carries on; `set -Z` writes
		// its complaint and ends the script at 2. One shell, two spellings,
		// two answers — and the fatality splits with it, which
		// Semantics.BadSetOptionNameFatal and BadSetOptionLetterFatal hold
		// (#2629).
		SetInvalidOptionNameStatus:   1,
		SetInvalidOptionLetterStatus: 2,
		BuiltinBadOption:             "illegal option %[2]s",
		// Lower case, which is this shell's and not dash's: measured
		// 2026-09-17 in the pinned image, `read -d` is `no arg for -d
		// option` and so are `-p`, `-n`, `-t` and `-u`. `read` is the only
		// builtin here that reaches the sentence at all — every other
		// option-taking letter in the panel's own builtins answers something
		// else — so the whole of the measurement is those five (#3367).
		OptionNeedsArgument: "%[1]s: no arg for -%[2]s option",
		UlimitBadOption:     "ulimit: unrecognized option: %[1]s",
		// Written bare — no script, no line — and reported 1,
		// where `unset -Z`, `read -Z` and `trap -Z` in this same shell each
		// carry the script and the line and report 2. So it is this
		// builtin's exception rather than a house style of ours. The
		// wording carries the builtin's name for the reason every
		// unprefixed wording in the panel does: NamesBuiltinInLocation takes
		// it back out of the located form, and nothing takes it out of this
		// one (#3141).
		UlimitBadOptionUnprefixed: true,
		UlimitBadOptionStatus:     1,
		// `ulimit -a`, row for row as the engine writes it — the label in a
		// fixed 32-column field, the letter in its own parenthesis at the
		// end, `(kb)` where bash writes `(kbytes, -d)`, and the units spelled
		// into the label rather than into a separate column. Measured
		// 2026-09-14, BusyBox v1.37.0 on linux/arm64, and byte-identical from
		// a glibc build on Debian 12 and a musl one on Alpine 3.
		//
		// Five of the fifteen name limits only Linux has — RLIMIT_NICE,
		// RLIMIT_SIGPENDING, RLIMIT_MSGQUEUE, RLIMIT_RTPRIO and RLIMIT_LOCKS
		// — and a build without one leaves its row out rather than printing a
		// number it does not have; see Runner.HasRlimit. That is measured
		// rather than assumed, because #2278 was right that it could not be
		// read off a Linux run alone:
		//
		//   - bash 5.3, zsh and dash each print, on macOS, their own Linux
		//     table minus the rows for limits this kernel has no number for
		//     — every surviving row byte-identical, padding included. Three
		//     shells, one rule: the row goes and nothing else moves. They do
		//     not agree on which rows: zsh drops `-m: resident set size` as
		//     well, where bash keeps `max memory size`, because Darwin's
		//     sys/resource.h defines RLIMIT_RSS as RLIMIT_AS and the two
		//     read that differently. That is a disagreement about which
		//     limits exist, not about what becomes of a row for one that
		//     does not.
		//   - The field width is a literal and not computed from the rows
		//     present. bash settles that across the platform boundary: its
		//     widest Linux label by far is `real-time non-blocking time`, a
		//     row macOS has no limit for, and every row that remains there
		//     still sits in the Linux column rather than closing up around a
		//     set whose widest label is nine characters shorter. dash says
		//     the same from inside one table: its 20-wide field is
		//     overflowed by `locked memory(kbytes)`, which pushes that one
		//     row's value a column right and nothing else's — a computed
		//     width could not do that. And BusyBox agrees from the other
		//     side: at v1.28, whose table is a different shape again, the
		//     field runs eight characters past its widest label, which no
		//     widest-plus-padding rule leaves.
		//
		// So the ten rows a BSD kernel can answer stand here in the column
		// this measurement found them in. This is the first table in this
		// package written from a Linux run, which is why Runner.HasRlimit is
		// new: the other four were written from the macOS panel and have no
		// row a platform could take away — #2806, where a Linux build of our
		// bash prints eleven rows against the real shell's seventeen.
		//
		// What this table still does not say is which of the five *letters*
		// this shell's `ulimit` accepts when the limit does exist. The three
		// shells above refuse all five on macOS and read all five on Linux,
		// so that is an answer per shell *and* per platform rather than a
		// value on an axis, and it is left open in #2805.
		UlimitListing: []interp.UlimitListingRow{
			{Prefix: "core file size (blocks)         (-c) ", Res: interp.ResourceCore},
			{Prefix: "data seg size (kb)              (-d) ", Res: interp.ResourceData, Scale: 1024},
			{Prefix: "scheduling priority             (-e) ", Res: interp.ResourceSchedulingPriority, Scale: 1},
			{Prefix: "file size (blocks)              (-f) ", Res: interp.ResourceFileSize},
			{Prefix: "pending signals                 (-i) ", Res: interp.ResourcePendingSignals, Scale: 1},
			{Prefix: "max locked memory (kb)          (-l) ", Res: interp.ResourceLockedMemory, Scale: 1024},
			{Prefix: "max memory size (kb)            (-m) ", Res: interp.ResourceResidentSet, Scale: 1024},
			{Prefix: "open files                      (-n) ", Res: interp.ResourceOpenFiles, Scale: 1},
			{Prefix: "POSIX message queues (bytes)    (-q) ", Res: interp.ResourceMessageQueues, Scale: 1},
			{Prefix: "real-time priority              (-r) ", Res: interp.ResourceRealtimePriority, Scale: 1},
			{Prefix: "stack size (kb)                 (-s) ", Res: interp.ResourceStack, Scale: 1024},
			{Prefix: "cpu time (seconds)              (-t) ", Res: interp.ResourceCPUTime, Scale: 1},
			{Prefix: "max user processes              (-u) ", Res: interp.ResourceProcesses, Scale: 1},
			{Prefix: "virtual memory (kb)             (-v) ", Res: interp.ResourceAddressSpace, Scale: 1024},
			{Prefix: "file locks                      (-x) ", Res: interp.ResourceFileLocks, Scale: 1},
		},
		UmaskBadOption:  "illegal option %[1]s",
		PrintfBadOption: "illegal option %[1]s",

		// Numbers. The capital is this shell's, on a message it otherwise
		// words like the lower-case ones around it.
		InvalidNumber:   "Illegal number: %s",
		NumericArgument: "%[1]s: Illegal number: %[2]s",
		ShiftBadNumber:  "Illegal number: %[1]s",
		UmaskBadMask:    "illegal mode: %[1]s",
		// The whole operand quoted back, with no word for what in it was
		// wrong — the same message a numeric mask gets.
		UmaskBadSymbolicMode: "illegal mode: %[1]s",
		UmaskBadMaskStatus:   2,
		UlimitBadNumber:      "bad number",
		UlimitCannotChange:   "error setting limit: %[3]s",

		// The jobs family: the spec first and the sentence after it, which is
		// the reverse of dash's order.
		NoSuchJob:           "%[2]s: no such job",
		NoSuchJobStatus:     2,
		KillNoSuchJob:       "%[1]s: no such job",
		WaitNoSuchJob:       "%[1]s: no such job",
		WaitNoSuchJobStatus: 2,
		// An operand that is no job spec at all is refused by the *number*
		// reader rather than by the job table, and it is the same sentence
		// this shell writes for `shift -1`, `exit abc` and `return abc` —
		// one reader for all of them, where ours had a second one here
		// saying `abc: not a pid`. Measured 2026-09-14: `/t.sh: wait: line
		// 1: Illegal number: abc` at 2, beside `%1: no such job` above,
		// which is the job table answering a word that *is* a spec (#2801).
		WaitBadJob:       "wait: Illegal number: %[1]s",
		WaitBadJobStatus: 2,

		// `kill`. The shell's name and nothing else in front of every one of
		// these — BuiltinNamesTheShellAlone above is where that is said,
		// rather than the unprefixed flags, which write no name at all.
		//
		// One newline and not two: this string ended in one and killFailed
		// adds another, so every failed send was followed by a blank line.
		KillNoSuchProcess: "can't kill pid %[1]s: No such process",
		KillInvalidSignal: "bad signal name '%[1]s'",
		// **There is no option complaint here.** Everything after the dash
		// is a signal to this shell, so `kill -Q`, `kill -NOPE`, `kill -9x`
		// and `kill -99` are all `bad signal name '<the whole word>'` at 1 —
		// measured 2026-09-16 against BusyBox 1.37.0, all four. The wording
		// is KillInvalidSignal's because the reading is the same one, and
		// the status follows: this shell has no route to 2 for a signal it
		// did not recognize (#3139).
		KillIllegalOption: "bad signal name '%[1]s'",
		// The number reader's own sentence, and it is `invalid number` here
		// rather than the `Illegal number` the ash family's other builtins
		// write — measured: `kill notapid` is `ash: invalid number
		// 'notapid'` at 1 where `wait abc` is `Illegal number: abc` at 2.
		KillNotAPid: "invalid number '%[1]s'",
		// `kill -l` distinguishes its own refusal from the signal spec's,
		// which is the one place this shell has two sentences for one shape:
		// `kill -l nope` is `unknown signal 'nope'` where `kill -s nope` is
		// `bad signal name 'nope'`.
		KillListBadNumber: "unknown signal '%[1]s'",
		KillUsage:         "you need to specify whom to kill",
		// 1, not 2. There is no route to 2 in this applet at all — every
		// `kill` failure is 1, the bare `kill` included.
		KillUsageStatus: 1,
		// 1, not 2: a spec this shell will not take is an argument
		// complaint however it was written. `kill -99`, `kill -Q` and
		// `kill -s` with nothing after it are all 1, measured.
		KillBadOptionStatus: 1,
		// And 1 here for the same reason, which is what `kill -s 99` needed:
		// a number out of range reaches this route rather than the one
		// above, and the real shell answers both the same way. Measured
		// 2026-09-16: `kill -s 99 $$`, `kill -l nope` and `kill notapid`
		// are all 1 in BusyBox 1.37.0.
		KillArgumentStatus: 1,
		KillListing:        interp.KillListingZeroFirst,

		// `getopts` names nothing in front of its two complaints, as dash
		// does.
		GetoptsBadOption:       "Illegal option -%[1]s",
		GetoptsMissingArgument: "No arg for -%[1]s option",
		GetoptsUnprefixed:      true,
		// The usage line is not one of those two: it carries the shell, the
		// builtin and the line as everything else here does, and only the
		// word for the slot the shell writes into differs — `var` where the
		// substrate and bash write `name`. Measured 2026-09-14, with no
		// operands and with one, which draw the same line: `/t.sh: getopts:
		// line 1: usage: getopts optstring var [arg]`, status 2, and the
		// script carries on (#2801).
		//
		// Written with the builtin in front as bash's and ksh93's entries
		// are; NamesBuiltinInLocation takes it back out again, because this
		// dialect puts the name in the location instead.
		BuiltinUsage: map[string]string{
			"getopts": "getopts: usage: getopts optstring var [arg]",
		},

		// A number `read` could not read, worded by the letter it was for
		// rather than by one sentence with the operand in it. Measured
		// 2026-09-17 in the pinned image: `read -u x v` is `invalid file
		// descriptor`, `-n x v` is `invalid count` and `-t x v` is `invalid
		// timeout`, each the whole message with the operand nowhere in it,
		// and all three at 2 where ours answered one generic sentence at 1
		// (#3367).
		ReadBadDescriptorSpec: "read: invalid file descriptor",
		ReadBadCount:          "read: invalid count",
		ReadBadTimeout:        "read: invalid timeout",
		ReadBadNumberStatus:   2,

		// `cd`, with the OS's reason where dash gives none.
		CdCannotChange: "can't cd to %[1]s: %[2]s",
		CdStatus:       2,

		// `test`. The middle word is blamed rather than the first — `[ a b c
		// ]` is `b: unknown operand` — and an unknown operator leaves an
		// operand behind, so the same wording names the word after it.
		TestNamesFirstOperand:   false,
		TestUnknownLongOperator: interp.TestUnknownOperatorLeavesAnOperand,
		// And the two-word form is parsed here as well, where six of the
		// seven columns send it straight to the unary evaluation: `[ -Q g.f
		// ]` is `g.f: unknown operand` rather than `-Q`, because the word
		// this shell has no operator for is a string and the one after it is
		// one operand too many. `[ n -eq 5 ]` beside it is the control that
		// says a complaint really about the operand names the operand in
		// both (#3278).
		TestNamesTheWordTheParseStoppedAt: true,
		TestUnaryExpected:                 "%[1]s: unknown operand",
		TestBinaryExpected:                "%[1]s: unknown operand",
		TestIntegerExpected:               "%[1]s: out of range",
		// The word the parse stopped at is named here too, which the bare
		// sentence could not say: `[ -z a b c ]` is `b: unknown operand`,
		// the first word past `-z a`, where dash names the last word its own
		// parse took (#3550).
		TestTooManyArguments: "%[1]s: unknown operand",
		TestOperandExpected:  "argument expected",
		// And the same sentence with the operator named, which is the other
		// half of that parse: a binary operator this shell has, standing in
		// the trailing position, is missing its right operand rather than
		// leaving an operand behind. Measured 2026-09-18, `[ 1 -eq ]` is
		// `ash: -eq: argument expected` and `[ g.f -ot ]` names `-ot`, while
		// `[ 1 -eq 1 -a ]` keeps the bare sentence above — a connective is
		// not one of the operators this names. `==` is in the set here and
		// `=~` is not, which is the same rule read at this shell's own
		// roster (#3550).
		TestTrailingBinaryOperandExpected: "%[1]s: argument expected",
		TestMissingBracket:                "missing %[1]s",

		// The remarks a shell with no terminal makes about job control, both
		// spellings of the same sentence.
		MonitorDenied:         "can't access tty; job control turned off",
		NoJobControlAtStartup: "can't access tty; job control turned off",

		// `alias -g` is a name this shell cannot find, said with nothing in
		// front of it at all — no shell, no line.
		AliasNotFound:             "%[1]s: %[2]s not found",
		AliasNotFoundUnprefixed:   true,
		UnaliasNotFound:           "%[1]s: %[2]s not found",
		UnaliasNotFoundUnprefixed: true,
		// `set -o` prints a name and its state in two columns and no header,
		// where dash writes `Current option settings` over its own.
		OptionListingWidth: 16,
		// The order of the listing, which is this shell's own table rather
		// than a sort — and it was dash's names sorted until #3366, so every
		// row was in the wrong place and two of them named options this
		// shell has not got. Measured 2026-09-17 in the pinned image,
		// BusyBox v1.37.0: `set -o` and `set +o` write these fourteen in
		// this order, and the order does not move with the states.
		//
		// There is no header line above it, which is the other thing that
		// parts this listing from dash's.
		OptionListingOrder: []string{
			"errexit", "noglob", "ignoreeof", "monitor", "noexec", "xtrace",
			"verbose", "noclobber", "allexport", "notify", "nounset",
			"errtrace", "vi", "pipefail",
		},

		// `set -x`. This shell **quotes**, which is the one place it parts
		// from its sibling on a question dash answers with a flat no: dash
		// traces `x=hello wor` where this writes `x='hello wor'`. The record
		// has said so since ash joined the panel, and `dialect/ash` said
		// nothing and took QuoteNever with it (#2443).
		//
		// The spelling is a fourth answer and not bash's — see
		// interp.QuoteSingleOnly for the three measurements behind it.
		TraceQuoting: interp.QuoteSingleOnly,
		// And a fourth alphabet, measured 2026-09-13 over every printable
		// ASCII punctuation character in three positions — 96 words, one
		// `echo` per word, from a script file under `env -i
		// PATH=/usr/bin:/bin`. Beyond interp's always-quoted set this shell
		// adds `*?[{}~#!=%`, and two of those characters are the whole of
		// what separates it from the other three:
		//
		//	`%` is quoted here and in no other panel member
		//	`]` is quoted in the other three and bare here
		//	`^` is quoted by bash and zsh and bare here
		//
		// No Leading, and that is measured rather than left out: `~a` and
		// `a~b` are both quoted, and so are `#a`/`a#b` and `=ab`/`ab=`/`a=b`,
		// so this shell has no position rule at all and sides with ksh93 and
		// zsh against bash. `+,-./:@_` are bare wherever they sit.
		TraceMetacharacters: interp.TraceMetacharacters{
			Anywhere: "*?[{}~#!=%",
		},
		// A fourth reading of the bracket line, and it needs no exemption to
		// express: `[ 1 -lt 2 ]` traces as `'[' 1 -lt 2 ]`, with the opening
		// `[` quoted like any other word and the closer bare only because
		// `]` is not in the alphabet above. `echo ] '[' 'a[b'` is the same
		// two facts away from a test, and `[ -n ']' ]` leaves both brackets
		// bare for the same reason ksh93 leaves only the last one. So the
		// zero value says it; it is written out because a reader who has met
		// interp.TraceBracketPairBare would otherwise take this shell's
		// bare `]` for that answer.
		TraceBareBracket: interp.TraceBracketQuotedLikeAnyWord,

		ParamNullOrNotSet: "parameter not set or null",
		// Silent for a count above `$#`, as bash is — there is no
		// ShiftTooMany here. BusyBox writes nothing and returns 1; the
		// sentence that used to sit on this line is dash's, and it was
		// unreachable while ShiftPastEndFatal was dash's too.
		TimesDecimals:   3,
		JobRunning:      "Running",
		JobDone:         "Done",
		JobExited:       "Done(%[1]d)",
		PrintfBadNumber: "invalid number '%[1]s'",
		// And the operand is quoted back from its first non-blank byte:
		// `printf '%d' "  7  "` is `invalid number '7  '` here, where bash
		// and dash echo the blanks they were handed (#2905).
		PrintfBadNumberEchoesPastTheBlanks: true,
		// `printf` is the BusyBox applet reached as a builtin and reports
		// the way an applet does — the shell's own basename and nothing
		// else, on every route. See BuiltinNamesTheShellAlone for the rows
		// and for the `shift` control that says the rest of this shell's
		// builtins still name the script and the line (#2913).
		// `kill` joins it: **every** message that applet writes carries the
		// shell's name and nothing else — `ash: can't kill pid 999999: No
		// such process`, `ash: invalid number 'notapid'`, `ash: you need to
		// specify whom to kill` — where its own `not found` carries
		// `<file>: line N:`. Measured 2026-09-17 against BusyBox v1.37.0 in
		// the pinned alpine image (#3165).
		//
		// And `test` with its two bracket spellings: every complaint any of
		// the three makes opens `ash:` and nothing else — measured
		// 2026-09-17 in the pinned image, `[ n -eq 5 ]` is `ash: n: out of
		// range`, `[ -Q g.f ]` is `ash: g.f: unknown operand`, `[ 1 -eq 2`
		// is `ash: missing ]`, and `test -Q g.f` under the other spelling is
		// the same again. Every other complaint from this shell opens
		// `<file>: <builtin>: line N:`, which is the control (#3278).
		BuiltinNamesTheShellAlone: map[string]bool{
			"printf": true, "kill": true, "test": true, "[": true, "[[": true,
		},
		PrintfBadVerb:     "%[2]s: invalid format",
		PrintfMissingVerb: "%[1]s: invalid format",
		// And the directive it names is written without the length
		// modifiers it just read past: `printf '%z' x` is
		// `ash: %: invalid format` where every other column names what was
		// written. `%5.2lz` is `%5.2` and `%+ #0z` is `%+ #0`, so the flags,
		// the width and the precision all survive; `%zq` is `%q`, so a
		// conversion character survives too (#3141).
		PrintfDirectiveDropsLengthModifiers: true,
	}
}

// Apply makes any adjustment that is not a vector value.
//
// Every call here is a builtin this shell does not have, checked by asking
// it: `typeset`, `declare`, `disown`, `mapfile`, `readarray`, `compgen`,
// `complete`, `builtin` and `enable` are each `not found`. `let` is *not*
// among them — `let "x=1+1"` sets x to 2 here — which is the one place this
// shell keeps a builtin dash gives up.
func Apply(r *interp.Runner) {
	// The prompt table, installed for the same reason the other dialects
	// install theirs: what a shell does to a prompt parameter before drawing
	// it is the dialect's answer, and "nothing but expansion" is an answer
	// rather than an absence.
	r.SetPromptStyle(PromptStyle())
	// The two `set -o` names this shell has beyond the ones the panel shares.
	// `pipefail` was already *taken* here and not listed, so `set -o
	// pipefail; set +o` did not say the shell was in it and a script saving
	// state with that output lost the option (#3366). `errtrace` was refused
	// outright.
	//
	// The names this shell has **not** got are the other half of the same
	// measurement and are not declared anywhere, because they left the
	// substrate's common table instead: `set -o emacs` and `set -o nolog`
	// are each `illegal option -o …` at 1 here.
	r.AddSetOptions("errtrace", "pipefail")
	// Two letters this shell spells its own way. `-b` is `notify` and `-I`
	// is `ignoreeof` — measured at 0 in the pinned image, with `$-` gaining
	// the letter each time, which is the half that says the letter reached
	// the state and not merely the parser. Neither is in the substrate's
	// shared letter table, because no other column in the panel has either.
	r.SetOptionLetterNames(map[rune]string{'b': "notify", 'I': "ignoreeof"})
	// `source` is `.` under a second name here, as it is in bash, zsh and
	// ksh93 — and not as it is in dash, which has no such command. Measured
	// 2026-09-16 in the pinned alpine image, BusyBox v1.37.0: `type source`
	// is `source is a special shell builtin`, `command -v source` answers
	// `source`, and `source ./args.sh sarg1` gives the sourced file the word
	// exactly as `.` does. `cmd/ash` answered `source: not found` at 127 for
	// all three, which is dash's answer and the shape of #3248 arriving at a
	// builtin rather than at an axis.
	if dot, ok := r.Builtin("."); ok {
		r.Register("source", dot)
	}
	// unanswered DisownAlwaysFails: the builtin is unregistered below, so
	// there is no `disown` here whose status could report anything.
	// TestDisownIsNotABuiltin pins the absence.
	r.Unregister("typeset")
	r.Unregister("declare")
	r.Unregister("disown")
	r.Unregister("mapfile")
	r.Unregister("readarray")
	r.Unregister("compgen")
	r.Unregister("compopt")
	r.Unregister("complete")
	r.Unregister("builtin")
	r.Unregister("enable")
	// `fc` is an external here, as it is for dash: `command -v fc` resolves a
	// path rather than naming a builtin.
	r.Unregister("fc")
}
