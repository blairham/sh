// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// ErrorKind classifies a parse failure.
//
// It exists so a dialect can word the failure its own way without matching on
// message text. The set is small on purpose: it distinguishes only the cases
// the panel *words differently*, not every way a parse can fail. dash says
// "Bad substitution" for anything wrong inside `${ }` and "Syntax error: …"
// for everything else, so those are the two.
type ErrorKind int

const (
	// ErrSyntax is a parse failure with no more specific classification.
	ErrSyntax ErrorKind = iota
	// ErrBadSubstitution is a parse failure inside `${ }`. Every shell in
	// the panel has a distinct message for it, and none of them mentions
	// which operator was wrong.
	ErrBadSubstitution
)

// Error is a parse failure with its position and kind.
type Error struct {
	Pos  Pos
	Kind ErrorKind
	Msg  string
}

func (e *Error) Error() string { return e.Pos.String() + ": " + e.Msg }
