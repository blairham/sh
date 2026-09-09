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
func Style() syntax.Style { return syntax.CoreStyle() }
