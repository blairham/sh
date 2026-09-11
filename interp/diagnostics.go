// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"syscall"

	"github.com/blairham/sh/syntax"
)

// Diagnostics is how a dialect reports failure.
//
// It is a third vector beside [Dialect] and [Semantics], and it exists because
// the first two could not hold what goes in it. A dialect flag says whether a
// construct parses; a semantics axis says which of two behaviors a construct
// has. Neither can say what a shell *prints* when it refuses, or which number
// it exits with — those are not sides of a question, they are values.
//
// It lives here, next to [Semantics] and [PosixSemantics], rather than in the
// shells built on the substrate. The argument is the one the presets already
// make: a dialect is a set of measured answers, and where the answers live is
// already settled. A shell that wants its own wording overrides the vector,
// exactly as it would override a semantics axis.
//
// The zero value means "the substrate's own", not "unset". That is the
// difference between this and [Semantics], and it is deliberate: a semantics
// axis with no answer is refused, because answering it would claim some
// shell's behavior. A status has no such claim to make — the process must
// exit with *some* number, and refusing to choose is not available. `sh` is
// itself a shell, so where a dialect says nothing, `sh` answers for itself.
// BadSubstitutionSubject is the text a bad-substitution diagnostic puts where
// its verb is.
//
// Three answers because the panel gives three. One names the expansion and
// nothing around it; one names the whole word exactly as it was written; and
// one names the run of the word that shares the expansion's quoting, with the
// quotes taken off — so `echo "pre${x@QQ}post"` blames all of
// `pre${x@QQ}post` there while `echo 'lit'"${x@QQ}"` blames `${x@QQ}` alone.
// The remaining two dialects name nothing at all and never reach this.
type BadSubstitutionSubject uint8

const (
	// NamesTheExpansion is the `${…}` and nothing else, which is the
	// substrate's own answer and the one a dialect gets by saying nothing.
	NamesTheExpansion BadSubstitutionSubject = iota
	// NamesTheQuotingRun is the run of spans around the expansion that share
	// its quoting, written without the quote characters that surrounded it.
	NamesTheQuotingRun
	// NamesTheWholeWord is the word as written, quotes and all.
	NamesTheWholeWord
)

