// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `compgen -A function` is the third caller of "whose function is this"
// (#1081), and the one #1035 could not reach: it generated from the exported
// Runner.FuncNames, which is every function callable and is also what the
// line editor completes from.
//
// Measured against real bash 5.3, `env -i` with `HOME`/`ZDOTDIR`/`HISTFILE`
// in a scratch directory. bash has the three as builtins, so a function
// listing there names only the person's:
//
//	f(){ :; }; compgen -A function      f, status 0
//	compgen -A function pu              nothing, status 1
//	compgen -A function pushd           nothing, status 1
//	pushd(){ :; }; compgen -A function pu   pushd, status 0
//
// The first row was the loud one and the middle two are the silent kind: a
// plausible list at status 0, and a prefix that had no match answered with
// one. Nothing here sets a new mark — a listing asks scriptFuncNames, which
// asks speaksForTheShell, which compares the declaration.

// TestAPreludeFunctionIsNotGeneratedAsACompletion: the listing, by byte, on
// both streams.
func TestAPreludeFunctionIsNotGeneratedAsACompletion(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "f() { echo f; }\ncompgen -A function\n", nil)
	if want := "f\n"; out != want {
		t.Errorf("stdout = %q, want exactly %q — the prelude's three are the shell's", out, want)
	}
	if errs != "" || st != 0 {
		t.Errorf("stderr %q status %d, want a silent 0", errs, st)
	}
	for _, name := range []string{"__helper", "p", "q"} {
		if strings.Contains(out, name) {
			t.Errorf("stdout = %q, want no mention of the prelude's %s", out, name)
		}
	}
}

// TestAPreludeOnlyShellGeneratesNoFunctionsAtAll: with nothing but the
// prelude defined there is nothing to offer, and `compgen` says so with 1
// rather than an empty 0 — a completer asks whether there is anything.
func TestAPreludeOnlyShellGeneratesNoFunctionsAtAll(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "compgen -A function\n", nil)
	if out != "" || errs != "" {
		t.Errorf("stdout %q stderr %q, want nothing written", out, errs)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1 — no matches is a failure here", st)
	}
}

// TestAPreludeFunctionIsNotAMatchForAPrefix: the silent row. A prefix only a
// prelude name has is "no matches", and the status is what a completer reads.
func TestAPreludeFunctionIsNotAMatchForAPrefix(t *testing.T) {
	for _, word := range []string{"p", "q", "__", "__helper"} {
		t.Run(word, func(t *testing.T) {
			out, errs, st := preludeListingRun(t, listingPrelude, "compgen -A function "+word+"\n", nil)
			if out != "" || errs != "" {
				t.Errorf("stdout %q stderr %q, want nothing written", out, errs)
			}
			if st != 1 {
				t.Errorf("status = %d, want 1", st)
			}
		})
	}
}

// TestARedefinedPreludeFunctionIsGeneratedAgain: the declaration comparison,
// through this caller. Real bash offers `pushd` here too, because there the
// person's function shadows the builtin and is a function; ours agrees for
// the same reason the listing does.
func TestARedefinedPreludeFunctionIsGeneratedAgain(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "p() { echo mine; }\ncompgen -A function p\n", nil)
	if want := "p\n"; out != want {
		t.Errorf("stdout = %q, want exactly %q — a redefinition is the script's own", out, want)
	}
	if errs != "" || st != 0 {
		t.Errorf("stderr %q status %d, want a silent 0", errs, st)
	}
}

// TestTheBuiltinListingIsUnchangedByAllOfThis: the half deliberately not
// moved. `compgen -A builtin` names the builtins and not the prelude's
// functions — real bash answers `compgen -A builtin pushd` with `pushd` and
// ours answers nothing at 1, which is the same divergence `type pushd`
// already is (#603): calling the name a builtin here would give the shell two
// answers to whether it is a function. A miss at 1 rather than a fabricated
// name, and it is recorded in docs/spec/semantics.md.
func TestTheBuiltinListingIsUnchangedByAllOfThis(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "compgen -A builtin p\n", nil)
	if strings.Contains(out, "\np\n") || strings.HasPrefix(out, "p\n") {
		t.Errorf("stdout = %q, want the prelude's `p` left out of the builtins", out)
	}
	if errs != "" {
		t.Errorf("stderr = %q, want nothing written", errs)
	}
	// `printf` and `pwd` are builtins and do begin with `p`, so this is a
	// real listing rather than an empty one — which is what makes the
	// assertion above about the prelude and not about the action failing.
	if !strings.Contains(out, "pwd\n") || st != 0 {
		t.Errorf("stdout %q status %d, want the builtins beginning with p at 0", out, st)
	}
}

// TestEveryFunctionIsStillCallableThroughTheExportedNames: the reason this is
// two sets and not one narrowing. FuncNames is what an embedder's line editor
// completes a command word from, so a prelude function has to be in it —
// narrowing it there is what would cost `pushd` its completion at a prompt.
func TestEveryFunctionIsStillCallableThroughTheExportedNames(t *testing.T) {
	r := preludeNamesRunner(t, listingPrelude, "f() { echo f; }\n")
	got := strings.Join(r.FuncNames(), " ")
	if want := "__helper f p q"; got != want {
		t.Errorf("FuncNames = %q, want %q — every function callable, the shell's included", got, want)
	}
}

// preludeNamesRunner installs pre as the dialect's own text, runs src on the
// same runner, and hands the runner back so its exported accessors can be
// asked directly.
func preludeNamesRunner(t *testing.T, pre, src string) *Runner {
	t.Helper()
	sem := permissive()
	var sink strings.Builder
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &sink, Stderr: &sink, Semantics: &sem, Diagnostics: &dg,
		Dir: t.TempDir(), Name: "testsh",
	})
	run := func(text string, prelude bool) {
		f, err := syntax.Parse(text, syntax.Core())
		if err != nil {
			t.Fatalf("parse %q: %v", text, err)
		}
		r.SourcingPrelude(prelude)
		if _, rerr := r.Run(context.Background(), f); rerr != nil {
			t.Fatalf("run %q: %v", text, rerr)
		}
		r.SourcingPrelude(false)
	}
	run(pre, true)
	run(src, false)
	return r
}
