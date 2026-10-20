// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A byte a value carries that is not a character on its own is written as an
// escape here too, in this dialect's octal spelling.
//
// The fault was one question in the core — "is a non-ASCII byte ordinary?" —
// answered for the *byte* where the reference answers it for the character,
// so both columns that call such a byte ordinary wrote a raw one into a
// listing (#4521). Measured 2026-09-26 on GNU bash 5.3.20 from a script file
// under `--noprofile --norc`.
func TestAListedByteThatIsNotACharacterIsEscaped(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a lone lead byte", `v=$'\xc3'; declare -p v`, "declare -- v=$'\\303'\n"},
		{"one in the middle of a value", `w=$'a\xc1b'; declare -p w`, "declare -- w=$'a\\301b'\n"},
		{"the high end of the range", `x=$'\xff'; declare -p x`, "declare -- x=$'\\377'\n"},
		{
			"a value holding both writes the character and escapes the byte",
			`z=$'\xc3\xa9\xff'; declare -p z`,
			"declare -- z=$'é\\377'\n",
		},
		// The controls, which are what say this is the high half and not the
		// escaping: a control byte was already escaped, and a character is
		// still written as itself.
		{"a control byte was already escaped", `y=$'\x01'; declare -p y`, "declare -- y=$'\\001'\n"},
		{"and a character is still written as itself", `u=é; declare -p u`, "declare -- u=\"é\"\n"},
		// A table's *key* asks the same question, through a function of its
		// own, and had the same answer.
		{
			"a key carrying one is escaped too",
			`declare -A m; m[$'k\xc3']=1; declare -p m`,
			"declare -A m=([$'k\\303']=\"1\" )\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