type Diagnostics struct {
	// TiedNamesRequired, TieToItself, AlreadyTiedScalar and TieWithAValue are
	// what `typeset -T` says when it cannot make a tie — see tiedscalar.go.
	//
	// One shell in the panel has the letter with this meaning, so its words
	// are the defaults and no dialect overrides them: bash refuses `-T`
	// outright and ksh93's `-T` declares a *type*, which is a different
	// builtin's worth of thing and stays refused by name. The fields are
	// here rather than inline so that a dialect which grows the letter has
	// somewhere to say it, which is the rule every other refusal follows.
	TiedNamesRequired string
	TieToItself       string
	AlreadyTiedScalar string
	// TieSecondMustBeArray is a scalar value on the array half:
	// `typeset -T S s=plain`.
	TieSecondMustBeArray string

	// SyntaxErrorStatus is the exit status of a script that did not parse.
	//
	// Measured across eight distinct syntax errors — a stray `}`, `echo (`,
	// an unterminated `if`, `case`, `for`, and more — and stable within each
	// shell: dash 2, bash 2, ksh93 3, zsh 1. It was hardcoded to 2 under a
	// comment reading "a syntax error is 2 in every shell in the panel",
	// which is true of half of them.
	//
	// Zero means the substrate's own, which is 2.
	SyntaxErrorStatus int

	// SourcedSyntaxErrorStatus is what a parse failure in a file read by `.`
	// reports, when that differs from SyntaxErrorStatus.
	//
	// It is a separate field because one shell answers the two differently:
	// zsh reports 1 for a syntax error it read from -c and 126 for the same
	// text read by `.`, where bash says 2 for both and ksh93 says 3 for both.
	// Folding them together would have given zsh one answer and lost the
	// other.
	//
	// Zero means "the same as SyntaxErrorStatus", which is the common case.
	SourcedSyntaxErrorStatus int

	// SourcedFatalStatus is what `.` reports when the file it read was given
	// up because of an error, in a dialect that catches one there at all —
	// see Semantics.FatalErrorEndsBorrowedTextOnly.
	//
	// `.` and not `eval`, which is the same split SourcedSyntaxErrorStatus
	// has and measured the same way: an error caught at an `eval` reports 1
	// in both catching shells, where the same failure caught at a `.` reports
	// 126 in zsh.
	//
	// Its own field because neither of the two shells that catch reports the
	// status the error itself carried, and they do not agree with each
	// other: measured over an unset parameter under `set -u`, a readonly
	// assignment, a division by zero and a bad substitution alike, `.`
	// reports 1 in ksh93 and 126 in zsh, where a fatal error that reaches
	// the top reports 1 in both. Nor is it the sourced *syntax* status,
	// which is 3 in ksh93 against this 1.
	//
	// Zero means the status the error already produced, which is what makes
	// ksh93 need no answer here.
	SourcedFatalStatus int

	// DotNoOperand is what `.` says when given no filename at all. No verbs.
	DotNoOperand string
	// DotNoOperandStatus is the status that carries. bash and ksh93 say 2,
	// zsh says 1; dash does not treat it as an error at all, which is a
	// semantics axis rather than a value here.
	//
	// Zero means the substrate's own, which is 2.
	DotNoOperandStatus int

	// DotNoOperandUnprefixed prints that usage with no location and no shell
	// name in front of it. ksh93 alone, and the same thing it does to `kill`'s
	// usage — a usage line is not a diagnostic there.
	DotNoOperandUnprefixed bool

	// DotCannotOpen is what `.` says when it cannot read the file. Two verbs,
	// positional because the shells order them differently: %[1]s is the
	// operand as written and %[2]s the reason.
	DotCannotOpen string
	// CannotExecute is what `exec` says when the command is there and will not
	// run — a file without the execute bit, a directory. Two verbs,
	// positional because the shells order them differently: %[1]s is the
	// command as written and %[2]s the reason.
	CannotExecute string
	// ExecCannotExecute is the same failure reported by `exec` rather than by
	// a command word. Two dialects name the builtin there and do not name it
	// for an ordinary command — dash says `exec: x: Permission denied` for
	// one and `x: Permission denied` for the other. Empty means "the same as
	// CannotExecute", which is bash and zsh.
	ExecCannotExecute string
	// ExecNotFound is what `exec` says when there is no such command at all,
	// which every shell words as some form of "not found" rather than with
	// the strerror text CannotExecute carries. Same two verbs; the reason is the
	// literal "not found", so most dialects ignore it.
	//
	// Empty means "the same as CannotExecute".
	ExecNotFound string
	// PathNotFound is what `exec` says when the operand had a slash in it
	// and there is no such file — as opposed to a bare name that was not on
	// PATH. Same two verbs.
	//
	// The distinction is real in three of the four: bash says `exec: x: not
	// found` for a bare name and `/p/x: No such file or directory` for a
	// path, and zsh says `command not found: x` against `no such file or
	// directory: /p/x`. It is the same split `.` has between DotNotFound and
	// DotCannotOpen, arrived at from the other direction.
	//
	// Empty means "the same as ExecNotFound".
	PathNotFound string

	// NamesResolvedPath makes a failed `exec` name the absolute path it
	// tried rather than the operand as written.
	//
	// bash alone, and only for `exec`: `exec ./ne.sh` in /tmp reports
	// "/tmp/ne.sh: Permission denied" there, where dash, ksh93 and zsh all
	// report "./ne.sh". The same bash reports `. ./nosuch.sh` as written, so
	// this is not a general habit of the shell and cannot be shared with the
	// `.` wording.
	NamesResolvedPath bool

	// DirectoryReason is the reason this dialect gives for `exec` on a
	// directory, when it is not the one the operating system reported.
	//
	// bash and ksh93 check for a directory themselves and say so — "Is a
	// directory" — where dash and zsh hand the path to execve and report the
	// EACCES it comes back with, as "Permission denied". Same failure, same
	// status of 126, two different explanations, and the difference is
	// whether the shell looked before it leapt.
	//
	// Empty means "whatever the operating system said", which is the first
	// pair.
	DirectoryReason string

	// TimeLayout is how the `time` keyword arranges its report. Zero is the
	// substrate's own: a blank line, then labeled `real`, `user` and `sys`
	// lines in the minutes-and-seconds form — the shape bash and ksh93
	// share, apart in only their decimals.
	TimeLayout TimeLayout
	// TimeDecimals is how many decimal places that report's seconds carry:
	// bash 3, ksh93 2. Zero means the substrate's own, which is 3. The
	// per-command layout does not read it — its line fixes its own widths.
	TimeDecimals int
	// TimeBare is what a `time` with no pipeline reports. The three shells
	// that have the keyword give three answers: bash reports a run of
	// nothing, ksh93 the shell's own user and sys with no real, zsh a
	// `shell` and a `children` line. Zero is the first, which is also the
	// substrate's own.
	TimeBare TimeBareLayout

	// TimesLayout is how `times` arranges what it prints. Zero is the
	// substrate's own, which is the two-line self-then-children shape dash,
	// bash and zsh share.
	TimesLayout TimesLayout
	// TimesDecimals is how many decimal places `times` gives its seconds, and
	// the panel offers four different answers to a question nobody would think
	// to ask: dash 6, bash 3, ksh93 and zsh 2.
	//
	// Zero means the substrate's own, which is 3.
	TimesDecimals int
	// TimesArguments is what `times` says when given an argument it refuses.
	// No verbs. Only zsh says anything — dash and bash ignore the argument, and
	// ksh93 never reaches a builtin because `times` is a reserved word there.
	TimesArguments string

	// NamesBuiltinInLocation puts the reporting builtin's name between the
	// shell's name and the line number: `zsh:shift:1: …` rather than
	// `zsh:1: …`.
	//
	// zsh alone, and it is a rule rather than a handful of special cases —
	// measured across `.`, `times`, `shift`, `cd`, `unset`, `read`, `trap` and
	// `break`, every one of which names itself. What does *not* get a segment
	// is equally consistent: a command that could not be found, a parse error,
	// an unset parameter, a division by zero, a redirection that would not
	// open. Those are the shell's own failures rather than a builtin's.
	//
	// The distinction is "whose diagnostic is this", not "where did it happen".
	// A builtin that runs a script — `.`, `eval` — is not the author of what
	// that script reports, and zsh agrees: an unset parameter inside a sourced
	// file is `./f.sh:2: NOPE: parameter not set`, with no `.` anywhere in it.
	NamesBuiltinInLocation bool

	// TestNamesFirstOperand makes a malformed three-argument `test` blame the
	// first word rather than the middle one: `test a b c` is "a: unexpected
	// operator" in dash and names `b` — the word that should have been an
	// operator — in the other three.
	TestNamesFirstOperand bool

	// TestUnaryExpected is what `test` says about a word where a unary
	// operator belonged. One verb: the word.
	TestUnaryExpected string
	// TestBinaryExpected is the same for a binary operator's place. One verb.
	//
	// Two fields because one dialect words them differently — "unary operator
	// expected" against "binary operator expected" — where the other three use
	// one message for both and simply set these to the same string.
	TestBinaryExpected string
	// TestIntegerExpected is a non-numeric operand to `-eq` and its siblings.
	// One verb: the operand.
	TestIntegerExpected string
	// TestOperandExpected is an operator with nothing after it. No verbs.
	TestOperandExpected string
	// ProcessSubstitutionNotInCondition is a `<(cmd)` standing as a
	// condition's operand in a dialect that does not allow one there. One
	// verb: the substitution as it was written, `<(cmd)` and not its inside.
	ProcessSubstitutionNotInCondition string
	// TestTooManyArguments is a well-formed expression with words left over.
	// No verbs.
	TestTooManyArguments string
	// TestMissingBracket is `[` without its closing `]`. No verbs — and every
	// shell in the panel words it differently, which is the whole reason these
	// four are fields rather than strings in the builtin.
	TestMissingBracket string

	// GetoptsBadOption is an option `getopts` does not have in its string.
	// One verb: the letter.
	GetoptsBadOption string
	// GetoptsMissingArgument is an option whose argument is not there. Same.
	GetoptsMissingArgument string
	// GetoptsNamesNoLine prints those with the shell's name and no line,
	// where this dialect gives a line to everything else. bash alone.
	GetoptsNamesNoLine bool
	// GetoptsUnprefixed prints them with neither a name nor a line. dash
	// alone, and the only diagnostic in the panel with nothing in front of
	// it at all.
	GetoptsUnprefixed bool

	// CdCannotChange is a directory `cd` could not move to. Two verbs,
	// positional because the shells order them differently and one does not
	// use the second at all: %[1]s is the operand as written and %[2]s the
	// reason.
	//
	//	bash   cd: /nope: No such file or directory
	//	dash   cd: can't cd to /nope
	//	ksh93  cd: /nope: [No such file or directory]
	//	zsh    no such file or directory: /nope
	//
	// dash gives no reason at all, so `cd` onto a file and `cd` onto nothing
	// read identically there — the one shell where the message cannot tell
	// you which it was.
	CdCannotChange string
	// CdStatus is what that reports. dash says 2 and the other three say 1.
	// Zero means the substrate's own, 1.
	CdStatus int
	// HashEmptyTable is what a bare `hash` says about the table this shell
	// does not keep. bash announces it, on standard output; the other three
	// print nothing, which the empty value means.
	HashEmptyTable string
	// HashNotFound is a hashed name that resolves to nothing. One verb: the
	// name. Empty means the substrate's own wording.
	HashNotFound string

	// CompleteNoSpec is `complete -p` or `-r` on a name nothing was
	// registered for. One verb: the name.
	CompleteNoSpec string

	// FunctionNameInvalid refuses to define a function whose name carries
	// punctuation, in the dialect that refuses one. One verb: the name.
	FunctionNameInvalid string
	// FunctionNameDiscipline replaces it when the name carries a dot, which
	// is a discipline function to the shell that says this.
	FunctionNameDiscipline string

	// DirectoryOnPathStatus is what a PATH search whose only match was a
	// directory reports, in a dialect that keeps the directory as its
	// answer. dash says 127 — the message names the candidate and the
	// status says not found — where ksh93 and zsh say 126. Zero means 126.
	DirectoryOnPathStatus int
	// CdHomeNotSet is `cd` with no operand and no HOME. One verb: the name.
	// Only the two dialects that treat it as an error say anything.
	CdHomeNotSet string
	// CdOldpwdNotSet is `cd -` with no OLDPWD. Same.
	CdOldpwdNotSet string
	// CdEmptyOperand is `cd ""`, and `cd` with HOME set to the empty string
	// in the one dialect that refuses that too. No verbs.
	//
	// A fifth branch rather than a shade of CdCannotChange or of
	// CdHomeNotSet: bash calls it `cd: null directory` and ksh93 `cd: bad
	// directory`, and neither sentence names a path or mentions HOME. The
	// three dialects that take an empty operand as somewhere say nothing, so
	// the empty value is their whole answer.
	CdEmptyOperand string
	// CdTooManyOperands is `cd` given more operands than it takes: two in
	// the dialects without the substitution form, three in the two with it.
	// No verbs.
	//
	// bash and zsh both say `cd: too many arguments` and leave different
	// statuses behind; ksh93 says nothing here and writes its usage block
	// instead, which is the field below.
	CdTooManyOperands string
	// CdTooManyOperandsShowsUsage writes BuiltinUsage["cd"] under that
	// refusal. True in ksh93, which writes the block and no sentence.
	//
	// Independent of the sentence rather than an alternative to it: nothing
	// stops a dialect writing both, and the two shells that write one each
	// write a different one.
	CdTooManyOperandsShowsUsage bool
	// CdTooManyOperandsStatus is what that refusal reports. Zero means
	// CdStatus, which is what a `cd` that could not move reports. bash and
	// ksh93 answer 2 here where their CdStatus is 1, because it is a usage
	// error rather than a directory that would not open; zsh answers 1 and
	// so needs nothing.
	CdTooManyOperandsStatus int
	// CdBadSubstitution is `cd old new` where old is not in the current
	// directory's path. One verb: the string that was not found.
	//
	// Only the two dialects with the form reach it, and they word it
	// differently enough that one of them names the operand and the other
	// does not: ksh93 says `cd: bad substitution` and zsh `cd: string not in
	// pwd: old`.
	CdBadSubstitution string

	// PrintfBadNumber is a numeric conversion given something that is not a
	// number. One verb: the operand.
	PrintfBadNumber string
	// PrintfBadNumberStatus is what that reports. Zero means 1.
	PrintfBadNumberStatus int
	// PrintfBadVerb is a conversion this shell does not have. Two verbs, and
	// the panel splits evenly between them: %[1]s is the conversion
	// character alone and %[2]s is the whole directive as written, so `%lQ`
	// is `Q` for half of them and `%lQ` for the other half.
	PrintfBadVerb string
	// PrintfBadVerbStatus is what that reports. Zero means 1.
	PrintfBadVerbStatus int
	// PrintfMissingVerb is a format that ended before its conversion
	// character — `%`, `%5`, `%ll` with nothing after them. One verb: the
	// whole directive as written, since there is no conversion character in
	// it to name.
	//
	// It is a separate wording and not the bad-conversion one with an empty
	// name. bash has a second complaint for it, `missing format character`,
	// and names the directive in it where its ordinary one names the
	// character; zsh reuses `invalid directive`; dash names nothing at all,
	// so its wording takes no verb.
	PrintfMissingVerb string
	// PrintfMissingVerbStatus is what that reports. Zero means 1.
	PrintfMissingVerbStatus int
	// PrintfMissingHexDigit is a `\x` in a format with no hexadecimal digit
	// after it. No verbs.
	//
	// Only the dialect that leaves the escape standing says anything, and it
	// is a warning rather than a failure: bash writes
	// `printf: missing hex digit for \x`, writes the two characters, and
	// still reports success. The dialects that read an empty digit run as a
	// zero write the NUL and say nothing.
	PrintfMissingHexDigit string
	// PrintfMissingUnicodeDigit is a `\u` or a `\U` with no hexadecimal digit
	// after it. One verb: the escape letter as written, so one wording covers
	// both spellings.
	//
	// Only the dialect that leaves the escape standing says anything, and it
	// is the same warning-without-failure PrintfMissingHexDigit is: bash
	// writes `printf: missing unicode digit for \u`, writes the two
	// characters, and still reports success. The dialect that reads an empty
	// run as a zero writes the NUL and the one that ends the pass ends it,
	// and neither says a word.
	PrintfMissingUnicodeDigit string
	// PrintfUsage is `printf` with no format at all. No verbs.
	// PrintfBadOption is a leading `-` word this dialect does not know. One
	// verb: the word as written.
	//
	// bash `printf: -q: invalid option`, dash `printf: Illegal option -q`,
	// ksh93 `printf: -q: unknown option`. zsh has none, because it takes an
	// unknown one as the format instead.
	PrintfBadOption string

	// PrintfBadOptionShowsUsage follows that complaint with the usage line.
	// bash and ksh93 do; dash prints the complaint alone.
	PrintfBadOptionShowsUsage bool

	// UmaskBadMask is a mask `umask` could not read. One verb: the operand
	// as written — except in zsh, which names nothing.
	//
	// bash `umask: 9999: octal number out of range`, dash
	// `umask: Illegal number: 9999`, ksh93 `umask: 9999: bad number`, zsh a
	// bare `bad umask`.
	UmaskBadMask string

	// UmaskBadMaskStatus is what that reports. Zero means the substrate's
	// own, which is 1 — dash alone says 2.
	UmaskBadMaskStatus int

	// UmaskBadSymbolicMode is a symbolic mode it could not read. Two verbs:
	// the whole argument and the character that stopped it.
	//
	// The panel splits on which to name. dash and ksh93 quote the argument
	// back — `umask: Illegal mode: u=q`, `umask: u=q: bad format` — while
	// bash and zsh name the character and say what kind it was.
	UmaskBadSymbolicMode string

	// AliasNotFound is `alias` naming one the table does not hold. Two verbs:
	// the builtin and the name.
	//
	//	bash   alias: nope: not found
	//	dash   alias: nope not found
	//	ksh93  nope: alias not found
	//
	// zsh prints nothing, which AliasReportsNotFound answers rather than an
	// empty string here — an empty wording means "the substrate's own".
	AliasNotFound string

	// AliasNotFoundUnprefixed writes it without the shell and line in front,
	// which dash and ksh93 do here and almost nowhere else.
	AliasNotFoundUnprefixed bool

	// UnaliasNotFound is the same for `unalias`, and is a separate field
	// because zsh words it differently from anything its `alias` says — "no
	// such hash table element: nope", where its `alias` says nothing at all.
	UnaliasNotFound string

	// UnaliasNotFoundUnprefixed is that question for the `unalias` wording.
	UnaliasNotFoundUnprefixed bool

	// UnaliasUsage is what `unalias` prints when given no name and no -a.
	// Empty means it prints nothing, which is dash.
	UnaliasUsage string

	// UnsetNoOperands is what `unset` prints when it has nothing to unset:
	// no operand at all, no name after `-v` or `-f`, and no pattern after
	// `-m`. One verb, the builtin's name — which is `unfunction` where the
	// dialect renames the `-f` half, since the complaint follows the invoked
	// word.
	//
	// One field rather than one per spelling because the panel answers it
	// once: measured 2026-09-08, zsh 5.9.2 writes the same `not enough
	// arguments` for all four, and bash 5.3, bash 3.2 and dash are silent at
	// 0 for every one they have. Empty is therefore a real answer and not a
	// gap — the dialect says nothing and the builtin succeeds.
	//
	// ksh93 is the one member this does not cover: it answers with its usage
	// line and ends the script, `unset` being one of its special builtins.
	// That is the usage-line shape and not this sentence, so it is recorded
	// in the corpus rather than approximated here.
	UnsetNoOperands string

	// UnaliasUsageUnprefixed writes that without the shell and line in front.
	UnaliasUsageUnprefixed bool

	// UnaliasNoOperandStatus is what that reports. Zero means 2, except where
	// UnaliasUsage is empty and nothing was wrong, which is 0.
	UnaliasNoOperandStatus int

	// UnaliasAllWithOperands is `unalias -a` given a name as well. No verbs.
	// Only zsh refuses it, which UnaliasAllRefusesOperands answers.
	UnaliasAllWithOperands string

	// AliasListPrefix goes in front of every line of a listing. `alias ` in
	// bash, which is what makes its output text that can be read back, and
	// empty in the other three.
	AliasListPrefix string

	// UmaskBadSymbolicOperator is that complaint where what was wanted was
	// one of `+-=` rather than one of `rwx`, for the two dialects that tell
	// them apart: bash says "invalid symbolic mode operator" against
	// "invalid symbolic mode character", and zsh "bad symbolic mode
	// operator" against "bad symbolic mode permission".
	//
	// Empty means the dialect says the same thing to both, which is dash and
	// ksh93 — they name the argument and never reach the question.
	UmaskBadSymbolicOperator string
	// UmaskWhoAloneIsANumericComplaint answers `umask g` with the wording a
	// number it could not read gets, rather than with a symbolic one. zsh
	// alone, which says `bad umask` there and `bad symbolic mode operator:
	// X` for `umask X` — so which complaint it reaches for is not the same
	// question as whether it refuses.
	UmaskWhoAloneIsANumericComplaint bool

	// HereDocumentAtEOF is what a shell says about a here-document whose
	// delimiter never arrived, taking the line it began on and the delimiter
	// that was wanted. Empty means nothing is said, which is three of the
	// four — silence is the answer here rather than a missing wording.
	//
	// The remark is located where the input ran out and names the other line
	// inside itself, which is why it takes a line as a verb at all.
	HereDocumentAtEOF string

	// WaitBadJob is an operand to `wait` that names neither a process nor a
	// job, taking the word. Four wordings across the panel and no two alike:
	// one quotes it and names both things it could have been, one calls it an
	// illegal number, one lists what it would have taken, and one calls it a
	// job that was not found.
	WaitBadJob string
	// WaitBadJobStatus is what that reports. Zero means the substrate's own,
	// which is 2. Three answers: 1, 2, and the 127 of a command that is not
	// there — which is what the dialect saying "job not found" takes the
	// operand to have been.
	WaitBadJobStatus int

	// WaitNoSuchJob is a job spec `wait` cannot resolve — asked past
	// Semantics.WaitReportsAMissingJob, whose No is the engine that says
	// nothing at all. One verb: the spec as written. The status rides
	// WaitNoSuchJobStatus; zero means 127, a missing command's number,
	// which two of the three that speak report.
	WaitNoSuchJob       string
	WaitNoSuchJobStatus int

	// AmbiguousJobSpec is `%name` matching more than one job, in the one
	// engine that refuses it — see Semantics.AmbiguousJobNameIsRefused.
	// Two verbs: the builtin, and the text with its `%` already stripped,
	// which is how the engine writes it.
	AmbiguousJobSpec string

	// WaitNotOurChild is a number that is a plausible process id and is not
	// one of this shell's children, taking the number. Empty means nothing
	// is said, which is two of the four — the status is 127 in all of them
	// either way, so silence here is a wording rather than a behavior.
	WaitNotOurChild string

	// UnimplementedOptionLetters are, per builtin, the option letters this
	// dialect has and this shell does not.
	//
	// They are said to be *missing* rather than refused as unknown, because
	// refusing an option the shell really has is a different and worse
	// answer than not having it yet — and a script that meets it gets told
	// which of the two happened.
	UnimplementedOptionLetters map[string]string

	// ReadBadNumber is a count, timeout or descriptor argument to `read`
	// that is not a number, taking the word. The panel has a wording per
	// shell per letter; this is one for all of them, and a dialect that
	// wants the measured ones letter by letter is a refinement this field
	// does not block.
	ReadBadNumber string
	// ReadBadFileDescriptor is `read -u` on a descriptor this shell holds
	// nothing open at, taking the number as given. Empty means nothing is
	// said — one shell in the panel reports 1 in silence — so this path has
	// no fallback wording.
	ReadBadFileDescriptor string
	// ReadTimeoutStatus is what an expired `read -t` reports. bash says 128
	// plus SIGALRM's number; ksh93 and zsh say 1. Zero means 1.
	ReadTimeoutStatus int
	// ReadNoCoprocess is `read -p` in a dialect whose -p takes no argument
	// and names the coprocess as the source — there being none to read
	// from. Both shells with that shape report 1 and leave the variables
	// untouched; the words differ: ksh93 says `no query process`, zsh says
	// `-p: no coprocess`.
	ReadNoCoprocess string

	// CoprocessAlreadyRunning is a second `cmd |&` started while the first
	// coprocess is still running, in the dialect that spells a coprocess as
	// an operator. ksh93 says `process already exists` and it is fatal —
	// measured, the script ends at status 1 with the next line unreached.
	//
	// Only that dialect reaches it: the `coproc` word replaces its
	// predecessor silently in both shells that have it, so there is nothing
	// for them to word. Zero is the same text, because the construct exists
	// in one shell and its wording is not a variable.
	CoprocessAlreadyRunning string

	// BuiltinComplaintName is the name a builtin's complaints call it, by
	// the name it was invoked as, for a dialect whose complaint names a
	// different one. ksh93's `type` is `whence -v` and its refusals say so:
	// `type -t echo` there answers `whence: -t: unknown option`, with
	// `whence`'s usage line under it. Lookups keyed by builtin —
	// UnimplementedOptionLetters, BuiltinUsage — still use the invoked
	// name; only the wording changes.
	BuiltinComplaintName map[string]string
	// ShiftBadNumber is an operand to `shift` that is not one, taking the
	// word. Only reached in a dialect that reads the operand as a count
	// rather than as an option: bash names it and asks for a number, dash
	// calls it an illegal one.
	ShiftBadNumber string

	// BuiltinBadSubscript is what a declaration says about a subscripted
	// operand it will not take, per builtin. Three verbs: %[1]s the builtin,
	// %[2]s the whole operand, %[3]s the name before the subscript.
	//
	// Empty means the dialect says what it says about any other bad name,
	// which is two of the three that refuse it. The third has two complaints
	// of its own — one about the subscript naming the base, one about array
	// elements naming the operand — and they are not its bad-name wording.
	BuiltinBadSubscript map[string]string
	// SubscriptRefusalNamesBuiltin is which of those name the builtin in the
	// *location*. One does and one does not, in the same shell, which is why
	// this is a set rather than following NamesBuiltinInLocation.
	SubscriptRefusalNamesBuiltin map[string]bool

	// ReadonlyElementRefusal, IntegerElementRefusal and LocalElementRefusal
	// are what a declaration says about a subscripted operand whose element
	// cannot carry what the declaration is asking the *variable* to be. Two
	// verbs each: %[1]s the base name, %[2]s the subscript as written.
	//
	// One dialect has all three and the others have none, because the others
	// take the operand — see Semantics.ReadonlyElement and its two
	// neighbors. Three strings rather than one because the shell that has
	// them words the three differently, and it is the wording that tells a
	// script which of the three it ran into.
	ReadonlyElementRefusal string
	IntegerElementRefusal  string
	LocalElementRefusal    string

	// UmaskBadOption is an option `umask` does not have. One verb.
	UmaskBadOption string

	// UmaskBadOptionStatus is what that reports. Zero means the substrate's
	// own, which is 2 — zsh alone says 1, where it says 2 for a bad *mask*.
	UmaskBadOptionStatus int

	// UmaskUsage follows a bad option, where the dialect prints one. bash and
	// ksh93 do; dash and zsh print the complaint alone. Empty means none.
	UmaskUsage string

	// UmaskUsageUnprefixed writes it with no location and no shell name in
	// front, which is what ksh93 does with a usage line.
	UmaskUsageUnprefixed bool

	// LetNoExpression is `let` with nothing to evaluate. No verbs.
	//
	// bash `let: expression expected`, ksh93 a bare usage line, zsh
	// `not enough arguments`.
	LetNoExpression string

	// LetNoExpressionStatus is what that reports. Zero means the substrate's
	// own, which is 1 — ksh93 alone says 2, treating it as a usage error
	// where bash and zsh treat it as an ordinary failure.
	LetNoExpressionStatus int

	// LetNoExpressionUnprefixed writes that complaint with no location and no
	// shell name in front. ksh93 alone, which is what it does with every
	// usage line.
	LetNoExpressionUnprefixed bool

	// UlimitBadOption is an option `ulimit` does not have — which includes a
	// resource letter this dialect lacks. One verb: the letter.
	UlimitBadOption string

	// UlimitBadOptionStatus is what that reports. Zero means the substrate's
	// own, which is 2.
	UlimitBadOptionStatus int

	// UlimitBadNumber is a limit it could not read. One verb.
	UlimitBadNumber string

	// UlimitBadNumberStatus is what that reports. Zero means 1.
	UlimitBadNumberStatus int

	// UlimitListing is `ulimit -a`, one row per line in the dialect's own
	// order and layout — see UlimitListingRow. Empty refuses the letter's
	// listing as the unanswered question it is.
	UlimitListing []UlimitListingRow

	// UlimitCannotChange is the kernel refusing the change — raising a hard
	// limit, most often. One verb: the reason.
	UlimitCannotChange string

	// BuiltinBadOption is an option a builtin does not have. Two verbs: the
	// builtin's name and the option as written.
	//
	// One wording rather than one per builtin, because the shape is the same
	// for every one of them within a dialect: bash `export: -Q: invalid
	// option`, dash `export: Illegal option -Q`, ksh93 `export: -Q: unknown
	// option`, zsh `export: bad option: -Q`.
	BuiltinBadOption string

	// BuiltinBadOptionStatus is what that reports where it is not fatal.
	// Zero means the substrate's own, which is 2 — zsh says 1.
	// BadOptionNaming is which part of a leading `-` word a complaint about
	// it names. Three answers, and `--version` tells them apart:
	//
	//	bash, dash   --            the first letter it cannot use
	//	ksh93        --version     the whole word as written
	//	zsh          -v            the first letter it does not know, with
	//	                           every leading dash skipped first
	//
	// zsh's is the one that needs saying twice: `unset --version` is `-e`
	// there, not `-v`, because `v` is one of unset's own options and is
	// consumed before an unknown letter is reached. That is the tell that it
	// walks the bundle rather than naming a piece of the word.
	//
	// A single-dash bundle sharpens the second answer: `read -rx` is `-x` in
	// ksh93 too — every shell walks the bundle and names the letter it
	// stopped on — so the whole word survives only for a word that begins
	// with `--`, which is where the three rules diverge at all.
	BadOptionNaming BadOptionName

	BuiltinBadOptionStatus int

	// OptionNeedsArgument is an argument-taking letter whose bundle ended
	// the argument list — `read -p` with nothing after it. Two verbs: the
	// builtin's name and the letter without its dash. The status is
	// BuiltinBadOptionStatus's, which is measured: every shell reports this
	// the way it reports an option it does not have, 2 everywhere but zsh's
	// 1, and bash and ksh93 print the same usage line after either.
	//
	// The fallback is bash's own wording — `read: -p: option requires an
	// argument` — which the substrate had said before any dialect was
	// measured saying it. ksh93 names the argument it wanted per letter
	// (`-d: delim argument expected`), which one string per dialect does
	// not carry; it keeps the fallback.
	OptionNeedsArgument string

	// BuiltinBadName is what a builtin says about an operand that is not a
	// name, by builtin name. Two verbs: the builtin and the operand.
	//
	// A map where BuiltinBadOption is one string, because two of the four
	// word this one per builtin where they word that one per dialect: ksh93
	// says `export: 1x: is not an identifier` and `readonly: 1x: invalid
	// variable name`, and zsh writes the reason before the operand for
	// `export` and `readonly` and after it for `unset`.
	BuiltinBadName map[string]string

	// BuiltinBadNameNumeric is that wording where the operand begins with a
	// digit, for the one dialect that tells the two apart: zsh says `not an
	// identifier: 1x` for `1x` and `not valid in this context: a-b` for
	// `a-b`. An empty entry means the dialect says the same to both.
	BuiltinBadNameNumeric map[string]string

	// BuiltinBadNameStatus is what that reports where it is not fatal. Zero
	// means 1 — dash says 2.
	BuiltinBadNameStatus int

	// BuiltinBadNameKeepsValue quotes the operand back as written, `1x=v` and
	// all, rather than the name in front of the `=`. True in bash and ksh93;
	// dash and zsh name `1x`.
	BuiltinBadNameKeepsValue bool

	// BuiltinUsage is the usage line that follows, by builtin name. bash and
	// ksh93 print one and word it per builtin, which is why this is a map
	// where the complaint above is a single string. dash and zsh print none.
	BuiltinUsage map[string]string

	// BuiltinHelp is what a builtin answers `--help` with, by builtin name.
	// An entry is the *whole* answer and goes to standard output, which is
	// what tells this apart from every neighbor here: a refusal is a
	// diagnostic and this is not one.
	//
	// A name with no entry has no such answer, and `--help` is then an
	// option the builtin does not have — which is the ordinary refusal
	// above, and is what one whole dialect does for every builtin it has.
	// So the map answers "does this builtin answer --help" as well as
	// "with what", and no separate axis says whether the shell has the
	// option at all.
	//
	// The word has to be exactly `--help` and has to stand where an option
	// stands. Measured: an abbreviation is refused, so is `--help=x`,
	// `builtin alias -- --help` is an operand, and `builtin read -d --help`
	// hands `--help` to `-d` as its argument — which is why the shared
	// option parser answers this rather than a pre-scan of the argument
	// list, since only the parser knows which letters take an argument.
	BuiltinHelp map[string]string

	// BuiltinHelpStatus is what a builtin exits with after answering
	// `--help`. Zero means 2, the substrate's answer for the adjacent
	// question above — a builtin asked about its own usage — and the same
	// default BuiltinBadOptionStatus takes.
	//
	// It defaults to something rather than to nothing on purpose. The
	// option parser reports "the builtin is finished" to its thirteen
	// callers as a nonzero code and has no other way to say it, so a help
	// status of 0 would print the answer and then run the builtin anyway.
	// A default that cannot be zero keeps that unreachable.
	BuiltinHelpStatus int

	// The four lines `type` prints, one per kind of thing a name can be.
	// One verb each — the name — except TypeExternal, which takes the path
	// as a second.
	//
	//	bash   if is a shell keyword     ls is /bin/ls
	//	dash   if is a shell keyword     ls is /bin/ls
	//	ksh93  if is a keyword           ls is a tracked alias for /bin/ls
	//	zsh    if is a reserved word     ls is /bin/ls
	//
	// A function is the same story again: "a function" in two of them, "a
	// shell function" in a third, and a fourth that names itself in the line.
	TypeKeyword  string
	TypeBuiltin  string
	TypeFunction string
	TypeExternal string

	// TypeUndefinedFunction is that line again for a function whose body has
	// not been read yet, in the two shells that have such a thing. One verb,
	// the name, and empty in a shell where a function is a function:
	//
	//	zsh    myfn is an autoload shell function
	//	ksh93  myfn is an undefined function
	//
	// Which name is one is [Runner.SetUndefinedFunctions], asked in the same
	// place and by the same rule that decides how it lists.
	TypeUndefinedFunction string

	// TypeNotFound is a name `type` could not account for. One verb: the
	// name. Two of the four write it with no shell name or location in
	// front, which TypeNotFoundUnprefixed says.
	//
	//	bash   bash: line 1: type: nope: not found
	//	dash   nope: not found
	//	ksh93  ksh: whence: nope: not found
	//	zsh    nope not found
	TypeNotFound           string
	TypeNotFoundUnprefixed bool

	// TypeNotFoundOnStdout writes that line to standard output rather than
	// to standard error, which is half the panel:
	//
	//	bash   standard error
	//	dash   standard output
	//	ksh93  standard error
	//	zsh    standard output
	//
	// It is a question of its own and not a consequence of the wording. Two
	// shells treat a name they could not account for as a *report* — part of
	// what the reader asked `type` for, and so an answer — and two treat it
	// as a complaint about the request. Nothing else about the builtin
	// follows from which: the status is settled separately by
	// TypeNotFoundStatus, and the prefix by TypeNotFoundUnprefixed.
	//
	// The stream is visible in ways the wording is not. On the two shells
	// that report it, `type nope 2>/dev/null` still prints the line and
	// `p=$(type -p nope)` captures it; on the two that complain, both are
	// silent. It is also why a multi-name invocation reads in order there —
	// `type -t f cd if ls` puts every line, found or not, in one stream.
	//
	// A dialect that both prefixes the line and reports it on standard
	// output is not in the panel, but the two fields do not constrain each
	// other: the prefix is chosen first and the stream carries whatever
	// results.
	TypeNotFoundOnStdout bool

	// TypeNotFoundStatus is what `type` reports when a name was not
	// accounted for. Zero means 1, which is three of the four; dash answers
	// with a missing command's 127.
	TypeNotFoundStatus int

	// CommandVNotFound is what `command -V` says about a name that is
	// nothing, which is `type`'s complaint with a different name in front:
	// two shells blame `command`, and the two that keep the shell's name off
	// the line here keep it off there too — TypeNotFoundUnprefixed,
	// TypeNotFoundOnStdout and TypeNotFoundStatus speak for both builtins.
	// One verb, the name.
	CommandVNotFound string

	// FunctionListingHeader is how a function said back whole begins —
	// `declare -f`, and the `type` that follows its sentence with the body.
	// Two verbs: the name, and the laid-out body, which starts at its
	// opening brace. bash gives the brace a line of its own and zsh keeps it
	// on the header's, which is why the join is the dialect's to word; the
	// layout inside the braces is the dialect's function layout.
	FunctionListingHeader string

	// BuiltinUsageUnprefixed writes it with no location and no shell name in
	// front, which is what ksh93 does with every usage line.
	BuiltinUsageUnprefixed bool

	// JobLine is one row of a `jobs` listing. Four verbs: the number, the
	// marker that says which job `%%` means, the state and the command.
	//
	// All four are filled in, and none of them agrees with another about any
	// of it — measured to the byte:
	//
	//	bash   [1]-  Running                    sleep 0.3 &
	//	dash   [2] + Running
	//	ksh93  [2] +  Running                 <command unknown>
	//	zsh    [1]  - running    sleep 0.3
	//
	// The marker spacing, the width of the state column and the case of the
	// word are all this string's business. Whether the command is there is
	// not: dash and ksh93 kept no text for a `&` job, which
	// JobsShowBackgroundCommand answers, and JobUnknownCommand is only what
	// to print in its place.
	//
	// ksh93's running state carries a leading space of its own, so that its
	// stopped and running lines end in the same column. Odd, and exactly
	// what it does.
	JobLine string

	// JobLineLong is the same row as `jobs -l` writes it, with the process
	// id in it. Five verbs: the number, the marker, the process id, the
	// state and the command.
	//
	// A separate format rather than a field spliced into JobLine, because
	// where the id goes is not one rule. Measured to the byte, with a
	// five-digit id:
	//
	//	bash   [1]+ 41293 Running                    sleep 0.4 &
	//	dash   [1] + 41293 Running
	//	ksh93  [1] + 41293\t Running                 <command unknown>
	//	zsh    [1]  + 41293 running    sleep 0.4
	//
	// bash spends one of the two spaces after its marker on the id; the
	// other three insert the id and keep the spacing they had. ksh93 puts a
	// tab after it. And dash narrows its state column by exactly what the id
	// took, so that the command column stays where it was — the only shell
	// in the panel that does, and invisible in practice, because dash keeps
	// no command text and what moves is trailing whitespace. The width here
	// is the measured one for a five-digit id rather than that arithmetic.
	//
	// Empty means JobLine with the id and a space in front of the state,
	// which no dialect in the panel relies on.
	JobLineLong string

	// JobRunning, JobStopped and JobDone are the states a job is listed in.
	// No verbs.
	//
	// JobDone is the one a job is in exactly once: the listing that reports
	// it is the listing that forgets it.
	JobRunning string
	JobStopped string
	JobDone    string

	// JobDoneNotice replaces JobDone when the shell is *reporting* that a job
	// ended, rather than listing one that has. Empty uses JobDone for both,
	// which is what three of the four want. No verbs.
	//
	// ksh93 needs the two: it announces `Done` and then lists the same job
	// as `Running`, because its listing has not noticed what its reaper
	// already said. Two statements about one job, and both are ksh's.
	JobDoneNotice string

	// JobExited replaces JobDone where the job ended with a non-zero status.
	// One verb: the status. Empty leaves JobDone standing for both, which is
	// what a dialect that does not distinguish them wants.
	//
	//	bash   Exit 1
	//	dash   Done(1)
	JobExited string

	// JobUnknownCommand is printed in the command column of a job whose text
	// the shell did not keep. No verbs.
	//
	// ksh93 alone: `<command unknown>`, where dash leaves the column empty.
	// Whether the text was kept is the semantics question — this is only
	// what to print in its place.
	JobUnknownCommand string

	// JobStoppedNotice is what an interactive shell says when the job in
	// front of it stopped — what ^Z prints. Four verbs: the job number, the
	// marker, the shell's own name and the command.
	//
	// Empty prints the `jobs` listing's own row, which is what three of the
	// four do: `[1]+  Stopped   sleep 40` in bash, and the same shape in
	// dash and ksh93 with each one's own state word. zsh writes a sentence
	// instead and names itself in it, which no listing row does.
	JobStoppedNotice string

	// JobStoppedNoticeOnANewLine starts that notice on a line of its own.
	//
	// The terminal echoed `^Z` where the cursor was and left it there. bash
	// and zsh write a newline before the notice, so it lands under the echo;
	// dash and ksh93 write it straight after, on the same line. Measured
	// through a pseudo-terminal, which is the only place the difference
	// exists.
	JobStoppedNoticeOnANewLine bool

	// JobResumedInForeground is how `fg` names the job it put back in front.
	// Three verbs: the number, the marker and the command.
	//
	// Empty prints the command alone, which is what three of the four do.
	// zsh prints a listing row with a state of its own — `[1]  + continued
	// sleep 3` — and that word appears nowhere else, which is why this is a
	// format rather than a fourth entry beside JobRunning and JobStopped.
	JobResumedInForeground string

	// JobResumedInBackground is the same for `bg`, and here all four differ:
	// bash writes the row's head and the command with an `&` after it, dash
	// the number and the command, ksh93 a tab between them and no space
	// before the `&`, and zsh the same `continued` row it prints for `fg`.
	//
	// Empty prints the command with ` &` after it.
	JobResumedInBackground string

	// StoppedJobsAtExit is the warning an interactive shell gives when
	// leaving would abandon a stopped job — see
	// Semantics.StoppedJobsHoldTheExit, which is what decides whether it
	// stays at all. One verb: the shell's own name.
	//
	// It wins over RunningJobsAtExit where there is one of each: measured,
	// a session holding a suspended job and a `sleep 40 &` is told about the
	// suspended one in both shells that say anything.
	//
	// Written without the location prefix every other diagnostic carries:
	// one of the two shells that says this names itself in the sentence and
	// the other names nobody, and neither writes a line number.
	StoppedJobsAtExit string

	// RunningJobsAtExit is the same warning for a job that is still running,
	// which a shell gives only where the session has asked it to — see
	// Runner.ChecksRunningJobsAtExit. Measured through a pseudo-terminal:
	// bash 5.3.15 with `shopt -s checkjobs` says `There are running jobs.`
	// and zsh 5.9.2 says `zsh: you have running jobs.`, each the same shape
	// as its own stopped-job sentence with one word changed.
	//
	// A second wording rather than a parameter of the first, because the two
	// shells do not build it the same way: one says `stopped`/`running` where
	// the other says `suspended`/`running`, so there is no shared sentence
	// with a word in it.
	RunningJobsAtExit string

	// StoppedJobsAtExitStatus is what the `exit` that was held back reports.
	// Zero is what zsh answers, which is also the shape of a dialect that
	// never holds an exit at all; bash answers 1, a builtin that failed.
	//
	// One number for both kinds of job, which is measured and not assumed:
	// `exit` held back by a *running* job reports 1 in bash and 0 in zsh,
	// exactly as the stopped one does.
	StoppedJobsAtExitStatus int

	// EmptyRedirectTarget replaces CannotOpen and CannotCreate where the
	// target expanded to nothing. One verb: the name, which is empty — it is
	// there so the shape matches the other two rather than because it says
	// anything.
	//
	// ksh93 alone, and it is two departures at once: no bracketed reason,
	// and "open" even where the redirection was creating. Empty leaves the
	// ordinary wordings standing, which is what the other three want.
	EmptyRedirectTarget string

	// AmbiguousRedirect is a redirection whose target did not expand to
	// exactly one word. One verb: the target *as it was written*, which is
	// what bash names — `$e`, not what `$e` came to.
	//
	// Only the dialect that expands a target as an ordinary word has one,
	// because only there can the result be a number of words other than one.
	AmbiguousRedirect string

	// DuplicationTargetIsNotADescriptor is `>&word` or `<&word` where the
	// word expanded to exactly one thing and that thing is not a descriptor
	// number. Two verbs: the target *as it was written*, and what it
	// expanded to.
	//
	// The verbs are two because the shells name two different things. bash
	// calls it an ambiguous redirect and names the expansion; ksh93 says the
	// file unit number is bad and names the expansion too; zsh names nobody
	// at all and says only that a file number was expected, which is why a
	// wording here may use neither verb.
	DuplicationTargetIsNotADescriptor string

	// EmptyDuplicationTarget replaces it where the word expanded to nothing.
	// The same two verbs, and here the first earns its place: bash writes
	// `"": Bad file descriptor` for `<&""`, quotation marks and all, which is
	// the target as it was written and not the empty string it came to.
	//
	// Empty leaves DuplicationTargetIsNotADescriptor standing, which is what
	// zsh wants — it says the same thing either way — and what the core
	// wants, which has nothing else to say.
	EmptyDuplicationTarget string

	// ExportNotAFunction is `export -f` given a name that is not one. One
	// verb: the name.
	ExportNotAFunction string

	// ExportFunctionOptionRefused is `export -f` in a dialect that knows the
	// letter and will not carry a function. No verbs.
	//
	// One shell needs it, and needing it is the measurement: it answers
	// `export -q` with `bad option: -q` and `export -f` with `invalid
	// option(s)`, naming the letter in one and not the other. `-f` is not
	// unknown to it — it means functions to that shell's `typeset` — so what
	// it refuses is the combination rather than the letter, and it says so
	// differently. Empty leaves `-f` to the ordinary unknown-option path,
	// which is what the other two want.
	ExportFunctionOptionRefused string

	// ReturnOutsideAFunction is a `return` with nothing to return from —
	// neither a function nor a sourced file. No verbs.
	//
	// Only one shell in the panel says anything at all: the other three obey
	// it and end the script, so there is nothing for them to word.
	ReturnOutsideAFunction string

	// LoopControlOutsideALoop is a `break` or a `continue` with no loop
	// around it. One verb: the builtin's own name.
	//
	// The panel divides three ways and the wording carries only the first
	// two. dash and bash called as `sh` say nothing at all, so their answer
	// is the empty string; bash names the three loops POSIX has, and zsh
	// names the four it has. Whether the misuse also stops the script is a
	// separate question — see Semantics.LoopControlOutsideALoopIsFatal —
	// because bash says this and carries on.
	//
	// Measured 2026-09-10 with `echo t; break; echo after`:
	//
	//	dash        nothing, `after` runs, status 0
	//	bash 5.3    break: only meaningful in a `for', `while', or `until' loop
	//	bash-as-sh  nothing, `after` runs, status 0
	//	bash 3.2    the same as bash 5.3, at its own line number
	//	zsh         break: not in while, until, select, or repeat loop
	//
	// The name is written into the message here and stripped again by the
	// dialect that puts a builtin's name in the location instead, which is
	// how `zsh:break:1: not in while, …` comes out without saying `break`
	// twice.
	LoopControlOutsideALoop string

	// UnsetBadFunctionName is what `unset -f` says about an operand that
	// could not be a function name. One verb: the operand.
	UnsetBadFunctionName string

	// UnsetFunctionNotFound is what `unset -f` says about a name no function
	// has. One verb: the name. zsh words it about its table rather than
	// about the function, and puts the builtin in the location as it does
	// with every message.
	UnsetFunctionNotFound string

	// BadArraySubscript is what an assignment says about a subscript that
	// lands before the array's first element. Two verbs: the name, and the
	// subscript *as written* — bash names `a[x-2]`, not the `-1` it evaluated
	// to, where ksh93 and zsh name the array alone.
	BadArraySubscript string

	// BadArrayLiteralSubscript is the same refusal reached through an array
	// literal, `a=([0]=p)`, which two of the three word differently from the
	// plain form. Three verbs: the name, the subscript as written, and the
	// value — bash names the element as it stands in the parentheses,
	// `[-1]=p`, and zsh names the subscript alone. Empty falls back to
	// BadArraySubscript, which is the honest answer for a dialect that has
	// not been measured to word the two apart.
	BadArrayLiteralSubscript string

	// ArrayLiteralThroughASubscript is what the refusing shell says about
	// `a[i]=(p q)`. Two verbs: the name, and the subscript *as written* —
	// `a[$i]`, not the 1 it evaluated to, which is measured rather than
	// assumed: bash quotes the text back in both builds. One sentence covers
	// every target, an array, a scalar, a declared table and an unset name
	// alike, because what it objects to is the list and not the name.
	//
	// Only a dialect answering SubscriptedArrayLiteralRefused has anything to
	// put here.
	ArrayLiteralThroughASubscript string

	// ArrayValueToNonArray is what the splicing shell says when the name a
	// subscripted literal writes through holds a plain string. One verb: the
	// name, without the subscript. An *unset* name is not this — it becomes
	// an array — so what it reports is a name already holding something that
	// is not one.
	ArrayValueToNonArray string

	// SliceOfAnAssociativeArray is the same refusal for a declared table,
	// which the splicing shell words differently: a subscript over a table
	// names a key rather than a position, so there is no span for the words
	// to replace. One verb: the name.
	SliceOfAnAssociativeArray string

	// AppendToANumericSlice is what the shell that splices characters says
	// when `+=` is written at a subscript of a name carrying an arithmetic
	// attribute. It takes the name, which that shell's own sentence does not
	// use — the location carries it — so this is a field of its own rather
	// than one of the two above worded again. A number has no characters for
	// a span to name and so nothing for the value to join: where a plain `=`
	// at the same subscript quietly lands whole, this one is refused and ends
	// the script. Only a dialect answering ScalarSubscriptIsACharacter Yes has
	// anything to put here.
	AppendToANumericSlice string

	// UnsetBadSubscript wraps the sentence about an `unset` operand whose
	// subscript would not evaluate. One verb: that sentence, already worded by
	// ArithError. Empty leaves it to stand alone, which is what bash and zsh
	// do; ksh93 alone names the builtin in front of it, having worded the
	// identical failure in an expansion without one.
	UnsetBadSubscript string

	// UnsetSubscriptBeforeTheFirstElement is what `unset a[i]` says about a
	// subscript that lands before the array's first element. Two verbs: the
	// name, and the subscript *as written*.
	//
	// A field of its own rather than BadArraySubscript used twice, because
	// two of the three shells that reach the boundary word the `unset` route
	// differently from the assignment: one drops the array's name and keeps
	// the bare subscript, and both put the builtin's name in front. The third
	// says the same sentence by both routes, which is what makes a shared
	// field look adequate until it is measured.
	UnsetSubscriptBeforeTheFirstElement string

	// UnsetNotAnArray is what `unset a[@]` says when the name holds a value
	// that is not an array. One verb: the name.
	//
	// Only the shell that answers UnsetArraySpanRemovesTheElements has
	// anything to say here, and it is the same in both builds measured: the
	// spelling means "take every element away" and a scalar has none to take,
	// so it is refused rather than emptied. The shell that leaves one empty
	// element instead treats a scalar as the single element it is and empties
	// it without a word, so it words nothing.
	UnsetNotAnArray string

	// ExpansionFailureStatusFromCommandString is what a shell exits with when
	// an expansion failed *and the program came from an argument* — `-c` —
	// rather than from a file or standard input.
	//
	// bash alone, and only there: 127 given with `-c`, 1 from a file or from
	// standard input, for `set -u`, `${x?}` and a `@` transformation whose
	// letter does not exist alike. Measured over eight other ways it stops,
	// none of which differs by invocation. Zero leaves the dialect's
	// ordinary fatal status standing either way.
	//
	// It covers a *failed* expansion and not an unreadable word, which is
	// the line the same shell draws itself: `${x@QQ}` on a value exits 127
	// under `-c` and `${(q)x}` — a bad substitution for a different reason,
	// found while reading the word rather than while expanding it — exits 1
	// from the same invocation.
	ExpansionFailureStatusFromCommandString int

	// ParamErrorMessage is what `${x?word}` says. Two verbs: the parameter
	// and the word. The shape is unanimous — `x: word` — and only the
	// default word below is not.
	ParamErrorMessage string

	// ParamNullOrNotSet is the word `${x:?}` uses when none was given. That
	// form covers two cases at once and each shell says so differently:
	//
	//	bash   parameter null or not set
	//	dash   parameter not set or null
	//	ksh93  parameter null  (only when it is there and empty)
	//	zsh    parameter not set
	//
	// Empty falls back to `parameter not set`, which is what plain `${x?}`
	// says in all four and what zsh says for both forms.
	ParamNullOrNotSet string

	// ParamNull is the word for a parameter that is *there and empty*, where
	// a shell tells that from one that is absent. ksh93 alone.
	ParamNull string

	// SelfName is what the shell calls itself in a diagnostic, whatever it was
	// invoked as.
	//
	// Three of the four name themselves by argv[0], verbatim and however long
	// it is, which is what an empty value here means and what `$0` reports on
	// the two routes that have no other name for it. Measured by handing each
	// shell an argv[0] of its own:
	//
	//	exec -a weirdname bash <<< 'if'
	//	weirdname: line 2: syntax error: unexpected end of file
	//
	//	exec -a weirdname zsh <<< 'if'
	//	zsh: parse error near `\n'
	//
	// So it is not a shortening of argv[0] — a symlink named `myzsh`, and a
	// symlink named `sh`, both still say `zsh`, and `exec -a` says so most
	// plainly. The name is fixed, and `$0` is unaffected: it still reports the
	// path the shell was invoked by, which is why this is a separate answer
	// rather than a different value for Runner.Name.
	//
	// It applies only where the shell names *itself*, which is the two routes
	// that have no file: `-c` and standard input. A script is named by its
	// own path — measured, all four print the script and not the shell — so
	// the script route keeps Runner.Name, which is the path. That is the same
	// three-way split `$0` is decided by rather than a second rule.
	SelfName string

	// LocationNamesTheCurrentFile puts the file a failing line was read from
	// where the shell's name would go: a sourced file while it runs, and the
	// file a function was defined in when the function is called after the
	// sourcing has finished — `./inc.sh: line 1: nosuch: command not found`
	// rather than `outer.sh: line 1: …`. The name is the operand as the
	// shell constructed it — `./inc.sh` as written, the joined path for a
	// PATH hit — never the resolved absolute path.
	//
	// bash and zsh, and in zsh only outside a function, where
	// LocationNamesTheFunction has not already replaced the name. dash and
	// ksh93 keep the script's own name in both cases while still counting
	// the sourced file's lines; while the sourced file runs each also labels
	// it in its own place — dash writes the path after the location,
	// `outer.sh: 3: ./inc.sh: …`, and ksh93 writes `.: line 3:` after a
	// location pinned where the `.` was — which the corpus records rather
	// than this field claiming it.
	LocationNamesTheCurrentFile bool

	// LocationNamesTheFunction puts the function a message came from where
	// the file's name would go, and counts the line within the function
	// rather than within the file. zsh alone.
	//
	// The count is the offset from the line the function was written on, so
	// a body on the same line as its `f() {` is offset zero — and zsh leaves
	// the number out entirely there rather than writing a nought.
	LocationNamesTheFunction bool

	// SetInvalidOptionName is a long `set -o` name this shell does not have.
	// One verb: the name.
	//
	//	bash   set: bogusname: invalid option name
	//	dash   set: Illegal option -o bogusname
	//	ksh93  set: bogusname: bad option(s)
	//	zsh    set: no such option: bogusname
	//
	// dash writes `-o` whichever way it was asked, so the operator is part
	// of the wording rather than a verb. zsh puts the builtin's name in the
	// location instead of in the sentence, which the location already does.
	// ksh93 follows it with the usage line it keeps in BuiltinUsage.
	SetInvalidOptionName string

	// SetImmovableOptionName is a long `set -o` name this shell *has* and
	// will not move. One verb: the name, as the script spelled it.
	//
	//	default   set: chaselinks: not implemented
	//	zsh       set: can't change option: chaselinks
	//
	// Not the same answer as SetInvalidOptionName, and the difference is the
	// one a script can act on: the first is a shell that is missing
	// something and the second is a typo. The default is this
	// implementation's own words rather than a shell's, because the shells
	// that reach it are answering about a name they have and we do not do —
	// there is no borrowed sentence for that.
	//
	// Only a dialect with an option table of its own (SetOptionTable) can
	// reach the *spoken* form of this: without one, a name outside the
	// substrate's table is an invalid name instead. zsh says it about the
	// five options that are about being interactive — `interactive`,
	// `monitor`, `shinstdin`, `singlecommand` and `zle` — measured in a
	// non-interactive `-c` run, and about nothing else in the other 180.
	//
	// Its status and whether it ends the script are SetInvalidOptionStatus
	// and Semantics.BadSetOptionNameFatal, the same two the invalid name
	// uses: measured, zsh answers `set -o onecmd` and `set -o zzznosuch`
	// with the same 1 and stops the script at both.
	SetImmovableOptionName string

	// ImmovableOptionLetters are, per builtin, the option letters this shell
	// *has* and will not move — the letter half of SetImmovableOptionName,
	// and a third answer beside "does not have it" and "has not built it".
	//
	// One dialect fills it in, with the same letter its `set -o` table
	// refuses under a name: measured, `set -t` there is `can't change
	// option: -t` at 1 and fatally, which is the wording, status and
	// fatality `set -o singlecommand` gets — and the letter is echoed back
	// rather than the name it abbreviates, which is why this is a table of
	// letters rather than a lookup through the name.
	//
	// Read before UnimplementedOptionLetters, because the two say different
	// things and only one of them can be true of a letter: "we have not built
	// it" is this implementation's confession, and this field is the shell's
	// own refusal. A letter in both would be the paired-table failure #1709
	// was.
	ImmovableOptionLetters map[string]string

	// SetInvalidOptionLetter is an option letter this shell does not have.
	// Two verbs: the letter as the script spelled it, sign and all, and the
	// letter on its own.
	//
	//	bash   set: -q: invalid option
	//	dash   set: Illegal option -q
	//	ksh93  set: -q: unknown option
	//	zsh    set: bad option: -q
	//
	// Two verbs rather than one because the sign splits the panel: `set +q`
	// is echoed back as `+q` by bash and ksh93 and as `-q` by dash and zsh,
	// which those two say by writing the `-` into the wording and taking the
	// bare letter. Measured 2026-09-05 on `-q`, `-j`, `-z` and `-A`, the
	// letters all six of bash 5.3, bash 3.2, bash-as-`sh`, dash, ksh93 and
	// zsh refuse. zsh's `set:` comes from the location, as everywhere else.
	//
	// A letter the dialect *has* and this shell has not implemented is a
	// different answer and belongs in UnimplementedOptionLetters under
	// "set", not here.
	SetInvalidOptionLetter string

	// SetInvalidOptionNameUsage repeats BuiltinUsage["set"] under a refused
	// `set -o` name, as well as under a refused letter.
	//
	// ksh93 alone. bash prints its `set` usage line after a bad *letter* —
	// which is what it does after any builtin's bad letter — and not after a
	// bad option name, whose complaint is a sentence of its own. dash and
	// zsh print no usage line for either. So the letter always gets one
	// where the dialect has one, and this says whether the name does too.
	SetInvalidOptionNameUsage bool

	// SetInvalidOptionStatus is what a refused `set` option reports —
	// either spelling. Zero means 2, which is three of the four; zsh
	// answers 1.
	//
	// One value for the letter and the name because the panel answers them
	// identically, measured 2026-09-05 on `-q`, `-j`, `-z` and `-A`, which
	// are the letters all six of bash 5.3, bash 3.2, bash-as-`sh`, dash,
	// ksh93 and zsh refuse: `set -q` reports exactly what
	// `set -o nosuchoption` reports in every one of them, and so does the
	// same letter given to the invocation. Two fields would be two names for
	// one measurement, and the one that was not being read would be the one
	// that drifted — which is what this replaced: the letter never asked at
	// all and the front end exited 2 for everybody (#483).
	SetInvalidOptionStatus int

	// MonitorDenied is `set -m` asked for by a shell the dialect says needs
	// a terminal for it (Semantics.MonitorNeedsATerminal), in the dialect's
	// words. One verb: the spelling the script used, `-m` or `monitor`,
	// which is the piece zsh echoes back.
	//
	//	dash   set: can't access tty; job control turned off
	//	zsh    set: can't change option: -m
	//
	// zsh's `set:` comes from the location, as everywhere else.
	MonitorDenied string

	// MonitorDeniedStatus is what that reports. Zero — dash's answer — means
	// the denial is a remark rather than a failure: the option is left off
	// and `set` still reports success. zsh answers 1, and fatally, the same
	// way it treats any other `set` refusal.
	MonitorDeniedStatus int

	// UnknownConditionOption is `[[ -o name ]]` given a name this shell does
	// not have, in the dialect's words, for the one dialect that says
	// anything (Semantics.UnknownConditionOptionIsAStatus). One verb: the
	// name exactly as the script wrote it, before any of the namespace's own
	// folding — measured, zsh echoes `Err_Exit` back with its capitals and
	// its underscore.
	//
	//	zsh    no such option: zzz
	//
	// zsh's location prefix comes from the location, as everywhere else.
	UnknownConditionOption string

	// UnknownConditionOptionStatus is what `[[ ]]` reports when that
	// happened and nothing later in the condition overrode it. zsh answers
	// 3, which is neither of the two a condition otherwise gives — that is
	// the whole of how the third value is visible from outside.
	UnknownConditionOptionStatus int

	// KilledCommandNotice is what a shell says when a signal ended a
	// command. Three verbs: the process id, the words for the signal, and
	// the command written back out.
	//
	// All three are used by one dialect and none by all of them, which is
	// the whole shape of this message:
	//
	//	bash   <shell>: line 2: 52505 Killed: 9                  /bin/sh -c 'kill -KILL $$'
	//	ksh93  <shell>: line 2: 52517: Killed
	//	dash   Killed: 9
	//
	// bash pads the words to a fixed twenty-seven columns and writes the
	// command straight after them — the padding is a constant and not the
	// width of the widest signal, which is why the same column appears on a
	// machine whose words are much shorter. dash prints the words alone,
	// which is what KilledCommandNoticeUnprefixed is for.
	KilledCommandNotice string

	// KilledCommandNoticeBareForTerminate replaces the notice when the
	// signal was SIGTERM, and is written with no location and no process id
	// — the words and the command alone. Two verbs: the words, the command.
	//
	// bash 5.3 alone, and for that one signal alone out of the nine this was
	// measured over. bash 3.2 writes the full prefix there, and no other
	// shell in the panel treats SIGTERM apart, so this looks like a
	// regression rather than a decision. Reproduced because the dialect is
	// bash 5.3; empty leaves the ordinary notice standing, which is what
	// every other dialect wants.
	KilledCommandNoticeBareForTerminate string

	// KilledCommandNoticeUnprefixed writes that notice with no location in
	// front of it. dash alone, and unlike every other message it prints:
	// this one carries neither the shell's name nor the line.
	KilledCommandNoticeUnprefixed bool

	// SignalDescriptions are this shell's own words for a signal, for the
	// ones it does not take from the machine.
	//
	// ksh93 alone. It says `Memory fault` where the host says `Segmentation
	// fault`, `Abort` where the host says `Abort trap`, and carries no
	// number after either — its table travels with the shell rather than
	// with the platform, so it is written here rather than read from the
	// host. A signal missing from it falls back to the host's words, which
	// is what the other two use for every signal.
	SignalDescriptions map[syscall.Signal]string

	// BackquotedSubstitutionRestartsLines counts a backquoted substitution's
	// body from line one rather than from where it was written.
	//
	// dash alone, and only for backticks — its `$( … )` is numbered from the
	// file like everyone else's, so this is not a shell that fails to track
	// the offset but one that keeps two different answers for the two
	// spellings of one construct:
	//
	//	                          bash  dash  ksh93  zsh
	//	x=$(nosuchcmd) on line 4     4     4      4    4
	//	x=`nosuchcmd`  on line 4     4     1      4    4
	//
	// Here rather than in Semantics because it is a question about where a
	// message says something happened, which is what Location and
	// RedirectFailureLine already are — and because a shell that refused to
	// answer it would have to refuse to report the line at all.
	BackquotedSubstitutionRestartsLines bool

	// JobStarted announces a backgrounded job. Two verbs: the job number and
	// the process id.
	//
	//	bash   [1] 13292
	//	ksh93  [1]\t12886
	//
	// The separator is the whole difference, and it is a tab in one of them.
	JobStarted string

	// JobNoticeShowsAmpersand puts the `&` back on the command of a job the
	// shell is reporting as finished.
	//
	// ksh93 alone, and not the same question as JobRunningShowsAmpersand:
	// that one is bash, in a listing, while the job runs. Neither shell does
	// both.
	JobNoticeShowsAmpersand bool

	// JobRunningShowsAmpersand puts the `&` back on the command of a job
	// that is still running.
	//
	// bash alone, and only while it runs: the same job listed after it ends
	// has no `&`. So it is part of rendering the line rather than part of
	// the text that was kept.
	JobRunningShowsAmpersand bool

	// DisownNoCurrentJob is a bare `disown` with nothing to let go of, as
	// one line with no verbs — the two shells that speak here agree on
	// nothing about its shape. Empty means silence with the failing status,
	// which is the third shell's answer and the substrate's.
	DisownNoCurrentJob string

	// NoSuchJob is a job spec that names nothing. Two verbs: the builtin and
	// the spec as written.
	//
	// Measured 2026-09-05 on `jobs %9`, which is the one of the three
	// builtins that reaches this in a script — `fg` and `bg` refuse for want
	// of job control first in bash and zsh:
	//
	//	bash (all three)  jobs: %9: no such job
	//	dash              jobs: No such job: %9
	//	ksh93u+           jobs: no such job — no spec at all
	//	zsh 5.9.2         jobs: %9: no such job
	//
	// So the shared default is bash's and zsh's, and the two that differ say
	// so. ksh93's format uses neither verb, which Wording allows.
	NoSuchJob string

	// NoSuchJobStatus is what that reports. Zero means 1, which is bash's
	// and ksh93's.
	//
	// Measured on the same probe, and it is the field that makes a slot
	// answerable without reading a listing: `jobs %2 >/dev/null 2>&1; echo
	// $?` is a yes/no about one slot with no text to race, and it grades
	// nothing at all while the number is wrong. dash reports 2 and zsh
	// reports 127 — the status of a command that is not there, which is what
	// zsh takes a job that is not there to be, and the same number its `wait`
	// uses for the same question.
	NoSuchJobStatus int

	PrintfUsage string
	// PrintfUsageUnprefixed prints it bare, as ksh93 prints every usage.
	PrintfUsageUnprefixed bool
	// PrintfUsageStatus is what that reports. Zero means 2.
	PrintfUsageStatus int

	// TrapBadSignal is what `trap` says about a condition that names no
	// signal it knows. One verb: the condition as written.
	//
	// The status is not a field beside it: all four report 1, which is the
	// only part of this any two of them agree on.
	TrapBadSignal string
	// TrapBadSignalUnprefixed prints that complaint with no location and no
	// shell name in front of it. dash alone, and only for `trap`: its `kill`
	// diagnostics carry the prefix like anyone's. bash does the same thing to
	// `trap`'s *usage* line and not to this one, which is why the flag is on
	// the message rather than on the builtin.
	TrapBadSignalUnprefixed bool

	// TrapBarePrintNeedsCondition is `trap -P` with nothing to print, taking
	// nothing. bash only, since bash is the only dialect with `-P`.
	TrapBarePrintNeedsCondition string
	// TrapPrintsSignalPrefix goes in front of a signal's name when printing
	// what is trapped: bash writes `trap -- : SIGINT` where the other three
	// write `INT`. Empty in three of the four, and never used for EXIT,
	// which is not a signal.
	// LocalOutsideAFunction is the refusal of `local` at the top level,
	// taking nothing: bash says it can only be used in a function and dash
	// says it is not in one.
	LocalOutsideAFunction string
	// TrapCouldNotParse follows the parse failure when a dialect reads a
	// trap's action as the trap is set, taking nothing. zsh only, since zsh
	// is the only dialect that reads it then.
	TrapCouldNotParse      string
	TrapPrintsSignalPrefix string
	// TrapConditionRequired is the refusal of `trap EXIT`, taking nothing.
	// ksh93 only, since ksh93 is the only dialect that refuses the form.
	TrapConditionRequired string

	// KillNoSuchProcess is a target that is not there. One verb: the pid.
	//
	// The four are worth reading together, because they are the same fact
	// four ways and only one of them names the process the way a script
	// could parse:
	//
	//	bash   kill: (999999) - No such process
	//	dash   kill: No such process
	//	ksh93  kill: 999999: no such process
	//	zsh    kill 999999 failed: no such process
	KillNoSuchProcess string
	// KillNotPermitted is a target that exists and is not ours. One verb.
	KillNotPermitted string
	// KillInvalidSignal is a name or number naming no signal, as `-s Q` or
	// `-l Q` spells it. One verb: the specification.
	KillInvalidSignal string
	// KillIllegalOption is that same failure spelled as a flag: `kill -Q`.
	//
	// Two verbs, positional: %[1]s is the specification as written and %[2]s
	// its first character alone. dash names only the character — `kill -99`
	// is "Illegal option -9" and `kill -SIGCONT` is "Illegal option -S" —
	// because it stopped reading at the first thing that was not an option.
	//
	// Two fields because two dialects answer "what did you just give me" by
	// where it appeared rather than by what it was — dash calls `-Q` an
	// illegal *option* and `-s Q` an invalid *signal*, and ksh93 an unknown
	// option against an unknown signal name. bash and zsh set both to the
	// same string, which is the same shape as the two `test` operator
	// wordings and for the same reason.
	KillIllegalOption string
	// KillNotAPid is an operand that is not a number. One verb: the operand.
	KillNotAPid string

	// KillNoSuchJob is a `%` spec that names no job — a different complaint
	// from a number that names no process, in every shell that speaks here.
	// One verb: the spec as written. (ksh93 dies on this one — a fault, not
	// a wording — which is deliberately not reproduced; it falls back to
	// the substrate's own line there.)
	KillNoSuchJob string
	// KillUsage is `kill` with nothing to signal. No verbs.
	KillUsage string
	// KillUsageUnprefixed prints that usage with no location and no shell
	// name in front of it. ksh93 alone.
	KillUsageUnprefixed bool
	// KillTargetUnprefixed does the same for a target that could not be
	// signaled. Also ksh93 alone, and the two are separate fields because
	// they are separate questions with the same answer only here: ksh93
	// prints `kill: 999999: no such process` bare and
	// `/bin/ksh: kill: abc: Arguments must be …` prefixed, so what decides
	// it is whether the complaint is about a target or about an argument.
	KillTargetUnprefixed bool
	// KillMissingSignalArgument is `-s` with nothing after it. One verb: the
	// option, since two dialects name it and two do not.
	KillMissingSignalArgument string
	// KillUnknownSignalHint is a second line after an unrecognized signal,
	// pointing at `kill -l`. zsh alone; empty means no second line.
	KillUnknownSignalHint string
	// KillUsageStatus is the status for `kill` with no operands: 2 in bash,
	// dash and ksh93, and 1 in zsh. Zero means the substrate's own, 2.
	KillUsageStatus int
	// KillBadOptionStatus is the status for an unknown or incomplete option.
	// bash and zsh report 1 where dash and ksh93 report 2 — ksh93 treating
	// an unknown option as a usage error, which is also why it prints its
	// usage after one. Zero means the substrate's own, 1.
	KillBadOptionStatus int
	// KillArgumentStatus is the status for an operand that is not a target
	// and a signal name that is not a signal. dash alone reports 2, and it
	// is the one dialect for which this is not the same question as the
	// option status. Zero means the substrate's own, 1.
	KillArgumentStatus int

	// FileNotFound is how this dialect spells the reason a file was not
	// there, when it does not quote the operating system's own text. No
	// verbs.
	//
	// dash alone: `cannot open b: No such file` where the C string is "No
	// such file or directory". It is a *reason* rather than a message for the
	// same purpose the arithmetic ones are — every message that quotes a
	// reason gets it, rather than each of them spelling it out.
	FileNotFound string

	// LowercaseReason lowercases the strerror text this dialect quotes.
	//
	// zsh alone: `permission denied` where the other three print the C
	// string's own `Permission denied`. It is a property of the shell rather
	// than of any one message, which is why it is a flag here instead of
	// being spelled out in every format that carries a reason.
	LowercaseReason bool

	// DotNotFound is what `.` says when the operand had no slash in it and
	// PATH did not have it — as opposed to a path that would not open.
	//
	// Same two verbs. Only dash sets it: it says ".: name: not found" for a
	// bare name and ".: cannot open path: …" for a path, where bash, ksh93 and
	// zsh use one message for both. Empty means "the same as DotCannotOpen".
	DotNotFound string
	// DotIsADirectory is what `.` says when the operand names a **directory**,
	// for a dialect that answers DotDirectoryOperandIsAnError yes and does not
	// reuse DotCannotOpen for it.
	//
	// Three verbs rather than two, because bash names the builtin here and
	// names it nowhere else on this builtin: measured 2026-09-08, `. ./` is
	// `.: ./: is a directory` and `source ./` is `source: ./: is a directory`,
	// where the same bash reports a path that is not there as plain
	// `./nope.sh: No such file or directory` through DotCannotOpen for both
	// spellings. So %[1]s is the operand as written, %[2]s the reason, and
	// %[3]s the builtin as it was invoked.
	//
	// Empty means "the same as DotCannotOpen", which is what ksh93 wants: it
	// says `.: ./: cannot open [Is a directory]`, the one sentence it uses for
	// every failure, with the reason filling the bracket. The two shells that
	// call a directory no error at all never reach this.
	DotIsADirectory string
	// DotCannotOpenStatus is the status that carries when the failure is not
	// fatal: bash says 1 and zsh says 127. dash and ksh93 end the script
	// instead, so this never speaks for them.
	//
	// Zero means the substrate's own, which is 1.
	DotCannotOpenStatus int

	// ParseFailureNamesItsOwnLine says the parse-failure wording already
	// carries the line, so the location must not carry it as well. ksh93
	// writes `syntax error at line 3` and would otherwise be prefixed into
	// `line 3: syntax error at line 3`.
	//
	// It is only the parse failure. A runtime diagnostic in a script is
	// prefixed there as usual — `line 2: nosuchcmd: not found` — which is why
	// this is not simply the script location being absent.
	ParseFailureNamesItsOwnLine bool

	// MissingFuncBodyOmitsTheLine drops the line from the location of a parse
	// failure where a function's body was expected and never began.
	//
	//	zsh -c 'f() ;'     zsh: parse error near `;'
	//	zsh -c 'f()'       zsh: parse error near `()'
	//	zsh -c 'if true'   zsh:1: parse error near `true'
	//
	// zsh alone, and only for that one failure: the last of the three is an
	// input that ran out too, so this is not "an end of input" and not the
	// kind of failure either. What it is, is `syntax.Error.FuncBody`, which
	// the parser sets where it knows — see there for the corner a newline
	// between the parens and the failure opens up.
	MissingFuncBodyOmitsTheLine bool
	// NotABuiltin is `builtin`'s refusal of a name that is not one. One verb:
	// %[1]s the name.
	NotABuiltin string
	// CommandStringParsedWhole reads all of a `-c` command before running any
	// of it. zsh alone does, so `sh -c 'echo one
	// { fi; }'` prints one everywhere else and nothing there. A *script* is
	// read a line at a time in all four, which is why this asks only about
	// the command string.
	CommandStringParsedWhole bool

	// StdinProgramSurvivesAParseFailure reports a line that did not parse and
	// then reads the next one, rather than ending the shell, when the program
	// itself arrived on standard input.
	//
	//	printf 'echo one\n{ fi; }\necho three\n' | sh
	//
	// prints one, the complaint, and *three* in zsh, and exits 0. bash, dash
	// and ksh93 stop at the complaint and exit with their parse status. It is
	// the route rather than the text that decides: the same three lines in a
	// file stop zsh too, and exit 1.
	//
	// The status is not forced. What ran last reports as it always would —
	// `false` at the end is 1 and `exit 7` is 7 — and where nothing runs after
	// the failure the shell is left with StatusForParseError, which is why the
	// program above ending at the bad line exits 1 while the one with a good
	// line after it exits 0. Two failures in one program are two complaints
	// and two recoveries.
	//
	// Beside CommandStringParsedWhole because it is the same kind of question
	// — how the front end reads a program, per route, for one dialect — and
	// this is the route that one does not cover.
	StdinProgramSurvivesAParseFailure bool
	// NamesTheInputInLocation puts *where the script came from* between the
	// shell's name and the line, for a parse failure only: bash writes
	// `bash: -c: line 1:` when it read the script from -c and plain
	// `bash: line 1:` for a runtime diagnostic on the same input. What the
	// input is called is the front end's to say — nothing here knows that a
	// shell has a -c at all — so this is only whether it is said.
	NamesTheInputInLocation bool
	// EchoesTheOffendingLine repeats the source line after a parse failure,
	// as bash's second line: "bash: -c: line 1: `{ fi; }'". Only after a token
	// the grammar did not want; an input that simply ran out gets no echo,
	// which EchoesLine decides.
	EchoesTheOffendingLine bool

	// ScriptNotFound is what a shell says when the script operand names
	// nothing at all. Two verbs, positional because the shells order them
	// differently and two do not use the second: %[1]s is the path as the
	// operand wrote it and %[2]s the reason.
	//
	//	bash   <shell>: nosuch.sh: No such file or directory
	//	dash   <shell>: 0: cannot open nosuch.sh: No such file
	//	ksh93  <shell>: nosuch.sh: not found
	//	zsh    <shell>: can't open input file: nosuch.sh
	//
	// Empty means the substrate's own, `%[1]s: %[2]s`.
	ScriptNotFound string

	// ScriptNotFoundStatus is what that reports. Zero means the substrate's
	// own, which is 127 — the number a command that is not there carries, and
	// what bash, ksh93 and zsh answer. dash alone says 2, treating an operand
	// it could not open as a usage error rather than as a program it could
	// not run.
	ScriptNotFoundStatus int

	// ScriptNotReadable is the same for a script that *is* there and would
	// not open: no read permission, or a path that is not a file at all.
	// Same two verbs.
	//
	//	bash   <shell>: unread.sh: Permission denied
	//	dash   <shell>: 0: cannot open unread.sh: Permission denied
	//	ksh93  <shell>: unread.sh: cannot open [Permission denied]
	//	zsh    <shell>: can't open input file: unread.sh
	//
	// Empty means "the same as ScriptNotFound", which is what dash and zsh
	// want — neither tells the two failures apart in words, and zsh does not
	// tell them apart in the status either.
	ScriptNotReadable string

	// ScriptNotReadableStatus is what that reports. Zero means the
	// substrate's own, which is 126 — the number a command that exists and
	// will not run carries, and what bash and ksh93 answer. zsh reports 127
	// for this as well as for a missing file, and dash 2 for both.
	//
	// Measured on a mode-000 file and on a directory, which are the two ways
	// a path that is there refuses to be read. A file whose *contents* are
	// not a script is a different question, and not this one.
	ScriptNotReadableStatus int

	// InvocationNamesTheUnreadLine writes a line number into a diagnostic
	// about the invocation itself — the script operand that would not open,
	// reported before any line has been read.
	//
	// dash alone: `<shell>: 0: cannot open …`, where bash, ksh93 and zsh
	// print their name and nothing else. The number is always nought, which
	// is what "no line yet" is in a shell that counts from one, so this is a
	// switch rather than a verb.
	InvocationNamesTheUnreadLine bool

	// InvocationUsage is the shell's own usage block, written under a `set`
	// option the invocation was refused. Two verbs: the name the shell was
	// invoked by, and that name's last path element.
	//
	// Two verbs because the panel's two shells that print one disagree about
	// which they write. Measured 2026-09-05 through a symbolic link named
	// `myksh`: bash spells the whole word it was invoked by — a path, when
	// that is what was typed — and ksh93 spells only the last element of it.
	//
	// Distinct from BuiltinUsage["set"], which is what the *builtin* prints:
	// at an invocation these two shells print their own usage instead, and
	// the block is a fact about the shell rather than about `set`.
	InvocationUsage string

	// InvocationNameRefusalNamesTheShell reports a refused `set -o` name at
	// an invocation exactly as the builtin would, with the shell's own name
	// standing where the builtin's would:
	//
	//	<shell>: line 0: <shell>: zzznosuch: invalid option name
	//
	// bash alone, and only for the long spelling — its refused *letter* is
	// its command-line parser speaking, with no location and no second name.
	// dash, ksh93 and zsh word both spellings the same way at an invocation:
	// the sentence with nothing naming `set`, after the plain invocation
	// prefix. Measured 2026-09-05.
	InvocationNameRefusalNamesTheShell bool

	// ScriptLocation is Location for a script read from a file, when the two
	// differ. ksh93 is the only shell in the panel where they do: `ksh -c`
	// names no location at all, and `ksh script` says "line 2". Zero means
	// "the same as Location", which is true of the other three.
	ScriptLocation LocationStyle

	// The wording of individual failures. Each is a format string, and empty
	// means the substrate's own — so a dialect states only where it differs,
	// the same way a semantics preset does.
	//
	// These are the failures the panel words differently *for the same
	// diagnosis*. Where a shell reaches a different diagnosis — dash calling
	// `[[ ( x ) ]]` "word unexpected (expecting \")\")" where we say the
	// paren is unexpected — no wording can close the gap, and none is
	// offered here.

	// UnmatchedQuote is a quote the input ran out inside — five verbs,
	// because each dialect names a different part of the same end of file:
	// %[1]s the opener as written, %[2]s the closer that never came,
	// %[3]s the text from the opener to the end of its line, %[4]d the
	// line the opener is on, and %[5]d the line the input ran out on.
	// Empty keeps the substrate's own sentence. The dialect that closes a
	// quote at end of input and runs never reaches this — that is a
	// grammar flag, not a wording.
	UnmatchedQuote string
	// UnmatchedBackquote is the same failure inside `` ` ``, for the one
	// dialect that words the old substitution differently from a quote.
	// Empty falls back to UnmatchedQuote.
	UnmatchedBackquote string
	// UnmatchedCmdSubst is `$(` the input ran out inside. Same verbs.
	UnmatchedCmdSubst string
	// UnmatchedProcSubst is `<(` or `>(` the input ran out inside. Same
	// verbs, and empty falls back to UnmatchedCmdSubst: the parentheses hold
	// a program either way, and three of the four shells that have the
	// construct say about it exactly what they say about `$(`. The fourth
	// reaches a different diagnosis rather than a different sentence — it
	// names the end of file where it names an unmatched parenthesis for
	// `$(` — which is why this is a wording of its own and not an argument
	// the other one takes.
	UnmatchedProcSubst string
	// UnmatchedBraceSubst is `${` the input ran out inside. Same verbs.
	UnmatchedBraceSubst string
	// UnmatchedNearMaxBytes cuts the quoted text — %[3]s above, the word a
	// construct ran out inside — to at most this many bytes, appending
	// `...` where it is that long or longer. Zero prints the whole of it,
	// which is what three of the four dialects want and is also right for
	// the two that never render the text at all.
	//
	// Measured on the one dialect that elides, 2026-09-07, bisected by
	// length: nineteen bytes come back whole, twenty come back with `...`
	// after them although nothing was cut, and everything longer is cut to
	// twenty and marked. So the ellipsis says "twenty or more" rather than
	// "something was removed", and a limit that appended it only when it
	// cut would be wrong on exactly the boundary case.
	//
	// **Bytes, and it cuts between them.** `v=$(echo héllo wörld aaaaaaa`
	// comes back as `v=$(echo hör…` — eighteen characters in twenty bytes
	// — and pushing a multibyte character across the boundary splits it:
	// measured, a word whose twentieth byte begins `é`, `€` or an emoji
	// answers with that character's lead byte alone — 0xC3, 0xE2 or 0xF0 — and the diagnostic is not
	// valid UTF-8. So this is a byte
	// slice and deliberately not a rune-aware one — rounding down to a
	// character boundary would be a nicer diagnostic than the shell's and
	// would not match it.
	UnmatchedNearMaxBytes int
	// UnmatchedArithSubst is `$((` or `$[` the input ran out inside. Same
	// verbs, and both spellings take it: everything about them but the
	// delimiters is one construct, and %[2]s carries which closer never
	// came — `)` for the first and `]` for the second, which is what the
	// dialect that echoes the closer prints for each.
	//
	// Empty falls back to the substrate's own sentence, deliberately not to
	// UnmatchedQuote. An opener with no wording of its own gets the quote's
	// sentence today, and for this one that would be a shell being made to
	// say something about a quote where no quote was written. Every preset
	// in the panel states an answer here, so the fallback is what the core
	// and a new dialect get rather than what any measured shell relies on.
	UnmatchedArithSubst string
	// UnmatchedReportedAtOpener puts an unmatched quote's diagnostic on
	// the line the opener is on rather than the line the input ran out on.
	UnmatchedReportedAtOpener bool
	// CmdSubstUnmatchedAtEnd reports an unmatched `$(` at the line after
	// the input's last, the way UnterminatedEndsOnNextLine does for an
	// open `if` — one dialect answers the two constructs differently,
	// which is why this is its own switch.
	CmdSubstUnmatchedAtEnd bool

	// NoclobberRefusal is the file `set -C` would not overwrite. Two
	// verbs: %[1]s the name as written, %[2]s the errno reason. Empty
	// where the dialect words it as any other failed create.
	NoclobberRefusal string
	// NoclobberRefusalCoversAFailedOpen words an open that failed for a
	// reason of its own as the option's refusal, where the name held
	// something that is not a regular file: a directory, a socket, a
	// dangling symlink, a `/dev/tty` in a session with no controlling
	// terminal. bash, ksh93 and dash report what the open said — `Is a
	// directory` — and zsh reports `file exists` for all of them.
	//
	// Reached only under `set -C`, and only after the exclusive create
	// came back EEXIST and the file turned out not to be regular: see
	// openThroughNoclobber, which is where the panel that settled this is
	// written down. A gate's refusal is never reworded here.
	NoclobberRefusalCoversAFailedOpen bool
	// NoclobberFallbackIsAnOpen words that same second open as an open
	// rather than as a create: dash says `cannot open d: Is a directory`
	// under `set -C` and `cannot create d: Is a directory` for the
	// identical redirection with the option off, where ksh93 says `cannot
	// create` both times. The two shells behave identically; only the verb
	// moves. Reached only on the fallback, so it says nothing about any
	// other failed open.
	NoclobberFallbackIsAnOpen bool

	// SyntaxError wraps a parse failure's own text. One verb: the text.
	SyntaxError string

	// EvalNaming and SourceFileNaming are where the name of borrowed text
	// goes in a diagnostic about it. Two fields because they are two
	// questions, and one shell answers them differently: bash puts a sourced
	// file's path where its own name goes — `./f.sh: line 3: …` — and labels
	// `eval` after its name instead, `bash: eval: line 2: …`.
	EvalNaming       SourceNaming
	SourceFileNaming SourceNaming
	// EvalSourceName is what `eval`'s text is called when it is named. Empty
	// means "eval", which is three of the four; zsh calls it `(eval)`.
	EvalSourceName string
	// SourceFileIsTheBuiltin names the builtin that read a file rather than
	// the file: ksh93 reports `.` where the other three report the path.
	SourceFileIsTheBuiltin bool
	// UnterminatedEndsOnNextLine puts the end of input on the line after the
	// text rather than on its last: `eval "if"` is line 2 in bash and line 1
	// in the other three.
	UnterminatedEndsOnNextLine bool

	// CondOperand is a token standing where a conditional operator wanted a
	// word — `[[ $k == (a|b) ]]` in a dialect with no bare pattern groups,
	// or `[[ -n ]]` with nothing after the operator at all. Four verbs:
	// %[1]s the offending token, %[2]s `unary` or `binary`, %[3]s the
	// operator that was waiting, and %[4]d the line.
	//
	// Empty means the dialect says what it says about any token the grammar
	// did not want, which is three of the four: `\`(' unexpected`, with
	// nothing about the condition. bash is the exception and words it as a
	// statement about the operator rather than about the token.
	CondOperand string

	// CondSyntaxUnexpected is a token the grammar did not want *inside*
	// `[[ ]]`, where the dialect words it differently from the same token
	// anywhere else. Three verbs, the same as SyntaxUnexpected: %[1]s the
	// token, %[2]s what would have been valid, %[3]d the line.
	//
	// Empty is "whatever it says about any such token", which is three of
	// the four: ksh93's `` `-z' unexpected `` and zsh's ``parse error near
	// `-z'`` are the sentences those shells give a stray token wherever it
	// stands. bash is the exception and drops the words `unexpected token`
	// here — `` syntax error near `-z' `` — which is measurable only
	// because it keeps them everywhere else.
	CondSyntaxUnexpected string

	// CondSyntaxPreamble is a line one dialect writes *before* that one,
	// naming the construct rather than the token: bash's `syntax error in
	// conditional expression: unexpected token `-z'`. Two verbs: %[1]s the
	// token and %[2]d the line — and the line is the `[[`'s, not the
	// token's, which is why it is a second verb rather than the location
	// the report already carries.
	//
	// Empty means no such line, which is every dialect but one.
	CondSyntaxPreamble string

	// CondUnterminatedPreamble is the same idea for a `[[` the input ran
	// out inside of, and the same dialect writes it: `unexpected EOF while
	// looking for `]]'`, again at the `[[`'s line and again in front of the
	// ordinary sentence. Two verbs: %[1]s the closer it was waiting for and
	// %[2]d the line.
	//
	// It is a *second* field rather than the one above because bash writes
	// this line for `[[` and for nothing else: `if`, `for`, `case`, `{` and
	// `(` left open all get one line and it is the ordinary one. Measured.
	CondUnterminatedPreamble string

	// AnonymousFunctionName is what a function with no name is called where
	// one is wanted — a frame, `$0`, a diagnostic. Empty means `(anon)`,
	// which is what the one dialect with the construct says.
	AnonymousFunctionName string

	// SyntaxUnexpected is a token the grammar did not want. Three verbs:
	// %[1]s the token, %[2]s what would have been valid where the parser
	// knows, and %[3]d the line, for the dialect that has no location of its
	// own to put it in.
	SyntaxUnexpected string
	// SyntaxUnexpectedWord is the same for an *ordinary* word, which one
	// dialect refuses to quote: `word unexpected` where a reserved word or an
	// operator is `"fi" unexpected`. Empty means "the same as
	// SyntaxUnexpected", which is three of the four.
	SyntaxUnexpectedWord string
	// SyntaxRedirectUnexpected replaces the message where the unexpected
	// token is itself a redirection operator. No verbs.
	//
	// dash alone: `cat < < x` is `redirection unexpected` there and names the
	// token in the other three. Empty means the dialect makes no distinction.
	SyntaxRedirectUnexpected string

	// SyntaxExpecting is appended when the parser knows what would have been
	// valid. One verb: that word. Empty means the dialect never says.
	SyntaxExpecting string

	// ForName is a `for` whose variable is not one. Two verbs: %[1]s the word
	// as written and %[2]d the line, for the dialect that carries its own.
	ForName string
	// ForNameStatus is what that reports, where it is not this dialect's
	// ordinary syntax-error status. Two of the four report 1 for it and
	// their syntax errors are 2 and 3 — so a refusal that is a parse failure
	// by every other measure carries a different number. Zero means the
	// syntax-error status.
	ForNameStatus int

	// Unterminated is input that ran out with a construct still open, and it
	// is four verbs because the panel names four different parts of that one
	// state rather than wording a shared diagnosis four ways:
	//
	//	%[1]s  the construct — `if`, `for`, `case`, `{`
	//	%[2]d  the line the construct began on
	//	%[3]s  the innermost unclosed keyword, `then` inside an `if`
	//	%[4]s  the word that would have closed it, `fi`
	//	%[5]s  the last token before the input ran out
	//	%[6]d  the line the failure is on, for a dialect that has no location
	//	       of its own to put it in
	//
	// bash names the first two, ksh93 the third, dash the fourth and zsh the
	// fifth. Empty means the substrate's own, which names the construct.
	Unterminated string
	// UnterminatedNoConstruct is the same state with *nothing* open to name:
	// `f()` given no body at all, where the parens have already closed.
	//
	// Its own wording because the dialect that names the construct has to say
	// something when there is no construct, and what it says is a shorter
	// sentence rather than the same one with a hole in it. The dialects whose
	// wording never mentioned a construct leave this empty and keep the
	// sentence they already have.
	UnterminatedNoConstruct string
	// BadSubstitution replaces a parse failure inside `${ }` entirely. No
	// verbs: no shell in the panel says which operator was wrong.
	BadSubstitution string
	// BadSubstitutionAtRun words the *deferred* report — an expansion the
	// grammar marked bad and the run then reached. One verb: the inside of
	// the braces as written. Only a dialect whose parse-time wording is not
	// a runtime one needs it: ksh93 refuses `${x ~}` while reading with a
	// syntax error and reports the `@` family it defers as
	// `${x@j}: bad substitution` when reached. Empty falls back to
	// BadSubstitution, which is a runtime wording everywhere else.
	BadSubstitutionAtRun string
	// BadSubstitutionNames is what the verb above is filled with. The
	// default names the expansion; two dialects name the *word* it sits in
	// and do not agree on how much of a word counts, which is why this is
	// three answers and not a flag.
	BadSubstitutionNames BadSubstitutionSubject
	// ExpansionFlagsError is a character a parenthesized expansion-flag
	// group could not carry, reported when the expansion is reached. Two
	// verbs: the 1-based position counted from the `$`, and the whole
	// `${…}` text. Only the dialect whose grammar has the group can reach
	// it, and that dialect's own wording is the fallback.
	ExpansionFlagsError string
	// NotFound is a command name that resolved to nothing. One verb: the
	// name.
	NotFound string
	// SetArrayNeedsAName is what `set -A` with nothing after it says, where
	// the dialect refuses it. One verb: the letter as written, `-A` or `+A`.
	//
	// Empty means the dialect answers a missing name with a *listing* of the
	// arrays it has — zsh, measured — which this engine does not build, so
	// the listing is named as missing instead. The two are not one refusal
	// with two wordings: one shell is telling the script it left an operand
	// out and the other is being asked a question it would have answered.
	SetArrayNeedsAName string

	// BadNameRefusalHidesTheBuiltin names the builtins whose bad-name
	// refusal does *not* carry the builtin in its location, in a dialect
	// that otherwise puts it there.
	//
	// zsh is the dialect and `set` is the builtin: `set -A 1v q` is
	// `<script>:1: not an identifier: 1v` where `unset 1x` and `typeset 1w`
	// from the same shell are `<script>:unset:1:` and `<script>:typeset:1:`.
	// The same shape SubscriptRefusalNamesBuiltin has, and a set for the
	// same reason — the sentence and the location are decided separately and
	// this builtin is where they part.
	BadNameRefusalHidesTheBuiltin map[string]bool

	// InconsistentType is what a declaration says when the plain word it was
	// given is assigned over a name whose cell is really holding an array or
	// a keyed table. One verb: the name.
	//
	// The builtin is named in the *location* rather than in the sentence,
	// which is where the one dialect with the refusal puts it:
	// `zsh:typeset:1: b: inconsistent type for assignment`, and
	// `f:readonly: b: …` from inside a function.
	//
	// Empty means the dialect does not refuse the line at all, which is what
	// ScalarOverACompoundIsAnInconsistentType answers — the wording exists
	// only for the dialect that says yes.
	InconsistentType string

	// ReadonlyVariableInDeclaration replaces it when the assignment was made
	// through a declaration utility. Two verbs: the name and the builtin.
	//
	// dash alone puts the builtin in front — `export: x: is read only` —
	// where its plain form says only the name. Empty leaves the wording
	// below standing for both, which is what the other three want here.
	ReadonlyVariableInDeclaration string
	// ReadonlyRefusalNamesBuiltin is which declaration builtins use that
	// wording. Empty means none, and the wording is then never reached.
	//
	// A set rather than a flag because one dialect answers it per builtin:
	// bash writes `declare: r: readonly variable` and `typeset: …` and
	// leaves the name out of `export: …` and `readonly: …`, which are the
	// two spellings POSIX has. dash names both of the two it has. ksh93 and
	// zsh name none, including ksh93's own `typeset`.
	ReadonlyRefusalNamesBuiltin map[string]bool
	// ReadonlyRemovalNamesBuiltin is the same set for a *plus* form refused
	// the attribute it asked to take off — `typeset +r x` on a frozen name.
	// Nil falls back to ReadonlyRefusalNamesBuiltin, which is what every
	// dialect but one wants.
	//
	// A set of its own because one shell answers the two shapes differently
	// through the identical word. Measured 2026-09-07:
	//
	//	ksh93  typeset x=2    → <script>: line 2: x: is read only
	//	       typeset +r x   → <script>[2]: typeset: x: is read only
	//
	// One word, two sentences and two locations, so the builtin's name
	// cannot decide it alone. The plus form there takes the shape `set -A`
	// takes — the builtin's own location with the builtin named — and the
	// assignment through the same word takes the plain line form. bash names
	// the builtin for both and needs no second answer.
	ReadonlyRemovalNamesBuiltin map[string]bool

	// DeclareNoSuchVariable is what `declare -p` and `typeset -p` say about
	// a name that is not there. One verb: the name. The message follows the
	// builtin's name, so the prefix rules place it — `declare: nosuch: not
	// found` against `no such variable: nosuch` behind a location that
	// already names the builtin. Reached only where
	// DeclarePrintReportsAMissingName said yes.
	DeclareNoSuchVariable string
	// IntegerBadBase is an output base the dialect will not spell — `typeset
	// -i64 a=100` and `typeset -i1 f=5`.
	//
	// One shell complains and leaves the name with nothing: `zsh:typeset:1:
	// invalid base (must be 2 to 36 inclusive): 64`, status 1, and the
	// script carries on. The other takes any base in silence and renders
	// plain what it cannot spell, which is what an empty entry means — see
	// Semantics.IntegerBaseDigits for what a dialect can spell.
	//
	// Two arguments: the builtin and the base as written.
	IntegerBadBase string

	// ReadonlyVariable is an assignment to a readonly name. One verb: the
	// name.
	ReadonlyVariable string
	// UnsetReadonly is `unset` refusing to remove a readonly name. One verb:
	// the name — the *base* name, since a subscripted operand is refused by
	// the variable it indexes rather than by the element.
	//
	// A field of its own rather than ReadonlyVariable's wording reused,
	// because only one dialect in the panel words the two the same way.
	// Three of the four name `unset` in the sentence, and one of those three
	// calls it a warning rather than an error; the fourth writes exactly what
	// it writes for an assignment. Empty falls back to the default below,
	// which is what the dialect the default was measured from wants.
	UnsetReadonly string
	// InvalidNumber is the reason given when arithmetic text is not a
	// number. No verbs: it is a reason, not a message — ArithError wraps it
	// with the expression and the offending token.
	InvalidNumber string
	// ArithErrorNamesThePrefix puts what the evaluator had consumed when
	// the token failed in the report's leading position — `08+1` is blamed
	// as `08`, `1+08` as `1+08` — rather than the whole expression.
	ArithErrorNamesThePrefix bool

	// FdVariableWithoutADescriptor is `exec {name}>&-` when the name holds
	// no descriptor number. One verb: the variable's name as written,
	// braces stripped.
	FdVariableWithoutADescriptor string

	// MultiDigitDuplicationTarget is `>&10` in the dialect that will not
	// take a duplication target wider than one digit — see
	// Semantics.MultiDigitDuplicationTargetIsAnError. One verb: the target as
	// it was written.
	//
	// The one shell that refuses names nothing at all and words it as a
	// syntax error, which is why the field exists rather than a shared
	// sentence with a hole in it: `Syntax error: Bad fd number`, said at the
	// line the redirection is on, with no number and no file in it.
	MultiDigitDuplicationTarget string

	// FdNumberOverLimit is a redirection whose descriptor number is at or
	// above the process's limit on open files, where the dialect refuses one
	// — see Semantics.FdNumberBoundedByOpenFileLimit. One verb: the number.
	//
	// Empty leaves the shape every other bad descriptor takes, `N: Bad file
	// descriptor`, which is what bash says here and is the shared wording
	// rather than a special case. ksh93 names the thing instead of the number
	// and quotes a different errno, so it says so.
	FdNumberOverLimit string

	// NoJobControl is `bg` or `fg` in a shell with none, for the dialects
	// that say so before anything else. One verb: the builtin's name.
	NoJobControl string

	// NoJobControlAtStartup is what an *interactive* shell says because it
	// wanted the monitor and had no terminal to run one on. No verbs.
	//
	// A different sentence from NoJobControl, which is a builtin refusing an
	// operand. This one is nobody asking: the shell decided for itself at
	// startup and is reporting what it could not have.
	//
	// Measured 2026-09-05 with no terminal on any of the three standard
	// streams, scratch HOME and scratch HISTFILE, on `-i script.sh`, `-i -c`
	// and `-i -s` alike — the same line on all three, and on none of the
	// non-interactive routes:
	//
	//	bash 5.3.15   bash: no job control in this shell
	//	bash 3.2.57   bash: no job control in this shell
	//	bash as `sh`  sh: no job control in this shell
	//	dash          <name>: 0: can't access tty; job control turned off
	//	ksh93u+       nothing — it runs the monitor without a terminal
	//	zsh 5.9.2     nothing
	//
	// Empty says nothing, which is two of the four and is the base's answer:
	// the standard does not have the shell remark on it.
	//
	// bash 5.3.15 writes a *second* line above this one — `bash: cannot set
	// terminal process group (11143): Inappropriate ioctl for device` — and
	// it is deliberately not reproduced. Three reasons, and the first is the
	// one that settles it: bash 3.2.57 and bash 3.2 run as `sh` do not write
	// it at all, so it is one version's extra line rather than bash's
	// wording. It is also that shell reporting the failure of an ioctl this
	// shell never makes, and there is nothing here whose failure it would be
	// describing. And it carries the shell's own pid, which no script can act
	// on and which no recording of would be the same twice.
	NoJobControlAtStartup string

	// NoJobControlAtStartupNamesTheScript puts `$0` in front of that remark
	// rather than the shell's own name. dash and only dash, of the two that
	// say anything: it writes `<script>: 0: can't access tty; job control
	// turned off` on `-i script.sh` and `/bin/dash: 0: …` on `-i -c` and
	// `-i -s`, which is `$0` on all three. bash writes its own name on all
	// three, `-i script.sh` included, and never the script's.
	//
	// A field rather than a reading of InvocationNamesTheUnreadLine, which
	// dash is also the only one to set. One shell answering two questions the
	// same way is not evidence that they are one question.
	NoJobControlAtStartupNamesTheScript bool

	// FcNoSuchEvent is `fc` with no history, in the dialect that reports
	// it. No verbs.
	FcNoSuchEvent string

	// OptionListingHeader opens `set -o`'s table where the dialect has one:
	// dash and ksh93 write "Current option settings" first.
	OptionListingHeader string
	// OptionListingWidth pads the name column: bash 15, dash 16, ksh93 25,
	// zsh 22. Zero means bash's.
	OptionListingWidth int
	// OptionListingTabbed puts a tab between the name and the state, which
	// bash alone does.
	OptionListingTabbed bool
	// PlusOListsActive makes `set +o` one line naming only what is on, the
	// --default form ksh93 writes, instead of re-inputtable per-option
	// lines.
	PlusOListsActive bool
	// KillListing is the shape of `kill -l` with no operands.
	KillListing KillListingForm

	// ReadArgCount is a bare `read` in the dialect that wants a name. No
	// verbs.
	ReadArgCount string

	// ArithInvalidBase refuses a base the dialect does not go up to. One
	// verb: the base as written.
	ArithInvalidBase string
	// ArithEmptyExpression is `$(( ))` in the dialect that wants a primary
	// there. No verbs.
	ArithEmptyExpression string

	// DigitTooGreatForBase is the reason when a literal carries a digit its
	// base does not allow, such as `08` read as octal. No verbs. It is not
	// InvalidNumber because it is a different diagnosis, and bash words the
	// two differently: `08` is "value too great for base" where a name-shaped
	// operand is an arithmetic syntax error.
	DigitTooGreatForBase string
	// NumericArgument is a builtin given an argument that is not a number,
	// such as `exit abc`. Two verbs, positional because the shells order them
	// differently: %[1]s is the builtin's name and %[2]s the argument.
	//
	// Separate from InvalidNumber, which this was folded into and should not
	// have been. They are two questions: dash answers this one "Illegal
	// number: abc" and words the arithmetic failure by an entirely different
	// route. One field cannot hold both without one dialect's answer to one
	// question being read as its answer to the other.
	NumericArgument string
	// ShiftTooMany is `shift` past the end. One verb: the count, as a
	// number — which a format is free to ignore, and dash's does.
	ShiftTooMany string
	// ShiftNegativeCount is the other end of the same range: a count below
	// zero, where ShiftTooMany is one above `$#`. Two verbs, the same pair
	// ShiftTooMany's format is handed — %[1]d the count as a number, %[2]s
	// the operand as written — because the panel picks different ones:
	// bash names the word, zsh names neither, and ksh93 reuses the wording
	// it gives a count that is too large.
	//
	// Empty means the dialect does not have this complaint, which is dash's
	// answer: a negative count is a word that is not a number there, and
	// ShiftBadNumber says so. ShiftNegativeIsOutOfRange is what decides
	// which of the two is reached.
	ShiftNegativeCount string
	// ArithFloatDigits is how many significant digits a float is written to.
	// ksh93 shows 15 and zsh 17, which is why `0.1+0.2` is 0.3 in one and
	// 0.30000000000000004 in the other from the same arithmetic. Zero means
	// 17, so a dialect that has floats and has not said is not silently
	// rounded.
	ArithFloatDigits int
	// ArithFloatKeepsPoint writes a whole float with a trailing point, so
	// `1.5+2.5` is `4.` rather than `4`. zsh alone does it, and it is what
	// keeps a float visible as one.
	ArithFloatKeepsPoint bool
	// ArithInfinity is how an infinity is written — `inf` in ksh93 and `Inf`
	// in zsh, with the sign in front of either. Empty leaves Go's own
	// spelling, which is neither.
	ArithInfinity string
	// ArithNotANumber is how a NaN is written: `nan` and `NaN` respectively.
	ArithNotANumber string
	// ArithInvalidFloatOperation is the refusal when a float reaches an
	// operator defined only on integers. One verb: %[1]s the operator.
	ArithInvalidFloatOperation string
	// ArithNegativeExponent is the refusal when `**` meets an exponent
	// below zero in a dialect whose answer is an error rather than a float.
	ArithNegativeExponent string

	// SelectPrompt is what `select` asks with when PS3 is unset. No verbs.
	// bash and ksh93 write `#? ` and zsh writes the same two characters the
	// other way round, which makes it a value rather than an axis.
	SelectPrompt string
	// ArithError wraps a failed arithmetic expansion. Three verbs, all
	// positional because the shells order them differently and not every
	// shell uses all three: %[1]s is the expression as written, %[2]s the
	// reason, and %[3]s the token the failure is attributed to.
	//
	// Only bash names a token. The other three formats simply do not mention
	// %[3]s, which costs nothing — an indexed format ignores arguments past
	// the highest index it uses.
	ArithError string
	// ArithOperandExpected is the reason when an expression wanted a value
	// and found something that could not be one: `$((%))`, `$((1+&2))`. One
	// verb, the text from the refused byte to the end of the expression,
	// which only the shells that name anything here use.
	//
	// It is a reason rather than a message for the same purpose the others
	// here are: ArithError wraps it, so a dialect states the reason once and
	// the shape once instead of repeating the shape in every reason.
	//
	// Empty falls back to ArithExpressionRanOut, which is what the two
	// dialects that word the two failures identically want.
	ArithOperandExpected string
	// ArithIllegalByte is the reason when the arithmetic reader met a byte
	// that is part of no token at all, at a position where the expression
	// could legally have stopped: `$((@))` and `$((1 @))` are
	// `illegal character: @` in the one shell that has this sentence.
	//
	// One verb, the refused byte — not the text from it to the end of the
	// expression, which is what ArithOperandExpected takes. The two sentences
	// name different things about the same failure, which is why this cannot
	// be a second wording of that field.
	//
	// Empty falls back to ArithOperandExpected, which is right for the three
	// dialects that have no such sentence — and they never reach it anyway,
	// because the kind is only produced where Dialect.ArithBytesRefusedOutright
	// names the byte.
	ArithIllegalByte string
	// ArithExpressionRanOut is the reason when an expression wanted a value
	// and reached the end of the text instead: `$((1+))`, `$((~))`. One verb,
	// the operator that was left wanting, which only the shell that names one
	// uses.
	//
	// A separate field from ArithOperandExpected because two of the panel
	// word the two apart — ksh93 has "more tokens expected" against
	// "arithmetic syntax error", and zsh "operand expected at end of string"
	// against "operand expected at `%'". One field had to pick one of them,
	// and picking the end-of-input wording made every found-a-token failure
	// claim the expression had run out. bash words both the same way, which
	// is why it went unnoticed: it is the column a conformance number is
	// usually read against.
	ArithExpressionRanOut string
	// ArithFailureStatus is the status a failed arithmetic expression carries.
	// bash reports 1, the status of a command that failed, because it finds
	// the failure while *expanding* rather than while parsing — we find it
	// earlier, so the difference has to be stated. Zero means the dialect's
	// general syntax-error status, which is what the other three want.
	ArithFailureStatus int
	// ArithBadOperator is the reason when text where an operator belonged
	// could not have been one — `1 @`. bash alone words it separately from an
	// operand standing in an operator's place; empty falls back to
	// ArithOperatorExpected, which is what the other three want.
	ArithBadOperator string
	// ArithCharacterMissing is the reason when the character-code operator
	// has nothing after it to take the code of: `$((##))`.
	//
	// Only the dialect with the operator can reach it, and it words the
	// failure as neither an operand nor an operator one — `character missing
	// after ##`, naming the two-character spelling whichever was written.
	ArithCharacterMissing string
	// ArithOperatorExpected is the reason when an expression has something
	// left over: `$((1 2))`. Same verb, and one shell puts it inside the
	// reason — "operator expected at `2'".
	ArithOperatorExpected string
	// DivisionByZero is the reason itself, which dash and ksh93 spell
	// differently. No verbs.
	DivisionByZero string
	// ArithEmptySubscript is the complaint about a subscript written with
	// nothing between the brackets where an expression reads it: `$(( a[] ))`,
	// which is what `$(( a[$w] ))` is once an empty `$w` has gone in. One
	// verb, the name — used by the dialect that names it and ignored by the
	// one whose sentence is about the subscript rather than about the name.
	//
	// Two of the three shells that reach it write something, and they write
	// different shapes: `m[]: bad array subscript` names the subscript back
	// and `invalid subscript` names nothing at all. Which of them a dialect
	// writes, and whether the expression survives it, is
	// Semantics.EmptyArithSubscript; this is only the wording.
	ArithEmptySubscript string

	// The math-function sentences: `functions -M` registers a shell function
	// under a name arithmetic can call, and one shell in the panel has the
	// facility, so the other five leave all seven of these empty. See
	// interp/mathfunc.go, where each was measured.
	//
	// The three at the call are *complete* sentences and are not wrapped by
	// ArithError — measured, `zsh:1: unknown function: nosuchmf` where an
	// ordinary failure in the same place is `zsh:1: bad math expression:
	// operand expected at end of string`. The four at the registration are a
	// builtin's and carry its name in the location the way every other
	// builtin complaint here does.

	// MathFunctionUnknown is a name arithmetic called that no registration
	// answers for. One verb: the name.
	MathFunctionUnknown string
	// MathFunctionArgumentCount is a call with too few or too many
	// arguments. One verb: the call *as written*, source text and all — so
	// `mf( 5 , 6 )` keeps its spaces.
	MathFunctionArgumentCount string
	// MathFunctionMissingImpl is a registration whose implementation is not
	// a function when the call arrives. One verb: the implementation's name,
	// not the registered one. Registration itself never checks, so this is
	// the only place a name nobody defined is reported.
	MathFunctionMissingImpl string
	// MathFunctionTooManyOperands is a registration with a fifth operand.
	// One verb: the builtin's name.
	MathFunctionTooManyOperands string
	// MathFunctionBadName is a registration whose first operand is not an
	// identifier. Two verbs: the builtin's name and the operand.
	MathFunctionBadName string
	// MathFunctionBadMinimum is a minimum that is not a count. Two verbs:
	// the builtin's name and the operand as written.
	MathFunctionBadMinimum string
	// MathFunctionBadMaximum is a maximum that is not a count, or one below
	// the minimum without being the -1 that means no bound. Two verbs, as
	// above.
	MathFunctionBadMaximum string

	// SubstringRangeError wraps a substring offset or length that would not
	// evaluate. Two verbs: the parameter as written — `x`, or `a[@]` when a
	// subscript was given — and the arithmetic sentence, already worded by
	// ArithError. Empty leaves the sentence to stand alone, which is what two
	// of the three shells with substrings do; bash alone puts the parameter in
	// front of it.
	//
	// A separate field from ArithError rather than a flag on it, because the
	// same shell wraps a subscript's failure without any such prefix: `${a[b
	// c]}` is blamed on `b c` and `${x:b c}` on `x: b c`. One field could not
	// say both.
	SubstringRangeError string

	// UnrecognizedModifier is the reason when a substring range read as a
	// modifier list names one the dialect does not have. One verb: the
	// segment as written.
	//
	// Only reachable where SubstringRangeReadsModifiers is yes, so only one
	// dialect fills it in.
	UnrecognizedModifier string
	// UnrecognizedModifierAlone is the same complaint with nothing named,
	// which is what the shell says when the segment *began* with a modifier
	// and has text left over — `${x:ha}`, where `h` is one and `a` following
	// it in the same segment is not. No verbs.
	//
	// A second field rather than an empty verb, because the two sentences do
	// not differ by a substitution: one ends in a quoted name and the other
	// ends.
	UnrecognizedModifierAlone string

	// SubstringErrorNamesTheWholeRange blames a failing offset together with
	// everything written after it: `${x:1+:2}` is `1+:2` rather than `1+`.
	// ksh93 alone, which reads `offset:length` as one string and reports from
	// the failing point to its end — so a failing *length* is named on its own
	// there, having nothing after it.
	SubstringErrorNamesTheWholeRange bool
	// EqualsNotFound is `=cmd` naming nothing. One verb: the name. zsh omits
	// the colon it uses everywhere else, which is why this is not NotFound.
	EqualsNotFound string
	// UnboundVariable is an unset parameter under `set -u`. One verb: the
	// name. bash calls it unbound where the other three call it not set.
	UnboundVariable string
	// UnboundPositional is the same failure for a parameter whose name is not
	// a variable name — `$1`, and `$!` before any background command. One
	// verb: the name, without its `$`. Empty means "the same as
	// UnboundVariable", which is true of three of the four — bash alone
	// writes the `$` back, saying `$1: unbound variable` and
	// `$!: unbound variable` where it says `NOPE: unbound variable` for a
	// name.
	//
	// Named for the positional because that is where it was found, and it
	// holds for `$!` too because bash's rule is about the *sigil* rather than
	// about the parameter: measured, `set -u; echo "[$!]"` says
	// `$!: unbound variable` in all three bash columns and dash says
	// `!: parameter not set`, which is exactly the pair this field and
	// UnboundVariable already carry.
	UnboundPositional string
	// AssignThroughExpansionBadName is an assignment written inside an
	// expansion whose parameter cannot be assigned to at all — `${#::=w}`,
	// where `#` is not a name, and `${@:=w}`, which every shell in the panel
	// refuses. Two verbs: the name without its `$`, and the expansion as it
	// was written, quoting run and all.
	//
	// Four wordings, measured 2026-09-11 with `set --`:
	//
	//	bash 5.3 / as-sh / 3.2   $@: cannot assign in this way
	//	dash                     @: bad variable name
	//	ksh93                    ${@:=abc}: bad substitution
	//	zsh 5.9.2                not an identifier: @
	//
	// bash writes the sigil back and dash does not; ksh93 names neither and
	// blames the whole expansion, which is the second verb's only reader —
	// and it names the *quoting run* rather than the braces alone, so
	// `x${@:=abc}y` and `"${@:=abc}"` are blamed whole.
	//
	// The status is not here. dash exits 2 where the other three exit 1, and
	// that is Semantics.FatalErrorStatusIsOne, which this failure already
	// goes through — a second number would be the same axis written twice.
	//
	// Its own field rather than a reuse of a builtin's bad-name wording
	// because the two are worded differently by the shell that has the
	// construct: `set -A 1v q` there is `not an identifier: 1v` with the
	// builtin hidden, and this is `not an identifier: #` from an expansion
	// that names no builtin to hide. The sentences coincide today and the
	// routes do not, and one field for both would tie a future change on
	// either route to the other.
	//
	// zsh's is the fallback because it is the one shell whose grammar has
	// `${name::=word}`, the operator that reaches this question on every
	// name. The conditional `${name:=word}` beside it is in every dialect,
	// which is why the other three are filled in (#1541).
	AssignThroughExpansionBadName string
	// BadPattern is a pattern the dialect rejects. One verb: the pattern.
	BadPattern string
	// CannotOpen is a redirection that could not be opened for reading. Two
	// verbs, positional because the shells order them differently: %[1]s is
	// the name as written and %[2]s the reason.
	//
	// All four word it differently, and only one of them puts a verb in it:
	// bash says `f: No such file or directory`, dash `cannot open f: No such
	// file`, ksh93 `f: cannot open [No such file or directory]` and zsh `no
	// such file or directory: f` — the reason first.
	CannotOpen string

	// CannotCreate is the same failure for a redirection that was making the
	// file rather than reading it. Same two verbs.
	//
	// A separate field because two of the four make the distinction: dash and
	// ksh93 say "create" where they said "open", and bash and zsh say the
	// same thing either way.
	CannotCreate string

	// RedirectionWithNoCommand is a command that is only redirections, in a
	// dialect that has the null-command hook and whose null-command
	// parameter is empty. No verbs.
	//
	// Reached only through Semantics.NullCommandVariable, so a dialect
	// without the hook never needs a wording: there a command that is only
	// redirections opens its files, runs nothing and succeeds. The one shell
	// that has it says `redirection with no command` and reports 1, and says
	// it for an unset parameter and an emptied one alike.
	RedirectionWithNoCommand string

	// RedirectFailureLine is which line a redirect that could not be opened
	// is reported at, when the redirect and the command it belongs to are on
	// different lines.
	//
	// Measured on `while read x` … `done < missing`, and again on a brace
	// group, and again on a command split by a backslash: three answers.
	// bash names the redirect's own line. dash and zsh name the line the
	// command began on. ksh93 names the line *before* the redirect's, in all
	// three shapes, which reads as an off-by-one of its own and is recorded
	// as what it does rather than as what it might mean.
	//
	// Zero is the substrate's own answer, which is the line the command
	// began on: that is where a statement is already reported, and a shell
	// told nothing does not go looking for a second position.
	RedirectFailureLine RedirectLine

	// RedirectFailureStatus is what a command whose redirect could not be
	// opened reports. Zero means the substrate's own, which is 1.
	//
	// dash alone says 2, for a read and a write alike and in a brace group
	// and a subshell alike. Not fatal there — the script carries on — which
	// is what makes this a different question from FatalErrorStatusIsOne.
	RedirectFailureStatus int

	// BuiltinWriteError is a builtin whose output write failed — into a
	// descriptor closed with `>&-`, most plainly. Two verbs, positional:
	// %[1]s is the builtin's name and %[2]s the reason.
	//
	//	bash   echo: write error: Bad file descriptor
	//	dash   echo: echo: I/O error
	//
	// dash opens with the name twice and fixes the reason as "I/O error"
	// whatever the errno was, so its format uses %[1]s in both places and
	// never mentions %[2]s. Empty means nothing is said, which is the other
	// two and the substrate's own: ksh93 fails silently, and zsh does not
	// fail at all — the status is the semantics axis's answer either way,
	// so silence here is a wording rather than a behavior.
	BuiltinWriteError string

	// DirectoryNotFound is FileNotFound for a write rather than a read.
	//
	// dash alone again, and a different string from its own FileNotFound:
	// `cannot create a/b: Directory nonexistent` where a failed read of the
	// same path is `cannot open a/b: No such file`. The OS says "No such file
	// or directory" for both.
	DirectoryNotFound string

	// TraceStyle and TraceQuoting are how `set -x` prints. They are here
	// rather than in Semantics because they decide what is *written*, not
	// what happens — the same argument the wording formats make.
	TraceStyle   TraceStyle
	TraceQuoting TraceQuoting
	// TraceForHeader is what a `for` loop prints at each iteration. Zero is
	// TraceForNone, which is dash's and ksh93's answer and the substrate's
	// own. `case` diverges the same way and is not reproduced: bash prints
	// `case $v in` once, zsh prints `case v (pattern)` once per pattern it
	// tries, and the corpus records the difference rather than claiming it.
	TraceForHeader TraceForHeader
	// TracePrefixRepeatsAtIndirection repeats the trace prefix's first
	// character once per level of indirection — an `eval`, a sourced file or
	// a command substitution the traced command is inside.
	//
	// bash alone, measured 2026-09-11: `set -x; eval :` traces `+ eval :`
	// then `++ :`, and `eval "eval :"` reaches `+++ :`. A function call and a
	// subshell add nothing, so the count is of text being read again rather
	// than of the stack. dash, ksh93 and zsh leave the prefix alone at every
	// depth.
	//
	// The *first character* rather than the whole prefix, which is what makes
	// it a rule about the prefix rather than about the plus sign:
	// `PS4='XY '` traces `XY eval :` and then `XXY :`.
	TracePrefixRepeatsAtIndirection bool

	// Location is how the shell prefixes a diagnostic with where it
	// happened. Measured, and all four differ:
	//
	//	dash    dash: 1: [[: not found
	//	bash    bash: line 1: [[: not found
	//	ksh93   ksh: [[: not found
	//	zsh     zsh:1: [[: not found
	//
	// Zero is LocationNone, the substrate's own — the shell's name and
	// nothing else, which is what this printed before any of it was a
	// dialect's answer.
	Location LocationStyle
	// BuiltinLocation is Location for a message a *builtin* is speaking,
	// in a dialect that names the place two ways in the one script.
	//
	// ksh93 is the dialect: `script[1]: cd: ...` from a builtin against
	// `script: line 1: nosuchcmd: not found` from everything else. The two
	// are the same question asked about different speakers rather than one
	// answer, which is why it is a second field and not a fifth style.
	//
	// LocationNone means the dialect names the place one way, which is every
	// other dialect. A dialect wanting a builtin to name no place at all has
	// never been measured and would need more than this field.
	BuiltinLocation LocationStyle
	// StdinLocation is Location for a script arriving on standard input,
	// where there is no $0 to name — zsh drops the line and keeps only its
	// name there. Zero means "the same as Location".
	StdinLocation LocationStyle
	// StdinBuiltinLocation is BuiltinLocation for the same route: zsh
	// drops the prefix down to the builtin's own name, and ksh93 moves to
	// `name[line]:` — a shape it uses nowhere else on this route.
	StdinBuiltinLocation LocationStyle

	// ScriptBuiltinLocation is BuiltinLocation for a script read from a
	// file, the way ScriptLocation is Location for one. ksh93 names line 1
	// in a file and not under `-c`, in both of its styles.
	ScriptBuiltinLocation LocationStyle
}

