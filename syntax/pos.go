// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package syntax turns shell source into tokens and a syntax tree.
//
// It is written from docs/spec, which is written from POSIX, from vendor
// documentation, and from recorded runs of real shells. See CLEANROOM.md.
package syntax

import "fmt"

// Pos is a location in the input.
//
// Every token carries one. That is not only for diagnostics: this package is
// meant to run on the keystroke path — a shell re-parses the current line on
// every keypress to highlight it — so a caller needs to map a token back to
// the characters the user is looking at, and it needs to do so on input that
// is incomplete or malformed.
type Pos struct {
	// Offset is the byte offset from the start of the input, zero-based.
	Offset int
	// Line is the one-based line number.
	Line int
	// Col is the one-based column, counted in bytes rather than in runes or
	// grapheme clusters. Callers rendering a caret under a token want display
	// width, which depends on the terminal and on Unicode data this package
	// deliberately does not carry; Offset is the honest anchor for that.
	Col int
}

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// IsValid reports whether p refers to a place in some input. The zero Pos does
// not: lines and columns are one-based, so a zero Line means unset rather than
// "the first line".
func (p Pos) IsValid() bool { return p.Line > 0 }

// After reports whether p is later in the input than q.
func (p Pos) After(q Pos) bool { return p.Offset > q.Offset }
