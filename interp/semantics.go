// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// Answer is one axis's value, and it has three states rather than two.
//
// Unspecified is the point of the type. The vector contains *only* the places
// the shells disagree — everything they agree about never became an axis and
// is simply the implementation — so leaving one unset is not an oversight, it
// is saying "no shell has been chosen here". A script that depends on such an
// axis is refused, naming it, rather than silently getting one shell's answer.
type Answer uint8

const (
	// Unspecified refuses the behavior rather than guessing at it.
	Unspecified Answer = iota
	Yes
	No
)

func (a Answer) String() string {
	switch a {
	case Yes:
		return "yes"
	case No:
		return "no"
	}
	return "unspecified"
}

// Semantics is where the shells disagree about what identical syntax *means*.
//
// This is the structure docs/spec/semantics.md argued for, and the argument
// is worth restating because the obvious alternative looks fine. Grammar
// differences are additive — a construct either parses or it does not — and
// syntax.Dialect models those. Semantic differences are conflicts: the same
// text means different things, and no amount of adding or removing features
// produces one from another. They need switches.
//
// Every field is named for the behavior rather than for the shell that wants
// it, which the spec requires and the measurements insist on: ksh93 accepts
// `&>` or does not depending on which build is installed, twelve years apart
// under the same name, so a field called `Ksh` could not be given a value.
//
// Eighteen axes were measured across four shells and produced eight distinct
// groupings — no ordering of the shells explains the data, which is why this
// is a vector and not a level.
type Semantics struct {
	// SplitParamExpansion field-splits the result of an unquoted parameter
	// expansion. False in zsh, and narrower than "word splitting": zsh still
	// splits an unquoted *command* substitution, so this is two fields and
	// not one.
	SplitParamExpansion Answer
	// SplitCommandSubstitution field-splits an unquoted command
	// substitution. True everywhere measured, including zsh.
	SplitCommandSubstitution Answer

	// GlobExpansionResults matches the *result* of an expansion against the
	// filesystem. False in zsh, where only a pattern written literally in the
	// source is expanded. The same rule decides whether `[[ abc == $p ]]`
	// treats $p as a pattern, which is one behavior observed twice rather
	// than two quirks.
	GlobExpansionResults Answer
	// GlobNoMatchIsError makes a pattern matching nothing an error instead of
	// passing it through. True only in zsh.
	GlobNoMatchIsError Answer

	// AssignmentPrefixPersistsOnSpecialBuiltin keeps `x=1 shift` set
	// afterwards. POSIX requires it; dash and ksh93 comply and bash and zsh
	// do not.
	AssignmentPrefixPersistsOnSpecialBuiltin Answer

	// EchoInterpretsEscapes expands backslash escapes in `echo` without -e.
	// True in dash and zsh, false in bash and ksh93 — a grouping no other
	// axis produces.
	EchoInterpretsEscapes Answer

	// LengthOfSpecialIsCount makes `${#@}` the number of positional
	// parameters. False in dash, which gives the length of the joined
	// string. The first axis measured where dash stands alone, and a silent
	// one: both answers are plausible numbers.
	LengthOfSpecialIsCount Answer

	// ArithLeadingZeroIsOctal reads `0100` as sixty-four. False in zsh, where
	// it is one hundred. The quietest divergence measured — nothing warns,
	// both are plausible numbers, and file modes are written this way.
	ArithLeadingZeroIsOctal Answer
	// FatalErrorStatusIsOne is the status a fatal shell error carries.
	// True in bash, ksh93 and zsh; dash alone exits 2.
	//
	// It began as an arithmetic-only axis and was generalised on evidence:
	// a failed arithmetic expansion, a readonly reassignment and a `shift`
	// past the end are three unrelated errors, and every shell gives all
	// three the same status. The split is a property of the shell, not of
	// the error, so it is one axis rather than three.
	//
	// *Which* errors are fatal is a separate question and stays per-error —
	// ReadonlyReassignmentFatal and ShiftPastEndFatal answer it, and the
	// shells genuinely disagree there. A silent axis either way: scripts
	// that branch on `$?` rather than on truthiness read the failure
	// correctly under one group and misread it under the other.
	FatalErrorStatusIsOne Answer
	// ArithNameValueRecurses re-evaluates a name-shaped value as an
	// expression: with `x=abc`, `$((x+1))` is 1 in bash and zsh, because
	// `abc` is looked up in turn and is unset. dash and ksh93 error instead.
	// The interpreter followed the two that agree and said so in a comment,
	// which is the shape of a guess rather than a measurement.
	ArithNameValueRecurses Answer
	// ArithInvalidOctalDigitIsError rejects `08` once a leading zero has
	// been read as octal. True in dash and bash; ksh93 falls back to decimal
	// and yields 8.
	//
	// This is the second axis the vector could not express with one field.
	// `ArithLeadingZeroIsOctal` was doing two jobs: `0100` is 64 in dash,
	// bash and ksh93 and 100 in zsh, so ksh93 *is* octal — but it is octal
	// and tolerant, and one boolean cannot say that. The `${!x}` note in
	// semantics.md records the same failure mode; this is it happening a
	// second time, which makes it a limit of the model rather than a quirk.
	//
	// zsh never reaches this: nothing there made the zero octal.
	ArithInvalidOctalDigitIsError Answer
	// IndirectionYieldsName makes `${!x}` the *name* rather than the value it
	// names: with `x=y`, ksh93 gives `x` and bash gives the value of `y`.
	//
	// Only reachable where the grammar parses `${!x}` at all, which is bash
	// and ksh93 — dash and zsh reject it. That is the point: a three-way
	// divergence became a grammar flag plus a binary axis, and neither half
	// needed a third state. `semantics.md` records `${!x}` as the axis the
	// binary table could not express; this is the shape that expresses it.
	IndirectionYieldsName Answer
	// BraceExpansion expands `{a,b}` and `{1..3}`. Absent from dash, where
	// the word is a literal.
	//
	// It lives here rather than in syntax.Dialect even though it is
	// additive, because the token stream is identical either way: the
	// parser produces the same word, and only expansion differs. It is also
	// silent in the `&>` sense — `echo {1..3}` prints something either way,
	// and nothing reports that one of them is not what was meant.
	BraceExpansion Answer
	// EqualsExpansion replaces an unquoted word beginning with `=` by the
	// path of the command named after it: `echo =ls` prints /bin/ls. zsh
	// alone, and silent in the `&>` sense — the other three take the word
	// literally and report nothing, so the same script prints two different
	// things and neither shell complains.
	//
	// Its failure is not silent: a name that resolves to nothing is fatal to
	// the script, like any other failed expansion.
	EqualsExpansion Answer
	// UnterminatedBracket is what `[` without a closing `]` means in a
	// pattern, and it is the axis that does not fit Answer.
	//
	//	case "[" in [) hit;; *) miss;; esac
	//	bash  → hit          a literal `[`
	//	ksh93 → hit          a literal `[`
	//	dash  → miss         a class that can never match
	//	zsh   → bad pattern  an error
	//
	// Three answers, and it is load-bearing rather than exotic: `[` is the
	// name of the test builtin.
	//
	// It gets its own type rather than a wider Answer. The prediction in
	// semantics.md was that Answer would have to grow a third state; writing
	// it showed that would be worse, because every other axis is genuinely
	// binary and a wider Answer would let `BracketBadPattern` be assigned to
	// any of them and still compile. An axis with three answers gets a type
	// with three values; the twenty-three binary ones keep the type that
	// says so.
	UnterminatedBracket BracketPolicy
	// TraceAssignmentsSeparately gives each assignment of `a=1 b=2` its own
	// trace line. True in bash and ksh93; dash and zsh put them on one.
	TraceAssignmentsSeparately Answer
	// TraceShowsItsOwnDisabling prints `set +x` before acting on it. True in
	// dash, bash and zsh; ksh93 applies the change first, so the command
	// that stops tracing leaves no trace of itself.
	TraceShowsItsOwnDisabling Answer
	// UnsetPositionalIsAllowed lets `$1` expand to nothing under `set -u`
	// rather than being an error. ksh93 alone, and quiet where it differs:
	// a script that reads an argument it was not given carries on there and
	// stops everywhere else.
	UnsetPositionalIsAllowed Answer
	// SignalHandlerSeesEarlierStatus shows a signal handler the status from
	// before the command that triggered it rather than that command's own.
	// zsh alone: after `false; kill -INT $$`, zsh's handler reads 1 where
	// the others read 0, because `kill` succeeded.
	SignalHandlerSeesEarlierStatus Answer
	// ExitTrapIsFunctionLocal fires an EXIT trap set inside a function when
	// that function returns, rather than when the script ends. zsh alone; a
	// trap set at the top level behaves the same everywhere.
	ExitTrapIsFunctionLocal Answer
	// SIGPrefixAccepted reads `SIGINT` as a name for the same signal `INT`
	// names, wherever a signal can be named.
	//
	// dash alone says no, and says it in three places for two different
	// reasons — the prefix is simply not part of a signal's name there:
	//
	//	trap 'x' SIGINT   trap: SIGINT: bad trap
	//	kill -SIGINT $$   kill: Illegal option -S
	//	kill -s SIGINT $$ kill: invalid signal number or name: SIGINT
	//
	// It is one axis rather than one per builtin because it is a property of
	// how the shell reads a signal name, and the shell that refuses it
	// refuses it everywhere. Asked only where the prefix is actually present
	// and stripping it would name a signal: `trap 'x' INT` needs no answer
	// from anyone, and neither does `SIGNOPE`, which names nothing either way.
	SIGPrefixAccepted Answer

	// KillListAcceptsName lets `kill -l` translate a name into a number, as
	// the reverse of what it does with one. True in bash, ksh93 and zsh.
	//
	// dash's `-l` takes an *exit status* rather than a signal, so `kill -l 9`
	// agrees with everyone by arriving there another way and `kill -l INT` is
	// an illegal number. One question with two answers rather than a feature
	// dash is missing, which is why it is an axis and not a gap.
	KillListAcceptsName Answer

	// ExitTrapRunsOnSignalDeath fires the EXIT trap when the shell is ending
	// because a signal it had no handler for killed it, rather than because
	// it reached the end or ran `exit`.
	//
	//	trap 'echo bye' EXIT; kill -INT $$
	//
	// prints bye in bash and ksh93 and prints nothing in dash and zsh, and
	// all four report 130. A two-two split on whether dying counts as
	// exiting.
	ExitTrapRunsOnSignalDeath Answer

	// ExitArgument is how strict `exit` is about what it is given, and it is
	// an ordering rather than a side:
	//
	//	exit -1     dash → error 2   bash → 255   ksh93, zsh → 255
	//	exit abc    dash → error 2   bash → 2     ksh93, zsh → 0
	//
	// dash rejects both, bash rejects only the one that is not a number, and
	// ksh93 and zsh take anything. Three behaviors on a line, so a policy
	// rather than a bool — the same shape as UnterminatedBracket, and for
	// the same reason.
	ExitArgument ExitArgumentPolicy
	// BracketCaretNegates reads `[^abc]` as a negated class. dash alone
	// treats `^` as an ordinary character, so `[^abc]` matches a caret there
	// and everything-but there elsewhere: the two answers are both matches,
	// on different inputs, with nothing to warn on.
	BracketCaretNegates Answer
	// GetoptsAssignmentRestartsWord makes assigning OPTIND begin the word
	// again, dropping any position inside a cluster.
	//
	// True in bash, dash and ksh93, and it is the *assignment* that does it
	// rather than the value: `set -- -ab; getopts ab o; OPTIND=1` writes the
	// number OPTIND already held, and those three still restart and read `a`
	// a second time where zsh carries on to `b`.
	GetoptsAssignmentRestartsWord Answer

	// GetoptsClearsOptarg empties OPTARG when `getopts` reports a bad option
	// rather than leaving it unset. zsh alone, and a script testing
	// `${OPTARG-}` can tell the two apart.
	GetoptsClearsOptarg Answer

	// CdWithoutHomeIsAnError makes `cd` with no operand and no HOME a
	// failure. True in bash and ksh93; dash and zsh stay where they are and
	// report success, which is the quieter answer and the surprising one.
	// The same axis answers `cd -` with no OLDPWD.
	CdWithoutHomeIsAnError Answer
	// CdDashPrintsTheDirectory writes the new directory when `cd -` moves.
	// True in bash, dash and ksh93; zsh alone is silent.
	CdDashPrintsTheDirectory Answer

	// PrintfReportsBadNumber complains when a numeric conversion is given
	// something that is not a number. True in bash and dash, false in ksh93
	// and zsh — and all four print the zero either way, so the complaint sits
	// beside the output rather than instead of it.
	PrintfReportsBadNumber Answer
	// PrintfBackslashC is what `\c` means in a printf format, and it is three
	// different things rather than a switch:
	//
	//	printf "a\cbZ"   bash, dash  a\cbZ      two literal characters
	//	                 ksh93       a<0x02>Z   \cX is control-X
	//	                 zsh         a          the output stops there
	//
	// Measured by the bytes rather than by the display, which is the only way
	// to tell the middle one from the last: ksh93's output *looks* truncated
	// next to zsh's until the control character is read as a byte.
	PrintfBackslashC PrintfBackslashCPolicy
	// PrintfOutputPrecedesComplaint writes what `printf` produced before it
	// complains about the rest, rather than after.
	//
	// True in ksh93 alone. It is visible only where both streams arrive at
	// one place, which is exactly how the corpus reads them: `printf "[%z]"`
	// is `[` then the complaint there and the complaint then `[` in the other
	// three, whose output is still sitting in a buffer when the complaint
	// goes out.
	PrintfOutputPrecedesComplaint Answer
	// PrintfEmptyIsNotANumber complains about a numeric conversion given an
	// operand that is present and empty. bash alone: `printf '%d' ""` is an
	// error there and a zero in the other three, all of which print the zero
	// anyway. An argument that is *missing* is never an error in any of them.
	PrintfEmptyIsNotANumber Answer

	// PrintfQuote is how `%q` quotes, which is three answers and an absence
	// rather than a switch — see PrintfQuoteStyle.
	PrintfQuote PrintfQuoteStyle

	// RedirectsWriteToEveryTarget sends a command's output to *all* of the
	// files it redirects to rather than only the last: `echo x >a >b` fills
	// both in zsh and leaves `a` empty in the other three.
	//
	// Silent either way — the shells that write once report no error, and the
	// script looks like it worked — which is the `&>` failure mode in a
	// redirection. Asked only where a command redirects one stream twice,
	// because that is the only place it decides anything.
	RedirectsWriteToEveryTarget Answer

	// KillStatus is what `kill` reports when it was given several targets
	// and they did not all agree. Three answers, and no two of them are the
	// majority:
	//
	//	kill -0 $$ 999999    bash → 0   dash, ksh93 → 1   zsh → 1
	//	kill 999998 999999   bash → 1   dash, ksh93 → 1   zsh → 2
	//
	// bash reports success if it signaled anything at all, and zsh reports
	// the number that failed — which is a status carrying a count rather
	// than a verdict, and the reason this is a policy rather than a bool.
	KillStatus KillStatusPolicy
	// CommandNotFoundStatusIsNotFound makes `command -v` answer 127 for a
	// name that is nothing, rather than a plain 1. dash alone says yes; the
	// other three report a failure and leave 127 to mean a command that was
	// looked for and run.
	CommandNotFoundStatusIsNotFound Answer

	// SetFTurnsOffGlobbing makes `set -f` the short spelling of `set -o
	// noglob`. True in bash, dash and ksh93. zsh spells that option the long
	// way only: there `-f` is about startup files and leaves globbing alone,
	// so `set -f; echo *.txt` lists the files.
	SetFTurnsOffGlobbing Answer

	// ArithIntegerOperatorRefusesFloat rejects a float where only an integer
	// will do — `7 % 2.5`, `1.5 & 1`, a shift. ksh93 says yes and refuses;
	// zsh says no and truncates. It does not arise in a shell without floats,
	// which is why bash and dash leave it unanswered.
	ArithIntegerOperatorRefusesFloat Answer

	// RegexQuotingMakesLiteral treats a quoted right operand of `=~` as a
	// literal string. True in bash alone; ksh93 and zsh keep it a regex, so
	// quoting a regex is unportable in either direction.
	RegexQuotingMakesLiteral Answer

	// LastPipelineElementInCurrentShell runs the last command of a pipeline
	// in this shell, so `echo x | read v` sets v. True in ksh93 and zsh.
	LastPipelineElementInCurrentShell Answer

	// ShiftPastEndFatal ends a non-interactive shell when `shift` runs off
	// the end. True in dash and ksh93.
	ShiftPastEndFatal Answer
	// ReadonlyReassignmentFatal ends the script when a readonly variable is
	// assigned. True everywhere but bash, measured with a plain assignment in
	// a script file — adding a redirect makes it a command and reverses the
	// answer, which is the contaminated-probe trap docs/spec/oracle.md
	// records.
	ReadonlyReassignmentFatal Answer

	// DeclaredNameWithoutValueIsEmpty gives a name a value when it is
	// declared without one: `local u` or `typeset u`. zsh alone says yes, so
	// `${u-UNSET}` is empty there and UNSET in bash and ksh93 — the name
	// exists in all three, but only zsh considers it set.
	DeclaredNameWithoutValueIsEmpty Answer
	// TypesetLocalNeedsKeywordFunction restricts `typeset`'s local scope to
	// functions defined with the `function` word. ksh93 says yes: in
	// `f() { typeset x=1; }` the assignment reaches the caller's `x`, and in
	// `function f { typeset x=1; }` it does not. bash and zsh make no such
	// distinction, which is why the two definition forms are interchangeable
	// there and are not in ksh93. dash has no `typeset` at all, which is why
	// the axis is absent rather than false there.
	//
	// It asks about `typeset` and not about `local` because `local` is
	// unanimous: every shell that has it — all but ksh93, which does not —
	// makes it local in a function defined either way.
	TypesetLocalNeedsKeywordFunction Answer

	// SelectLayout is how `select` draws its menu. Three engines rather than
	// two answers, which is why it has its own type.
	SelectLayout SelectMenuLayout
	// SelectPromptNeedsTerminal withholds PS3 unless the input is a terminal.
	// ksh93 alone says yes, which is why a ksh93 script's transcript has the
	// menu in it and no prompt.
	SelectPromptNeedsTerminal Answer
	// SelectEofIsSuccess makes the input running out a success. zsh alone
	// says yes; the other two report 1.
	SelectEofIsSuccess Answer
	// SelectAssumesUnboundedWidth treats an unset COLUMNS as no limit rather
	// than as 80. zsh says yes — with no terminal to ask it puts forty items
	// on one line — and bash says no. It does not arise for a menu that is
	// always vertical, which is why ksh93 leaves it unanswered.
	SelectAssumesUnboundedWidth Answer
	// SelectEofEndsPromptLine writes a newline to standard error when the
	// input runs out, closing the line the prompt left open. zsh alone does;
	// bash closes the line on standard *output* instead, which is a different
	// question and the field below.
	SelectEofEndsPromptLine Answer
	// SelectEofPrintsNewline writes a newline to standard *output* when the
	// input runs out — the one thing this loop prints that does not go to
	// standard error. bash alone does it.
	SelectEofPrintsNewline Answer

	// AssignmentUpdatesPipelineStatus counts a bare assignment as a command
	// for the pipeline-status record. bash says yes, so `false | true; x=1`
	// replaces the two elements with one holding 0; zsh says no and leaves
	// them. Every other shape of command updates it in both.
	AssignmentUpdatesPipelineStatus Answer
	// UnsetEndsTheProducedPipelineStatus makes `unset` permanent. zsh says
	// yes and the name never fills again; in bash the producer outlives it.
	// It is the opposite of what a produced *scalar* does, where unset ends
	// it in both — `unset RANDOM` leaves an ordinary empty name everywhere.
	UnsetEndsTheProducedPipelineStatus Answer

	// ArrayScalarIsTheWholeArray decides what a plain `$a` gives when `a` is
	// an array: zsh says every element joined by a space, and bash and ksh93
	// say the first element alone. dash has no arrays, which is why the axis
	// is absent rather than false there.
	ArrayScalarIsTheWholeArray Answer

	// ArrayBaseIsZero indexes arrays from 0. True in bash and ksh93, false in
	// zsh, which counts from 1. dash has no arrays at all, which is why the
	// axis is absent rather than false there.
	ArrayBaseIsZero Answer

	// DollarZeroInFunctionIsFunctionName makes `$0` inside a function the
	// function's name. True only in zsh.
	DollarZeroInFunctionIsFunctionName Answer

	// BuiltinSyntaxErrorFatal ends a non-interactive shell when text handed
	// to a special builtin does not parse — `eval "if"`, or a sourced file
	// with an unterminated `if` in it.
	//
	// True only in dash, which is the POSIX rule that a special builtin's
	// failure is fatal; bash, ksh93 and zsh report it and carry on. One axis
	// covers both callers because the answers are the same for both in every
	// shell measured, where the *status* is not — that is two fields on
	// Diagnostics.
	BuiltinSyntaxErrorFatal Answer

	// DotWithNoOperandIsAnError decides whether `.` with no filename is a
	// failure at all. False in dash, which does nothing and reports success;
	// true in bash, ksh93 and zsh.
	//
	// Separate from the status and from the fatality because the panel splits
	// four ways on `.` alone — dash 0, bash 2 surviving, ksh93 2 fatal, zsh 1
	// surviving — and one field with four answers would have to invent a type
	// to hold what is really three independent questions.
	DotWithNoOperandIsAnError Answer

	// DotMissingFileFatal ends the script when `.` cannot read its file.
	// True in dash and ksh93, false in bash and zsh — the same split as
	// ShiftPastEndFatal, and for the same POSIX reason.
	DotMissingFileFatal Answer

	// DotPassesArguments gives a sourced file its own positional parameters
	// from the words after the filename, restoring the caller's afterwards.
	//
	// False in dash, which ignores them, so `. f.sh ARG` leaves `$1` as the
	// caller's; true in bash, ksh93 and zsh. With no words after the filename
	// every shell leaves the parameters alone, so the axis only speaks when
	// there are some.
	DotPassesArguments Answer

	// ExecFailureRunsExitTrap runs a `trap … EXIT` handler when `exec` could
	// not run the command it was given. True in dash and bash, false in ksh93
	// and zsh.
	//
	// A *successful* exec runs no handler anywhere, and that is not an axis:
	// the trap died with the process the exec replaced. Only the failure has
	// a shell left to decide anything, and the panel splits on it.
	ExecFailureRunsExitTrap Answer

	// TimesRejectsArguments makes `times` refuse an argument rather than
	// ignore it. True in zsh, false in dash and bash.
	//
	// ksh93 answers neither: `times` is a reserved word there, so `times foo`
	// is a *syntax* error and no builtin ever runs. That is a grammar question
	// rather than this one, and it is recorded in the corpus rather than
	// modeled here.
	TimesRejectsArguments Answer

	// EmptyPathIsTheCurrentDirectory searches the current directory when PATH
	// is set and empty.
	//
	// True in dash, bash and zsh; false in ksh93. `PATH=` reads like "nowhere"
	// and is not: an empty PATH is one *empty element*, and an empty element
	// means the current directory, so three of the four will still run a
	// command sitting next to the script. Measured with the command in the
	// current directory, which is the only arrangement that tells the two
	// answers apart — with it anywhere else all four report not-found and the
	// axis is invisible.
	//
	// `PATH=:` is not this question. Two empty elements is unanimous: every
	// shell searches the current directory for it.
	EmptyPathIsTheCurrentDirectory Answer

	// ExecTakesOptions lets `exec` read options of its own, such as
	// `-a name` to choose the argv[0] the command sees. True in bash, ksh93
	// and zsh; false in dash, where a leading `-a` is the name of a command
	// and is reported as not found.
	//
	// The answer has to come before the command is looked up, because it
	// decides which word the command is.
	ExecTakesOptions Answer

	// DotFallsBackToCurrentDirectory looks in the current directory for a
	// `.` operand with no slash in it, after PATH has missed.
	//
	// True only in bash. PATH is searched first everywhere, and wins over an
	// identically named file in the current directory in all four — this is
	// only about what happens when PATH does not have it.
	DotFallsBackToCurrentDirectory Answer

	// TestAcceptsDoubleEqual makes `==` a synonym for `=` in `test` and `[`,
	// so `test a == a` is a string comparison. True in bash, ksh93 and zsh.
	//
	// False in dash, and false does not mean "compares unequal": it means the
	// word is not an operator at all, so `test a == b` is three words with no
	// operator among them and is reported as one. The answer therefore has to
	// come before the comparison, not after it.
	//
	// This is only about `test` and `[`. Inside `[[ ]]` the same spelling is
	// a pattern match, which is a different question entirely.
	TestAcceptsDoubleEqual Answer

	// SignalDeathStatusIsTwoFiftySix encodes a command killed by a signal as
	// 256 + the signal rather than 128 + the signal. True only in ksh93,
	// which reports 265 for KILL and 271 for TERM where the other three
	// report 137 and 143.
	//
	// Measured across eight signals; it is not a special case for any one of
	// them. POSIX requires only "greater than 128", which decides nothing,
	// so the preset follows the three that agree.
	SignalDeathStatusIsTwoFiftySix Answer

	// PipefailOption is whether `set -o pipefail` exists, making a pipeline
	// report its last failing element rather than its last element. True in
	// bash, ksh93 and zsh; absent from dash and from POSIX, where a pipeline
	// is defined to report its last command and nothing offers to change it.
	//
	// Not a wording difference: where it is absent the name is not an option
	// at all, so `set -o pipefail` fails and the pipeline goes on reporting
	// its last element — which is the answer a script guarding against a
	// failure upstream is specifically trying not to get.
	PipefailOption Answer

	// ErrexitSeesPipefailFailure lets `set -e` stop for a failure that only
	// pipefail produced — a pipeline whose last element succeeded and whose
	// earlier one did not. True in bash and zsh; false in ksh93, which runs
	// on.
	//
	// Absent rather than false in dash, which has no pipefail, so the
	// question cannot arise there and is never asked.
	//
	// Narrower than it looks: an ordinary failing pipeline — `true | false` —
	// stops all three, and this is only about the failure the option adds.
	ErrexitSeesPipefailFailure Answer

	// PrintfAssignsWithV makes `printf -v name fmt args` put the formatted
	// text in a variable and print nothing. True in bash and zsh; dash and
	// ksh93 have no such option and reject it as an unknown one.
	//
	// It is how a script formats a value without a command substitution, so
	// without it the text goes to stdout and the variable stays empty — two
	// wrongs at once, and both silent.
	PrintfAssignsWithV Answer

	// PrintfRejectsUnknownOption treats any leading word starting with `-` as
	// an option and refuses one it does not know. True in bash, dash and
	// ksh93, where even `printf "-%s\n" x` is an error because the format
	// itself begins with a dash.
	//
	// False in zsh, which recognizes the options it has and takes anything
	// else as the format — so `printf -q x` prints `-q` there and is an error
	// in the other three.
	PrintfRejectsUnknownOption Answer

	// UmaskPrintsFourDigits writes the mask with a leading zero — `0022`
	// against zsh's `022`. True in bash, dash and ksh93.
	//
	// Only about printing: all four read `022` and `0022` alike, and the
	// symbolic form `umask -S` is identical in every one of them.
	UmaskPrintsFourDigits Answer

	// UmaskSetWithSPrints echoes the new mask when `umask -S mask` both sets
	// and is asked for the symbolic form. True only in bash, which prints
	// `u=rwx,g=,o=` after setting; the other three set and say nothing.
	//
	// Only for that combination: `umask mask` is silent in all four, and
	// `umask -S` with no mask prints in all four.
	UmaskSetWithSPrints Answer
}

