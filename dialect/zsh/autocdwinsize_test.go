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

// This shell keeps $LINES and $COLUMNS as parameters of its own, which is a
// stronger claim than bash's `checkwinsize` and is asserted as behavior rather
// than as a bit.
//
// The bit is what this test used to read, and a bit cannot tell the two
// readings apart. bash has the pair only when it is *interactive* and leaves
// both unset otherwise; here they are the shell's own — present under plain
// `-c` with no prompt anywhere, `0` rather than unset where there is no
// terminal to ask, and typed. Measured 2026-09-11 against zsh 5.9.2, each line
// under `-f -c` on a pipe:
//
//	${COLUMNS-UNSET}   0                 and `UNSET` in the other five columns
//	${(t)COLUMNS}      integer-special
//	COLUMNS="3+4"      7                 an expression, because of the type
//
// The terminal half — the numbers a real window gives, and a window that
// changes under a running script — needs a pseudo-terminal and is measured in
// interp, where the parameter is produced. See TestWindowSizeFollowsTheTerminal.
func TestThisShellHasTheWindowSizeAsParametersOfItsOwn(t *testing.T) {
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		`print -r -- "[${COLUMNS-UNSET}][${LINES-UNSET}][${(t)COLUMNS}][${(t)LINES}]"; `+
			`COLUMNS="3+4"; print -r -- "[$COLUMNS]"`)
	if err != nil {
		t.Fatal(err)
	}
	const want = "[0][0][integer-special][integer-special]\n[7]\n"
	if out != want || st != 0 {
		t.Errorf("out %q status %d, want %q status 0 — a shell with no window still has both names", out, st, want)
	}
	// And no name in the option namespace reaches it, which is what makes
	// this the whole of the answer: there is nothing a script could set.
	r := preset.Runner(dialecttest.Base{Dir: t.TempDir()})
	for _, name := range []string{"checkwinsize", "check_window_size"} {
		if _, known := r.DialectOption(name); known {
			t.Errorf("%s is a name in this shell's namespace; real zsh has none for this", name)
		}
	}
}
