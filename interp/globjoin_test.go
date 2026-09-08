// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// TestGlobJoinDoesNotClean states the property the whole fix rests on, at the
// one place that can state it plainly.
//
// filepath.Join would answer `<base>` for the first row and `/` for the
// separator rows: cleaning is not a detail of Join, it is the whole of what
// it adds. The trailing-separator rows are the branch — the root, and a Dir
// written with a slash on the end — where appending a second one would give
// `//name`.
func TestGlobJoinDoesNotClean(t *testing.T) {
	for _, tc := range []struct{ dir, name, want string }{
		{"/base", ".", "/base/."},
		{"/base", "..", "/base/.."},
		{"/base", "x", "/base/x"},
		{".", "x", "./x"},
		{"/", "x", "/x"},
		{"/", ".", "/."},
		{"/base/", "x", "/base/x"},
	} {
		if got := globJoin(tc.dir, tc.name); got != tc.want {
			t.Errorf("globJoin(%q, %q) = %q, want %q", tc.dir, tc.name, got, tc.want)
		}
	}
}
