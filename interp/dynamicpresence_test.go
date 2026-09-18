// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A produced parameter that is not *there* yet.
//
// A producer returns a string, so it can answer "empty" and cannot answer
// "unset" — and the two are a difference a script can see. Before this seam a
// dialect with a parameter that arrives partway through a run had no way to
// say so, and the only spelling available was an empty value, which takes the
// opposite branch of every operator that separates them.

func TestAProducedParameterCanBeAbsentUntilAPredicateSaysOtherwise(t *testing.T) {
	const probe = `printf '[%s][%s][%s]' "$P" "${P-UNSET}" "${P+SET}"`
	for _, w := range []struct {
		name    string
		present bool
		want    string
	}{
		{"before the predicate says so", false, "[][UNSET][]"},
		{"and after", true, "[made][made][SET]"},
	} {
		t.Run(w.name, func(t *testing.T) {
			out, st := run(t, probe, func(r *Runner) {
				r.SetDynamic("P", func(*Runner) string { return "made" })
				r.SetDynamicPresence("P", func(*Runner) bool { return w.present })
			})
			if out != w.want || st != 0 {
				t.Errorf("got %q/%d, want %q/0", out, st, w.want)
			}
		})
	}
}

// A producer with no predicate beside it is there from the moment it is
// registered, which is what every other produced parameter in this tree is.
func TestAProducedParameterWithNoPredicateIsAlwaysThere(t *testing.T) {
	out, st := run(t, `printf '[%s][%s]' "${P-UNSET}" "${P+SET}"`, func(r *Runner) {
		r.SetDynamic("P", func(*Runner) string { return "made" })
	})
	if out != "[made][SET]" || st != 0 {
		t.Errorf("got %q/%d, want %q/0", out, st, "[made][SET]")
	}
}

// And `set -u` sees the absence, which is the half a script relies on: the
// option exists to stop exactly this read.
func TestNounsetStopsForAnAbsentProducedParameter(t *testing.T) {
	out, st := run(t, `set -u; printf '[%s]' "$P"; printf after`, func(r *Runner) {
		sem := permissive()
		r.Semantics = &sem
		r.SetDynamic("P", func(*Runner) string { return "made" })
		r.SetDynamicPresence("P", func(*Runner) bool { return false })
	})
	if st == 0 {
		t.Errorf("the read should have stopped the script, got %q/%d", out, st)
	}
	if strings.Contains(out, "after") {
		t.Errorf("nothing after the read should run, got %q", out)
	}
}

// Whether this shell has *ever* been inside a function call, which the call
// depth cannot answer: it is 0 both before the first call and after the last
// one returns, and one dialect's call-depth parameter is unset in the first
// state and set in the second.
func TestHasEnteredAFunctionIsNotTheCallDepth(t *testing.T) {
	const probe = `printf '[%s]' "$P"; f() { printf '[%s]' "$P"; }; f; printf '[%s]' "$P"`
	out, st := run(t, probe, func(r *Runner) {
		r.SetDynamic("P", func(rr *Runner) string {
			if rr.HasEnteredAFunction() {
				return "yes"
			}
			return "no"
		})
	})
	if want := "[no][yes][yes]"; out != want || st != 0 {
		t.Errorf("got %q/%d, want %q/0", out, st, want)
	}
}
