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
// The axes were measured across four shells and produced groupings that
// overlap and contradict — no ordering of the shells explains the data,
// which is why this is a vector and not a level.
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
	// EchoExpandsEscEscape admits `\e` for the escape character in an `echo`
	// argument: bash 5.3 and zsh do, dash bash 3.2 and ksh93 write the two
	// characters.
	//
	// It is a separate axis from EchoExpandsCapitalEscEscape below because
	// the two shells that split the letters split them in *opposite*
	// directions, so no single answer describes either one — ksh93 has `\E`
	// and not `\e`, zsh has `\e` and not `\E` (#908). It is the same
	// asymmetry the `%b` site has, and it is asked separately there: see
	// PrintfBEscEscape.
	//
	// Asked only where an `echo` argument actually carries a `\e`.
	EchoExpandsEscEscape Answer
	// EchoExpandsCapitalEscEscape admits `\E` in an `echo` argument: bash 5.3
	// and ksh93 do, dash bash 3.2 and zsh write the two characters. See
	// EchoExpandsEscEscape for why the two letters are two questions.
	//
	// Asked only where an `echo` argument actually carries a `\E`.
	EchoExpandsCapitalEscEscape Answer
	// EchoInterpretsEscapes expands backslash escapes in `echo` without -e.
	// True in dash and zsh, false in bash and ksh93 — a grouping no other
	// axis produces.
	EchoInterpretsEscapes Answer

	// DollarSingleBackslashC is what `\c` means inside `$'…'`, and like the
	// `\c` of a printf format it is three different things rather than a
	// switch — see DollarSingleControlPolicy. Asked only for a `$'…'` that
	// has a `\c` in it.
	DollarSingleBackslashC DollarSingleControlPolicy
	// DollarSingleUnknownEscape is what becomes of a backslash before a
	// character no escape claims — `$'\q'` — see DollarSingleUnknownPolicy.
	// Asked only when such an escape is actually there.
	DollarSingleUnknownEscape DollarSingleUnknownPolicy
	// DollarSingleNulTruncates ends the decoded text at the first NUL an
	// escape produces, which is C-string semantics: `$'a\0b'` is `a` in
	// bash and ksh93 and the three bytes `a`, NUL, `b` in zsh.
	//
	// The truncation is the *span's*, not the word's: `$'a\0b'ccc` is `accc`
	// in the shells that truncate, so what is lost is the remainder of the
	// quoted text and nothing else. Reached only where a decoded escape
	// actually yields a zero byte — `\0`, an octal or hex escape that comes
	// to zero, and `\c@`, which is the same zero by another road.
	DollarSingleNulTruncates Answer

	// ReadOptions is the set of letters `read` takes, a `:` after a letter
	// marking one whose argument follows it — the getopts convention, the
	// same one the shared option reader speaks. The letters are the
	// dialect's own: bash spells the array option `-a` and takes the array's
	// name as the option's argument, ksh93 and zsh spell it `-A` and take
	// the name as the first operand, and zsh reads `-n` as a flag where bash
	// and ksh93 read a count after it. `-p` splits the same way: `p:` takes
	// a prompt for the terminal in bash and dash, a bare `p` names the
	// coprocess as the source in ksh93 and zsh. Empty means `r`, the one
	// letter POSIX gives the builtin.
	ReadOptions string
	// UnsetOptions is the same question asked of `unset`, spelled the same
	// way. The letters split three ways and no two dialects have the same
	// set: `-v` and `-f` are unanimous, `-n` is bash 5.3's and ksh93's — and
	// is refused by bash 3.2, dash and zsh — and `-m`, which reads its
	// operands as *patterns* and unsets every parameter whose name matches
	// one, is zsh's alone. Measured 2026-09-05 across the panel. Empty means
	// `vf`, which is what POSIX gives the builtin.
	UnsetOptions string
	// ReadZeroTimeout is what `read -t 0` asks of the stream — a poll, a
	// read of what is already waiting, or a read that commits once it has
	// begun. Asked only where `-t 0` is actually written; every other
	// timeout is a deadline and needs no answer. See ReadZeroTimeoutStyle
	// for the measurements.
	ReadZeroTimeout ReadZeroTimeoutStyle
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
	// ReadTimeoutKeepsWhatArrived decides what an expired `read -t` leaves
	// behind: bash assigns whatever had arrived before the deadline and
	// ksh93 and zsh touch no name at all, leaving the variable's earlier
	// value. Asked only on the timeout, never at end of input, where all
	// three assign.
	//
	// The distinction the wording is careful about is that bash does not
	// *clear* the variable — it assigns a short read, and clearing is only
	// what that looks like when nothing had arrived. Measured with a stream
	// that delivers half a line and then stalls:
	//
	//	{ printf part; sleep 0.5; printf 'ial\n'; } |
	//	  sh -c 'v=old; read -t 0.2 v; echo "$? [$v]"'
	//
	//	bash 5.3  142 [part]      ksh93  1 [old]      zsh  0 [partial]
	//
	// zsh's row is not this axis and is why the axis is worded around the
	// timeout rather than around the partial text: its `-t` bounds the wait
	// for the stream to become readable and nothing after that, so once a
	// byte has arrived it reads the line to the end however long that takes
	// and reports success. A zsh timeout therefore only ever happens with
	// nothing to assign, which is the same observable as leaving the name
	// alone; the two are told apart by how long the read takes, not by what
	// it assigns.
	ReadTimeoutKeepsWhatArrived Answer

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

	// TransformLetterCheckedOnlyWhenValued delays the check of a `@`
	// operator's letter until the name has a value. Yes makes `${u@QQ}` on
	// an unset name empty at status 0 while the identical spelling on a set
	// one is a bad substitution — the same word meaning two different things
	// depending on what a variable happens to hold.
	//
	// Reached only by a grammar that *has* the family, which is one shell;
	// to the rest `${u@QQ}` is an unknown operator whatever the value, and
	// nothing here is asked. So this is an axis with one measured answer,
	// deliberately: making an operator's validity depend on a value is not a
	// rule anything should inherit by having a `@` family, and the shell
	// that does it should have to say so. An empty array counts as no value,
	// measured — `a=(); ${a[@]@Z}` is quiet and `a=(x); ${a[@]@Z}` is not.
	TransformLetterCheckedOnlyWhenValued Answer

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
	// The interpreter once followed the two that agree and said so in a
	// comment, which is the shape of a guess rather than a measurement; it
	// asks here now, and arithValueOf is where the ask is made.
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
	// BraceRangePadsToEndpointWidth keeps the leading zeros of a range
	// endpoint and pads every element to the widest endpoint, zeros after
	// the sign: `{01..3}` is `01 02 03` and `{-03..3..3}` is `-03 000 003`.
	// True in bash and zsh; ksh93 strips the padding and prints `1 2 3` and
	// `-3 0 3`. Asked only when an endpoint is written with leading zeros,
	// and only in a dialect whose braces expand at all — dash never reaches
	// it.
	BraceRangePadsToEndpointWidth Answer
	// BraceRangeStepSignHonored takes a written step's sign at its word:
	// the walk leaves the first endpoint in the direction the sign says, so
	// a sign pointing away from the far endpoint ends the range after one
	// element. ksh93's `{10..1..3}` is `10`, its `{1..10..-3}` is `1`, and
	// letters answer the same way — `{a..e..-1}` is `a`. False in bash and
	// zsh, where the endpoints decide the direction and the step
	// contributes magnitude alone. Asked only when the sign and the
	// endpoints disagree.
	BraceRangeStepSignHonored Answer
	// BraceRangeNegativeStepReverses hands a negative step's sign to the
	// order of the result rather than to the walk: the range is walked
	// endpoint to endpoint and then reversed, so zsh's `{3..1..-1}` is
	// `1 2 3` and its `{1..10..-4}` is `9 5 1` — bash's `1 5 9` backwards,
	// not the `10 6 2` that swapping the endpoints would give. True in
	// zsh; false in bash, whose `{3..1..-1}` stays `3 2 1`, and in ksh93,
	// which reaches the question only when the sign agrees with the
	// endpoints and then keeps their order too. Asked only for a written
	// negative step whose sign was not already honored.
	BraceRangeNegativeStepReverses Answer
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
	// with three values; the binary ones keep the type that says so.
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

	// QuitIgnoredWhenNotInteractive makes an untrapped SIGQUIT do nothing at
	// all rather than end the shell.
	//
	//	kill -QUIT $$; echo after
	//
	// prints after and exits 0 in bash 5.3 and zsh, and kills the shell with
	// SIGQUIT in dash and ksh93. It is asked only where those disagree — an
	// untrapped QUIT in a shell that is not interactive — because every other
	// case is unanimous: all five in the panel ignore it with `-i`, and a QUIT
	// with a trap runs the handler everywhere.
	//
	// Two things are worth recording beside the split. The first is that this
	// is a *version* divergence as much as a shell one: bash 3.2 dies by
	// SIGQUIT where bash 5.3 ignores it, so the two bash columns of the corpus
	// differ here and a claim about "bash" that does not say which build is
	// incomplete. The second is that where it is ignored it is ignored
	// properly rather than deferred — measured with a signal sent from another
	// process, bash 5.3 and zsh survive that too.
	//
	// What `trap - QUIT` then means is a further question this does not
	// answer, and the panel splits differently on it: after a handler is
	// installed and removed again, bash 5.3 still ignores the signal and zsh
	// dies by it.
	QuitIgnoredWhenNotInteractive Answer

	// HangupIsAnOrderlyExit makes an untrapped SIGHUP end the shell the way
	// `exit 1` would rather than by the signal's default action.
	//
	//	kill -HUP $$; echo after
	//
	// reports 1 in zsh and 129 in bash, dash and ksh93, and prints nothing
	// after it anywhere. The number is the visible half; the discipline
	// behind it is the whole answer, and three further measurements say so:
	// the shell's caller sees an ordinary exit rather than a death by
	// SIGHUP, the EXIT trap runs, and an `exit 5` inside that trap wins the
	// status the way it would after any other ending.
	//
	// That the EXIT trap runs is what makes this an axis of its own rather
	// than a number to special-case. ExitTrapRunsOnSignalDeath asks whether
	// dying counts as exiting, and zsh answers no — `trap 'echo bye' EXIT;
	// kill -TERM $$` prints nothing there. `kill -HUP $$` prints bye in the
	// same shell, which is only consistent if SIGHUP never produced a death
	// to ask the question about.
	//
	// It is one signal, and only this one. Measured across the nineteen
	// signals whose default action ends a process — HUP, INT, QUIT, ILL,
	// TRAP, ABRT, FPE, BUS, SEGV, SYS, PIPE, ALRM, TERM, USR1, USR2, XCPU,
	// XFSZ, VTALRM and PROF — the panel is unanimous on every one except
	// QUIT, which QuitIgnoredWhenNotInteractive covers, and this. An
	// external SIGHUP is answered the same way, so it is a disposition
	// rather than something the `kill` builtin does on its way past.
	HangupIsAnOrderlyExit Answer

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
	// PrintfUnfinishedConversionIsAPercent writes a bare `%` for a format
	// that ended before its conversion character, and reports success,
	// rather than complaining about a conversion it could not read.
	//
	//	printf 'a%'    bash  a, and `%': missing format character   st=1
	//	printf 'a%5'   zsh   a, and %5: invalid directive           st=1
	//	printf 'a%ll'  dash  a, and missing format character        st=2
	//	printf 'a%5'   ksh93 a%                                     st=0
	//
	// The whole unfinished conversion becomes the one character: `a%5` and
	// `a%ll` are both `a%` there, so the prefix that was scanned is dropped
	// rather than written back.
	//
	// Asked only where a format actually ends inside a conversion.
	PrintfUnfinishedConversionIsAPercent Answer
	// PrintfHexEscape is how a printf format reads `\x`, and it is four
	// answers rather than a presence:
	//
	//	printf 'a\x80Z'   bash, ksh93, zsh  a<0x80>Z    one raw byte
	//	                  dash              a\x80Z      not an escape at all
	//	printf 'a\x0ffZ'  bash, zsh         a<0x0f>ffZ  two digits, then text
	//	                  ksh93             a<0xc3><0xbf>Z  every digit, a code point
	//	printf 'a\xZ'     bash              a\xZ        and a complaint
	//	                  ksh93, zsh        a<0x00>Z    an empty digit run is zero
	//	                  dash              a\xZ        not an escape at all
	//
	// Asked only where a `\x` is actually in the format. It is a question
	// about the *format*, and never about a `%b` argument: that site has its
	// own table and its own axis, PrintfBHexEscape below.
	PrintfHexEscape PrintfHexEscapePolicy
	// PrintfBHexEscape is how a `%b` argument reads `\x`, which is a
	// different question from the one PrintfHexEscape answers: ksh93 reads
	// `\x41` in a format and writes the four characters as they stand in a
	// `%b`, so the site decides as much as the shell does.
	//
	//	printf '%b' 'a\x41Z'  bash, zsh    aAZ
	//	                      dash, ksh93  a\x41Z
	//	printf '%b' 'a\xZ'    bash         a\xZ, and the same complaint a
	//	                                   format's empty digit run draws
	//	                      zsh          a<0x00>Z
	//	                      dash, ksh93  a\xZ
	//
	// The readings themselves are the format's four, which is why this is
	// the same enumeration: a shell that has the escape here reads its
	// digits the way it reads a format's.
	//
	// Asked only where a `%b` argument actually carries a `\x`.
	PrintfBHexEscape PrintfHexEscapePolicy
	// PrintfBEscEscape admits `\e` in a `%b` argument for the escape
	// character: bash and zsh do, dash and ksh93 write the two characters.
	//
	// It is a separate axis from PrintfBCapitalEscEscape below because the
	// two shells that split them split them in opposite directions, so no
	// single answer describes either one: ksh93 has `\E` and not `\e`, and
	// zsh has `\e` and not `\E`.
	//
	// Asked only where a `%b` argument actually carries a `\e`.
	PrintfBEscEscape Answer
	// PrintfBCapitalEscEscape admits `\E` in a `%b` argument: bash and ksh93
	// do, dash and zsh write the two characters. See PrintfBEscEscape for
	// why the two letters are two questions.
	//
	// Asked only where a `%b` argument actually carries a `\E`.
	PrintfBCapitalEscEscape Answer
	// PrintfBStopIsPadded puts what a `\c` left of a `%b` argument through the
	// conversion's field all the same — the width, the precision and the
	// left-justifying flag. bash, dash and zsh do; ksh93 alone writes the
	// partial text as it stands:
	//
	//	printf '[%5b]'   'a\cb'   five  [    a      ksh93  [a
	//	printf '[%-5b]'  'a\cb'   five  [a          ksh93  [a
	//	printf '[%.1b]'  'ab\cc'  five  [a          ksh93  [ab
	//
	// It is a property of the *stop* and not of the conversion: with nothing
	// stopping it ksh93 pads and truncates like the rest, so `printf '[%5b]'
	// 'ab'` is `[   ab` in all six.
	//
	// Asked only where a `\c` actually stopped a `%b` *and* the field would
	// change the text, so an ordinary `printf '%b' 'a\cb'` needs no dialect.
	PrintfBStopIsPadded Answer
	// PrintfBOctalWithoutZero reads a `%b` argument's `\nnn` as octal with no
	// leading zero to introduce it. bash and dash do; ksh93 and zsh want the
	// `\0` and write `\101` as the four characters it is.
	//
	// The `\0nnn` form itself is unanimous and asks nothing — it is the XSI
	// escape `echo` expands, and reading it as a format's octal is what put
	// a backspace and a `1` where every shell writes an `A` (#798).
	//
	// This one is not `echo`'s answer at the other site: an `echo` argument's
	// `\101` is an escape in dash alone, where a `%b` argument's is an escape
	// in bash too.
	//
	// Asked only where a `%b` argument actually carries such an escape.
	PrintfBOctalWithoutZero Answer
	// PrintfLengthModifiers is which C length modifiers a conversion may
	// carry between its precision and its verb — `%zX`, `%ld`, `%jd`.
	//
	// Three answers, and every one of them *ignores* the modifier rather
	// than acting on it: `%hhd` with 300 is 300 and not 44, and `%lld` with
	// the largest signed 64-bit value is that value, in every shell that
	// takes the modifier at all. A shell's arithmetic is one width and the
	// modifier cannot change it, so this is about what a format may say and
	// never about what it means.
	//
	// Asked only where a conversion actually carries one of the letters, so
	// a dialect is never questioned about `%s`.
	PrintfLengthModifiers PrintfLengthModifierSet
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

	// PrintfTimeConversion gives `printf` a `%(fmt)T`: an epoch through a
	// date format, with the format written inside the conversion. bash 5.3's
	// alone among the panel — dash and zsh call `%(` a directive they do not
	// have, and bash 3.2 an invalid format character.
	//
	// The operand is seconds since the epoch, and two numbers are not times:
	// -1 is now and -2 is when the shell started. A missing operand is now
	// as well, and an empty format is the C locale's time of day.
	//
	// ksh93 also has a `%T`, and it is not this one: its operand is a date
	// *string* — `now`, `tomorrow` — and a number earns a warning and the
	// current time instead. Answered No there and recorded in
	// docs/spec/semantics.md rather than modeled, because reading a date the
	// way ksh93 reads one is its own feature.
	//
	// Asked only where a format actually carries a `%(`, so a dialect
	// without the conversion is never questioned about `%s`.
	PrintfTimeConversion Answer

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

	// SubshellJobTable is what a subshell sees of the jobs its parent
	// started. Three answers, and neither of the two-way splits it contains
	// is the same pair:
	//
	//	sleep 1 & jobs -p | cat; echo T    bash, ksh93 → the pid   dash, zsh → nothing
	//	sleep 1 & (jobs -p); echo T        ksh93 → the pid         bash, dash, zsh → nothing
	//
	// so no single yes-or-no can hold both rows for bash. See
	// SubshellJobsKeptOutsideACompound for what bash is doing and for the
	// part of it that is measured and not modeled.
	SubshellJobTable SubshellJobTable

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
	// The letters that describe the invocation *route* rather than an option
	// a script could set — `c` and `s` — are not here either, for the reason
	// `i` is not: they are facts about the invocation that the front end
	// carries in, read off Runner.Route. Where the panel splits over them
	// they have axes of their own, below.
	//
	// The letters a shell turns on only *when* it is interactive are a
	// second vector of their own; see InteractiveOptionLetters, which
	// replaces this one rather than adding to it.
	DefaultOptionLetters string

	// InteractiveOptionLetters is DefaultOptionLetters for a shell that is
	// interactive. It **replaces** the other rather than being appended to
	// it, and that is the whole reason it is a second string instead of a
	// field of letters to add.
	//
	// ksh93 is what forces the shape: measured, its `$-` goes from `hB` for
	// a script to `imBE` for `-i script.sh`, so it *drops* `h` — its
	// command-tracking option, which is on for a script and off at a prompt.
	// A "letters to add" field could not have said that, and a field that
	// could only add would have recorded three shells correctly and one
	// wrongly. bash goes `hB` to `hiBH` and zsh `569X` to `569XZi`, both of
	// which a replacement expresses just as well.
	//
	// Empty means the shell has no separate answer and DefaultOptionLetters
	// stands for both. dash is the panel member that leaves it so: its `$-`
	// is empty either way, so there is nothing for a second string to say.
	//
	// Two letters are deliberately *not* in it, and both for the same reason
	// the route letters are not in DefaultOptionLetters — they are facts the
	// runner holds rather than a string it prints:
	//
	//   - `i` itself, which is unanimous and comes from Runner.Interactive.
	//   - `m`, the monitor. ksh93 is the only shell in the panel that turns
	//     job control on for `-i script.sh` — measured, `set -o` reports
	//     `monitor on` there, and off in bash and zsh, which is exactly why
	//     the letter appears in its row and no other. Writing `m` into this
	//     string would report a monitor that is not running. The letter comes
	//     from Runner.monitor or it does not come at all; that ksh93 turns it
	//     on for `-i script.sh` where the front end does not is measured and
	//     recorded in docs/spec/invocation.md, and is a separate question
	//     from this one.
	//
	// Read rather than `ask`ed, exactly as DefaultOptionLetters is: a dialect
	// that answers nothing shows the letters it shows for a script, and
	// refusing a whole `$-` expansion over an unanswered field would break
	// `case $- in *e*)` in every script running under a preset that has not
	// chosen.
	InteractiveOptionLetters string

	// CommandStringShowsCInDollarDash puts `c` in `$-` when the program came
	// from `-c`. Two against two: bash and ksh93 do, dash and zsh do not, so
	// there is no majority to follow and this is a switch.
	//
	// The POSIX preset says yes, from the text rather than from a vote: `$-`
	// is defined as the option flags specified on invocation, and `-c` is
	// one of them.
	//
	// Read without asking, unlike most axes. A dialect that answers nothing
	// shows no letter, which is the same thing an unanswered
	// DefaultOptionLetters does; refusing a whole `$-` expansion over it
	// would break `case $- in *e*)`, the ordinary errexit check, in every
	// script that runs under a preset which has not chosen.
	CommandStringShowsCInDollarDash Answer

	// CommandStringShowsSInDollarDash also puts `s` there under `-c`.
	//
	// ksh93 alone, and the shape of the disagreement is worth stating: `s`
	// itself is unanimous for the standard-input route — with `-s` written
	// or not, and at a prompt — so what splits the panel is only whether a
	// command string counts. Read down ksh93's rows and its rule is "no
	// script file was named" where the other three's is "the program came
	// from standard input"; the two agree everywhere except here.
	//
	// Read without asking, for the reason above.
	CommandStringShowsSInDollarDash Answer

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

	// ProcessSubstitutionInCondition lets `<(cmd)` stand as a condition's
	// operand — `[[ $v == <(cmd) ]]` — and be performed there.
	//
	// bash alone. zsh reads the word and then refuses it, at status 2 and in
	// a sentence of its own; ksh93 refuses earlier still, while reading, and
	// dash has no `[[ ]]` to refuse it in. So the answer is no for three of
	// the four, and what differs between them is only when and in what words
	// — which is exactly the split between this axis and Diagnostics.
	//
	// It is asked *before* the substitution is performed. A shell that
	// refuses the word must not have started the command first, and that is
	// observable: the command has side effects.
	ProcessSubstitutionInCondition Answer

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

	// TypeOptions is the rest of `type`'s letters, in the getopts spelling
	// the other optstrings use — `-a` for every resolution a name has, `-p`
	// and `-P` for the path alone, `-f` to leave the functions out. Empty
	// means none beyond what the two axes above already give, which is
	// dash's answer: its `type` has no options at all, and
	// TypeEndsOptionsWithDashDash already says so.
	TypeOptions string

	// TypePSearchesPathPastTheShell is what `type -p` does about a name the
	// shell would answer itself: ksh93 and zsh search PATH anyway and name
	// the file, bash prints nothing at all and reports 0 — its `-p` speaks
	// only when the plain answer would have been a file. Asked only with
	// the letter, so a dialect without it never meets the question.
	TypePSearchesPathPastTheShell Answer

	// TypePathAnswerIsASentence is the shape of `-p`'s answer: zsh words it
	// the way its plain `type` does — `echo is /bin/echo`, and the not-found
	// complaint for a miss — where bash and ksh93 print the bare path and
	// meet a miss with silence and the failing status.
	TypePathAnswerIsASentence Answer

	// TypeFSaysTheFunctionBack turns `-f` around: in zsh the letter *prints*
	// a function — the definition, laid out, nothing else — where bash and
	// ksh93 use it to leave functions out of the search.
	TypeFSaysTheFunctionBack Answer

	// ArraysAreSparse makes an unassigned subscript no element at all, so
	// `a=(x); a[5]=y` is an array of two. True in bash and ksh93; zsh reads
	// the whole extent and finds the gap empty, giving five.
	//
	// The store is sparse either way — only the reading differs — so this is
	// asked when an array *has* a gap and never otherwise, which is almost
	// every array there is.
	ArraysAreSparse Answer

	// OperatorDistributesOverStarSubscript applies an operator written on
	// `${a[*]}` — a trim, a replacement, a case change — to each element
	// before the join, so `${a[*]#a}` on `(aa ab)` is `a b`. True in bash and
	// ksh93; zsh joins first and applies the operator to the joined string
	// once, giving `a ab`.
	//
	// Only the star form is an axis. On `${a[@]}` every shell with arrays
	// applies the operator to each element, and the two readings of `[*]`
	// often agree — a suffix trim that stops at the last element, most
	// patterns that match nothing — so this is asked only when they differ.
	OperatorDistributesOverStarSubscript Answer

	// ExportCarriesFunctions gives `export` its `-f`, which writes a
	// function into a child's environment. True in bash alone: the other
	// three have no way to carry a function at all, and each rejects the
	// option as an option — two of them fatally.
	ExportCarriesFunctions Answer

	// ExportTakesTheAttributeOff gives `export` its `-n`, which takes the
	// export attribute off a name and leaves the name itself alone. True in
	// bash alone; the other three refuse the letter as an option, two of
	// them fatally.
	//
	// The same shape as ExportCarriesFunctions and for the same reason: what
	// the letter *means* is not in question anywhere it exists — the name
	// stays set and stops reaching a child — only whether the dialect has it
	// at all. So there is no wording here, and a dialect that says no sends
	// `-n` down the ordinary unknown-option path to collect its own refusal.
	// Measured 2026-09-05: `dash: 1: export: Illegal option -n` and the
	// script ends, `ksh: export: -n: unknown option` with a usage line and
	// the script ends, `zsh:export:1: bad option: -n` with `export` failing
	// at 1 and the script carrying on.
	//
	// A wording field would be the wrong tool even for the one shell that
	// carries on: unlike `-f`, which zsh knows and refuses in words of its
	// own, `-n` is simply not a letter any of the three has.
	ExportTakesTheAttributeOff Answer

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
	// eaten there is nothing left to unset. Recorded as
	// `name/a-lone-dash-given-to-a-builtin` and
	// `name/unset-v-validates-the-lone-dash`.
	LoneDashIsAnOption Answer

	// ReturnOutsideAFunctionIsRefused reports a `return` that has nothing to
	// return from and carries on, instead of ending the script with the
	// status it was given. True in bash alone.
	//
	// Asked only where there is nothing to return from. Inside a function
	// and inside a sourced file all four obey it, so the question is about
	// the one case they split on.
	ReturnOutsideAFunctionIsRefused Answer

	// UnknownConditionOptionIsAStatus makes `[[ -o name ]]` with a name this
	// shell does not have a status of its own with a complaint, instead of
	// the plain false that a name it has but has not set would give. True in
	// zsh alone; bash and ksh93 answer 1 and say nothing, and dash has no
	// `[[ ]]` to ask it in.
	//
	// Asked only where the shells disagree, which is at a name none of them
	// would recognize. A name this shell has is read the same way in all
	// three and nothing is asked.
	//
	// Not the same question as BadSetOptionNameFatal, and measured rather
	// than assumed to be: the name that ends a zsh script when `set -o` is
	// given it leaves `[[ ]]` running, with the complaint said and the next
	// command reached. One construct's refusal is not the other's.
	//
	// The status is a third value rather than a false, which the combining
	// operators show: `[[ ! -o zzz ]]` is 3 and not 0, so `!` leaves it
	// alone, and `[[ -o zzz || 1 == 1 ]]` is 0, so `||` goes on past it the
	// way it would past a false. Measured across the whole truth table on
	// zsh 5.9.2.
	UnknownConditionOptionIsAStatus Answer

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

	// LocalInheritsTheExportAttribute gives a local declaration the export
	// attribute of the name it shadows, so a child sees the local's value
	// under the shadowed name. bash and dash say yes; zsh says no and hands
	// the child nothing at all under that name for as long as the function
	// runs.
	//
	// Asked only where the shadowed name is exported — explicitly or by
	// having been inherited — and only where a scope was actually taken.
	// Declaring a name nothing has exported asks nothing, and a local
	// declared `-x` says so outright and asks nothing either.
	//
	// The value is not the question: the local's own value is what a child
	// is told in the dialects that answer yes, and whether a valueless
	// declaration still shows the outer value is
	// ValuelessDeclarationHidesTheOuterValue rather than this.
	//
	// ksh93 has no `local`, so the question reaches it only through
	// `typeset` in a keyword-defined function, where a child is told
	// nothing — the same answer as zsh by a different road, because that
	// shell's `typeset` takes the attribute off any name it assigns, at the
	// top level as well as in a function. Only the local half is modeled.
	LocalInheritsTheExportAttribute Answer
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

	// ExportListing is the shape `export -p` writes: bash spells each name
	// as a clustered declaration (`declare -x V="1"`), and the other three
	// repeat the command word (`export V='1'`).
	ExportListing DeclarationListingForm
	// ReadonlyListing is the same question from `readonly -p`, where zsh
	// parts ways with its own export listing and writes `typeset -r R=2`.
	ReadonlyListing DeclarationListingForm

	// CoprocEndsInAnArray publishes a started coprocess's near ends as the
	// two elements of an array — `${COPROC[0]}` to read and `${COPROC[1]}` to
	// write, with the process in `COPROC_PID` — which is bash's model and the
	// reason its `coproc` takes a name. zsh answers no: it has no name for a
	// coprocess and no array, and a script reaches the ends with `print -p`
	// and `read -p` instead. Asked only when a coprocess is started, so a
	// dialect without the word never meets it.
	CoprocEndsInAnArray Answer

	// BareDeclarationListing is the shape `export` and `readonly` write with
	// no operands and no `-p` — which is not always the shape `-p` writes.
	// dash and both bash builds answer the bare form exactly as they answer
	// `-p`; ksh93 and zsh drop the command word for the bare form alone and
	// write a plain `V='a b'`, which no `-p` anywhere writes because it could
	// not be read back as a declaration. Measured across the panel from one
	// exported and one readonly name.
	//
	// One field for both builtins, because no shell in the panel splits them:
	// where the bare form differs from `-p` it differs for both, and by the
	// same rule.
	BareDeclarationListing DeclarationListingForm

	// DeclarePrintReportsAMissingName makes `typeset -p nosuch` say so and
	// fail. bash and zsh report it (with their own wording — see
	// Diagnostics.DeclareNoSuchVariable) and answer 1 even when other names
	// listed fine; ksh93 prints nothing for the missing name and answers 0.
	DeclarePrintReportsAMissingName Answer

	// DeclareOptions is the set of letters `declare` and `typeset` take,
	// spelled the way ReadOptions is. The letters are the dialect's own:
	// `-g` declares a global in the two shells that have the letter and is
	// an unknown option in ksh93, whose bad typeset options are fatal, and
	// `-F` names functions in bash while it sets a float's precision in the
	// other two. Empty means `aAiprx`, the set the substrate implemented
	// before the letters were a question.
	DeclareOptions string

	// TypesetBadOptionFatal ends the script over an option `typeset` does
	// not have. ksh93 counts `typeset` among its special builtins and stops
	// there; bash and zsh report it and carry on. Asked only when the
	// refusal has happened, so a shell with no `typeset` never meets it.
	TypesetBadOptionFatal Answer

	// JobSpecsByName resolves `%name` — the job whose command begins with
	// the text — and `%?text`, the one whose command contains it. POSIX
	// gives both spellings; dash answers "no such job" to every spec that
	// is not a number, `%%`, `%+` or `%-`.
	JobSpecsByName Answer
	// AmbiguousJobNameIsRefused is `%name` matching more than one job: bash
	// refuses it as an ambiguous job spec where ksh93 and zsh take the most
	// recent match. Asked only on a second match.
	AmbiguousJobNameIsRefused Answer
	// WaitReportsAMissingJob says a job spec `wait` cannot resolve earns a
	// complaint — see Diagnostics.WaitNoSuchJob — and a failing status.
	// ksh93 says nothing at all and reports 0.
	WaitReportsAMissingJob Answer
	// WaitNWaitsForTheNextJob gives `wait` a `-n`: block until whichever
	// job finishes first and report its status, 127 with no jobs at all.
	// bash's letter alone; the other three refuse or misread it.
	WaitNWaitsForTheNextJob Answer
	// WaitForAJobFailsWhenInterrupted has a `wait` that names a job report a
	// plain 1 when a trapped signal cuts it short, rather than the status
	// that signal encodes. True in ksh93 alone, and only with an operand:
	// `wait $!` and `wait %1` both report 1 there where its *bare* `wait`
	// reports 286 for USR1 — 256 plus the signal, its own encoding for a
	// command a signal killed.
	//
	// bash 3.2, bash 5.3, dash and zsh make no distinction between the two
	// forms and report 158 for either, so the preset follows the four that
	// agree. Measured with a background job outliving the signal, so the
	// answer is about the interruption and not about the job's own status.
	WaitForAJobFailsWhenInterrupted Answer
	// DisownRemovesTheJob makes `disown` take the job out of the table, so
	// a later `jobs` no longer lists it: bash and zsh. ksh93's disown only
	// shields the job from the HUP an exiting shell would send — a signal
	// this engine never forwards — and its `jobs` goes on listing the job.
	DisownRemovesTheJob Answer

	// JobsOptions is the set of letters `jobs` takes, spelled the way
	// ReadOptions is. The letters are the dialect's own and the sets are
	// not nested: POSIX and dash have `-l` and `-p` alone, bash adds
	// `-n -r -s -x`, ksh93 adds only `-n`, and zsh adds `-r -s` plus three
	// of its own. Empty means `lp`, which is POSIX's pair and the only one
	// every shell in the panel has.
	//
	// It is a semantics field rather than a constant because a letter one
	// shell has and another has never heard of is a *refusal* in the second
	// one: `jobs -r` lists the running jobs in bash and is an illegal
	// option in dash, and a shared letter set would have this engine accept
	// it everywhere and answer dash's scripts differently from dash.
	JobsOptions string
	// JobsPidsOnlyOption makes `jobs -p` print one process id per line and
	// nothing else — no number, no marker, no state, no command. dash, bash
	// and ksh93 all do; zsh reads the same letter as "put the job's process
	// *group* id in the listing" and prints its ordinary rows, so a
	// `kill $(jobs -p)` written for one of the first three kills nothing
	// there.
	//
	// Asked only where the letter was given, and only in a dialect that has
	// it, so a listing with no `-p` never reaches it.
	JobsPidsOnlyOption Answer
	// JobsStateFiltersAccumulate decides `jobs -r -s`, where both of the
	// state filters are named at once: zsh lists a job matching *either*
	// state, bash lets the last letter given decide and lists only the jobs
	// in that state — so `jobs -rs` there is `jobs -s`.
	//
	// Asked only when both letters arrive together. One of them alone means
	// the same thing in both shells, and the two dialects without the
	// letters cannot reach the question at all.
	JobsStateFiltersAccumulate Answer

	// DeclareGlobalReachesPastALocal is `declare -g x=new` with a `local x`
	// standing in front of the name: bash writes the global cell and leaves
	// the local untouched, zsh assigns the visible cell — the local — and
	// leaves the global alone. Asked only there: with no local in front,
	// both write the global, which is what the letter is for.
	DeclareGlobalReachesPastALocal Answer

	// LocalOptions is the same question asked of `local`, whose answers do
	// not follow `typeset`'s: dash has `local` and gives it no options at
	// all, so `local -r x` declares a variable named `-r` there — and then
	// refuses it as a bad name. Empty means none, dash's answer and the
	// substrate's old behavior.
	LocalOptions string

	// BareLocalListing is what `local` with no operands writes — three
	// shapes from the three shells that can reach it, so it is a form
	// rather than a flag. See BareLocalListingForm.
	BareLocalListing BareLocalListingForm

	// SetListing is what `set` with no arguments writes — see
	// SetListingForm. All four list, but not the same things: one follows
	// the variables with every defined function, and one lists special
	// parameters and tied arrays no other shell has.
	SetListing SetListingForm

	// ListingControlEscape is how a `$'...'` listing spells a control byte —
	// see ControlEscapeStyle. A field of its own rather than a part of the
	// quoting style, because two dialects that quote the same way spell a
	// control byte differently.
	ListingControlEscape ControlEscapeStyle

	// SetListingQuoting is how that listing spells a value. The styles are
	// the shared listing vocabulary: bash quotes only where it must and
	// closes-reopens with a backslash, dash single-quotes everything, and
	// ksh93 reaches for `$'...'`.
	SetListingQuoting ListingQuotingStyle

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

	// ShiftOptionWords is which leading-`-` words `shift` reads as options
	// rather than as its count, and it is three answers rather than a
	// presence — see ShiftOptionWordPolicy.
	//
	//	shift -x   bash, dash  -x: the count, and not a number
	//	           ksh93, zsh  -x: an option, and not one they have
	//	shift -1   bash, dash, zsh  -1: the count
	//	           ksh93            -1: an option, and not one it has
	//
	// zsh is what makes this three: it refuses `-x` as an option and reads
	// `-1` as a count that is out of range, so "reads options" and "reads
	// every dash word as an option" are not the same answer.
	//
	// A lone `-` is not a dash word in any reading here and reaches the
	// count, which is bash's and dash's answer for it; ksh93 and zsh each
	// do something else with that one word and neither is modeled — see
	// docs/spec/semantics.md. Nor is `--` a dash word, which is asked about
	// separately — see ShiftDoubleDashEndsOptions.
	//
	// Asked only for a word that actually begins with a `-`.
	ShiftOptionWords ShiftOptionWordPolicy
	// ShiftDoubleDashEndsOptions takes `--` as the end-of-options marker and
	// reads what follows as the count. bash, ksh93 and zsh do; dash calls
	// `--` an illegal number, having no option parsing here for a marker to
	// end.
	//
	// It is not ShiftOptionWords: bash reads no dash word as an option and
	// still honors the marker, so the two questions have different answers
	// in the same shell. Only the *first* `--` is the marker —
	// `shift -- --` complains about the second in all three that take it.
	//
	// Asked only where the operand actually is `--`.
	ShiftDoubleDashEndsOptions Answer
	// ShiftNegativeIsOutOfRange reads a negative count as a number that is
	// out of range rather than as a word that is not a number. bash, ksh93
	// and zsh do, at status 1 and in three different wordings
	// (Diagnostics.ShiftNegativeCount); dash calls `-1` an illegal number,
	// which is the same complaint it makes about `-x`.
	//
	// It is the other end of ShiftTooMany — one count, out of range in two
	// directions — so the same ShiftPastEndFatal decides whether it ends the
	// script, and it does: fatal in dash and ksh93, survivable in bash and
	// zsh, with `$#` untouched either way.
	//
	// Asked only where the count really is negative, which in ksh93 means
	// only after a `--`: a bare `-1` is an option there.
	ShiftNegativeIsOutOfRange Answer

	// WaitReadsOptions reads a leading `-` word as an option rather than as
	// a job to wait for. Three of the four do; zsh has none, and answers
	// `wait -x` with the job it could not find.
	WaitReadsOptions Answer

	// CommandRejectsUnknownOption refuses a leading `-` word that is not one
	// of `command`'s own options, rather than taking it as the command.
	//
	// All four read `-v` and `-p`. bash, dash and ksh93 refuse anything
	// else; zsh alone stops reading options there, so `command -q ls` is
	// `command not found: -q` in zsh and a refused option in the other
	// three. The same shape printf already has. Recorded as
	// `cmd/command-with-an-option-nobody-has`, with a letter no panel shell
	// owns: the first probe used -x, which is a real ksh93 option, and read
	// ksh93 as tolerant off ksh93's own feature.
	CommandRejectsUnknownOption Answer

	// GetoptsRejectsUnknownOption is the same question for `getopts`, which
	// has no options at all here — so any leading `-` word is the one being
	// asked about, and it would otherwise be the optstring.
	//
	// bash and ksh93 refuse it: `getopts -q o` is an unknown option there
	// and an optstring of `-q` in dash and zsh. Recorded as
	// `getopts/a-dash-word-where-the-optstring-belongs`, with a letter no
	// panel shell owns: ksh93 has `-a` for real, and the first probe used
	// it — ksh93's answer stood, wrongly, at No until the probe was rerun
	// with -q.
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

	// TrapBodyLine is which lines a diagnostic from inside a trap's body
	// names. See TrapBodyLineStyle.
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

	// SubscriptCommaIsARange reads the comma in `${a[1,3]}` as the separator
	// of a range — elements 1 through 3 — rather than as the arithmetic comma
	// operator, whose value is its right operand and names element 3 alone.
	//
	// The same characters with two meanings, which is what puts it here
	// rather than in a grammar flag: `${a[1,3]}` is one subscript in every
	// shell that has subscripts at all, and they disagree about what it
	// says. Measured on `a=(w x y z)`: zsh 5.9.2 gives `w x y`, and bash
	// 5.3.15, bash 3.2.57, bash as `sh` and ksh93 all give `z`. dash has no
	// subscript to read.
	//
	// Asked only where the two readings differ, which is what keeps
	// `${a[2,2]}` — one element under either — from needing an answer.
	SubscriptCommaIsARange Answer

	// ScalarSubscriptIsACharacter reads `${s[2]}` on a plain string as its
	// second character, rather than as an element of the one-element array a
	// scalar reads as.
	//
	// Measured on `s=hello`: zsh 5.9.2 gives `h` for `${s[1]}` and `e` for
	// `${s[2]}`, where bash 5.3.15, bash 3.2.57, bash as `sh` and ksh93 all
	// give `hello` for `${s[0]}` and nothing for either of the others. Both
	// readings answer, neither reports, and an empty string is a plausible
	// element — so a script cannot tell which shell it is on except by the
	// value it gets, which is the definition of a conflict rather than an
	// addition.
	//
	// A range and a character go together: `${s[2,4]}` is the substring
	// `ell` in the shell that reads characters, and the arithmetic comma's
	// element 4 — nothing — in the shells that do not. But they are two
	// axes, because `${a[1,3]}` on an *array* is a range without a character
	// anywhere in it.
	//
	// Asked only where the two readings differ: a one-character string at
	// the dialect's first subscript is itself under either reading.
	ScalarSubscriptIsACharacter Answer

	// MultibyteEncodingIsHonored decodes the locale's character encoding, so
	// that `${#s}`, `${s:off:len}` and a subscript on a scalar count
	// characters rather than bytes.
	//
	// Measured 2026-09-05 with `s=héllo; echo ${#s}` under
	// `LC_ALL=en_US.UTF-8`: bash 5.3.15, bash 3.2.57, bash as `sh`, ksh93u+
	// and zsh 5.9.2 all answer 5, and dash answers 6. Under `LC_ALL=C` every
	// one of them answers 6, dash included — so this is not "four shells
	// count characters", it is "four shells honor the encoding the locale
	// names and one has no multibyte decoder at all". `s=日本語; echo ${#s}`
	// separates them further: 3 against 9.
	//
	// Which encoding is in force is **not** a second axis. It is state read
	// off the runner's own variables, exactly as PATH and IFS are, and it
	// moves inside a running shell: `LC_ALL=C; s=héllo; echo ${#s}` gives 6
	// in every panel member with nothing exported. See interp/multibyte.go
	// for the precedence and the codesets, and driver/startup.go for the
	// same reasoning applied to POSIX mode (#691, #733).
	//
	// Silent either way, which is why it is an axis and not a bug in one
	// place: both answers are plausible numbers and neither shell reports
	// anything.
	//
	// Asked only where the two readings differ — a value whose bytes are all
	// ASCII is the same length and has the same positions under both — so a
	// shell that never sees a non-ASCII byte never needs an answer.
	MultibyteEncodingIsHonored Answer

	// NegativeSubscriptPastTheStartInserts places a new element in front of
	// every other when a negative subscript counts back past the first one:
	// `a=(p q); a[-3]=x` leaves three elements with `x` at the head, however
	// far past the start the subscript reached. True in zsh alone; bash and
	// ksh93 refuse the subscript and end the script.
	//
	// Asked only for a *negative* subscript that lands before the first
	// element, which is the only spelling that can. A non-negative one below
	// the base — `a[0]` where the first element is 1 — is refused by every
	// shell measured, zsh included, so it needs no answer from anyone.
	NegativeSubscriptPastTheStartInserts Answer

	// ArrayLiteralSubscriptIsAKey reads a subscript written inside an array
	// literal as the text between the brackets rather than as an arithmetic
	// expression — and, because the two go together, makes such a literal
	// declare a keyed array rather than an indexed one.
	//
	// One concept with two consequences, like whether an assignment prefix
	// survives a special builtin. True in ksh93, where `a=([1+1]=c)` stores
	// under the three characters and `${a[2]}` finds nothing; false in bash
	// and zsh, where the subscript is evaluated and the value lands at 2.
	//
	// Asked only where the two readings differ. A plain decimal numeral
	// evaluates to itself, so `a=([2]=c)` fills the same slot either way and
	// never reaches the question — which is what keeps the ordinary way to
	// build a sparse array available in a core that has chosen no shell.
	//
	// dash has no array literal at all, so the axis is absent there rather
	// than false.
	ArrayLiteralSubscriptIsAKey Answer

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
	// /usr/bin/cd. Recorded as `hash/a-builtin-counts-except-in-zsh`.
	HashSearchesPathAlone Answer

	// UnderscoreTracksTheLastArgument moves `$_` to the previous simple
	// command's last expanded argument — the command word itself when it
	// had none, and empty after a bare assignment. bash and zsh; dash and
	// ksh93 keep no such parameter at all.
	//
	// What the two that do not keep it hold instead was recorded here as
	// the shell's own path, forever, and that was measured false: they
	// hold whatever the environment brought and nothing when it brought
	// nothing, because `_` is an ordinary name there. The claim survived
	// because the harness writes a shell's path as `<shell>` and the cells
	// were empty either way — a real path would have shown. Re-measured
	// with `_` scrubbed from the environment and again with `_=X` in it,
	// on both the `-c` and the script route.
	UnderscoreTracksTheLastArgument Answer

	// UnderscoreStartsAtTheInvocation writes argv[0] into `$_` before the
	// first command runs, so a script reading it at the top finds how the
	// shell was started. bash alone, in both builds and under either
	// argv[0]; dash, ksh93 and zsh leave it as it was.
	//
	// Not the same question as UnderscoreTracksTheLastArgument, which is
	// why it is its own axis rather than a consequence of that one: zsh
	// answers yes to tracking and still starts empty, so a startup write
	// gated on tracking would give zsh a value no zsh has.
	//
	// The value is the *invocation* rather than the executable — the same
	// binary reached through a symlink named `sh` writes `sh` — and rather
	// than `$0`, which a `-c` invocation takes from its first operand.
	UnderscoreStartsAtTheInvocation Answer

	// UnderscoreInheritsFromTheEnvironment lets an `_` the shell was handed
	// in its environment show through. Everywhere but zsh, which discards
	// it and starts empty however it was invoked.
	//
	// It is the other half of the startup value and it decides what the
	// half above means: bash writes argv[0] only when the environment said
	// nothing, so an exported `_` wins over the invocation in every shell
	// that reads one. Only reachable with an environment, which is why the
	// case that pins it carries one — no snippet can put a name in the
	// environment of the shell already running it.
	//
	// `_` is exported by some shells as the command they are about to run,
	// so this is not a hypothetical: it is what a shell started by another
	// shell actually finds.
	UnderscoreInheritsFromTheEnvironment Answer

	// FdVariableOutlivesTheCommand keeps a `{name}>f` descriptor open past
	// the simple command that carried it — two of the three that have the
	// grammar; ksh93 takes it back with the command's other redirections,
	// so the number the variable holds is already dead.
	FdVariableOutlivesTheCommand Answer

	// ExecOpenedFdReachesACommand hands a descriptor that `exec`'s own
	// redirection list opened to whatever the shell runs next — the flock
	// and shared-log idioms, and every script that gives a child a logging
	// descriptor. Four of the five say yes; ksh93 alone closes anything
	// above 2 that `exec` opened when it invokes another program, which its
	// manual states as the rule rather than leaving it to be discovered.
	//
	// POSIX decides nothing here: the Shell Command Language says whether
	// standard input, output and error are open for a utility and is silent
	// about the rest, so both answers conform and there is no majority to
	// defer to on the standard's authority.
	//
	// It is narrower than "that shell hands nothing over", and the boundary
	// is measured. A descriptor the *caller* opened crosses in every shell,
	// this one included, and closing it closes it for the child everywhere.
	// A command's own redirection crosses everywhere too — `sh -c '… >&3'
	// 3>f` writes, and so does `exec 3>f; sh -c '… >&3' 3>&3`, where
	// restating the number on the command brings it back. What is withheld
	// is what `exec` opened: the numbered form, a `{v}>f` the shell numbered
	// itself, and a `9<&3` duplicated from an inherited descriptor, while
	// the inherited 3 it was copied from still crosses.
	//
	// Read where the outbound table is built, so an external command and a
	// process replacement get the same answer — measured the same in both,
	// which is one divergence rather than two.
	ExecOpenedFdReachesACommand Answer

	// FdVariableBadCloseIsAnError refuses `exec {name}>&-` when the name
	// holds no descriptor number. ksh93 says nothing and reports success.
	FdVariableBadCloseIsAnError Answer

	// FdNumberBoundedByOpenFileLimit refuses a redirection whose descriptor
	// number is at or above the process's soft limit on open files. bash and
	// ksh93 do; dash and zsh accept the number and let whatever comes next
	// fail on it, or not at all.
	//
	// There is no *language* bound anywhere in the panel — no shell has a
	// ceiling of its own, and the one that bites is the kernel's `ulimit -n`.
	// bash reports the errno it gets: with the limit at 20, `exec 20>f` is
	// `20: Bad file descriptor` and status 1, `exec 19>f` is silent, and
	// lowering the limit lowers the ceiling exactly. ksh93 refuses the same
	// numbers in its own words. dash and zsh answer 0 for `exec 8>f` under a
	// limit of 6 and leave the descriptor unusable, which is the shape of not
	// asking rather than of a different answer.
	//
	// It is asked at the disagreement rather than on every redirection: a
	// number below the limit is nobody's question, and a Runner with no
	// GetRlimit has no limit to be asked about. Reached most often through
	// MultiDigitFdNumber, which is what lets a script write a number that
	// large at all — under the three shells that read one digit, the only way
	// to a descriptor above nine is to let the shell pick it.
	FdNumberBoundedByOpenFileLimit Answer

	// JobControlAbsenceIsReportedFirst refuses `bg` and `fg` before
	// reading the operand when there is no job control — bash and zsh; dash
	// and ksh93 read their operands and options first and complain about
	// those.
	JobControlAbsenceIsReportedFirst Answer

	// StoppedJobsHoldTheExit keeps an interactive shell alive when leaving
	// would abandon a job that is stopped: the shell says so and stays, and
	// the attempt has to be made a second time.
	//
	// bash and zsh; dash and ksh93 leave at once and the job is left stopped
	// with nothing able to name it. Measured through a pseudo-terminal —
	// `sleep 40`, ^Z, `exit` — for `exit` and for the end of input alike,
	// which behave the same in both shells that hold.
	//
	// What counts as having been told is measured too, and it is not simply
	// "warned once": a `jobs` listing counts, so `exit` straight after one
	// leaves; any other command does not, so `echo hi` between the ^Z and the
	// `exit` still warns; and a job stopping afterwards starts it over.
	StoppedJobsHoldTheExit Answer

	// CdpathAnnouncesTheDirectory prints where CDPATH sent a `cd`, when
	// the winning entry was not a plain dot — three of the four; zsh moves
	// in silence.
	CdpathAnnouncesTheDirectory Answer

	// FcEmptyHistoryIsAnError has `fc` report the event it cannot find —
	// zsh; bash and dash answer a script with silence at 0.
	FcEmptyHistoryIsAnError Answer

	// TestIntegerRefusalIsSilent has `[ a -eq 1 ]` fail with no sentence at
	// status 1 — ksh93; the other three complain at 2.
	TestIntegerRefusalIsSilent Answer

	// MissingFileIsOlder has `-nt` and `-ot` count a path that does not
	// exist as older than any file that does, so `f -nt missing` and
	// `missing -ot f` hold whenever f exists. bash and ksh93; dash and zsh
	// answer false unless both files exist. Asked only there: with both
	// files present the comparison is unanimous, and the mirrored cases —
	// a missing file being *newer* — are false in every shell measured.
	// One axis for `test`, `[` and `[[ ]]` alike, because every shell
	// answers its two constructs the same way.
	MissingFileIsOlder Answer

	// TerminalTestRequiresANumber has `-t` refuse an operand that is not a
	// number, at status 2 with the dialect's integer wording — bash and
	// dash; ksh93 and zsh answer a silent false at 1, and so does the bash
	// 3.2 that macOS ships. Asked only for such an operand: a numeric
	// descriptor is answered false the same way everywhere a terminal is
	// absent.
	TerminalTestRequiresANumber Answer

	// ReadRequiresAVariableName refuses a bare `read`: dash's "arg count"
	// at 2, where the other three read into REPLY.
	ReadRequiresAVariableName Answer

	// StdinProgramReadInBlocks takes a program arriving on standard input
	// as much at a time as the descriptor will give, rather than a line at
	// a time. Whatever the block swallowed has left the descriptor, so a
	// `read`, an external command, or anything else the script points at
	// standard input finds only what had not arrived yet.
	//
	// dash alone, measured: `printf 'read x\necho "[$x]"\nDATA\n' | sh`
	// prints `[]` there and then runs `DATA` as a command, where bash,
	// ksh93 and zsh hand the second line to `read` and never parse it.
	// docs/spec/invocation.md has the grid, including the case that shows
	// what the difference really is — `exec 0< file` mid-program replaces
	// the *rest of the program* in the three, and only what follows the
	// block in dash.
	//
	// A bool rather than an Answer, and deliberately: the panel is four to
	// one, so a common denominator exists, and "refuse to read a piped
	// script at all" is not an answer any shell could ship. False is
	// reading by the line, which is what the substrate does.
	//
	// It is the standard-input route's question alone. A script named as an
	// operand is opened separately from standard input, so nothing is
	// shared and all four behave the same way; `-c` reads no descriptor at
	// all. The sibling question for a command string is
	// Diagnostics.CommandStringParsedWhole.
	StdinProgramReadInBlocks bool

	// StdinOptionNamesTheOperands lets the standard-input option name the
	// operands of an invocation that also carries a command string — `sh -sc
	// CMD name a`. The stdin option's rule is that no operand is `$0`: the
	// shell keeps its own name and every operand is a positional parameter,
	// so `$0` is the shell and `$#` is 2. The command string's rule is that
	// the first operand is `$0` and only the rest are parameters, so `$0` is
	// `name` and `$#` is 1.
	//
	// Yes in ksh93 and zsh, no in bash and dash — measured with `-sc`, `-s
	// -c` and `-c -s` alike, since order and bundling change nothing.
	//
	// Asked only when both are given, which is the only place the panel
	// disagrees. Where the program comes from is not this question: all four
	// run the command string, and the corpus pins that separately. Either
	// option alone is unanimous too — the command string names the first
	// operand `$0`, and standard input leaves `$0` as the shell — and with
	// no operands at all the two rules agree by having nothing to name.
	//
	// It has no answer in PosixSemantics, and that is the honest zero
	// rather than an omission: the standard gives `-c` and `-s` separate
	// synopses and says the second is assumed only when the first is absent,
	// so it never describes an invocation carrying both. A 2-2 split with no
	// standard to break it is refused until a dialect chooses.
	StdinOptionNamesTheOperands Answer

	// PlusSignedCommandStringIsDollarZero gives a plus-signed command string
	// `$0` for itself: `sh +c CMD name a` leaves `$0` as CMD and makes every
	// operand a positional parameter, where the minus spelling would have
	// made `name` `$0` and only `a` a parameter.
	//
	// True in ksh93 alone. bash, dash and zsh read `+c` as `-c` in every
	// respect, and all four *run* the command string either way — the sign
	// changes nothing about where the program comes from, which the corpus
	// pins separately.
	//
	// A bool rather than an Answer, for the reason
	// StdinProgramReadInBlocks is one: the panel is three to one, so a
	// common denominator exists, and refusing an invocation every shell
	// runs is not an answer any shell could ship. False is the majority
	// answer and the standard's own — POSIX has no plus spelling of the
	// option at all, so reading it as the option it spells invents nothing.
	//
	// Two further things ksh93 does with `+c` are measured and deliberately
	// not modeled, because they do not agree with each other and read as
	// defects of the 2012 build rather than as a rule: the operands also
	// reach the program as literal words appended to its last command, and
	// a command string of a single word is looked up on PATH and run as a
	// file. docs/spec/invocation.md records both.
	PlusSignedCommandStringIsDollarZero bool

	// LoginProfileWhenNonInteractive has a login shell read its login
	// profile even when there is a script to run rather than a person to
	// prompt. A shell is a login shell when argv[0] begins with a dash,
	// which is what `login` and every terminal emulator's "run as a login
	// shell" does, and the question is only what that then means for a
	// shell that is not going to prompt.
	//
	// dash, ksh93 and zsh read theirs; bash alone reads nothing. Measured
	// 2026-09-05 with a scratch HOME, on all four of the script-operand,
	// `-c`, standard-input and `-s` routes, and the answer is the same on
	// every one of them — this is a fact about the shell rather than about
	// the route. docs/spec/invocation.md has the grid, including the two
	// facts that keep the axis from being wider than it is: an interactive
	// login shell reads its profile in all four, so the interactive route
	// asks nothing, and an explicit `--login` makes bash read it too, so
	// this is about login-ness *inferred from argv[0]* and not about being
	// a login shell as such.
	//
	// A bool rather than an Answer: there is no third thing to do, and
	// "refuse to start" is not an answer any shell could ship.
	//
	// False is the zero value and it is the minority answer, which is the
	// opposite of the way StdinProgramReadInBlocks is named, and on purpose.
	// The majority behavior here is to *read a file out of the invoking
	// person's home directory*, and a Semantics nobody has filled in belongs
	// to a library embedder or a test rather than to a shell — neither of
	// which should touch a home directory because a field was left at its
	// default. A dialect that wants it says so, and PosixSemantics does.
	//
	// Which file a login shell reads is a separate question and is
	// LoginStartupFiles below. The system-wide /etc/profile is still not
	// read at all, on any route; see docs/spec/invocation.md.
	//
	// It is about login-ness **inferred from argv[0]** and not about being a
	// login shell as such, which is measured: an explicit `-l` or `--login`
	// makes bash read its profile with a script to run, so the option
	// overrides this rather than setting the same bit. See
	// StartupFileOptions.Login.
	LoginProfileWhenNonInteractive bool

	// NonInteractiveStartupVariable names a variable whose value is expanded
	// and sourced by a shell that is *not* going to prompt. Empty means the
	// shell has no such file, which is three of the four.
	//
	// A name rather than a bool, for the reason the profile's own filename is
	// not modeled as an axis: what a shell calls the thing is a per-dialect
	// fact and not a disagreement about behavior. One shell in the panel has
	// it, under a name of its own, and the other three do nothing at all with
	// that name — measured 2026-09-05 on a script operand, `-c` and a program
	// on standard input alike.
	//
	// It is the exact counterpart of `$ENV`, which the front end reads for
	// every dialect and reads *only* when interactive. The two never overlap:
	// the shell that has this reads this and not `$ENV` when it is not
	// interactive, and reads neither at a prompt, where it has a file of its
	// own name instead.
	//
	// Two more measured properties, both shared with the profile and both the
	// reason it is sourced where it is. The value is expanded before it is
	// opened, since `$HOME/…` is the usual spelling; and the file is run *by*
	// the shell that is about to run the script, so it sees that shell's `$0`,
	// `$#`, positional parameters and options, and an `exit 3` in it exits 3
	// with the script never run. A file that is not there is not a failure.
	//
	// **POSIX mode suppresses it**, which is measured and is why this is one
	// field rather than two. The shell that has it reads nothing when started
	// with the standard's own posix option, and nothing when invoked as `sh` —
	// the two spellings of the same mode. So the absence in the `sh` column is
	// the mode again rather than a second fact about a second name.
	NonInteractiveStartupVariable string

	// StartupDirectoryVariable names a variable whose value replaces the home
	// directory as the place the startup files below are looked for. Empty
	// means the home directory, which is three of the four.
	//
	// zsh alone has one, `ZDOTDIR`, and it redirects *all* of its files
	// rather than one of them — measured 2026-09-05, with every file under
	// the named directory read and none of the same names under `$HOME`. It
	// is read afresh for each file rather than once, which is also measured
	// and is the reason a person's `~/.zshenv` setting `ZDOTDIR` works at
	// all: the file that sets it is found under the home directory and every
	// file after it under the directory it named.
	//
	// A variable name rather than a path, for the reason
	// NonInteractiveStartupVariable is one: what the shell calls the thing is
	// the dialect's, and the value is the person's.
	StartupDirectoryVariable string

	// UnconditionalStartupFile names a file read on *every* invocation —
	// login or not, prompting or not, `-c` and a script alike. Empty means
	// the shell has no such file, which is three of the four.
	//
	// zsh alone has one, `.zshenv`, and it is the only startup file any shell
	// in the panel reads for a plain `sh -c cmd`. Measured 2026-09-05 with a
	// scratch home directory on all four routes.
	//
	// First of the files, before the profile: measured, `zsh -l -i` reads
	// `.zshenv`, `.zprofile`, `.zshrc` and `.zlogin`, in that order.
	UnconditionalStartupFile string

	// LoginStartupFiles names the profile a login shell reads, most preferred
	// first, as whitespace-separated names. **The first one that can be read
	// is the only one read**, which is bash's rule and reduces to "the file"
	// for every shell with one name for it.
	//
	// Measured 2026-09-05 with a scratch home directory holding a marker for
	// every name: bash reads `.bash_profile`, falls back to `.bash_login` when
	// that is absent and to `.profile` when both are, and reads exactly one of
	// the three. dash, ksh93 and the POSIX preset name `.profile`; zsh names
	// `.zprofile`. Empty means the shell reads no profile, which is what a
	// Semantics nobody has filled in should do — see
	// LoginProfileWhenNonInteractive for why a default must not reach into a
	// home directory.
	//
	// *Whether* a login shell reads it when there is a script to run rather
	// than a person to prompt is the separate question
	// LoginProfileWhenNonInteractive asks; this is only which file.
	//
	// One string rather than a slice, which is how EchoOptions, ReadOptions
	// and JobsOptions already spell a list and is not only consistency: a
	// slice anywhere in this struct makes the whole vector uncomparable, and
	// `==` against another vector is something a test — and an embedder —
	// may already be doing. No shell in the panel names a startup file with
	// a space in it, so nothing is lost by the separator.
	LoginStartupFiles string

	// LateLoginStartupFile names a login file read *after* the interactive
	// file rather than before it. Empty for three of the four.
	//
	// zsh alone has one, `.zlogin`, and the position is the whole of why it
	// is a second field: measured, an interactive login zsh reads `.zprofile`,
	// then `.zshrc`, then `.zlogin`, so a person's `.zlogin` sees what their
	// `.zshrc` did. It is read for a non-interactive login shell too, in the
	// dialects that read a profile there at all.
	LateLoginStartupFile string

	// InteractiveStartupFile names the file read when the shell is
	// interactive, in the startup directory. Empty means the shell has no
	// file of its own name and reads `$ENV` instead, which is dash, ksh93 and
	// the standard.
	//
	// bash names `.bashrc` and zsh names `.zshrc`. Measured 2026-09-05: both
	// read theirs and neither reads `$ENV`, and the shell that reads `$ENV`
	// reads nothing of its own name — the two are alternatives rather than a
	// sequence.
	//
	// **POSIX mode replaces it with `$ENV`**, which is the interactive half of
	// what NonInteractiveStartupVariable records and is measured the same way:
	// bash invoked as `sh` reads `$ENV` at a prompt and does not read
	// `.bashrc`, and so does zsh invoked as `sh`. So this is not suppressed in
	// the mode the way the non-interactive file is — the standard has a file
	// here and the shell reads the standard's one instead of its own.
	InteractiveStartupFile string

	// InteractiveStartupFileWhenLogin has an interactive *login* shell read
	// the interactive file as well as its profile.
	//
	// The panel's one disagreement about startup ordering, and it is why the
	// four combinations of login and interactive are not four independent
	// facts. Measured 2026-09-05 through a pseudo-terminal: zsh reads
	// `.zshrc` for `zsh -l -i` and bash does *not* read `.bashrc` for `bash
	// -l -i` — a person's `.bashrc` is reached from a login bash only because
	// their `.bash_profile` sources it by hand, which is why every bash
	// tutorial tells them to.
	//
	// Asked only where InteractiveStartupFile names something. A shell whose
	// interactive file is `$ENV` reads it in both cases — measured, `-sh -i`
	// reads `.profile` and then `$ENV` in dash, ksh93 and bash-as-`sh` alike —
	// so there is nothing here to answer.
	InteractiveStartupFileWhenLogin Answer

	// StartupFileOptions names the invocation options that say which of the
	// files above to skip, and which file to read in place of the interactive
	// one. The zero value is a shell with no way to skip them.
	//
	// It is a startup input like the files themselves, and the reason it is
	// modeled at all is that **a broken startup file has to be escapable**: a
	// shell whose only `.zshrc` raises an error every time it starts is a
	// shell a person cannot repair from.
	StartupFileOptions StartupFileOptions

	// ArrayLengthWithoutSubscriptIsCount makes `${#a}` of an array the
	// number of elements, which is zsh's reading; bash and ksh93 measure
	// the element the bare name yields. Asked only where the two answers
	// differ.
	ArrayLengthWithoutSubscriptIsCount Answer

	// EmptyArrayAtIsOneEmptyField hands a quoted "${a[@]}" of an empty
	// array one empty field: ksh93 alone, and the reason careful scripts
	// write "${a[@]+"${a[@]}"}".
	EmptyArrayAtIsOneEmptyField Answer

	// SubstringNegativeLengthIsEmpty answers `${x:1:-2}` with nothing at
	// all: ksh93; bash and zsh count the negative length from the end.
	SubstringNegativeLengthIsEmpty Answer

	// SubstringRangeReadsModifiers makes `${x:h}` a *modifier* rather than
	// an arithmetic offset: zsh, where the range is also that shell's
	// history-modifier syntax; bash, ksh93 and dash read it as the
	// expression it looks like everywhere else.
	//
	// The two spellings share every byte of their punctuation, so the reading
	// is decided before either is evaluated, and it is decided by the first
	// byte: a range segment that begins with an unquoted letter is a
	// modifier. `${x:_q:2}`, `${x: i:2}`, `${x:(i):2}`, `${x:$i:2}` and
	// `${x:"h"}` are all substrings in that shell for that reason.
	//
	// Asked only where a segment does begin with one, so `${x:1:2}` needs no
	// answer from anyone.
	SubstringRangeReadsModifiers Answer

	// LinenoCountsFromTheFunction numbers `$LINENO` inside a function from
	// the line the function was written on: zsh; the other three count from
	// the file.
	LinenoCountsFromTheFunction Answer

	// ArithBaseAbove36 admits `37#…` through `64#…`, whose letters split
	// into cases and whose last two digits are `@` and `_`. bash and ksh93
	// take the full 64; zsh stops at 36 and says so.
	ArithBaseAbove36 Answer
	// ArithOverflowSaturates clamps integer overflow at the edge: ksh93
	// holds max+1 at the maximum where the other shells wrap. Asked only
	// when an overflow actually happened.
	ArithOverflowSaturates Answer
	// EmptyArithExpressionIsAnError refuses `$(( ))`: dash wants a primary
	// and stops the script; the other three answer zero.
	EmptyArithExpressionIsAnError Answer
	// TildePlusMinusExpands turns `~+` into $PWD and `~-` into $OLDPWD,
	// only while the variable is set — a fresh shell's `~-` stays literal.
	// bash, ksh93 and zsh have the pair; dash keeps both as written. zsh
	// alone still answers `~-` after `unset OLDPWD`, from directory state
	// of its own this runner does not keep — recorded, not reproduced.
	TildePlusMinusExpands Answer

	// SetHasTraceLetters gives `set` the -E and -T letters, which carry
	// the ERR trap (and DEBUG with RETURN) into functions and subshells the
	// dialect otherwise bounds them out of. bash alone: dash and ksh93
	// refuse the letters, and zsh spells different options with them, so
	// only a refusal is honest elsewhere. Recorded as
	// `opt/set-e-carries-the-err-trap`.
	SetHasTraceLetters Answer

	// SetHasTheHLetter gives `set` the -h letter at all. Three of the four
	// have it and no two mean quite the same thing by it — which option it
	// abbreviates is SetHLetterTracksCommands — while dash refuses the
	// letter outright, fatally, the way it refuses any letter it does not
	// have.
	SetHasTheHLetter Answer

	// SetHLetterTracksCommands makes `set -h` the short spelling of command
	// tracking — the option bash lists as hashall and ksh93 as trackall,
	// permission to remember where commands were found. zsh answers no: its
	// -h abbreviates histignoredups, a history option, and leaves command
	// hashing alone. Asked only where the letter is written, like
	// SetFTurnsOffGlobbing: the long names raise no question.
	SetHLetterTracksCommands Answer

	// MonitorNeedsATerminal ties turning `set -m` on to having a terminal.
	// Measured in shells run with none, which is what a script has: bash
	// and ksh93 grant the option silently; dash remarks `can't access tty;
	// job control turned off` and reports success with the option left off;
	// zsh refuses it at 1, fatally. The two refusal shapes are the
	// dialect's own wording and status — Diagnostics.MonitorDenied and
	// MonitorDeniedStatus. A runner whose front end gave it a person to
	// report jobs to (JobControl) has a terminal, so the question is asked
	// only without one. Turning the option *off* is granted everywhere.
	MonitorNeedsATerminal Answer

	// InteractiveMonitorNeedsATerminal ties the monitor an *interactive*
	// shell turns on for itself to having a terminal, which is a different
	// question from the one above: that one is a script asking with `set -m`,
	// and this one is nobody asking at all.
	//
	// The rule the answer qualifies is unanimous and is not an axis. Measured
	// 2026-09-05 on `-i script.sh` with a scratch HOME and a pseudo-terminal:
	// bash 5.3.15, dash, ksh93u+ and zsh 5.9.2 all report `monitor on` and
	// all four put `m` in `$-`. So an interactive shell runs the monitor, and
	// a front end that leaves it off is wrong on every route rather than in
	// one dialect.
	//
	// What splits is the same invocation with no terminal anywhere: ksh93
	// still reports `monitor on` and `imBE`, and bash, dash and zsh all
	// report it off and leave `m` out. True in bash, dash and zsh; false in
	// ksh93.
	//
	// The terminal that counts is a terminal on any of the three standard
	// streams, and that is measured rather than assumed. A controlling
	// terminal with all three redirected elsewhere is *not* enough — bash,
	// dash and zsh all report the monitor off there — and a pseudo-terminal
	// on any one of the three alone is enough for all three of them. So the
	// question the front end has to answer is about the descriptors it was
	// handed, which is the one it can answer.
	//
	// It is not MonitorNeedsATerminal read a second time, and bash is what
	// separates them: `bash -c 'set -m'` with no terminal turns the monitor
	// on, and `bash -i script.sh` with no terminal leaves it off. One shell,
	// two answers, so an explicit request and an automatic one are two
	// questions.
	//
	// The preset says a terminal *is* needed, and this is the rarer case
	// where the text does not decide. XCU says of `-m` that it "shall be
	// enabled by default for interactive shells" and puts no terminal in
	// that sentence, but it also defines job control throughout in terms of
	// a controlling terminal, so the sentence is silent about having none
	// rather than permissive about it. Silent text gets the answer that
	// claims less — a shell with no terminal does not report a monitor —
	// which is three of the four as well.
	//
	// Read rather than `ask`ed, exactly as InteractiveOptionLetters is: the
	// answer is wanted once at startup, before the program has run a line, so
	// refusing over an unanswered field would put "the shells disagree here"
	// ahead of every `-i script.sh` under a preset that has not chosen. An
	// unanswered field reads as Yes — a terminal is needed and the monitor
	// stays off, which is the majority and the quiet answer.
	//
	// zsh is worth knowing about and is not this axis. With a terminal it
	// puts `m` in `$-` and announces its jobs while its own `set -o` still
	// lists `monitor off` — it disagrees with itself, and what is recorded
	// here is the state the other two readers report.
	InteractiveMonitorNeedsATerminal Answer

	// InteractiveScriptAnnouncesJobs gives an interactive shell running a
	// *named script file* somebody to tell about its jobs: the job number
	// and pid as one starts, and the `Done` row as one ends. True in dash,
	// ksh93 and zsh; false in bash.
	//
	// A different question from AnnouncesBackgroundJob, which asks whether
	// the *start* is announced at all and is answered No by dash alone. Both
	// are read on this route, and dash is why they cannot be one field: it
	// announces the end of a job here and never the beginning.
	//
	// Measured 2026-09-05 through a pseudo-terminal, scratch HOME and scratch
	// HISTFILE, on `sh -i script.sh` running `sleep 0.3 &` between two
	// echoes:
	//
	//	bash 5.3.15   nothing         bash 3.2.57  nothing
	//	bash as `sh`  nothing         dash         the `Done` row, no start
	//	ksh93u+       both            zsh 5.9.2    both
	//
	// It is not the monitor asked a second time. The monitor is unanimous on
	// this route with a terminal — InteractiveMonitorNeedsATerminal records
	// that — and this is not, so a front end that turned both on together
	// would give bash an announcement no bash makes.
	//
	// It is however *gated* on the monitor, which is measured: with no
	// terminal anywhere, dash and zsh leave the monitor off and say nothing
	// about the job either, and ksh93 runs the monitor without one and
	// announces both ends. So the notice rides on the monitor and this axis
	// is what the one dialect that runs a monitor and stays quiet anyway is
	// for.
	//
	// And bash's silence is not about where the commands come from, which is
	// the reading the grid rules out: `bash -i < script` with the program on
	// a *pipe* announces both, and so does `bash -i -c`. Measured, bash is
	// silent on exactly one interactive route, the one whose program is a
	// named file — which is why this axis names the route rather than the
	// terminal.
	//
	// The preset says no. XCU has nothing to say about a notice on this
	// route, and where the text is silent the preset takes the answer that
	// claims less: a shell that has not been asked for a job report does not
	// write one. It is also the intersection — the whole panel is quiet on
	// this route only if bash is — and the core is the intersection rather
	// than the majority.
	//
	// Read rather than `ask`ed, exactly as InteractiveMonitorNeedsATerminal
	// is and for the same reason: the answer is wanted once at startup,
	// before the program has run a line, so an unanswered field would put
	// "the shells disagree here" ahead of every `-i script.sh` under a preset
	// that has not chosen — including scripts that never mention a job.
	//
	// `-i -c` is a separate question and is deliberately not this one. On
	// that route bash, ksh93 and zsh announce and dash does not, which is a
	// different split and therefore a different axis; `docs/spec/invocation.md`
	// has the grid.
	InteractiveScriptAnnouncesJobs Answer

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

	// PipefailSubstitutesTheBareSignal reports an element pipefail chose over
	// the pipeline's last one, and which died of a signal, as the signal's
	// *number* rather than as the status a command killed by that signal
	// reports.
	//
	// True in ksh93 alone, and it is not the same question as
	// SignalDeathStatusIsTwoFiftySix. That axis is about every status a
	// signal death produces, and ksh93 answers it consistently everywhere it
	// was measured — a foreground command, a subshell, a command
	// substitution, a `wait`, the shell dying by its own hand as its parent
	// sees it, and the *last* element of a pipeline are all 256 + n there.
	// This is the one place the convention stops: with `pipefail` set,
	//
	//	kill-me-with-TERM | cat     ksh93  15    bash/zsh  143
	//	kill-me-with-PIPE | head -1  ksh93  13    bash/zsh  141
	//	( exit 42 )       | cat      ksh93  42    bash/zsh   42
	//	cat </dev/null | kill-me     ksh93 271    bash/zsh  143
	//
	// The last row is why this is about the *substitution* and not about the
	// pipeline: an element that fails in the position the pipeline reports
	// anyway keeps the ordinary encoding, and only the status pipefail went
	// looking for is bare. An ordinary non-zero exit is unchanged either way,
	// so a signal is the whole of the difference.
	//
	// Measured builtin and external, first and middle, in pipelines of two
	// and of three, with SIGPIPE and SIGTERM. Absent rather than false in
	// dash, which has no pipefail, and asked only where a substitution
	// actually happened and actually was a signal death.
	PipefailSubstitutesTheBareSignal Answer

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
	// refused 600. The probe lives in docs/spec/semantics.md rather than in
	// the corpus — bash and ksh93 announce the killed writer by process id,
	// which no golden record can hold.
	UlimitBlockIsKilobyte Answer

	// UlimitHasResidentSet is `ulimit -m`. True in bash, dash and ksh93; zsh
	// has no such letter and reports it as a bad option. Recorded as
	// `ulimit/a-letter-zsh-does-not-have`.
	UlimitHasResidentSet Answer

	// UlimitHasProcessCount is `ulimit -u`. True in bash, ksh93 and zsh; dash
	// has no such letter. Recorded as `ulimit/a-letter-dash-does-not-have`.
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

	// MultiDigitDuplicationTargetIsAnError refuses `>&10` — a duplication
	// whose *target* is written with more than one digit. True in dash
	// alone; the other four read the number and fail at run time with `10:
	// Bad file descriptor` if nothing is open there, at status 1, and the
	// script carries on.
	//
	// The companion question, how many digits may stand *before* the
	// operator, is the grammar's and has the opposite dissenter: bash alone
	// reads `exec 10>f` as a redirection where the other three run a command
	// called `10` (Dialect.MultiDigitFdNumber). Two questions that split the
	// panel the other way round cannot be one flag read from both ends.
	//
	// It is not a parse refusal, though the shell that has it words it as a
	// syntax error: `sh -n -c 'echo hi >&10'` accepts the input and exits 0,
	// and a script prints its earlier lines before stopping on this one. The
	// grammar takes the construct everywhere, so this is a semantics axis.
	//
	// The width is what is refused and not the value — `>&08` names
	// descriptor 8 and is refused too — which is what keeps this separate
	// from FdNumberBoundedByOpenFileLimit. And it is the *expanded* word:
	// `n=10; echo hi >&$n` is refused where `n=9` is not.
	//
	// The refusal ends the script, which travels with the answer rather than
	// being an axis of its own — one shell refuses and that shell stops. The
	// status is FatalErrorStatusIsOne's, as every fatal error's is.
	//
	// Asked only where a target really is wider than one digit; `>&2` is
	// nobody's question.
	MultiDigitDuplicationTargetIsAnError Answer

	// RedirectErrorOnSpecialBuiltinFatal ends a non-interactive shell when a
	// redirection written on a *special* builtin cannot be made — a file that
	// will not open, a descriptor that is not there, a number the open-file
	// limit refuses. POSIX states it outright, and it is one rule with a wide
	// reach: `exec 3>/nope/x`, `: 3>/nope/x`, `eval : 3>/nope/x` and
	// `. /dev/null 3>/nope/x` all stop where any of them does.
	//
	// The failure is the *redirection's*, so the builtin never runs and its
	// own status is never reached; the status of the shell that stops is
	// FatalErrorStatusIsOne's, which is why dash exits 2 here and the rest
	// exit 1 without this needing a status of its own.
	//
	// The boundary is measured rather than assumed. A regular builtin
	// (`true 3>/nope/x`) and an external command are unaffected everywhere,
	// and so is a *compound* command's own redirection — `{ echo x; } 3>/f`
	// carries on in all five, because the redirection is the group's and not
	// a special builtin's. Inside a subshell it ends the subshell alone and
	// the parent runs on; inside a function it ends the shell.
	//
	// **This axis is POSIX mode, not a shell.** The panel splits three to
	// two — dash, ksh93 and bash-as-`sh` stop; bash and zsh carry on — and
	// the bash column and the bash-as-`sh` column are the same binary. The
	// flip is reachable at runtime in both shells that have such a mode, and
	// that is what makes this an axis rather than a quirk of an invocation:
	// `set -o posix` makes bash 5.3 and bash 3.2 stop, `set +o posix` makes
	// bash invoked as `sh` carry on, and `emulate sh` or `emulate ksh` makes
	// zsh stop where `emulate zsh` does not. zsh invoked as `sh` stops too,
	// so the same argv[0] moves two different binaries the same way.
	//
	// So a dialect's field here is where the shell *starts*, and the shell's
	// own posix knob moves it — see dialect/bash's `posix` option and
	// dialect/zsh's `emulate`. Nothing is attached to argv[0]: naming the
	// invocation would record the accident and lose the rule.
	RedirectErrorOnSpecialBuiltinFatal Answer

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

	// UnsetReadonlyFatal ends a non-interactive shell when `unset` is asked
	// to remove a readonly name. True in dash and zsh; bash and ksh93 report
	// it, leave the value standing and carry on with a status of 1.
	//
	// The refusal itself is not the axis. Every shell in the panel refuses,
	// keeps the value, and says so — `readonly x=1; unset x` leaves `x` as 1
	// in all six — so *that* is the core answer and only what follows the
	// refusal splits.
	//
	// A field of its own rather than BadNameToUnsetFatal, which it agrees
	// with on all four dialect defaults. They are separable because bash's
	// POSIX mode moves this one and not that one: `set -o posix` makes bash
	// 5.3 stop here, and it makes no difference to `unset 1x` — a name bash
	// 5.3 accepts in silence whatever the mode. That is the shape
	// FatalErrorStatusIsOne's note describes, where *which* errors are fatal
	// stays per-error even when two errors happen to split the panel alike.
	//
	// Like RedirectErrorOnSpecialBuiltinFatal, a dialect's field is where the
	// shell *starts* and its own posix knob moves it — see SetPosixMode. zsh
	// is the difference between the two: it carries on past a failed
	// redirection on a special builtin and stops here, under every
	// `emulate`, so the two axes cannot be one flag. ksh93 is the same
	// difference the other way round.
	//
	// bash 3.2 is fatal in neither mode, so this is bash 5's rule and not
	// bash's; the preset carries the version that measured it.
	UnsetReadonlyFatal Answer

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

	// BadSubscriptToUnsetFatal ends the script when an `unset` operand's
	// subscript will not evaluate. True in bash, where a bad expression ends
	// it wherever one is written; false in ksh93 and zsh, which leave a failed
	// builtin behind and go on. dash has no subscript to evaluate.
	//
	// Asked only for an operand whose subscript actually failed, so `unset
	// a[1]` needs no answer from anyone.
	BadSubscriptToUnsetFatal Answer

	// UnsetArraySpan is what `unset` does to the elements a subscript names,
	// and the panel gives three answers rather than two — see
	// UnsetArraySpanPolicy.
	//
	// One field for `unset a[@]` and for `unset a[3]`, because in the shell
	// that parts from the rest they are one rule: `unset` of a span replaces
	// that span with a single empty element, so `a[3]` is a span of one and
	// comes back blank in place while `[@]` is the whole array and comes back
	// as one empty element. Measured across spans of one, two and all — see
	// docs/spec/measurements.md.
	UnsetArraySpan UnsetArraySpanPolicy
}

