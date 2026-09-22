// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// The line editor, reached from inside a command.
//
// Every other seam between a shell and its editor runs the other way — the
// editor calls the shell to run a widget, to draw a prompt, to highlight a
// line. This is the one that runs this way round, and it exists because one
// builtin genuinely needs it: a command that hands a person a line to edit and
// keeps what they accepted.
//
// It is a hook for [Runner.ReplaceProcess]'s reason. This package is a library
// and a Runner embedded in another program has no editor and no terminal; a
// front end that *is* a shell says so by filling this in, and what taking the
// terminal back into raw mode means is that front end's business rather than
// this one's. Nil is the ordinary case and is a shell with no editor to
// re-enter — a script, a `-c` line, a hook, an embedder.

// LineEdit is one read of the line editor a command asked for.
//
// A struct rather than a pair of strings because what a caller has to say
// about the read is open-ended and every field of it is measured: which of
// them are set decides what the keys do, not only what is drawn.
type LineEdit struct {
	// Prompt is drawn in front of the line, and empty draws nothing at all —
	// which is the ordinary case and is not the same as a prompt of a space.
	Prompt string

	// Initial is the text the line starts from, with the cursor after it.
	// This is the whole difference from the read at a prompt, where the line
	// starts empty.
	Initial string

	// History makes the session's own history reachable from the line. False
	// — the ordinary case — is a line the history keys do nothing on.
	History bool

	// EndOnEndOfInput makes an end-of-input keystroke on an empty line end the
	// read, reported as [LineEditEndOfInput]. False leaves that key doing
	// whatever else it does, which is what makes the read end only on a
	// newline or an interrupt.
	EndOnEndOfInput bool
}

// LineEditEnd is how a read of the line editor finished.
type LineEditEnd int

const (
	// LineEditAccepted is a line the person accepted. It is the only ending
	// that carries text.
	LineEditAccepted LineEditEnd = iota

	// LineEditInterrupted is the line abandoned. What a caller owes the
	// interrupt — a status, unwinding the rest of the line the command was on
	// — is the caller's, since only it knows what it was doing.
	LineEditInterrupted

	// LineEditEndOfInput is end of input on an empty line, which reaches a
	// caller only where [LineEdit.EndOnEndOfInput] asked for it.
	LineEditEndOfInput

	// LineEditUnavailable is an editor that could not be entered — the
	// terminal would not go into raw mode, or the read failed. Distinct from
	// an interrupt because nothing was abandoned: there was never a line.
	LineEditUnavailable
)

// SplitOnIFS splits plain text into fields the way word splitting does.
//
// Exported because a builtin outside this package can be handed a line of text
// that has to become a list — and the rule is not a `strings.Split`: a run of
// IFS whitespace is one delimiter while each non-whitespace separator delimits
// on its own, and `IFS=` set to nothing does not split at all. A dialect
// writing its own would be a second implementation of the spec this one
// implements.
//
// The text is plain rather than in the escaped form an expansion result is
// carried as, which is what a line a person typed is.
func (r *Runner) SplitOnIFS(s string) []string {
	ifs, set := r.ifs()
	return r.splitFieldsAskPlain(s, ifs, set)
}

// JoinOnIFS joins fields the way `$*` joins them: with the first character of
// IFS, with a space where IFS is unset, and with nothing where it is set and
// empty.
//
// The counterpart of [Runner.SplitOnIFS], and exported for the same reason —
// a builtin that shows a list as one line of text and reads it back has to use
// the same separator in both directions or the round trip is lossy.
func (r *Runner) JoinOnIFS(fields []string) string {
	return strings.Join(fields, r.ifsFirst(r.ifs()))
}
