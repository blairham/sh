// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// SetPlainLineShortcutForTest switches the route for a plain declaration line
// on or off and returns what it was, so a test can run the same script both
// ways.
func SetPlainLineShortcutForTest(on bool) (was bool) {
	was, plainLineShortcut = plainLineShortcut, on
	return was
}

// PlainLineOperandsForTest is how many declaration operands a plain line's
// route declared.
func PlainLineOperandsForTest(r *Runner) int { return r.plainLineOperands }
