// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// The clobber-override marker, asserted on the filesystem rather than on an
// exit status. Status 0 is exactly what made #1247 invisible: `echo hi >>! f`
// reported success, wrote a file called `!`, and left the named file absent,
// so any test grading this by status alone would have passed against the bug.
//
// Measured on zsh 5.9.2, 2026-09-07, each with noclobber on and the target
// already there — the only cell in which an override decides anything:
//
//	$ setopt noclobber; echo one > f; echo two >!  f   # f holds `two`
//	$ setopt noclobber; echo one > f; echo two >>! f   # f holds `one` then `two`
//	$ setopt noclobber; echo two >>| f                 # f is created, holds `two`
//
// And the same three with `|` and `!` exchanged, and the same four again
// behind `&>` and `&>>`.

// markerRun runs one snippet in a scratch directory and hands back the
// combined output, the status, and a reader for the files it left behind.
func markerRun(t *testing.T, src string) (string, int, func(string) string) {
	t.Helper()
	dir := t.TempDir()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "sh", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	return out, st, func(name string) string {
		b, rerr := os.ReadFile(filepath.Join(dir, name))
		if rerr != nil {
			return "<absent>"
		}
		return string(b)
	}
}

// TestTheOverrideMarkerWritesTheFileItNames is the fix for #1247, in the cell
// where the override is the only thing that can produce the result: noclobber
// is on and the target already exists, so a plain `>` would refuse and a
// spelling read as `>` plus a word would write `!` instead.
func TestTheOverrideMarkerWritesTheFileItNames(t *testing.T) {
	for _, tc := range []struct {
		op   string
		want string
	}{
		{">|", "two\n"},
		{">!", "two\n"},
		{">>|", "one\ntwo\n"},
		{">>!", "one\ntwo\n"},
		{"&>|", "two\n"},
		{"&>!", "two\n"},
		{"&>>|", "one\ntwo\n"},
		{"&>>!", "one\ntwo\n"},
	} {
		out, st, file := markerRun(t, "setopt noclobber\necho one > f\necho two "+tc.op+" f\n")
		if st != 0 || out != "" {
			t.Errorf("%s: status %d and output %q, want a silent 0", tc.op, st, out)
		}
		if got := file("f"); got != tc.want {
			t.Errorf("%s: f holds %q, want %q", tc.op, got, tc.want)
		}
		// The half a status cannot see: nothing may be written to a file
		// named for the marker. This is the whole of the bug.
		if got := file("!"); got != "<absent>" {
			t.Errorf("%s: a file named `!` was written holding %q — the marker was read as a filename",
				tc.op, got)
		}
	}
}

// TestAnOverriddenAppendCreatesTheFile is the other half, and the one that
// needs the semantics axis as well as the grammar: here noclobber refuses to
// let `>>` create a missing file, so `>>|` and `>>!` are the only spellings
// that can produce one.
func TestAnOverriddenAppendCreatesTheFile(t *testing.T) {
	for _, op := range []string{">>|", ">>!", "&>>|", "&>>!"} {
		out, st, file := markerRun(t, "setopt noclobber\necho two "+op+" f\n")
		if st != 0 || out != "" {
			t.Errorf("%s: status %d and output %q, want a silent 0", op, st, out)
		}
		if got := file("f"); got != "two\n" {
			t.Errorf("%s: f holds %q, want it created holding two", op, got)
		}
	}
	// And the refusal being overridden, so the rows above are not vacuous.
	out, st, file := markerRun(t, "setopt noclobber\necho two >> f\n")
	if st != 1 {
		t.Errorf("plain >> under noclobber: status %d, want 1", st)
	}
	if out != "zsh:2: no such file or directory: f\n" {
		t.Errorf("plain >> under noclobber said %q, want the whole located refusal", out)
	}
	if got := file("f"); got != "<absent>" {
		t.Errorf("plain >> under noclobber created f holding %q, want no file", got)
	}
}

// TestNoclobberLetsAnAppendToAnExistingFileThrough is the control for the
// test above: the divergence is about *creating* a file and not about
// appending, so this cell must pass with no override at all.
func TestNoclobberLetsAnAppendToAnExistingFileThrough(t *testing.T) {
	out, st, file := markerRun(t, "setopt noclobber\necho one > f\necho two >> f\n")
	if st != 0 || out != "" {
		t.Errorf("status %d and output %q, want a silent 0", st, out)
	}
	if got := file("f"); got != "one\ntwo\n" {
		t.Errorf("f holds %q, want both lines", got)
	}
}

// TestTheOverrideMarkerOnACompoundCommand is the shape found in a real plugin
// tree — `done >>! …` — which is where the parse rather than the open is what
// the operator has to survive.
func TestTheOverrideMarkerOnACompoundCommand(t *testing.T) {
	out, st, file := markerRun(t,
		"setopt noclobber\nfor i in 1 2; do echo $i; done >>! f\n")
	if st != 0 || out != "" {
		t.Errorf("status %d and output %q, want a silent 0", st, out)
	}
	if got := file("f"); got != "1\n2\n" {
		t.Errorf("f holds %q, want both loop iterations", got)
	}
}

func TestClobberOverrideMarkerIsOn(t *testing.T) {
	if !zsh.Dialect().ClobberOverrideMarker {
		t.Error("zsh reads `>!`, `>>|`, `>>!` and the same four behind `&>`")
	}
}

func TestNoclobberBlocksAppendCreateIsYes(t *testing.T) {
	if got := zsh.Semantics().NoclobberBlocksAppendCreate; got != interp.Yes {
		t.Errorf("NoclobberBlocksAppendCreate = %v, want Yes: noclobber reaches `>>` here", got)
	}
}