// StartupFileOptions are the invocation options that change which startup
// files a shell reads: the escape hatches from a startup file that is wrong.
//
// Each field holds the spellings the dialect accepts, whitespace-separated and
// exactly as they are written on a command line — `--norc`, `-f`. A
// single-dash entry of one letter also matches inside a bundle, so `-if` is
// `-i` and `-f`; a double-dash entry matches a whole word and nothing else.
// Empty means a shell with no such option, and the zero value is a shell with
// none at all.
//
// Strings rather than slices, for the reason LoginStartupFiles is one: a slice
// reached from Semantics makes the whole vector uncomparable, and an option
// spelling has no whitespace in it to lose.
//
// Measured 2026-09-05 across the panel with a scratch home directory. bash has
// three of the four and spells them long; zsh has only the first and spells it
// both ways; dash and ksh93 have none, so a startup file that breaks them is
// escaped by moving the file. The shell that has no escape is the reason the
// other two are worth carrying.
type StartupFileOptions struct {
	// SuppressAll names the options that suppress every startup file. zsh's
	// `-f` and `--no-rcs`, and measured they mean *every* one: `zsh -f -l -i`
	// reads no `.zshenv`, no `.zprofile`, no `.zshrc` and no `.zlogin`.
	//
	// It is not the POSIX `-f`, which turns globbing off. zsh gives the
	// letter this meaning instead — measured, `zsh -f -c 'echo /etc/pas*'`
	// still expands the pattern — which is why the letter is a per-dialect
	// spelling here rather than a set option every shell shares.
	SuppressAll string

	// Login names the options that make this a login shell whatever argv[0]
	// said. `-l` in all four, and `--login` in three of them — dash refuses
	// the long spelling outright, with `Illegal option --` at status 2.
	//
	// It is not simply a second way to set the same bit, and that is why it
	// belongs here rather than beside LoginShell: the option reads the
	// profile **even with a script to run**, where login-ness inferred from
	// argv[0] does not in every dialect. Measured 2026-09-05: `bash --login
	// -c cmd` reads its profile and `exec -a -bash bash -c cmd` reads
	// nothing, so an explicit option makes the panel unanimous where
	// LoginProfileWhenNonInteractive says it is not.
	//
	// Without it a login shell can only be started by exec'ing with a
	// dashed argv[0], which is what `login` does and what a person at a
	// terminal cannot.
	Login string

	// SuppressLogin names the options that suppress the login profile and
	// leave the rest. bash's `--noprofile`, and nobody else's.
	//
	// It beats Login above, which is measured: `bash --noprofile --login -i`
	// reads no profile.
	SuppressLogin string

	// SuppressInteractive names the options that suppress the interactive
	// file and leave the rest. bash's `--norc`, and nobody else's.
	//
	// It suppresses the file the shell reads *of its own name* and not
	// `$ENV`: measured, `bash --posix --norc -i` still reads `$ENV`, because
	// in that mode the standard's file is the one it was going to read.
	SuppressInteractive string

	// NameInteractive names the options whose operand — the next word — is
	// read in place of the interactive file. bash's `--rcfile` and its
	// synonym `--init-file`.
	//
	// It replaces rather than adds, and it loses to everything that suppresses
	// the file: measured, `bash --norc --rcfile f -i` reads neither, and so
	// does `bash --rcfile f -l -i`, where a login shell was not going to read
	// an interactive file at all.
	NameInteractive string
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

// PosixSemantics is what the specification requires, which is not what any
// shell does in full — it is the right target for a portability check and the
// wrong one for a runtime.
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
		EchoOptions:                 "n",
		EchoExpandsHexEscapes:       No,
		EchoExpandsEscEscape:        No,
		EchoExpandsCapitalEscEscape: No,
		// The POSIX read: -r alone. The counts, delimiters and descriptors
		// the dialects add are theirs to add, and the two count axes are
		// unreachable without the letters that raise them.
		ReadOptions: "r",
		// POSIX gives `unset` both letters and no others.
		UnsetOptions: "vf",
		// The POSIX jobs: -l and -p, and `-p` means the process ids alone.
		// The state filters and the rest are the dialects' additions, and
		// the two axes their letters raise are unreachable without them.
		JobsOptions:             "lp",
		JobsPidsOnlyOption:      Yes,
		LengthOfSpecialIsCount:  Yes,
		ArithLeadingZeroIsOctal: Yes,
		// dash is the panel's POSIX-faithful member and the only one
		// exiting 2, so the POSIX preset follows it. The standard itself
		// requires only "greater than zero", which decides nothing.
		FatalErrorStatusIsOne:  No,
		ArithNameValueRecurses: No,
		// A login shell reads ~/.profile whether or not it is going to
		// prompt: dash, ksh93 and zsh, with bash the holdout. POSIX names
		// ~/.profile as the file a login shell reads and does not make it
		// conditional on being interactive, so the standard and the
		// majority agree here.
		LoginProfileWhenNonInteractive: true,
		// And the file it reads: the standard names ~/.profile, which is
		// dash's and ksh93's name for it too. One entry rather than a
		// fallback chain — bash is the only shell in the panel that tries
		// more than one name.
		//
		// InteractiveStartupFile is deliberately left empty here, which is
		// not an omission: the standard's interactive file is `$ENV`, and an
		// empty name is how a dialect says so.
		LoginStartupFiles: ".profile",
		// The three brace-range axes are left unanswered: a brace that
		// never expands never asks them.
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
		// POSIX spells `export` with one option, `-p`. So the standard's
		// answer about `-n` is that there is no such letter, and a preset
		// that said nothing would refuse `export -n` as an unchosen axis
		// rather than as the unknown option the standard makes it.
		ExportTakesTheAttributeOff: No,
		// POSIX names the noglob letter itself: `set -f`, reported in `$-`
		// as `f`. Only zsh answers otherwise.
		NoglobLetterIsF: Yes,
		// POSIX defines `$-` as the option flags specified on invocation,
		// and `-c` is one of them; `-s` is not, when it was not written.
		// The preset takes the text rather than a vote, which is the rule
		// everywhere here, and the panel splits two against two anyway.
		CommandStringShowsCInDollarDash:            Yes,
		CommandStringShowsSInDollarDash:            No,
		ArithInvalidOctalDigitIsError:              Yes,
		RegexQuotingMakesLiteral:                   No,
		ProcessSubstitutionInCondition:             No,
		LastPipelineElementInCurrentShell:          No,
		ShiftPastEndFatal:                          Yes,
		ReadonlyReassignmentFatal:                  Yes,
		ReadonlyReassignmentFatalFromCommandString: Yes,
		ReadonlyReassignmentByDeclarationFatal:     Yes,
		ArrayBaseIsZero:                            Yes,
		// The standard has no subscript, and the nearest reading it does
		// have is its arithmetic: a comma there is the operator whose value
		// is its right operand, and a string is not a sequence a subscript
		// reaches into. Both are also what every panel member but one does.
		SubscriptCommaIsARange:      No,
		ScalarSubscriptIsACharacter: No,
		// XCU defines ${#parameter} as the length of the value "in
		// characters", and defines a character as what the locale's
		// LC_CTYPE category says one is. So the standard's answer is yes,
		// and it is also what every panel member but dash does.
		MultibyteEncodingIsHonored:         Yes,
		DollarZeroInFunctionIsFunctionName: No,
		// A special builtin's failure is fatal to a non-interactive shell,
		// which the standard states outright. dash is the only member of the
		// panel that still does it, and the preset follows the standard
		// rather than the majority.
		BuiltinSyntaxErrorFatal: Yes,
		// POSIX makes a special builtin's failure fatal, and a bad option is
		// one.
		BadOptionToSpecialBuiltinFatal: Yes,
		// XCU's `[n]>&word` takes "one or more digits", so the standard
		// admits `>&10` and the preset follows it. The shell that refuses is
		// the dissenter here, which is worth noting because it is usually
		// the panel's POSIX-faithful member.
		MultiDigitDuplicationTargetIsAnError: No,
		// A redirection that cannot be made is a special builtin's failure
		// as well, and the standard names it in so many words. Three of the
		// five follow it, and the two that do not both reach this answer as
		// soon as their own posix mode is on.
		RedirectErrorOnSpecialBuiltinFatal: Yes,
		// A bad name is a special builtin's failure too, and the standard
		// makes no exception for `unset`.
		BadNameToDeclarationFatal: Yes,
		BadNameToUnsetFatal:       Yes,
		// And so is a refusal to unset a readonly name: the standard makes a
		// special builtin's failure fatal and names no exception for this
		// one either.
		UnsetReadonlyFatal: Yes,
		// `local` needs a function to be local to, and saying so is what
		// three of the four do — the substrate keeps the answer it had
		// before the question was one.
		LocalOutsideAFunctionIsAnError: Yes,
		// The standard has no `local`; dash is the closest reading, and it
		// leaves the outer value visible until the first assignment.
		ValuelessDeclarationHidesTheOuterValue: No,
		// The standard has no `local` either, and it does have the export
		// attribute belong to the *name* for the life of the shell — so a
		// declaration of that name keeps it, which is what both shells with
		// a `local` worth the reading do.
		LocalInheritsTheExportAttribute: Yes,
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
		// POSIX has `set` write each variable as an assignment "in a format
		// that can be reused as input", and dash — its closest reading —
		// single-quotes every value and lists no functions. The standard
		// gives `local` to nobody and `typeset` no options, so those two
		// keep their zero values: no option letters, and a bad one reported
		// rather than fatal — `typeset` is not one of the builtins POSIX
		// marks special.
		SetListing:        SetListingAssignments,
		SetListingQuoting: ListingQuoteAlwaysDoubled,
		// dash quotes every listed value and never reaches `$'...'`, so the
		// numeric fallback is never asked for there; the octal one is what
		// POSIX's own `printf` writes, and is the reading to start from.
		ListingControlEscape: ControlEscapeOctal,
		// POSIX has the operand-less `export` and `readonly` write output
		// "in a form that may be reused as input", which is the command word
		// and the assignment — the same thing `-p` writes.
		ExportListing:          DeclareListingCommandWord,
		ReadonlyListing:        DeclareListingCommandWord,
		BareDeclarationListing: DeclareListingCommandWord,
		BareLocalListing:       BareLocalListsNothing,
		TypesetBadOptionFatal:  No,
		// POSIX gives `%string` and `%?string` outright, has `wait` answer
		// for a job that is not there, and calls a string matching more
		// than one job unspecified — refusing is the reading that invents
		// nothing. It has no `wait -n` and no `disown` at all, so the
		// second stays unanswered and the letter is refused.
		JobSpecsByName:            Yes,
		AmbiguousJobNameIsRefused: Yes,
		WaitReportsAMissingJob:    Yes,
		WaitNWaitsForTheNextJob:   No,
		// POSIX has an interrupted `wait` report a status above 128 and does
		// not carve out the form that names a job; four of the five measured
		// builds agree.
		WaitForAJobFailsWhenInterrupted: No,
		DotMissingFileFatal:             Yes,
		DotWithNoOperandIsAnError:       Yes,
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
		SetHasTraceLetters:              No,
		// POSIX names -h itself, as command tracking: "locate and remember
		// utilities invoked by functions as those functions are defined".
		// dash is the one shell that refuses the letter, and overrides.
		SetHasTheHLetter:         Yes,
		SetHLetterTracksCommands: Yes,
		// POSIX ties -m to process groups and job notices, not to a
		// terminal; the two shells that want one override.
		MonitorNeedsATerminal: No,
		// The standard does not answer this one. XCU says `-m` "shall be
		// enabled by default for interactive shells" and names no terminal
		// in that sentence, but it also defines job control throughout in
		// terms of a controlling terminal — so the sentence is silent about
		// the case where there is none rather than permissive about it.
		// Where the text is silent the preset takes the answer that claims
		// less: a shell with no terminal does not say it is running a
		// monitor. It is also three of the four.
		InteractiveMonitorNeedsATerminal: Yes,
		// The standard says nothing about announcing a job to a shell that
		// was handed a script to run, so the preset claims less and says
		// nothing. It is the intersection as well: bash is silent here and
		// the other three are not, and a core made of what they all do is
		// the quiet one.
		InteractiveScriptAnnouncesJobs:  No,
		TildePlusMinusExpands:           No,
		UnderscoreTracksTheLastArgument: No,
		// POSIX has no `$_`, so nothing is written at startup and a name
		// the environment carried is an ordinary variable that shows
		// through — which is also the majority, five of the six.
		UnderscoreStartsAtTheInvocation:      No,
		UnderscoreInheritsFromTheEnvironment: Yes,
		// The majority answers: full bases, wrapping overflow, zero for an
		// empty expression.
		TestIntegerRefusalIsSilent: No,
		// POSIX has no -nt or -ot at all; dash, its closest reading, wants
		// both files to exist.
		MissingFileIsOlder: No,
		// POSIX gives -t a file descriptor, and dash refuses anything that
		// is not a number.
		TerminalTestRequiresANumber:      Yes,
		FcEmptyHistoryIsAnError:          No,
		JobControlAbsenceIsReportedFirst: No,
		// The standard describes `exit` as exiting and says nothing about a
		// job left stopped, so the base leaves; bash and zsh, which stay and
		// warn, override.
		StoppedJobsHoldTheExit:       No,
		CdpathAnnouncesTheDirectory:  Yes,
		FdVariableOutlivesTheCommand: Yes,
		FdVariableBadCloseIsAnError:  Yes,
		// The standard says nothing about a ceiling, so this follows the
		// panel: bash and ksh93 hand the kernel's refusal back, dash and zsh
		// report success on a number the process cannot hold.
		FdNumberBoundedByOpenFileLimit: Yes,
		// The standard is silent and four of the five hand the descriptor
		// over, which is what the flock and shared-log idioms are built on.
		ExecOpenedFdReachesACommand:        Yes,
		ReadRequiresAVariableName:          No,
		ArrayLengthWithoutSubscriptIsCount: No,
		EmptyArrayAtIsOneEmptyField:        No,
		SubstringNegativeLengthIsEmpty:     No,
		// The standard has no modifiers and no history syntax, so a range is
		// the arithmetic it looks like — which is also what three of the four
		// do with it.
		SubstringRangeReadsModifiers:  No,
		LinenoCountsFromTheFunction:   No,
		ArithBaseAbove36:              Yes,
		ArithOverflowSaturates:        No,
		EmptyArithExpressionIsAnError: No,
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
// Almost no axis survives — the two fields below are the whole of it — and
// that is not a defect of the panel. The axes exist because they diverge;
// everything shells agree about never became one.
//
// So it is *not* the counterpart in the sense of being an equally complete
// preset, and the asymmetry is the sharpest consequence of the whole
// specification rather than an oversight in this function. syntax.Core() gives
// every flag in its set a value because grammar differences are *additive*: a
// construct either parses or it does not, so "what every shell accepts" is a
// well-defined intersection. Semantic differences are conflicts. There is no
// intersection of "an unquoted expansion is split" and "it is not", and no
// value of a boolean means both, which is why Answer has three states and why
// nearly every axis comes back Unspecified here and is then refused by name.
//
// Nothing in this package therefore has a "default" answer to an axis to
// document. A spec entry that says otherwise is wrong; docs/spec/README.md
// spells out how an entry is required to name the field that governs it.
//
// The pairing this implies is not an inconsistency: accept the constructs
// every real shell accepts, and mean what the shell most scripts were written
// against means — syntax.Core() with the Semantics() of dialect/bash. Which
// pairing to use is not this package's decision. This is the substrate: it
// owns the *mechanism*, and a shell built on it owns the policy, the same way
// the gate and the event stream are defined here and the sandbox backends and
// protocols are not. The presets exist so that choice can be spelled in one
// line rather than one per axis.
func CoreSemantics() Semantics {
	return Semantics{
		SplitCommandSubstitution: Yes,
		LengthOfSpecialIsCount:   Yes,
		// Every shell in the panel reads a profile for a login shell, so the
		// core reads one too; the disagreement is only over what it is
		// called. `.profile` is the standard's name and nobody's brand,
		// which is the same choice `$ENV` is for the interactive file and
		// made for the same reason — this binary is not bash and must not
		// claim `.bashrc`.
		//
		// The zero Semantics still names nothing, and that is the split
		// worth keeping: a vector nobody filled in belongs to a library
		// embedder or a test, neither of which should touch a home
		// directory because a field was left at its default.
		LoginStartupFiles: ".profile",
		// And a way to say so. All four shells in the panel take `-l`, so
		// the common denominator has it even though the standard does not
		// — which is the one respect in which this differs from
		// PosixSemantics here. Without it the only way to start a login
		// shell is to exec with a dashed argv[0], which is what `login`
		// does and what a person at a keyboard cannot.
		StartupFileOptions: StartupFileOptions{Login: "-l"},
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

// swapSemantics moves an axis at run time, copy-on-write.
//
// A subshell clone shares the vector by pointer, so the change goes on a
// fresh copy and stays this runner's own — which is also what keeps a mode
// entered inside a subshell inside it. The same shape a dialect uses from
// outside the package, kept here because the core has a mode of its own to
// switch: `set -o posix`.
func (r *Runner) swapSemantics(change func(*Semantics)) {
	s := r.sem()
	change(&s)
	r.Semantics = &s
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
			r.unanswered("exit: this argument")))
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
			r.unanswered(`printf: \c`)))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// PrintfHexEscapePolicy is how a printf format reads `\x`.
