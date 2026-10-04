// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **What a `<<-` body does with a continuation that stands before any text**
// — syntax.Dialect.HeredocLeadingContinuationStripsTheNextLine and
// HeredocStrippedLoneBackslashIsKept. Measured 2026-10-03 from script files:
// bash strips the joined line's tabs, dash keeps a lone backslash that had a
// tab before it and takes the next line as written, zsh strips only the first
// line's tabs.
func TestALeadingContinuationInAStrippedBody(t *testing.T) {
	src := "while IFS= read -r l; do echo \"[$l]\"; done <<-EOF\n\t\\\n\tfoo\n\\\n\tbar\nEOF\n"
	for name, want := range map[string]string{
		"bash": "[foo]\n[bar]\n",
		"dash": "[\\]\n[\tfoo]\n[bar]\n",
		"zsh":  "[\tfoo]\n[\tbar]\n",
	} {
		out, _, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
		if err != nil {
			t.Fatal(err)
		}
		if out != want {
			t.Errorf("%s: got %q, want %q", name, out, want)
		}
	}
}
