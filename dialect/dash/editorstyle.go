// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash

import "github.com/blairham/sh/repl"

// EditorStyle is what dash draws while a line is being typed.
//
// // Measured: `^C` appears after the abandoned line. dash has no line editor,
// so that is the terminal echoing the interrupt character rather than dash
// drawing it — but a shell that takes the terminal raw, as this one does,
// has to draw it to look the same.
func EditorStyle() repl.EditorStyle {
	return repl.EditorStyle{Interrupt: "^C"}
}
