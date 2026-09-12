// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// The order this shell publishes the letters of `$-` in, and it is the only
// sorted one in the panel: the whole string by byte, so the digits lead, the
// capitals follow and the lowercase letters come last.
//
// Measured on zsh 5.9.2, 2026-09-12, the interactive row through a
// pseudo-terminal with a scratch home directory:
//
//	set -f; set -u; set -e   569Xefu
//	set -C                   569CX
//	set -C -e                569CXe
//	set -o noglob            569FX
//	set -e -C, on stdin      569CXes
//	-l -c                    569Xl
//	-i -c                    569XZim
//
// The `-C` rows are what make it a sort rather than an append: the capital
// lands in front of a startup letter the shell already held, where every
// other member of the panel keeps its startup letters together.
func TestDollarDashLetterOrder(t *testing.T) {
	if got, want := zsh.Semantics().DollarDashLetterOrder, "569BCEFHTXZacefhilmnstuvx"; got != want {
		t.Errorf("DollarDashLetterOrder = %q, want %q", got, want)
	}
}

// And the order as this shell writes it, against the startup letters `569X`.
func TestDollarDashIsWrittenInThatOrder(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set -f; set -u; set -e; echo "[$-]"`, "[569Xefu]"},
		{`set -C; echo "[$-]"`, "[569CX]"},
		{`set -C; set -e; echo "[$-]"`, "[569CXe]"},
		{`set -o noglob; echo "[$-]"`, "[569FX]"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if out != tc.want+"\n" {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want+"\n")
		}
	}
}

// This shell is the one that does not spend `-f` on globbing, and the letter
// still reaches `$-`: it stands for the startup files, whose name here is
// `norcs`, and the letter reports that name's state.
//
// Measured on zsh 5.9.2, 2026-09-12:
//
//	zsh -c 'set -f; setopt'          nohashdirs, norcs
//	zsh -c 'set -f; echo $-'         569Xf
//	zsh -c 'set -o norcs; echo $-'   569Xf
//	zsh -f -c 'echo $-'              569Xf
//	zsh -f -c 'set +f; setopt'       nohashdirs
//	zsh -f -c 'set +f; echo $-'      569X
//
// The third row is what makes this the option's state and not a note that the
// letter was written, and the last two are the same claim from the other end:
// the letter withdraws when the name does, on a route that never wrote it
// (#1542). Globbing is untouched throughout, which is the row below.
func TestTheFLetterStandsForTheStartupFiles(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the letter writes the name", `set -f; [[ -o norcs ]]; echo "rcs=$? [$-]"`, "rcs=0 [569Xf]"},
		{"and takes it back", `set -f; set +f; [[ -o norcs ]]; echo "rcs=$? [$-]"`, "rcs=1 [569X]"},
		{"the name alone brings the letter", `set -o norcs; echo "[$-]"`, "[569Xf]"},
		{"and losing the name takes it away", `set -f; set +o norcs; echo "[$-]"`, "[569X]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
			if err != nil {
				t.Fatalf("run %q: %v", tc.src, err)
			}
			if out != tc.want+"\n" {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want+"\n")
			}
		})
	}
}

// And the half that says the letter is not evidence for globbing being off:
// with `f` in `$-`, a pattern still matches. Five of the panel answer the
// pattern back unexpanded here.
func TestTheFLetterLeavesGlobbingAlone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zz.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	src := `set -f; case $- in *f*) echo letter;; *) echo none;; esac; echo zz.*`
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: dir}, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "letter\nzz.txt\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
