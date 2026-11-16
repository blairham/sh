// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// plainKeyShortcut switches the read below on. It is a variable so a test can
// run the same script with every key read by the full road and require the
// same answers.
var plainKeyShortcut = true

// plainKeyValue is the key a subscript of one bare `$name` comes to, where
// the name is an ordinary scalar holding plain text: that text, read once.
//
// assocKeyIn reads a key two ways and compares them — expanded with its
// quotes removed, and rendered as written — and asks the dialect which to
// take only where the two differ. For a subscript that is one unquoted bare
// `$name`, both are the variable's value. The expansion reads it through the
// word loop, and the rendering reads it again through expandParam, because
// the expansion's mark of standing inside a subscript kept it off the plain
// read (#6423). F-Sy-H reads its state tables this way, `$_fsh_state[$name]`
// and the like, a few dozen times a highlight call.
//
// What the full road does around the two reads does nothing for such a
// word, so it is left out with them:
//
//   - the quoted-text wording is for a refusal, and a plain read refuses
//     nothing;
//   - the leading tilde is a literal `~`, and the word opens with a `$`;
//   - the substitution hold is for a `$(…)` in the subscript, and there is
//     none;
//   - a failed expansion cannot happen on an ordinary scalar that is set.
//
// plainParamValueIn settles everything else before it reads: the name is an
// identifier or a positional parameter, not a reference, a produced
// parameter, an array or a watched name; it is set; and its value is plain
// text with no bracket. A word it declines goes to the full road having read
// nothing.
func (r *Runner) plainKeyValue(w *syntax.Word) (string, bool) {
	if !plainKeyShortcut {
		return "", false
	}
	v, ok := r.plainParamValueIn(w, true)
	if !ok {
		return "", false
	}
	r.plainKeyReads++
	return v, true
}
