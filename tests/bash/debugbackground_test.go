// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A pipeline sent to the background fires the DEBUG trap in the shell that
// starts it, once per simple element, with $BASH_COMMAND naming the element;
// a compound element and a list fire nothing. Measured 2026-10-16 on bash
// 5.3.20 and 3.2.57, `-c` (#6801).
func TestABackgroundPipelineFiresTheDebugTrapInTheShell(t *testing.T) {
	const trap = `trap 'echo "D[$BASH_COMMAND]"' DEBUG; `
	for _, tc := range []struct{ src, want string }{
		{`true & wait`, "D[true]|D[wait]"},
		{`true | true | true & wait`, "D[true]|D[true]|D[true]|D[wait]"},
		{`{ true; } | true & wait`, "D[true]|D[wait]"},
		{`! true & wait`, "D[true]|D[wait]"},
		{`x=1 & wait`, "D[x=1]|D[wait]"},
		{`f(){ :; }; f & wait`, "D[f]|D[wait]"},
		// The controls: nothing fires for a compound or a list.
		{`(true) & wait`, "D[wait]"},
		{`true && true & wait`, "D[wait]"},
		{`[[ a ]] & wait`, "D[wait]"},
		// And once only where the job carries the trap.
		{`set -T; true & wait`, "D[true]|D[wait]"},
	} {
		src := trap + tc.src
		if strings.HasPrefix(tc.src, "set -T") {
			src = "set -T; " + trap + strings.TrimPrefix(tc.src, "set -T; ")
		}
		out, _ := runBash(t, t.TempDir(), src)
		if want := strings.ReplaceAll(tc.want, "|", "\n") + "\n"; out != want {
			t.Errorf("%s\n got %q\nwant %q", src, out, want)
		}
	}
}
