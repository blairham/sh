// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import "github.com/blairham/sh/repl"

// EditorStyle is what ksh93 draws while a line is being typed.
//
// // Measured: real ksh93 leaves the abandoned line as it is and draws no mark
// after it.
func EditorStyle() repl.EditorStyle {
	return repl.EditorStyle{Interrupt: ""}
}