// There is deliberately no CoreSemantics, and the absence is the sharpest
// consequence of the whole specification.
//
// syntax.Core() exists because grammar differences are *additive*: a
// construct either parses or it does not, so "what every shell accepts" is a
// well-defined intersection. Semantic differences are conflicts. There is no
// intersection of "an unquoted expansion is split" and "it is not", and no
// value of a boolean means both. A semantics preset therefore has to name a
// shell, while a grammar preset does not.
//
// The pairing this implies is not an inconsistency: accept the constructs
// every real shell accepts, and behave like the one scripts were written
// against — syntax.Core() with BashSemantics().
//
// Which pairing to use is not this package's decision. This is the substrate:
// it owns the *mechanism*, and a shell built on it owns the policy, the same
// way the gate and the event stream are defined here and the sandbox backends
// and protocols are not. The presets exist so that choice can be spelled in
// one line rather than eighteen.

// PosixSemantics is what the specification requires, which is not what any
// shell does in full — it is the right target for a portability check and the
// wrong one for a runtime.
// SelectMenuLayout is how a shell draws a `select` menu. The engines differ
// enough that the same nine items are nine lines in two shells and one line in
// the third, so this is a named choice rather than a flag.
type SelectMenuLayout int

const (
	// SelectMenuVertical is one item per line, always. ksh93's, whose column
	// mode is reached on the terminal's height rather than its width.
	SelectMenuVertical SelectMenuLayout = iota
	// SelectMenuVerticalThenColumns is bash's: one item per line while the
	// list would fit on one line, and tab-separated columns once it would
	// not — which is the opposite way round from how it sounds.
	SelectMenuVerticalThenColumns
	// SelectMenuColumns is zsh's: always packed into columns padded with
	// spaces, so even three items share one line.
	SelectMenuColumns
)