// LocationStyle is one shell's way of saying where a diagnostic happened.
//
// It is an enumeration rather than a format string because the shapes are a
// closed set that was measured, and a format string invites a fifth spelling
// that no shell actually uses.
type LocationStyle int

const (
	// LocationNone names the shell and stops: `ksh: msg`. Also the
	// substrate's own.
	LocationNone LocationStyle = iota
	// LocationColonLine is `dash: 1: msg`.
	LocationColonLine
	// LocationLineWord is `bash: line 1: msg`.
	LocationLineWord
	// LocationTightLine is `zsh:1: msg`.
	LocationTightLine
	// LocationLineWordAfterFirst is `ksh: line 2: msg`, and `ksh: msg` on
	// line 1 — the line is named only once there is a line worth naming.
	//
	// ksh93's answer for `-c`, and only for `-c`: a script file names line 1
	// like any other, which ScriptLocation already carries. Measuring only
	// `sh -c 'one-liner'` cannot tell this from LocationNone, and that is how
	// LocationNone got here.
	LocationLineWordAfterFirst
	// LocationBracketLine is `ksh: script[1]: msg` — the line in brackets,
	// against the shell's name rather than after it.
	//
	// ksh93 uses it for the messages a *builtin* speaks, and the word form
	// for everything else, which is what BuiltinLocation selects between.
	LocationBracketLine
	// LocationBracketLineAfterFirst is LocationBracketLine with the line
	// left out on line 1, the same way LocationLineWordAfterFirst leaves it
	// out — ksh93's answer for `-c`, where line 1 names no line at all.
	LocationBracketLineAfterFirst
	// LocationNameOnly is the shell's name and nothing more: `zsh: msg`,
	// with no line however deep in the input the failure was. Distinct from
	// LocationNone so the Stdin fields can choose it — their zero already
	// means "the same as Location".
	LocationNameOnly
	// LocationBuiltinNameOnly is the *builtin's* name alone: `shift: msg`,
	// no shell and no line — zsh's answer for a builtin's complaint when
	// the script arrived on standard input. A message the shell itself
	// speaks falls back to the shell's name.
	LocationBuiltinNameOnly
)

