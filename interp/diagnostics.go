// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"
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
	// BadSubstitution replaces a parse failure inside `${ }` entirely. No
	// verbs: no shell in the panel says which operator was wrong.
	BadSubstitution string
	// NotFound is a command name that resolved to nothing. One verb: the
	// name.
	NotFound string
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
	// ArithError wraps a failed arithmetic expansion. Three verbs, all
	// positional because the shells order them differently and not every
	// shell uses all three: %[1]s is the expression as written, %[2]s the
	// reason, and %[3]s the token the failure is attributed to.
	//
	// Only bash names a token. The other three formats simply do not mention
	// %[3]s, which costs nothing — an indexed format ignores arguments past
	// the highest index it uses.
	ArithError string
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
	// CannotOpen is a redirection that could not be opened. Two verbs: the
	// name and the reason.
	CannotOpen string

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
)

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
	return d
}

// Report renders a complete diagnostic for a shell called name at line.
//
// Exported because the first thing a shell reports is usually a syntax error,
// and that happens before a Runner exists — whoever parsed the script has to
// render it with the same answers the Runner would have used.
func (d Diagnostics) Report(name string, line int, msg string) string {
	return d.prefix(name, line) + msg
}

// prefix renders the start of a diagnostic for a shell called name at line.
func (d Diagnostics) prefix(name string, line int) string {
	if name == "" {
		name = "sh"
	}
	switch d.Location {
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
