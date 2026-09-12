// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import "github.com/blairham/sh/syntax"

// Style is how a formatter lays this shell's scripts out.
//
// The core's, and this preset is the honest kind of default rather than a
// measured one: nine ksh scripts are installed on the machine the corpus was
// taken from, four of them indent anything at all, and no style guide for this
// shell was found to appeal to. The question was put and came back empty, so
// the common answer stands until something better is measured — which is a
// different statement from "ksh was measured to agree", and docs/spec/style.md
// keeps the two apart.
//
// What this shell does contribute is a grammar refusal the formatter inherits
// for free: `function f() { … }` is rejected here at parse time, so the
// hybrid spelling never reaches the printer under this dialect.
//
// [syntax.Style.BraceShortForm] is left at the core's ExpandShortForm on
// purpose, and it is reachable here: this shell writes `case x { … }` as well
// as `case x in … esac` (#1928). Expanding is right for it where preserving is
// right for zsh, because **the shell itself calls the spelling obsolete** —
// `ksh -n` on a brace-opened `case` prints “ warning: `{' instead of `in' is
// obsolete “ beside the parse. Rewriting to the keyword form is what the
// vendor is asking for; keeping it would be preserving a deprecation.
func Style() syntax.Style { return syntax.CoreStyle() }
