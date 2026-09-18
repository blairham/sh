// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// `set -u` does not reach arithmetic here: a name an expression reads and
// nothing ever set is zero with the option on, exactly as it is with the
// option off.
//
// One of two holdouts in the panel — dash is the other, and they are the two
// POSIX-minimal members: 2.5.2 writes `set -u` about parameter *expansion* and
// 2.6.4 hands arithmetic to the ISO C integer expressions without saying the
// option reaches them. Measured 2026-09-18 against BusyBox v1.37.0 through the
// container route the oracle reaches this shell by: `set -u; echo $((b))` with
// `b` unset prints `0` at status 0 and the script runs on, where bash, ksh93
// and zsh all refuse it (#3574).
func TestTheOptionDoesNotReachArithmetic(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set -u; echo "v=$((b)) ok"`, "v=0 ok"},
		{`set -u; a=1; echo "v=$((a+b)) ok"`, "v=1 ok"},
		{`set -u; echo "v=$(( (b) * 2 )) ok"`, "v=0 ok"},
	} {
		if out, st := run(t, tc.src); strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
	// The control: the option is on and still refuses an ordinary expansion,
	// so what is being measured is its reach and not whether it works here.
	if out, st := run(t, `set -u; echo "[$b]"`); st == 0 || strings.Contains(out, "[") {
		t.Errorf(`set -u; echo "[$b]" gave %q at %d, want a refusal`, out, st)
	}
}

// The axis. ArithNounsetRefusalIsFatal is deliberately unanswered beside it:
// with no refusal raised here there is nothing for it to be about.
func TestTheArithmeticNounsetAxis(t *testing.T) {
	if got := ash.Semantics().ArithUnsetNameUnderNounsetIsRefused; got != interp.No {
		t.Errorf("ArithUnsetNameUnderNounsetIsRefused = %v, want no", got)
	}
}
