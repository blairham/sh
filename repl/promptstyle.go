// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// PromptStyle is what a dialect does to a prompt parameter's value before it
// is drawn.
//
// The value is not the prompt. Every shell in the panel transforms it first
// and no two transform it the same way, so the transformation is data a caller
// supplies rather than a rule this package holds — the treatment syntax.Layout
// gets, and for the same reason: a rendering with a shell's taste baked into
// it is a rendering that belongs to that shell.
//
// The zero value draws the value as it stands. That is what a caller without a
// dialect gets, and it is the substrate's own answer rather than a borrowed
// one: told nothing about how to transform the text, it does not transform it.
type PromptStyle struct {
	// Expand runs the value through parameter and command expansion each time
	// the prompt is drawn, so `PS1='$PWD> '` follows the directory and
	// `PS1='$(date +%H:%M) '` follows the clock.
	//
	// Three of the four do this always. zsh does not, unless asked with
	// `setopt PROMPT_SUBST` — which is why this is a field and not a
	// constant, and why it will eventually have to be settable while the
	// shell is running rather than only when it starts.
	Expand bool
}
