// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"testing"
)

// Single quotes in an associative key stop what they hold: a substitution
// written there is never performed, on a read or on a store, so
// `${h['$(: >marker)']}` makes no file. Measured 2026-10-03 on bash 5.3.20
// under `-c` (#5569, a regression from #5561, whose reading is zsh's alone).
// The bare substitution is the control: it does run, so the marker can be
// made by this harness.
func TestAnApostropheQuotedKeyRunsNothing(t *testing.T) {
	for _, c := range []struct {
		src  string
		made bool
	}{
		{`typeset -A h; h[k]=v; : "${h['$(: >marker)']}"`, false},
		{`typeset -A h; h[k]=v; n=0; : "${h['$((n+=1))']}"; [ $n = 0 ] || : >marker`, false},
		{`typeset -A h; h['$(: >marker)']=1`, false},
		{`typeset -A h; h[k]=v; : "${h[$(: >marker)]}"`, true},
	} {
		dir := t.TempDir()
		runBash(t, dir, c.src)
		_, err := os.Stat(filepath.Join(dir, "marker"))
		if made := err == nil; made != c.made {
			t.Errorf("%s: marker made %v, want %v", c.src, made, c.made)
		}
	}
}
