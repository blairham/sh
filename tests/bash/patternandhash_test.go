// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A pattern operand of `[[ ]]` that does not expand stops a `-c` script the
// way the same expansion stops it as a command word. Measured 2026-10-03 on
// bash 5.3.20: `[[ x == ${~p} ]]; echo after`, the same inside a `for` loop
// and inside an `if`, each print `${~p}: bad substitution` once and exit 1
// without the `after`.
func TestAPatternThatDoesNotExpandStopsTheScript(t *testing.T) {
	for _, src := range []string{
		`[[ x == ${~p} ]]; echo after`,
		`[[ x != ${~p} ]]; echo after`,
		`for s in a b; do [[ $s == ${~p} ]] && echo y; done; echo after`,
		`if [[ x == ${~p} ]]; then :; fi; echo after`,
	} {
		out, errs, code := runEmptyArray(t, src)
		if out != "" || code != 1 || strings.Count(errs, "bad substitution") != 1 {
			t.Errorf("%q: out %q err %q status %d, want nothing, one bad substitution, 1", src, out, errs, code)
		}
	}
}

// `hash -d` passes over a name written with a slash the way `hash` does,
// silent at 0, and still refuses a name it has not got. Measured 2026-10-03
// on bash 5.3.20.
func TestForgettingANameWithASlashIsSilent(t *testing.T) {
	for _, tc := range []struct {
		src, errs string
		code      int
	}{
		{`hash -d nd=/tmp`, "", 0},
		{`hash -d a/b`, "", 0},
		{`hash -d /tmp`, "", 0},
		{`hash -d zz`, "hash: zz: not found", 1},
		{`hash -d a=b`, "hash: a=b: not found", 1},
	} {
		_, errs, code := runEmptyArray(t, tc.src)
		if code != tc.code || (tc.errs == "") != (errs == "") || !strings.Contains(errs, tc.errs) {
			t.Errorf("%q: err %q status %d, want %q and %d", tc.src, errs, code, tc.errs, tc.code)
		}
	}
}

// A substring of a name with no value reads neither its offset nor its
// length, where the same expansion on a set and empty name reads both.
// Measured 2026-10-03 on bash 5.3.20.
func TestASubstringOfAnUnsetNameReadsNoOffset(t *testing.T) {
	for _, tc := range []struct{ src, out string }{
		{`unset u; echo "[${u:1/0}]"; echo end`, "[]\nend\n"},
		{`unset u; echo "[${u::1/0}]"; echo end`, "[]\nend\n"},
		{`a=(x); echo "[${a[5]:1/0}]"; echo end`, "[]\nend\n"},
		{`unset u; printf "[%s]" "${u::=A}" "$u"; echo`, "[][]\n"},
		// The control: set and empty is a value, and its offset is read.
		{`e=; echo "[${e:1/0}]"; echo end`, ""},
	} {
		out, _, _ := runEmptyArray(t, tc.src)
		if out != tc.out {
			t.Errorf("%q: out %q, want %q", tc.src, out, tc.out)
		}
	}
}
