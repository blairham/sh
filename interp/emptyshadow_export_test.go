// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// SetEmptyShadowShortcutForTest switches the route for a `local` operand over
// a name with nothing to shadow on or off and returns what it was, so a test
// can run the same script both ways.
func SetEmptyShadowShortcutForTest(on bool) (was bool) {
	was, emptyShadowShortcut = emptyShadowShortcut, on
	return was
}

// EmptyShadowsForTest is how many `local` operands took that route.
func EmptyShadowsForTest(r *Runner) int { return r.emptyShadows }

// EmptyReturnsForTest is how many names a scope's return put back through
// the route for an empty shadow over a still-plain name.
func EmptyReturnsForTest(r *Runner) int { return r.emptyReturns }
