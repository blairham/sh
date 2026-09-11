// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"fmt"
	"strings"
	"testing"
)

// Four `shopt` names read the opposite of bash 5.3, and the surface they read
// it on is a capture surface: `shopt -p` is snapshotted by an agent harness
// and sourced back ahead of every later command, so a state this shell will
// not read back is a complaint on standard error before each of them.
//
// Measured 2026-09-10 against bash 5.3.15 (`shopt -p`, with and without `-l`,
// a name at a time). Sourcing that shell's own snapshot into this one wrote
// seven `not implemented` lines; four of them are gone here and the other
// three are behaviors this shell genuinely does not have — see #1712 and the
// prose over shoptStates and shoptRecorded.

// TestTheCompletionNamesReadBashsDefault: `complete_fullquote` is this
// implementation's own state — the completer backslashes every shell
// metacharacter in a name it offers — and the other three are recorded at
// bash's default because the option decides what a *completer* offers and
// this one offers nothing behind any of them.
func TestTheCompletionNamesReadBashsDefault(t *testing.T) {
	for _, name := range []string{
		"complete_fullquote", "force_fignore", "hostcomplete", "progcomp",
	} {
		t.Run(name, func(t *testing.T) {
			want := fmt.Sprintf("%-20s\ton\n", name)
			out, st := runBash(t, t.TempDir(), "shopt "+name)
			if out != want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, want)
			}
		})
	}
}

// TestASnapshotOfThoseNamesReadsBackSilently is the failure the issue was
// filed on, written as it was measured: the four lines a real bash writes
// about these names are re-applied here without a word.
func TestASnapshotOfThoseNamesReadsBackSilently(t *testing.T) {
	const snapshot = `shopt -s complete_fullquote
shopt -s force_fignore
shopt -s hostcomplete
shopt -s progcomp
echo ok
`
	out, st := runBash(t, t.TempDir(), snapshot)
	if out != "ok\n" || st != 0 {
		t.Errorf("out %q status %d, want the four lines to say nothing", out, st)
	}
}

// TestARecordedShoptNameRemembersAndPromisesNothing: the three that are
// recorded move in both directions, exactly as bash moves them, and the move
// stays inside a subshell because the store is in the variable table a clone
// deep-copies. What none of them buys is behavior, which is the whole of
// what "recorded" claims.
func TestARecordedShoptNameRemembersAndPromisesNothing(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		"shopt -u progcomp; shopt -p progcomp; shopt -s progcomp; shopt -p progcomp; "+
			"( shopt -u hostcomplete; shopt -p hostcomplete ); shopt -p hostcomplete")
	want := strings.Join([]string{
		"shopt -u progcomp",
		"shopt -s progcomp",
		"shopt -u hostcomplete",
		"shopt -s hostcomplete",
		"",
	}, "\n")
	if out != want || st != 0 {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// TestTheStateNamesStillRefuseAMoveTheyCannotHonour: the other side of the
// bargain, unchanged. `complete_fullquote` is a claim about this completer,
// so turning it off would be promising to stop quoting.
func TestTheStateNamesStillRefuseAMoveTheyCannotHonour(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "shopt -u complete_fullquote")
	if !strings.Contains(out, "shopt: complete_fullquote: not implemented") || st != 1 {
		t.Errorf("out %q status %d, want the refusal at 1", out, st)
	}
}
