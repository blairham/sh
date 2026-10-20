// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A base declared on one of the shell's produced integers is listed and read:
// measured 2026-10-16 on zsh 5.9.2, `-f`, under `env -i PATH=/usr/bin:/bin` —
// see Semantics.ProducedIntegerReadsInItsBase (#6758).
func TestAProducedIntegerReadsInItsDeclaredBase(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -i2 LINENO; echo $LINENO`, "2#1\n"},
		{`typeset -i16 RANDOM; echo ${RANDOM[1,3]}`, "16#\n"},
		{`typeset -i16 RANDOM; x=$(typeset -p RANDOM); echo ${x%%=*}`, "typeset -i16 RANDOM\n"},
		{`typeset -i16 SECONDS; SECONDS=20; echo $SECONDS; typeset -p SECONDS`, "16#14\ntypeset -i16 SECONDS=20\n"},
		// The control: the arithmetic reading is the number itself.
		{`typeset -i16 RANDOM; echo $(( RANDOM >= 0 ))`, "1\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
