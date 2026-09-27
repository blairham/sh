// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// Asking whether a parameter this shell has not got is there.
//
// The seven names dialect/zsh/parameter.go registers refuse by name when they
// are *read*, which is deliberate and is what lets a module load over one of
// them — see interp/absentparam.go, #1058 and #1146. `${+name}` is not a read:
// it substitutes `1` or `0` and never the value. It refused anyway, and the
// two other spellings of the same question answered, in this shell, in one
// run (#4882).
func TestAskingWhetherAnUnimplementedParameterIsThere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the three spellings agree now",
			`print -r -- "[${+dis_builtins}]"; [[ -v dis_builtins ]]; print -r -- "[$?]"; print -r -- "[${dis_builtins+isthere}]"; print -r -- after`,
			"[0]\n[1]\n[]\nafter\n",
		},
		{
			"and the feature test a script actually writes",
			`(( ${+functions_source} )) && print -r -- would-use-it; print -r -- reached`,
			"reached\n",
		},
		// A subscript and an expansion flag leave the reading alone, which is
		// measured: `${+v[1]}` and `${(k)+v}` on an unset name are both `0`
		// in zsh 5.9.2.
		{"a subscript does not make it a read", `print -r -- "[${+modules[1]}]"; print -r -- after`, "[0]\nafter\n"},
		{"nor does a flag", `print -r -- "[${(k)+patchars}]"; print -r -- after`, "[0]\nafter\n"},
		// The answer is a real one, from the same shell: a name that is there
		// answers 1 and one nothing has heard of answers 0.
		{
			"the control on either side",
			`print -r -- "[${+PATH}][${+usergroups}][${+neverheardof}]"`,
			"[1][0][0]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// And a *read* of one still ends the script, naming the parameter.
//
// The control that says the exemption was widened by one spelling rather than
// switched off — every row here carries a value out of the name, and the
// first two carry the `+` as well.
func TestReadingAnUnimplementedParameterStillRefuses(t *testing.T) {
	for _, src := range []string{
		`print -r -- "[${+dis_builtins#a}]"`,
		`print -r -- "[${+dis_builtins%a}]"`,
		`print -r -- "[$dis_builtins]"`,
		`print -r -- "[${#dis_builtins}]"`,
		`print -r -- "[${dis_builtins[x]}]"`,
		`print -r -- "[${(k)dis_builtins}]"`,
	} {
		t.Run(src, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), src+"\nprint -r -- unreached")
			if !strings.Contains(out, "dis_builtins: parameter not implemented yet") {
				t.Errorf("%s = %q, want the parameter named", src, out)
			}
			if strings.Contains(out, "unreached") {
				t.Errorf("%s = %q, want the script to end there", src, out)
			}
			if st == 0 {
				t.Errorf("%s = %q at 0, want a failure", src, out)
			}
		})
	}
}
