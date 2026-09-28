// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The three shapes of a parenthesized `case` pattern list this shell reads as
// one word, and the neighboring reading that is the control.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, run `-f` over a script file under `set -n` with
// the option moved on the line before the `case`, 2026-09-27. Every row's
// answer is the reference's.
var caseListRows = []struct {
	name string
	src  string
	// narrowed is what the reference does with `shglob` on. Every row but the
	// last is accepted with it off.
	narrowed bool
}{
	{"a blank inside the list", "case 'a b' in (a b) echo m;; esac\n", true},
	{"a newline inside the list", "case a in (a\n|b) echo m;; esac\n", true},
	{"the whole list written as nothing", "case '' in ( ) echo em;; (*) echo star;; esac\n", true},
	// The row above was called "an alternative written as nothing" until
	// #4817, and the name was the bug: `( )` is a *list* with nothing in it,
	// and the four rows below are alternatives written as nothing, which the
	// option does not move. Measured in the same run, accepted with the
	// option on and off alike — and the last of them is the one that says so
	// loudest, holding two empty alternatives and no list at all.
	{"a trailing empty alternative", "case a in (a|b|) echo t;; esac\n", false},
	{"a leading empty alternative", "case '' in (|https|git) echo e;; esac\n", false},
	{"an empty alternative in the middle", "case a in (a||b) echo e;; esac\n", false},
	{"nothing but empty alternatives", "case a in (|) echo e;; esac\n", false},
	// The control, and the reason this option is keyed on the pattern list
	// rather than on "the header is read the standard's way": the `;` in a
	// header is the neighboring zsh-alone reading and the option does not
	// move it. Measured in the same run — accepted with the option on and
	// off alike. Without a row that holds still, three rows moving together
	// say nothing about what moved them.
	{"a separator in the header", "case x; in a) :;; esac\n", false},
}

func caseListRunner(t *testing.T) *interp.Runner {
	t.Helper()
	r := optionStateRunner(t)
	if r.Dialect == nil {
		t.Fatal("the runner has no dialect, so there is no grammar to move")
	}
	return r
}

func parsesHere(t *testing.T, r *interp.Runner, src string) bool {
	t.Helper()
	_, err := syntax.Parse(src, *r.Dialect)
	return err == nil
}

// TestTheOptionNarrowsTheCasePatternList is the option's own row: it reaches
// the grammar rather than being remembered, and it reaches it both ways.
func TestTheOptionNarrowsTheCasePatternList(t *testing.T) {
	for _, row := range caseListRows {
		t.Run(row.name, func(t *testing.T) {
			r := caseListRunner(t)
			if !parsesHere(t, r, row.src) {
				t.Fatal("refused before the option moved, so the row cannot show a narrowing")
			}
			if code := setOption(r, "shglob", true); code != 0 {
				t.Fatalf("turning the option on answered %d", code)
			}
			if got := !parsesHere(t, r, row.src); got != row.narrowed {
				t.Errorf("with the option on the line is refused=%v, want %v", got, row.narrowed)
			}
			if code := setOption(r, "shglob", false); code != 0 {
				t.Fatalf("turning the option off answered %d", code)
			}
			if !parsesHere(t, r, row.src) {
				t.Error("the line is still refused after the option went back off, so it only narrows")
			}
		})
	}
}

// TestAnEmulationNarrowsTheCasePatternListThroughTheOption is the emulation's
// row, and the two rows after it are what say **the option decides and the
// mode does not**.
//
// A table keyed on the mode would answer every one of the first four rows
// correctly and be wrong about the last two, which is the shape a grid keyed
// on the wrong noun has: breadth along the mode reads exactly like
// confirmation until something holds the mode fixed and moves the option.
func TestAnEmulationNarrowsTheCasePatternListThroughTheOption(t *testing.T) {
	// The emulation's own defaults, measured in the same run: `shglob` is on
	// under `sh` and `ksh` and off under `zsh` and `csh`.
	for _, tc := range []struct {
		mode     string
		narrowed bool
	}{
		{"zsh", false},
		{"sh", true},
		{"ksh", true},
		{"csh", false},
	} {
		t.Run("emulate "+tc.mode, func(t *testing.T) {
			r := caseListRunner(t)
			applyEmulation(r, tc.mode, false)
			for _, row := range caseListRows {
				want := row.narrowed && tc.narrowed
				if got := !parsesHere(t, r, row.src); got != want {
					t.Errorf("%s: refused=%v, want %v", row.name, got, want)
				}
			}
		})
	}

	// The mode held fixed at zsh's own and the option moved: the narrowing
	// follows the option.
	t.Run("the option narrows inside the shell's own mode", func(t *testing.T) {
		r := caseListRunner(t)
		applyEmulation(r, "zsh", false)
		if code := setOption(r, "shglob", true); code != 0 {
			t.Fatalf("turning the option on answered %d", code)
		}
		if parsesHere(t, r, caseListRows[0].src) {
			t.Error("the list is still read as one word, so the mode is what decides and not the option")
		}
	})

	// And the other way round: the mode held at `sh` and the option turned
	// off puts the reading back.
	t.Run("the option widens inside a narrowed mode", func(t *testing.T) {
		r := caseListRunner(t)
		applyEmulation(r, "sh", false)
		if code := setOption(r, "shglob", false); code != 0 {
			t.Fatalf("turning the option off answered %d", code)
		}
		if !parsesHere(t, r, caseListRows[0].src) {
			t.Error("the list is still refused, so the mode is what decides and not the option")
		}
	})
}
