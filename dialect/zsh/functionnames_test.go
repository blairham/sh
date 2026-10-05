// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// functionNamesRunner is a zsh runner with output captured, and a run helper
// that parses with this dialect's grammar.
func functionNamesRunner(t *testing.T) (*interp.Runner, *strings.Builder, func(string)) {
	t.Helper()
	sem, diag, dl := Semantics(), Diagnostics(), Dialect()
	var out strings.Builder
	dir := t.TempDir()
	r := &interp.Runner{
		Stdout: &out, Stderr: &out,
		Semantics: &sem, Diagnostics: &diag, Dialect: &dl,
		Dir: dir, Name: "zsh", Vars: map[string]string{"PATH": dir},
	}
	Apply(r)
	run := func(src string) {
		t.Helper()
		f, err := syntax.Parse(src, Dialect())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
	}
	return r, &out, run
}

// The keys-only reading of `$functions` has to be the view's keys exactly —
// SetDynamicAssocKeys is a shorter route to the same answer, not a second
// opinion — across the three kinds of name the view treats differently: a
// defined function, one waiting to be autoloaded (whose value is a stub text
// rather than a rendering) and one that has been removed again.
func TestTheFunctionNamesAreTheViewsKeys(t *testing.T) {
	r, out, run := functionNamesRunner(t)
	run("f(){ echo a; if true; then :; fi }\ng(){ :; }\ngone(){ :; }\nunset -f gone\nautoload -Uz waiting\n")
	if out.Len() > 0 {
		t.Fatalf("setting up wrote %q", out.String())
	}
	var viewKeys []string
	for k := range zshFunctionsView(r) {
		viewKeys = append(viewKeys, k)
	}
	slices.Sort(viewKeys)
	names := zshFunctionNames(r)
	if !slices.Equal(names, viewKeys) {
		t.Errorf("zshFunctionNames = %q, the view's keys = %q", names, viewKeys)
	}
	// The control: the view has to hold the pending name at all, or the row
	// above compares two lists that both left it out.
	if !slices.Contains(viewKeys, "waiting") || !slices.Contains(viewKeys, "f") {
		t.Errorf("the view's keys %q lack a defined or a pending function", viewKeys)
	}
}

// Reading the names renders no body. This is a performance property written
// as a correctness test, because it is invisible in what the script reads and
// expensive in fact: on the maintainer's real configuration (2026-10-05) zi's
// `.zi-diff-functions` read `${(qk)functions[@]}` around each plugin it
// loaded, and each read rendered every function in the shell — about 8% of
// the CPU of an interactive start, for six calls. The counter is the
// assertion, for the reason TestAKeyedProducerAnswersWithoutBuildingTheTable
// gives: an answer that happens to be right does not say which route made it.
func TestReadingTheFunctionNamesRendersNoBody(t *testing.T) {
	r, out, run := functionNamesRunner(t)
	whole := 0
	view := r.DynamicAssocs["functions"]
	if view == nil {
		t.Fatal("no whole-table producer registered for functions")
	}
	r.DynamicAssocs["functions"] = func(r *interp.Runner) interp.AssocArray {
		whole++
		return view(r)
	}
	run("b(){ :; }\na(){ echo x; }\n" +
		`print -r -- "[${(k)functions}]"` + "\n" +
		`print -r -- "[${(j: :)${(qk)functions[@]}}]"` + "\n" +
		`for n in ${(k)functions}; do print -rn -- "<$n>"; done; print` + "\n")
	if got, want := out.String(), "[a b]\n[a b]\n<a><b>\n"; got != want {
		t.Errorf("the names = %q, want %q", got, want)
	}
	if whole != 0 {
		t.Errorf("reading the names produced the whole table %d times, want 0", whole)
	}
	// The control: a read that needs the values still goes through the
	// wrapper, so a zero above is the keys route and not a wrapper nothing
	// calls.
	run(`: "${(kv)functions}"` + "\n")
	if whole == 0 {
		t.Error("`${(kv)functions}` did not reach the whole-table producer: the count above proves nothing")
	}
}
