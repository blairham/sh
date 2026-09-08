// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import "github.com/blairham/sh/interp"

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
func PromptStyle() interp.PromptStyle {
	return interp.PromptStyle{
		Expand: true,
		// Measured with nothing assigned: real ksh93 prompts `$ ` and continues
		// with `> `, which is what the substrate does anyway. Stated rather than
		// left empty so the dialect describes itself.
		Default:          "$ ",
		DefaultContinued: "> ",
		// Not AssignsWithNobodyToPrompt, and the reason is measured rather
		// than a simplification: ksh93 with nobody to prompt leaves PS1
		// *unset* and has PS2 set to `> `. That is a split between the two
		// parameters, which this table has no way to say — it assigns them
		// together — so the answer taken is the one PS1 gives, because PS1
		// is what a startup file guards on. The standing difference is
		// ksh93's non-interactive PS2, recorded in docs/spec/prompt.md.
		//
		// And the one column that assigns PS1 late. Measured through a pty
		// with `$ENV` naming a file that prints `${PS1+set}`: ksh93 has PS2
		// and PS4 in hand while that file runs and PS1 *unset*, and reads
		// `$ ` by the time a prompt is drawn. The other five have PS1 before
		// the file. See interp.PromptStyle.DefaultsFollowTheStartupFiles.
		DefaultsFollowTheStartupFiles: true,
		// ksh93 has an escape character and nothing behind it: `\u` drew `u`,
		// `\h` drew `h`, `\w` drew `w`. The backslash goes and the letter
		// stands, which is what DropEscape says and why the table is empty.
		//
		// `\!` draws the history number by that same rule rather than by a
		// table entry: dropping the backslash leaves a bare `!`, and a bare
		// `!` is the history number in ksh93 — which History is for.
		Escape:  '\\',
		Unknown: interp.DropEscape,
		// Measured: `<!>` drew 1, 2 and 3 on successive prompts with a
		// writable history file, `<!!>` drew `<!>`, and `<a!b>` drew `<a1b>`.
		// bash, dash and zsh draw a bare `!` as a bare `!`.
		History: '!',
	}
}
