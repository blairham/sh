// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// What `getopts` does with OPTIND once a script has unset it. Measured
// 2026-10-16 under `-c` on zsh 5.9.2, ksh93u+, bash 5.3.20 and BusyBox ash
// 1.37.0 — see Semantics.GetoptsLeavesAnUnsetOptindUnset (#6660).
func TestGetoptsLeavesAnUnsetOptindUnset(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`unset OPTIND; getopts a o -a; echo "[$?] [$o] [${OPTIND-unset}]"; getopts a o -a -a; echo "[$?] [$o] [${OPTIND-unset}]"`, "[0] [a] [unset]|[0] [a] [unset]"},
		{`f() { unset OPTIND; getopts a o -a; echo "in [${OPTIND-unset}]"; }; f; echo "[${OPTIND-unset}]"`, "in [unset]|[unset]"},
		// The scan after the unset: from its own place, or from the start.
		{`getopts ab o -a -b; unset OPTIND; getopts ab o -a -b; echo "$o ${OPTIND-u}"; getopts ab o -a -b; echo "$? ${OPTIND-u}"`, "b u|1 u"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if want := strings.ReplaceAll(tc.want, "|", "\n") + "\n"; out != want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, want)
		}
	}
}
