// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `LONG_LIST_JOBS` decides whether a job *notice* names the job's process id.
//
// It was accepted and then ignored until #4491 — the option went into the
// recorded store, the notice went on being written without a pid, and the two
// states of it produced byte-identical output while every listing reported the
// difference back faithfully. The notice itself is an interactive surface and
// is measured where one exists, in cmd/zsh; what is asked here is the wiring:
// the option moves [interp.Semantics.JobNoticeNamesThePID] and reports itself
// off the same place.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — with `-f`.
func TestLongListJobsMovesTheAxis(t *testing.T) {
	sem := zsh.Semantics()
	if got := sem.JobNoticeNamesThePID; got != interp.No {
		t.Errorf("the preset answers %v, want No — the option is off in a shell nobody has asked", got)
	}
	for _, c := range []struct{ name, src, want string }{
		{"off in a shell nobody has asked", "[[ -o longlistjobs ]] && print on || print off\n", "off\n"},
		{"on once it is set", "setopt longlistjobs\n[[ -o longlistjobs ]] && print on || print off\n", "on\n"},
		{
			// The namespace's own spellings reach the axis too — a single
			// `no` prefix, and underscores ignored.
			"the underscored spelling",
			"setopt LONG_LIST_JOBS\n[[ -o long_list_jobs ]] && print on || print off\n",
			"on\n",
		},
		{
			"and off again",
			"setopt longlistjobs\nunsetopt nolonglistjobs\n[[ -o longlistjobs ]] && print on || print off\n",
			"on\n",
		},
		{
			"the options parameter",
			"setopt longlistjobs\nzmodload zsh/parameter\nprint -r -- \"${options[longlistjobs]}\"\n",
			"on\n",
		},
		{
			// The state is the semantics vector's rather than a bit in the
			// recorded store, which is what makes a subshell's change stay in
			// the subshell.
			"a subshell keeps its own answer",
			"(setopt longlistjobs; [[ -o longlistjobs ]] && print in-on || print in-off)\n" +
				"[[ -o longlistjobs ]] && print out-on || print out-off\n",
			"in-on\nout-off\n",
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

// An option that defaults off is named in the `setopt` listing once it has
// been set, and the listing reads the axis now that the axis is where the
// state lives — which is the half that already worked and must not be traded
// away: moving the state onto the axis would be no gain if the listing then
// described a shell that no longer exists.
//
// Measured on zsh 5.9.2, 2026-09-25: `setopt longlistjobs; setopt` writes
// `longlistjobs` among the deviations.
func TestLongListJobsIsListedWhenItIsOn(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "setopt longlistjobs\nsetopt\n")
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if !strings.Contains(out, "longlistjobs\n") {
		t.Errorf("the `setopt` listing is %q; an option turned on is named there", out)
	}
}
