// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package dialect holds one package per shell the substrate imitates.
//
// Nothing here is imported by syntax or interp, and that is the point. The
// substrate defines the *questions* — a grammar flag, a semantics axis, a
// diagnostic value — and each shell answers them in its own package. Adding a
// shell adds a directory; it does not touch the core.
//
// Each package exports the same four functions, one per vector:
//
//	Dialect()     what parses
//	Semantics()   what it means where the shells conflict
//	Diagnostics() how failure is reported
//	Style()       how a formatter lays it back out
//
// Style is the newest and the thinnest, because most of what a formatter
// decides is not a shell's to decide. It earns its place on one question:
// zsh's brace-spelled bodies parse to the tree the keyword spelling parses
// to, so nothing but a dialect's stated preference can say which spelling a
// formatter writes back. docs/spec/style.md holds the measurements.
//
// No shell is defined in terms of another. Every preset starts from
// syntax.POSIX or interp.PosixSemantics — the standard, which is not a shell
// and has no successors — and overrides only what was measured to differ.
// Deriving zsh from bash, which is what this replaced, once silently gave zsh
// bash's answer for whether a readonly reassignment is fatal; zsh's own answer
// is the opposite, and nothing caught it until something exercised it. A
// preset that inherits from a sibling inherits its future mistakes too.
package dialect