// BadOptionName is which part of a leading `-` word a bad-option complaint
// names. See Diagnostics.BadOptionNaming.
type BadOptionName int

const (
	// BadOptionFirstCharacter names the first letter the builtin cannot use,
	// dashes not skipped — so `--version` is `--`, and `read -rx` is `-x`
	// because `r` is an option it has. bash and dash, and the substrate's
	// own.
	BadOptionFirstCharacter BadOptionName = iota
	// BadOptionWholeWord names a `--` word as written: `--version`. A
	// single-dash bundle still names the letter — `read -rx` is `-x` here
	// too. ksh93.
	BadOptionWholeWord
	// BadOptionFirstUnknownLetter skips every leading dash and names the
	// first letter the builtin does not know: `--version` is `-v`, and `-e`
	// where `v` is an option it has. zsh.
	BadOptionFirstUnknownLetter
)

func (b BadOptionName) String() string {
	switch b {
	case BadOptionWholeWord:
		return "BadOptionWholeWord"
	case BadOptionFirstUnknownLetter:
		return "BadOptionFirstUnknownLetter"
	}
	return "BadOptionFirstCharacter"
}

// Wording renders one failure, using the dialect's format when it has one.
//
// The fallback is the substrate's own wording, so a dialect that says nothing
// about a failure still produces a sensible message rather than an empty one.
// Both formats must take the same verbs, which is what the field comments
// document.
func Wording(custom, fallback string, args ...any) string {
	if custom == "" {
		custom = fallback
	}
	if !strings.Contains(custom, "%") {
		// A format is allowed to ignore what it is given. dash's `shift`
		// message names no count where ksh93's does, and passing the count
		// to both is simpler than deciding per dialect which to pass —
		// provided the unused one does not become "%!(EXTRA int=5)", which
		// is exactly what it did.
		return custom
	}
	return fmt.Sprintf(custom, args...)
}

