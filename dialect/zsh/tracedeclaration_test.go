// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A declaration utility's `name=value` operand is traced the way this shell
// writes an assignment of its own — the target bare and only the value
// quoted — where bash quotes the whole word.
//
// Measured 2026-09-18 on zsh 5.9.2, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with `set -x` on line 1. `typeset 'x=1'` was
// this column's answer for every one of these before #3654, and the two
// neighbors `export` and `declare` were no better — the issue's table had
// them right by accident, since a value with nothing in it to quote hides
// the difference.
func TestADeclarationsOperandIsTracedAsAnAssignment(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`typeset x=1`, "> typeset x=1\n"},
		{`export ev=1`, "> export ev=1\n"},
		{`declare dv=2`, "> declare dv=2\n"},
		{`local lv=3`, "> local lv=3\n"},
		{`readonly rv=4`, "> readonly rv=4\n"},
		{`integer iv=5`, "> integer iv=5\n"},
		{`typeset -i y=6`, "> typeset -i y=6\n"},
		{`typeset x=1 y=2`, "> typeset x=1 y=2\n"},
		// The value is quoted on its own, which is where the two readings
		// part: bash writes `typeset 'x=a b'` for the first of these.
		{`typeset x="a b"`, "> typeset x='a b'\n"},
		{`typeset x=a=b`, "> typeset x='a=b'\n"},
		{`typeset x="*"`, "> typeset x='*'\n"},
		{`typeset x=`, "> typeset x=''\n"},
		{`export x="a b" PATH`, "> export x='a b' PATH\n"},
		// And the command decides, not the word: the same bytes after a
		// command that declares nothing are one quoted word.
		{`echo "x=a b"`, "> echo 'x=a b'\n"},
		// A spelling this shell does not read as a declaration's command
		// word makes the operand an ordinary word again, which is what says
		// the trace and the expansion agree about what the line meant.
		{`'typeset' x="a b"`, "> typeset 'x=a b'\n"},
		{`c=typeset; $c x="a b"`, "> typeset 'x=a b'\n"},
	} {
		out, _ := runZsh(t, dir, "set -x\n"+tc.src+"\n")
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: %q\n does not carry %q", tc.src, out, tc.want)
		}
	}
}
