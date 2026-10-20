// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `HUP` decides whether a session that is leaving sends SIGHUP to the jobs it
// is abandoning, and it is **on** in a fresh zsh.
//
// It was accepted and then ignored until #4509 — the name went into the
// recorded store, `disown`'s own comment said in as many words that this
// engine never sent the signal, and both states of the option produced a
// session that left its jobs running and said nothing. The send and the
// sentence are an interactive surface and are measured where one exists, in
// cmd/zsh; what is asked here is the wiring: the option moves
// [interp.Runner.SendsHangupToJobsAtExit] — the same switch bash reaches
// under `shopt -s huponexit`, not a second one — and reports itself off the
// same place.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — with `-f`.
func TestHupMovesTheSwitch(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"on in a shell nobody has asked", "[[ -o hup ]] && print on || print off\n", "on\n"},
		{"off once it is unset", "unsetopt hup\n[[ -o hup ]] && print on || print off\n", "off\n"},
		{
			// The namespace's own spellings reach it too — a single `no`
			// prefix, and the `nohup` name is the option's negation rather
			// than the command of the same spelling.
			"the negated spelling",
			"setopt nohup\n[[ -o nohup ]] && print no-on || print no-off\n",
			"no-on\n",
		},
		{
			"and on again",
			"unsetopt hup\nsetopt hup\n[[ -o hup ]] && print on || print off\n",
			"on\n",
		},
		{
			"the options parameter",
			"unsetopt hup\nzmodload zsh/parameter\nprint -r -- \"${options[hup]}\"\n",
			"off\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// And the listing, which is the half that already worked and must not be
// traded away: moving the state onto the switch would be no gain if the
// listing then described a shell that no longer exists.
//
// An option that defaults **on** is named in the `setopt` listing only once
// it has been turned off, under its `no` spelling — measured on zsh 5.9.2,
// 2026-09-25: `unsetopt hup; setopt` writes `nohup` among the deviations, and
// a shell nobody has asked writes neither spelling.
func TestHupIsListedOnlyWhenItIsOff(t *testing.T) {
	quiet, st := runZsh(t, t.TempDir(), "setopt\n")
	if st != 0 {
		t.Fatalf("status %d: %q", st, quiet)
	}
	if strings.Contains(quiet, "nohup\n") || strings.Contains(quiet, "\nhup\n") {
		t.Errorf("the listing of a fresh shell is %q; `hup` is at its default and is not a deviation", quiet)
	}
	out, st := runZsh(t, t.TempDir(), "unsetopt hup\nsetopt\n")
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if !strings.Contains(out, "nohup\n") {
		t.Errorf("the `setopt` listing is %q; an option turned off its default is named there", out)
	}
}

// The three axes the option gates, at this shell's answers. They are read
// only from a shell whose switch is on, so a preset that answered them the
// other way would be a shell that hangs up nothing, or hangs up the wrong
// jobs, with every option listing still reporting `hup`.
//
// Measured 2026-09-25 through a pseudo-terminal against zsh 5.9.2, each on a
// session started `-fiV +Z` — interactive, and not a login shell:
//
//	sleep 30 & / exit          `zsh: warning: 1 jobs SIGHUPed`, and the job is gone
//	one running, one STOPped   `1 jobs SIGHUPed`
//	two STOPped, none running  nothing at all
//	trap … EXIT / sleep 3 &    the warning, and then the trap's line
func TestTheHangupAxesAreZshs(t *testing.T) {
	sem := zsh.Semantics()
	for _, c := range []struct {
		name string
		got  interp.Answer
		want interp.Answer
	}{
		{"no login shell is required", sem.HangupAtExitNeedsALoginShell, interp.No},
		{"a stopped job is left alone", sem.HangupAtExitSkipsStoppedJobs, interp.Yes},
		{"and it is all before the EXIT trap", sem.HangupAtExitPrecedesTheExitTrap, interp.Yes},
	} {
		if c.got != c.want {
			t.Errorf("%s: the preset answers %v, want %v", c.name, c.got, c.want)
		}
	}
}
