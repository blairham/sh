// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// The third column with a `%a`, and it agrees with bash rather than with the
// shell whose twelve-digit default it does not share: the zero fill goes
// between the `0x` and the digits. Measured 2026-09-17 against dash 0.5.12
// under LC_ALL=C (#3089).
func TestAHexFloatZeroFillGoesInsideThePrefix(t *testing.T) {
	out, st := answersRun(t, `printf '[%030a]' 1.5`)
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if want := "[0x00000000000000000000001.8p+0]"; !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
}
