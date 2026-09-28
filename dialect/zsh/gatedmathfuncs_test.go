// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A math function does not exist until `zsh/mathfunc` is loaded.
//
// The **`f:` third** of the same gate: #4922 did the parameters, #5029 the
// builtins, and both left this standing — `zmodloadEnforce` said in so many
// words that an `f:` feature is left alone, which was true while the
// forty-seven stood from startup and there was nothing for a selection to
// take.
//
// Every `want` is the reference's own answer, measured 2026-09-28 on
// `/opt/homebrew/bin/zsh` — zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go
// version -m` says *not a Go executable* for it — `-f` from a script file
// under `env -i PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME` and stdin
// at `/dev/null` (#5060).
//
// **Every name is asked in both states**, which is the point rather than
// symmetry: a row run only with the module loaded cannot show the gate, and a
// row run only without it cannot show that the function still works. The
// second column below is what makes the first one evidence.
func TestAMathFunctionWaitsForItsModule(t *testing.T) {
	for _, tc := range []struct {
		fn, arg, loaded string
	}{
		{"sqrt", "4", "2.\n"},
		{"floor", "4", "4.\n"},
		{"ceil", "4", "4.\n"},
		{"fabs", "4", "4.\n"},
		// The four that answer with an integer rather than a float, so the
		// gate is not being read off the shape of the result.
		{"abs", "4", "4\n"},
		{"int", "4", "4\n"},
		{"ilogb", "8", "3\n"},
	} {
		t.Run(tc.fn, func(t *testing.T) {
			// Absent before the load, by name, at 1.
			out, st := runZsh(t, t.TempDir(),
				"print $(( "+tc.fn+"("+tc.arg+") ))")
			if want := "zsh:1: unknown function: " + tc.fn + "\n"; out != want || st != 1 {
				t.Errorf("before the load = %q (status %d), want %q at 1", out, st, want)
			}
			// And there after it, answering — which is the control that says
			// the function itself is right and only the gate was missing.
			out, st = runZsh(t, t.TempDir(),
				"zmodload zsh/mathfunc\nprint $(( "+tc.fn+"("+tc.arg+") ))")
			if out != tc.loaded || st != 0 {
				t.Errorf("after the load = %q (status %d), want %q", out, st, tc.loaded)
			}
		})
	}
}

// The sentence is the one a name nobody registered gets, which is what says
// the withdrawal is the right shape and not a third diagnostic.
func TestAWithdrawnMathFunctionAnswersLikeAnUnknownOne(t *testing.T) {
	dir := t.TempDir()
	gated, _ := runZsh(t, dir, "print $(( sqrt(4) ))")
	unknown, _ := runZsh(t, dir, "print $(( nosuchmf(1) ))")
	if gated != "zsh:1: unknown function: sqrt\n" {
		t.Errorf("a gated name = %q", gated)
	}
	if unknown != "zsh:1: unknown function: nosuchmf\n" {
		t.Errorf("a name nothing has = %q", unknown)
	}
}

// TestTheGateDoesNotCloseOnTheModuleThatOpensIt is the trap #4922 records for
// parameters and #5029 for builtins, met a third time.
//
// `zmodload` loads a module only when this shell has every feature it names,
// and a withdrawn function is exactly what that test would read as missing.
// Without the split in KnownMathFunction, withdrawing the forty-seven at
// startup makes the load refuse with `not implemented yet` — the gate firing
// at the very names the module is about to bring.
func TestTheGateDoesNotCloseOnTheModuleThatOpensIt(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zmodload zsh/mathfunc\nprint st=$?\nzmodload -e zsh/mathfunc\nprint e=$?")
	if want := "st=0\ne=0\n"; out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
}

// An unload takes them back, which is #5025's rule reaching the third kind,
// and a load after it brings them again.
func TestUnloadingTheModuleTakesTheFunctionsBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zmodload zsh/mathfunc\nzmodload -u zsh/mathfunc\nprint $(( sqrt(4) ))")
	if want := "zsh:3: unknown function: sqrt\n"; out != want || st != 1 {
		t.Errorf("after the unload = %q (status %d), want %q at 1", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(),
		"zmodload zsh/mathfunc\nzmodload -u zsh/mathfunc\nzmodload zsh/mathfunc\n"+
			"print $(( sqrt(4) ))")
	if want := "2.\n"; out != want || st != 0 {
		t.Errorf("after the reload = %q (status %d), want %q", out, st, want)
	}
}

// And a narrowed selection brings only what it named, which is #5045's
// transition rule reaching the third kind.
//
// The deselection row is the one that separates the two readings: an
// installer keyed on the *module* would put every function back and could not
// see the `-f:sqrt`.
func TestANarrowedSelectionMovesOneFunction(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the one it named answers",
			"zmodload -F zsh/mathfunc f:sqrt\nprint $(( sqrt(4) ))",
			"2.\n",
		},
		{
			"and the one it did not, does not",
			"zmodload -F zsh/mathfunc f:sqrt\nprint $(( floor(4) ))",
			"zsh:2: unknown function: floor\n",
		},
		{
			"a deselection after a whole load",
			"zmodload zsh/mathfunc\nzmodload -F zsh/mathfunc -f:sqrt\nprint $(( sqrt(4) ))",
			"zsh:3: unknown function: sqrt\n",
		},
		{
			"and leaves its neighbors standing",
			"zmodload zsh/mathfunc\nzmodload -F zsh/mathfunc -f:sqrt\nprint $(( floor(4) ))",
			"4.\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want {
				t.Errorf("= %q, want %q", out, tc.want)
			}
		})
	}
}

// The control that keeps this from being read as "arithmetic lost its
// functions": a name a script registers with `functions -M` is not this
// module's and is never withdrawn.
func TestAScriptsOwnMathFunctionIsUntouched(t *testing.T) {
	// The value is the reference's, measured rather than assumed: `mf(1)` is
	// **1** in zsh 5.9.2 and here alike. An earlier draft asserted 9 — what
	// the function writes to REPLY — and the row failed, which is the whole
	// reason a `want` is a measurement and not a guess.
	out, st := runZsh(t, t.TempDir(),
		"mf(){ REPLY=9; }\nfunctions -M mf\nprint $(( mf(1) ))")
	if want := "1\n"; out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
}
