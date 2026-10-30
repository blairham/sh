// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// An `exit` inside `eval`'d text locates the EXIT trap at the `exit`'s line as
// the text numbers it, and not at the `eval`'s — the other reading of
// Semantics.EvalPutsTheCallersLineBack, which ksh93 takes (#6955).
//
// Measured 2026-10-30 on zsh 5.9.2 under `-f` from script files, with the
// three-line body the neighboring tests use and the text held in `x` so its
// lines are not the file's: the subshell's trap after `eval "$x"` on line 9,
// whose text ends `exit 0` on its line 3, is at 3; the shell's own after an
// `eval` on line 5 whose text's line 2 is `exit 0` is at 2. Only `$LINENO` is
// held here. zsh also names the location `(eval)` there, as if the trap ran
// inside the text, which this shell does not do and which is #6954.
func TestAnExitInEvalTextLocatesTheExitTrapInTheText(t *testing.T) {
	const body = "trap \":\nnosuch_t\necho L=\\$LINENO\" EXIT\n"
	for _, c := range []struct{ name, src, out string }{
		{"a subshell's trap", "x=\":\n:\nexit 0\"\n(\n" + body + ":\neval \"$x\"\n:\n)\n:\n", "L=3\n"},
		{"the shell's own trap", ":\n" + body + ":\n:\neval ':\nexit 0'\n:\n", "L=2\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, _ := runZshSplitOnRoute(t, interp.RouteScriptFile, strings.TrimSuffix(c.src, "\n"))
			if out != c.out {
				t.Errorf("wrote %q, want %q", out, c.out)
			}
		})
	}
}
