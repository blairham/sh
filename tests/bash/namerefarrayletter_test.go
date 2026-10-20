// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The `n` letter beside `a` or `A` is dropped where the declaration makes a
// global binding and refused where it makes a local one — measured on bash
// 5.3.20, the only shell that spells both letters.
func TestTheReferenceLetterBesideAnArrayLetter(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"at the top level", `typeset -an t; t=(x); echo "[${t[*]}]"`, "[x]\n"},
		{"with a value", `typeset -na t=v; echo "[${t[*]}]"`, "[v]\n"},
		{"sent global from a call", `f() { typeset -g -an d; d=(y); }; f; echo "[${d[*]}]"`, "[y]\n"},
		{"local refuses", `f() { typeset -an l 2>/dev/null; echo "st=$?"; }; f`, "st=1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := answersRun(t, c.src)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}
