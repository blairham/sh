// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A private array whose assignment would not expand keeps its elements. The
// failure ends the function body, and an `always` block is where what the
// name holds afterwards can be read. Measured 2026-10-15 on zsh 5.9.2 from a
// file: `[array-local-hide-special][1 2]`. It stored the empty string, through
// the store a private array keeps its kind by (#6544).
func TestAPrivateArrayKeepsItsElementsWhenTheValueWouldNotExpand(t *testing.T) {
	src := "zmodload zsh/param/private\n" +
		"f() { private -a a=(1 2); { a=$((1/0)) } always { print -r -- \"[${(t)a}][$a]\"; TRY_BLOCK_ERROR=0 } }\n" +
		"f\n"
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if want := "f: division by zero\n[array-local-hide-special][1 2]\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}
