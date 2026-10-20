// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// TestALocalThroughCommandDeclaresNothing: `command local a=1` declares
// nothing, assigns nothing and reports 0 here — the scope the prefix puts the
// declaration in is not the function's (#3370).
//
// Measured 2026-09-18 on dash 0.5.12, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, and the same in BusyBox v1.37.0. bash declares
// the local, which is what makes it a split.
//
// The last row is what says the builtin still runs rather than being skipped:
// this shell gives `local` no option letters, so `-x` is a name, and the
// refusal is the one a bare `local` gives for a name it cannot have.
func TestALocalThroughCommandDeclaresNothing(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
	}{
		{`f() { command local a=1; echo "[${a-UNSET}]"; }; a=outer; f; echo "[$a]"`, "[outer]\n[outer]"},
		{`f() { command local b; echo "[${b-UNSET}] st=$?"; }; f`, "[UNSET] st=0"},
		{`command local a=1; echo "[${a-UNSET}] st=$?"`, "[UNSET] st=0"},
		{`f() { local c=2; echo "[$c]"; }; f`, "[2]"},
		{`f() { command export d=1; echo "[$d]"; }; f`, "[1]"},
	} {
		if out, _ := runDash(t, t.TempDir(), tc.src); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s wrote %q, want %q", tc.src, out, tc.want)
		}
	}
	out, st := runDash(t, t.TempDir(), `f() { command local -x c=1; }; f`)
	if !strings.Contains(out, "local") || !strings.Contains(out, "-x: bad variable name") || st != 2 {
		t.Errorf("a bad name under the prefix wrote %q at %d, want the builtin's own refusal at 2", out, st)
	}
}
