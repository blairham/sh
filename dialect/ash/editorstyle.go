// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash

import "github.com/blairham/sh/repl"

// EditorStyle is what ash draws while a line is being typed.
//
// `^C` after the abandoned line, which is what a shell taking the terminal raw
// has to draw to look like one that lets the terminal echo it.
func EditorStyle() repl.EditorStyle {
	return repl.EditorStyle{Interrupt: "^C"}
}
