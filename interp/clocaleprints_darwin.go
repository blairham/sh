// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// cLocalePrintsHighByte says whether a byte past ASCII is a printing
// character under the C locale, which is the C library's answer and not the
// shell's: here it is macOS's.
//
// Measured 2026-10-02 on zsh 5.9.2 against this machine's libc, through
// `(V)` under `LC_ALL=C` for every byte from 0x80 to 0xff: 0x80 to 0x9f and
// 0xad are written `\M-` and their low half, and the other ninety-five are
// written as themselves. 0xad is Latin-1's soft hyphen, which this C locale
// classes as a format character rather than a printing one.
func cLocalePrintsHighByte(c byte) bool {
	return c >= 0xa0 && c != 0xad
}
