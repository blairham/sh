// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash

import (
	"fmt"
	"math"
	"strings"
)

// optindRefuses is the check every store into OPTIND makes here, and the
// sentence a value that fails it earns.
//
// Measured 2026-10-15 on dash 0.5.12, `OPTIND=$1` under `-c` with the value
// as the operand:
//
//	taken     07, 00, -0, -00, +3, +0, " 3", "3 ", "\t3", "\n3", "3\r",
//	          2147483647, 0000000002147483647
//	refused   "", " ", x, 1x, 0x10, 0x, +, +-3, "- 1", "+ 3", "3 3", -1,
//	          2147483648, 4294967295, 9223372036854775808, an Arabic-Indic
//	          digit
//
// So a number is optional blanks — every C-locale space, a newline and a
// carriage return among them — an optional sign, at least one ASCII digit and
// optional blanks again, and it is in range when it is neither below zero nor
// above the largest int: `-0` passes and `-1` does not, and leading zeros do
// not count against the range. The value is stored as written; the check only
// decides whether it is stored.
func optindRefuses(value string) (string, bool) {
	if optindNumber(value) {
		return "", false
	}
	return fmt.Sprintf("Illegal number: %s", value), true
}

func optindNumber(value string) bool {
	s := strings.TrimLeft(value, " \t\n\v\f\r")
	s = strings.TrimRight(s, " \t\n\v\f\r")
	negative := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		negative = s[0] == '-'
		s = s[1:]
	}
	if s == "" {
		return false
	}
	var n uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
		if n <= math.MaxInt32 {
			n = n*10 + uint64(c-'0')
		}
	}
	if negative {
		return n == 0
	}
	return n <= math.MaxInt32
}
