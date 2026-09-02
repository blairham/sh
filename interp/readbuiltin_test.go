// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What `read` does when the input runs out, which is two questions and was
// answered as one. Every row here is unanimous across the panel — it is the
// core's behavior, not a dialect's.
func TestReadAtEndOfInput(t *testing.T) {
	for _, tc := range []struct {
		name, input, src, want string
	}{
		{
			// The one that bites: a stale value after the loop reads as the
			// last line rather than as nothing.
			"a failing read clears the variable",
			"", `l=keep; read -r l; echo "st=$? l=[$l]"`, "st=1 l=[]\n",
		},
		{
			"every named variable is cleared, not just the first",
			"", `a=1; b=2; read -r a b; echo "[$a][$b]"`, "[][]\n",
		},
		{
			// Both answers at once: there is a line, and there will not be
			// another.
			"a final line without a newline is read and reports failure",
			"x", `read -r l; echo "st=$? l=[$l]"`, "st=1 l=[x]\n",
		},
		{
			// The consequence, and the reason the status is not a bug to fix:
			// returning 0 would run the last line twice, once as the line and
			// once as the empty read after it.
			"an unterminated last line is dropped by a loop",
			"a\nb", `while read -r l; do printf "<%s>" "$l"; done; echo`, "<a>\n",
		},
		{
			"a line that is there reports success",
			"a\n", `read -r l; echo "st=$? l=[$l]"`, "st=0 l=[a]\n",
		},
		{
			// An empty line is not end of input, and the status says so.
			"an empty line is a line",
			"\n", `l=keep; read -r l; echo "st=$? l=[$l]"`, "st=0 l=[]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, func(r *Runner) {
				r.Stdin = strings.NewReader(tc.input)
			})
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}
