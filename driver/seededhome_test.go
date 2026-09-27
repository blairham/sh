// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A shell whose environment has no `HOME` seeds one from the password entry
// where its dialect says so, and the front end is what decides **when** —
// #4654.
//
// The seed has to happen after the emulation and before anything a person
// wrote. An emulation taken from `argv[0]` or from `--emulate` is what answers
// the axis, and this front end applies it *after* the dialect's prelude has
// run — so a seed inside interp's own per-chunk setup would fire on the
// prelude, under the preset's answer, and a binary called `sh` would fill in a
// home its mode says it does not. Measured: it did, until the call moved here.

// seedShell is a dialect whose preset seeds a home and whose emulation builtin
// turns that off, which is the shape zsh has: the mode decides, and the mode
// arrives after the prelude.
func seedShell(entry string, applied *bool) driver.Shell {
	sem := interp.PosixSemantics()
	sem.StartupFillsAnAbsentHome = interp.Yes
	sem.EmulationOption = nameInitials()
	return driver.Shell{
		Name:    "testsh",
		Dialect: syntax.Core(),
		// Assembled rather than inherited, and with no `HOME` in it at all —
		// which is the case under test. The suite's own environment carries
		// a scratch home (internal/testenv), so a shell handed that one has
		// nothing to seed and the rows below would agree for the wrong
		// reason.
		Env:       []string{"PATH=/usr/bin:/bin"},
		Semantics: sem,
		Register: func(r *interp.Runner) {
			r.UserHomeDir = func(name string) (string, bool) {
				if name != "" {
					return "", false
				}
				return entry, true
			}
			r.Register("become", func(rr *interp.Runner, _ context.Context, args []string) int {
				*applied = true
				// What the real one does for every axis it carries: replace
				// the vector, which from here on is what the shell answers.
				s := *rr.Semantics
				s.StartupFillsAnAbsentHome = interp.No
				rr.Semantics = &s
				return 0
			})
		},
	}
}

func TestAnAbsentHomeIsSeededAfterTheEmulation(t *testing.T) {
	const entry = "/the/password/entry"
	for _, c := range []struct {
		name  string
		argv0 string
		want  string
	}{
		// Under its own name no emulation is applied, so the preset stands
		// and the home is seeded.
		{"the dialect's own name", "testsh", entry},
		// Under a name that picks an emulation, the builtin has answered No
		// by the time the seed is asked — which is the ordering, and the row
		// that was wrong.
		{"a name that picks an emulation", "sh", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			applied := false
			sh := seedShell(entry, &applied)
			var out bytes.Buffer
			sh.Stdout, sh.Stderr = &out, &out
			if st := driver.MainArgs(sh, []string{c.argv0, "-c", `printf "[%s]" "$HOME"`}); st != 0 {
				t.Fatalf("status %d: %s", st, out.String())
			}
			if got := strings.TrimSpace(out.String()); got != "["+c.want+"]" {
				t.Errorf("$HOME = %s, want [%s] (emulation applied: %v)", got, c.want, applied)
			}
		})
	}
}

// And a `HOME` the environment did hand over is left alone, which is the
// control: the row above is about an absent one and not about the seed
// winning.
func TestASeededHomeDoesNotReplaceOne(t *testing.T) {
	applied := false
	sh := seedShell("/the/password/entry", &applied)
	sh.Env = []string{"HOME=/was/given", "PATH=/usr/bin:/bin"}
	var out bytes.Buffer
	sh.Stdout, sh.Stderr = &out, &out
	if st := driver.MainArgs(sh, []string{"testsh", "-c", `printf "[%s]" "$HOME"`}); st != 0 {
		t.Fatalf("status %d: %s", st, out.String())
	}
	if got := strings.TrimSpace(out.String()); got != "[/was/given]" {
		t.Errorf("$HOME = %s, want the one the environment gave", got)
	}
}
