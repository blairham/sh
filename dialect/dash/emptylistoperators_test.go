// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// A `+` that fires on an empty positional list is one empty field here.
// Measured 2026-10-03 on dash.
func TestAFiredAlternateOnNoParametersIsOneField(t *testing.T) {
	if out, _ := runDash(t, t.TempDir(), `set --; set -- "${@:+w}"; echo $#`); out != "1\n" {
		t.Errorf("got %q, want %q", out, "1\n")
	}
}
