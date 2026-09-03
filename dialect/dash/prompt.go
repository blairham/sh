// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash

import "github.com/blairham/sh/repl"

// PromptStyle is what dash does to a prompt parameter's value before drawing
// it.
//
// dash expands the value. An inherited `PS1='<$LOGNAME>@ '` draws the name,
// and `PS1='$((1+1))'` draws 2. It has no escape language beyond what
// expansion itself gives — which is where a `\\$` drawing a bare `$` comes
// from, since that is what a backslash does to a dollar during expansion and
// not a prompt feature at all.
//
// Measured through a pty rather than taken from documentation: the panel
// disagrees about this, and the disagreement is why the field exists.
func PromptStyle() repl.PromptStyle {
	return repl.PromptStyle{
		Expand: true,
		// Measured with nothing assigned: real dash prompts `$ ` and continues
		// with `> `, which is what the substrate does anyway. Stated rather than
		// left empty so the dialect describes itself.
		Default:          "$ ",
		DefaultContinued: "> ",
		// dash has no escape language. `\u` draws `\u`, and a `\$` drawing a
		// bare dollar is expansion's doing rather than the prompt's.
	}
}
