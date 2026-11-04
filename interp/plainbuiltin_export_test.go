// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// SetPlainBuiltinShortcutForTest switches plainBuiltinCommand on or off and
// returns what it was, so a test can run the same script both ways.
func SetPlainBuiltinShortcutForTest(on bool) (was bool) {
	was, plainBuiltinShortcut = plainBuiltinShortcut, on
	return was
}

// PlainBuiltinsForTest is how many commands plainBuiltinCommand ran itself.
func PlainBuiltinsForTest(r *Runner) int { return r.plainBuiltins }

// PlainDeclarationLinesForTest is how many declaration lines
// plainBuiltinCommand ran itself.
func PlainDeclarationLinesForTest(r *Runner) int { return r.plainDeclarationLines }

// PlainValuedDeclarationLinesForTest is how many of those had a `name=$param`
// operand.
func PlainValuedDeclarationLinesForTest(r *Runner) int { return r.plainValuedDeclarationLines }