// escapeToken spells a token the way a diagnostic can print it: a newline is
// the one that arrives as itself and has to be written rather than obeyed.
func escapeToken(s string) string {
	switch s {
	case "\n":
		return `\n`
	case "":
		return s
	}
	return s
}

// SourceReport is a diagnostic about borrowed text — the text `eval` was
// given, or a file `.` read — with the source named the way this dialect
// names it.
//
// Three shapes, and the shells split three ways over one question: dash names
// the source after the location, bash and ksh93 before it, and zsh puts it
// where the shell's own name goes and prints no label.
func (d Diagnostics) SourceReport(naming SourceNaming, shell, source string, line int, msg string) string {
	switch naming {
	case SourceReplacesShell:
		return d.prefix(source, "", false, line) + msg
	case SourceBeforeLocation:
		if shell == "" {
			shell = "sh"
		}
		return shell + ": " + source + ": " + d.locationOnly(line) + msg
	}
	return d.prefix(shell, "", false, line) + source + ": " + msg
}

// SourceNaming is where the name of borrowed text goes.
type SourceNaming int

const (
	// SourceAfterLocation names it last: dash's `dash: 3: ./f.sh: …`. The
	// zero value, and the substrate's own.
	SourceAfterLocation SourceNaming = iota
	// SourceBeforeLocation names it between the shell and the line: bash's
	// `bash: eval: line 2: …`, and ksh93 for both kinds.
	SourceBeforeLocation
	// SourceReplacesShell names it *instead* of the shell: zsh's
	// `(eval):1: …`, and bash for a sourced file.
	SourceReplacesShell
)

