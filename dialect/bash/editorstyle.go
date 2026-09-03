// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import "github.com/blairham/sh/repl"

// EditorStyle is what bash draws while a line is being typed.
//
// // Measured: real bash draws `^C` after the abandoned line and starts the next
// prompt below it.
func EditorStyle() repl.EditorStyle {
	return repl.EditorStyle{Interrupt: "^C"}
}
