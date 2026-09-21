// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// POSIX mode changes three things about `.` at once, and they are one
// change: the standard gives the builtin `$PATH` and nothing else.
//
// Measured 2026-09-21 on bash 5.3.20 under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`, each probe with `echo after` on the same line behind it, from a
// directory holding a readable `cwdf`:
//
//	                          bash                        set -o posix
//	. cwdf                    reads it, 0, runs on        `.: cwdf: file not found`, 1, ends
//	. notthere                `notthere: No such …`, runs on   `.: notthere: file not found`, 1, ends
//	source notthere           the same, naming nothing    `source: notthere: file not found`
//	command . notthere        —                           reports, 1, and the script runs on
//	set -o posix; set +o posix; . cwdf   —                 reads it again, 0
//
// The fatality is the standard's own answer and every column takes it —
// bash 5.3.20, bash 3.2.57, dash 0.5.12 and zsh 5.9.2 all leave the line
// after a failed `.` unrun under the `sh` name, where plain bash and plain
// zsh run it. The fallback is not: BusyBox ash has one and has no POSIX
// mode, so that half is a dialect's answer rather than a written-in No. See
// Semantics.DotFallsBackToCurrentDirectoryInPosixMode.
func TestPosixModeGivesDotThePathAndNothingElse(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cwdf"), []byte("echo cwd-hit\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// PATH deliberately does not hold the run directory, because the
	// fallback is only reachable once the search has missed — with the
	// directory on PATH every probe below finds the file the ordinary way
	// and the mode moves nothing visible.
	const pathOff = "PATH=/usr/bin:/bin\n"

	for _, tc := range []struct {
		name   string
		src    string
		want   []string
		unwant []string
		status int
	}{{
		name:   "the current directory is off the search",
		src:    pathOff + "set -o posix\n. cwdf\necho after\n",
		want:   []string{".: cwdf: file not found"},
		unwant: []string{"cwd-hit", "after"},
		status: 1,
	}, {
		name:   "and the miss is fatal",
		src:    pathOff + "set -o posix\n. notthere\necho after\n",
		want:   []string{".: notthere: file not found"},
		unwant: []string{"after"},
		status: 1,
	}, {
		// The verb is the word the script wrote, which is the half that was
		// a written-in dot even where the sentence was already right.
		name:   "and `source` names itself",
		src:    pathOff + "set -o posix\nsource notthere\n",
		want:   []string{"source: notthere: file not found"},
		status: 1,
	}, {
		// The control for all three: outside the mode the fallback reads the
		// file, the miss is the errno sentence, and the script runs on.
		name:   "none of it outside the mode",
		src:    pathOff + ". cwdf\n. notthere\necho after\n",
		want:   []string{"cwd-hit", "notthere: No such file or directory", "after"},
		unwant: []string{"file not found"},
		status: 0,
	}, {
		// Leaving the mode reaches this dialect's own answer and not the
		// standard's opposite, which is what the saved fields are for.
		name:   "and leaving it puts both back",
		src:    pathOff + "set -o posix\nset +o posix\n. cwdf\n. notthere\necho after\n",
		want:   []string{"cwd-hit", "notthere: No such file or directory", "after"},
		status: 0,
	}, {
		// The fatality goes through the usage door, so `command` catches it
		// exactly as it catches every other fatal failure of this builtin.
		name:   "and `command` survives it",
		src:    pathOff + "set -o posix\ncommand . notthere\necho after\n",
		want:   []string{".: notthere: file not found", "after"},
		status: 0,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if st != tc.status {
				t.Errorf("status %d, want %d (output %q)", st, tc.status, out)
			}
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("output = %q, want %q in it", out, w)
				}
			}
			for _, w := range tc.unwant {
				if strings.Contains(out, w) {
					t.Errorf("output = %q, want no %q in it", out, w)
				}
			}
		})
	}
}

// The two axes the mode reads, as this dialect answers them.
func TestTheDotAxesPosixModeMoves(t *testing.T) {
	s := bash.Semantics()
	if got, want := s.DotFallsBackToCurrentDirectoryInPosixMode, interp.No; got != want {
		t.Errorf("DotFallsBackToCurrentDirectoryInPosixMode = %v, want %v", got, want)
	}
	// The plain half is the control: an axis whose two halves agreed would
	// be a mode that moves nothing, and this one is the whole finding.
	if got, want := s.DotFallsBackToCurrentDirectory, interp.Yes; got != want {
		t.Errorf("DotFallsBackToCurrentDirectory = %v, want %v", got, want)
	}
}
