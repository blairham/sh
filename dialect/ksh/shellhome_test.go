// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// Nothing is seeded into an absent `HOME` at startup here, and a `HOME` that
// has been removed is an absent one rather than an empty one — #4654.
//
// Measured 2026-09-26 on `/bin/ksh` (AT&T ksh93u+ 2012); `go version -m` says
// *not a Go executable* for it. Under `env -u HOME`, `cd` and
// `HOME=/tmp; unset HOME; cd` are both `cd: bad directory` at 1, where zsh
// answers the second at 0 in silence — and `env -u HOME ksh -c 'print -r --
// $HOME'` writes nothing, so nothing is seeded either. The first row is the
// control: every column that can refuse refuses a `cd` in a shell that never
// had a home.
//
// See interp.Semantics.StartupFillsAnAbsentHome and
// interp.Semantics.CdRemembersAHomeThatWasUnset.
func TestTheShellHoldsNoHomeOfItsOwn(t *testing.T) {
	if got := ksh.Semantics().StartupFillsAnAbsentHome; got != interp.No {
		t.Errorf("StartupFillsAnAbsentHome = %v, want interp.No", got)
	}
	if got := ksh.Semantics().CdRemembersAHomeThatWasUnset; got != interp.No {
		t.Errorf("CdRemembersAHomeThatWasUnset = %v, want interp.No", got)
	}
	dir := t.TempDir()
	for _, tc := range []struct{ name, src string }{
		{"never had one", "cd"},
		// The discriminating row: the same shell, the same absent `HOME`,
		// and nothing between them but an assignment. zsh answers 0 and says
		// nothing here.
		{"assigned, then unset", "HOME=/somewhere; unset HOME; cd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{
				Dir: dir,
				// No `HOME` at all, which is the case under test — and
				// assembled rather than inherited, so the suite's own
				// scratch home does not answer these rows for us.
				Env: []string{"PATH=" + dir},
			}, tc.src+"\n")
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if st == 0 || !strings.Contains(out, "bad directory") {
				t.Errorf("`%s` said %q at %d, want %s and a non-zero status",
					tc.src, out, st, "bad directory")
			}
		})
	}
}
