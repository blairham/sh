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

	// EchoOptions is the set of letters `echo` reads as options: `n` for
	// every shell measured, `e` everywhere but dash, `E` in bash and zsh
	// alone. A word carrying any other letter is not an option at all — the
	// whole word becomes an operand, which is unanimous and is why `echo
	// -nq hi` prints `-nq hi` in all four. Empty means `n`.
	EchoOptions string
	// EchoLastEscapeFlagWins decides `echo -e -E`: bash lets the last flag
	// win and prints the backslashes, zsh lets -e win whatever the order.
	// Reached only when -e came first — the other order agrees everywhere —
	// and only in a dialect whose EchoOptions has both letters.
	EchoLastEscapeFlagWins Answer
	// EchoExpandsHexEscapes admits `\xHH` alongside the XSI set: bash and
	// zsh do, dash and ksh93 print it as written.
	EchoExpandsHexEscapes Answer
	// EchoExpandsEscEscape admits `\e` and `\E` for the escape character:
	// everyone with escapes but dash, whose set is the XSI list alone.
	EchoExpandsEscEscape Answer
	// EchoInterpretsEscapes expands backslash escapes in `echo` without -e.
	// True in dash and zsh, false in bash and ksh93 — a grouping no other
	// axis produces.
	EchoInterpretsEscapes Answer

	// ReadOptions is the set of letters `read` takes, a `:` after a letter
	// marking one whose argument follows it — the getopts convention, the
	// same one the shared option reader speaks. The letters are the
	// dialect's own: bash spells the array option `-a` and takes the array's
	// name as the option's argument, ksh93 and zsh spell it `-A` and take
	// the name as the first operand, and zsh reads `-n` as a flag where bash
	// and ksh93 read a count after it. Empty means `r`, the one letter POSIX
	// gives the builtin.
	ReadOptions string
	// ReadPartialCountSucceeds decides `read -n N` when the input ends
	// after some but fewer than N characters: ksh93 calls the read a
	// success and bash reports 1, both keeping what arrived. Asked only
	// there — a full count, a delimiter, or a wholly empty input answers
	// the same way everywhere.
	ReadPartialCountSucceeds Answer
	// ReadExactCountKeepsPartial decides what `read -N N` leaves behind
	// when the input ends short: bash assigns the partial text and ksh93
	// assigns nothing, both reporting 1. Asked only on that partial text.
	ReadExactCountKeepsPartial Answer

	// BuiltinWriteErrorFailsTheCommand makes a builtin whose output write
	// failed — into a descriptor closed with `>&-`, most plainly — report
	// status 1. True in bash, dash and ksh93; zsh keeps the builtin's own
	// status and quietly loses the text.
	//
	// Whether anything is *said* about it is the dialect's wording —
	// Diagnostics.BuiltinWriteError — not a second axis: bash and dash
	// complain, ksh93 fails silently, and zsh has nothing to word because it
	// does not fail. Asked only when a write has actually failed, so `echo
	// hi` on an open stream needs no dialect.
	BuiltinWriteErrorFailsTheCommand Answer

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
	// ExitInTrapReportsEarlierStatus makes a bare `exit` in an EXIT trap
	// report the status the shell had when the trap began, rather than that
	// of the trap's own last command.
	//
	//	trap "false; exit" 0; true
	//
	// is 0 in bash, dash and ksh93 and 1 in zsh. Only the bare form: `exit 7`
	// is 7 everywhere, and a trap that does not exit at all leaves the
	// script's status alone in all four.
	//
	// Found on an installed script — /usr/bin/bzless traps `stty …; exit` on
	// EXIT, and the `stty` failing made the script exit 1 where every shell
	// exits 0. A wrong exit status is what a caller branches on, so this is
	// the quiet kind of difference.
	ExitInTrapReportsEarlierStatus Answer

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

	// NoglobLetterIsF puts `f` in `$-` while noglob is on, which is the
	// letter POSIX gives it and what bash, dash and ksh93 report. False in
	// zsh, which reports the capital: `-F` is the short option that means
	// noglob there, `-f` being about startup files — the same split
	// SetFTurnsOffGlobbing records, seen from the reading side.
	NoglobLetterIsF Answer

	// DefaultOptionLetters is what `$-` starts with before the script has
	// set anything: the single-letter options a shell turns on at startup.
	// Measured identical under `-c`, a script file and standard input —
	// bash and ksh93 report `hB`, zsh `569X`, dash nothing at all.
	//
	// The letters that describe the invocation route rather than an option
	// a script could set — `c` for a command string, `s` for standard
	// input, `i` for interactive — are not modeled: they are the front
	// end's to know, not the same on any two shells, and no deterministic
	// assertion can be written against them. A script's own `case $- in
	// *e*)` never depends on them.
	DefaultOptionLetters string

	// ArithIntegerOperatorRefusesFloat rejects a float where only an integer
	// will do — `7 % 2.5`, `1.5 & 1`, a shift. ksh93 says yes and refuses;
	// zsh says no and truncates. It does not arise in a shell without floats,
	// which is why bash and dash leave it unanswered.
	ArithIntegerOperatorRefusesFloat Answer

	// ArithNegativeExponentIsError refuses `2**-1` rather than answering
	// with a float. bash says yes and stops the expression; ksh93 and zsh
	// say no and answer 0.5. It does not arise where the grammar has no
	// `**`, which is why dash leaves it unanswered.
	ArithNegativeExponentIsError Answer

	// RegexQuotingMakesLiteral treats a quoted right operand of `=~` as a
	// literal string. True in bash alone; ksh93 and zsh keep it a regex, so
	// quoting a regex is unportable in either direction.
	RegexQuotingMakesLiteral Answer

	// LastPipelineElementInCurrentShell runs the last command of a pipeline
	// in this shell, so `echo x | read v` sets v. True in ksh93 and zsh.
	LastPipelineElementInCurrentShell Answer

	// RedirectTargetIsAnOrdinaryWord expands a redirection's target the way
	// an argument is expanded — split into fields and matched as a pattern —
	// and requires the result to be exactly one word. True in bash alone:
	//
	//	e="a b"; echo hi > $e      bash refuses; the rest write to `a b`
	//	e="x*";  echo hi > $e      bash refuses where two files match, and
	//	                           writes to the match where one does; the
	//	                           rest create a file named `x*`
	//
	// The other three expand it and stop there: no splitting, no matching,
	// whatever it came to is the name. A tilde expands either way.
	//
	// Doing bash's expansion and then quietly taking the first field is the
	// answer no shell gives, and it is the one this had: `> $e` wrote to `a`,
	// and `> $e` with a pattern truncated whichever file happened to match.
	RedirectTargetIsAnOrdinaryWord Answer

	// TypePrintsFunctionBody makes `type name` follow "name is a function"
	// with the function itself, reformatted. True in bash alone; the other
	// three stop at the sentence.
	//
	// Answered and not yet honored: printing a body needs a printer for the
	// syntax tree, which this does not have — see the `type` builtin, which
	// refuses rather than printing the sentence and silently dropping the
	// half of the answer that was asked for.
	TypePrintsFunctionBody Answer

	// TypeEndsOptionsWithDashDash makes `type -- name` skip the `--`. True
	// in bash, ksh93 and zsh; dash has no options for it at all, so `--` is
	// a name there and gets answered as one before the real names are.
	TypeEndsOptionsWithDashDash Answer

	// TypeNamesTheKindWithDashT gives `type` its `-t`, which answers one
	// bare word per name — keyword, function, builtin or file — and prints
	// nothing at all for a name it cannot account for, only the failing
	// status. The scripted form of the question: a word to compare against
	// rather than a sentence to parse. True in bash alone; ksh93 and zsh
	// refuse the letter the way they refuse any option they do not have,
	// and dash reads it as a name like the rest of its operands.
	TypeNamesTheKindWithDashT Answer

	// ArraysAreSparse makes an unassigned subscript no element at all, so
	// `a=(x); a[5]=y` is an array of two. True in bash and ksh93; zsh reads
	// the whole extent and finds the gap empty, giving five.
	//
	// The store is sparse either way — only the reading differs — so this is
	// asked when an array *has* a gap and never otherwise, which is almost
	// every array there is.
	ArraysAreSparse Answer

	// ExportCarriesFunctions gives `export` its `-f`, which writes a
	// function into a child's environment. True in bash alone: the other
	// three have no way to carry a function at all, and each rejects the
	// option as an option — two of them fatally.
	ExportCarriesFunctions Answer

	// AnnouncesBackgroundJob prints the job number and the process id when a
	// job is backgrounded, before the next prompt. True in bash, ksh93 and
	// zsh; dash says nothing at all.
	//
	// Only ever at a prompt: no shell announces one to a script.
	AnnouncesBackgroundJob Answer

	// UnsetFunctionChecksTheName judges the operand `unset -f` was given as
	// a name, and refuses one that could not be a function name. True in
	// ksh93 alone.
	//
	// Not the same question as the one below, and measured to be: ksh93
	// refuses `1x` and is quiet about a well formed name that is not
	// defined, where zsh is the other way round.
	UnsetFunctionChecksTheName Answer

	// UnsetFunctionReportsMissing complains when `unset -f` names a function
	// that is not defined. True in zsh alone, which reports it about any
	// name it does not hold, well formed or not.
	//
	// Unsetting a function that *is* there is quiet in all four.
	UnsetFunctionReportsMissing Answer

	// LoneDashIsAnOption eats a `-` given to a builtin on its own instead of
	// passing it on as an operand. True in zsh alone.
	//
	// Only visible once something looks at the operands. `unset -` is quiet
	// in bash because its bare form validates nothing, not because the dash
	// was eaten — `unset -v -`, which does validate, names the dash there.
	// zsh reports `not enough arguments` instead, because after the dash is
	// eaten there is nothing left to unset.
	LoneDashIsAnOption Answer

	// ReturnOutsideAFunctionIsRefused reports a `return` that has nothing to
	// return from and carries on, instead of ending the script with the
	// status it was given. True in bash alone.
	//
	// Asked only where there is nothing to return from. Inside a function
	// and inside a sourced file all four obey it, so the question is about
	// the one case they split on.
	ReturnOutsideAFunctionIsRefused Answer

	// BadSetOptionNameFatal ends the script when `set -o` is given a name
	// this shell does not have. True in dash, ksh93 and zsh.
	//
	// Not the same question as BadOptionToSpecialBuiltinFatal, and measured
	// rather than assumed to be: a bad option *letter* to the same builtin
	// is fatal in only two of them, and zsh does not so much as complain
	// about `set -Q`. So one shell treats an unknown name as worse than an
	// unknown letter, which is why this is a field of its own.
	BadSetOptionNameFatal Answer

	// CdLastPathOptionWins lets the last of `cd -L` and `cd -P` decide.
	// True in bash, dash and ksh93 — `cd -P -L` is logical there. zsh gives
	// `-P` the answer wherever it appears, so both orders resolve.
	//
	// Asked only when both were given, because that is the only time the
	// two rules differ.
	CdLastPathOptionWins Answer

	// CdRefusesUnknownOption refuses a letter `cd` does not have rather than
	// reading the word as a directory. True in bash, dash and ksh93; zsh
	// looks for somewhere called `-Q` instead, because its `cd` takes two
	// operands — `cd old new` — and a leading dash word is the first of
	// them there.
	//
	// Only about an *unknown* letter. `-L` and `-P` are options in all four
	// and are not asked about.
	CdRefusesUnknownOption Answer

	// ChildInterruptEndsTheScript stops the script when a child was ended by
	// an interrupt, instead of carrying on with the next command. True in
	// ksh93 alone, and for SIGINT alone — measured across QUIT, TERM, HUP,
	// USR1 and PIPE, every one of which it carries on from.
	//
	// It ends the whole script rather than the construct around it: from
	// inside a loop, the loop and everything after it are abandoned too.
	// The status is 128 plus the signal, which is not the same shell's
	// answer for a command killed by one — that is 256 plus it.
	ChildInterruptEndsTheScript Answer

	// ReportsAnyKilledPipelineElement remarks on a signal that ended an
	// element of a pipeline other than the last. True in dash alone.
	//
	// bash and ksh93 report only the element whose status the pipeline
	// takes: `sh -c 'kill -ABRT $$' | cat` is silent in both, and the same
	// command as the *last* element is not. dash says the same thing
	// wherever the element stands.
	//
	// Unreachable in zsh, which says nothing about a killed command at all,
	// so the question never arises there.
	ReportsAnyKilledPipelineElement Answer

	// ReportsACommandKilledBySignal says out loud that a signal ended a
	// command, rather than leaving the status to carry it alone. True in
	// bash, dash and ksh93; zsh says nothing — measured with a terminal as
	// well as without one, so it is not the prompt-only rule that governs a
	// background job's announcement.
	//
	// Not asked for the two signals nothing reports. ^C and a broken pipe
	// are how a command is meant to end, and all four stay quiet about
	// those, so there is no disagreement there to put to a dialect.
	ReportsACommandKilledBySignal Answer

	// JobsShowBackgroundCommand puts the command of a `&` job in a `jobs`
	// listing. True in bash and zsh; dash prints an empty column there and
	// ksh93 a placeholder.
	//
	// Only for a `&` job, which is the whole reason this is not a question
	// about rendering a command at all: both of the shells that leave it out
	// here *do* print the command of a job they stopped themselves. They
	// kept nothing for this kind of job, and the listing is where that shows.
	JobsShowBackgroundCommand Answer

	// JobsListNewestFirst puts the most recent job at the top of a `jobs`
	// listing. True in dash and ksh93; bash and zsh list oldest first.
	//
	// A two-two split, which is the usual shape here and the reason this is
	// a field rather than a choice: there is no ordering of the shells that
	// explains it.
	JobsListNewestFirst Answer

	// JobsListFinishedJobs includes a job that has already ended in a `jobs`
	// listing, once, before forgetting it. True in bash, dash and ksh93; zsh
	// drops a finished job without ever mentioning it.
	//
	// The forgetting is not the axis and is not optional: every shell in the
	// panel reports a finished job at most once, so a second `jobs` shows
	// nothing. A shell that kept them would grow a listing for the length of
	// the session.
	JobsListFinishedJobs Answer

	// ShiftPastEndFatal ends a non-interactive shell when `shift` runs off
	// the end. True in dash and ksh93.
	ShiftPastEndFatal Answer
	// ReadonlyReassignmentByDeclarationFatal ends the script when a
	// declaration utility assigns to a readonly name — `export x=2`,
	// `typeset x=2`. True in dash, ksh93 and zsh; bash reports it and
	// carries on.
	//
	// A different set of shells from the plain assignment above, which is
	// what makes it a question of its own: bash stops for `x=2` given as an
	// argument and never stops for this one.
	ReadonlyReassignmentByDeclarationFatal Answer

	// ReadonlyReassignmentFatalFromCommandString is the same question for a
	// shell whose program came from an argument rather than from a file.
	//
	// One dialect answers the two differently: `bash -c 'readonly x=1;
	// x=2; echo after'` stops and exits 1, and the same three lines in a
	// file print `after` and exit 0. The other three are fatal either way.
	//
	// Asked only for an assignment standing as a command of its own. The
	// dialect that splits is not fatal for `export x=2` or `x=2 cmd` by
	// either route, so those keep the answer above.
	ReadonlyReassignmentFatalFromCommandString Answer

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
	// ValuelessDeclarationHidesTheOuterValue makes `local u` in a function
	// hide any outer `u` — the local exists unset, so `${u-UNSET}` fires the
	// default even when the caller had a value. Reached only when
	// DeclaredNameWithoutValueIsEmpty said no: a name declared *empty* hides
	// the outer value by having one of its own.
	//
	// bash hides it, and so does ksh93's `typeset` in a keyword function;
	// dash leaves the caller's value showing through until the first
	// assignment. This is the shape used to declare a local before
	// assigning it conditionally, so the difference is silent: the function
	// reads the caller's value where it expected nothing.
	ValuelessDeclarationHidesTheOuterValue Answer
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

	// DeclareListing is the shape of what `declare -p` and `typeset -p`
	// write back. Three engines rather than two answers — see
	// DeclarationListingForm.
	DeclareListing DeclarationListingForm

	// DeclareValueQuoting is how a listed declaration spells its value. A
	// field of its own over the shared vocabulary because it does not follow
	// the dialect's other listings: the engine that single-quotes its
	// aliases and traps double-quotes its declarations.
	DeclareValueQuoting ListingQuotingStyle

	// DeclarePrintReportsAMissingName makes `typeset -p nosuch` say so and
	// fail. bash and zsh report it (with their own wording — see
	// Diagnostics.DeclareNoSuchVariable) and answer 1 even when other names
	// listed fine; ksh93 prints nothing for the missing name and answers 0.
	DeclarePrintReportsAMissingName Answer

	// SelectLayout is how `select` draws its menu. Three engines rather than
	// two answers, which is why it has its own type.
	SelectLayout SelectMenuLayout

	// AliasParsesOptions lets `alias` read leading `-` words as options. True
	// in bash, ksh93 and zsh; dash reads none, so `alias -p` is a name there
	// and the answer is "-p not found" rather than a refusal.
	AliasParsesOptions Answer

	// AliasHasPrintOption gives `alias` a `-p`, which prints the listing with
	// `alias ` in front of every line. bash and ksh93 have it — and it is
	// what bash's plain listing already looks like, so it is only visible in
	// ksh93. dash parses no options for `alias` at all, so `-p` is a *name*
	// there and the answer is "not found"; zsh has options and refuses it.
	AliasHasPrintOption Answer

	// AliasReportsNotFound says something when `alias` is given a name the
	// table does not hold. True in bash, dash and ksh93; zsh reports 1 and
	// prints nothing.
	AliasReportsNotFound Answer

	// UnaliasReportsNotFound is that question for `unalias`, and the panel
	// does not pair the two: ksh93 complains about `alias nope` and is silent
	// about `unalias nope`, and zsh does exactly the reverse. One field could
	// not say that.
	UnaliasReportsNotFound Answer

	// AliasNotFoundStatusCounts makes `alias` report how many names it could
	// not find rather than a plain 1: `alias n1 n2 n3` is 3 in ksh93 and 1 in
	// the other three.
	//
	// About `alias` alone — ksh93's own `unalias` answers 1 however many were
	// missing — so it is asked where the count is known and not where the
	// complaint is printed.
	AliasNotFoundStatusCounts Answer

	// UnaliasAllRefusesOperands makes `unalias -a name` an error that clears
	// nothing. zsh alone: "-a: too many arguments", status 1, table intact.
	// The other three take the `-a`, ignore the names and empty the table.
	UnaliasAllRefusesOperands Answer

	// AliasQuoting is how a value is spelled in a listing — four engines, no
	// two alike. See ListingQuotingStyle.
	AliasQuoting ListingQuotingStyle

	// TrapQuoting is that same question asked of `trap`, and it is a
	// separate field because one dialect answers the two differently: zsh
	// writes an alias holding a tab as `$'a\tb'` and a trap holding one as
	// a plainly quoted `'a<tab>b'`.
	TrapQuoting ListingQuotingStyle

	// TrapBodyLine is which lines a diagnostic from inside a trap's body
	// names. See TrapBodyLineStyle.
	// TrapActionIsParsedWhenSet reads a trap's action when the trap is set
	// rather than when it fires, and refuses a trap whose action will not
	// parse.
	//
	// zsh alone. The other three store the text: `trap "if" EXIT` is taken
	// and complains at the end, and `trap "if" INT` is taken and never
	// complains at all, because the trap never fires.
	TrapActionIsParsedWhenSet Answer

	// TrapBodyRunsWhatParsed runs each line of a trap's body as it parses,
	// so the part before a syntax error has already run by the time the
	// error is reported.
	//
	// bash and dash do — `trap "echo a
	// if" EXIT` prints `a` and then complains. ksh93 reads the whole body
	// first and prints nothing. zsh answers no by construction rather than
	// by measurement: it reads the action when the trap is set, so by the
	// time a trap fires the whole body has parsed and there is no partial
	// run to have. The two answers cannot be told apart there.
	TrapBodyRunsWhatParsed Answer

	// TrapParseFailureNamesWhereItFired puts the runtime location in front
	// of a trap body's parse failure — where the trap fired — rather than
	// the line the parse gave out on.
	//
	// ksh93 alone, and the two are different numbers: a body set on line 2
	// and fired from line 5 reports `w5.sh: line 5: syntax error at line 6`.
	// bash and dash name the parse position in both places. zsh is not
	// asked, because it reads the action when the trap is set and never
	// reaches a parse failure at fire time.
	TrapParseFailureNamesWhereItFired Answer

	// SymbolicMaskTakesMoreThanOneOperator lets one `umask` clause turn on
	// several: `umask u+rw-x` is 0122 from 022 in three of the four. zsh
	// takes a single operator per clause and names the second one.
	SymbolicMaskTakesMoreThanOneOperator Answer

	// SymbolicMaskSetsWithoutAWho takes `umask -- =w`, where `=` has no who
	// before it and means all three groups. Three of the four do; zsh wants
	// one, and names a character that is not in the input when it does not
	// get one.
	SymbolicMaskSetsWithoutAWho Answer

	// SymbolicMaskWhoAloneSetsIt reads `umask g` as `umask g=`, denying that
	// group everything. ksh93 alone. bash and dash refuse it, and zsh
	// answers it with the complaint it gives a number it could not read.
	SymbolicMaskWhoAloneSetsIt Answer

	// SymbolicMaskTakesTheSetuidLetter accepts `s` in a clause, which
	// changes no bits — a umask has no setuid bit to deny — and is accepted
	// by three of the four all the same. zsh refuses it.
	SymbolicMaskTakesTheSetuidLetter Answer

	// SymbolicMaskTakesTheStickyLetter is the same question about `t`, and a
	// different set of shells: bash and ksh93 take it, dash and zsh do not.
	// Two fields because the two letters are not answered together.
	SymbolicMaskTakesTheStickyLetter Answer

	// ShiftReadsOptions reads a leading `-` word that is not a number as an
	// option rather than as the count.
	//
	// ksh93 and zsh do, and refuse it as one; bash and dash read it as the
	// count and complain about the number. Same input, two different kinds
	// of complaint — and both are fatal in the dialects where a special
	// builtin's failure is, which `shift` is.
	ShiftReadsOptions Answer

	// WaitReadsOptions reads a leading `-` word as an option rather than as
	// a job to wait for. Three of the four do; zsh has none, and answers
	// `wait -x` with the job it could not find.
	WaitReadsOptions Answer

	// CommandRejectsUnknownOption refuses a leading `-` word that is not one
	// of `command`'s own options, rather than taking it as the command.
	//
	// All four read `-v` and `-p`. bash and dash refuse anything else;
	// ksh93 and zsh stop reading options there, so `command -x ls` is
	// `command not found: -x`. The same shape printf already has.
	CommandRejectsUnknownOption Answer

	// GetoptsRejectsUnknownOption is the same question for `getopts`, which
	// has no options at all here — so any leading `-` word is the one being
	// asked about, and it would otherwise be the optstring.
	//
	// bash alone refuses it: `getopts -a x` is `-a: invalid option` there and
	// an optstring of `-a` in dash and zsh. ksh93 has `-a` for real, and is
	// left reading none for the same reason as above.
	GetoptsRejectsUnknownOption Answer

	// ShiftCountIsArithmetic reads `shift`'s operand as an expression rather
	// than as a plain number: `shift 1+1` moves two and `shift n` moves
	// whatever n holds.
	//
	// ksh93 and zsh do. An unset name is zero in an expression, so
	// `shift abc` shifts nothing and succeeds there, where bash and dash
	// call it a number they cannot read.
	ShiftCountIsArithmetic Answer

	// ReportsAKilledCommandInACommandSubstitution remarks on a command that
	// a signal ended inside `$(…)`.
	//
	// bash does not, and does remark on the same command inside `( … )`, so
	// this is not the subshell question in another spelling. dash and ksh93
	// report it wherever it happened; zsh remarks on none of them and never
	// reaches this.
	//
	// Asked only inside a substitution, so the three dialects that answer
	// the wider question the same way everywhere are not asked twice.
	ReportsAKilledCommandInACommandSubstitution Answer

	TrapBodyLine TrapBodyLineStyle

	// ExitTrapFiresPastTheEnd counts the EXIT trap as having fired on the
	// line after the script's last, rather than on its first.
	//
	// Only asked by a dialect whose TrapBodyLine needs a firing line at all,
	// and only for EXIT, which has no line of its own. zsh says yes: its
	// EXIT trap reports the line the parser stopped at. ksh93 says no, which
	// makes an EXIT body read like a small script of its own.
	ExitTrapFiresPastTheEnd Answer
	// SelectPromptNeedsTerminal withholds PS3 unless the input is a terminal.
	// ksh93 alone says yes, which is why a ksh93 script's transcript has the
	// menu in it and no prompt.
	SelectPromptNeedsTerminal Answer
	// SelectTakesUnterminatedReply counts a final reply that has no trailing
	// newline. zsh alone: `printf 2 | sh -c 'select x in a b; do ...'` picks
	// `b` there, and bash and ksh93 ignore the line and end the loop with 1.
	//
	// The same question `read` answers, and the opposite outcome — the panel
	// is unanimous for `read` and split here, so that one is the core's
	// behavior and this one is an axis. Reachable only from a pipe or a file,
	// since a terminal ends every line.
	SelectTakesUnterminatedReply Answer

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

	// HashReportsAMissingName has `hash name` complain and answer 1 when
	// the name resolves to nothing. bash, dash and zsh do; ksh93 — whose
	// hash is an alias for `alias -t` — says nothing and reports success.
	HashReportsAMissingName Answer

	// HashSearchesPathAlone counts only what PATH holds: zsh answers
	// `hash shift` with "no such command" where the other three accept a
	// builtin or a function as hashable. Measured with `shift`, which no
	// PATH carries — `cd` was the contaminated probe, macOS ships
	// /usr/bin/cd.
	HashSearchesPathAlone Answer

	// PunctuatedFunctionNameIsRefused stops the script when a function
	// whose name carries `-` or `.` is defined. ksh93 alone: bash and zsh
	// define and run it, and dash never parses the definition at all.
	PunctuatedFunctionNameIsRefused Answer

	// DirectoryOnPathIsACandidate keeps a directory the PATH search found as
	// the failed candidate when no later entry runs, so the report names the
	// directory rather than saying the command was never found.
	//
	// Every shell measured continues the search past the directory — that is
	// unanimous, and is what makes a shim directory early on PATH work at
	// all. They part ways only when nothing later matches: bash reports the
	// name as not found at all (status 127), where dash, ksh93 and zsh
	// report the directory they could not run. dash alone keeps 127 for the
	// status even then, which is DirectoryOnPathStatus's question.
	DirectoryOnPathIsACandidate Answer

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

	// TrapParsesOptions reads a leading `-` word as an option rather than as
	// the action to run.
	//
	// Three of the four do. zsh does not, so `trap -p` sets a trap whose
	// action is the word `-p` and the failure surfaces later, when it fires
	// — which is what this shell did for every dialect before there were
	// options here at all.
	//
	// Asked of the letters a dialect knows as much as of the ones it does
	// not, because zsh takes `-p` as the action just as it takes `-Q`. A
	// lone `-` is trap's own word for "put it back" and is never an option,
	// and `--` ends them in all four.
	TrapParsesOptions Answer

	// TrapPrintsWithP makes `trap -p` write the traps currently set, and
	// `trap -p condition ...` only the named ones. bash and ksh93 have it;
	// dash rejects the letter along with every other.
	TrapPrintsWithP Answer

	// TrapPrintsBareWithP makes `trap -P condition ...` write the action
	// alone, with no `trap --` around it. bash only, and it is the one option
	// that insists on an operand: printing all of them is `-p`'s job.
	TrapPrintsBareWithP Answer

	// TrapListsSignalsWithL makes `trap -l` list the signal names, the way
	// `kill -l` does. bash only — ksh93 refuses the letter.
	TrapListsSignalsWithL Answer

	// TrapPrintsBareWithConditions makes `trap -p condition ...` write the
	// action alone rather than the whole `trap -- action condition` line.
	//
	// ksh93 only, and it is why `-p` and bash's `-P` are two questions and
	// not one: ksh93 reaches bash's `-P` output through `-p` with an operand,
	// and has no `-P` at all.
	TrapPrintsBareWithConditions Answer

	// TrapOneArgumentIsACondition reads `trap EXIT` as "put EXIT back"
	// rather than as an action with no condition to attach it to.
	//
	// Three of the four do, which makes `trap EXIT` the short spelling of
	// `trap - EXIT`. ksh93 refuses the form and the refusal ends the script.
	TrapOneArgumentIsACondition Answer

	// TrapReportsAnUnknownSingleCondition complains when that one word turns
	// out not to name a condition. zsh says nothing — and does complain about
	// `trap : foo`, so this is the single-word form's own answer rather than
	// zsh declining to check at all.
	TrapReportsAnUnknownSingleCondition Answer

	// TrapSingleUnknownConditionIsUsage prints the usage line rather than
	// naming the word. bash does: with one word it cannot tell a misspelled
	// condition from an action someone forgot to give a condition to. dash
	// names the word the same way it does anywhere else.
	TrapSingleUnknownConditionIsUsage Answer

	// TrapHasErrCondition makes `trap … ERR` a condition rather than a
	// misspelled signal: the action runs after every command that fails
	// where `set -e` would judge it — with or without `set -e` on, which is
	// measured rather than assumed. dash alone refuses the name, with the
	// same words it refuses any other word that names no signal.
	TrapHasErrCondition Answer

	// TrapHasDebugCondition makes `trap … DEBUG` run the action before each
	// simple command. dash alone refuses the name.
	TrapHasDebugCondition Answer

	// TrapHasReturnCondition makes `trap … RETURN` a condition that fires
	// when a sourced file finishes, and when a function whose own body set
	// the trap returns. bash alone; the other three refuse the name the way
	// they refuse any word that names no signal.
	TrapHasReturnCondition Answer

	// ErrTrapRunsInsideFunctions fires the ERR trap for a failure inside a
	// function the trap was not set in. bash does not — there a function
	// does not inherit the ERR trap, so only the call itself is judged
	// where the trap can see it. ksh93 and zsh fire it inside too.
	//
	// The suppression is per *frame*, not per depth, which is measured: a
	// trap set inside a function fires in that function and at the top
	// level after it returns, and does not fire inside a sibling function
	// entered afterwards, though the sibling's own failing call still does.
	ErrTrapRunsInsideFunctions Answer

	// ErrTrapRunsInSubshells fires the ERR trap for a failure inside a
	// subshell or a command substitution. zsh alone: `trap 'echo E' ERR;
	// x=$(false; echo hi)` captures an E there and nowhere else. bash and
	// ksh93 reset the trap on the way into the child, the way they reset
	// every trap that is not ignored.
	ErrTrapRunsInSubshells Answer

	// DebugTrapRunsInsideCalls fires the DEBUG trap before commands inside
	// a function or a sourced file the trap was not set in. bash does not;
	// ksh93 and zsh do. Not the ERR axis under another name, and not only
	// because bash controls the two with different options: a sourced file
	// bounds DEBUG there and does not bound ERR — measured, with a
	// top-level trap of each, `false` inside a dotted file fires ERR and
	// the commands of the same file fire no DEBUG.
	DebugTrapRunsInsideCalls Answer

	// DebugTrapRunsInSubshells fires the DEBUG trap inside a subshell or a
	// command substitution. ksh93 and zsh do — a command substitution there
	// captures the handler's output into the variable — and bash does not,
	// which is a grouping ErrTrapRunsInSubshells does not have: ksh93
	// carries DEBUG into the child and not ERR.
	DebugTrapRunsInSubshells Answer

	// A subshell starts with the parent's handled traps back at their
	// defaults and only an ignored signal still ignored — POSIX, and
	// unanimous in the working state. What `trap` *lists* in the child is
	// where the panel splits, and it splits by the kind of boundary, so the
	// question is asked once per kind rather than once. The listing survives
	// until the child modifies a trap, at which point every shell that kept
	// it shows the child's own state instead — `(trap '' USR2; trap)` lists
	// USR2 and nothing the parent had.

	// SubshellKeepsTrapListing makes `trap` inside `( … )` or `$( … )` still
	// list the traps the parent had, though a handled one no longer fires —
	// the save=$(trap) idiom POSIX carves out, extended to the compound.
	// bash and ksh93; dash and zsh list only what survived the entry.
	SubshellKeepsTrapListing Answer

	// PipelineElementKeepsTrapListing is the same question asked of a
	// pipeline element that runs in a subshell environment, and the panel
	// pairs off the other way: bash and zsh keep the listing there, ksh93
	// and dash do not. `trap 'echo x' USR1; trap | cat` prints the trap in
	// bash and zsh and nothing in the other two — the shape issue #339
	// measured.
	PipelineElementKeepsTrapListing Answer

	// BackgroundJobKeepsTrapListing asks it of `… &`. bash alone: the other
	// three list nothing the parent had there.
	BackgroundJobKeepsTrapListing Answer

	// KeptTrapListingIncludesExit says a kept listing shows the parent's
	// EXIT trap alongside the signals. bash and ksh93 list it; zsh keeps a
	// pipeline element's listing and still drops EXIT from it. Unanswerable
	// where nothing is kept, so dash never reaches the question.
	KeptTrapListingIncludesExit Answer

	// SubshellHidesInheritedIgnoredTraps drops an *inherited* ignore from
	// the child's listing while the signal stays ignored in fact: zsh, where
	// `trap '' INT; (trap)` prints nothing and `(kill -INT $$; echo alive)`
	// still prints alive. The other three list what POSIX says is still a
	// current trap. An ignore the child sets itself is listed everywhere.
	SubshellHidesInheritedIgnoredTraps Answer

	// LocalOutsideAFunctionIsAnError refuses `local x=2` written where there
	// is no function to be local to.
	//
	// bash and dash refuse it, zsh takes it and sets a global instead. ksh93
	// has no `local` at all, so it never reaches the question.
	LocalOutsideAFunctionIsAnError Answer

	// LocalOutsideAFunctionIsFatal ends the script rather than carrying on
	// after that refusal. dash does; bash says the same thing and runs the
	// next command.
	LocalOutsideAFunctionIsFatal Answer

	// UmaskPrintsFourDigits writes the mask as four digits, always — `0022`
	// against zsh's `022`. True in bash, dash and ksh93.
	//
	// False is not "three digits". zsh writes a C octal literal with a
	// minimum of three, so the leading zero comes back as soon as the owner
	// group denies anything: `022` and `077`, but `0333` and `0777`. Reading
	// this as a flat three printed `333` where zsh prints `0333`.
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

	// UlimitBlockIsKilobyte counts `ulimit -c` and `-f` in 1024-byte blocks
	// rather than POSIX's 512. True only in bash.
	//
	// Measured rather than read: `ulimit -f 1` then writing until the kernel
	// objected. bash allowed 1000 bytes and refused 1200; dash, ksh93 and zsh
	// refused 600.
	UlimitBlockIsKilobyte Answer

	// UlimitHasResidentSet is `ulimit -m`. True in bash, dash and ksh93; zsh
	// has no such letter and reports it as a bad option.
	UlimitHasResidentSet Answer

	// UlimitHasProcessCount is `ulimit -u`. True in bash, ksh93 and zsh; dash
	// has no such letter.
	UlimitHasProcessCount Answer

	// UlimitSetsBothLimits lowers the hard limit along with the soft one when
	// neither -H nor -S was given — which is what makes `ulimit -t 3600`
	// irreversible. True in bash, dash and ksh93.
	//
	// False in zsh, which sets only the soft limit and leaves the hard one
	// where it was, so the same line there can be undone.
	UlimitSetsBothLimits Answer

	// BadOptionToSpecialBuiltinFatal ends the script when a special builtin is
	// given an option it does not have. True in dash and ksh93, which is the
	// POSIX rule that a special builtin's failure is fatal; bash and zsh
	// report it and carry on.
	//
	// A different question from BuiltinSyntaxErrorFatal, which is about text
	// that would not *parse* and is true for dash alone. Measured across
	// `export`, `readonly` and `unset`.
	BadOptionToSpecialBuiltinFatal Answer

	// BadNameToDeclarationFatal ends the script when `export` or `readonly` is
	// given an operand that is not a name. True in dash, ksh93 and zsh; bash
	// reports every bad operand, exports the well-formed ones and carries on
	// with a status of 1.
	BadNameToDeclarationFatal Answer

	// BadNameToUnsetFatal is that question again for `unset`, and is a
	// separate field because ksh93 answers the two differently: `export 1x`
	// ends the script there where `unset 1x` prints the same kind of
	// complaint, returns 1 and carries on.
	//
	// Not a question about `unset` being less special than the other two — a
	// bad *option* to ksh93's `unset` is fatal, which is what makes the split
	// about the kind of failure rather than about the builtin.
	BadNameToUnsetFatal Answer

	// DeclarationNameOperands says what may stand where `export` and
	// `readonly` want a name, beyond a plain name itself.
	//
	// zsh is the only one that takes anything more: the special parameters
	// are names to it, which is why `export -` is a complaint in three of the
	// four and not in the fourth.
	DeclarationNameOperands NameOperands

	// UnsetNameOperands is that question for `unset`, and is a separate field
	// because two dialects answer it differently from the declarations. zsh
	// answers the two with *disjoint* sets — `export ?` is fine there and
	// `unset ?` is not, while `unset 12` is fine and `export 12` is not — and
	// bash 5.3 checks a name for `export` and nothing at all for `unset`. One
	// field could not say either.
	UnsetNameOperands NameOperands

	// DeclarationTakesASubscript accepts `export a[0]` and `readonly a[0]`,
	// naming an element rather than a variable. ksh93 does; bash and dash
	// refuse it in the words they give any other bad name.
	//
	// A separate question from the name strictness above, because the answer
	// is per builtin: bash refuses it here and takes it for `unset`, and the
	// two builtins sit on different strictnesses in every shell, so no rule
	// over that strictness gives all four.
	DeclarationTakesASubscript Answer

	// UnsetTakesASubscript is the same question asked of `unset`, where the
	// answers are not the same: bash, ksh93 and zsh take it and dash refuses
	// it.
	UnsetTakesASubscript Answer
}

