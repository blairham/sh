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
//
// The three fields are int32 rather than int, and that is a size decision
// with a measurement behind it. A Pos is not a thing the tree holds one of:
// a [Word] holds two and every [Span] inside it holds another, so a Pos is
// the single most repeated shape in a parsed program. At three ints it was
// 24 bytes and a Word was 72; at int32 it is 12 and a Word is 48, which is
// also a Go size class rather than three quarters of one. Over the scripts a
// real interactive startup reads, that plus the [Span] field order took the
// retained tree down by a fifth — and the retained tree is what the heap is
// sized by, which is what the collector's page mapping and zeroing are
// proportional to (#2073).
//
// The bound it buys that with is 2GiB of input, per file. A shell script that
// large is not a thing, and the parser already holds the whole of its input
// as one string, so the limit that would be reached first is not this one.
type Pos struct {
	// Offset is the byte offset from the start of the input, zero-based.
	Offset int32
	// Line is the one-based line number.
	Line int32
	// Col is the one-based column, counted in bytes rather than in runes or
	// grapheme clusters. Callers rendering a caret under a token want display
	// width, which depends on the terminal and on Unicode data this package
	// deliberately does not carry; Offset is the honest anchor for that.
	Col int32
}

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// IsValid reports whether p refers to a place in some input. The zero Pos does
// not: lines and columns are one-based, so a zero Line means unset rather than
// "the first line".
func (p Pos) IsValid() bool { return p.Line > 0 }

// After reports whether p is later in the input than q.
func (p Pos) After(q Pos) bool { return p.Offset > q.Offset }
