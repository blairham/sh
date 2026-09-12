// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package smoke

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The block-store rows, asked of every shell rather than of the two the suite
// drives.
//
// The suite's own command models bash and zsh in full and drives only those,
// so the two rows grading the block store have never been asked of `sh`. The
// recording lives in repl, which all three binaries share, which means the sh
// column was untested for a reason unrelated to whether it works.
//
// These rows are in an ordinary test rather than only in the smoke command
// because they are the deterministic part of that suite: an `echo`, an exit
// status, and a file. Nothing here suspends a job, resumes one, or depends on
// an external command being on PATH — which is what keeps the rest of the
// suite out of `make check`.

// The shells this asks, built once for the whole run into a directory of their
// own. Built here rather than named in the environment so that the rows run
// wherever the tests do; a variable nobody sets is a row that is green by
// never having been asked.
var (
	probeOnce sync.Once
	probeDir  string
	probeErr  error
	probeLog  string
)

func buildProbeShells() {
	probeDir, probeErr = os.MkdirTemp("", "smoke-blocks")
	if probeErr != nil {
		return
	}
	for bin, pkg := range map[string]string{
		"sh":   "./cmd/sh",
		"bash": "./cmd/bash",
		"zsh":  "./cmd/zsh",
	} {
		cmd := exec.Command("go", "build", "-o", filepath.Join(probeDir, bin), pkg)
		cmd.Dir = "../.."
		if out, err := cmd.CombinedOutput(); err != nil {
			probeErr, probeLog = err, string(out)
			return
		}
	}
}

func probeShell(tb testing.TB, name string) string {
	tb.Helper()
	probeOnce.Do(buildProbeShells)
	if probeErr != nil {
		tb.Skipf("cannot build the shells to drive: %v\n%s", probeErr, probeLog)
	}
	return filepath.Join(probeDir, name)
}

// shDialect is the sh column, and it carries no prompt escapes on purpose.
// These rows do not grade a prompt — an escape this dialect spells differently
// would fail them for a reason that is not theirs — and the session still
// synchronizes, because what it waits on is the anchor both prompts end in.
func shDialect() Dialect {
	return Dialect{Name: "sh", RCFile: ".shrc", DefaultPrompt: "$ "}
}

func TestBlocksEveryShell(t *testing.T) {
	for _, d := range []Dialect{shDialect(), Bash(), Zsh()} {
		t.Run(d.Name, func(t *testing.T) {
			ctx := context.Background()
			dir, err := home(t.TempDir(), d)
			if err != nil {
				t.Fatalf("scratch home: %v", err)
			}
			s := &session{dialect: d, home: dir, bin: probeShell(t, d.Name), path: os.Getenv("PATH")}
			if err := s.start(ctx); err != nil {
				t.Fatalf("no session: %v\nstartup drew: %s", err, s.startupDrawn())
			}
			defer s.stop()

			// The line, the status it succeeded with, and the output it kept.
			if err := s.runProbe(blockProbe); err != nil {
				t.Fatalf("the line to record did not run: %v\nscreen: %s", err, s.drawn())
			}
			r, body, err := s.blockFor(ctx, blockProbe.line)
			if err != nil {
				t.Fatalf("%v", err)
			}
			if r.Status != 0 {
				t.Errorf("recorded status %d for a line that succeeded", r.Status)
			}
			// The body as well as the record: a store that kept the line and
			// lost what it printed is half a feature, and the two halves ship
			// separately by design.
			if !strings.Contains(body, blockProbe.mark) {
				t.Errorf("the record kept no output holding %q: %q", blockProbe.mark, Readable(body))
			}

			// And a failing line carries its own status rather than the
			// session's. Asked in the same session as the succeeding one, so a
			// shell recording a constant fails one of the two.
			if err := s.recover(); err != nil {
				t.Fatalf("back to a prompt: %v", err)
			}
			if err := s.runProbe(blockStatusProbe); err != nil {
				t.Fatalf("the failing line did not run: %v\nscreen: %s", err, s.drawn())
			}
			r2, _, err := s.blockFor(ctx, blockStatusProbe.line)
			if err != nil {
				t.Fatalf("%v", err)
			}
			if r2.Status != blockFailStatus {
				t.Errorf("recorded status %d, and the line exited with %d", r2.Status, blockFailStatus)
			}
		})
	}
}

// The rc line that turns on "a line beginning with a space is not remembered".
//
// A whole line rather than an option name, because the two shells keep the
// rule in different kinds of thing — zsh in an option, bash in a variable —
// and a field holding only the name would have to be read differently per
// dialect anyway. A dialect with no entry has no spelling for the rule and is
// not asked: `sh` has neither, so there is nothing there to honour.
var ignoreSpaceSetting = map[string]string{
	"zsh":  "setopt hist_ignore_space\n",
	"bash": "HISTCONTROL=ignorespace\n",
}

