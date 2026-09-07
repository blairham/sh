// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// NoclobberBlocksAppendCreate and the override that answers it. Both halves
// are asserted on the filesystem: the bug that produced them (#1247) reported
// status 0 throughout, so a test grading only the status cannot see either.

// runAppend runs src with the override marker in the grammar and the axis set,
// and returns the output, the status and a reader for the scratch directory.
func runAppend(t *testing.T, src string, blocks interp.Answer) (string, int, func(string) string) {
	t.Helper()
	var dir string
	out, st := runGrammar(t, src,
		func(d *syntax.Dialect) { d.ClobberOverrideMarker = true; d.AmpersandRedirect = true },
		func(r *interp.Runner) {
			r.Semantics.NoclobberBlocksAppendCreate = blocks
			dir = r.Dir
		})
	return out, st, func(name string) string {
		if dir == "" {
			t.Fatal("the runner had no Dir, so a relative redirect went into the source tree")
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "<absent>"
		}
		return string(b)
	}
}

// TestNoclobberBlocksAppendCreateYesRefusesToCreate is the axis answering Yes:
// under noclobber an append to a name that is not there is a refusal.
func TestNoclobberBlocksAppendCreateYesRefusesToCreate(t *testing.T) {
	for _, op := range []string{">>", "&>>"} {
		out, st, file := runAppend(t, "set -C; echo hi "+op+" f", interp.Yes)
		if st == 0 {
			t.Errorf("%s: status 0, want a refusal", op)
		}
		if out == "" {
			t.Errorf("%s: refused silently, which is the failure this axis exists to avoid", op)
		}
		if got := file("f"); got != "<absent>" {
			t.Errorf("%s: f holds %q, want no file at all", op, got)
		}
	}
}

// TestNoclobberBlocksAppendCreateNoCreatesTheFile is the axis answering No,
// which is what the standard requires: noclobber is about `>` and an append
// still creates.
func TestNoclobberBlocksAppendCreateNoCreatesTheFile(t *testing.T) {
	for _, op := range []string{">>", "&>>"} {
		out, st, file := runAppend(t, "set -C; echo hi "+op+" f", interp.No)
		if out != "" || st != 0 {
			t.Errorf("%s: got %q status %d, want a silent 0", op, out, st)
		}
		if got := file("f"); got != "hi\n" {
			t.Errorf("%s: f holds %q, want it created holding hi", op, got)
		}
	}
}

// TestTheAxisIsOnlyAskedUnderNoclobber. An append with the option off must
// never reach the axis, because an unanswered axis refuses — so a Runner
// built from a bare Semantics would otherwise be unable to append at all,
// which is the commonest redirection there is.
func TestTheAxisIsOnlyAskedUnderNoclobber(t *testing.T) {
	out, st, file := runAppend(t, "echo hi >> f", interp.Unspecified)
	if out != "" || st != 0 {
		t.Errorf("got %q status %d, want a silent 0: the axis was asked with noclobber off", out, st)
	}
	if got := file("f"); got != "hi\n" {
		t.Errorf("f holds %q, want it created holding hi", got)
	}
}

// TestTheAxisUnansweredUnderNoclobberIsRefusedByName is the other side of the
// same rule, and the one that keeps an unanswered axis honest: reached, it
// refuses and says which question it could not answer.
func TestTheAxisUnansweredUnderNoclobberIsRefusedByName(t *testing.T) {
	out, st, file := runAppend(t, "set -C; echo hi >> f", interp.Unspecified)
	if st == 0 {
		t.Error("status 0 for an unanswered axis, want a refusal")
	}
	if out == "" {
		t.Fatal("an unanswered axis refused silently")
	}
	if got := file("f"); got != "<absent>" {
		t.Errorf("f holds %q, want no file: the refusal let the open through", got)
	}
}

// TestTheOverrideMarkerCreatesWhereTheAxisRefuses is the point of the marker.
// With the axis at Yes, `>>` cannot create the file and each override
// spelling can — which is the only way to tell the override apart from a
// plain append at all.
func TestTheOverrideMarkerCreatesWhereTheAxisRefuses(t *testing.T) {
	for _, op := range []string{">>|", ">>!", "&>>|", "&>>!"} {
		out, st, file := runAppend(t, "set -C; echo hi "+op+" f", interp.Yes)
		if out != "" || st != 0 {
			t.Errorf("%s: got %q status %d, want a silent 0", op, out, st)
		}
		if got := file("f"); got != "hi\n" {
			t.Errorf("%s: f holds %q, want it created holding hi", op, got)
		}
		if got := file("!"); got != "<absent>" {
			t.Errorf("%s: wrote a file named `!` holding %q — the marker became a filename", op, got)
		}
	}
}

// TestTheOverrideMarkerTruncatesWhereNoclobberRefuses is the same claim for
// the truncating half, which is where noclobber's refusal is unanimous. The
// control is the plain `>` in the same cell: it must refuse, or the override
// rows below it prove nothing.
func TestTheOverrideMarkerTruncatesWhereNoclobberRefuses(t *testing.T) {
	out, st, file := runAppend(t, "set -C; echo one > f; echo two > f", interp.No)
	if st == 0 {
		t.Error("plain > over an existing file under noclobber: status 0, want a refusal")
	}
	if out == "" {
		t.Error("plain > over an existing file under noclobber refused silently")
	}
	if got := file("f"); got != "one\n" {
		t.Errorf("plain > under noclobber: f holds %q, want the original", got)
	}
	for _, op := range []string{">|", ">!", "&>|", "&>!"} {
		out, st, file := runAppend(t, "set -C; echo one > f; echo two "+op+" f", interp.No)
		if out != "" || st != 0 {
			t.Errorf("%s: got %q status %d, want a silent 0", op, out, st)
		}
		if got := file("f"); got != "two\n" {
			t.Errorf("%s: f holds %q, want it truncated to two", op, got)
		}
		if got := file("!"); got != "<absent>" {
			t.Errorf("%s: wrote a file named `!` holding %q", op, got)
		}
	}
}
