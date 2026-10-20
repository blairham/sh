// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A subscripted write over a reference with nothing to point at is the
// ordinary unaimed *use* refusal here, and it ends the script.
//
// This column was wrong the same way bash's was and in its own words: the
// subscript was thrown away and the value aimed the reference. Measured
// 2026-09-24 against ksh93u+ 2012-08-01, `env -i` from a script file:
//
//	typeset -n ref; ref[0]=foo    `ref: no reference name`, and nothing
//	                              after it runs
//
// where `typeset -n ref; ref=foo` aims the reference as it always did (#4178).
func TestASubscriptedWriteOverAnUnaimedReferenceIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			"the unaimed use refusal is written",
			"typeset -n ref\nref[0]=foo",
			"ref: no reference name", "",
		},
		{
			"and nothing after it runs",
			"typeset -n ref\nref[0]=foo\necho after",
			"ref: no reference name", "after",
		},
		{
			"a plain write still aims it",
			"typeset -n ref\nref=foo\ntypeset -p ref",
			"typeset -n ref=foo", "no reference name",
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
