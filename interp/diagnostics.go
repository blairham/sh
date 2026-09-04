// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
	"fmt"
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
// It lives here, next to [BashSemantics] and the rest, rather than in the
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
type Diagnostics struct {
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
	// CdHomeNotSet is `cd` with no operand and no HOME. One verb: the name.
	// Only the two dialects that treat it as an error say anything.
	CdHomeNotSet string
	// CdOldpwdNotSet is `cd -` with no OLDPWD. Same.
	CdOldpwdNotSet string

	// PrintfBadNumber is a numeric conversion given something that is not a
	// number. One verb: the operand.
	PrintfBadNumber string
	// PrintfBadNumberStatus is what that reports. Zero means 1.
	PrintfBadNumberStatus int
	// PrintfBadVerb is a conversion this shell does not have. One verb, and
	// the panel does not agree on what to put in it: half name the character
	// *after* the one they could not read and half name the conversion.
	PrintfBadVerb string
	// PrintfBadVerbStatus is what that reports. Zero means 1.
	PrintfBadVerbStatus int
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

	// WaitNotOurChild is a number that is a plausible process id and is not
	// one of this shell's children, taking the number. Empty means nothing
	// is said, which is two of the four — the status is 127 in all of them
	// either way, so silence here is a wording rather than a behavior.
	WaitNotOurChild string

	// WaitOptionLetters are the option letters this dialect's `wait` has and
	// this shell does not implement — bash's `n`, `f` and `p`. They are said
	// to be missing rather than refused as unknown, because refusing one the
	// shell really has is a different and worse answer than not having it
	// yet.
	WaitOptionLetters string

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
	//	bash, dash   --            the character after the first dash
	//	ksh93        --version     the whole word as written
	//	zsh          -v            the first letter it does not know, with
	//	                           every leading dash skipped first
	//
	// zsh's is the one that needs saying twice: `unset --version` is `-e`
	// there, not `-v`, because `v` is one of unset's own options and is
	// consumed before an unknown letter is reached. That is the tell that it
	// walks the bundle rather than naming a piece of the word.
	BadOptionNaming BadOptionName

	BuiltinBadOptionStatus int

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

	// TypeNotFoundStatus is what `type` reports when a name was not
	// accounted for. Zero means 1, which is three of the four; dash answers
	// with a missing command's 127.
	TypeNotFoundStatus int

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

	// UnsetBadFunctionName is what `unset -f` says about an operand that
	// could not be a function name. One verb: the operand.
	UnsetBadFunctionName string

	// UnsetFunctionNotFound is what `unset -f` says about a name no function
	// has. One verb: the name. zsh words it about its table rather than
	// about the function, and puts the builtin in the location as it does
	// with every message.
	UnsetFunctionNotFound string

	// UnsetParameterStatusFromCommandString is what a shell exits with when
	// a parameter could not be expanded *and the program came from an
	// argument* — `-c` — rather than from a file or standard input.
	//
	// bash alone, and only there: 127 given with `-c`, 1 from a file or from
	// standard input, for `set -u` and `${x?}` alike. Measured over eight
	// other ways it stops, none of which differs by invocation. Zero leaves
	// the dialect's ordinary fatal status standing either way.
	UnsetParameterStatusFromCommandString int

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

	// SetInvalidOptionNameStatus is what that reports. Zero means 2, which
	// is three of the four; zsh answers 1.
	SetInvalidOptionNameStatus int

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

	// NoSuchJob is a job spec that names nothing. Two verbs: the builtin and
	// the spec as written.
	NoSuchJob string

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
	// NotABuiltin is `builtin`'s refusal of a name that is not one. One verb:
	// %[1]s the name.
	NotABuiltin string
	// CommandStringParsedWhole reads all of a `-c` command before running any
	// of it. zsh alone does, so `sh -c 'echo one
	// { fi; }'` prints one everywhere else and nothing there. A *script* is
	// read a line at a time in all four, which is why this asks only about
	// the command string.
	CommandStringParsedWhole bool
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
	// BadSubstitution replaces a parse failure inside `${ }` entirely. No
	// verbs: no shell in the panel says which operator was wrong.
	BadSubstitution string
	// NotFound is a command name that resolved to nothing. One verb: the
	// name.
	NotFound string
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

	// ReadonlyVariable is an assignment to a readonly name. One verb: the
	// name.
	ReadonlyVariable string
	// InvalidNumber is the reason given when arithmetic text is not a
	// number. No verbs: it is a reason, not a message — ArithError wraps it
	// with the expression and the offending token.
	InvalidNumber string
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
	// ArithOperandExpected is the reason when an expression needed a value
	// and found none: `$((1+))`. One verb, the offending token, which only
	// the shell that names one uses.
	//
	// It is a reason rather than a message for the same purpose the others
	// here are: ArithError wraps it, so a dialect states the reason once and
	// the shape once instead of repeating the shape in every reason.
	ArithOperandExpected string
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
	// ArithOperatorExpected is the reason when an expression has something
	// left over: `$((1 2))`. Same verb, and one shell puts it inside the
	// reason — "operator expected at `2'".
	ArithOperatorExpected string
	// DivisionByZero is the reason itself, which dash and ksh93 spell
	// differently. No verbs.
	DivisionByZero string
	// EqualsNotFound is `=cmd` naming nothing. One verb: the name. zsh omits
	// the colon it uses everywhere else, which is why this is not NotFound.
	EqualsNotFound string
	// UnboundVariable is an unset parameter under `set -u`. One verb: the
	// name. bash calls it unbound where the other three call it not set.
	UnboundVariable string
	// UnboundPositional is the same failure for `$1` rather than `$NAME`.
	// One verb: the number, without its `$`. Empty means "the same as
	// UnboundVariable", which is true of three of the four — bash alone
	// writes the `$` back, saying `$1: unbound variable` where it says
	// `NOPE: unbound variable` for a name.
	UnboundPositional string
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
)

