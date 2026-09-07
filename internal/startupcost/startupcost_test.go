// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package startupcost_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// BenchmarkFirstPromptWithRichRC is the case #1383 was invisible without.
//
// Same route as BenchmarkFirstPromptWithRC and a different file: forty sourced
// plugins and one pattern-heavy prompt-theme substitution, which is the shape
// of a configuration somebody actually starts their terminal with. On the
// synthetic rc this project was measured *faster* than every reference shell
// while being 56x slower than real zsh on a real one, and both numbers were
// honest. This is the one that moves when the matcher does: measured on the
// change that closed #1383, the pattern work alone went from 40.0s to 0.42s.
func BenchmarkFirstPromptWithRichRC(b *testing.B) {
	for _, s := range all(b) {
		if s.PatternWork == "" || (len(s.RCPromptArgs) == 0 && len(s.RCEnv) == 0) {
			continue
		}
		dir := b.TempDir()
		if _, err := startupcost.WriteRichRC(s, dir); err != nil {
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

// TestTheRichRCIsReadAndItsPatternWorkRuns is what makes the rich half worth
// timing, and it is two claims rather than one.
//
// The first is the same claim TestTheRCFileIsActuallyRead makes: a shell
// pointed at a file it ignores is indistinguishable, in a timing loop, from
// one that reads it very fast.
//
// The second is the one this file did not have, and is the reason #1383's
// sibling trap keeps working. A shell that *refuses* the construct it is being
// timed on runs the clock over a parse error and reports a very good number:
// an agent's benchmark on this repository showed no regression at all for
// exactly that reason, because `syntax.Core()` rejected the construct and the
// output went to io.Discard. So the rich rc draws the sentinel only where the
// substitution produced the answer the whole panel gives, and this test is
// what says the gate is wired the right way round — that reaching the prompt
// means the work was done and not merely attempted.
func TestTheRichRCIsReadAndItsPatternWorkRuns(t *testing.T) {
	for _, s := range all(t) {
		if s.PatternWork == "" {
			continue
		}
		t.Run(s.Name, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := startupcost.WriteRichRC(s, dir); err != nil {
				t.Fatal(err)
			}
			// The prompt is the only place the mark can come from here,
			// because withoutPromptMark takes it out of the environment —
			// so drawing it means the rc ran *and* its pattern work
			// answered what the panel answers.
			if _, err := startupcost.RunPrompt(withoutPromptMark(s), dir); err != nil {
				skipWithoutATerminal(t, err)
				t.Errorf("%s did not reach a prompt from the rich rc, so it did not do the pattern work: %v", s.Name, err)
			}
		})
	}
}

// TestTheRichRCGateRefusesAWrongAnswer proves the gate above can bite.
//
// A guard nothing has ever tripped is a guard nobody knows the shape of. This
// one moves the expected answer to a value no substitution produces, and the
// mark must then *not* be drawn — which is the failure mode a refused
// construct produces, manufactured on purpose. Without this, a gate wired to
// a condition that is always true would pass every test in this file.
func TestTheRichRCGateRefusesAWrongAnswer(t *testing.T) {
	for _, s := range all(t) {
		if s.PatternWork == "" {
			continue
		}
		t.Run(s.Name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path, err := startupcost.WriteRichRC(s, dir)
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// The answer the gate compares against, moved out of reach.
			// Nothing this shell computes has that length.
			broken := replaceOnce(string(body), `= "9"`, `= "999999"`)
			if broken == string(body) {
				t.Fatalf("the rich rc no longer gates on the answer, so this test proves nothing:\n%s", body)
			}
			if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
				t.Fatal(err)
			}
			// Shortened, because this one concludes from *not* seeing the
			// mark and would otherwise sit out the full read timeout four
			// times over. Five seconds is a dozen times what the same rc
			// takes to reach a prompt when the gate lets it — measured
			// above, in the test that requires success and keeps the
			// generous timeout for that reason.
			short := withoutPromptMark(s)
			short.PromptTimeout = 5 * time.Second
			if _, err := startupcost.RunPrompt(short, dir); err == nil {
				t.Errorf("%s drew the mark for an answer the pattern work never produced, so the gate measures nothing", s.Name)
			}
		})
	}
}

func replaceOnce(s, old, new string) string {
	i := strings.Index(s, old)
	if i < 0 {
		return s
	}
	return s[:i] + new + s[i+len(old):]
}

// TestWriteRichRCRefusesASubjectWithNoPatternWork keeps the rich case from
// quietly decaying into the plain one.
//
// The pattern work is the entire reason the rich rc exists — the sourcing of
// forty files is the cheap half, and #1383 was in the other one. A subject
// that lost its PatternWork would still get a perfectly good rc, still reach
// a prompt, and still be timed, under a benchmark name promising it had been
// measured on something realistic. Refusing is the only answer that cannot be
// misread.
func TestWriteRichRCRefusesASubjectWithNoPatternWork(t *testing.T) {
	t.Parallel()
	bare := startupcost.Subject{Name: "no-pattern-work", RCName: ".bare"}
	if _, err := startupcost.WriteRichRC(bare, t.TempDir()); err == nil {
		t.Fatal("wrote a rich rc for a subject with no pattern work, so the rich case can silently become the plain one")
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
