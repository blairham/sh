// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

// PromptChar draws the character a person types after.
//
// It is here rather than with the rest of the roster because it is the one
// segment a prompt cannot do without: with no elements configured at all the
// engine draws a bare character, and the least surprising thing a
// configuration that names one element can name is this.
//
// Two states, and the reason they are states rather than two segments is the
// lookup chain: a prompt character that turns red when the last command failed
// is one PROMPT_CHAR_ERROR_FOREGROUND, and nothing else.
//
// The character itself comes from the caller. This package names no shell, and
// which character a shell prompts with is exactly the kind of thing a shell
// decides — `$` and `%` are the same segment with a different letter in it.
func PromptChar(char string) Segment {
	return SegmentFunc(func(settings *Settings, ctx *Context) (Rendered, bool) {
		state := "ok"
		if ctx.Status != 0 {
			state = "error"
		}
		return Rendered{
			Content: settings.Param("prompt_char", state, "SYMBOL", char),
			State:   state,
		}, true
	})
}
