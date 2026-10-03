// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **The order a bare `alias` lists in** — Semantics.AliasListingWalksTheTable.
// dash and BusyBox ash walk their table, by bucket and then by when a name was
// first defined; the others sort by name. Measured 2026-10-03 on dash 0.5.12
// and BusyBox ash in the pinned image:
//
//	alias aa=1 ab=1 ac=1 ba=1 bb=1 zz=1 za=1; alias   ba bb zz za aa ab ac
//	alias u=1 N=1; alias u=2; alias                    u N   (one bucket)
//	alias u=1 N=1; unalias u; alias u=3; alias         N u
func TestABareAliasListingWalksTheTableInDashAndAsh(t *testing.T) {
	src := "alias aa=1 ab=1 ac=1 ba=1 bb=1 zz=1 za=1; alias\n" +
		"unalias -a; alias u=1 N=1; alias u=2; alias\n" +
		"unalias -a; alias u=1 N=1; unalias u; alias u=3; alias\n"
	walked := "ba='1'\nbb='1'\nzz='1'\nza='1'\naa='1'\nab='1'\nac='1'\n" +
		"u='2'\nN='1'\n" +
		"N='1'\nu='3'\n"
	for _, name := range []string{"dash", "ash"} {
		out, _, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
		if err != nil {
			t.Fatal(err)
		}
		if out != walked {
			t.Errorf("%s: got %q, want %q", name, out, walked)
		}
	}
	// And a sorting dialect is untouched: bash lists by name.
	out, _, err := presets["bash"].Combined(t, dialecttest.Base{Dir: t.TempDir()}, "alias zz=1 aa=1; alias\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "alias aa='1'\nalias zz='1'\n"; out != want {
		t.Errorf("bash: got %q, want %q", out, want)
	}
}