//
// Four answers, measured digit by digit rather than assumed from the two the
// escape is usually described with. The panel splits on three separate
// details and each split falls in a different place, which is why this is one
// enumeration and not a bool:
//
//   - Whether the escape exists. dash has no `\x`, so `printf 'a\x41Z'` is
//     the four characters as written.
//   - How wide the digit run is. bash and zsh stop at two and the value is a
//     byte, so `\x0ff` is 0x0f followed by an `f`. ksh93 takes every digit
//     that follows and the value is a *code point* once there are more than
//     two of them: `\xff` is one byte and `\x0ff` is U+00FF in UTF-8.
//   - What an empty digit run means. bash leaves `\x` standing and says so
//     on standard error; ksh93 and zsh read it as zero and write a NUL.
type PrintfHexEscapePolicy int

const (
	// PrintfHexEscapeUnspecified is no answer, and is refused like any other.
	PrintfHexEscapeUnspecified PrintfHexEscapePolicy = iota
	// PrintfHexEscapeAbsent has no `\x` at all, so the backslash and the
	// letter stand as written: dash.
	PrintfHexEscapeAbsent
	// PrintfHexEscapeByte reads at most two digits as one byte, and leaves
	// `\x` with no digit after it as written: bash 3.2 and bash 5.3.
	PrintfHexEscapeByte
	// PrintfHexEscapeByteOrNul reads the same two digits, and an empty digit
	// run as a zero: zsh.
	PrintfHexEscapeByteOrNul
	// PrintfHexEscapeCodePoint reads every digit that follows. Up to two of
	// them is a byte and three or more is a code point written in UTF-8, and
	// an empty run is a zero: ksh93.
	PrintfHexEscapeCodePoint
)

