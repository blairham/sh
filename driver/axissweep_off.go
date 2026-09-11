// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !shaxissweep

package driver

import "github.com/blairham/sh/interp"

// axisMutate is the identity in every build but the sweep's.
//
// The front end reads the vector directly for a dozen questions the
// interpreter never sees — the startup files, the version option, what `-s`
// does with its operands — so interp's own hook does not reach them. Those
// fields would come out of the axis sweep (#2031) reading "nothing objected"
// when what actually happened is that nothing was moved, which is the one
// answer the sweep must never give.
//
// Hooked in withDefaults because that is the single gate every route passes
// through, including the interactive one.
func axisMutate(s interp.Semantics) interp.Semantics { return s }
