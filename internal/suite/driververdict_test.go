// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import "testing"

// The two driver-verdict counters differ on one shape, and that shape is the
// whole question "how much of this suite cannot be graded here".
//
// [Report.StrictOnFailure] was read as answering it and cannot: its
// denominator is the strict files, and a file whose reference *failed* is by
// that fact a file the two shells rarely match byte for byte. Asked per file
// it returns `0/0` — on `V06parameter`, whose reference demonstrably fails at
// its own first chunk, as readily as on a file with nothing wrong. **A
// vacuous denominator reads exactly like a clean result**, and on the
// strength of that reading `docs/spec/zsh-suite.md` subtracted twenty chunks
// across nine files from the target. Measured with the counter below, it is
// two files (#5203).
func TestTheTwoDriverVerdictCountersPartOnAFailingFile(t *testing.T) {
	for _, tc := range []struct {
		name             string
		verdict          bool
		res              Result
		wantStrict, want int
	}{
		// The shape that parts them, and the reason this function exists.
		{"failed and not strict", true, Result{RefStatus: 1}, 0, 1},
		// Where they agree, which is why the old counter looked right.
		{"failed and strict", true, Result{RefStatus: 1, Strict: true}, 1, 1},
		{"ran cleanly", true, Result{RefStatus: 0, Strict: true}, 0, 0},
		// A suite that states no verdict of its own says nothing either way:
		// elsewhere a non-zero status is the file's own result.
		{"no verdict, failed", false, Result{RefStatus: 1}, 0, 0},
		{"no verdict, failed and strict", false, Result{RefStatus: 1, Strict: true}, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotStrict, got := driverVerdicts(tc.verdict, tc.res)
			if gotStrict != tc.wantStrict || got != tc.want {
				t.Errorf("driverVerdicts(%v, %+v) = (%d, %d), want (%d, %d)",
					tc.verdict, tc.res, gotStrict, got, tc.wantStrict, tc.want)
			}
		})
	}
}

// And the property that makes the new counter worth having: it is never
// smaller than the old one, so a column can print both without the pair
// reading as a contradiction.
func TestTheWiderVerdictCounterIsNeverTheSmaller(t *testing.T) {
	for _, verdict := range []bool{true, false} {
		for _, strict := range []bool{true, false} {
			for _, st := range []int{0, 1, 2, -1} {
				strictOnly, wider := driverVerdicts(verdict, Result{RefStatus: st, Strict: strict})
				if wider < strictOnly {
					t.Errorf("verdict=%v strict=%v status=%d: wider %d < strict-only %d",
						verdict, strict, st, wider, strictOnly)
				}
			}
		}
	}
}
