// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blairham/sh/internal/wild"
)

// shellThat writes a shell script that behaves as body says and returns its
// path, for use as one of the two shells being compared. It receives the
// script to run as $1, exactly as a real shell does.
func shellThat(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// Two shells that behave alike agree, however many scripts they are given.
func TestIdenticalShellsAgree(t *testing.T) {
	dir := t.TempDir()
	script := write(t, dir, "s", "#!/bin/sh\necho hi\n")

	rep := wild.RunSweep(context.Background(), []string{script}, wild.UnderTest{Path: "/bin/sh"}, "/bin/sh", 10*time.Second)
	if rep.Ran != len(wild.Probes) || rep.Agreed != rep.Ran || len(rep.Mismatches) != 0 {
		t.Errorf("%+v", rep)
	}
}

// And a difference in *either* the output or the status is reported.
func TestADifferenceIsReported(t *testing.T) {
	dir := t.TempDir()
	script := write(t, dir, "s", "#!/bin/sh\necho hi\n")

	for _, tc := range []struct{ name, body string }{
		{"different output", `echo different`},
		{"different status", `sh "$@"; exit 7`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			odd := shellThat(t, "odd", tc.body)
			rep := wild.RunSweep(context.Background(), []string{script}, wild.UnderTest{Path: odd}, "/bin/sh", 10*time.Second)
			if len(rep.Mismatches) != len(wild.Probes) {
				t.Errorf("got %d mismatches, want %d: %+v", len(rep.Mismatches), len(wild.Probes), rep)
			}
		})
	}
}

// A script that waits is not evidence about either shell, so it is counted
// apart rather than reported as a difference.
func TestAScriptThatWaitsIsNotAMismatch(t *testing.T) {
	dir := t.TempDir()
	script := write(t, dir, "s", "#!/bin/sh\nsleep 30\n")

	start := time.Now()
	rep := wild.RunSweep(context.Background(), []string{script}, wild.UnderTest{Path: "/bin/sh"}, "/bin/sh", 300*time.Millisecond)
	if rep.Timedout != len(wild.Probes) || len(rep.Mismatches) != 0 || rep.Ran != 0 {
		t.Errorf("%+v", rep)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Errorf("took %v, so the timeout is not being applied", elapsed)
	}
}

// The sweep contains what it runs: a fresh directory each time, and no
// standard input to wait on.
func TestARunIsContained(t *testing.T) {
	dir := t.TempDir()
	mark := filepath.Join(dir, "escaped")
	// Writes a file in the working directory, and reports what it found
	// there — which must be nothing from the run before it.
	script := write(t, dir, "s", "#!/bin/sh\nls | tr '\\n' ' '\ntouch ran\n")

	rep := wild.RunSweep(context.Background(), []string{script}, wild.UnderTest{Path: "/bin/sh"}, "/bin/sh", 10*time.Second)
	if rep.Agreed != rep.Ran {
		t.Fatalf("%+v", rep)
	}
	if _, err := os.Stat(mark); err == nil {
		t.Error("a run wrote into the directory it was launched from")
	}
	if _, err := os.Stat(filepath.Join(dir, "ran")); err == nil {
		t.Error("a run wrote beside the script rather than in its own directory")
	}
	// Reading standard input ends rather than waiting, which the timeout
	// would otherwise have to catch.
	reader := write(t, dir, "r", "#!/bin/sh\nread x; echo \"[$x]\"\n")
	rep = wild.RunSweep(context.Background(), []string{reader}, wild.UnderTest{Path: "/bin/sh"}, "/bin/sh", 2*time.Second)
	if rep.Timedout != 0 {
		t.Errorf("reading standard input waited: %+v", rep)
	}
}

// The shell's own path appears in its diagnostics and the two shells have
// different ones, so it is replaced — but only the whole path. Replacing the
// base name as well turned `GNU bashbug` into `GNU <shell>bug` and reported
// two identical outputs as a difference.
func TestNormalizingDoesNotEatTheScriptsOwnWords(t *testing.T) {
	dir := t.TempDir()
	// Prints a word that contains the reference shell's base name.
	script := write(t, dir, "s", "#!/bin/sh\necho 'GNU shbug 1.0'\n")

	rep := wild.RunSweep(context.Background(), []string{script}, wild.UnderTest{Path: "/bin/sh"}, "/bin/sh", 10*time.Second)
	if len(rep.Mismatches) != 0 {
		t.Errorf("a word containing the shell's name was reported as a difference: %+v", rep.Mismatches)
	}
	// And the path *is* replaced, which is what the normalizing is for: two
	// shells whose diagnostics name themselves must still compare equal.
	odd := shellThat(t, "sh", `echo "$0: bad"; exit 2`)
	other := shellThat(t, "sh2", `echo "$0: bad"; exit 2`)
	rep = wild.RunSweep(context.Background(), []string{script}, wild.UnderTest{Path: odd}, other, 10*time.Second)
	if len(rep.Mismatches) != 0 {
		t.Errorf("two shells naming themselves were reported as different: %+v", rep.Mismatches)
	}
}

