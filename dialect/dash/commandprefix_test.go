// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// `command` in front of a special builtin takes its prefix away here, where
// ksh93 looks through the word. Measured 2026-09-18 on dash 0.5.12 from a
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` (#3448).
func TestCommandTakesASpecialBuiltinsPrefixAway(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ src, want string }{
		{`s=base; s=C command :; echo "[$s]"`, "[base]\n"},
		{`s=base; s=E command eval :; echo "[$s]"`, "[base]\n"},
		// The control: the bare special builtin's prefix persists, so the
		// difference is `command`'s alone.
		{`s=base; s=P :; echo "[$s]"`, "[P]\n"},
	} {
		out, st := runDash(t, dir, row.src)
		if out != row.want || st != 0 {
			t.Errorf("%s\n got %q (status %d)\nwant %q", row.src, out, st, row.want)
		}
	}
}
