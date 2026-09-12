// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// The order of the letters in `$-` is the dialect's, and it is a sequence
// rather than a rule: measured, the panel holds four different disciplines
// and one of them is a table nobody outside that shell can see. See
// Semantics.DollarDashLetterOrder — the values here are made up, because
// which real shell publishes which order is asserted in dialect/.
func TestDollarDashTakesTheOrderTheDialectPublishes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		order string
		want  string
	}{{
		name:  "no order is the order the substrate produces",
		order: "",
		want:  "[789Zaeu]",
	}, {
		// The two halves that matter: a letter the script set lands in
		// front of one the shell started with, and the startup letters
		// themselves are reordered among each other.
		name:  "the letters are placed and not appended",
		order: "au789eZ",
		want:  "[au789eZ]",
	}, {
		name:  "one the order does not name follows the ones it does",
		order: "Z98",
		want:  "[Z987aeu]",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			sem.DefaultOptionLetters = "789Z"
			sem.DollarDashLetterOrder = tc.order
			out, _ := run(t, `set -e; set -u; set -a; echo "[$-]"`,
				func(r *Runner) { r.Semantics = &sem })
			if out != tc.want+"\n" {
				t.Errorf("got %q, want %q", out, tc.want+"\n")
			}
		})
	}
}

// `set -f` in a shell that spends the letter on something other than globbing
// writes the option the dialect names, and `$-` reports that option's state
// rather than the fact the letter was written.
//
// Two claims in one test because they are one state: a shell where the letter
// wrote a flag of its own would pass the first two rows and fail the third,
// which reaches the name without ever writing the letter.
func TestTheFLetterMovesTheOptionTheDialectNames(t *testing.T) {
	for _, tc := range []struct {
		name    string
		snippet string
		want    string
	}{
		{"the letter turns the name on", `set -f; echo "[$-]"`, "[9f]"},
		{"and off again", `set -f; set +f; echo "[$-]"`, "[9]"},
		{"the name alone brings the letter", `set -o vi; echo "[$-]"`, "[9f]"},
		{"and losing the name takes it away", `set -f; set +o vi; echo "[$-]"`, "[9]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			sem.DefaultOptionLetters = "9"
			sem.SetFTurnsOffGlobbing = No
			// Any name this shell really holds will do: the axis is that
			// the letter and the name are one state, not which state.
			sem.SetFLetterOption = "vi"
			sem.DollarDashLetterOrder = "9f"
			out, _ := run(t, tc.snippet, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want+"\n" {
				t.Errorf("got %q, want %q", out, tc.want+"\n")
			}
		})
	}
}

// And a dialect that names nothing leaves `set -f` where it was: accepted,
// inert, and with no letter to show for it. The shell that spends `-f` on
// globbing never reaches this; the one being guarded against is a dialect
// that answers No to the globbing axis and has nothing else to say.
func TestTheFLetterIsInertWhereTheDialectNamesNothing(t *testing.T) {
	sem := CoreSemantics()
	sem.SetFTurnsOffGlobbing = No
	out, st := run(t, `set -f; echo "st=$? [$-]"`, func(r *Runner) { r.Semantics = &sem })
	if out != "st=0 []\n" || st != 0 {
		t.Errorf("got %q at %d, want an accepted no-op with no letter", out, st)
	}
}
