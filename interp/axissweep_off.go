// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !shaxissweep

package interp

// axisMutate is the identity in every build but the sweep's.
//
// The axis sweep (#2031) asks, of each field of Semantics, whether anything
// fails when it is moved — because an axis nothing objects to is not a
// measurement, it is a claim about real shells that nobody checked. Asking
// that needs a way to move a field in a *running* shell, and the only honest
// place to do it is where the interpreter reads the vector, so that a test
// building its own Semantics is covered exactly like a dialect binary.
//
// It is a build tag rather than an environment check so that the shipped
// binary has no such seam at all: this body is empty, it inlines to nothing,
// and no value a person can set reaches the vector.
func axisMutate(s Semantics) Semantics { return s }
