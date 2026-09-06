// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"slices"
	"testing"

	"github.com/blairham/sh/syntax"
)

// The line editor completes a command word from every function callable, and
// a dialect written as shell has `dirs`, `popd` and `pushd` among them — so
// narrowing the accessor it reads would take those three off Tab, silently,
// at a prompt where nothing else offers them either (`BuiltinNames` does not
// name them).
//
// That is why #1081 was two sets rather than one filter: `compgen -A function`
// asks the script's own functions and this asks everything callable. The test
// is here rather than in `interp` because this is the caller that would pay,
// and a change made over there cannot see it.
func TestTabStillOffersAFunctionTheDialectDefined(t *testing.T) {
	r := newTestRunner(nil)
	f, err := syntax.Parse("pushd() { :; }\nmine() { :; }\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	// As a front end installs one: the dialect's own text, not the script's.
	r.SourcingPrelude(true)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	r.SourcingPrelude(false)

	names := runnerCompleter{r: r}.names()
	for _, want := range []string{"pushd", "mine"} {
		if !slices.Contains(names, want) {
			t.Errorf("names() = %v, want %q offered — a prelude function is still a command a person can type", names, want)
		}
	}
	// And the completion a person would actually see.
	got := runnerCompleter{r: r}.Complete(Completion{
		Line: "pus", Point: 3, Start: 0, Word: "pus", Command: true,
	})
	if len(got) != 1 || got[0] != "pushd" {
		t.Errorf("Complete(%q) = %q, want just %q", "pus", got, "pushd")
	}
}