// The line the session is told to forget, and the one typed after it.
//
// Marks that are not in either line, for the reason every probe here needs
// one: a wait on text the line contains is answered by the terminal's echo of
// the keystrokes, so the row would pass for a shell that ran nothing.
//
// hiddenProbe's leading space is the gesture itself and is load-bearing
// whitespace. The store keeps a line exactly as typed, so it is also what the
// lookup afterwards matches on.
var (
	hiddenProbe      = probe{" echo hidden-$((6 * 7))-ran", "hidden-42-ran"}
	afterHiddenProbe = probe{"echo after-$((6 * 7))-recorded", "after-42-recorded"}
)

// A line the session was told to forget is not kept as a block either (#2273).
//
// The gesture is a leading space and it means "run this but do not write it
// down" — the same sentence an empty HISTFILE says about a whole session,
// scoped to one line. The store honoured the session-wide version from the
// day it was written and not this one, so a line a person had deliberately
// hidden was kept anyway, with its output, for as long as the store lived.
//
// Three things are asked in one session, and the order is what makes the
// absence mean anything:
//
//   - An ordinary line IS recorded. Without this the row passes on a shell
//     that records nothing at all, which is the same shape "nothing was
//     recorded" has when it is correct.
//   - A line is typed after the hidden one and waited for. The index is
//     append-only, so once that record is there the hidden one is not merely
//     late.
//   - The history file, read after a clean exit, kept the control line and
//     not the hidden one. That is the premise rather than the finding: it
//     says the rule was actually on, so a rc file that failed to apply fails
//     here, where it reads as what it is, instead of silently turning the
//     real assertion into a row that cannot fail.
func TestALineToldToBeForgottenIsNotKeptAsABlock(t *testing.T) {
	for _, d := range []Dialect{Bash(), Zsh()} {
		t.Run(d.Name, func(t *testing.T) {
			setting, ok := ignoreSpaceSetting[d.Name]
			if !ok {
				t.Skipf("%s has no spelling for the rule", d.Name)
			}
			ctx := context.Background()
			dir, err := home(t.TempDir(), d)
			if err != nil {
				t.Fatalf("scratch home: %v", err)
			}
			rc, err := os.OpenFile(filepath.Join(dir, d.RCFile), os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				t.Fatalf("rc file: %v", err)
			}
			if _, err := rc.WriteString(setting); err != nil {
				t.Fatalf("rc file: %v", err)
			}
			if err := rc.Close(); err != nil {
				t.Fatalf("rc file: %v", err)
			}

			s := &session{dialect: d, home: dir, bin: probeShell(t, d.Name), path: os.Getenv("PATH")}
			if err := s.start(ctx); err != nil {
				t.Fatalf("no session: %v\nstartup drew: %s", err, s.startupDrawn())
			}
			defer s.stop()

			// The control, so a session that records nothing cannot pass.
			if err := s.runProbe(blockProbe); err != nil {
				t.Fatalf("the control line did not run: %v\nscreen: %s", err, s.drawn())
			}
			if _, _, err := s.blockFor(ctx, blockProbe.line); err != nil {
				t.Fatalf("the control line was not recorded, so this session cannot answer the question: %v", err)
			}

			// The line the session was told to forget, and one after it.
			if err := s.recover(); err != nil {
				t.Fatalf("back to a prompt: %v", err)
			}
			if err := s.runProbe(hiddenProbe); err != nil {
				t.Fatalf("the hidden line did not run: %v\nscreen: %s", err, s.drawn())
			}
			if err := s.recover(); err != nil {
				t.Fatalf("back to a prompt: %v", err)
			}
			if err := s.runProbe(afterHiddenProbe); err != nil {
				t.Fatalf("the line after it did not run: %v\nscreen: %s", err, s.drawn())
			}
			if _, _, err := s.blockFor(ctx, afterHiddenProbe.line); err != nil {
				t.Fatalf("the line after the hidden one was not recorded: %v", err)
			}

			if err := s.noBlockFor(ctx, hiddenProbe.line); err != nil {
				t.Errorf("%v", err)
			}

			// And the premise: the rule was on, so the file declined it too.
			if err := s.recover(); err != nil {
				t.Fatalf("back to a prompt: %v", err)
			}
			if err := s.typeLine("exit"); err != nil {
				t.Fatalf("exit: %v", err)
			}
			if _, err := s.waitForExit(); err != nil {
				t.Fatalf("the session did not end: %v", err)
			}
			written, err := os.ReadFile(filepath.Join(dir, ".sh_history"))
			if err != nil {
				t.Fatalf("history file: %v", err)
			}
			if !strings.Contains(string(written), blockProbe.line) {
				t.Fatalf("the history file kept no ordinary line, so the rule cannot be read from it: %q",
					Readable(string(written)))
			}
			if strings.Contains(string(written), hiddenProbe.mark) ||
				strings.Contains(string(written), strings.TrimPrefix(hiddenProbe.line, " ")) {
				t.Errorf("the history file kept the hidden line, so the rule was never on: %q",
					Readable(string(written)))
			}
		})
	}
}