func PosixSemantics() Semantics {
	return Semantics{
		SplitParamExpansion:                      Yes,
		SplitCommandSubstitution:                 Yes,
		GlobExpansionResults:                     Yes,
		GlobNoMatchIsError:                       No,
		AssignmentPrefixPersistsOnSpecialBuiltin: Yes,
		EchoInterpretsEscapes:                    No,
		LengthOfSpecialIsCount:                   Yes,
		ArithLeadingZeroIsOctal:                  Yes,
		// dash is the panel's POSIX-faithful member and the only one
		// exiting 2, so the POSIX preset follows it. The standard itself
		// requires only "greater than zero", which decides nothing.
		FatalErrorStatusIsOne:          No,
		ArithNameValueRecurses:         No,
		BraceExpansion:                 No,
		BracketCaretNegates:            No,
		ExitTrapIsFunctionLocal:        No,
		SignalHandlerSeesEarlierStatus: No,
		UnsetPositionalIsAllowed:       No,
		TraceShowsItsOwnDisabling:      Yes,
		TraceAssignmentsSeparately:     No,
		ExitArgument:                   ExitArgStrict,
		EqualsExpansion:                No,
		// POSIX gives `test` one spelling of string equality, so `==` is not
		// an operator; the three shells that accept it added it.
		TestAcceptsDoubleEqual: No,
		// POSIX requires only "greater than 128" for a command killed by a
		// signal, which decides nothing; three of the four use 128.
		SignalDeathStatusIsTwoFiftySix: No,
		// POSIX gives printf no options at all, so there is nothing to
		// assign with and a leading `-` word is not one.
		PrintfAssignsWithV:         No,
		PrintfRejectsUnknownOption: Yes,
		// POSIX shows the mask in a form that can be read back; three of the
		// four write four octal digits.
		UmaskPrintsFourDigits: Yes,
		// POSIX says setting the mask writes nothing.
		UmaskSetWithSPrints: No,
		// POSIX defines a pipeline's status as its last command's, and
		// offers nothing to change it.
		PipefailOption:                     No,
		ArithInvalidOctalDigitIsError:      Yes,
		RegexQuotingMakesLiteral:           No,
		LastPipelineElementInCurrentShell:  No,
		ShiftPastEndFatal:                  Yes,
		ReadonlyReassignmentFatal:          Yes,
		ArrayBaseIsZero:                    Yes,
		DollarZeroInFunctionIsFunctionName: No,
		// A special builtin's failure is fatal to a non-interactive shell,
		// which the standard states outright. dash is the only member of the
		// panel that still does it, and the preset follows the standard
		// rather than the majority.
		BuiltinSyntaxErrorFatal:   Yes,
		DotMissingFileFatal:       Yes,
		DotWithNoOperandIsAnError: Yes,
		// The standard gives `.` a filename and nothing else; passing
		// positional parameters to a sourced file is an extension three of
		// the four grew. And it reads the file from PATH, with no mention of
		// the current directory as a fallback.
		DotPassesArguments:             No,
		DotFallsBackToCurrentDirectory: No,
		// The standard says a special builtin's failure is fatal and says
		// nothing about a trap on the way out; dash, the panel's
		// POSIX-faithful member, runs it, so the preset follows the shell
		// rather than the silence. `exec` takes no options in the standard —
		// -a is an extension three of the four grew.
		ExecFailureRunsExitTrap: Yes,
		ExecTakesOptions:        No,
		// The standard says an empty element is the current directory and
		// makes no exception for the whole variable being empty, so the
		// preset follows the text and the majority together.
		EmptyPathIsTheCurrentDirectory: Yes,
		// The standard says `times` takes no operands and does not say what to
		// do with one; the two shells that follow it most closely ignore it.
		TimesRejectsArguments: No,
	}
}

