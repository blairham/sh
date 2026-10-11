// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// SetPlainPatternShortcutForTest switches the plain-pattern answer on or off
// and returns what it was, so a test can run the same script both ways.
func SetPlainPatternShortcutForTest(on bool) (was bool) {
	was, plainPatternShortcut = plainPatternShortcut, on
	return was
}

// SetPlainParamShortcutForTest switches the plain `$name` answer on or off and
// returns what it was, so a test can run the same script both ways.
func SetPlainParamShortcutForTest(on bool) (was bool) {
	was, plainParamShortcut = plainParamShortcut, on
	return was
}

// PlainParamWordForTest is plainParamWord, for a test that has to show the
// shortcut is reached at all under a dialect's own runner.
func PlainParamWordForTest(r *Runner, w *syntax.Word) (string, bool) {
	return r.plainParamWord(w)
}
