// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A value an **assignment prefix** put in this cell is this call's own, so a
// valueless `-n` adopts it.
//
// "A value the name already holds aims the reference" was gated on the binding
// not being fresh, which is right for the *caller's* value and wrong for a
// prefix's: the fresh binding a declaration takes is not empty when a prefix
// displaced something into it. So `r=tgt f` with `f() { declare -n r; }` left
// the reference unaimed here and a refusal that bash writes went unwritten.
//
// Measured 2026-09-24 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh` over
// script files, against bash 5.3.20 and bash 5.3.15, which agree:
//
//	f() { declare -n r; declare -p r; }; r=tgt f
//	  `declare -nx r="tgt"` — aimed at what the prefix held
//	f() { declare -n r; }; r=/ f
//	  `` declare: `/': invalid variable name for name reference ``
//	r=tgt; f() { declare -n r; declare -p r; }; f
//	  `declare -n r` — the caller's value is not this call's
//
// The third is the control, and it is why the binding being fresh was the
// wrong test on its own: both make it fresh, and only one is adopted (#4178).
func TestAPrefixValueIsAdoptedByAValuelessReferenceDeclaration(t *testing.T) {
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row `nameref11.sub` grades.
			"a prefix's bad value earns the refusal",
			"f() { declare -n r; }\nr=/ f",
			"`/': invalid variable name for name reference", "",
		},
		{
			"and a prefix's good value aims the reference",
			"f() { declare -n r; declare -p r; }\nr=tgt f",
			`declare -nx r="tgt"`, "",
		},
		{
			// The control that says it is the prefix and not the freshness.
			"the caller's value is not adopted",
			"r=tgt\nf() { declare -n r; declare -p r; }\nf",
			"declare -n r", `r="tgt"`,
		},
		{
			// The `local` spelling takes the same road — folded rather than
			// left behind, which is where this tree keeps growing a second
			// helper that misses the fix.
			"local -n adopts a prefix value too",
			"f() { local -n r; declare -p r; }\nr=tgt f",
			`declare -nx r="tgt"`, "",
		},
		{
			"and local -n earns the same refusal",
			"f() { local -n r; }\nr=/ f",
			"local: `/': invalid variable name for name reference", "",
		},
		{
			// A prefix on the declaration itself already worked and must
			// stay working.
			"a prefix on the declaration itself is unchanged",
			"r=/ declare -n r",
			"`/': invalid variable name for name reference", "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
			if tc.absent != "" && strings.Contains(out, tc.absent) {
				t.Errorf("= %q, want it not to contain %q", out, tc.absent)
			}
		})
	}
}
