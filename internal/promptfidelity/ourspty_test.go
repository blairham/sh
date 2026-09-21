// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package promptfidelity

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The source half, against a real binary on a real terminal.
//
// fidelity_test.go grades written-out screens, which says nothing about
// whether a *shell* can be driven into one — and a harness that graded
// correctly and could not reach a prompt would report every row as "it
// never became ready" while every unit test passed.
//
// **A two-row prompt**, because that is the spec's rule for this instrument
// and because a one-row prompt cannot tell a frame that is correct in its
// pieces from one that is wrong in its nesting. The configuration here puts
// the directory on the upper row and the prompt character on the row being
// typed, and both rows are asserted.

// buildShell builds a dialect binary for the test, or skips where the
// toolchain is not reachable.
func buildShell(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain to build a shell with")
	}
	binary := filepath.Join(t.TempDir(), "fidelity-zsh")
	build := exec.Command("go", "build", "-o", binary, "github.com/blairham/sh/cmd/zsh")
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("building a shell to drive: %v\n%s", err, out)
	}
	return binary
}

// The whole source, end to end: a real binary, a real terminal, a
// configuration, and a two-row prompt on the grid afterwards.
func TestOursDrawsATwoRowPromptOnARealTerminal(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "prompt.conf")
	// Two rows, a color that is not the terminal's default, and a directory
	// whose name the test wrote — so the assertions are about this prompt
	// and not about anything a shell draws by default.
	// DIR_MAX_DEPTH, because a scratch directory's path is longer than the
	// terminal and a wrapped upper row would put the second row of the
	// *directory* where the prompt character is — which is the shape a
	// one-row assertion cannot tell from a working prompt.
	const conf = `
LEFT_ELEMENTS = dir newline prompt_char
ICONS = none
DIR_MAX_DEPTH = 1
DIR_FOREGROUND = 31
PROMPT_CHAR_OK_FOREGROUND = 76
PROMPT_CHAR_SYMBOL = "%"
`
	if err := os.WriteFile(config, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	where := filepath.Join(dir, "somewhere-particular")
	if err := os.MkdirAll(where, 0o700); err != nil {
		t.Fatal(err)
	}

	ours := Ours{Binary: buildShell(t), Config: config}
	if ok, why := ours.Available(); !ok {
		t.Fatalf("the source says it cannot run: %s", why)
	}
	grid, err := ours.Render(Context{Dir: where, Home: dir, Columns: 60})
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}

	if !strings.Contains(grid.Text(0), "somewhere-particular") {
		t.Errorf("the upper row is not the directory:\n%s", grid.String())
	}
	if !strings.Contains(grid.Text(1), "%") {
		t.Errorf("the row being typed on is not the prompt character:\n%s", grid.String())
	}
	// And in color, because a harness that captured a prompt without its
	// appearance would grade every row on text alone — which is the blind
	// spot the cell grid exists to close, reached through the one path that
	// could reintroduce it.
	if got := grid.Cell(0, 0); got.Fg.String() != "31" {
		t.Errorf("the directory is drawn fg=%s, want 31 — the capture lost the color", got.Fg)
	}
	if got := grid.Cell(1, 0); got.Fg.String() != "76" {
		t.Errorf("the prompt character is drawn fg=%s, want 76", got.Fg)
	}
	// The echo of what the harness typed is gone: a comparison against a
	// program that was never typed at would otherwise report the echo as a
	// difference in every row.
	if strings.Contains(grid.String(), "printf") {
		t.Errorf("the harness's own typing is on the screen:\n%s", grid.String())
	}
}

// And the context reaches the prompt: a pinned status is the status the
// segment draws.
func TestAPinnedStatusReachesTheSegment(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "prompt.conf")
	const conf = `
LEFT_ELEMENTS = status newline prompt_char
ICONS = none
`
	if err := os.WriteFile(config, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}

	ours := Ours{Binary: buildShell(t), Config: config}
	grid, err := ours.Render(Context{Dir: dir, Home: dir, Status: 42, Columns: 60})
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	if !strings.Contains(grid.Text(0), "42") {
		t.Errorf("the pinned status is not on the prompt:\n%s", grid.String())
	}
}
