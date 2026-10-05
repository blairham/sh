// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// At a prompt, a diagnostic from `eval`'s text is worded as one from the typed
// line where the eval is not inside a function, and located as a script's
// eval text, `(eval):N:`, where it is (#6075). See
// interp.Diagnostics.EvalTextAtAPromptSpeaksAsTheLine.
//
// Measured 2026-10-05 on zsh 5.9.2 (`zsh -f -i`) through a pseudo-terminal
// and on a pipe alike; every expected line is that shell's. This shell wrote
// `(eval):` with no line in both places.
func TestEvalTextAtAPromptIsNamedWhereZshNamesIt(t *testing.T) {
	for _, c := range []struct{ typed, want string }{
		{"eval \"echo \\$((1/0))\"\n", "zsh: division by zero\n"},
		{"eval \"echo )\"\n", "zsh: parse error near `)'\n"},
		{"eval \"set -u; echo \\$nope\"\n", "zsh: nope: parameter not set\n"},
		{"h(){ eval \"true\necho \\$((1/0))\"; }; h\n", "(eval):2: division by zero\n"},
		{"m(){ eval \"cd /nonexist\"; }; m\n", "(eval):cd:1: no such file or directory: /nonexist\n"},
		{"n(){ eval \"echo )\"; }; n\n", "(eval):1: parse error near `)'\n"},
		// The control: a function the eval defines and calls is the
		// function's.
		{"eval \"g(){ echo \\$((1/0)); }; g\"\n", "g: division by zero\n"},
	} {
		t.Setenv("HOME", t.TempDir())
		_, errs, _ := prompt(t, c.typed, "zsh", "-f", "-i")
		if !strings.Contains(errs, c.want) {
			t.Errorf("%q: stderr %q, want %q in it", c.typed, errs, c.want)
		}
	}
}
