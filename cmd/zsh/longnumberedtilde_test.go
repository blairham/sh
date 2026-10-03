// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// TestALongNumberedTildeWarnsAndALongBareOneIsAName pins that only a bare
// numeral of one or two characters is a directory stack index — three or more
// are a user name — and that a numeral overflowing 64 bits warns before the
// tilde is answered (#5694). Measured 2026-10-03 on zsh 5.9.2 under -f.
func TestALongNumberedTildeWarnsAndALongBareOneIsAName(t *testing.T) {
	const big = "99999999999999999999"
	for _, tc := range []struct{ src, out, errs string }{
		{`print ~100; print after`, "", "zsh:1: no such user or named directory: 100\n"},
		{`print ~001; print after`, "", "zsh:1: no such user or named directory: 001\n"},
		{`print ~10; print after`, "", "zsh:1: not enough directory stack entries.\n"},
		{`x=(~01 ~+001 ~-0001); print ${#x}`, "3\n", ""},
		{`print ~+100; print after`, "", "zsh:1: not enough directory stack entries.\n"},
		{
			`print ~` + big + `/x; print after`, "",
			"zsh:1: number truncated after 19 digits: " + big + "/x\nzsh:1: no such user or named directory: " + big + "\n",
		},
		{
			`print ~9223372036854775808; print after`, "",
			"zsh:1: number truncated after 18 digits: 9223372036854775808\nzsh:1: no such user or named directory: 9223372036854775808\n",
		},
		{
			`print ~+` + big + `; print after`, "",
			"zsh:1: number truncated after 19 digits: " + big + "\nzsh:1: not enough directory stack entries.\n",
		},
		{
			`setopt nonomatch; print ~` + big + ` ~100`, "~" + big + " ~100\n",
			"zsh:1: number truncated after 19 digits: " + big + "\n",
		},
	} {
		out, errs, _ := runZsh(t, "-fc", "pushd -q /tmp; "+tc.src)
		if out != tc.out || errs != tc.errs {
			t.Errorf("%s\n got %q %q\nwant %q %q", tc.src, out, errs, tc.out, tc.errs)
		}
	}
}
