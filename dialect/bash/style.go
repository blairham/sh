// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import "github.com/blairham/sh/syntax"

// Style is how a formatter lays this shell's scripts out.
//
// Every field is the core's. The Google Shell Style Guide is written for this
// shell and names two of them — "Indent 2 spaces. No tabs." and `; then` /
// `; do` on the header's line — and the installed corpus agrees with both: 133
// of 220 bash files indent two spaces against 81 at four, and the header's
// line wins 1336:162 for `then` and 198:73 for `do`.
//
// The 60/36 indent split is not on its own a decision, and this preset does
// not pretend otherwise; it is the named guide that settles it. See
// docs/spec/style.md.
func Style() syntax.Style { return syntax.CoreStyle() }