// locationOnly is the location without a name in front of it, for the one
// shape that has already printed one.
func (d Diagnostics) locationOnly(line int) string {
	switch d.Location {
	case LocationColonLine:
		return fmt.Sprintf("%d: ", line)
	case LocationLineWord:
		return fmt.Sprintf("line %d: ", line)
	case LocationTightLine:
		return fmt.Sprintf("%d: ", line)
	}
	return ""
}

// ParseFailureLine is where a parse failure happened, or 0 if the error does
// not say.
//
// It sits beside ParseFailure and is exported for the same reason: the front
// end and the builtins that parse borrowed text must agree, and the line is as
// much a part of the dialect's answer as the wording. One shell puts an
// unterminated construct on the line *after* the input's last, so reading
// `Pos.Line` directly gets it wrong — which is what the front end did while
// `eval` and `.` got it right, the two paths disagreeing exactly as one shared
// front end exists to prevent.
func (d Diagnostics) ParseFailureLine(err error) int {
	var se *syntax.Error
	if !errors.As(err, &se) {
		return 0
	}
	if se.Kind == syntax.ErrUnmatched {
		// The three openers that hold a program, together: the dialect that
		// puts an unmatched `$(` at the line after the input's last puts
		// `<(` and `>(` there too, which is measured rather than assumed.
		if se.Token == "$(" || se.Token == "<(" || se.Token == ">(" {
			if d.CmdSubstUnmatchedAtEnd && se.EndLine > 0 {
				return se.EndLine
			}
		} else if d.UnmatchedReportedAtOpener {
			return se.Pos.Line
		}
		if se.EofLine > 0 {
			return se.EofLine
		}
		return se.Pos.Line
	}
	if d.UnterminatedEndsOnNextLine && se.EndLine > 0 {
		return se.EndLine
	}
	return se.Pos.Line
}

