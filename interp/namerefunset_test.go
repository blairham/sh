// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// `unset` through a name reference asks the **target** whether it is frozen,
// because the target is where the unset lands. See
// Runner.frozenNameOfAnUnset, where the measurements are.
//
// The store followed the reference for the *delete* and the refusal was asked
// a dozen lines earlier against the name the script wrote, so the two halves
// read different names and both directions were wrong.
//
// It is the rule Runner.frozenNameOfAnAssignment already states for a write:
// a reference and what it points at are frozen separately, and only the second
// one can refuse what goes through it.
func TestUnsetThroughAReferenceAsksTheTargetsFreeze(t *testing.T) {
	// A **frozen reference** at an unfrozen name: the unset goes through and
	// takes the target, and the reference is still there aimed at it.
	out, st := runNameref(t, `x=1; typeset -rn RO=x
unset RO
echo "st=$? x=[${x-GONE}]"
typeset -p RO`)
	if !strings.Contains(out, "st=0 x=[GONE]") {
		t.Errorf("got %q at %d, want the target taken at 0", out, st)
	}
	if !strings.Contains(out, `declare -nr RO='x'`) {
		t.Errorf("got %q, want the frozen reference still aimed at x", out)
	}
	if strings.Contains(out, "readonly") {
		t.Errorf("got %q, want no refusal — the reference's freeze is not the target's", out)
	}

	// And the other way round: an unfrozen reference at a **frozen name** is
	// refused, naming the target, and the target survives. This is the half
	// that used to remove a frozen variable in silence.
	out, _ = runNameref(t, `y=1; readonly y; typeset -n RT=y
unset RT
echo "y=[${y-GONE}]"`)
	if !strings.Contains(out, "y: cannot unset: readonly variable") {
		t.Errorf("got %q, want the target's refusal, naming the target", out)
	}
	if strings.Contains(out, "y=[GONE]") {
		t.Errorf("got %q, want the frozen variable left standing", out)
	}

	// `unset -n` is the other half and is untouched: the letter names the
	// reference, so its own freeze refuses.
	out, _ = runNameref(t, `z=1; typeset -rn RZ=z; unset -n RZ; echo "z=[${z-GONE}]"`)
	if !strings.Contains(out, "RZ: cannot unset: readonly variable") {
		t.Errorf("got %q, want the reference's own freeze to refuse `unset -n`", out)
	}

	// The element aim, which the narrower redirect already covered and which
	// the wider one must not change: one element goes and the rest of the
	// array stays.
	out, _ = runNameref(t, `B=(p q); typeset -n e=B[1]; unset -v e; echo "[${B[*]}] n=${#B[@]}"`)
	if !strings.Contains(out, "[p] n=1") {
		t.Errorf("got %q, want B[1] alone taken away", out)
	}

	// And the ordinary shapes, which have to keep answering as they did:
	// a reference at a name, one aimed at nothing, and one aimed at a name
	// that was never set.
	for _, tc := range []struct{ name, src, want string }{
		{"a plain target", `w=1; typeset -n rw=w; unset rw; echo "st=$? w=[${w-GONE}]"`, "st=0 w=[GONE]"},
		{"nothing to point at", `typeset -n u; unset u; echo "st=$?"`, "st=0"},
		{"a target nothing set", `typeset -n v=nosuch; unset v; echo "st=$?"`, "st=0"},
	} {
		out, _ := runNameref(t, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: got %q, want %q", tc.name, out, tc.want)
		}
	}
}
