// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `NOTIFY` is a semantics axis here and not a bit in the recorded store.
//
// What the axis decides — when a finished job's notice is written — needs a
// terminal and a job, and is measured in cmd/zsh where one exists. What is
// asked here is the half that made the bug invisible for as long as it lasted:
// the option reported itself faithfully on every surface while the shell went
// on behaving as though it were on, so a test that only read the surfaces
// would have passed throughout (#4524).
func TestNotifyIsAnAxisAndNotAStoredBit(t *testing.T) {
	s := zsh.Semantics()
	if got, want := s.FinishedJobNoticeArrivesAtOnce, interp.Yes; got != want {
		t.Errorf("FinishedJobNoticeArrivesAtOnce = %v, want %v — `NOTIFY` is on out of the box here", got, want)
	}
}

// And the surfaces still report it, which is the half that must not be traded
// away: moving the state onto the axis would be no gain if `[[ -o … ]]`, the
// `options` parameter and the `setopt` listing then described a shell that no
// longer exists.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — with `-f` and each snippet in a file of its
// own.
func TestNotifyReportsItsState(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"on by default", "[[ -o notify ]] && print on || print off\n", "on\n"},
		{
			// The namespace's own spellings reach the axis too — a single
			// `no` prefix on the way in, and underscores ignored on the way
			// back out.
			"the underscored spelling",
			"unsetopt NOTIFY\n[[ -o no_tify ]] && print on || print off\n",
			"off\n",
		},
		{
			"the options parameter",
			"unsetopt notify\nzmodload zsh/parameter\nprint -r -- \"${options[notify]}\"\n",
			"off\n",
		},
		{
			// A subshell's change stays in the subshell, which is what the
			// axis buys over a stored bit.
			"subshell-local",
			"(unsetopt notify; [[ -o notify ]] && print in-on || print in-off)\n" +
				"[[ -o notify ]] && print out-on || print out-off\n",
			"in-off\nout-on\n",
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

// An option that defaults on is named in the `setopt` listing by its negation
// once it has been moved, and the listing reads the axis now that the axis is
// where the state lives. Measured on zsh 5.9.2, 2026-09-25: `unsetopt notify;
// setopt` writes `nohashdirs`, `nonotify` and `norcs`, in that order.
func TestNotifyIsListedWhenItIsOff(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "unsetopt notify\nsetopt\n")
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if !strings.Contains(out, "nonotify\n") {
		t.Errorf("the `setopt` listing is %q; an option turned off is named there by its negation", out)
	}
}
