// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A prefix that would not expand ends the script with `set -x` on as it does
// without it, on the road where the trace line waits for the redirections
// too. Measured 2026-10-15 on dash 0.5.12: the complaint, status 2, and
// neither the command nor the next line runs. Here the trace's expansion put
// the failure on the record before the route took its walk, so the command
// ran with the name empty (#6544).
func TestATracedPrefixThatWouldNotExpandEndsTheScript(t *testing.T) {
	for _, src := range []string{
		"x=4\nset -x\nx=$((1/0)) eval 'echo RAN[$x]'\necho after\n",
		"x=4\nset -x\nx=$((1/0)) eval 'echo RAN[$x]' >/dev/null\necho after\n",
	} {
		out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
		if err != nil {
			t.Fatal(err)
		}
		if want := "dash: 3: arithmetic expression: division by zero: \"1/0\"\n"; out != want || st != 2 {
			t.Errorf("%q\n got %q at %d\nwant %q at 2", src, out, st, want)
		}
	}
}