// arithParseFailure words an expression the parser refused, blaming expr.
//
// The text blamed is a parameter rather than the error's own, because it is
// not always the text that failed: one dialect names a substring's offset
// together with everything after it in the range, so `${x:1+:2}` is reported
// as `1+:2` where the parser was handed `1+`. Every other caller passes what
// the parser saw.
func (d Diagnostics) arithParseFailure(se *syntax.Error, expr string) string {
	reason, fallback := d.ArithOperandExpected, "operand expected"
	switch se.Kind {
	case syntax.ErrArithOperandEnd:
		// Empty is "the same wording as the other operand failure" rather
		// than "no wording", which is what keeps the two dialects that do not
		// distinguish the cases from having to write one sentence twice.
		reason, fallback = d.ArithExpressionRanOut, "operand expected"
		if reason == "" {
			reason = d.ArithOperandExpected
		}
		if readOn(se, expr) {
			// The dialect blames more text than the parser was handed, which
			// means it read more than the parser did — so what ran out for us
			// did not run out for it, and the wording is the other one.
			//
			// It is the substring range: one dialect reports `${x:1+:2}` as
			// `1+:2` where the parser saw `1+`, because it reads the offset
			// and the length as one string. Naming the whole range and
			// reading the whole range are the same fact about that shell, so
			// the blamed text is enough to know it and no second flag has to
			// be kept in step with the first.
			reason, fallback = d.ArithOperandExpected, "operand expected"
		}
	case syntax.ErrArithIllegalByte:
		reason, fallback = d.ArithIllegalByte, "illegal character"
		if reason == "" {
			reason = d.ArithOperandExpected
		}
	case syntax.ErrArithCharacterMissing:
		reason, fallback = d.ArithCharacterMissing, "character missing after ##"
	case syntax.ErrArithOperator:
		reason, fallback = d.ArithOperatorExpected, "operator expected"
	case syntax.ErrArithBadOperator:
		reason, fallback = d.ArithBadOperator, "operator expected"
		if reason == "" {
			// Only one dialect separates the two; for the rest the operator
			// wording covers both.
			reason = d.ArithOperatorExpected
		}
	}
	return Wording(d.ArithError, "%[1]s: %[2]s",
		expr, Wording(reason, fallback, se.Token), se.Token)
}

// readOn reports whether the dialect blaming expr read past what the parser
// was handed — the blamed text starts with the expression and continues.
func readOn(se *syntax.Error, expr string) bool {
	return len(expr) > len(se.Expr) && strings.HasPrefix(expr, se.Expr)
}

// ParseFailure words a parse error the way this dialect words it.
//
// It lives here rather than in the front end because the front end is not the
// only one reporting parse failures: `eval` and `.` parse borrowed text and
// have to say the same thing about the same failure. The kind is what
// decides, never the message text — dash says "Bad substitution" for anything
// wrong inside `${ }` and "Syntax error: …" for everything else, and matching
// on our own phrasing to tell those apart would break the first time the
// phrasing changed.
// unexpectedToken words a token the grammar did not want, which is one
// sentence shared by two failures: a token in the wrong place anywhere, and
// an operand a conditional operator could not take in a dialect with no
// sentence of its own for that.
func (d Diagnostics) unexpectedToken(se *syntax.Error) string {
	form := d.SyntaxUnexpected
	if se.Class == syntax.ClassWord && d.SyntaxUnexpectedWord != "" {
		form = d.SyntaxUnexpectedWord
	}
	if se.Redirect && d.SyntaxRedirectUnexpected != "" {
		// One dialect does not name the token here at all.
		return d.SyntaxRedirectUnexpected
	}
	if se.Construct == "[[" && d.CondSyntaxUnexpected != "" {
		// A token refused inside a condition, where this dialect says
		// something shorter than it says anywhere else.
		form = d.CondSyntaxUnexpected
	}
	msg := Wording(form, `"%[1]s" unexpected`, se.Token, se.Expected, se.Pos.Line)
	if se.Expected != "" && d.SyntaxExpecting != "" {
		msg += Wording(d.SyntaxExpecting, "", se.Expected)
	}
	return msg
}

// nearText is the quoted word an unmatched construct ran out inside, cut to
// the length this dialect prints.
//
// See UnmatchedNearMaxBytes for the measurement. The comparison is `>=` and
// not `>` because the shell appends its ellipsis at exactly the limit, with
// nothing removed, and the slice is by byte because the shell's is.
func (d Diagnostics) nearText(text string) string {
	if d.UnmatchedNearMaxBytes <= 0 || len(text) < d.UnmatchedNearMaxBytes {
		return text
	}
	return text[:d.UnmatchedNearMaxBytes] + "..."
}

