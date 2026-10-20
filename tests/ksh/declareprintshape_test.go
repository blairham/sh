// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// `-p` picks the *shape* as well as the set, which is the half of the
// filtered listing that only shows outside bash.
//
// Measured 2026-09-15 on ksh93u+ 2012-08-01 and on zsh 5.9.2, which agree
// against bash here: a bare `typeset -i` writes `n=1` and `typeset -p -i`
// writes `typeset -i n=1`. bash writes its own `declare -i n="1"` for both,
// so a filter routed through the bare listing would have looked right there
// and written the wrong form in two columns of three.
func TestDashPKeepsTheReissuableShapeWhenAnAttributeLetterFiltersIt(t *testing.T) {
	const setup = "typeset -i n=1; plain=2; "
	for _, c := range []struct {
		name string
		src  string
		want string
		deny string
	}{{
		"the bare listing is the value alone",
		setup + "typeset -i",
		"n=1",
		"typeset -i n=1",
	}, {
		"and -p beside the letter is the declaration",
		setup + "typeset -p -i",
		"typeset -i n=1",
		"plain",
	}, {
		"written as one word too",
		setup + "typeset -pi",
		"typeset -i n=1",
		"plain",
	}} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKshWithPrelude(t, c.src)
			if st != 0 {
				t.Fatalf("status %d: %q", st, out)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("got %q, want a line %q", out, c.want)
			}
			if c.deny != "" && strings.Contains(out, c.deny) {
				t.Errorf("got %q, which should not carry %q", out, c.deny)
			}
		})
	}
}
