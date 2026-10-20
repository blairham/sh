// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// A subshell's options are the subshell's, through every route that reads or
// writes them (#1855).
//
// This dialect installs its option namespace once, on the shell the front end
// built, and a subshell is a clone of that shell — so the three seams have to
// be handed the runner they are answering *about*. Measured on zsh 5.9.2:
// every kind of option read the spawning shell here, which is what said the
// registration was the cause rather than any one entry's storage.
//
// The five rows are the five kinds this dialect has behind one namespace: an
// option it only records, two it answers from a semantics axis, one a matcher
// reads, and one backed by a substrate `set -o` name. They are asserted
// together because a fix to one entry's storage would move one row.
func TestASubshellsOptionsAreReadInTheSubshell(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			"a recorded option",
			`(setopt correct; [[ -o correct ]]; echo "st=$?")`,
			"st=0\n",
		},
		{
			"an axis this dialect turns on",
			`(setopt shwordsplit; [[ -o shwordsplit ]]; echo "st=$?")`,
			"st=0\n",
		},
		{
			// The other direction of an axis, and the row that says the
			// answer is read rather than defaulted: unsetopt in the subshell
			// has to be visible to the subshell's own condition.
			"an axis this dialect turns off",
			`(unsetopt multios; [[ -o multios ]]; echo "st=$?")`,
			"st=1\n",
		},
		{
			"an option the pattern matcher reads",
			`(setopt nullglob; [[ -o nullglob ]]; echo "st=$?")`,
			"st=0\n",
		},
		{
			// The one where a wrong answer is not only a wrong answer. With
			// errexit held, a condition that answers false ends the shell it
			// is in — so this row used to print nothing at all rather than
			// printing the wrong status, which is a control-flow difference
			// and not a reporting one.
			"an option backed by a substrate name, under errexit",
			`(set -e; [[ -o errexit ]]; echo "st=$?")`,
			"st=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The same namespace by its other name: `set -o` reaches this dialect's table
// too, so a listing inside a subshell is the subshell's listing and a
// `set -o name` inside one does not escape.
//
// Measured on zsh 5.9.2: the `set +o` capture written inside `(setopt autocd;
// … )` holds `set -o autocd`, and `(set -o autocd)` applies inside and leaves
// the outer shell without it. Ours wrote `set +o autocd` for the first, and
// for the second left the subshell unchanged and turned the option on in the
// *outer* shell — the same registration, read and written.
func TestASubshellsOwnListingAndMove(t *testing.T) {
	t.Run("the listing is the subshell's", func(t *testing.T) {
		const src = `out=$( (setopt autocd; set +o) )
case $out in
(*"set -o autocd"*) echo listing=on ;;
(*"set +o autocd"*) echo listing=off ;;
(*) echo listing=absent ;;
esac
`
		if out, _ := runZsh(t, t.TempDir(), src); out != "listing=on\n" {
			t.Errorf("got %q, want %q", out, "listing=on\n")
		}
	})
	t.Run("the move does not escape", func(t *testing.T) {
		const src = `inner=$( (set -o autocd; setopt) )
outer=$(setopt)
case $inner in (*autocd*) echo inner=on ;; (*) echo inner=off ;; esac
case $outer in (*autocd*) echo outer=on ;; (*) echo outer=off ;; esac
`
		if out, _ := runZsh(t, t.TempDir(), src); out != "inner=on\nouter=off\n" {
			t.Errorf("got %q, want %q", out, "inner=on\nouter=off\n")
		}
	})
}
