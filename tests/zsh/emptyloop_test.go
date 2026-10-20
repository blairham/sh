// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/dialecttest"
)

// A loop with nothing to test and nothing to run, in both keywords.
//
// This is the one shell whose grammar admits the shape: `until; do done` is a
// syntax error in bash 5.3.20, ksh93u+, dash 0.5.12 and BusyBox ash 1.37.0,
// and so is an incomplete `until` at end of input, so zsh 5.9.2 is the whole
// of the expectation and there is no axis to carry — nothing disagrees with
// it.
//
// Measured 2026-09-20, `timeout 3 env -i PATH=/usr/bin:/bin LC_ALL=C zsh -f
// s.sh`, standard input on the null device, three runs each:
//
//	until; do \n done; echo C     never returns, 99% CPU
//	while; do \n done; echo C     never returns
//	until; do :; done; echo C     `C`, status 0
//	echo A; until true            `A`, status 0
//	echo A; until false           never returns
//	echo A; while false           `A`, status 0
//
// The first two rows are the claim and the next four are what make it one:
// an empty condition on its own is status 0 — which is what stops the third
// row and what makes `while; do :; done` run forever — and an empty body on
// its own changes nothing. Read as "an empty condition is 0" alone, which is
// what this shell did, `until` leaves before its first pass and exits 0 where
// zsh is still going.
//
// The 99% is the other half of the measurement: a run that never returns
// with its input on the null device could be a blocked read, which would want
// the opposite fix. Both shells spin (#3852).
func TestALoopWithNoConditionAndNoBody(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		ends            bool
	}{
		{
			name: "until with neither never ends",
			src:  "until; do\ndone; echo C",
		},
		{
			name: "while with neither never ends",
			src:  "while; do\ndone; echo C",
		},
		{
			// An empty condition on its own is 0, which is what `until`
			// stops on — the reading that was right about this row and
			// wrong about the first.
			name: "until with an empty condition and a body ends",
			src:  "until; do :; done; echo C",
			want: "C",
			ends: true,
		},
		{
			// And an empty body on its own changes nothing: the condition
			// still decides.
			name: "until with a condition and an empty body ends",
			src:  "echo A; until true",
			want: "A",
			ends: true,
		},
		{
			name: "until with a condition and an empty body can still spin",
			src:  "echo A; until false",
			want: "A",
		},
		{
			name: "while with a condition and an empty body ends",
			src:  "echo A; while false",
			want: "A",
			ends: true,
		},
		{
			// The control that is not this shape at all.
			name: "an ordinary loop is untouched",
			src:  "for i in 1 2; do :; done; echo D",
			want: "D",
			ends: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runUntilStopped(t, tc.src)
			switch {
			case tc.ends && err != nil:
				t.Fatalf("%s: did not end on its own: %v", tc.src, err)
			case !tc.ends && err == nil:
				t.Fatalf("%s: ended on its own, where zsh 5.9.2 never returns; output %q",
					tc.src, out)
			}
			if got := strings.Join(strings.Fields(out), " "); got != tc.want {
				t.Errorf("%s: output %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// runUntilStopped runs src with a bound on it, reporting what it wrote and
// whether the bound is what ended it.
//
// A non-nil error is the whole assertion for a row that must not return, so
// the bound has to be one the shell cannot reach by being slow: every row
// here that does end, ends in microseconds. It is short rather than generous
// because half of these rows are meant to hit it.
//
// The context is what stops the ones that do. A loop running no commands
// passes through no command, which is where cancel.go puts the one door a
// caller's cancellation is noticed at, so this could not have been written
// before the same change added the check on the loop's own back edge — the
// goroutine would have spun for the rest of the package's run.
func runUntilStopped(t *testing.T, src string) (string, error) {
	t.Helper()
	f := preset.Parse(t, src)
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{
		Name: "sh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
		Stdout: &buf, Stderr: &buf,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := r.Run(ctx, f); done <- err }()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("run %q: %v", src, err)
		}
		return buf.String(), err
	case <-time.After(20 * time.Second):
		t.Fatalf("run %q: the bound did not stop it, so the loop ignores its context", src)
		return "", nil
	}
}
