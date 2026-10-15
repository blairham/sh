// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// SetPlainScalarStoreShortcutForTest switches the plain store on or off and
// returns what it was, so a test can run the same script both ways.
func SetPlainScalarStoreShortcutForTest(on bool) (was bool) {
	was, plainScalarStoreShortcut = plainScalarStoreShortcut, on
	return was
}

// StoresAPlainScalarForTest is storesAPlainScalar, for a test that has to show
// the shortcut is reached at all.
func StoresAPlainScalarForTest(r *Runner, a *syntax.Assign) bool {
	return r.storesAPlainScalar(a)
}

// SetAssignmentsAloneShortcutForTest switches assignmentsAloneCommand on or
// off and returns what it was, so a test can run the same script both ways.
func SetAssignmentsAloneShortcutForTest(on bool) (was bool) {
	was, assignmentsAloneShortcut = assignmentsAloneShortcut, on
	return was
}

// SetPlainArithShortcutForTest switches plainArithValue on or off and returns
// what it was, so a test can run the same script both ways.
func SetPlainArithShortcutForTest(on bool) (was bool) {
	was, plainArithShortcut = plainArithShortcut, on
	return was
}
