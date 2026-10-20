// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// This shell has two aliases before it reads anything: `run-help=man` and
// `which-command=whence`. Measured 2026-09-26 on zsh 5.9.2
// (aarch64-apple-darwin25.4.0), run `-f` with `env -u FPATH` over a script
// file — they survive `-f`, so they are the shell's own and not a startup
// file's (#4597).
//
// Through the prelude rather than pasted in front of the snippet, which is
// the thing that makes this a test of the dialect: an alias a test wrote
// itself would pass with the prelude empty.
func TestTheTwoStartupAliases(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(), `alias`)
	want := "run-help=man\nwhich-command=whence\n"
	if out != want || st != 0 {
		t.Errorf("a bare alias = %q (status %d), want %q", out, st, want)
	}
	// And they are in the text of the dialect rather than in a table only Go
	// can see, which is what lets a script redefine or drop them.
	for _, name := range []string{"run-help", "which-command"} {
		if !strings.Contains(zsh.Prelude(), "alias "+name+"=") {
			t.Errorf("the prelude defines no %s alias", name)
		}
	}
}

// The control the issue asks for, and the reason it is worth writing down:
// the refusal is correctly worded and correctly numbered for a name that
// really is absent, so what these rows show is that these two names are not.
func TestUnaliasReachesTheStartupAliases(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(), `unalias run-help;      print "A=$?"
unalias which-command; print "B=$?"
unalias run-help;      print "C=$?"
alias;                 print "D=$?"`)
	want := "A=0\nB=0\n" +
		"zsh:unalias:3: no such hash table element: run-help\nC=1\n" +
		"D=0\n"
	if out != want || st != 0 {
		t.Errorf("unalias over the startup aliases = %q (status %d), want %q", out, st, want)
	}
}

// They expand, which is the half a table consulted only by `alias` would not
// reach: `which-command` is `whence` and `run-help` is `man`.
func TestTheStartupAliasesExpand(t *testing.T) {
	// Parsed with the runner's alias tables in hand, which is the only route
	// that takes the alias at all — see
	// dialecttest.Preset.CombinedThroughTheAliases.
	out, st, err := preset.CombinedThroughTheAliases(t, dialecttest.Base{Dir: t.TempDir()},
		`which-command print
print "st=$?"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "print\nst=0\n"
	if out != want || st != 0 {
		t.Errorf("which-command = %q (status %d), want %q", out, st, want)
	}
}
