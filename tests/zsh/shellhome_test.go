// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// The home directory this shell holds, as opposed to the `HOME` parameter a
// script can see — #4654, and two halves of one mechanism.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), run `-f`; `go version -m` says *not a Go
// executable* for it. The binary was copied to files called `sh` and `ksh` so
// that nothing but the invocation name differs, and every row was read a
// second way — `--emulate <mode>` on the reference under its own name — which
// is what says the mode carries these and not the word.

// A shell started with no `HOME` at all seeds one from the password entry of
// the user the process runs as, under `zsh` and under no other mode:
//
//	env -u HOME <as zsh> -c 'print -r -- $HOME'    /Users/…
//	env -u HOME <as sh>  -c 'print -r -- $HOME'    nothing
//	env -u HOME <as ksh> -c 'print -r -- $HOME'    nothing
//
// bash is the control from the other side: `env -i bash -c 'echo "[$HOME]"'`
// is empty and a written `~` is a path all the same, which is a password entry
// read at the *tilde* rather than at startup and is TildeWithNoHome's
// question. See interp.Semantics.StartupFillsAnAbsentHome.
//
// It is also why the three sh-family rows of `CdWithoutHomeIsAnError` are
// reachable at all — under `zsh` there is no shell without a `HOME` for that
// axis to answer about — so the pair is asserted together, per mode.
func TestTheEmulationDecidesWhetherAnAbsentHomeIsFilledIn(t *testing.T) {
	if got := zsh.Semantics().StartupFillsAnAbsentHome; got != interp.Yes {
		t.Errorf("the preset answers %v, want interp.Yes", got)
	}
	for _, tc := range []struct {
		mode           string
		fills, nowhere interp.Answer
	}{
		{"zsh", interp.Yes, interp.No},
		{"sh", interp.No, interp.Yes},
		{"ksh", interp.No, interp.Yes},
		{"csh", interp.No, interp.Yes},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			// The vector after the builtin has run, because the seed itself
			// is the front end's — see interp.Runner.SeedHomeDirectory for
			// where in the startup sequence it happens and why the emulation
			// has to be in place first.
			r := preset.Runner(dialecttest.Base{Dir: t.TempDir()})
			b, ok := r.Builtin("emulate")
			if !ok {
				t.Fatal("no `emulate` builtin, so nothing here is being asked")
			}
			if st := b(r, context.Background(), []string{"--", tc.mode}); st != 0 {
				t.Fatalf("emulate %s: status %d", tc.mode, st)
			}
			if got := r.Semantics.StartupFillsAnAbsentHome; got != tc.fills {
				t.Errorf("emulate %s left StartupFillsAnAbsentHome %v, want %v", tc.mode, got, tc.fills)
			}
			// The pair, because the first is what makes the second reachable.
			if got := r.Semantics.CdWithoutHomeIsAnError; got != tc.nowhere {
				t.Errorf("emulate %s left CdWithoutHomeIsAnError %v, want %v", tc.mode, got, tc.nowhere)
			}
		})
	}
}

// And once a shell has had a home, removing the parameter leaves it an *empty*
// destination rather than no destination — the same silent 0 an inherited
// `HOME=` gives. Measured under `env -u HOME`, with the reference called `sh`
// so that its own name does not seed one:
//
//	cd                                     1, `HOME not set`
//	HOME=/tmp; unset HOME; cd              0, nothing
//	HOME=;     unset HOME; cd              0, nothing
//	f(){ HOME=/tmp; }; f; unset HOME; cd   0, nothing
//	(HOME=/tmp); cd                        1, `HOME not set`
//
// The first row is the control and it is what says the rows under it are about
// having had a home: every column that can refuse refuses a `cd` in a shell
// that never had one. bash 5.3.20, bash 3.2.57 and ksh93 answer 1 on every
// row. The same answer under every emulation here, which is what keeps it off
// the emulation table beside `cdNowhere`.
//
// See interp.Semantics.CdRemembersAHomeThatWasUnset.
func TestCdRemembersAHomeThatWasUnset(t *testing.T) {
	if got := zsh.Semantics().CdRemembersAHomeThatWasUnset; got != interp.Yes {
		t.Errorf("the preset answers %v, want interp.Yes", got)
	}
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src string
		wantSaid  bool
	}{
		{"never had one", "cd", true},
		{"assigned, then unset", "HOME=/somewhere; unset HOME; cd", false},
		{"assigned empty, then unset", "HOME=; unset HOME; cd", false},
		{"a function's assignment counts", "f(){ HOME=/somewhere; }; f; unset HOME; cd", false},
		{"a subshell's does not", "(HOME=/somewhere; unset HOME); cd", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Under `emulate sh`, because a zsh under its own name never has
			// an absent `HOME` to be asked about — the axis above has
			// already filled one in, and every row here would read 0 for the
			// wrong reason.
			out, _, err := preset.Combined(t, dialecttest.Base{
				Dir: dir,
				// No `HOME` at all, which is the case under test — and
				// assembled rather than inherited, so the suite's own
				// scratch home does not answer these rows for us.
				Env: []string{"PATH=" + dir},
			}, "emulate sh\n"+tc.src+"\n")
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if said := strings.Contains(out, "HOME not set"); said != tc.wantSaid {
				t.Errorf("said %q, want `HOME not set` present=%v", out, tc.wantSaid)
			}
		})
	}
}
