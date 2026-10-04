// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// TestAFiredPlusOverNoParametersIsAnEmptyField: `"${@:+x}"` with nothing set
// is one empty field, where `"$@"` is none. Measured 2026-10-03 on dash
// 0.5.12. See interp.Semantics.AFiredAlternateOnAnEmptyListIsOneField.
func TestAFiredPlusOverNoParametersIsAnEmptyField(t *testing.T) {
	src := `n() { echo "$#"; }; set --; n "${@:+x}"; n "$@"; set -- a; n "${@:+x}"`
	if out, _ := answersRun(t, src); out != "1\n0\n1\n" {
		t.Errorf("got %q, want 1, 0, 1", out)
	}
}
