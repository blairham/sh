// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A base declared on one of the shell's produced integers is listed and not
// read, and SECONDS made an integer is whole seconds in that base. Measured
// 2026-10-16 on ksh93u+ under `-c` — see
// Semantics.ProducedIntegerReadsInItsBase (#6758).
func TestAProducedIntegerListsItsDeclaredBase(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -i2 LINENO; echo $LINENO; typeset -p LINENO`, "1\ntypeset -i 2 LINENO=1\n"},
		{`typeset -i16 RANDOM; x=$(typeset -p RANDOM); echo ${x%%=*}`, "typeset -i 16 RANDOM\n"},
		{`typeset -i16 SECONDS; typeset -p SECONDS`, "typeset -i 16 SECONDS=16#0\n"},
		{`typeset -i16 SECONDS; SECONDS=20; echo $SECONDS`, "16#14\n"},
		{`typeset -i SECONDS; typeset -p SECONDS; echo $SECONDS`, "typeset -i SECONDS=0\n0\n"},
	} {
		out, _ := runKsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
