// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import "github.com/blairham/sh/repl"

// PromptStyle is what ksh93 does to a prompt parameter's value before drawing
// it.
//
// ksh93 expands the value.
//
// It does two further things that are not settled here: a bare `!` becomes the
// history number, and a backslash is dropped from whatever follows it, so
// `\\u` draws `u` rather than the user name.
//
// Measured through a pty rather than taken from documentation: the panel
// disagrees about this, and the disagreement is why the field exists.
func PromptStyle() repl.PromptStyle {
	return repl.PromptStyle{Expand: true}
}
