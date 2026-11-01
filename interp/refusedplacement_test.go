// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An array literal whose element is refused where it would go asks whether the
// elements placed in front of it stay, and only then: Yes keeps them and starts
// the name over first, No places nothing and leaves a name that held an array
// holding it. See Semantics.RefusedPlacementKeepsWhatTheLiteralPlaced (#6852).
func TestARefusedPlacementAsksWhetherTheLiteralKeepsWhatItPlaced(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"kept", Yes, "[1]"},
		{"nothing placed", No, "[7]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, stmt := range []string{"b=(1 [-5]=x 3)", "b=(1 [1+]=x 3)", "b=(1 [$(true)]=x 3)"} {
				src := "b=(7)\n" + stmt + "\n" + `echo "[${b[*]}]"` + "\n"
				out, _ := runGrammar(t, src, nil, func(r *Runner) {
					giveUpLineSemantics(r)
					r.Semantics.EmptySubscriptInAnIndexedLiteralIsBad = Yes
					r.Semantics.RefusedPlacementKeepsWhatTheLiteralPlaced = tc.answer
				})
				if !strings.Contains(out, tc.want+"\n") {
					t.Errorf("%s: output %q, want %s", stmt, out, tc.want)
				}
			}
		})
	}
}

// The question is put only once a placement has been refused, so a literal
// that places asks nothing and a vector that leaves it open still runs one.
func TestALiteralThatPlacesAsksNothingAboutARefusal(t *testing.T) {
	t.Parallel()
	src := "b=(7)\nb=(1 [3]=x 4)\n" + `echo "${b[*]}"` + "\n"
	out, st := runGrammar(t, src, nil, func(r *Runner) {
		giveUpLineSemantics(r)
		r.Semantics.RefusedPlacementKeepsWhatTheLiteralPlaced = Unspecified
	})
	if out != "1 x 4\n" || st != 0 {
		t.Errorf("output %q status %d, want the literal placed and nothing asked", out, st)
	}
}