func (p PrintfHexEscapePolicy) String() string {
	switch p {
	case PrintfHexEscapeAbsent:
		return "absent"
	case PrintfHexEscapeByte:
		return "two digits, one byte"
	case PrintfHexEscapeByteOrNul:
		return "two digits, one byte, and no digits is a NUL"
	case PrintfHexEscapeCodePoint:
		return "every digit, a code point"
	}
	return "unspecified"
}

// hexEscape resolves the axis, and only for a format that has a `\x` in it.
func (r *Runner) hexEscape() PrintfHexEscapePolicy {
	p := r.sem().PrintfHexEscape
	if p == PrintfHexEscapeUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`printf: \x`)))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// bHexEscape resolves the `%b` site's `\x` reading, and only for an argument
// that has a `\x` in it.
//
// Separate from hexEscape because the site is half the question: ksh93 reads
// every digit of a format's `\x41` and writes the four characters as they
// stand in a `%b`.
func (r *Runner) bHexEscape() PrintfHexEscapePolicy {
	p := r.sem().PrintfBHexEscape
	if p == PrintfHexEscapeUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`printf: \x in a %b argument`)))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// PrintfLengthModifierSet is which C length modifiers a printf conversion may
// carry, which is a set rather than a switch: the panel splits three ways and
// two of the three take a different number of letters.
type PrintfLengthModifierSet int

