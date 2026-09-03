// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import "github.com/blairham/sh/repl"

// PromptStyle is what bash does to a prompt parameter's value before drawing
// it.
//
// bash expands the value each time the prompt is drawn — parameters, command
// substitutions and arithmetic alike, so `PS1='$(echo sub)@ '` runs the
// command again at every prompt.
//
// It also has a backslash language of its own, where `\\u` is the user name and
// `\\W` the directory's last component. That is a separate question and is not
// settled here.
//
// Measured through a pty rather than taken from documentation: the panel
// disagrees about this, and the disagreement is why the field exists.
func PromptStyle() repl.PromptStyle {
	return repl.PromptStyle{Expand: true}
}
