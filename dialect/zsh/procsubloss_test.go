// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// The instrument for #1901: how often a process substitution loses the output
// its body wrote.
//
// It is off by default and is not a gate. What it measures is a race whose
// rate is about one run in four thousand on this machine, so a case that
// asserted on one run would be a coin toss and a case that asserted on forty
// thousand would be twenty seconds of the gate spent on a number that is only
// interesting when it changes. `-procsub.rounds` turns it on:
//
//	go test ./dialect/zsh -run TestAProcessSubstitutionDeliversItsBody \
//	    -procsub.rounds 5000 -procsub.workers 8
//
// **Concurrency is the measurement, not a way to go faster.** The loss needs
// the shell's writing end and the command's reading end to arrive at the pipe
// within a few microseconds of each other, and eight shells running at once is
// what makes that happen often enough to count. One worker finds nothing.
//
// The tally is the point rather than a pass or a fail. The three shapes it
// separates are three faces of one defect and they answer different questions:
//
//   - a **short** answer — `[two]`, `[one]` — is a write the body made that
//     the pipe refused with EPIPE and the shell dropped on the floor;
//   - an **empty** answer is both writes accepted and the bytes gone anyway,
//     which is the pipe being torn down under a reader that had not finished
//     opening it;
//   - a **hang** is that same reader never woken at all.
//
// See #1901 for the measurements these names come from.
var (
	procSubRounds  = flag.Int("procsub.rounds", 0, "rounds per worker for the process-substitution loss instrument")
	procSubWorkers = flag.Int("procsub.workers", 8, "concurrent shells for the process-substitution loss instrument")
	procSubBound   = flag.Duration("procsub.bound", 15*time.Second, "how long one round may take before it counts as a hang")
)

// The two spellings a reader of a `<(cmd)` can be written in, because the
// defect is the pipe's and not the builtin's: #1901 was filed against
// `sysopen`, and the plain redirection loses the same bytes at the same rate.
var procSubLossCases = []struct{ name, src, want string }{
	{
		"through a redirection",
		"buf=\nwhile IFS= read -r l; do buf=$buf$l; done < <(print one; print two)\nprint -r -- \"[$buf]\"",
		"[onetwo]\n",
	},
	{
		"through sysopen",
		`sysopen -r -o cloexec -u fd <(print -n one; print -n two) || { print -r -- failed; return }
buf=
while sysread -i $fd chunk; do buf=$buf$chunk; done
print -r -- "[$buf]"`,
		"[onetwo]\n",
	},
}

func TestAProcessSubstitutionDeliversItsBody(t *testing.T) {
	if *procSubRounds <= 0 {
		t.Skip("set -procsub.rounds to run the process-substitution loss instrument")
	}
	for _, tc := range procSubLossCases {
		t.Run(tc.name, func(t *testing.T) {
			ran, tally := procSubLossTally(t, tc.src, tc.want)
			t.Logf("%d rounds, %d lost", ran, procSubLossTotal(tally))
			for _, line := range procSubLossLines(tally) {
				t.Log(line)
			}
			if n := procSubLossTotal(tally); n > 0 {
				t.Errorf("%d of %d rounds did not deliver the body's output", n, ran)
			}
		})
	}
}

// procSubLossTally runs the snippet in workers concurrent shells and counts
// what came back that was not the answer.
//
// Each round is on a deadline of its own rather than the test's, because the
// failure this is here to count includes one that never returns: a round that
// blocks forever would otherwise end the run at the package timeout and take
// the tally with it. The blocked shell is left where it is — it holds nothing
// the rest of the run needs, and killing it is not a thing a Runner offers.
func procSubLossTally(t *testing.T, src, want string) (int64, map[string]int) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	preset := dialecttest.Preset{
		Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
		Diagnostics: zsh.Diagnostics, Apply: zsh.Apply, Prelude: zsh.Prelude,
	}
	var ran atomic.Int64
	var mu sync.Mutex
	tally := map[string]int{}
	note := func(k string) {
		mu.Lock()
		tally[k]++
		mu.Unlock()
	}
	var wg sync.WaitGroup
	for range *procSubWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dir := t.TempDir()
			for range *procSubRounds {
				done := make(chan string, 1)
				go func() {
					var out strings.Builder
					r := preset.Runner(dialecttest.Base{
						Dir: dir, Stdout: &out, Stderr: &out,
						Vars: map[string]string{"PATH": dir},
					})
					st, rerr := r.Run(context.Background(), f)
					r.CleanUp()
					done <- fmt.Sprintf("%q status %d error %v", out.String(), st, rerr)
				}()
				ran.Add(1)
				select {
				case got := <-done:
					if got != fmt.Sprintf("%q status 0 error <nil>", want) {
						note(got)
					}
				case <-time.After(*procSubBound):
					note("a round that never finished")
				}
			}
		}()
	}
	wg.Wait()
	return ran.Load(), tally
}

func procSubLossTotal(tally map[string]int) int {
	n := 0
	for _, c := range tally {
		n += c
	}
	return n
}

// procSubLossLines is the tally in a stable order, so two runs of the
// instrument can be read side by side.
func procSubLossLines(tally map[string]int) []string {
	lines := make([]string, 0, len(tally))
	for k, n := range tally {
		lines = append(lines, fmt.Sprintf("%6d  %s", n, k))
	}
	sort.Strings(lines)
	return lines
}
