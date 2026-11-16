// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// SetPlainKeyShortcutForTest switches the single read of a subscript of one
// bare `$name` on or off and returns what it was, so a test can run the same
// script both ways.
func SetPlainKeyShortcutForTest(on bool) (was bool) {
	was, plainKeyShortcut = plainKeyShortcut, on
	return was
}

// PlainKeyReadsForTest is how many keys came to by the single read.
func PlainKeyReadsForTest(r *Runner) int { return r.plainKeyReads }
