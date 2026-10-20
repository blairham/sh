// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestACaseArmsAlternativesAreListedSpaced: `declare -f` writes a blank on
// each side of the `|` between an arm's alternatives, with or without the
// arm's parenthesis. Measured 2026-10-02 on bash 5.3.20 (#5138).
func TestACaseArmsAlternativesAreListedSpaced(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `f() { case $1 in a|b) :;; (c|d) :;; esac; }; declare -f f`)
	for _, want := range []string{"        a | b)\n", "        c | d)\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("listing %q lacks %q", out, want)
		}
	}
}
