// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// SetUnreadHoldShortcutForTest switches the route that leaves out an unread
// operand's hold on or off and returns what it was, so a test can run the same
// script both ways.
func SetUnreadHoldShortcutForTest(on bool) (was bool) {
	was, unreadHoldShortcut = unreadHoldShortcut, on
	return was
}

// UnreadHoldsForTest is how many operand holds the route left out.
func UnreadHoldsForTest(r *Runner) int { return r.unreadHolds }
