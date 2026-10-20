// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// `up-line-or-search` and `down-line-or-search` are names this shell knows.
//
// Not an advanced spelling somebody opts into: **macOS's `/etc/zshrc` binds
// the arrows to these**, so on that machine they are what Up and Down do in a
// shell with no user rc at all. Missing from the table, they were a key bound
// to nothing — the arrow did nothing whatever (#2435).

func TestTheSearchingWalkIsANameThisShellKnows(t *testing.T) {
	for name, want := range map[string]repl.Widget{
		"up-line-or-search":   repl.WidgetPreviousHistoryMatching,
		"down-line-or-search": repl.WidgetNextHistoryMatching,
	} {
		r, _ := zleRunner(t, "bindkey '^G' "+name+"\n")
		table := zsh.KeyBindings(r, repl.KeymapMain)
		got, bound := table["\a"]
		if !bound {
			t.Errorf("%s: the key is not in the table at all", name)
			continue
		}
		if got.Widget != want {
			t.Errorf("%s: bound to widget %d, want %d", name, got.Widget, want)
		}
		if got.Widget == repl.WidgetNone {
			t.Errorf("%s: bound to nothing, which is a key that does nothing", name)
		}
	}
}

// And the listing says them back, which is the half a `bindkey` with no
// operands has to answer for.
func TestTheSearchingWalkIsListedBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "bindkey '^G' up-line-or-search\nbindkey '^G'\n")
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	if !strings.Contains(out, "up-line-or-search") {
		t.Errorf("listing = %q, want the name back", out)
	}
}
