// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `set -o` is `setopt` under a POSIX spelling, and not a table of its own
// (#1080).
//
// Measured 2026-09-06, zsh 5.9.2, `env -i` with `HOME`/`ZDOTDIR`/`HISTFILE`
// in a scratch directory:
//
//	set +o | wc -l                      185
//	set -o autocd; setopt | grep autocd  autocd
//	setopt autocd; set +o | grep autocd  set -o autocd
//	set -o Err_Exit; [[ -o errexit ]]    0
//
// This dialect wrote the substrate's shared 23 instead, in bash's vocabulary
// — `braceexpand`, `hashall`, `histexpand`, `nolog`, `notify`, `onecmd`,
// `physical` and `trackall` are eight names ours listed that zsh never
// writes, and 170 of zsh's were absent. Both exit 0 and neither says a word,
// which is what makes it the silent kind: `set +o` is a capture surface, and
// a caller reading ours recorded a zsh with 23 options and never learnt it
// had asked the wrong question.
//
// These cases grade a slice the case sets itself rather than the whole
// listing, for the reason every listing case here does — a table grades the
// build, and it grows.

// TestSetOListsThisDialectsOwnTable: the count and three names that could
// only come from zsh's table, against three that could only come from the
// substrate's.
func TestSetOListsThisDialectsOwnTable(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "set +o")
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if want := 185; len(lines) != want {
		t.Errorf("%d rows, want %d — zsh 5.9.2 writes that many", len(lines), want)
	}
	for _, want := range []string{"set +o autocd", "set +o autopushd", "set +o extendedglob"} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("stdout has no %q — the rows are not this dialect's table", want)
		}
	}
	// The eight the issue names, which are the substrate's vocabulary and
	// not zsh's output. Written as whole rows so a name that is a substring
	// of a real zsh option cannot pass by accident.
	for _, gone := range []string{
		"braceexpand", "hashall", "histexpand", "nolog",
		"notify", "onecmd", "physical", "trackall",
	} {
		for _, sign := range []string{"set -o ", "set +o "} {
			if strings.Contains(out, sign+gone+"\n") {
				t.Errorf("stdout has %q — zsh does not write that name", sign+gone)
			}
		}
	}
}

// TestSetOAndSetoptAreOneNamespace: written through one and read through the
// other, in both directions. This is what makes the listing above a fact
// about the shell rather than a second table that happens to be longer.
func TestSetOAndSetoptAreOneNamespace(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `set -o autocd; echo "st=$?"; setopt`)
	if !strings.Contains(out, "st=0\n") || !strings.Contains(out, "autocd\n") || st != 0 {
		t.Errorf("out %q status %d, want `set -o autocd` visible to a bare `setopt`", out, st)
	}

	// Read back with the shell's own words rather than through `grep`, which
	// the tests here have no PATH for.
	out, st = runZsh(t, t.TempDir(),
		`setopt autocd; v=$(set +o); case $v in *"set -o autocd"*) echo seen ;; *) echo missing ;; esac`)
	if !strings.Contains(out, "seen\n") || st != 0 {
		t.Errorf("out %q status %d, want `setopt autocd` visible to `set +o`", out, st)
	}
}

// TestSetOTakesThisDialectsSpellings: case folded, underscores ignored, one
// `no` prefix negating — the same normalization `setopt` does, because it is
// the same namespace. `set -o Err_Exit` is one question in zsh and an unknown
// name in every other shell.
func TestSetOTakesThisDialectsSpellings(t *testing.T) {
	for _, spelling := range []string{"Err_Exit", "ERREXIT", "e_r_r_e_x_i_t"} {
		out, st := runZsh(t, t.TempDir(), `set -o `+spelling+`; echo "st=$?"; [[ -o errexit ]]; echo "c=$?"`)
		if !strings.Contains(out, "st=0\n") || !strings.Contains(out, "c=0\n") || st != 0 {
			t.Errorf("set -o %s: out %q status %d, want it accepted and readable", spelling, out, st)
		}
	}
}

// TestSetORefusesTwoDifferentWays: a name zsh does not have, and a name it
// has that will not move — different sentences, both at 1, both ending the
// script, which is what `set` does here and `setopt` does not.
func TestSetORefusesTwoDifferentWays(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "set -o zzznosuch\necho after")
	if want := "zsh:set:1: no such option: zzznosuch\n"; out != want {
		t.Errorf("out = %q, want exactly %q — nothing after it", out, want)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}

	// `onecmd` is one of the twelve borrowed spellings, for `singlecommand`
	// — one of the five names about being interactive that a non-interactive
	// zsh refuses to move. Measured: real zsh says the same sentence at the
	// same status and stops there too.
	out, st = runZsh(t, t.TempDir(), "set -o onecmd\necho after")
	if want := "zsh:set:1: can't change option: onecmd\n"; out != want {
		t.Errorf("out = %q, want exactly %q — nothing after it", out, want)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// TestSetoptStillCarriesOnPastItsOwnRefusal: the same two refusals through
// the other spelling are *not* fatal, because `setopt` is not a special
// builtin. One namespace, two builtins, and only one of them stops a script.
func TestSetoptStillCarriesOnPastItsOwnRefusal(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "setopt zzznosuch\necho after")
	if !strings.Contains(out, "zsh:setopt:1: no such option: zzznosuch\n") ||
		!strings.Contains(out, "after\n") || st != 0 {
		t.Errorf("out %q status %d, want the complaint and then `after`", out, st)
	}
}

// TestTheEightBashNamesAreStillAccepted: the eight the issue names are gone
// from the *listing* and must not be gone from the *namespace* — seven of
// them are spellings zsh takes as input and never writes.
//
// Five now answer exactly what real zsh answers, where three of those five
// were `not implemented` at 2 before. The other two are the divergence
// docs/spec/semantics.md already records: `histexpand` and `physical` are
// zsh's `banghist` and `chaselinks`, two of the thirteen names whose state
// this shell reads through the substrate and cannot move, so it refuses
// rather than granting falsely. Real zsh grants both.
func TestTheEightBashNamesAreStillAccepted(t *testing.T) {
	for _, c := range []struct {
		name, want string
		status     int
	}{
		{"braceexpand", "st=0\n", 0},
		{"hashall", "st=0\n", 0},
		{"nolog", "st=0\n", 0},
		{"notify", "st=0\n", 0},
		{"trackall", "st=0\n", 0},

		{"histexpand", "zsh:set:1: can't change option: histexpand\n", 1},
		{"physical", "zsh:set:1: can't change option: physical\n", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), `set -o `+c.name+`; echo "st=$?"`)
			if strings.Contains(out, "no such option") {
				t.Errorf("out = %q, want the name still in the namespace", out)
			}
			if out != c.want || st != c.status {
				t.Errorf("out = %q status %d, want exactly %q at %d", out, st, c.want, c.status)
			}
		})
	}
}