// CoreSemantics fixes the axes every shell in the core panel agrees on and
// leaves the rest unspecified.
//
// It is the counterpart of syntax.Core(), built the same way: that refuses
// constructs not every shell has, and this refuses *behaviors* not every
// shell shares. A script that runs under it depends on nothing the panel
// disagrees about, which makes it a portability check rather than a runtime —
// the same role docs/spec/core.md gave strict POSIX.
//
// Only two of the fourteen axes survive, and that is not a defect of the
// panel. The axes exist because they diverge; everything shells agree about
// never became one.
func CoreSemantics() Semantics {
	return Semantics{
		SplitCommandSubstitution: Yes,
		LengthOfSpecialIsCount:   Yes,
	}
}

// sem returns the runner's semantics, defaulting to the core.
//
// Defaulting to a *shell* would be the substrate answering a question that is
// not its to answer. Defaulting to the core answers it honestly: what every
// shell agrees on is done, and anything else is refused until something above
// chooses. A shell built on this package sets the field; that is its job.
func (r *Runner) sem() Semantics {
	if r.Semantics != nil {
		return *r.Semantics
	}
	return CoreSemantics()
}

// ExitArgumentPolicy is how strict `exit` is about its argument.
type ExitArgumentPolicy int

const (
	// ExitArgUnspecified is no answer, and is refused like any other.
	ExitArgUnspecified ExitArgumentPolicy = iota
	// ExitArgStrict refuses anything that is not a non-negative number:
	// dash.
	ExitArgStrict
	// ExitArgNumeric refuses text but wraps a negative: bash.
	ExitArgNumeric
	// ExitArgLenient takes anything, reading text as zero: ksh93 and zsh.
	ExitArgLenient
)

