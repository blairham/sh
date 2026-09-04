// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

func TestBackgroundJobsRunAndAreWaitedFor(t *testing.T) {
	tests := []struct{ src, want string }{
		{`/bin/echo bg & wait`, "bg\n"},
		{`/usr/bin/true & wait; printf "%s" "$?"`, "0"},
		// Starting a job succeeds even when the job will not.
		{`/usr/bin/false & printf "%s" "$?"`, "0"},
		// A builtin or compound has no process of its own and still runs.
		{`{ printf grouped; } & wait`, "grouped"},
	}
	for _, tc := range tests {
		if got, _ := run(t, tc.src, nil); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestDollarBangIsAnswerableImmediately(t *testing.T) {
	// The pid is set by the goroutine that starts the process, so starting a
	// job cannot return until it is known — `$!` is read on the very next
	// line and must not race it.
	got, _ := run(t, `/bin/sleep 0 & printf "%s" "$!"`, nil)
	pid, err := strconv.Atoi(strings.TrimSpace(got))
	if err != nil || pid <= 0 {
		t.Fatalf(`$! = %q, want a pid`, got)
	}
	// A job with no process of its own reports zero rather than a stale pid.
	if got, _ := run(t, `{ :; } & printf "%s" "$!"`, nil); got != "0" {
		t.Errorf(`$! for a compound job = %q, want 0`, got)
	}
}

func TestBackgroundJobIsItsOwnProcessGroup(t *testing.T) {
	// docs/design.md committed to real process groups before any code
	// existed, on the grounds that a shell whose jobs are goroutines cannot
	// deliver a signal to a job or hand it the terminal. This is that
	// commitment, checked against the kernel rather than against the code.
	if !HasProcessGroups {
		t.Skip("process groups are not available on this platform")
	}
	got, _ := run(t, `/bin/sleep 1 & printf "%s" "$!"`, nil)
	pid, err := strconv.Atoi(strings.TrimSpace(got))
	if err != nil || pid <= 0 {
		t.Fatalf(`$! = %q, want a pid`, got)
	}

	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		t.Skipf("the job already exited: %v", err)
	}
	if pgid != pid {
		t.Errorf("job pgid = %d, want %d — it should lead its own group", pgid, pid)
	}
	ours, err := syscall.Getpgid(syscall.Getpid())
	if err == nil && pgid == ours {
		t.Errorf("the job shares this process's group (%d), so it is not a job", ours)
	}
}

// TestAnOperandThatNamesNeitherAProcessNorAJob is four wordings and three
// statuses, and none of them was the substrate's own.
func TestAnOperandThatNamesNeitherAProcessNorAJob(t *testing.T) {
	for _, tc := range []struct {
		name   string
		dg     Diagnostics
		want   string
		status int
	}{
		{
			"quoting it and naming both things it could have been",
			Diagnostics{WaitBadJob: "wait: `%[1]s': not a pid or valid job spec", WaitBadJobStatus: 1},
			"`nope': not a pid or valid job spec", 1,
		},
		{
			"calling it an illegal number",
			Diagnostics{WaitBadJob: "wait: Illegal number: %[1]s", WaitBadJobStatus: 2},
			"Illegal number: nope", 2,
		},
		{
			"calling it a job that was not found, with the status of a missing command",
			Diagnostics{WaitBadJob: "wait: job not found: %[1]s", WaitBadJobStatus: 127},
			"job not found: nope", 127,
		},
		{
			// The zero value: the substrate's own words and its own status.
			"saying nothing of its own",
			Diagnostics{},
			"nope: not a pid", 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, `wait nope; printf "st=%s" "$?"`, func(r *Runner) {
				dg := tc.dg
				r.Diagnostics = &dg
			})
			if !strings.Contains(out, tc.want) {
				t.Errorf("output = %q, want %q in it", out, tc.want)
			}
			if !strings.Contains(out, fmt.Sprintf("st=%d", tc.status)) {
				t.Errorf("output = %q, want status %d", out, tc.status)
			}
			_ = st
		})
	}
}

// TestAPidThatIsNotOurChild is 127 in every dialect and said out loud in two,
// so the wording is an answer and the status is not.
func TestAPidThatIsNotOurChild(t *testing.T) {
	spoken, _ := run(t, `wait 999999; printf "st=%s" "$?"`, func(r *Runner) {
		r.Diagnostics = &Diagnostics{WaitNotOurChild: "wait: pid %[1]d is not a child of this shell"}
	})
	if !strings.Contains(spoken, "pid 999999 is not a child") {
		t.Errorf("output = %q, want the complaint", spoken)
	}
	if !strings.Contains(spoken, "st=127") {
		t.Errorf("output = %q, want 127", spoken)
	}

	silent, _ := run(t, `wait 999999; printf "st=%s" "$?"`, nil)
	if strings.Contains(silent, "child") {
		t.Errorf("output = %q, want nothing said", silent)
	}
	if !strings.Contains(silent, "st=127") {
		t.Errorf("output = %q, want 127 even in silence", silent)
	}
}

