// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `banghist` is on in a fresh shell, and it shows on four surfaces.
//
// Measured 2026-09-13 against zsh 5.9.2 with `-f` on both sides. It was
// recorded here as *this* shell's habit rather than zsh's, which is the defect
// #2542 is about — and the tree already contradicted itself: parameter.go has
// needed it to start on since #1527, because its `unset "options[name]"`
// measurement moves `equals` and `banghist` *off* to show an unset is a move
// and not a reset. An option already off could not have demonstrated that.
func TestBangHistIsOnInAFreshShell(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the condition", "[[ -o banghist ]] && print -r -- on || print -r -- off\n", "on\n"},
		{
			// The alias moves with it, which is what says the two spellings
			// are one option rather than two entries that happen to agree.
			"the alias", "[[ -o histexpand ]] && print -r -- on || print -r -- off\n", "on\n",
		},
		{
			"the options parameter",
			"zmodload zsh/parameter\nprint -r -- \"${options[banghist]}\"\n", "on\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// And the listing says it the way zsh says it: an option that is on has its
// *negation* named among the unset ones.
//
// This is the row that separates a real correction from moving a deviation
// somewhere else. Both shells are internally consistent — each listing agrees
// with its own `[[ -o ]]` — so changing the recorded default moves the
// condition and the listing together rather than trading one disagreement for
// another. `hashcmds` is the entry that fails this test and is why it was left
// alone.
func TestBangHistIsListedTheWayZshListsIt(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "unsetopt\n")
	if st != 0 {
		t.Fatalf("status %d", st)
	}
	if !strings.Contains(out, "nobanghist") {
		t.Errorf("the unset listing has no `nobanghist`; an option that is on is named by its negation there")
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "banghist" {
			t.Error("`banghist` is listed as unset, which is the state this corrects")
		}
	}
}

// Turning it off still works, and puts every surface back together.
//
// The option is recorded rather than acted on — nothing reads it to decide
// whether to expand a `!` — so what has to hold is that the four views agree
// with each other however it is moved, not that anything behaves differently.
func TestBangHistCanStillBeTurnedOff(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"unsetopt banghist\n"+
			"[[ -o banghist ]] && print -r -- on || print -r -- off\n"+
			"zmodload zsh/parameter\nprint -r -- \"${options[banghist]}\"\n")
	if want := "off\noff\n"; out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
}
