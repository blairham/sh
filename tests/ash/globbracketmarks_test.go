// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestAQuotedCharacterInAGlobBracketIsMarkedAsCaseMarksIt: pathname expansion
// reads a quoted character inside a bracket the way `case` does — a member
// backslash before `- ! ^ * ? [`, and the bare character before anything
// else. Measured 2026-10-03 in the pinned image over a directory holding
// `aXc`, `a\c` and `abc` for each X.
func TestAQuotedCharacterInAGlobBracketIsMarkedAsCaseMarksIt(t *testing.T) {
	for _, tc := range []struct{ x, want string }{
		{")", `a)c|`},
		{"(", `a(c|`},
		{"|", `a|c|`},
		{"~", `a~c|`},
		{"#", `a#c|`},
		{"-", `a-c|a\c|`},
		{"!", `a!c|a\c|`},
		{"*", `a*c|a\c|`},
		{"?", `a?c|a\c|`},
	} {
		dir := t.TempDir()
		for _, name := range []string{"a" + tc.x + "c", `a\c`, "abc"} {
			if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		out, _, err := preset.Combined(t, dialecttest.Base{
			Name: "ash", Env: []string{"PATH=/usr/bin:/bin"}, Dir: dir,
		}, `printf '%s|' a[\`+tc.x+`]c`)
		if err != nil {
			t.Fatal(err)
		}
		if out != tc.want {
			t.Errorf("a[\\%s]c: got %q, want %q", tc.x, out, tc.want)
		}
	}
}