func (e ExitArgumentPolicy) String() string {
	switch e {
	case ExitArgStrict:
		return "strict"
	case ExitArgNumeric:
		return "numeric"
	case ExitArgLenient:
		return "lenient"
	}
	return "unspecified"
}

// exitArgument resolves the axis, and only for an argument that is actually
// questionable — `exit 3` needs no answer from anyone.
func (r *Runner) exitArgument() ExitArgumentPolicy {
	p := r.sem().ExitArgument
	if p == ExitArgUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			"exit: this argument: the shells disagree here and no dialect was chosen"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// PrintfBackslashCPolicy is what `\c` means in a printf format.
type PrintfBackslashCPolicy int

const (
	// PrintfBackslashCUnspecified is no answer, and is refused like any other.
	PrintfBackslashCUnspecified PrintfBackslashCPolicy = iota
	// PrintfBackslashCLiteral writes the two characters: bash, dash.
	PrintfBackslashCLiteral
	// PrintfBackslashCControl reads `\cX` as control-X: ksh93.
	PrintfBackslashCControl
	// PrintfBackslashCStops ends the output there: zsh.
	PrintfBackslashCStops
)

func (p PrintfBackslashCPolicy) String() string {
	switch p {
	case PrintfBackslashCLiteral:
		return "literal"
	case PrintfBackslashCControl:
		return "control character"
	case PrintfBackslashCStops:
		return "stops the output"
	}
	return "unspecified"
}

// backslashC resolves the axis, and only for a format that has a `\c` in it.
func (r *Runner) backslashC() PrintfBackslashCPolicy {
	p := r.sem().PrintfBackslashC
	if p == PrintfBackslashCUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			`printf: \c: the shells disagree here and no dialect was chosen`))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// PrintfQuoteStyle is how `%q` quotes a word so the shell can read it back.