// NameOperands is what a builtin takes where it wants a name.
type NameOperands int

const (
	// NameOperandsUnspecified is no answer, and is refused like any other.
	NameOperandsUnspecified NameOperands = iota
	// PlainNamesOnly takes a name and nothing else: bash, dash and ksh93,
	// for all three builtins.
	PlainNamesOnly
	// NamesAndSpecialParameters also takes `?`, `*`, `@`, `#`, `!`, `-`, `$`
	// and `0`: zsh's `export` and `readonly`. Not the other digits — `export
	// 0` is quiet there and `export 1` is "not an identifier", which is the
	// difference between a special parameter and a positional one.
	NamesAndSpecialParameters
	// NamesAndPositionals also takes any all-digit operand: zsh's `unset`,
	// where `unset 12` is quiet. `0` falls in here too, so both of zsh's
	// answers take it and they agree on nothing else.
	NamesAndPositionals
	// AnythingIsAName checks nothing at all: bash 5.3's bare `unset`, which
	// is quiet about `unset 1x`, `unset "a b"` and `unset -- -` alike while
	// its `export` refuses every one of them.
	//
	// A change within bash rather than a difference between shells — bash
	// 3.2 refuses all three — so the `bash32` and `bash` columns of a corpus
	// case here disagree on purpose.
	AnythingIsAName
)

