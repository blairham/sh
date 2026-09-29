// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `clobberempty` narrows `noclobber`: a plain `>` may truncate an existing
// regular file with nothing in it.
//
// `A04redirect.ztst` stops on "CLOBBER_EMPTY", which sets **both** options
// and checks that the first write lands in an empty file and the second is
// refused once that file has bytes in it. The option was recorded here —
// remembered and reported and read by nothing.
//
// Measured 2026-09-29 on zsh 5.9.2:
//
//	                       noclobber   noclobber + clobberempty
//	file is empty          refused     truncated, status 0
//	file has bytes in it   refused     refused
//	file does not exist    created     created
//
// The rows check the file's contents as well as the status, because a
// refusal that still truncated would report 1 and leave the file empty,
// which is the failure this option is most likely to produce.
func TestClobberEmptyNarrowsNoclobber(t *testing.T) {
	for _, tc := range []struct {
		name, opts, setup, wantBody string
		wantRefused                 bool
	}{
		{"an empty file may be truncated", "noclobber clobberempty", ": > foo", "W\n", false},
		{"one with bytes in it may not", "noclobber clobberempty", "print seed > foo", "seed\n", true},
		{"and a missing name is created", "noclobber clobberempty", ":", "W\n", false},
		// Without the narrowing, the empty file is refused like any other.
		{"noclobber alone refuses the empty one", "noclobber", ": > foo", "", true},
		// And `unsetopt` puts that back, which is what keeps it a switch.
		{"and taking it off again restores that", "noclobber clobberempty\nunsetopt clobberempty", ": > foo", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			out, st, errs := runZshSplit(t, dir,
				"setopt "+tc.opts+"\n"+tc.setup+"\nprint W > foo\nprint -r -- \"st=$?\"\n")
			// The status is read from the script's own `print`, not from the
			// runner's: the row under test is redirecting standard output at
			// the file it is about, so nothing is reported through it.
			wantSt := "st=0\n"
			if tc.wantRefused {
				wantSt = "st=1\n"
				if errs == "" {
					t.Errorf("no diagnostic, want the refusal")
				}
			} else if errs != "" {
				t.Errorf("stderr %q, want silence", errs)
			}
			if out != wantSt || st != 0 {
				t.Errorf("out %q status %d, want %q", out, st, wantSt)
			}
			body, err := os.ReadFile(filepath.Join(dir, "foo"))
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if string(body) != tc.wantBody {
				t.Errorf("file holds %q, want %q", body, tc.wantBody)
			}
		})
	}
}

// The boundary, which is the **size** and only for a regular file.
//
// A directory is refused with both options on, and a symlink is decided by
// what it points at — the same resolution every other open here uses.
//
// **A device and a fifo are deliberately not here.** With both options on
// they are allowed, and they are allowed by `noclobber` alone too, so no row
// built from them could tell this option working from this option missing.
// That is the control this test would otherwise have mistaken for evidence.
func TestWhatCountsAsEmpty(t *testing.T) {
	t.Run("a directory is not an empty file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "foo"), 0o755); err != nil {
			t.Fatal(err)
		}
		out, _, errs := runZshSplit(t, dir,
			"setopt noclobber clobberempty\nprint W > foo\nprint -r -- \"st=$?\"\n")
		if out != "st=1\n" || errs == "" {
			t.Errorf("out %q err %q, want the refusal", out, errs)
		}
	})
	for _, tc := range []struct {
		name, target, wantBody string
		wantSt                 string
	}{
		{"a symlink to an empty file follows it", "", "W\n", "st=0\n"},
		{"and a symlink to a full one likewise", "seed\n", "seed\n", "st=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "real"), []byte(tc.target), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("real", filepath.Join(dir, "foo")); err != nil {
				t.Fatal(err)
			}
			out, _, _ := runZshSplit(t, dir,
				"setopt noclobber clobberempty\nprint W > foo\nprint -r -- \"st=$?\"\n")
			if out != tc.wantSt {
				t.Errorf("out %q, want %q", out, tc.wantSt)
			}
			body, err := os.ReadFile(filepath.Join(dir, "real"))
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if string(body) != tc.wantBody {
				t.Errorf("target holds %q, want %q", body, tc.wantBody)
			}
		})
	}
}

// The operators the narrowing does not reach, and the one it does.
//
// `>|` is the override and `>>` is a different question, so neither is this
// option's; `&>` truncates exactly as `>` does and follows it.
func TestWhichOperatorsTheNarrowingReaches(t *testing.T) {
	for _, tc := range []struct{ name, setup, op, wantSt string }{
		{"the override is already allowed", "print seed > foo", ">|", "st=0\n"},
		{"appending is a different question", "print seed > foo", ">>", "st=0\n"},
		{"both streams follow `>` on a full file", "print seed > foo", "&>", "st=1\n"},
		{"and on an empty one", ": > foo", "&>", "st=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, _ := runZshSplit(t, t.TempDir(),
				"setopt noclobber clobberempty\n"+tc.setup+"\nprint W "+tc.op+"foo\nprint -r -- \"st=$?\"\n")
			if out != tc.wantSt {
				t.Errorf("out %q, want %q", out, tc.wantSt)
			}
		})
	}
}

// When the retry itself fails, the refusal's own wording stands rather than
// the second open's reason — which is the tail this shares with the branch
// for a device that will not open, so the two cannot drift.
//
// Measured: `chmod 000` on the empty file gives `file exists: foo`, not
// `permission denied: foo`. Writing this branch its own tail is how it would
// have said the second thing.
func TestAFailedRetryKeepsTheRefusalsWording(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	out, _, errs := runZshSplit(t, dir,
		"setopt noclobber clobberempty\nprint W > foo\nprint -r -- \"st=$?\"\n")
	if out != "st=1\n" {
		t.Errorf("out %q, want st=1", out)
	}
	if errs == "" || !strings.Contains(errs, "file exists") {
		t.Errorf("stderr %q, want the refusal's own wording", errs)
	}
}
