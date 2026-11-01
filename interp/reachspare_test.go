// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sync"
	"testing"

	"github.com/blairham/sh/syntax"
)

// TestASharedPatternsScratchIsNeverHeldTwice runs the theme's substitution
// from many goroutines at once. The end analysis's scratch is handed back to
// the pattern's automaton after each expansion and taken by the next (#6380);
// two substitutions writing one scratch would garble each other's ends, and
// the race detector sees the writes. Every run has to give the nine X real
// zsh gives.
func TestASharedPatternsScratchIsNeverHeldTwice(t *testing.T) {
	t.Parallel()
	e := &syntax.ParamExpr{All: true}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				o := benchOpts(t, benchPattern, benchSubject)
				got := replace(benchSubject, benchPattern, e, o, armOrder{}, benchDeclined,
					func(matchReport, string) string { return "X" })
				if got != "XXXXXXXXX" {
					t.Errorf("the substitution gave %q", got)
					return
				}
			}
		})
	}
	wg.Wait()
}
