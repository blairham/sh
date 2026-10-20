// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A local tie's halves do not inherit the export attribute of the names they
// shadow (#5098).
//
// The shadow a declaration takes deliberately leaves that one attribute
// alone, because whether a local inherits it is the dialect's answer and
// `localExportAttribute` asks it. The ordinary declaration loop asks; the tie
// path did not — so a tie declared over an **exported global of the same
// name** kept the attribute on the half that shadowed it.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device.
//
// **The exported global is the ingredient**, and it is why this took a while
// to reduce: every shape without one agrees, and did before the fix. The rows
// below carry the pair with and without it.
func TestATiesHalvesDoNotInheritTheExportTheyShadow(t *testing.T) {
	const tie = "outer=(i n)\n" + `print -r -- "${(t)OUTER} / ${(t)outer}"`
	dir := t.TempDir()
	for _, tc := range []struct{ name, pre, letters, want string }{
		{
			"the array half over an exported global of its name",
			"export outer=old", "-xT", "scalar-local-tied-export / array-local-tied",
		},
		{
			"and with no letter at all",
			"export outer=old", "-T", "scalar-local-tied / array-local-tied",
		},
		{
			"the scalar half over an exported global of its name",
			"export OUTER=old", "-T", "scalar-local-tied / array-local-tied",
		},
		// The letter is the scalar's own, so it keeps the attribute where
		// the array half does not — that pair is the rule stated twice.
		{
			"the scalar keeps the letter it was written",
			"export OUTER=old", "-xT", "scalar-local-tied-export / array-local-tied",
		},
		{
			"both names exported",
			"export OUTER=old outer=old2", "-xT", "scalar-local-tied-export / array-local-tied",
		},
		// And with nothing exported in the way, which is every shape that
		// already agreed.
		{"nothing to inherit", "", "-xT", "scalar-local-tied-export / array-local-tied"},
		{"nor with no letter", "", "-T", "scalar-local-tied / array-local-tied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.pre + "\nf(){ local " + tc.letters + " OUTER outer; " + tie + " }\nf\n"
			if out, st := runZsh(t, dir, src); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
	// `typeset` and `declare` shadow too, under the letter that leaves them
	// a scope: `-xT` makes them global here (#5095), so there is nothing for
	// them to inherit from and the rows would be about that instead.
	for _, word := range []string{"typeset", "declare"} {
		t.Run(word+" shadows the same way", func(t *testing.T) {
			src := "export outer=old\nf(){ " + word + " -T OUTER outer; " + tie + " }\nf\n"
			const want = "scalar-local-tied / array-local-tied\n"
			if out, st := runZsh(t, dir, src); out != want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, want)
			}
		})
		// And under the letter they take no scope at all, so the exported
		// global simply *is* the name and both halves read as exported —
		// which is the row that says this fix is about the shadow and not
		// about the letter.
		t.Run(word+" is global under the letter", func(t *testing.T) {
			src := "export outer=old\nf(){ " + word + " -xT OUTER outer; " + tie + " }\nf\n"
			const want = "scalar-tied-export / array-tied-export\n"
			if out, st := runZsh(t, dir, src); out != want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, want)
			}
		})
	}
}

// The same fact as the listing writes it, which is the form the suite asks
// for and the one that named this issue.
func TestTheListingOfATieOverAnExportedGlobal(t *testing.T) {
	const tie = "outer=(i n)\ntypeset -p OUTER outer"
	dir := t.TempDir()
	for _, tc := range []struct{ name, pre, want string }{
		{
			"over an exported global",
			"export outer=old",
			"local -xT OUTER outer=( i n )\ntypeset -aT OUTER outer=( i n )",
		},
		{
			"and with none",
			"",
			"local -xT OUTER outer=( i n )\ntypeset -aT OUTER outer=( i n )",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.pre + "\nf(){ local -xT OUTER outer; " + tie + " }\nf\n"
			if out, st := runZsh(t, dir, src); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
	// At the top level there is no shadow and nothing to inherit, so the
	// exported global simply *is* the name — which is the row that says the
	// fix is about the shadow and not about the letter.
	t.Run("at the top level the global keeps its own", func(t *testing.T) {
		src := "export outer=old\ntypeset -xT OUTER outer\n" + tie + "\n"
		const want = "export -T OUTER outer=( i n )\ntypeset -axT OUTER outer=( i n )"
		if out, st := runZsh(t, dir, src); out != want+"\n" || st != 0 {
			t.Errorf("out %q status %d, want %q at 0", out, st, want+"\n")
		}
	})
}

// And the ordinary declarations over the same exported global, which are what
// say the shadow is right in general and only the tie path was not asking.
func TestAnOrdinaryLocalOverAnExportedGlobalWasAlreadyRight(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a scalar", "export v=old\nf(){ local v=new; " + `print -r -- "${(t)v}"` + " }\nf", "scalar-local"},
		{"an array", "export a=old\nf(){ local -a a=(i n); " + `print -r -- "${(t)a}"` + " }\nf", "array-local"},
		{
			"one that writes the letter itself",
			"export a=old\nf(){ local -xa a=(i n); " + `print -r -- "${(t)a}"` + " }\nf", "array-local-export",
		},
		{"and under the other word", "export v=old\nf(){ typeset v=new; " + `print -r -- "${(t)v}"` + " }\nf", "scalar-local"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src+"\n"); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}
