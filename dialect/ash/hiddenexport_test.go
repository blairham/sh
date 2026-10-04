// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// TestAHiddenExportReachesNoChild: a valueless local hides an exported name
// from a child as well as from the script, though a local with a value is
// still exported. Measured 2026-10-03 in the pinned image. See
// interp.Semantics.AHiddenExportStillReachesAChild.
func TestAHiddenExportReachesNoChild(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`export FOO=bar; f() { local FOO; env | grep '^FOO=' || echo none; }; f`, "none\n"},
		{`export FOO=bar; g() { local FOO; env | grep '^FOO=' || echo none; }; f() { local FOO=mid; g; }; f`, "none\n"},
		{`export FOO=bar; f() { local FOO=x; env | grep '^FOO=' || echo none; }; f`, "FOO=x\n"},
		{`export FOO=bar; f() { local FOO; }; f; env | grep '^FOO='`, "FOO=bar\n"},
	} {
		if out, _ := run(t, tc.src); out != tc.want {
			t.Errorf("%s\n got %q, want %q", tc.src, out, tc.want)
		}
	}
}
