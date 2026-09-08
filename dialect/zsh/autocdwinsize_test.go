// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `setopt autocd` is implemented here and not merely recorded, and it is the
// same switch bash's `shopt -s autocd` moves.
//
// The behavior is asserted rather than the bit, and from the direction that
// separates this shell from the other one that has the option: zsh moves in
// *silence*, where bash writes `cd -- sub` first. Measured 2026-09-08 through
// a pseudo-terminal against zsh 5.9.2 started `-f`, since the name is
// interactive-only in both shells and a `-c` probe shows neither doing
// anything.
func TestAutoCdMovesTheShellInSilence(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	interactive := dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": t.TempDir()}, Interactive: true,
	}
	out, st, err := preset.Combined(t, interactive, `setopt auto_cd; sub; pwd`)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "sub") + "\n"; out != want || st != 0 {
		t.Errorf("out %q status %d, want %q status 0 — moved, and with nothing said", out, st, want)
	}
	// And a script does not get it, which is measured in this shell too:
	// `zsh -f -c 'setopt autocd; sub'` is `command not found` at 127.
	script := dialecttest.Base{Dir: dir, Vars: map[string]string{"PATH": t.TempDir()}}
	out, _, err = preset.Combined(t, script, `setopt auto_cd; sub; pwd`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sub") || !strings.HasSuffix(out, dir+"\n") {
		t.Errorf("in a script: out %q, want a refusal and no move", out)
	}
}

// This shell keeps $LINES and $COLUMNS abreast of the window with no option
// name for it at all, so the assertion is on the state a fresh runner starts
// in — there is nothing a script could run to turn it on or off.
//
// Measured 2026-09-08 through a pseudo-terminal against zsh 5.9.2 started
// `-f`: `COLUMNS=80 LINES=24` at the first prompt on an 80x24 terminal, and
// `132`/`40` at the next prompt after a resize.
func TestThisShellTracksTheWindowSizeWithNoNameForIt(t *testing.T) {
	r := preset.Runner(dialecttest.Base{Dir: t.TempDir()})
	if !r.TracksWindowSize() {
		t.Error("a fresh zsh does not track the window size, but real zsh assigns both variables unasked")
	}
	// And no name in the option namespace reaches it, which is what makes the
	// default the whole of the answer.
	for _, name := range []string{"checkwinsize", "check_window_size"} {
		if _, known := r.DialectOption(name); known {
			t.Errorf("%s is a name in this shell's namespace; real zsh has none for this", name)
		}
	}
}
