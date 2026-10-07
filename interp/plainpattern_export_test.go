// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// SetPlainPatternShortcutForTest switches the plain-pattern answer on or off
// and returns what it was, so a test can run the same script both ways.
func SetPlainPatternShortcutForTest(on bool) (was bool) {
	was, plainPatternShortcut = plainPatternShortcut, on
	return was
}
