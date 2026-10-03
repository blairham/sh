// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
	"time"
)

// **`&` returns at once for a pipeline whose last element runs no program**
// (#5530). Measured 2026-10-02: `/bin/sleep 1 | true &` returns at once in
// zsh 5.9.2 and bash 5.3.20, and `jobs` lists the job running; this shell
// waited out the whole pipeline.
func TestABackgroundedPipelineWithABuiltinLastReturnsAtOnce(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a builtin", `/bin/sleep 3 | true & jobs; kill %1`, "[1]  + running    /bin/sleep 3 | \n       running    true\n"},
		{"a brace group", `/bin/sleep 3 | { : } & jobs; kill %1`, "[1]  + running    /bin/sleep 3 | \n       running    { :; }\n"},
		{"the status is still waited for", `/bin/sleep 0.3 | (exit 3) & wait $!; print w=$?`, "w=3\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			start := time.Now()
			if got := runZshC(c.src); got != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, got, c.want)
			}
			if took := time.Since(start); took > 2*time.Second {
				t.Errorf("%s took %v: `&` waited for the pipeline", c.src, took)
			}
		})
	}
}