// A shell named by a bare word must not have that word struck out of the
// scripts' output.
//
// The reference shell is named on the command line and defaults to `bash`, so
// normalize was handed a bare word and replaced every occurrence of it — a
// word that appears in a great many scripts' own output for reasons that have
// nothing to do with which shell is running. `GNU bashbug` became
// `GNU <shell>bug` on one side and stayed as it was on the other, and two
// identical outputs were reported as a difference.
//
// The two sides have to be *named* differently for this to bite, which is
// exactly the sweep's own arrangement: ours is a built binary's path and the
// reference is whatever was typed on the command line. Here they are one
// shell under two names, so anything they disagree about is the harness.
func TestAShellsNameIsNotStruckOutOfTheOutput(t *testing.T) {
	dir := t.TempDir()
	// The script says the reference's name as part of a longer word, which is
	// what a bare-word replacement would cut in half.
	script := write(t, dir, "s", "#!/bin/sh\necho 'GNU shbug'\n")

	rep := wild.RunSweep(context.Background(), []string{script}, wild.UnderTest{Path: "/bin/sh"}, "sh", 10*time.Second)
	if rep.Ran == 0 {
		t.Skip("sh did not run here")
	}
	if rep.Agreed != rep.Ran || len(rep.Mismatches) != 0 {
		t.Errorf("one shell under two names disagreed with itself: %+v", rep.Mismatches)
	}
}

// A script that does not produce the same thing twice is not evidence about
// either shell, and must not be reported as a difference between them.
//
// A third of what this sweep reported was exactly this: scripts whose output
// carries a timestamp or a process id, which differ between any two runs
// including two runs of the *same* shell.
func TestAScriptThatIsNotTheSameTwiceIsNotAMismatch(t *testing.T) {
	dir := t.TempDir()
	// A process id, which is what the real cases carry.
	script := write(t, dir, "s", "#!/bin/sh\necho \"pid $$\"\n")

	rep := wild.RunSweep(context.Background(), []string{script}, wild.UnderTest{Path: "/bin/sh"}, "/bin/sh", 10*time.Second)
	if rep.Unstable != len(wild.Probes) {
		t.Errorf("unstable = %d, want %d: %+v", rep.Unstable, len(wild.Probes), rep)
	}
	if len(rep.Mismatches) != 0 {
		t.Errorf("reported %d differences for a script that differs from itself: %+v", len(rep.Mismatches), rep)
	}
}

// And the check must not swallow a real difference: a script that repeats
// itself is still compared, and a shell that answers it differently is still
// reported.
func TestAStableDifferenceIsStillReported(t *testing.T) {
	dir := t.TempDir()
	script := write(t, dir, "s", "#!/bin/sh\necho hi\n")
	odd := shellThat(t, "odd", `echo different`)

	rep := wild.RunSweep(context.Background(), []string{script}, wild.UnderTest{Path: odd}, "/bin/sh", 10*time.Second)
	if len(rep.Mismatches) != len(wild.Probes) {
		t.Errorf("got %d mismatches, want %d: %+v", len(rep.Mismatches), len(wild.Probes), rep)
	}
	if rep.Unstable != 0 {
		t.Errorf("unstable = %d, want 0: a script that repeats itself is stable", rep.Unstable)
	}
}

// The instability that matters is the *reference* shell's, because that is
// what a difference is measured against. A shell under test that cannot
// repeat itself is a fault in the shell under test, and is reported.
func TestOnlyTheReferenceHasToRepeatItself(t *testing.T) {
	dir := t.TempDir()
	script := write(t, dir, "s", "#!/bin/sh\necho hi\n")
	// Answers with something new every time, which is a real difference from
	// a reference that does not.
	odd := shellThat(t, "odd", `echo "$$"`)

	rep := wild.RunSweep(context.Background(), []string{script}, wild.UnderTest{Path: odd}, "/bin/sh", 10*time.Second)
	if len(rep.Mismatches) != len(wild.Probes) {
		t.Errorf("got %d mismatches, want %d: %+v", len(rep.Mismatches), len(wild.Probes), rep)
	}
}

