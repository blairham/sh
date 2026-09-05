// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package startupcost_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/startupcost"
)

// skipWithoutATerminal turns "this machine cannot make a pseudo-terminal" into
// a skip rather than a failure. A terminal we could not open is not evidence
// about a shell — the same answer the other tests that need one give.
func skipWithoutATerminal(tb testing.TB, err error) {
	tb.Helper()
	if errors.Is(err, pty.ErrUnsupported) {
		tb.Skipf("no pseudo-terminal here: %v", err)
	}
}

// report replaces the benchmark's own ns/op with the time each start actually
// took, which is not the same number and was off by a factor of twenty-five.
//
// A benchmark times the whole call, and the whole call includes ending the
// shell afterwards: killing a session leader that owns a controlling terminal
// and reaping it. Measured, that teardown costs one of the reference shells
// about 190ms and costs the others under a millisecond, so the default figure
// reported zsh as taking two hundred milliseconds to draw a prompt when the
// same binary, measured start to prompt, draws one in about eight. Nothing
// about that is startup, and a reference shell credited with it is a
// comparison that flatters us for no reason.
func report(b *testing.B, total time.Duration, n int) {
	b.Helper()
	if n == 0 {
		return
	}
	b.ReportMetric(float64(total.Nanoseconds())/float64(n), "ns/op")
}

// built holds the binaries, built once for the whole run. Building them per
// benchmark would put a compiler in the timing loop of the thing being timed.
var (
	buildOnce sync.Once
	buildDir  string
	buildErr  error
)

func ours(tb testing.TB) []startupcost.Subject {
	tb.Helper()
	buildOnce.Do(func() {
		buildDir, buildErr = os.MkdirTemp("", "startupcost")
		if buildErr != nil {
			return
		}
		for bin, pkg := range map[string]string{
			"our-bash": "./cmd/bash",
			"our-zsh":  "./cmd/zsh",
		} {
			cmd := exec.Command("go", "build", "-o", filepath.Join(buildDir, bin), pkg)
			cmd.Dir = "../.."
			if out, err := cmd.CombinedOutput(); err != nil {
				buildErr = err
				tb.Logf("build %s: %s", pkg, out)
				return
			}
		}
	})
	if buildErr != nil {
		tb.Skipf("cannot build the shells to measure: %v", buildErr)
	}
	// Warmed once. The first execution of a freshly linked binary on macOS
	// pays a one-time validation cost that a shell somebody installed months
	// ago does not, and 288ms of it landed in the first sample the day this
	// was written — which is a fact about a build, not about a shell.
	subjects := startupcost.Ours(buildDir)
	for _, s := range subjects {
		if _, err := startupcost.RunCommand(s); err != nil {
			tb.Skipf("%s would not run: %v", s.Name, err)
		}
	}
	return subjects
}

func references(tb testing.TB) []startupcost.Subject {
	tb.Helper()
	var found []startupcost.Subject
	for _, s := range startupcost.References() {
		if s.Path == "" {
			tb.Logf("%s: not installed here, so not compared against", s.Name)
			continue
		}
		found = append(found, s)
	}
	return found
}

func all(tb testing.TB) []startupcost.Subject {
	tb.Helper()
	return append(ours(tb), references(tb)...)
}

// BenchmarkCommandString is the `-c` path: what a script's every subshell
// pays, and the one that is over before anything is drawn.
func BenchmarkCommandString(b *testing.B) {
	for _, s := range all(b) {
		b.Run(s.Name, func(b *testing.B) {
			var total time.Duration
			n := 0
			for b.Loop() {
				d, err := startupcost.RunCommand(s)
				if err != nil {
					b.Fatalf("%s: %v", s.Name, err)
				}
				total, n = total+d, n+1
			}
			report(b, total, n)
		})
	}
}

// BenchmarkFirstPrompt is process start to the first prompt on a real
// terminal: what a person waits for on every new window.
//
// Both halves are measured because one of them is about to get more expensive.
// An rc file read on every interactive start is landing separately, and a
// baseline recorded only for the shell that reads nothing would stop being a
// baseline the day it lands.
func BenchmarkFirstPrompt(b *testing.B) {
	for _, s := range all(b) {
		b.Run(s.Name, func(b *testing.B) {
			var total time.Duration
			n := 0
			for b.Loop() {
				d, err := startupcost.RunPrompt(s, "")
				if err != nil {
					b.Fatalf("%s: %v", s.Name, err)
				}
				total, n = total+d, n+1
			}
			report(b, total, n)
		})
	}
}

