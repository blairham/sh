// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/testenv"
)

// This suite starts shells, so it runs in a home of its own — a shell reads
// startup files and writes history from the environment it was handed, and a
// suite that hands it the developer's environment is measuring the developer's
// dotfiles. internal/testenv is the guard; its own package comment is the
// argument for its shape.
func TestMain(m *testing.M) {
	if testenv.Assembled() {
		os.Exit(m.Run())
	}
	os.Exit(testenv.Run("cmd/ash", m))
}

// `+i` asks for a prompt here, where the other six columns take one away —
// Semantics.PlusSignedInteractiveLetterStillPrompts, and this is the column
// that makes it an axis (#3221).
//
// Measured 2026-09-16 on BusyBox ash 1.37.0 in the pinned Alpine image, the
// program on a pipe so that nothing but the invocation could make the shell
// interactive: `ash +i -c 'echo $-'` writes `can't access tty; job control
// turned off` and reports `ci`, byte for byte with `ash -i -c`, and so do
// `ash -i +i -c` and `ash +i -i -c`. A shell started with neither letter
// reports `c` and says nothing about a terminal.
//
// This column has been wrong five separate times for want of being asked
// (#3228), and the sign is exactly the kind of detail a reader borrows from
// the other six. Pinned here rather than left to the axis vector, because a
// value nothing reads back is a claim and not a measurement.
func TestThePlusSignedLetterStillAsksForAPrompt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		argv    []string
		prompts bool
	}{
		{"the plus alone", []string{"ash", "+i"}, true},
		{"the letter, for comparison", []string{"ash", "-i"}, true},
		{"the letter then its plus", []string{"ash", "-i", "+i"}, true},
		{"the plus then the letter", []string{"ash", "+i", "-i"}, true},
		{"neither letter", []string{"ash"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var o, e bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr = &o, &e
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			go func() {
				_, _ = w.WriteString("echo TYPED\n")
				_ = w.Close()
			}()
			t.Cleanup(func() { _ = r.Close() })
			sh.Stdin = r
			code := driver.MainArgs(sh, tc.argv)
			both := o.String() + e.String()
			if code != 0 {
				t.Fatalf("status %d, said %q", code, both)
			}
			if !strings.Contains(both, "TYPED") {
				t.Fatalf("said %q, want the typed line to have run under either answer", both)
			}
			if got := strings.Contains(both, "$ "); got != tc.prompts {
				t.Errorf("prompted = %v, want %v — said %q", got, tc.prompts, both)
			}
		})
	}
}
