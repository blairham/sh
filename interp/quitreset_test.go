// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os/signal"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestAResetRestoresTheDefaultOnAnIgnoredSignal covers the second of the two
// axes an untrapped SIGQUIT reaches.
//
// The first says whether the shell is ignoring the signal at all; this one
// says whether `trap -` takes that ignore away and hands the signal back its
// default action. It is reached only through the first and only after a
// reset, which is most of what these cases are checking: an axis nothing
// reaches is one a preset may leave unanswered without breaking any script,
// and three of the presets do.
//
// No signal is delivered here. The ignored path sends nothing, and the fatal
// path records a death that DieBySignal — nil for a Runner that is not a
// shell — never carries out.
func TestAResetRestoresTheDefaultOnAnIgnoredSignal(t *testing.T) {
	// `trap '' QUIT` calls signal.Ignore for the whole test binary, and one
	// case below needs it to say that a later ignore undoes the reset.
	t.Cleanup(func() { signal.Reset(syscall.SIGQUIT) })

	const quitKilled = 128 + int(syscall.SIGQUIT)
	for _, c := range []struct {
		name       string
		ignored    Answer
		restores   Answer
		src        string
		wantOut    string
		wantStatus int
	}{
		{
			"a reset hands the signal back its default action",
			Yes, Yes,
			"trap - QUIT\nkill -QUIT $$\necho after\n",
			"", quitKilled,
		},
		{
			"a reset leaves the ignore standing",
			Yes, No,
			"trap - QUIT\nkill -QUIT $$\necho after\n",
			"after\n", 0,
		},
		{
			// Unspecified throughout the rest: an axis that is reached
			// writes a diagnostic and reports 2, so a clean run is the
			// evidence that nothing asked.
			"with no reset the question never arises",
			Yes, Unspecified,
			"kill -QUIT $$\necho after\n",
			"after\n", 0,
		},
		{
			"a reset of another signal is not this one",
			Yes, Unspecified,
			"trap - INT\nkill -QUIT $$\necho after\n",
			"after\n", 0,
		},
		{
			"an ignore set after the reset undoes it",
			Yes, Unspecified,
			"trap - QUIT\ntrap '' QUIT\nkill -QUIT $$\necho after\n",
			"after\n", 0,
		},
		{
			// The reason dash, ksh93 and bash 3.2 may leave it unanswered:
			// a shell that was never ignoring the signal is killed by it
			// with the reset or without, so both readings run the script
			// identically and neither has to be chosen.
			"a shell that never ignored it is killed either way",
			No, Unspecified,
			"trap - QUIT\nkill -QUIT $$\necho after\n",
			"", quitKilled,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := killSem()
			sem.QuitIgnoredWhenNotInteractive = c.ignored
			sem.QuitResetRestoresTheDefault = c.restores
			out, errs, st := killRun(t, c.src, sem, Diagnostics{})
			if out != c.wantOut {
				t.Errorf("output %q, want %q", out, c.wantOut)
			}
			if st != c.wantStatus {
				t.Errorf("status %d, want %d", st, c.wantStatus)
			}
			if errs != "" {
				t.Errorf("diagnostic %q, want none", errs)
			}
		})
	}
}

// And when it is reached unanswered it refuses by name rather than picking a
// side, which is what makes the six cases above evidence of anything.
func TestAnUnansweredResetAxisRefusesByName(t *testing.T) {
	sem := killSem()
	sem.QuitIgnoredWhenNotInteractive = Yes
	sem.QuitResetRestoresTheDefault = Unspecified
	_, errs, st := killRun(t, "trap - QUIT\nkill -QUIT $$\necho after\n", sem, Diagnostics{})
	if errs == "" {
		t.Error("no diagnostic, want the unanswered axis named")
	}
	if st != 2 {
		t.Errorf("status %d, want 2", st)
	}
}
