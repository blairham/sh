// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"strings"
	"testing"
)

// `fc`'s editor road, measured 2026-09-21 against bash 5.3.20 under `env -i`
// with a scratch `HOME` and `TMPDIR`, the list seeded by running the commands
// with `set -o history`, and the editor a recording stand-in on `PATH`.
//
// The machinery is the substrate's and is asserted in interp/fceditor_test.go
// against the seams rather than against a shell. What is here is the two
// things that are bash's own: the whole road end to end through this
// dialect's list, and the fallback editor's **name**, which is the one part
// of the road `set -o posix` moves and which no other column in the panel can
// even be asked about.

// The road, end to end. `/bin/cat` prints the file it was handed, so the
// output is what was written for the editor, then the shell echoing what came
// back, then that line running.
//
// An absolute path rather than a name on `PATH`, because the harness gives
// each run a `PATH` holding only its own directory — which is what the other
// case here needs, and which would otherwise make this one a lookup failure.
func TestFcEditsThroughTheEditorAndRunsWhatComesBack(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runBash(t, dir, "TMPDIR="+dir+"\nhistory -s 'echo one'\nfc -e /bin/cat\n")
	if want := "echo one\necho one\none\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
	// And the spool file is gone. Its directory is the run's own, so
	// anything left in it is ours.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the run's directory: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".sh-fc-") {
			t.Errorf("the spool file was left behind: %s", e.Name())
		}
	}
}

// The fallback editor is `vi`, and `ed` while `set -o posix` is on.
//
// Measured with neither `FCEDIT` nor `EDITOR` set: bash runs `vi`, and the
// same shell with the option on runs `ed`. That is the one row of this road
// no other column can answer — zsh has no `posix` option at all, and answers
// `vi` under `emulate sh` as well.
//
// Asserted through a `PATH` holding nothing that runs, so the name is
// **reported** and never started. That is not a way around the assertion: an
// editor is the one program a shell runs expecting a person, and a test that
// reached a real `vi` would hang until the run timed out.
func TestTheFallbackEditorIsViAndEdUnderPosix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"vi by default", "", "vi: command not found"},
		{"ed under posix", "set -o posix\n", "ed: command not found"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			out, st := runBash(t, dir, "TMPDIR="+dir+"\nhistory -s 'echo one'\n"+c.src+"fc\n")
			if st != 1 {
				t.Errorf("out = %q status = %d, want 1", out, st)
			}
			if !strings.HasSuffix(out, c.want+"\n") {
				t.Errorf("out = %q, want it to end with %q", out, c.want)
			}
			if strings.Contains(out, "one\n") {
				t.Errorf("out = %q, the entry ran without an editor", out)
			}
		})
	}
}

// And an editor that left the file **empty** is silence at 0 here, which is
// the other side of Semantics.FcEmptyEditIsAnError — zsh complains and ends
// the shell over the same editor, and that half is asserted in dialect/zsh.
//
// `cp /dev/null` is the editor, which truncates what it is handed and is also
// the second proof that the editor is a command line rather than a program
// name: two words, and the file is the third.
func TestAnEditorThatEmptiesTheFileIsSilentHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runBash(t, dir,
		"TMPDIR="+dir+"\nhistory -s 'echo one'\nfc -e '/bin/cp /dev/null'\necho after\n")
	if want := "after\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
}
