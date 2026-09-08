// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash

import "github.com/blairham/sh/interp"

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
func PromptStyle() interp.PromptStyle {
	return interp.PromptStyle{
		Expand: true,
		// Measured with nothing assigned: real dash prompts `$ ` and continues
		// with `> `, which is what the substrate does anyway. Stated rather than
		// left empty so the dialect describes itself.
		Default:          "$ ",
		DefaultContinued: "> ",
		// And dash assigns them to a script as well, which is one of the
		// three answers the panel gives. Measured on `-c` and on a script
		// file alike with nothing inherited: dash reports PS1 `$ ` and PS2
		// `> ` where the three bash members and ksh93 leave PS1 unset and
		// zsh sets it to the empty string. See
		// interp.PromptStyle.AssignsWithNobodyToPrompt.
		AssignsWithNobodyToPrompt:          true,
		DefaultWithNobodyToPrompt:          "$ ",
		DefaultContinuedWithNobodyToPrompt: "> ",
		// dash has no escape language. `\u` draws `\u`, and a `\$` drawing a
		// bare dollar is expansion's doing rather than the prompt's.
	}
}
