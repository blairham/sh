// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash

import "github.com/blairham/sh/syntax"

// Style is how a formatter lays this shell's scripts out.
//
// The core's, and labeled a default rather than evidence for the same reason
// dash's is: there is no ash style guide to fall back on, and there are no ash
// scripts on the machine to count — this shell does not run on macOS at all,
// which is the whole of why it has no oracle column.
//
// It reaches few of the formatter's questions in any case: no brace short
// form, no `;;&`, and a `[[ ]]` that is a builtin rather than a construct with
// a layout of its own.
func Style() syntax.Style { return syntax.CoreStyle() }