// Instability in the status alone counts too. A script can print the same
// thing every time and still answer differently, and comparing only the
// output would call that a difference between the shells.
//
// The counter lives beside the script rather than in the run's directory,
// because each run gets a directory of its own and nothing in it survives.
func TestAScriptWhoseStatusVariesIsNotAMismatch(t *testing.T) {
	dir := t.TempDir()
	script := write(t, dir, "s", `#!/bin/sh
d=$(dirname "$0")
n=0
[ -f "$d/n" ] && n=$(cat "$d/n")
echo $((n+1)) > "$d/n"
echo stable
exit $n
`)

	rep := wild.RunSweep(context.Background(), []string{script}, wild.UnderTest{Path: "/bin/sh"}, "/bin/sh", 10*time.Second)
	if rep.Unstable != len(wild.Probes) {
		t.Errorf("unstable = %d, want %d: %+v", rep.Unstable, len(wild.Probes), rep)
	}
	if len(rep.Mismatches) != 0 {
		t.Errorf("reported %d differences for a script whose status varies: %+v", len(rep.Mismatches), rep)
	}
}

// A repeat that times out is not an answer, and must not be read as one.
//
// A timed-out run comes back as a zero outcome — no output, status 0 — which
// is indistinguishable from a reference that legitimately printed nothing and
// succeeded. Comparing it anyway would call such a script stable on the
// strength of a run that never finished.
//
// The script here hangs from its second run on, so the reference answers once
// and then stops answering.
//
// The hanging run is ended by canceling the sweep rather than by the sweep's
// deadline, and that is the whole of what makes this test reliable. It used to
// be given 300ms, which asked two things of one number: that a run forking
// /bin/sh and reading a file finish *inside* it, and that the hang exceed it.
// Under load the first run missed too, and the report came back
// `{Ran:0 Timedout:2 Unstable:0}` — a real deadline losing to a real machine,
// which raising the number would only have made rarer and slower. So the fast
// run is given a minute it will never come near, the script says when it is
// about to hang, and the sweep is stopped on that word. Nothing here is
// waiting on a clock.
func TestARepeatThatTimesOutIsNotStability(t *testing.T) {
	dir := t.TempDir()
	hanging := filepath.Join(dir, "hanging")
	// `exec sleep`, so the hang *is* the shell rather than a child of it:
	// killing the shell leaves a child holding the output pipes open, and
	// os/exec then waits out its WaitDelay before Run returns. Replacing the
	// shell puts the process being killed and the process holding the pipes
	// back together, which takes a second off the test.
	script := write(t, dir, "s", `#!/bin/sh
d=$(dirname "$0")
n=0
[ -f "$d/n" ] && n=$(cat "$d/n")
echo $((n+1)) > "$d/n"
if [ "$n" -ge 1 ]; then
	: > "$d/hanging"
	exec sleep 300
fi
exit 0
`)
	// Prints something and never runs the script, so the reference is the
	// only thing driving the counter.
	odd := shellThat(t, "odd", `echo different`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Closed before the cancel rather than after it, so that a sweep which has
	// returned is a sweep whose cancel this goroutine had already announced.
	// Closing afterwards would leave the assertion below racing the scheduler
	// for the one thing it is there to prove.
	hung := make(chan struct{})
	go func() {
		// The file exists only once the script's second run has reached the
		// hang, so this cannot fire early — and a run that has reached the
		// hang is never coming back, so it cannot fire late either. That is
		// the difference between synchronizing and guessing: a deadline has
		// to be wrong in one direction or the other under enough load, and
		// this is right at any speed.
		for {
			if _, err := os.Stat(hanging); err == nil {
				close(hung)
				cancel()
				return
			}
			select {
			case <-t.Context().Done():
				return
			case <-time.After(time.Millisecond):
			}
		}
	}()

	// A minute is a backstop against a hang this test did not arrange — a
	// /bin/sh that will not start at all — and not the mechanism. Nothing here
	// is meant to reach it, and the script's own wait is five minutes so that
	// reaching it is still a *timeout*: a hang shorter than the backstop is not
	// a hang, and the sweep would read the run that finished it as an answer.
	rep := wild.RunSweep(ctx, []string{script}, wild.UnderTest{Path: odd}, "/bin/sh", time.Minute)
	if len(rep.Mismatches) != 0 {
		t.Errorf("reported %d differences on the strength of a run that timed out: %+v",
			len(rep.Mismatches), rep)
	}
	if rep.Unstable != 1 {
		t.Errorf("unstable = %d, want 1: %+v", rep.Unstable, rep)
	}
	// The shape as well as the count, because `Unstable == 1` on its own does
	// not say the comparison happened: the first probe has to have produced
	// one, and that is the run the old deadline ate.
	if rep.Ran != 1 {
		t.Errorf("ran = %d, want the first probe's comparison and no more: %+v", rep.Ran, rep)
	}
	// And that the sweep was ended by the script rather than by its deadline.
	// Without this the test still passes when the gate never fires — a minute
	// later, on the backstop — which is the failure a deadline-shaped test is
	// least able to notice about itself.
	select {
	case <-hung:
	default:
		t.Error("the sweep ended without the script ever reaching its hang, so the deadline is doing the work again")
	}
}
