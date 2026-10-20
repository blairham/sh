// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// A line continuation between a `$` and what it introduces stops the `$` at
// every form but a bare parameter *inside double quotes* here, and at nothing
// outside them. Measured 2026-09-16 on zsh 5.9.2 from script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, stdin on /dev/null, with `x=5` and
// `set -- a b`:
//
//	echo "[$\⏎x]"           [5]          a bare parameter is not stopped
//	echo "[$\⏎1]"           [a]          nor is a positional one
//	echo "[$\⏎{x}]"         [{x}]        stopped, and the `$` goes with it
//	echo "[$\⏎(echo hi)]"   [$(echo hi)] stopped, and the `$` stays
//	echo "[$\⏎[1+2]]"       [$[1+2]]     stopped, and the `$` stays
//	printf "[%s]\n" $\⏎{x}  [5]          outside quotes nothing is stopped
//	printf "[%s]\n" $\⏎[1+2]  [3]
//
// The brace is the row the extra flag exists for: the other shell that stops
// at one answers `${x}` from the same stop, so "which forms are stopped" and
// "what becomes of the `$`" are two questions and not one.
func TestAContinuationStopsADollarAtEveryFormButANameInDoubleQuotes(t *testing.T) {
	d := zsh.Dialect()
	if got := d.ContinuationStopsADollarAt; got != syntax.NoDollarForm {
		t.Errorf("ContinuationStopsADollarAt = %b, want no form", got)
	}
	if got, want := d.ContinuationStopsADollarAtInDoubleQuotes,
		syntax.EveryDollarForm&^syntax.DollarBareParameter; got != want {
		t.Errorf("ContinuationStopsADollarAtInDoubleQuotes = %b, want %b", got, want)
	}
	if !d.DollarGoesWhenAContinuationStopsItAtABrace {
		t.Error("zsh drops the `$` where it stops at a brace")
	}

	for _, tc := range []struct{ src, want string }{
		{"x=5; echo \"[$\\\nx]\"", "[5]\n"},
		{"set -- a b; echo \"[$\\\n1]\"", "[a]\n"},
		{"x=5; echo \"[$\\\n{x}]\"", "[{x}]\n"},
		{"echo \"[$\\\n(echo hi)]\"", "[$(echo hi)]\n"},
		{"echo \"[$\\\n[1+2]]\"", "[$[1+2]]\n"},
		{"x=5; printf '[%s]\\n' $\\\n{x}", "[5]\n"},
		{"printf '[%s]\\n' $\\\n[1+2]", "[3]\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil || out != tc.want {
			t.Errorf("%q: out = %q, err = %v, want %q", tc.src, out, err, strings.TrimSpace(tc.want))
		}
	}
}
