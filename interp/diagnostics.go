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
	// InvalidNumber is arithmetic text that is not a number. One verb: the
	// text.
	InvalidNumber string
	// ShiftTooMany is `shift` past the end. One verb: the count, as a
	// number — which a format is free to ignore, and dash's does.
	ShiftTooMany string
	// ArithError wraps a failed arithmetic expansion. Two verbs, and the
	// shells order them differently, so both are positional: %[1]s is the
	// expression as written and %[2]s the reason.
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