const (
	// PrintfLengthModifiersUnspecified is no answer, and is refused like any
	// other.
	PrintfLengthModifiersUnspecified PrintfLengthModifierSet = iota
	// PrintfLengthModifiersAbsent is a printf with no modifiers at all, so
	// `%ld` is a conversion `l` that does not exist: dash.
	PrintfLengthModifiersAbsent
	// PrintfLengthModifiersC89 is `h`, `l` and `L`, and exactly one of them:
	// zsh, which takes `%ld` and calls `%lld` an invalid directive. The set
	// is C89's, which is the reading that explains why `hh`, `ll`, `j`, `z`
	// and `t` — every one of them a C99 addition — are the ones refused.
	PrintfLengthModifiersC89
	// PrintfLengthModifiersC99 adds `hh`, `ll`, `j`, `z` and `t`, and takes
	// any run of the letters rather than one: bash and ksh93 read `%lll` and
	// `%zz` as happily as `%ll`, which is what makes this a skipped run and
	// not a list of spellings.
	PrintfLengthModifiersC99
)

func (p PrintfLengthModifierSet) String() string {
	switch p {
	case PrintfLengthModifiersAbsent:
		return "none"
	case PrintfLengthModifiersC89:
		return "h, l and L"
	case PrintfLengthModifiersC99:
		return "h, hh, l, ll, j, z, t and L"
	}
	return "unspecified"
}

