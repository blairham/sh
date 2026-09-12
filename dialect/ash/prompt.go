// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash

import "github.com/blairham/sh/interp"

// PromptStyle is what ash does to a prompt parameter's value before drawing
// it.
//
// It expands the value and has no escape language beyond what expansion gives
// — measured by running the shell rather than taken from documentation, which
// is the rule the panel's disagreement over this field exists for.
//
// The defaults are `$ ` and `> `, and they are assigned where a script can
// read them: a script under this shell reports PS1 `$ ` and PS2 `> ` with
// nothing inherited, which is dash's answer and not bash's.
func PromptStyle() interp.PromptStyle {
	return interp.PromptStyle{
		Expand:                             interp.PromptExpandsAlways,
		Default:                            "$ ",
		DefaultContinued:                   "> ",
		AssignsWithNobodyToPrompt:          true,
		DefaultWithNobodyToPrompt:          "$ ",
		DefaultContinuedWithNobodyToPrompt: "> ",
	}
}