// BenchmarkFirstPromptWithRC is the same start with an rc file to read, so the
// pair says what an rc costs rather than what a shell costs.
func BenchmarkFirstPromptWithRC(b *testing.B) {
	for _, s := range all(b) {
		if len(s.RCPromptArgs) == 0 && len(s.RCEnv) == 0 {
			continue
		}
		dir := b.TempDir()
		if _, err := startupcost.WriteRC(s, dir); err != nil {
			b.Fatalf("%s: %v", s.Name, err)
		}
		b.Run(s.Name, func(b *testing.B) {
			var total time.Duration
			n := 0
			for b.Loop() {
				d, err := startupcost.RunPrompt(s, dir)
				if err != nil {
					b.Fatalf("%s: %v", s.Name, err)
				}
				total, n = total+d, n+1
			}
			report(b, total, n)
		})
	}
}

// TestEverySubjectStartsBothWays is the guard the benchmarks need and cannot
// be: a benchmark that measures a shell which never drew a prompt would report
// a number for a timeout, and a benchmark nobody runs reports nothing at all.
//
// It is a test rather than a threshold. There is no assertion here about how
// long anything took — that is the machine's answer as much as the code's —
// only that both routes were reached, which is what makes a later timing run
// mean what it says. It also keeps the harness compiling and honest as the
// front end changes around it, which a file only run by hand would not.
func TestEverySubjectStartsBothWays(t *testing.T) {
	for _, s := range all(t) {
		t.Run(s.Name, func(t *testing.T) {
			if _, err := startupcost.RunCommand(s); err != nil {
				t.Errorf("-c: %v", err)
			}
			if _, err := startupcost.RunPrompt(s, ""); err != nil {
				skipWithoutATerminal(t, err)
				t.Errorf("prompt with no rc: %v", err)
			}
			dir := t.TempDir()
			if _, err := startupcost.WriteRC(s, dir); err != nil {
				t.Fatal(err)
			}
			if _, err := startupcost.RunPrompt(s, dir); err != nil {
				t.Errorf("prompt with an rc: %v", err)
			}
		})
	}
}

// TestTheRCFileIsActuallyRead is what keeps the pair honest. A shell handed an
// rc it silently ignores is indistinguishable, in a timing loop, from a shell
// that reads one very fast — and the with-and-without pair would then be two
// measurements of the same thing, reported as a difference of zero.
func TestTheRCFileIsActuallyRead(t *testing.T) {
	for _, s := range all(t) {
		t.Run(s.Name, func(t *testing.T) {
			dir := t.TempDir()
			path, err := startupcost.WriteRC(s, dir)
			if err != nil {
				t.Fatal(err)
			}
			// The mark is drawn by the rc rather than by the prompt, so
			// seeing it means the file ran. Appended to the body every
			// subject reads, so what is proven is the same file.
			extra := "\nprintf %s SHSTARTUPMARK\n"
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(body, extra...), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := startupcost.RunPrompt(withoutPromptMark(s), dir); err != nil {
				skipWithoutATerminal(t, err)
				t.Errorf("%s did not read the rc file it was pointed at: %v", s.Name, err)
			}
		})
	}
}

// withoutPromptMark takes the sentinel out of the prompt, so that finding it
// can only mean the rc file drew it.
//
// Through Env and not RCEnv, which is the whole of what makes this test bite.
// RCEnv is part of the rc route, so silencing the prompt through it silences
// nothing in a shell that has failed to take that route — and a RunPrompt
// rewritten to ignore the rc directory outright drew the sentinel from the
// ordinary prompt and passed.
func withoutPromptMark(s startupcost.Subject) startupcost.Subject {
	s.Env = append(append([]string{}, s.Env...), "PS1=$ ", "PROMPT=$ ")
	return s
}

// TestAShellThatNeverPromptsFailsRatherThanHangs is the guard the rest of this
// file leans on. Every other test here concludes from *not* seeing the
// sentinel, and a harness that waited forever for one would turn every such
// conclusion into a build that dies ten minutes later with a stack trace
// pointing at its own teardown. That is not a hypothetical: it is what a wait
// with no bound on it did to a CI run, after the measurement had already
// finished.
//
// `cat` stands in for the shell that will not prompt. It starts, it holds the
// terminal, and it writes nothing — which is exactly the shape of the failure,
// without needing a broken shell to produce it.
func TestAShellThatNeverPromptsFailsRatherThanHangs(t *testing.T) {
	t.Parallel()
	silent := startupcost.Subject{
		Name:          "cat",
		Path:          "/bin/cat",
		PromptArgs:    nil,
		PromptTimeout: 500 * time.Millisecond,
	}
	if _, err := os.Stat(silent.Path); err != nil {
		t.Skipf("no %s here: %v", silent.Path, err)
	}
	start := time.Now()
	_, err := startupcost.RunPrompt(silent, "")
	skipWithoutATerminal(t, err)
	if err == nil {
		t.Fatal("a program that drew no prompt was reported as having started")
	}
	if took := time.Since(start); took > 20*time.Second {
		t.Errorf("gave up after %v, which is a hang rather than a failure", took)
	}
}
