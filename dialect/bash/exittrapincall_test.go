// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `exit` inside a function fires the EXIT trap from inside the call here, so
// the body reads the function's own locals and the running function's name.
//
// Measured 2026-09-21 on bash 5.3.20 under `-c` and from a script file
// alike, `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME — 3.2.57
// agrees from a script file and not under `-c`, which the axis records. See
// interp.Semantics.ExitTrapRunsInsideTheExitingCall for the whole panel and
// for the control — the same function left by falling off its end reads the
// global here too.
func TestTheExitTrapSeesTheCallThatExited(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"a local of the function that exited",
			"v=global\ng() { local v=inner; trap 'echo \"v=$v\"' EXIT; exit 0; }\ng",
			"v=inner",
		},
		{
			// `main` is the outermost element from a script file and
			// absent under `-c`, which is what this helper runs; measured
			// both ways on 5.3.20.
			"and the stack that was standing",
			"g() { trap 'echo \"t:${FUNCNAME[*]}\"' EXIT; exit 0; }\nh() { g; }\nh",
			"t:g h",
		},
		{
			"but not one that returned first",
			"v=global\ng() { local v=inner; trap 'echo \"v=$v\"' EXIT; }\ng",
			"v=global",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := answersRun(t, c.src)
			if got := strings.TrimSpace(out); got != c.want {
				t.Errorf("output %q, want %q", got, c.want)
			}
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
		})
	}
}