func (d Diagnostics) ParseFailure(err error) string {
	var se *syntax.Error
	if !errors.As(err, &se) {
		return Wording(d.SyntaxError, "%s", parseMessage(err))
	}
	switch se.Kind {
	case syntax.ErrBadSubstitution:
		// Two verbs for the dialect that words this as a syntax error rather
		// than as a substitution that was bad: %[1]s the operator it could
		// not read, %[2]d the line.
		return Wording(d.BadSubstitution, se.Msg, se.Token, se.Pos.Line)
	case syntax.ErrArithOperand, syntax.ErrArithOperandEnd, syntax.ErrArithOperator,
		syntax.ErrArithBadOperator, syntax.ErrArithCharacterMissing,
		syntax.ErrArithIllegalByte:
		return d.arithParseFailure(se, se.Expr)
	case syntax.ErrForName:
		return Wording(d.ForName, "expected a name after `for`", se.Token, se.Pos.Line)
	case syntax.ErrCondOperand:
		if d.CondOperand != "" {
			return Wording(d.CondOperand, "", se.Token, se.Expected, se.LastToken, se.Pos.Line)
		}
		// A dialect with no sentence of its own names the token the way it
		// names any token the grammar did not want, which is what three of
		// the four do here: `\`(' unexpected` and nothing about `[[`.
		return d.unexpectedToken(se)
	case syntax.ErrUnexpected:
		return d.unexpectedToken(se)
	case syntax.ErrUnmatched:
		form := d.UnmatchedQuote
		switch se.Token {
		case "`":
			if d.UnmatchedBackquote != "" {
				form = d.UnmatchedBackquote
			}
		case "$(":
			form = d.UnmatchedCmdSubst
		case "<(", ">(":
			form = d.UnmatchedProcSubst
			if form == "" {
				form = d.UnmatchedCmdSubst
			}
		case "${":
			form = d.UnmatchedBraceSubst
		case "$((", "$[":
			// Assigned whatever the dialect says, empty included, because
			// empty here must *not* leave form as UnmatchedQuote — a shell
			// would then say something about a quote where the script wrote
			// none. What an empty format falls back to is Wording's job and
			// it is se.Msg, the substrate's own sentence, which is what this
			// opener wants. Restating that fallback here was dead code:
			// removing it was behaviorally identical under mutation, which
			// is how it was found.
			form = d.UnmatchedArithSubst
		}
		return Wording(form, se.Msg,
			se.Token, se.Expected, d.nearText(se.LastToken), se.Pos.Line, se.EofLine)
	case syntax.ErrUnterminated:
		form := d.Unterminated
		if se.Construct == "" && d.UnterminatedNoConstruct != "" {
			form = d.UnterminatedNoConstruct
		}
		return Wording(form, "syntax error: unterminated %[1]s",
			se.Construct, se.ConstructLine, se.Innermost, se.Expected,
			escapeToken(se.LastToken), se.Pos.Line)
	}
	return Wording(d.SyntaxError, "%s", se.Msg)
}

// Remark renders something the parser had to say about input it accepted
// anyway, or empty for a dialect that says nothing about it.
//
// Empty is the common answer: of the four, only one remarks on anything at
// parse time and only about one thing. Silence is expressed by having no
// wording rather than by the front end knowing which shells are quiet.
func (d Diagnostics) Remark(r syntax.Remark) string {
	if r.Kind != syntax.RemarkHeredocAtEOF || d.HereDocumentAtEOF == "" {
		return ""
	}
	return Wording(d.HereDocumentAtEOF, "", r.At.Line, r.Token)
}

// ForScript returns the diagnostics a script read from a file should use.
//
// A shell reports the *script's* name rather than its own once it is running
// one, and ksh93 also changes how it names the line. Both are properties of
// the invocation rather than of the dialect, which is why this returns a
// value instead of being another field somebody has to remember to set.
func (d Diagnostics) ForScript() Diagnostics {
	if d.ScriptLocation != LocationNone {
		d.Location = d.ScriptLocation
	}
	if d.ScriptBuiltinLocation != LocationNone {
		d.BuiltinLocation = d.ScriptBuiltinLocation
	}
	return d
}

// ForStdin returns the diagnostics a script arriving on standard input
// should use.
//
// There is no $0 to name on that route, and two of the four change shape
// rather than substituting a name: zsh trims its prefixes and ksh93 brackets
// the line. A property of the invocation, like ForScript.
func (d Diagnostics) ForStdin() Diagnostics {
	if d.StdinLocation != LocationNone {
		d.Location = d.StdinLocation
	}
	if d.StdinBuiltinLocation != LocationNone {
		d.BuiltinLocation = d.StdinBuiltinLocation
	}
	return d
}

// ScriptDiagnostic is what a shell prints when the script it was asked to run
// could not be read at all — the whole line, with the trailing newline on it.
//
// It is rendered here rather than in the front end for the reason every other
// wording is: which words a shell uses, and whether it writes a line it has
// not reached, are the dialect's answers. The front end owns only the fact
// that there was a script operand and that opening it failed.
//
// shell is what the shell calls itself — its own name, never the script's,
// which is measured: nothing has been read, so there is no `$0` yet.
func (d Diagnostics) ScriptDiagnostic(shell, path string, err error) string {
	format := d.ScriptNotFound
	if !errors.Is(err, fs.ErrNotExist) && d.ScriptNotReadable != "" {
		format = d.ScriptNotReadable
	}
	msg := Wording(format, "%[1]s: %[2]s", path, d.openReason(err, false))
	return d.invocationPrefix(shell) + msg + "\n"
}

// JobControlDiagnostic is the whole line an interactive shell writes because
// it wanted the monitor and had no terminal — with the trailing newline on it,
// and empty for the dialects that say nothing.
//
// Rendered here for the reason ScriptDiagnostic is: which words a shell uses,
// and whether it writes a line it has not reached, are the dialect's answers.
// The caller owns only the fact that there was no terminal.
//
// shell is what the shell calls itself, which on the script route is the
// script — measured, dash names the script here and names itself everywhere
// else, which is the same rule its other invocation diagnostics follow.
func (d Diagnostics) JobControlDiagnostic(shell string) string {
	if d.NoJobControlAtStartup == "" {
		return ""
	}
	return d.invocationPrefix(shell) + d.NoJobControlAtStartup + "\n"
}

// ScriptStatus is what a shell exits with when the script operand would not
// open.
//
// Two numbers rather than one, and the pair is the point: bash and ksh93
// answer 127 for a path that is not there and 126 for one that is there and
// will not open — a missing command's number against an unrunnable one's.
// zsh gives 127 to both and dash 2 to both, which they say by setting the two
// fields to one value rather than by this asking a different question of them.
func (d Diagnostics) ScriptStatus(err error) int {
	if errors.Is(err, fs.ErrNotExist) {
		if d.ScriptNotFoundStatus != 0 {
			return d.ScriptNotFoundStatus
		}
		return 127
	}
	if d.ScriptNotReadableStatus != 0 {
		return d.ScriptNotReadableStatus
	}
	return 126
}

// invocationPrefix is the start of a diagnostic about the invocation, where
// there is no line to name because nothing has been read.
//
// Three of the four print their name alone; the one that counts lines with a
// bare number writes the nought it has not left yet, which is what its own
// location style already spells.
func (d Diagnostics) invocationPrefix(shell string) string {
	if shell == "" {
		shell = "sh"
	}
	if d.InvocationNamesTheUnreadLine {
		return d.prefix(shell, "", false, 0)
	}
	return shell + ": "
}

// Report renders a complete diagnostic for a shell called name at line.
//
// Exported because the first thing a shell reports is usually a syntax error,
// and that happens before a Runner exists — whoever parsed the script has to
// render it with the same answers the Runner would have used.
func (d Diagnostics) Report(name string, line int, msg string) string {
	return d.prefix(name, "", false, line) + msg
}

// ReportFrom is Report for a parse failure, which one dialect prefixes with
// where the script came from as well as with the shell's name.
//
// input is the front end's label for it — "-c", and empty for a script file or
// for standard input, both of which that dialect leaves unnamed.
func (d Diagnostics) ReportFrom(name, input string, line int, msg string) string {
	if input != "" && d.NamesTheInputInLocation {
		name += ": " + input
	}
	return d.prefix(name, "", false, line) + msg
}

// ParseDiagnostic is everything a shell prints for a failed parse: the located
// message, and after it the echoed source line for the dialect that adds one.
//
// It is one call rather than a location and a message the caller joins,
// because the three decisions are not independent — the wording, whether the
// origin is named, and whether the line is echoed all turn on the same error —
// and they belong together here for the same reason the wording does: `eval`
// and `.` report the same failures and must say the same thing.
//
// name is the shell or the script; input is what the front end calls the
// origin, "-c" or empty; src is the whole script, for the echo.
func (d Diagnostics) ParseDiagnostic(name, input string, err error, src string) string {
	_, runtime := d.runtimeRefusal(err)
	if d.ParseFailureNamesItsOwnLine && !runtime {
		// The wording says where it was, so the location says only who.
		//
		// Only for a failure this dialect words as a parse failure. A
		// refusal it words as a *command's* — `for` with a bad name — has no
		// line in its sentence, so clearing the location left it with no
		// line at all: `<script>: 1x: invalid variable name` where the real
		// shell says `<script>: line 1: 1x: invalid variable name`. The flag
		// was read before anything asked which kind this was (#1076).
		d.Location = LocationNone
	}
	if d.MissingFuncBodyOmitsTheLine && missingFuncBody(err) {
		// One failure this dialect locates by name alone. Not LocationNone,
		// which is a different answer with the same rendering here and would
		// take the shell's name away from a dialect that prints one.
		d.Location = LocationNameOnly
	}
	line := d.ParseFailureLine(err)
	if line == 0 {
		// A failure that does not say where it was. Only the first line can
		// be pointed at honestly, and saying "line 0" would be worse.
		line = 1
	}
	if runtime {
		// Not a parse failure as far as this dialect is concerned, so it gets
		// the plain location and no echo.
		return d.Report(name, line, d.ParseFailure(err)+"\n")
	}
	out := d.condPreamble(name, input, err)
	out += d.ReportFrom(name, input, line, d.ParseFailure(err)+"\n")
	return out + d.echoLine(name, input, line, err, src)
}

// missingFuncBody reports whether err is a parse failure at the point where a
// function's body was expected and never began.
func missingFuncBody(err error) bool {
	var se *syntax.Error
	return errors.As(err, &se) && se.FuncBody
}

// condPreamble is the line one dialect writes in front of a token refused
// inside `[[ ]]`, or empty for the three that write none.
//
// It carries its own location, and that is the whole reason it is a line of
// its own rather than a longer wording: bash points it at the `[[` and points
// the line after it at the token, so a condition opened on line 1 and refused
// on line 2 names both.
func (d Diagnostics) condPreamble(name, input string, err error) string {
	var se *syntax.Error
	if !errors.As(err, &se) || se.Construct != "[[" {
		return ""
	}
	form, verb := d.CondSyntaxPreamble, se.Token
	if se.Kind == syntax.ErrUnterminated {
		form, verb = d.CondUnterminatedPreamble, se.Expected
	} else if se.Kind != syntax.ErrUnexpected {
		return ""
	}
	if form == "" {
		return ""
	}
	line := se.ConstructLine
	if line < 1 {
		line = 1
	}
	return d.ReportFrom(name, input, line, Wording(form, "", verb, line)+"\n")
}

// echoLine is the second line, or empty for none.
func (d Diagnostics) echoLine(name, input string, line int, err error, src string) string {
	text := d.offendingLine(line, err, src)
	if text == "" {
		return ""
	}
	return d.ReportFrom(name, input, line, text)
}

// SourceEcho is the echoed second line for text a builtin borrowed — `eval`'s
// string, or a sourced file — located the way that text names itself.
//
// It sits beside SourceReport for the reason ParseFailure and ParseDiagnostic
// do: the front end and the builtins that parse borrowed text have to say the
// same thing, and the echo is as much a part of the dialect's answer as the
// wording. Without it, `eval "case abc in @(abc|xyz)) echo m;; esac"` wrote
// the complaint about the token and not the line it came from — which is the
// only context a caller gets when the text was generated somewhere it cannot
// see (#1728).
//
// line is the failure's line *within src*, so a caller that could not work
// that out must not call this: indexing the borrowed text by the caller's own
// line would quote a line from somewhere else.
func (d Diagnostics) SourceEcho(naming SourceNaming, shell, source string, line int, err error, src string) string {
	text := d.offendingLine(line, err, src)
	if text == "" {
		return ""
	}
	return d.SourceReport(naming, shell, source, line, text)
}

// offendingLine is the quoted text of the echoed second line, without a
// location in front of it, or empty where there is none to write.
//
// Only a token the grammar did not want gets one: an input that simply ran out
// has no offending line to point at, and the shell that does this prints none
// for it. Nor does text nobody handed over — a prompt passes no source,
// because the line it would quote is still on the screen above the complaint,
// and splitting the empty string yields one empty line that came out as a
// bare "`'".
func (d Diagnostics) offendingLine(line int, err error, src string) string {
	if !d.EchoesTheOffendingLine || src == "" {
		return ""
	}
	var se *syntax.Error
	if !errors.As(err, &se) || se.Kind != syntax.ErrUnexpected {
		return ""
	}
	lines := strings.Split(src, "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	return "`" + lines[line-1] + "'\n"
}

// prefix renders the start of a diagnostic for a shell called name at line.
// prefixWithoutLine is the location with no line in it, which one dialect
// writes when a message comes from the first line of a function.
func (d Diagnostics) prefixWithoutLine(name, builtin string) string {
	if name == "" {
		name = "sh"
	}
	if builtin != "" && d.NamesBuiltinInLocation {
		name += ":" + builtin
	}
	if d.Location == LocationNone {
		return ""
	}
	return name + ": "
}

func (d Diagnostics) prefix(name, builtin string, byBuiltin bool, line int) string {
	if name == "" {
		name = "sh"
	}
	if byBuiltin && builtin != "" && d.BuiltinLocation == LocationBuiltinNameOnly {
		// The builtin speaks for itself: no shell, no line.
		return builtin + ": "
	}
	if builtin != "" && d.NamesBuiltinInLocation {
		// One dialect names the builtin that is speaking, between the shell
		// and the line. It rides on the shell's name rather than being a
		// fourth LocationStyle, because it composes with whichever style the
		// dialect already uses instead of replacing it.
		name += ":" + builtin
	}
	style := d.Location
	if byBuiltin && d.BuiltinLocation != LocationNone {
		style = d.BuiltinLocation
	}
	switch style {
	case LocationLineWordAfterFirst:
		if line <= 1 {
			return name + ": "
		}
		return fmt.Sprintf("%s: line %d: ", name, line)
	case LocationBracketLine:
		return fmt.Sprintf("%s[%d]: ", name, line)
	case LocationBracketLineAfterFirst:
		if line <= 1 {
			return name + ": "
		}
		return fmt.Sprintf("%s[%d]: ", name, line)
	case LocationColonLine:
		return fmt.Sprintf("%s: %d: ", name, line)
	case LocationLineWord:
		return fmt.Sprintf("%s: line %d: ", name, line)
	case LocationTightLine:
		return fmt.Sprintf("%s:%d: ", name, line)
	case LocationNameOnly, LocationBuiltinNameOnly:
		// The second reaches here for a message the shell itself speaks,
		// which keeps the shell's name the way the first always does.
		return name + ": "
	}
	return name + ": "
}

// SyntaxError reports the status a failed parse should carry.
// StatusForParseError is the status a particular parse failure reports, which
// is not always the dialect's general one.
func (d Diagnostics) StatusForParseError(err error) int {
	if st, ok := d.runtimeRefusal(err); ok {
		return st
	}
	return d.SyntaxStatus()
}

// runtimeRefusal reports whether the dialect refuses this at *run* time rather
// than while parsing, and with what status.
//
// `for 1x in a; do :; done` is the case: bash parses it and complains when it
// reaches it, so the complaint carries the status of a failed command and none
// of the decoration a parse failure gets — no naming of where the script came
// from, and no echoed source line. We find it while parsing, which is why the
// difference has to be said here rather than emerging from when it is noticed.
//
// The dialect having given the failure a status of its own is the signal, so
// there is one list and not two that could drift.
func (d Diagnostics) runtimeRefusal(err error) (int, bool) {
	var se *syntax.Error
	if !errors.As(err, &se) {
		return 0, false
	}
	switch se.Kind {
	case syntax.ErrForName:
		if d.ForNameStatus != 0 {
			return d.ForNameStatus, true
		}
	case syntax.ErrArithOperand, syntax.ErrArithOperandEnd, syntax.ErrArithOperator,
		syntax.ErrArithBadOperator, syntax.ErrArithCharacterMissing,
		syntax.ErrArithIllegalByte:
		// A malformed expression is found while expanding in bash, so the
		// command fails rather than the script failing to parse. The same
		// three consequences follow as for `for` with a bad name, which is
		// what makes this one predicate rather than three special cases.
		if d.ArithFailureStatus != 0 {
			return d.ArithFailureStatus, true
		}
	}
	return 0, false
}

func (d Diagnostics) SyntaxStatus() int {
	if d.SyntaxErrorStatus == 0 {
		return 2
	}
	return d.SyntaxErrorStatus
}

// sourcedSyntaxStatus is SyntaxStatus for text `.` read from a file, which is
// the same number in every dialect but zsh.
func (d Diagnostics) sourcedSyntaxStatus() int {
	if d.SourcedSyntaxErrorStatus == 0 {
		return d.SyntaxStatus()
	}
	return d.SourcedSyntaxErrorStatus
}

func (d Diagnostics) dotNoOperandStatus() int {
	if d.DotNoOperandStatus == 0 {
		return 2
	}
	return d.DotNoOperandStatus
}

// reasonText renders a strerror string the way this dialect quotes one.
//
// The substrate capitalizes, because that is what the C string says and what
// three of the four print. zsh lowercases everything, so it is one flag here
// rather than a lowercase spelling in every format that carries a reason.
func (d Diagnostics) reasonText(s string) string {
	if !d.LowercaseReason || s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// redirectFailureStatus is RedirectFailureStatus with the substrate's own
// answer for zero.
func (d Diagnostics) redirectFailureStatus() int {
	if d.RedirectFailureStatus == 0 {
		return 1
	}
	return d.RedirectFailureStatus
}

// timeDecimals is TimeDecimals with the substrate's own answer for zero.
func (d Diagnostics) timeDecimals() int {
	if d.TimeDecimals == 0 {
		return 3
	}
	return d.TimeDecimals
}

// timesDecimals is TimesDecimals with the substrate's own answer for zero.
func (d Diagnostics) timesDecimals() int {
	if d.TimesDecimals == 0 {
		return 3
	}
	return d.TimesDecimals
}

func (d Diagnostics) dotCannotOpenStatus() int {
	if d.DotCannotOpenStatus == 0 {
		return 1
	}
	return d.DotCannotOpenStatus
}

// PosixDiagnostics is dash's, which is also the substrate's own.
func PosixDiagnostics() Diagnostics { return Diagnostics{SyntaxErrorStatus: 2} }

// CoreDiagnostics is the substrate's own. Unlike [CoreSemantics] it refuses
// nothing: a status is not a claim about another shell.
func CoreDiagnostics() Diagnostics { return Diagnostics{} }

// diag reports the runner's diagnostics, defaulting to the substrate's own.
func (r *Runner) diag() Diagnostics {
	if r.Diagnostics != nil {
		return *r.Diagnostics
	}
	return CoreDiagnostics()
}

// RedirectLine is which line a failed redirect is reported at.
type RedirectLine uint8

const (
	// LineOfCommand is the line the command began on, which is where every
	// other diagnostic about it is reported. dash and zsh, always.
	//
	// It is also what every dialect does for a *simple* command, so this axis
	// only ever answers about a compound one. That took a command split by
	// backslash continuations to see: with `cat` on line 2 and its
	// `< missing` two lines below, all four name line 2, and reading the
	// redirect's own position there gave bash line 4.
	LineOfCommand RedirectLine = iota
	// LineOfRedirect is the redirect's own line, and applies to a compound
	// command: bash, whose `done < missing` on line 5 says 5 where the loop
	// opened on line 2.
	LineOfRedirect
	// LineBeforeRedirect is the line before that, and also applies to a
	// compound command: ksh93, which says 4 for that same loop — and says
	// line 1 for a compound written entirely on line 2, which prints no line
	// at all there.
	LineBeforeRedirect
)

// KillListingForm is the shape of `kill -l` with no operands.
type KillListingForm int

const (
	// KillListingPerLine is one name per line — ksh93's shape.
	KillListingPerLine KillListingForm = iota
	// KillListingNumbered is bash's: ` N) SIGNAME`, tab-separated, five to
	// a row.
	KillListingNumbered
	// KillListingSpaceJoined is zsh's single space-joined line.
	KillListingSpaceJoined
	// KillListingZeroFirst is dash's: a 0, then one name per line.
	KillListingZeroFirst
)
