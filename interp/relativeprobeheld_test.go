// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A relative file test is asked of the directory the shell holds, not of the
// name it remembers, and the two part when the directory is deleted and a new
// one made under the same name while the shell sits in it. Measured
// 2026-10-15: zsh 5.9.2, bash 5.3, bash 3.2, ksh93 and dash all answer from
// the deleted directory — nothing is in it, and `.` is still a directory —
// where this shell answered from the new one (#6494). Five of five, so the
// rows name no shell. See heldForRelativeProbe.

// probeRows runs each file test in the shell's directory and prints one
// letter per test: y or n.
const probeRows = `for t in "-e f" "-f f" "-e ." "-d ." "-w f" "-L l"; do
  if [ $t ]; then printf y; else printf n; fi
done
if ( [ -e f ] ); then printf y; else printf n; fi
echo
`

func relativeProbeRunner(t *testing.T, dir string) (*Runner, *strings.Builder) {
	t.Helper()
	sem := PosixSemantics()
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: dir, Stdout: out, Stderr: out,
	})
	// Take hold of the directory, which a real shell's kernel does at the
	// chdir. See runPart.
	runPart(t, r, "cd .\n")
	return r, out
}

func withFileAndLink(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("f", filepath.Join(dir, "l")); err != nil {
		t.Fatal(err)
	}
}

func resolvedTempDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestARelativeFileTestAsksTheDirectoryHeld(t *testing.T) {
	t.Parallel()
	t.Run("control: a directory nothing has happened to", func(t *testing.T) {
		// The positive the rows below need: with the file there, every
		// letter that can say yes does, so a row reading n is the case and
		// not a probe that cannot see.
		d := filepath.Join(resolvedTempDir(t), "d")
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		withFileAndLink(t, d)
		r, out := relativeProbeRunner(t, d)
		runPart(t, r, probeRows)
		if got := out.String(); got != "yyyyyyy\n" {
			t.Errorf("got %q, want %q", got, "yyyyyyy\n")
		}
	})

	t.Run("deleted and made again under the same name", func(t *testing.T) {
		root := resolvedTempDir(t)
		d := filepath.Join(root, "d")
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		r, out := relativeProbeRunner(t, d)
		if err := os.RemoveAll(d); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		// The new directory holds everything the control found, so a shell
		// asking by name answers the control's yyyyyyy.
		withFileAndLink(t, d)
		runPart(t, r, probeRows)
		if got, want := out.String(), "nnyynnn\n"; got != want {
			t.Errorf("got %q, want %q: the answer came from the new directory "+
				"rather than the one the shell is in", got, want)
		}
	})

	t.Run("deleted and nothing in its place", func(t *testing.T) {
		d := filepath.Join(resolvedTempDir(t), "d")
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		r, out := relativeProbeRunner(t, d)
		if err := os.Remove(d); err != nil {
			t.Fatal(err)
		}
		runPart(t, r, probeRows)
		if got, want := out.String(), "nnyynnn\n"; got != want {
			t.Errorf("got %q, want %q: `.` is still a directory to every shell in the panel", got, want)
		}
	})

	t.Run("an ancestor made unsearchable after the shell entered", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("permission bits do not refuse root")
		}
		root := resolvedTempDir(t)
		d := filepath.Join(root, "p", "d")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		withFileAndLink(t, d)
		r, out := relativeProbeRunner(t, d)
		if err := os.Chmod(filepath.Join(root, "p"), 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "p"), 0o755) })
		// The control: by name, the file really is out of reach now.
		if _, err := os.Stat(filepath.Join(d, "f")); err == nil {
			t.Fatal("the file is still reachable by name; the case is not set up")
		}
		runPart(t, r, probeRows)
		// A relative path from a working directory never passes through its
		// parent, so all five shells still see the file.
		if got, want := out.String(), "yyyyyyy\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("the directory itself made unsearchable after the shell entered", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("permission bits do not refuse root")
		}
		// The case that rules out a hold checked only when it was opened:
		// zsh, bash and dash are refused every lookup here, and so must the
		// descriptor be. Darwin's open to pass through is not (#6494).
		d := filepath.Join(resolvedTempDir(t), "d")
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		withFileAndLink(t, d)
		r, out := relativeProbeRunner(t, d)
		if err := os.Chmod(d, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(d, 0o755) })
		runPart(t, r, probeRows)
		if got, want := out.String(), "nnnnnnn\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
