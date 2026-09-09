// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash

import "github.com/blairham/sh/syntax"

// Style is how a formatter lays this shell's scripts out.
//
// The core's, on the same footing as ksh's: seven dash scripts on the machine
// and two of them voting on an indent is not a measurement, and POSIX has no
// style guide to fall back on. The common answer stands, labeled as a default
// rather than as evidence.
//
// This shell reaches the fewest of the formatter's questions in any case. It
// has no brace short form, no `;&`, no `[[ ]]` — so most of what Style asks is
// unreachable here, and the layout it does get is the core's plain one.
func Style() syntax.Style { return syntax.CoreStyle() }
