// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// bash has no precommand modifier class, so `exec` with only redirections
// and a `builtin` naming no builtin are traced as the command words, before
// the command runs. Measured 2026-11-02 against GNU bash 5.3.20
// (`/opt/homebrew/bin/bash`) under `-c` and `env -i PATH=/usr/bin:/bin`. The
// control for the one dialect that writes no line for these (#7097).
func TestAModifierThatRunsNothingIsStillTraced(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"exec's redirection form", `exec 3>/dev/null; echo ok`, "+ exec\n+ echo ok\nok\n"},
		{"a prefix", `x=1 exec 3>/dev/null; echo ok`, "+ x=1\n+ exec\n+ echo ok\nok\n"},
		{
			"no such builtin", `builtin nosuch; echo ok`,
			"+ builtin nosuch\nbash: line 1: builtin: nosuch: not a shell builtin\n+ echo ok\nok\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, "set -x; "+c.src)
			if err != nil {
				t.Fatalf("run %q: %v", c.src, err)
			}
			if out != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}
