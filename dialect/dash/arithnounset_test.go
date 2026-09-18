// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// `set -u` does not reach arithmetic here either: a name an expression reads
// and nothing ever set is zero with the option on.
//
// The other holdout beside BusyBox ash, and the panel's POSIX-faithful member,
// so this is the answer the standard's own preset takes. Measured 2026-09-18
// against dash from a script file under `env -i`: `set -u; echo $((b))` with
// `b` unset prints `0` at status 0 and the script runs on (#3574).
func TestTheOptionDoesNotReachArithmetic(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set -u; echo "v=$((b)) ok"`, "v=0 ok"},
		{`set -u; a=1; echo "v=$((a+b)) ok"`, "v=1 ok"},
	} {
		if out, st := answersRun(t, tc.src); strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
	// The control: the option still refuses an ordinary expansion here.
	if out, st := answersRun(t, `set -u; echo "[$b]"`); st == 0 || strings.Contains(out, "[") {
		t.Errorf(`set -u; echo "[$b]" gave %q at %d, want a refusal`, out, st)
	}
}

// The axis, which this dialect takes from the standard's preset rather than
// naming itself — so this pins that nothing has moved it.
func TestTheArithmeticNounsetAxis(t *testing.T) {
	if got := dash.Semantics().ArithUnsetNameUnderNounsetIsRefused; got != interp.No {
		t.Errorf("ArithUnsetNameUnderNounsetIsRefused = %v, want no", got)
	}
}
