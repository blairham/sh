// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `strftime` and the prompt's dates follow the script's `$TZ`, a POSIX zone
// string included; the agreed padding flags; and the operand checks (#5160).
// Measured 2026-10-01 on zsh 5.9.2 under `-c`; each row is what that shell
// wrote.
func TestStrftimeZonesFlagsAndOperands(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		{"an assigned TZ", "zmodload zsh/datetime; TZ=UTC; strftime %X 1200000001", "21:20:01\n"},
		{"a POSIX offset west", "zmodload zsh/datetime; TZ=UTC+5; strftime '%H:%M %Z %z' 0", "19:00 UTC -0500\n"},
		{"and east, with minutes", "zmodload zsh/datetime; TZ=FOO-5:30; strftime '%H:%M %Z %z' 0", "05:30 FOO +0530\n"},
		{"a quoted name", "zmodload zsh/datetime; TZ='<+0330>-3:30'; strftime '%H:%M %Z' 0", "03:30 +0330\n"},
		{"a name no database has is UTC", "zmodload zsh/datetime; TZ=Bogus/Zone; strftime '%H %Z' 0", "00 UTC\n"},
		{"the reverse reads the zone too", "zmodload zsh/datetime; TZ=UTC; strftime -r '%Y-%m-%d %H:%M' '2008-01-10 21:20'", "1200000000\n"},
		{"the prompt's date", "TZ=UTC+5; print -rP -- '%D{%H %Z}' | read v; print -r -- ${v#* }", "UTC\n"},
		{"the padding flags", "zmodload zsh/datetime; TZ=UTC; strftime '%-m|%_m|%0e|%-Oe|%Oe|%_H|%-S|%-M' 1181100005", "6| 6|06|6| 6| 3|5|20\n"},
		{"the suite's row", "zmodload zsh/datetime; TZ=UTC; strftime %-m_%f_%K_%L 1181100000", "6_6_3_3\n"},
		{"nanoseconds past a second", "zmodload zsh/datetime; strftime %N 1012615322 1000000000 2>&1; print $?", "zsh:strftime:1: 1000000000: invalid nanosecond value\n1\n"},
		{"negative nanoseconds", "zmodload zsh/datetime; strftime %N 1012615322 -1 2>&1; print $?", "zsh:strftime:1: -1: invalid nanosecond value\n1\n"},
		{"an empty nanosecond count", "zmodload zsh/datetime; strftime %N 1012615322 '' 2>&1; print $?", "zsh:strftime:1: : invalid decimal number\n1\n"},
		{"an empty epoch", "zmodload zsh/datetime; strftime %N '' 2>&1; print $?", "zsh:strftime:1: : invalid argument\n1\n"},
		{"an epoch that starts like a number", "zmodload zsh/datetime; strftime %N 1x 2>&1; print $?", "zsh:strftime:1: 1x: invalid decimal number\n1\n"},
		{"an epoch that does not", "zmodload zsh/datetime; strftime %N abc 2>&1; print $?", "zsh:strftime:1: abc: invalid argument\n1\n"},
		{"nanoseconds that do not either", "zmodload zsh/datetime; strftime %N 5 abc 2>&1; print $?", "zsh:strftime:1: abc: invalid decimal number\n1\n"},
		{"blank nanoseconds are nought", "zmodload zsh/datetime; strftime %N 5 ' '", "000000000\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, _ := runZsh(t, dir, c.src+"\n"); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
