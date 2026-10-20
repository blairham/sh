// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// `lithist` and `histappend`, which were held states and are switches.
//
// The two names bash's own `shopt.tests` was refused on, and the reason the
// file disagreed three times over for each of them: this shell recorded both
// **on**, so the `-p` listing, the `-s` listing and the on/off table each said
// something bash's defaults do not, and the `shopt -u` the file runs to put a
// known shell in front of itself was refused out loud (#4149).
//
// They are asserted here rather than beside `cmdhist` because the claim is a
// different one. A held state says "this shell is already in the state the
// name asks for"; a switch says "the name moves something, and the default is
// bash's". What each one moves is in interp — see
// interp.Runner.HistoryJoinsATypedCommand and RewritesTheHistoryFile for the
// measured tables — and what it does with what it moved is graded in repl,
// which is where a history is kept.
func TestTheTwoHistoryNamesAreSwitchesAtBashsDefault(t *testing.T) {
	for _, name := range []string{"lithist", "histappend"} {
		t.Run(name, func(t *testing.T) {
			// Off before anything asks, which is bash's own default and the
			// half this table used to have backwards. Status 1 goes with it:
			// a query about an option that is off is a failed test.
			offWant := fmt.Sprintf("%-20s\toff\n", name)
			if out, st := runBash(t, t.TempDir(), "shopt "+name); st != 1 || out != offWant {
				t.Errorf("shopt %s = %q status %d, want %q status 1", name, out, st, offWant)
			}
			// And in the off half of both listings, reissuably and not.
			out, _ := runBash(t, t.TempDir(), "shopt -u")
			if !strings.Contains(out, offWant) {
				t.Errorf("shopt -u listing lacks %q", offWant)
			}
			if out, st := runBash(t, t.TempDir(), "shopt -p "+name); st != 1 || out != "shopt -u "+name+"\n" {
				t.Errorf("shopt -p %s = %q status %d", name, out, st)
			}
			// Both directions move, quietly, which is what a switch is. The
			// second `shopt -u` is the one that used to be a refusal.
			for _, src := range []string{"shopt -s " + name, "shopt -u " + name} {
				if out, st := runBash(t, t.TempDir(), src); st != 0 || out != "" {
					t.Errorf("%s = %q status %d, want a quiet success", src, out, st)
				}
			}
			onWant := fmt.Sprintf("%-20s\ton\n", name)
			if out, st := runBash(t, t.TempDir(), "shopt -s "+name+"; shopt "+name); st != 0 || out != onWant {
				t.Errorf("shopt %s after -s = %q status %d, want %q status 0", name, out, st, onWant)
			}
			// And back, which a bit that only ever went one way would pass
			// the line above and fail here.
			if out, st := runBash(t, t.TempDir(),
				"shopt -s "+name+"; shopt -u "+name+"; shopt "+name); st != 1 || out != offWant {
				t.Errorf("shopt %s after -s -u = %q status %d, want %q status 1", name, out, st, offWant)
			}
		})
	}
}

// And the states themselves, read off the Runner rather than off the listing.
//
// The listing can agree with bash while nothing behind it moves — that is
// exactly what a recorded state is — so this asks the core what it holds.
// Both runs are on one runner, which is what makes the second half an
// assertion at all: the same shell is asked to move the name and then to move
// it back.
//
// The sense inverts at the option name, which is the shape
// `no_empty_cmd_completion` and `globskipdots` already have here: `lithist`
// off is a shell that joins, and `histappend` off is a shell that rewrites.
func TestTheTwoHistoryNamesMoveTheCoresOwnState(t *testing.T) {
	for _, c := range []struct {
		name string
		get  func(*interp.Runner) bool
	}{
		{"lithist", (*interp.Runner).HistoryJoinsATypedCommand},
		{"histappend", (*interp.Runner).RewritesTheHistoryFile},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := preset.Runner(dialecttest.Base{Dir: t.TempDir()})
			// The preset's own default, which is the state bash is in with
			// the option off.
			if !c.get(r) {
				t.Fatalf("the preset left the core on the option's own side")
			}
			for _, step := range []struct {
				src  string
				want bool
			}{{"shopt -s " + c.name, false}, {"shopt -u " + c.name, true}} {
				if st, err := r.Run(t.Context(), preset.Parse(t, step.src)); err != nil || st != 0 {
					t.Fatalf("%s: status %d, %v", step.src, st, err)
				}
				if got := c.get(r); got != step.want {
					t.Errorf("after %s the core holds %v, want %v", step.src, got, step.want)
				}
			}
		})
	}
}
