// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// dash has no arrays, so a bracketed word in front of a command is the command
// name and there is no element for an entry to see. The controls for #6985,
// where ksh93 and zsh show the entries behind a subscripted prefix entry its
// element, beside a scalar entry, which is seen here as everywhere. Measured
// 2026-10-29 against dash 0.5.12 under `env -i PATH=/usr/bin:/bin`, from `-c`
// and from a script file alike. See interp/prefixsubscriptinturn.go.
func TestAPrefixWithABracketIsTheCommandsName(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a scalar entry", `v=Q y=$v f`, "f y=Q\n"},
		{"a scalar entry, to a program", `v=Q y=$v /usr/bin/printenv y`, "Q\n"},
		{"a bracketed word is a command name", `b[1]=Q y=x f; echo after $?`, "dash: 2: b[1]=Q: not found\nafter 127\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runDash(t, t.TempDir(), "f() { echo \"f y=$y\"; }\n"+c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}
