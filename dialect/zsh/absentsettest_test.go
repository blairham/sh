// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// Asking whether a parameter this shell has not got is there.
//
// A name registered with [interp.Runner.SetAbsentParameter] refuses by name
// when it is *read*, which is deliberate and is what lets a module load over
// one — see interp/absentparam.go, #1058 and #1146. `${+name}` is not a read:
// it substitutes `1` or `0` and never the value. It refused anyway, and the
// two other spellings of the same question answered, in this shell, in one
// run (#4882).
//
// The name is **the tests' own** since #4909, where the last of
// `zsh/parameter`'s absent names became a view. Nothing in this dialect
// registers an absence any more, so a grid over its roster would be a grid
// with no rows in it, passing against a shell with the mechanism ripped out.
// See absentProbeParam.
func TestAskingWhetherAnUnimplementedParameterIsThere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the three spellings agree now",
			`print -r -- "[${+zshabsentprobe}]"; [[ -v zshabsentprobe ]]; print -r -- "[$?]"; print -r -- "[${zshabsentprobe+isthere}]"; print -r -- after`,
			"[0]\n[1]\n[]\nafter\n",
		},
		{
			"and the feature test a script actually writes",
			`(( ${+zshabsentprobe} )) && print -r -- would-use-it; print -r -- reached`,
			"reached\n",
		},
		// A subscript and an expansion flag leave the reading alone, which is
		// measured: `${+v[1]}` and `${(k)+v}` on an unset name are both `0`
		// in zsh 5.9.2.
		{"a subscript does not make it a read", `print -r -- "[${+zshabsentprobe[1]}]"; print -r -- after`, "[0]\nafter\n"},
		{"nor does a flag", `print -r -- "[${(k)+zshabsentprobe}]"; print -r -- after`, "[0]\nafter\n"},
		// The answer is a real one, from the same shell: a name that is there
		// answers 1 and one nothing has heard of answers 0.
		{
			"the control on either side",
			`print -r -- "[${+PATH}][${+zshabsentprobe}][${+neverheardof}]"`,
			"[1][0][0]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshAbsentProbe(t, t.TempDir(), tc.src)
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
		`print -r -- "[${+zshabsentprobe#a}]"`,
		`print -r -- "[${+zshabsentprobe%a}]"`,
		`print -r -- "[$zshabsentprobe]"`,
		`print -r -- "[${#zshabsentprobe}]"`,
		`print -r -- "[${zshabsentprobe[x]}]"`,
		`print -r -- "[${(k)zshabsentprobe}]"`,
	} {
		t.Run(src, func(t *testing.T) {
			out, st := runZshAbsentProbe(t, t.TempDir(), src+"\nprint -r -- unreached")
			if !strings.Contains(out, absentProbeParam+": parameter not implemented yet") {
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
