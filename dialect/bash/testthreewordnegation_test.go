// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A three-word `test` whose middle word is a connective reads the connective
// **before** a leading `!`, which is the other answer to
// Semantics.TestThreeWordsNegateBeforeAConnective.
//
// The probe that tells the two readings apart is a `-a` whose operand is a
// file that **exists**. This shell has a unary `-a`, so `[ -a / ]` is 0 and
// negating it would give 1; reading `-a` as the connective over the two
// non-empty strings `!` and `/` gives 0. A probe naming a file that is not
// there answers 0 either way and says nothing — which is why `[ ! -a x ]`
// alone cannot pin this column.
//
// Measured 2026-09-19 on bash 5.3.20 at /opt/homebrew, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on /dev/null.
// zsh 5.9.2 answers 0 as well and reaches it the other way — it has no unary
// `-a` at all, so negating first would refuse. dash 0.5.12 and BusyBox ash
// refuse `[ ! -a / ]`, and ksh93u+ answers 1, which is `[ -a / ]` negated
// (#3717).
func TestAThreeWordTestReadsTheConnectiveBeforeALeadingNegation(t *testing.T) {
	for _, c := range []struct {
		src    string
		status int
	}{
		// The discriminating pair: the unary reading of the last two words,
		// and the same words behind a `!`.
		{`[ -a / ]`, 0},
		{`[ ! -a / ]`, 0},
		{`[ ! -o / ]`, 0},
		// The same shape over a file that is not there, which both readings
		// answer alike and which is here to say so.
		{`[ -a /nonesuch ]`, 1},
		{`[ ! -a /nonesuch ]`, 0},
		// The binary reading still comes first, and the guard is unmoved.
		{`[ ! = x ]`, 1},
		{`[ x -a y ]`, 0},
		{`[ x -a "" ]`, 1},
		{`[ "" -o x ]`, 0},
		// Four words, where the leading `!` negates the three behind it.
		{`[ ! x -a y ]`, 1},
	} {
		t.Run(c.src, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src+"\n")
			if err != nil {
				t.Fatalf("run %q: %v", c.src, err)
			}
			if st != c.status {
				t.Errorf("%s = %d, want %d (output %q)", c.src, st, c.status, out)
			}
			if out != "" {
				t.Errorf("%s said %q, want nothing", c.src, out)
			}
		})
	}
}
