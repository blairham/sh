// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// TestALocalBadNameWithAValueWaitsForTheReturn: taken in silence, refused when
// the call returns, fatally; without a value it is refused on the spot.
// Measured 2026-10-03 in the pinned image. See
// interp.Semantics.LocalBadNameWithAValueIsRefusedAtTheReturn.
func TestALocalBadNameWithAValueWaitsForTheReturn(t *testing.T) {
	out, st := run(t, "f() {\n  local a=1 1x=5 b=2\n  echo \"in=$? $a $b\"\n}\nf\necho after\n")
	if !strings.HasPrefix(out, "in=0 1 2\n") || !strings.HasSuffix(out, "1x: bad variable name\n") ||
		strings.Contains(out, "local:") || strings.Contains(out, "after") || st != 2 {
		t.Errorf("got %q at %d, want the body run, then the refusal unlocated by the builtin, at 2", out, st)
	}
	out, st = run(t, "f() { local 1x; echo in; }\nf\necho after\n")
	if strings.Contains(out, "in\n") || !strings.Contains(out, "local: ") || st != 2 {
		t.Errorf("got %q at %d, want `local:` refusing on the spot at 2", out, st)
	}
}
