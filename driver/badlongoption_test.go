// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// A `--word` this front end could not place is refused in the **dialect's**
// words, and under the dialect's own usage block — #2298.
//
// It used to be one sentence for every shell: `unknown option "--badopt"`, a
// single line, no block, whatever was being imitated. Measured 2026-09-16
// with standard input on /dev/null, the three columns that reach this path
// disagree in every part of the answer — bash 5.3.20 writes `--badopt:
// invalid option` and twenty-two more lines, dash 0.5.12 writes `<shell>: 0:
// Illegal option --` and names no word at all, and BusyBox ash 1.37.0 writes
// `bad option '--badopt'`. All three at status 2.
//
// The block is asserted as well as the sentence, because the block is where
// the missing lines were: in bash's own suite this one word cost forty-four
// lines of the `invocation` file, two refusals' worth of a block this shell
// already had and printed in the letter's path alone.
func TestAnUnplaceableLongOptionIsRefusedInTheDialectsWords(t *testing.T) {
	const block = "Usage:\t%[1]s [option] ...\nSecond line"
	for _, tc := range []struct {
		name     string
		sentence string
		usage    string
		want     []string
		absent   []string
	}{
		{
			name:     "a sentence and a block",
			sentence: "%[1]s: invalid option",
			usage:    block,
			want:     []string{"testsh: --badopt: invalid option\n", "Usage:\ttestsh [option] ...\nSecond line\n"},
		},
		{
			name:     "a sentence and no block",
			sentence: "bad option '%[1]s'",
			want:     []string{"testsh: bad option '--badopt'\n"},
			absent:   []string{"Usage:"},
		},
		{
			// dash's shape: the sentence names no word, so Wording passes it
			// through and the word must not turn up anyway.
			name:     "a sentence with no word in it",
			sentence: "Illegal option --",
			want:     []string{"testsh: Illegal option --\n"},
			absent:   []string{"badopt"},
		},
		{
			// The word without its dashes, which is the second verb. Nobody
			// in the panel spells it this way and the verb is offered all
			// the same, for the reason every other paired wording offers
			// both: a dialect that wanted it would otherwise have to trim
			// the word itself.
			name:     "the word with its dashes off",
			sentence: "no such option: %[2]s",
			want:     []string{"testsh: no such option: badopt\n"},
		},
		{
			// A preset that names no sentence keeps the front end's own,
			// which is the core: the panel agrees on neither the verb, nor
			// the block, nor whether the word appears, so there is nothing
			// for a dialect-free shell to copy.
			name:   "a preset that names none",
			want:   []string{`testsh: unknown option "--badopt"` + "\n"},
			absent: []string{"Usage:"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := shell()
			sh.Diagnostics.InvocationBadLongOption = tc.sentence
			sh.Diagnostics.InvocationUsage = tc.usage
			out, errs, code := runArgs(t, sh, "testsh", "--badopt", "-c", "echo RAN")
			if code != 2 {
				t.Errorf("status %d, want 2", code)
			}
			if out != "" {
				t.Errorf("stdout = %q, want the command never to have run", out)
			}
			for _, w := range tc.want {
				if !strings.Contains(errs, w) {
					t.Errorf("stderr = %q, want %q in it", errs, w)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(errs, a) {
					t.Errorf("stderr = %q, want no %q in it", errs, a)
				}
			}
		})
	}
}

// The `0:` half of dash's line is the shell's own unread-line prefix and not
// part of the sentence, which is what says the two are separate facts: the
// same wording under a dialect that names no line writes no number, as the
// row above it in the first suite does.
func TestTheRefusalTakesTheDialectsInvocationPrefix(t *testing.T) {
	sh := shell()
	sh.Diagnostics.InvocationBadLongOption = "Illegal option --"
	sh.Diagnostics.Location = interp.LocationColonLine
	sh.Diagnostics.InvocationNamesTheUnreadLine = true
	_, errs, _ := runArgs(t, sh, "testsh", "--badopt")
	if want := "testsh: 0: Illegal option --\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
}

// And the two dialects whose option namespace takes a `--name` never arrive
// here at all: the word travels to the runner and is refused there, in the
// words a `set -o` name gets. Without this the sentence above would look like
// the answer for every shell, and it is the answer for three of the five.
func TestANamespaceThatTakesLongNamesRefusesItsOwnWay(t *testing.T) {
	sh := shell()
	sh.Semantics.LongOptionNamesASetOption = interp.Yes
	sh.Diagnostics.InvocationBadLongOption = "MUST NOT APPEAR"
	_, errs, code := runArgs(t, sh, "testsh", "--badopt", "-c", "echo RAN")
	if code == 0 {
		t.Fatalf("status 0, want a refusal — stderr %q", errs)
	}
	if strings.Contains(errs, "MUST NOT APPEAR") {
		t.Errorf("stderr = %q, want the runner's refusal and not the front end's", errs)
	}
}
