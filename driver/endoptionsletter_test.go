// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"
)

// The letter that ends the option reading: every word after the one it stands
// in is an operand, the way every word after `--` is.
//
// It is the front end's because what it does is to the invocation and not to
// the shell — nothing is turned on or off, and the same letter written at
// `set` is a bad option in the dialect that has this. See
// Semantics.EndOfOptionsInvocationLetter, where the panel is.
func TestTheEndOfOptionsLetterMakesEveryLaterWordAnOperand(t *testing.T) {
	script := writeScript(t, "echo RAN\n")
	for _, tc := range []struct {
		name   string
		letter string
		argv   []string
		out    string
		errs   string
		code   int
	}{
		// The word after it is an operand whatever it looks like, so the
		// command-string letter is a path the shell cannot open.
		{
			name: "the next word is an operand and not an option", letter: "b",
			argv: []string{"testsh", "-b", "-c", "echo RAN"},
			errs: "-c", code: 127,
		},
		// Wherever it sits among the words, not only first.
		{
			name: "it stops the reading from where it stands", letter: "b",
			argv: []string{"testsh", "-x", "-b", "-c", "echo RAN"},
			errs: "-c", code: 127,
		},
		// Both signs carry it: it turns nothing on, so there is nothing for
		// a sign to say.
		{
			name: "the plus spelling ends it too", letter: "b",
			argv: []string{"testsh", "+b", "-c", "echo RAN"},
			errs: "-c", code: 127,
		},
		// The rest of the *word* is still option letters, which is the half
		// that makes this end the loop over words rather than the loop over
		// letters.
		{
			name: "the rest of its own word is still options", letter: "b",
			argv: []string{"testsh", "-bc", "echo RAN"},
			out:  "RAN\n",
		},
		// And a letter that takes the next word still takes it, before the
		// reading stops.
		{
			name: "a letter after it still reaches forward", letter: "b",
			argv: []string{"testsh", "-bo", "xtrace", "-c", "echo RAN"},
			errs: "-c", code: 127,
		},
		// A script path after it is an operand, which is the ordinary
		// reading and is what says the stop is about options rather than
		// about operands.
		{
			name: "a script after it still runs", letter: "b",
			argv: []string{"testsh", "-b", script},
			out:  "RAN\n",
		},
		// The control: with no letter named, the same word is a `set`
		// option's letter and the reading goes on.
		{
			name: "with no letter named it is an ordinary option letter", letter: "",
			argv: []string{"testsh", "-b", "-c", "echo RAN"},
			errs: "-b", code: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := shell()
			sh.Semantics.EndOfOptionsInvocationLetter = tc.letter
			out, errs, code := runArgs(t, sh, tc.argv...)
			if code != tc.code {
				t.Fatalf("status %d, want %d — out %q, stderr %q", code, tc.code, out, errs)
			}
			if out != tc.out {
				t.Errorf("stdout %q, want %q", out, tc.out)
			}
			if tc.errs != "" && !strings.Contains(errs, tc.errs) {
				t.Errorf("stderr %q, want it to name %q", errs, tc.errs)
			}
			if tc.errs == "" && errs != "" {
				t.Errorf("stderr %q, want nothing", errs)
			}
		})
	}
}