// lengthModifiers resolves the axis, and only for a conversion that carries a
// letter one of the answers would take.
func (r *Runner) lengthModifiers() PrintfLengthModifierSet {
	p := r.sem().PrintfLengthModifiers
	if p == PrintfLengthModifiersUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`printf: a length modifier`)))
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
			r.unanswered("printf: %q")))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// DollarSingleControlPolicy is what `\c` means inside `$'…'`.
//
// The three answers were measured character by character rather than assumed
// from the usual "XOR 0x40" rule, and the measurement is why there are two
// decoding answers instead of one: the rules agree on every letter and on
// `@ [ \ ] ^ _` — the range where masking to five bits and toggling bit 6 are
// the same arithmetic — and part company everywhere else. `$'\c1'` is 0x11 in
// bash and `q` in ksh93.
type DollarSingleControlPolicy int

const (
	// DollarSingleControlUnspecified is no answer, and is refused like any
	// other.
	DollarSingleControlUnspecified DollarSingleControlPolicy = iota
	// DollarSingleControlMasked uppercases the character and keeps its low
	// five bits, with `?` reading as DEL: bash.
	//
	// The `?` is bash 5.3's answer. bash 3.2 has no special case and gives
	// 0x1f, which is what masking alone produces — dated rather than vetoed,
	// per docs/spec/core.md.
	DollarSingleControlMasked
	// DollarSingleControlToggled uppercases the character and toggles bit 6:
	// ksh93, where `\c?` is DEL because 0x3f toggles to 0x7f rather than
	// because anything special was said about it.
	DollarSingleControlToggled
	// DollarSingleControlAbsent is a dialect with no `\c` escape at all: zsh,
	// where the backslash falls to DollarSingleUnknownEscape like any other
	// character no escape claims.
	DollarSingleControlAbsent
)

