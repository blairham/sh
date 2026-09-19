// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// SetScriptListingLayout says how this shell arranges a whole script it has
// been asked to write back instead of running — see
// Semantics.ScriptListingOption, which holds the rest of the answer.
//
// Beside the runner rather than on the vector, for the reason a function
// listing's arrangement is: a [syntax.Layout] holds a function — the decoder
// that says what a `$'…'` escape comes to, which is the dialect's answer and
// not the printer's — and the semantics vector is a value compared with `==`.
func (r *Runner) SetScriptListingLayout(l syntax.Layout) { r.scriptListingLayout = l }

// ScriptListingLayout is the arrangement SetScriptListingLayout was given.
func (r *Runner) ScriptListingLayout() syntax.Layout { return r.scriptListingLayout }
