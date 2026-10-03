// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package printer_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The strongest promise is not that the tree survives but that the script
// does: original and formatted run identically under the real shell. Each
// variant runs in its own scratch directory with closed stdin, so samples
// that create files or wait on input still terminate and still compare.
func TestFormattingPreservesBehavior(t *testing.T) {
	for _, s := range samples {
		shell := "bash"
		if s.zsh {
			shell = "zsh"
		}
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			// Inside the subtest, and on the subtest's own t. Skipping on the
			// parent skips the whole tier from the first sample whose shell
			// is missing — so a machine with bash and no zsh lost every bash
			// case too, and reported that as a pass. A tier that can vanish
			// silently is worse than one that is absent.
			if _, err := exec.LookPath(shell); err != nil {
				t.Skipf("%s not installed", shell)
			}
			out := format(t, s.src, dialect(s.zsh), style(s.zsh))
			beforeOut, beforeErr, beforeStatus := run(t, shell, s.src)
			afterOut, afterErr, afterStatus := run(t, shell, out)
			if s.loose {
				beforeErr, afterErr = "", ""
			}
			if beforeOut != afterOut || beforeErr != afterErr || beforeStatus != afterStatus {
				t.Errorf("behavior changed under %s\nformatted:\n%s\nstdout %q -> %q\nstderr %q -> %q\nstatus %d -> %d",
					shell, out, beforeOut, afterOut, beforeErr, afterErr, beforeStatus, afterStatus)
			}
		})
	}
}

func run(t *testing.T, shell, script string) (stdout, stderr string, status int) {
	t.Helper()
	// The shell here is the machine's own bash or zsh, found on PATH, and not
	// this repository's: these tiers grade the formatter against the reference.
	// Once, on the macOS runner, that bash reported a builtin's write as
	// interrupted (`echo: write error: Interrupted system call`) in one of two
	// identical runs of `procsub-redirect`, run 37083990697 (#5501). That is the
	// reference's write meeting a signal, and it says nothing about the sample
	// or the formatter. It was not reproduced locally in about 6400 runs under
	// load, so a run that shows it is repeated, a bounded number of times.
	for attempt := 0; ; attempt++ {
		stdout, stderr, status = runOnce(t, shell, script)
		if attempt == 2 || !strings.Contains(stderr, "write error: Interrupted system call") {
			return stdout, stderr, status
		}
		t.Logf("%s reported an interrupted write; running the sample again", shell)
	}
}

func runOnce(t *testing.T, shell, script string) (stdout, stderr string, status int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-c", script)
	cmd.Dir = t.TempDir()
	cmd.Stdin = nil
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("timed out under %s:\n%s", shell, script)
	}
	if ee, ok := err.(*exec.ExitError); ok {
		status = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return so.String(), se.String(), status
}

// TestEverySampleIsDeterministic runs each sample twice *unformatted* and
// requires the two runs to agree.
//
// The tier above compares an unformatted run with a formatted one and blames
// the formatter for any difference. That inference only holds if the sample
// itself says the same thing twice — and one did not: `(cd /tmp && ls)`
// listed a directory the rest of the machine writes to, so the tier failed
// roughly one run in four, naming the formatter for someone else's file.
//
// A flake is bad; a flake that accuses the wrong component is worse, because
// the next person to see it goes looking in the printer. This test fails
// first and says which sample is at fault, so the diagnosis is one line
// instead of an afternoon.
func TestEverySampleIsDeterministic(t *testing.T) {
	for _, s := range samples {
		shell := "bash"
		if s.zsh {
			shell = "zsh"
		}
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			if _, err := exec.LookPath(shell); err != nil {
				t.Skipf("%s not installed", shell)
			}
			firstOut, firstErr, firstStatus := run(t, shell, s.src)
			againOut, againErr, againStatus := run(t, shell, s.src)
			if s.loose {
				firstErr, againErr = "", ""
			}
			if firstOut != againOut || firstErr != againErr || firstStatus != againStatus {
				t.Errorf("this sample does not say the same thing twice, so it cannot grade the formatter\n"+
					"source:\n%s\nstdout %q -> %q\nstderr %q -> %q\nstatus %d -> %d",
					s.src, firstOut, againOut, firstErr, againErr, firstStatus, againStatus)
			}
		})
	}
}

// The rerun is bounded and is for that one report only: a stand-in for the
// reference that reports the interrupted write once and then runs cleanly is
// read as its clean run, and one that always reports it still says so.
func TestAnInterruptedReferenceWriteIsRunAgain(t *testing.T) {
	dir := t.TempDir()
	once := filepath.Join(dir, "once")
	script := "#!/bin/sh\nif [ ! -e " + once + " ]; then : > " + once + "; echo 'bash: line 1: echo: write error: Interrupted system call' >&2; exit 1; fi\necho ok\n"
	fake := filepath.Join(dir, "fakesh")
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if out, errs, st := run(t, fake, ":"); out != "ok\n" || errs != "" || st != 0 {
		t.Errorf("got %q %q %d, want the clean second run", out, errs, st)
	}
	always := filepath.Join(dir, "always")
	if err := os.WriteFile(always, []byte("#!/bin/sh\necho 'x: write error: Interrupted system call' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, errs, st := run(t, always, ":"); !strings.Contains(errs, "Interrupted system call") || st != 1 {
		t.Errorf("got %q %d, want the report kept when every run makes it", errs, st)
	}
}