type PrintfQuoteStyle int

const (
	// PrintfQuoteUnspecified is no answer, and is refused like any other.
	PrintfQuoteUnspecified PrintfQuoteStyle = iota
	// PrintfQuoteBackslash escapes each character that needs it: bash, zsh.
	PrintfQuoteBackslash
	// PrintfQuoteSingle wraps the word in single quotes: ksh93.
	PrintfQuoteSingle
	// PrintfQuoteAbsent is a dialect without `%q` at all: dash, which calls
	// it an invalid directive like any other conversion it does not have.
	PrintfQuoteAbsent
)

func (p PrintfQuoteStyle) String() string {
	switch p {
	case PrintfQuoteBackslash:
		return "backslash"
	case PrintfQuoteSingle:
		return "single quoted"
	case PrintfQuoteAbsent:
		return "absent"
	}
	return "unspecified"
}

// quoteStyle resolves the axis, and only for a `%q` that is actually there.
func (r *Runner) quoteStyle() PrintfQuoteStyle {
	p := r.sem().PrintfQuote
	if p == PrintfQuoteUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			"printf: %q: the shells disagree here and no dialect was chosen"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// KillStatusPolicy is what `kill` reports when its targets disagreed.
type KillStatusPolicy int

const (
	// KillStatusUnspecified is no answer, and is refused like any other.
	KillStatusUnspecified KillStatusPolicy = iota
	// KillStatusAnyFailure reports 1 if any target failed: dash, ksh93.
	KillStatusAnyFailure
	// KillStatusAnySuccess reports 0 if any target was signaled: bash.
	KillStatusAnySuccess
	// KillStatusFailureCount reports how many failed: zsh.
	KillStatusFailureCount
)

func (k KillStatusPolicy) String() string {
	switch k {
	case KillStatusAnyFailure:
		return "any failure"
	case KillStatusAnySuccess:
		return "any success"
	case KillStatusFailureCount:
		return "failure count"
	}
	return "unspecified"
}

// killStatusPolicy resolves the axis, and only where the targets actually
// disagreed — `kill $$` needs no answer from anyone, and neither does a
// command whose every target failed for the same reason.
func (r *Runner) killStatusPolicy() KillStatusPolicy {
	p := r.sem().KillStatus
	if p == KillStatusUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			"kill: some of these targets: the shells disagree here and no dialect was chosen"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// killListAcceptsName resolves the axis, and only for an argument that is
// actually a name — `kill -l 9` needs no answer from anyone.
func (r *Runner) killListAcceptsName() Answer {
	a := r.sem().KillListAcceptsName
	if a == Unspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			"kill -l: a signal name: the shells disagree here and no dialect was chosen"))
		r.status = 2
		r.unspecified = true
	}
	return a
}

