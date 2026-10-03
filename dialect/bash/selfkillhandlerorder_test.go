// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **What the shell sends itself is handled before `kill` returns**, and so
// before the rest of an and-or list. Measured 2026-10-02 in bash 5.3.20, bash
// 3.2.57, dash, ksh93u+, zsh 5.9.2 and BusyBox ash alike (#5359).
func TestASelfSentSignalIsHandledBeforeKillReturns(t *testing.T) {
	src := `trap 'echo T' USR1; kill -USR1 $$ && echo a; { kill -USR1 $$; } && echo b; echo c`
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if want := "T\na\nT\nb\nc\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
