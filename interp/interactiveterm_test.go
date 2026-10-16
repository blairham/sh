// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// An interactive shell ignores a TERM it sends itself, untrapped, where every
// other shell dies of it.
//
// Measured 2026-10-16, `kill -TERM $$; echo alive` under `-i -c` and at a
// pseudo-terminal: bash 5.3, bash 3.2, ksh93u+, dash, zsh 5.9.2 and BusyBox
// ash 1.37.0 write `alive` and end at 0; without `-i` every one of them ends
// at 143, which is the pair that says the noun is the shell being
// interactive. `trap - TERM` takes the ignore away in zsh alone, the column
// whose reset is the default action for QUIT too
// (Semantics.QuitResetRestoresTheDefault). And in ksh93, which runs a
// subshell in the shell's own process, the subshell is what the signal ends:
// `(kill -TERM $$; echo inner); echo st=$?` is `st=271` there and `inner`,
// `st=0` everywhere else (#6718).
func TestAnInteractiveShellIgnoresItsOwnTerm(t *testing.T) {
	const termKilled = 128 + int(syscall.SIGTERM)
	for _, c := range []struct {
		name        string
		interactive bool
		restores    Answer
		runsOn      Answer
		src         string
		wantOut     string
		wantStatus  int
	}{
		{
			"interactive, it is ignored", true, Unspecified, Yes,
			"kill -TERM $$\necho alive\n", "alive\n", 0,
		},
		{
			"not interactive, it ends the shell", false, Unspecified, Yes,
			"kill -TERM $$\necho alive\n", "", termKilled,
		},
		{
			"a reset that keeps the ignore", true, No, Yes,
			"trap - TERM\nkill -TERM $$\necho alive\n", "alive\n", 0,
		},
		{
			"a reset that restores the default", true, Yes, Yes,
			"trap - TERM\nkill -TERM $$\necho alive\n", "", termKilled,
		},
		{
			"QUIT takes the same reset", true, Yes, Yes,
			"trap - QUIT\nkill -QUIT $$\necho alive\n", "", 128 + int(syscall.SIGQUIT),
		},
		{
			"a subshell that runs on", true, Unspecified, Yes,
			"(kill -TERM $$; echo inner); echo st=$?\n", "inner\nst=0\n", 0,
		},
		{
			"a subshell in the shell's own process", true, Unspecified, No,
			"(kill -TERM $$; echo inner); echo st=$?\n", "st=143\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := killSem()
			sem.QuitIgnoredWhenNotInteractive = No
			sem.QuitResetRestoresTheDefault = c.restores
			sem.SubshellRunsOnAfterSignalingTheShell = c.runsOn
			f, err := syntax.Parse(c.src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			var o, e bytes.Buffer
			dg := Diagnostics{}
			r := newTestRunner(t, &Runner{
				Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &dg,
				Name: "testsh", Interactive: c.interactive,
			})
			st, rerr := r.Run(context.Background(), f)
			if rerr != nil {
				t.Fatalf("run: %v", rerr)
			}
			if o.String() != c.wantOut {
				t.Errorf("output %q, want %q", o.String(), c.wantOut)
			}
			if st != c.wantStatus {
				t.Errorf("status %d, want %d", st, c.wantStatus)
			}
			if e.String() != "" {
				t.Errorf("diagnostic %q, want none", e.String())
			}
		})
	}
}
