// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package printer_test

import (
	"bytes"
	"context"
	"os/exec"
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
		if _, err := exec.LookPath(shell); err != nil {
			t.Skipf("%s not installed", shell)
		}
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
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