func (p DollarSingleControlPolicy) String() string {
	switch p {
	case DollarSingleControlMasked:
		return "masked to five bits"
	case DollarSingleControlToggled:
		return "toggled by 0x40"
	case DollarSingleControlAbsent:
		return "absent"
	}
	return "unspecified"
}

// dollarSingleControl resolves the axis, and only for a `$'…'` that has a `\c`
// in it.
func (r *Runner) dollarSingleControl() DollarSingleControlPolicy {
	p := r.sem().DollarSingleBackslashC
	if p == DollarSingleControlUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`$'\c'`)))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// DollarSingleUnknownPolicy is what a backslash does before a character no
// escape claims.
type DollarSingleUnknownPolicy int

const (
	// DollarSingleUnknownUnspecified is no answer, and is refused like any
	// other.
	DollarSingleUnknownUnspecified DollarSingleUnknownPolicy = iota
	// DollarSingleUnknownKeepsBackslash keeps both characters, so `$'\q'` is
	// a backslash and a `q`: bash.
	DollarSingleUnknownKeepsBackslash
	// DollarSingleUnknownDropsBackslash keeps the character alone, so `$'\q'`
	// is a `q`: ksh93 and zsh.
	DollarSingleUnknownDropsBackslash
)

func (p DollarSingleUnknownPolicy) String() string {
	switch p {
	case DollarSingleUnknownKeepsBackslash:
		return "keeps the backslash"
	case DollarSingleUnknownDropsBackslash:
		return "drops the backslash"
	}
	return "unspecified"
}

