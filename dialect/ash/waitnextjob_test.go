// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// `wait -n` is taken here and is not bash's `-n`. Ours refused the letter
// outright, which is a third thing and is what no shell does (#3245).
//
// Measured 2026-09-17 inside the pinned image
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0, with the jobs' **exit statuses** as the discriminator: a
// probe using bare `sleep` jobs reads this reading and bash's as one, which is
// the blind probe #3226 is about.
func TestWaitNextJobIsTheFirstToSucceed(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
		why  string
	}{
		{
			"the first job succeeded",
			"{ sleep 1; exit 0; } &\n{ sleep 2; exit 7; } &\nwait -n\necho st=$?",
			"st=0", "a job that exited 0 ends the wait at 0",
		},
		{
			"the first job did not",
			"{ sleep 1; exit 7; } &\n{ sleep 2; exit 0; } &\nwait -n\necho st=$?",
			"st=0", "and the wait goes on until one does, where bash stops at 7",
		},
		{
			"no job did",
			"{ sleep 1; exit 7; } &\nwait -n\necho st=$?",
			"st=129", "a constant, not any job's status",
		},
		{
			"a different failing status",
			"{ sleep 1; exit 254; } &\nwait -n\necho st=$?",
			"st=129", "which is what says the number is not the job's",
		},
		{
			"nothing to wait for",
			"wait -n\necho st=$?",
			"st=0", "where bash answers 127",
		},
		{
			"an operand is a plain wait",
			"{ sleep 1; exit 7; } &\np=$!\nwait -n $p\necho st=$?",
			"st=7", "the letter changes nothing once operands name the jobs",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src+"\n")
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q — %s", out, tc.want, tc.why)
			}
		})
	}
}

// And `-n` is the only letter this `wait` takes: every other one is refused,
// which is the control that says the letter is read rather than swallowed as
// an ordinary word.
func TestWaitTakesNoOtherLetter(t *testing.T) {
	// The letter named is the one inside the word, not the word: `wait -zz`
	// is `illegal option -z` in the reference and here.
	for _, tc := range []struct{ word, letter string }{
		{"-q", "-q"}, {"-zz", "-z"}, {"-p", "-p"},
	} {
		out, status := run(t, "wait "+tc.word+"\n")
		if status != 2 || !strings.Contains(out, "illegal option "+tc.letter) {
			t.Errorf("wait %s = %q at %d, want `illegal option %s` at 2",
				tc.word, out, status, tc.letter)
		}
	}
}
