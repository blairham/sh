// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"slices"
	"testing"
)

// TestTheNamesAnUnloadTakesAreTheModuleOwnOnes is the roster `zmodload -u`
// withdraws, asserted as a roster.
//
// It is asserted here rather than only through the shell because the
// difference it guards is **not observable from a script today**: `WATCHFMT`
// and `LOGCHECK` are stored parameters, and
// [interp.Runner.SetParameterWithdrawn] reaches producers — so a rule that
// withdrew them would be a no-op and `${+WATCHFMT}` would read 1 either way.
// A row that cannot fail is not a control, and this is the one that can: the
// rule is about *which names*, so the names are what it reads (#5025).
func TestTheNamesAnUnloadTakesAreTheModuleOwnOnes(t *testing.T) {
	for _, tc := range []struct {
		module string
		want   []string
	}{
		{"zsh/langinfo", []string{"langinfo"}},
		{"zsh/mapfile", []string{"mapfile"}},
		{"zsh/system", []string{"errnos", "sysparams"}},
		{"zsh/datetime", []string{"EPOCHSECONDS", "EPOCHREALTIME", "epochtime"}},
		// **The control.** The module is in zshGatedParameters for
		// `WATCHFMT` and `LOGCHECK`, and declares `p:WATCH` and `p:watch` in
		// zmodloadFeatures — two disjoint lists, so it owns neither pair and
		// the unload takes nothing. Measured: all four read `${+…}` of 1
		// after `zmodload zsh/watch; zmodload -u zsh/watch` in zsh 5.9.2.
		{"zsh/watch", nil},
		// And a module with no gated parameters at all, which is most of
		// them: the roster is empty rather than every `p:` feature it names.
		{"zsh/parameter", nil},
		{"zsh/zutil", nil},
	} {
		t.Run(tc.module, func(t *testing.T) {
			got := gatedParametersTheModuleOwns(tc.module)
			if !slices.Equal(got, tc.want) {
				t.Errorf("= %v, want %v", got, tc.want)
			}
		})
	}
}

// And the names it does **not** take are still names this shell gates, which
// is what keeps the row above from being read as "zsh/watch has no gated
// parameters". It has two; they are simply not its to take back.
func TestTheWatchModuleStillGatesItsPair(t *testing.T) {
	for _, name := range []string{"WATCHFMT", "LOGCHECK"} {
		if got := zshGatedParameterModule(name); got != "zsh/watch" {
			t.Errorf("%s waits for %q, want zsh/watch", name, got)
		}
		if slices.Contains(gatedParametersTheModuleOwns("zsh/watch"), name) {
			t.Errorf("%s is in the unload's roster, where the reference keeps it", name)
		}
	}
}