// dollarSingleUnknown resolves the axis, and only for an escape that really
// has no meaning.
func (r *Runner) dollarSingleUnknown() DollarSingleUnknownPolicy {
	p := r.sem().DollarSingleUnknownEscape
	if p == DollarSingleUnknownUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`$'\': an escape with no meaning`)))
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
			r.unanswered("kill: some of these targets")))
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
			r.unanswered("kill -l: a signal name")))
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
			r.unanswered("an unterminated bracket expression")))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// UnsetArraySpanPolicy is what `unset` does to the span of elements a
// subscript names — `a[@]` and `a[*]`, which every shell measured treats
// identically, and `a[3]`, which names a span of one.
//
// Three answers rather than a switch, and the third is not a variation on the
// other two: one shell does not read `@` as a spelling for "every element" at
// all, so the brackets hold an arithmetic expression like any other and `@` is
// not one. That is a different question from what is left behind, and folding
// it into a boolean would have had to call it "does not clear", which says
// nothing about why.
//
// The two readings of the whole-array spelling reach the single subscript
// unchanged, which is why there is one field and not two. Removing every
// element the subscript names removes the one `a[3]` names; replacing the span
// with a single empty element replaces a span of one with a blank in the same
// place, so the array keeps its length. The third answer parts from the other
// two only over whether `@` is an expression, and `3` is one in every reading,
// so at a single subscript it removes like the first.
type UnsetArraySpanPolicy int

const (
	// UnsetArraySpanUnspecified is no answer. It is refused for the
	// whole-array spelling, where the panel genuinely disagrees about the
	// result. A single subscript takes it as removal, because a preset with
	// no arrays of its own has already committed to that reading — see
	// UnsetTakesASubscript, where the POSIX preset has `unset a[0]` name an
	// element and remove it.
	UnsetArraySpanUnspecified UnsetArraySpanPolicy = iota
	// UnsetArraySpanIsAnExpression reads the brackets as it reads any other
	// subscript: ksh93, where `@` is not an expression and the operand is
	// reported as a bad one. A subscript that *is* an expression names its
	// element and the element is removed.
	UnsetArraySpanIsAnExpression
	// UnsetArraySpanRemovesTheElements takes away every subscript the span
	// names: bash, in both builds measured, where `a[@]` leaves the array
	// with nothing in it and `a[3]` leaves a hole. A name that is not an
	// array is reported rather than emptied, and one that holds nothing at
	// all is quietly left alone.
	UnsetArraySpanRemovesTheElements
	// UnsetArraySpanLeavesOneEmptyElement replaces what the subscript names
	// with a single empty element: zsh, where `unset` of a span is the span
	// becoming one empty string rather than the subscripts going away. A
	// three-element array under `[@]` comes back holding one empty element, a
	// scalar comes back empty, and a single subscript comes back blank in
	// place with the array's length unchanged.
	UnsetArraySpanLeavesOneEmptyElement
)

func (p UnsetArraySpanPolicy) String() string {
	switch p {
	case UnsetArraySpanIsAnExpression:
		return "a subscript"
	case UnsetArraySpanRemovesTheElements:
		return "removes every element"
	case UnsetArraySpanLeavesOneEmptyElement:
		return "leaves one empty element"
	}
	return "unspecified"
}

// unsetArraySpan resolves the axis for the whole-array spelling, where an
// unanswered dialect is refused rather than guessed at: the three answers
// leave three different arrays behind.
func (r *Runner) unsetArraySpan() UnsetArraySpanPolicy {
	p := r.sem().UnsetArraySpan
	if p == UnsetArraySpanUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("`unset a[@]`")))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// unsetBlanksInPlace resolves the same axis for a single subscript, where
// there is nothing to refuse: two of the three answers remove the element and
// no answer at all means removal too.
func (r *Runner) unsetBlanksInPlace() bool {
	return r.sem().UnsetArraySpan == UnsetArraySpanLeavesOneEmptyElement
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
		caret:        r.caretNegates(pattern),
		group:        r.dialect().PatternAlternation,
		quantified:   r.readsQuantifiedGroups(condition),
		numericRange: r.dialect().NumericRangePattern,
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

// ShiftOptionWordPolicy is which leading-`-` words `shift` reads as options.
//
// Three answers and not a bool, because the panel splits on *which* dash
// words rather than on whether there are any: zsh refuses `-x` as an option
// it does not have and reads `-1` as a count, so it is neither of the two
// answers a bool could give.
type ShiftOptionWordPolicy int

const (
	// ShiftOptionWordsUnspecified is no answer, and is refused like any
	// other.
	ShiftOptionWordsUnspecified ShiftOptionWordPolicy = iota
	// ShiftOptionWordsNone reads every dash word as the count, so `shift -x`
	// complains about a number: bash, dash.
	ShiftOptionWordsNone
	// ShiftOptionWordsNonNumeric reads a dash word as an option unless what
	// follows the dash is all digits, so `shift -x` is an option and
	// `shift -1` is a count: zsh.
	ShiftOptionWordsNonNumeric
	// ShiftOptionWordsAny reads every dash word as an option, digits and
	// all, so `shift -1` and `shift -0` are both refused as options: ksh93.
	ShiftOptionWordsAny
)

func (p ShiftOptionWordPolicy) String() string {
	switch p {
	case ShiftOptionWordsNone:
		return "none: a dash word is the count"
	case ShiftOptionWordsNonNumeric:
		return "an option unless it is all digits"
	case ShiftOptionWordsAny:
		return "every dash word is an option"
	}
	return "unspecified"
}

// shiftOptionWords resolves the axis, and only for a word that begins with a
// `-` and is neither a lone dash nor the end-of-options marker.
func (r *Runner) shiftOptionWords() ShiftOptionWordPolicy {
	p := r.sem().ShiftOptionWords
	if p == ShiftOptionWordsUnspecified {
		r.diagf("%s\n", r.unanswered("`shift -x` read as an option rather than as a count"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

func (r *Runner) ask(a Answer, axis string) bool {
	switch a {
	case Yes:
		return true
	case No:
		return false
	}
	r.diagf("%s\n", r.unanswered(axis))
	r.status = 2
	r.unspecified = true
	return false
}
