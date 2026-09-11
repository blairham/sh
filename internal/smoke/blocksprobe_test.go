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