// TestWaitingOnAChildWeDoHaveReportsItsStatus is the other side of the
// number that is not ours: `wait $!` on a real job reports what the job did,
// and a rule that answered 127 for everything would look right on the case
// above and be wrong on every ordinary use.
func TestWaitingOnAChildWeDoHaveReportsItsStatus(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`/usr/bin/true & wait $!; printf "st=%s" "$?"`, "st=0"},
		{`/usr/bin/false & wait $!; printf "st=%s" "$?"`, "st=1"},
	} {
		if got, _ := run(t, tc.src, nil); !strings.Contains(got, tc.want) {
			t.Errorf("%s: got %q, want %q in it", tc.src, got, tc.want)
		}
	}
}

// TestWaitReadsOptionsOrDoesNot is the axis, and both answers are a real
// dialect's: three refuse a leading `-` word as an option and one takes it as
// a job it cannot find.
func TestWaitReadsOptionsOrDoesNot(t *testing.T) {
	reads := func(r *Runner) {
		sem := PosixSemantics()
		sem.WaitReadsOptions = Yes
		sem.BadOptionToSpecialBuiltinFatal = Yes
		dg := Diagnostics{BuiltinBadOption: "wait: %[2]s: invalid option"}
		r.Semantics, r.Diagnostics = &sem, &dg
	}
	out, _ := run(t, `wait -x; printf "st=%s" "$?"`, reads)
	if !strings.Contains(out, "invalid option") {
		t.Errorf("reads options: got %q, want it refused as one", out)
	}
	// And not fatal, however the dialect answers the special-builtin rule:
	// `wait` is not one, so that rule is not this builtin's to follow.
	if !strings.Contains(out, "st=") {
		t.Errorf("reads options: got %q, want the script to carry on", out)
	}

	asJob := func(r *Runner) {
		sem := PosixSemantics()
		sem.WaitReadsOptions = No
		dg := Diagnostics{WaitBadJob: "wait: job not found: %[1]s", WaitBadJobStatus: 127}
		r.Semantics, r.Diagnostics = &sem, &dg
	}
	out, _ = run(t, `wait -x; printf "st=%s" "$?"`, asJob)
	if !strings.Contains(out, "job not found: -x") || !strings.Contains(out, "st=127") {
		t.Errorf("as a job: got %q", out)
	}
}

// TestDashDashEndsWaitsOptions, which is unanimous and is the control: the
// same leading dashes that are refused as an option end them here.
func TestDashDashEndsWaitsOptions(t *testing.T) {
	out, _ := run(t, `wait --; printf "st=%s" "$?"`, func(r *Runner) {
		sem := PosixSemantics()
		sem.WaitReadsOptions = Yes
		r.Semantics = &sem
	})
	if !strings.Contains(out, "st=0") {
		t.Errorf("got %q, want -- taken as the end of the options", out)
	}
}

// TestAnOptionTheDialectHasIsSaidToBeMissing rather than refused as unknown:
// refusing one the shell really has is a different and worse answer than not
// having it yet.
func TestAnOptionTheDialectHasIsSaidToBeMissing(t *testing.T) {
	out, _ := run(t, `wait -n; printf "st=%s" "$?"`, func(r *Runner) {
		sem := PosixSemantics()
		sem.WaitReadsOptions = Yes
		dg := Diagnostics{
			BuiltinBadOption:           "wait: %[2]s: invalid option",
			UnimplementedOptionLetters: map[string]string{"wait": "nfp"},
		}
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	if !strings.Contains(out, "not implemented yet") {
		t.Errorf("got %q, want it said to be missing", out)
	}
	if strings.Contains(out, "invalid option") {
		t.Errorf("got %q, want it not called unknown", out)
	}
}

// TestWaitRefusesABundleByItsFirstLetter, which is what makes `wait
// --version` come back as `--`: a leading `-` word is a bundle of
// single-letter options, and only the first is named. The same rule printf's
// options here already follow.
func TestWaitRefusesABundleByItsFirstLetter(t *testing.T) {
	out, _ := run(t, `wait --version`, func(r *Runner) {
		sem := PosixSemantics()
		sem.WaitReadsOptions = Yes
		dg := Diagnostics{BuiltinBadOption: "wait: %[2]s: invalid option"}
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	if !strings.Contains(out, "--: invalid option") {
		t.Errorf("got %q, want the first letter of the bundle named", out)
	}
	if strings.Contains(out, "--version: invalid") {
		t.Errorf("got %q, want the whole word not named", out)
	}
}
