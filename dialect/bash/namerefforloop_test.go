// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A `for` loop whose variable is a reference aimed at its **own name** writes
// *through* the reference, where one aimed at another name is re-pointed.
//
// The re-point is already the rule and is right: `v=1; typeset -n r=v; for r in
// x y` leaves `declare -n r="y"` with `v` untouched, because a loop variable is
// an assignment and an assignment to a reference aims it. A reference aimed at
// its own name is the one shape that rule cannot reach — there is no other name
// to aim it at — and an assignment to it resolves outward to the cell the
// binding stands in front of. The loop re-pointed it anyway, so `$ref` read
// empty on every pass and the caller's value never changed: a write that went
// nowhere at status 0.
//
// Measured 2026-09-24 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh` over
// script files, against bash 5.3.20 and bash 5.3.15, which agree, with an outer
// `ref=B`:
//
//	f() { typeset -n ref=ref; for ref in X Y; do printf '<%s>' "$ref"; done; }
//	  bash     `<X><Y>`, the outer `ref` left holding `Y`, and per iteration
//	           the write's `maximum nameref depth (8) exceeded` and then the
//	           read's `circular name reference`
//	  before   `<><>`, the outer `ref` still `B`, and no warning at all
//
// ksh93u+ never reaches any of this: it refuses `typeset -n ref=ref` outright
// with `invalid self reference`, so there is no axis here (#4178).
func TestALoopVariableAimedAtItsOwnNameWritesThrough(t *testing.T) {
	for _, tc := range []struct{ name, src, want, absent string }{
		{
			// The row: the value reaches the cell the binding stands in
			// front of, and a read inside the loop sees it.
			"the value reaches the outer cell",
			// stderr is sent away: this row is about the value, and the
			// harness merges the two streams so the warnings would land
			// between the fields.
			"ref=B\nf() { typeset -n ref=ref; for ref in X Y; do printf '<%s>' \"$ref\"; done; }\nf 2>/dev/null\nprintf '[%s]' \"$ref\"",
			"<X><Y>[Y]", "",
		},
		{
			// The write's sentence, once per iteration — and it is the
			// write's, not the read's.
			"and says the write's sentence per iteration",
			"ref=B\nf() { typeset -n ref=ref; for ref in X Y; do :; done; }\nf 2>&1 | grep -c 'maximum nameref depth'",
			"2", "",
		},
		{
			// The reference is still a reference afterwards: writing through
			// is not re-pointing, so nothing about the binding changed.
			"the reference is still aimed at itself",
			"ref=B\nf() { typeset -n ref=ref; for ref in X; do declare -p ref; done; }\nf 2>/dev/null",
			`declare -n ref="ref"`,
			"",
		},
		{
			// **A word that is no name is a value here, not a refusal** —
			// the other half of not being a re-aim.
			"a word that is no name is written through",
			"ref=B\nf() { typeset -n ref=ref; for ref in /; do echo body; done; printf 'st=%d' $?; }\nf 2>/dev/null\nprintf '[%s]' \"$ref\"",
			"body\nst=0[/]", "not a valid identifier",
		},
		{
			// Nowhere outside the reference for the value to land: the loop
			// gives up, the body never runs, and the reference is left alone.
			// This is the cost the store leaves to its caller.
			"a reference that is the outermost cell gives up the loop",
			"ref=B\nf() { declare -gn ref=ref; for ref in X; do echo body; done; printf 'st=%d' $?; }\nf 2>/dev/null\nprintf '[%s]' \"$(declare -p ref)\"",
			"st=1[declare -n ref=\"ref\"]", "body",
		},
		{
			// The control that says the re-point rule is untouched: another
			// name is still re-pointed and the target is left alone.
			"a reference aimed elsewhere is still re-pointed",
			"v=V\nf() { typeset -n r=v; for r in X; do :; done; declare -p r v; }\nf",
			"declare -n r=\"X\"\ndeclare -- v=\"V\"", "",
		},
		{
			// And an empty list writes nothing at all, so the reference and
			// the outer value both stand.
			"an empty list writes nothing",
			"ref=B\nf() { typeset -n ref=ref; for ref in; do :; done; }\nf\nprintf '[%s]' \"$ref\"",
			"[B]", "maximum nameref depth",
		},
		{
			// A plain name is a plain loop, which this must not have moved.
			"a plain loop variable is unchanged",
			"for p in X Y; do printf '<%s>' \"$p\"; done\nprintf '[%s]' \"$p\"",
			"<X><Y>[Y]", "warning",
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