// BadOptionName is which part of a leading `-` word a bad-option complaint
// names. See Diagnostics.BadOptionNaming.
type BadOptionName int

const (
	// BadOptionFirstCharacter names the character after the first dash, so
	// `--version` is `--`. bash and dash, and the substrate's own.
	BadOptionFirstCharacter BadOptionName = iota
	// BadOptionWholeWord names the word as written: `--version`. ksh93.
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
	if d.UnterminatedEndsOnNextLine && se.EndLine > 0 {
		return se.EndLine
	}
	return se.Pos.Line
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
	case syntax.ErrArithOperand, syntax.ErrArithOperator, syntax.ErrArithBadOperator:
		reason, fallback := d.ArithOperandExpected, "operand expected"
		switch se.Kind {
		case syntax.ErrArithOperator:
			reason, fallback = d.ArithOperatorExpected, "operator expected"
		case syntax.ErrArithBadOperator:
			reason, fallback = d.ArithBadOperator, "operator expected"
			if reason == "" {
				// Only one dialect separates the two; for the rest the
				// operator wording covers both.
				reason = d.ArithOperatorExpected
			}
		}
		return Wording(d.ArithError, "%[1]s: %[2]s",
			se.Expr, Wording(reason, fallback, se.Token), se.Token)
	case syntax.ErrForName:
		return Wording(d.ForName, "expected a name after `for`", se.Token, se.Pos.Line)
	case syntax.ErrUnexpected:
		form := d.SyntaxUnexpected
		if se.Class == syntax.ClassWord && d.SyntaxUnexpectedWord != "" {
			form = d.SyntaxUnexpectedWord
		}
		if se.Redirect && d.SyntaxRedirectUnexpected != "" {
			// One dialect does not name the token here at all.
			return d.SyntaxRedirectUnexpected
		}
		msg := Wording(form, `"%[1]s" unexpected`, se.Token, se.Expected, se.Pos.Line)
		if se.Expected != "" && d.SyntaxExpecting != "" {
			msg += Wording(d.SyntaxExpecting, "", se.Expected)
		}
		return msg
	case syntax.ErrUnterminated:
		return Wording(d.Unterminated, "syntax error: unterminated %[1]s",
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
	if d.ParseFailureNamesItsOwnLine {
		// The wording says where it was, so the location says only who.
		d.Location = LocationNone
	}
	line := d.ParseFailureLine(err)
	if line == 0 {
		// A failure that does not say where it was. Only the first line can
		// be pointed at honestly, and saying "line 0" would be worse.
		line = 1
	}
	if _, runtime := d.runtimeRefusal(err); runtime {
		// Not a parse failure as far as this dialect is concerned, so it gets
		// the plain location and no echo.
		return d.Report(name, line, d.ParseFailure(err)+"\n")
	}
	out := d.ReportFrom(name, input, line, d.ParseFailure(err)+"\n")
	return out + d.echoLine(name, input, line, err, src)
}

// echoLine is the second line, or empty for none.
//
// Only a token the grammar did not want gets one: an input that simply ran out
// has no offending line to point at, and the shell that does this prints none
// for it.
func (d Diagnostics) echoLine(name, input string, line int, err error, src string) string {
	if !d.EchoesTheOffendingLine {
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
	return d.ReportFrom(name, input, line, "`"+lines[line-1]+"'\n")
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
	case syntax.ErrArithOperand, syntax.ErrArithOperator, syntax.ErrArithBadOperator:
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
