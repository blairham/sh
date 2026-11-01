// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package testenv_test

import (
	"slices"
	"testing"

	"github.com/blairham/sh/internal/testenv"
)

// TestAChildIsToldNotToPauseAtExit pins the environment a re-executed child is
// handed: under -race, one GORACE entry carrying whatever the caller set with
// the pause taken away after it, so the caller's options survive and the pause
// cannot be put back by one of them; without the detector, the environment as
// it was given.
func TestAChildIsToldNotToPauseAtExit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		env      []string
		withRace []string
	}{
		{
			"nothing set",
			[]string{"PATH=/bin", "SH_TEST_X=1"},
			[]string{"PATH=/bin", "SH_TEST_X=1", "GORACE=atexit_sleep_ms=0"},
		},
		{
			"options of the caller's",
			[]string{"GORACE=halt_on_error=1", "PATH=/bin"},
			[]string{"PATH=/bin", "GORACE=halt_on_error=1 atexit_sleep_ms=0"},
		},
		{
			"a pause the caller asked for, twice over",
			[]string{"GORACE=atexit_sleep_ms=500", "PATH=/bin", "GORACE=history_size=3"},
			[]string{"PATH=/bin", "GORACE=atexit_sleep_ms=500 history_size=3 atexit_sleep_ms=0"},
		},
		{
			"an empty one",
			[]string{"GORACE=", "PATH=/bin"},
			[]string{"PATH=/bin", "GORACE=atexit_sleep_ms=0"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			given := slices.Clone(tc.env)
			got := testenv.WithoutRaceExitPause(given)
			want := tc.env
			if testenv.RaceDetector {
				want = tc.withRace
			}
			if !slices.Equal(got, want) {
				t.Errorf("got %q, want %q", got, want)
			}
			if !slices.Equal(given, tc.env) {
				t.Errorf("the caller's environment was changed to %q", given)
			}
		})
	}
}
