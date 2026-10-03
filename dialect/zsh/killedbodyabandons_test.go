// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
	"time"
)

// **A background body killed while it runs a pipeline or a `( … )` ends at
// once** (#5386). Measured 2026-10-02 on zsh 5.9.2 under `-f -c`: the `wait`
// returns as the signal lands, with the signal's status, and the body says
// nothing more. See interp.Runner.abandonOnDeath.
func TestAKilledBodyDoesNotWaitOutWhatItRuns(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a pipeline",
			`{ /bin/sleep 3 | cat; print after } & /bin/sleep 0.2; kill -TERM $!; wait $!; print end $?`,
			"end 143\n",
		},
		{
			"parentheses",
			`{ ( /bin/sleep 3 ); print after } & /bin/sleep 0.2; kill -TERM $!; wait $!; print end $?`,
			"end 143\n",
		},
		{
			"control: a program of its own",
			`{ /bin/sleep 3; print after } & /bin/sleep 0.2; kill -TERM $!; wait $!; print end $?`,
			"end 143\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			start := time.Now()
			got := runZshC(c.src)
			if got != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, got, c.want)
			}
			// The sleep is three seconds; a shell that waited it out is
			// past two whatever the machine's load.
			if took := time.Since(start); took > 2*time.Second {
				t.Errorf("%s took %v: the body waited out what it ran", c.src, took)
			}
		})
	}
}