func (n NameOperands) String() string {
	switch n {
	case PlainNamesOnly:
		return "PlainNamesOnly"
	case NamesAndSpecialParameters:
		return "NamesAndSpecialParameters"
	case NamesAndPositionals:
		return "NamesAndPositionals"
	case AnythingIsAName:
		return "AnythingIsAName"
	}
	return "NameOperandsUnspecified"
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
		// POSIX has `echo` and `printf` exit greater than zero when "an
		// error occurred", and a write that went nowhere is one; dash
		// complies. zsh is the holdout, keeping status 0.
		BuiltinWriteErrorFailsTheCommand: Yes,
		// The XSI echo: -n alone, no \x, no \e. The letters the dialects
		// add are theirs to add.
		EchoOptions:           "n",
		EchoExpandsHexEscapes: No,
		EchoExpandsEscEscape:  No,
		// The POSIX read: -r alone. The counts, delimiters and descriptors
		// the dialects add are theirs to add, and the two count axes are
		// unreachable without the letters that raise them.
		ReadOptions:             "r",
		LengthOfSpecialIsCount:  Yes,
		ArithLeadingZeroIsOctal: Yes,
		// dash is the panel's POSIX-faithful member and the only one
		// exiting 2, so the POSIX preset follows it. The standard itself
		// requires only "greater than zero", which decides nothing.
		FatalErrorStatusIsOne:          No,
		ArithNameValueRecurses:         No,
		BraceExpansion:                 No,
		BracketCaretNegates:            No,
		ExitTrapIsFunctionLocal:        No,
		SignalHandlerSeesEarlierStatus: No,
		// POSIX says a bare `exit` reports the status of the last command,
		// and in an EXIT trap it names the value `$?` had when the trap was
		// entered — which is what three of the four do.
		ExitInTrapReportsEarlierStatus: Yes,
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
		// POSIX counts these in 512-byte blocks, and names neither -m nor -u.
		UlimitBlockIsKilobyte: No,
		UlimitHasResidentSet:  No,
		UlimitHasProcessCount: No,
		// POSIX sets both when neither is named.
		UlimitSetsBothLimits: Yes,
		// POSIX defines a pipeline's status as its last command's, and
		// offers nothing to change it.
		PipefailOption: No,
		// POSIX names the noglob letter itself: `set -f`, reported in `$-`
		// as `f`. Only zsh answers otherwise.
		NoglobLetterIsF:                            Yes,
		ArithInvalidOctalDigitIsError:              Yes,
		RegexQuotingMakesLiteral:                   No,
		LastPipelineElementInCurrentShell:          No,
		ShiftPastEndFatal:                          Yes,
		ReadonlyReassignmentFatal:                  Yes,
		ReadonlyReassignmentFatalFromCommandString: Yes,
		ReadonlyReassignmentByDeclarationFatal:     Yes,
		ArrayBaseIsZero:                            Yes,
		DollarZeroInFunctionIsFunctionName:         No,
		// A special builtin's failure is fatal to a non-interactive shell,
		// which the standard states outright. dash is the only member of the
		// panel that still does it, and the preset follows the standard
		// rather than the majority.
		BuiltinSyntaxErrorFatal: Yes,
		// POSIX makes a special builtin's failure fatal, and a bad option is
		// one.
		BadOptionToSpecialBuiltinFatal: Yes,
		// A bad name is a special builtin's failure too, and the standard
		// makes no exception for `unset`.
		BadNameToDeclarationFatal: Yes,
		BadNameToUnsetFatal:       Yes,
		// `local` needs a function to be local to, and saying so is what
		// three of the four do — the substrate keeps the answer it had
		// before the question was one.
		LocalOutsideAFunctionIsAnError: Yes,
		// The standard has no `local`; dash is the closest reading, and it
		// leaves the outer value visible until the first assignment.
		ValuelessDeclarationHidesTheOuterValue: No,
		// POSIX has `trap` save the action and execute it when the
		// condition arises, so the text is not read until then. Three of
		// the four agree; zsh reads it as the trap is set.
		TrapActionIsParsedWhenSet: No,
		// A shell runs what it has read rather than reading everything
		// first, which is unanimous for a script and is the same reading
		// applied to a trap's body. ksh93 is the one that reads a trap
		// body whole.
		TrapBodyRunsWhatParsed: Yes,
		// And it names where the failure was, not where the trap fired,
		// which is what three of the four do.
		TrapParseFailureNamesWhereItFired: No,
		// POSIX gives `trap` the signals and EXIT, and nothing else — ERR,
		// DEBUG and RETURN are conditions the shells added. dash still
		// refuses all three.
		TrapHasErrCondition:    No,
		TrapHasDebugCondition:  No,
		TrapHasReturnCondition: No,
		// POSIX resets a subshell's handled traps to their defaults, so the
		// listing shows what survived: the ignored signals, which the
		// standard still counts as traps in effect. The allowance for
		// save=$(trap) is a may, not a shall, and the preset follows the
		// rule rather than the allowance. KeptTrapListingIncludesExit is
		// left unanswered because a listing that is never kept never asks.
		SubshellKeepsTrapListing:           No,
		PipelineElementKeepsTrapListing:    No,
		BackgroundJobKeepsTrapListing:      No,
		SubshellHidesInheritedIgnoredTraps: No,
		// POSIX gives `command` -p and -v and gives `getopts` none, so a
		// leading `-` word is an option to the first and the optstring to
		// the second.
		CommandRejectsUnknownOption: Yes,
		GetoptsRejectsUnknownOption: No,
		// POSIX gives `umask` chmod's symbolic mode: a who list, then one
		// or more actions, each an operator and its permissions. So several
		// operators in a clause are allowed, an omitted who means all three,
		// `s` and `t` are permission characters like any other — and a who
		// with no action at all, or a letter that is neither, is not a
		// symbolic mode. The preset follows the grammar.
		SymbolicMaskTakesMoreThanOneOperator: Yes,
		SymbolicMaskSetsWithoutAWho:          Yes,
		SymbolicMaskWhoAloneSetsIt:           No,
		SymbolicMaskTakesTheSetuidLetter:     Yes,
		SymbolicMaskTakesTheStickyLetter:     Yes,
		// POSIX gives all three a *name*, and neither a special parameter
		// nor a positional one is a name — a positional has `shift` to
		// remove it.
		DeclarationNameOperands: PlainNamesOnly,
		UnsetNameOperands:       PlainNamesOnly,
		// The core has arrays — they are in the common denominator even
		// though POSIX has none — so `unset a[0]` names an element and
		// removes it, which is what three of the four do and the only part
		// of this anybody writes. A *declaration* still names a variable
		// rather than an element, which is bash's and dash's answer.
		DeclarationTakesASubscript: No,
		UnsetTakesASubscript:       Yes,
		DotMissingFileFatal:        Yes,
		DotWithNoOperandIsAnError:  Yes,
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
		// The standard's 126 is for a command that was found and cannot be
		// executed; a directory qualifies, and three of the four report it.
		DirectoryOnPathIsACandidate: Yes,
		// POSIX's hash concerns utilities, and dash — its closest reading —
		// counts builtins and functions too, and reports a missing name.
		HashReportsAMissingName: Yes,
		HashSearchesPathAlone:   No,
		// POSIX has no such names; refusal is one shell's own answer.
		PunctuatedFunctionNameIsRefused: No,
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
		// The run-time option folds exactly the two consumers this function
		// serves — `case` and `[[ ]]` — and neither of the others: pathname
		// expansion has a fold of its own, and parameter expansion stays
		// exact. Which is why the fold sits here and not in patternOpts.
		fold: r.MatchOption(MatchFoldsCase),
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
