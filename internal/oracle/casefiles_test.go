// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// What Case.Files has to deliver, checked against the thing itself rather
// than against the field being set.
//
// The corpus rows are the other guard and the stronger one — a mechanism that
// quietly stopped writing would move `startup/a-login-shell-reads-a-profile`
// from `.bash_profile~main` to `main` in six columns and `make oracle -check`
// would call it drift — but they only speak on a machine that has the panel.
// These run anywhere.

// TestAFixtureFileLandsInTheShellsHomeBeforeItStarts. Both halves matter and
// neither implies the other: the file has to be *there*, and it has to be
// there in the directory the shell will look in, which is the same scratch
// directory the harness hands out as `$HOME`.
func TestAFixtureFileLandsInTheShellsHomeBeforeItStarts(t *testing.T) {
	t.Parallel()
	sh, ok := aShell(t)
	if !ok {
		return
	}
	got := Exec(context.Background(), sh, Case{
		ID:      "test/fixture",
		Files:   []File{{Name: ".a_profile", Contents: "marker\n"}},
		Args:    []string{"-c", ArgSnippet},
		Snippet: `cat "$HOME/.a_profile"; cat .a_profile`,
	})
	if want := "marker~marker"; got.Stdout != want {
		t.Errorf("stdout = %q, want %q (err %q, status %d)", got.Stdout, want, got.Stderr, got.Status)
	}
}

// TestAFixtureFileMayNameADirectoryThatIsNotThere. A startup directory a
// variable points at is one of the shapes this is for, and a case cannot
// mkdir before the shell starts by any other means.
func TestAFixtureFileMayNameADirectoryThatIsNotThere(t *testing.T) {
	t.Parallel()
	sh, ok := aShell(t)
	if !ok {
		return
	}
	got := Exec(context.Background(), sh, Case{
		ID:      "test/fixture-nested",
		Files:   []File{{Name: "zdot/.zshenv", Contents: "marker\n"}},
		Args:    []string{"-c", ArgSnippet},
		Snippet: `cat zdot/.zshenv`,
	})
	if got.Stdout != "marker" {
		t.Errorf("stdout = %q, want %q (err %q)", got.Stdout, "marker", got.Stderr)
	}
}

// TestAFixtureFileCannotClimbOutOfTheScratchDirectory.
//
// The corpus is source, and the machine that runs it is a machine somebody
// works on: a committed row naming `../../x` would write there once per shell
// per regeneration. Refused rather than sanitized, because a name that means
// something other than what it says is a case that measures something other
// than what it reads as.
func TestAFixtureFileCannotClimbOutOfTheScratchDirectory(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", "../escape", "a/../../escape", filepath.Join(t.TempDir(), "absolute"), "case.sh"} {
		c := Case{ID: "test/bad", Files: []File{{Name: name, Contents: "x"}}, Snippet: ":"}
		if err := c.validate(); err == nil {
			t.Errorf("Case.Files %q was accepted", name)
			continue
		}
		// And it is refused where a run would see it, not only where a test
		// calls validate: Exec validates first, and a harness error is not a
		// measurement.
		got := Exec(context.Background(), Found{Shell: Shell{Name: "none"}, Path: "/nonexistent"}, c)
		if got.Status != -1 || got.Stderr == "" {
			t.Errorf("Case.Files %q ran anyway: %+v", name, got)
		}
	}
}

// TestAGoodFixtureNameIsStillAccepted, because a refusal that refused
// everything would pass the test above and take the whole feature with it.
func TestAGoodFixtureNameIsStillAccepted(t *testing.T) {
	t.Parallel()
	for _, name := range []string{".profile", ".bash_profile", "zdot/.zshenv", "a.b..c"} {
		c := Case{ID: "test/good", Files: []File{{Name: name, Contents: "x"}}, Snippet: ":"}
		if err := c.validate(); err != nil {
			t.Errorf("Case.Files %q was refused: %v", name, err)
		}
	}
}

// TestTheStartupMarkersEchoTheirOwnNames. The corpus rows depend on it: a
// marker that printed anything else could not say *which* file was read,
// which is the only question those rows ask.
func TestTheStartupMarkersEchoTheirOwnNames(t *testing.T) {
	t.Parallel()
	got := startupMarkers(".bash_profile", ".profile")
	want := []File{
		{Name: ".bash_profile", Contents: "echo .bash_profile\n"},
		{Name: ".profile", Contents: "echo .profile\n"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d markers, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("marker %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// aShell is any panel member on this machine, for the two tests that need a
// real one. Which shell does not matter — the question is whether the file
// arrived, and every shell can `cat`.
func aShell(t *testing.T) (Found, bool) {
	t.Helper()
	found, _ := Resolve(context.Background())
	for _, f := range found {
		// Skip a column reached through a container: the fixture write is
		// the same code there, and the round trip costs seconds.
		if strings.Contains(f.Path, "docker") || f.Path == "" {
			continue
		}
		if _, err := os.Stat(f.Path); err == nil {
			return f, true
		}
	}
	t.Skip("no reference shell on this machine")
	return Found{}, false
}
