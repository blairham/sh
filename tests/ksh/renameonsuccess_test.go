// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `>;` is this shell's write that only lands if the command succeeded: the
// output goes to a temporary file in the target's own directory and is
// renamed over the target when the command ends at status 0. At any other
// status the target is left exactly as it was — and a target that was not
// there is still not there.
//
// Measured 2026-09-14 on ksh93u+ 2012-08-01, which is alone in the panel with
// it: bash 5.3.15, zsh 5.9.2 and dash all refuse the text with a syntax error
// at the `;` (#918).
func TestAWriteThatOnlyLandsIfTheCommandSucceeded(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		// before is written to the target first, or the target is absent
		// where it is empty.
		before string
		// want is what the target holds afterwards, and absent says the
		// target is not there at all.
		want   string
		absent bool
		status int
	}{
		{
			name: "a command that succeeds replaces the target",
			src:  "echo NEW >; t", before: "old\n", want: "NEW\n",
		},
		{
			name: "a command that fails leaves it alone",
			src:  "{ printf X; false; } >; t", before: "old\n", want: "old\n", status: 1,
		},
		{
			name: "a target that was not there is created on success",
			src:  "echo NEW >; t", want: "NEW\n",
		},
		{
			name: "and is not created on failure",
			src:  "{ printf X; false; } >; t", absent: true, status: 1,
		},
		{
			// The status the *whole* command ended at, not the last write.
			name: "a bare false with the redirection on it",
			src:  "false >; t", before: "old\n", want: "old\n", status: 1,
		},
		{
			name: "a descriptor number in front of it",
			src:  "echo NEW 1>; t", before: "old\n", want: "NEW\n",
		},
		{
			// It never truncates the target, so there is nothing for
			// noclobber to refuse — measured, ksh93 replaces the file.
			name: "noclobber does not reach it",
			src:  "set -C; echo NEW >; t", before: "old\n", want: "NEW\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "t")
			if tc.before != "" {
				if err := os.WriteFile(target, []byte(tc.before), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, st, err := preset.Combined(t, dialecttest.Base{Dir: dir}, tc.src)
			if err != nil {
				t.Fatalf("run %q: %v", tc.src, err)
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
			b, rerr := os.ReadFile(target)
			if tc.absent {
				if rerr == nil {
					t.Errorf("the target was created holding %q; a failed command creates nothing", b)
				}
			} else if rerr != nil {
				t.Errorf("reading the target: %v", rerr)
			} else if string(b) != tc.want {
				t.Errorf("target = %q, want %q", b, tc.want)
			}
			// Whichever way it went, the temporary must be gone: an
			// operator that leaves a hidden file beside every target it
			// touches is worse than the plain `>` it replaces.
			ents, derr := os.ReadDir(dir)
			if derr != nil {
				t.Fatal(derr)
			}
			for _, e := range ents {
				if e.Name() != "t" {
					t.Errorf("left behind %q", e.Name())
				}
			}
		})
	}
}

// The target's permissions survive the replacement. A rename brings the
// temporary file's own mode with it, so a file that had been made private
// would silently come back world-readable — measured on ksh93u+, `chmod 741`
// then `echo new >; f` leaves the mode at `-rwxr----x`.
func TestTheTargetKeepsItsModeAcrossTheRename(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "t")
	if err := os.WriteFile(target, []byte("old\n"), 0o741); err != nil {
		t.Fatal(err)
	}
	if _, _, err := preset.Combined(t, dialecttest.Base{Dir: dir}, "echo NEW >; t"); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0o741 {
		t.Errorf("mode = %v, want -rwxr----x", got)
	}
}

// And the whole reason #918 was filed: with `>;` in the grammar, `echo <->`
// is `echo` with `<-` and a `>;` whose target is the word after it. So the
// shell reports that it cannot open `-`, and what looked like the next
// command was this one's argument — `echo <->; echo done` prints no `done`,
// where `echo <->x`, having no `;` tight against the `>`, does.
//
// The diagnostic naming `-` rather than `->` was the clue: the `<` had
// already taken its operand.
func TestAngleDashAngleIsAReadFromDashAndARenameOnSuccess(t *testing.T) {
	for _, tc := range []struct {
		name, src, cannotOpen string
		reachesDone           bool
	}{
		{name: "the semicolon is swallowed by the operator", src: "echo <->; echo done", cannotOpen: "-"},
		{
			name: "a character after it leaves the semicolon alone",
			src:  "echo <->x; echo done", cannotOpen: "-", reachesDone: true,
		},
		{name: "the same with a range in front", src: "echo <1-9>; echo done", cannotOpen: "1-9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, tc.src)
			if err != nil {
				t.Fatal(err)
			}
			// The `<` took its operand, which is what the name in the
			// diagnostic says: `-` and not `->`.
			if !strings.Contains(out, tc.cannotOpen+": cannot open") {
				t.Errorf("out = %q, want a failed open of %q", out, tc.cannotOpen)
			}
			if got := strings.Contains(out, "done"); got != tc.reachesDone {
				t.Errorf("reached `done` = %v, want %v: %q", got, tc.reachesDone, out)
			}
		})
	}
}