// BracketPolicy is what an unterminated bracket expression means.
type BracketPolicy int

const (
	// BracketUnspecified is no answer, and is refused like any other.
	BracketUnspecified BracketPolicy = iota
	// BracketLiteral treats the `[` as an ordinary character: bash, ksh93.
	BracketLiteral
	// BracketNoMatch treats it as a class that matches nothing: dash.
	BracketNoMatch
	// BracketBadPattern rejects the pattern: zsh.
	BracketBadPattern
)

func (b BracketPolicy) String() string {
	switch b {
	case BracketLiteral:
		return "literal"
	case BracketNoMatch:
		return "no match"
	case BracketBadPattern:
		return "bad pattern"
	}
	return "unspecified"
}

// bracketPolicy resolves the axis, refusing when no dialect answered — and
// only for a pattern that actually has an unterminated bracket, which is the
// rule the caret axis uses for the same reason.
func (r *Runner) bracketPolicy() BracketPolicy {
	p := r.sem().UnterminatedBracket
	if p == BracketUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			"an unterminated bracket expression: the shells disagree here and no dialect was chosen"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// ask reads one axis.
//
// An unspecified axis is refused rather than guessed, and the refusal names
// it, because "this script depends on something the shells disagree about"
// is a useful thing to be told and a silent wrong answer is not.
//
// Callers consult an axis only when the input actually depends on it — `echo
// hi` does not ask about escapes and `echo 'a\tb'` does — which is what keeps
// the core usable rather than refusing everything.
// caretNegates resolves the `[^…]` axis, and only for a pattern that actually
// uses it — so a dialect is never questioned about syntax the pattern does not
// contain, and `[abc]` needs no answer from anyone.
func (r *Runner) caretNegates(pattern string) bool {
	if !strings.Contains(pattern, "[^") {
		return false
	}
	return r.ask(r.sem().BracketCaretNegates, "`^` negating a bracket class")
}

// matchPatternR is matchPattern with the caret axis resolved from the dialect.
// condition says the pattern stands inside `[[ ]]`, which one dialect reads by
// different rules from a `case` pattern.
func (r *Runner) matchPatternR(pattern, s string, condition bool) bool {
	o := patternOpts{
		caret:      r.caretNegates(pattern),
		group:      r.dialect().PatternAlternation,
		quantified: r.readsQuantifiedGroups(condition),
	}
	var bad bool
	if hasUnterminatedBracket(pattern) {
		o.bracket, o.bad = r.bracketPolicy(), &bad
	}
	matched := matchPattern(pattern, s, o)
	if bad {
		// zsh abandons the script rather than failing the match.
		// Measured: zsh abandons the script here with status 0, and with 1
		// when the same pattern fails against the filesystem. Both are
		// zsh's, and neither is guessable from the other.
		r.fatalPattern(pattern, 0)
		return false
	}
	return matched
}

// fatalPattern reports a pattern the dialect rejects outright.
func (r *Runner) fatalPattern(pattern string, status int) {
	r.diagf("%s\n", Wording(r.diag().BadPattern, "bad pattern: %s", pattern))
	r.status = status
	r.ctl = controlExit
}

func (r *Runner) ask(a Answer, axis string) bool {
	switch a {
	case Yes:
		return true
	case No:
		return false
	}
	r.diagf("%s: the shells disagree here and no dialect was chosen\n", axis)
	r.status = 2
	r.unspecified = true
	return false
}
