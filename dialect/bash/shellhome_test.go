// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// Nothing is seeded into an absent `HOME` at startup here, and a `HOME` that
// has been removed is an absent one rather than an empty one — #4654.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/bash` (5.3.20) and `/bin/bash`
// (3.2.57), both with `--norc`; `go version -m` says *not a Go executable* for
// each. `env -i bash -c 'echo "[$HOME]"'` is empty, so nothing is seeded — and
// a written `~` is a path all the same, which is the password entry read at
// the *tilde* rather than at startup and is TildeWithNoHome's question.
//
// And under `env -u HOME`, `cd` and `HOME=/tmp; unset HOME; cd` are both
// `cd: HOME not set` at 1, where zsh answers the second at 0 in silence. The
// first row is the control: every column that can refuse refuses a `cd` in a
// shell that never had a home, so what the second says is that this one does
// not remember having had one.
//
// See interp.Semantics.StartupFillsAnAbsentHome and
// interp.Semantics.CdRemembersAHomeThatWasUnset.
func TestTheShellHoldsNoHomeOfItsOwn(t *testing.T) {
	if got := bash.Semantics().StartupFillsAnAbsentHome; got != interp.No {
		t.Errorf("StartupFillsAnAbsentHome = %v, want interp.No", got)
	}
	if got := bash.Semantics().CdRemembersAHomeThatWasUnset; got != interp.No {
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
			if st == 0 || !strings.Contains(out, "HOME not set") {
				t.Errorf("`%s` said %q at %d, want %s and a non-zero status",
					tc.src, out, st, "HOME not set")
			}
		})
	}
}
