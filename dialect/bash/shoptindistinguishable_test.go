// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"fmt"
	"strings"
	"testing"
)

// Three `shopt` names whose two states cannot be told apart, recorded at
// bash's own default and acted on by nothing — the second ground for a place
// in shoptRecorded, added by #4149.
//
// The distinction that earns it is measured on the **reference**, not on this
// shell. For `extquote` and `noexpand_translation`, bash 5.3.15 itself answers
// identically in both states over every shape I could construct: eight for
// `extquote` — a default value, an alternate value, a pattern removal, a
// substitution, `$"…"`, a nested expansion, an assignment and a bare `$'…'`
// word — and five for `noexpand_translation`, including with `TEXTDOMAIN` and
// `TEXTDOMAINDIR` exported. There is no behavior to build.
//
// `globasciiranges` is unfalsifiable for a different reason and the difference
// is stated rather than blurred: bash *would* show it outside the C locale, and
// the suite runs every file under `LC_ALL=C`, so there the two states are one
// state and no observation could tell an implementation from a no-op.
//
// What this file guards is that recording claims nothing while still doing the
// two things recording is for: reading bash's default, and moving both ways.

// TestTheIndistinguishableNamesReadBashsDefault. The listing must not move
// when a name changes table — it is a capture surface, and this is the
// assertion that would fail if one of them were recorded at *this* shell's
// state instead of bash's.
func TestTheIndistinguishableNamesReadBashsDefault(t *testing.T) {
	for name, on := range map[string]bool{
		"extquote":             true,
		"globasciiranges":      true,
		"noexpand_translation": false,
		// The two completion names #4149 moved, on the first and third
		// grounds respectively — see shoptRecorded.
		"complete_fullquote": true,
		"progcomp_alias":     false,
	} {
		t.Run(name, func(t *testing.T) {
			state, status := "off", 1
			if on {
				state, status = "on", 0
			}
			want := fmt.Sprintf("%-20s\t%s\n", name, state)
			// `shopt name` answers 0 for an option that is on and 1 for one
			// that is off, so the status is the state read a second way and is
			// asserted beside the line rather than waived. Measured on bash
			// 5.3.15 the same day.
			out, st := runBash(t, t.TempDir(), "shopt "+name)
			if out != want || st != status {
				t.Errorf("out %q status %d, want %q at %d", out, st, want, status)
			}
		})
	}
}

// TestTheIndistinguishableNamesMoveBothWays is what they were refused for.
// Each direction is asserted with its status, because a refusal at 1 is the
// state this replaces and a silent grant of the state already held would pass
// a test that only checked one side.
func TestTheIndistinguishableNamesMoveBothWays(t *testing.T) {
	for _, name := range []string{
		"extquote", "globasciiranges", "noexpand_translation",
		"complete_fullquote", "progcomp_alias",
	} {
		t.Run(name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), strings.Join([]string{
				"shopt -s " + name + "; echo \"s=$?\"",
				"shopt -p " + name,
				"shopt -u " + name + "; echo \"u=$?\"",
				"shopt -p " + name,
			}, "\n"))
			want := "s=0\nshopt -s " + name + "\nu=0\nshopt -u " + name + "\n"
			// The call's own status is 1, and that is bash's rule rather than
			// this option's: the last line asks about a name that is off, and
			// `shopt -p name` answers 1 for an option that is unset exactly as
			// `shopt name` does. The two writes are the assertion, and each
			// carries its own status on the line above it.
			if out != want || st != 1 {
				t.Errorf("out %q status %d, want %q at 1", out, st, want)
			}
		})
	}
}

// TestARecordedMoveStaysInItsSubshell: the store is in the variable table a
// clone deep-copies, so a move inside a subshell does not escape. The same
// property the three completion names have, asserted for the new three because
// it is the store's and not the name's.
func TestARecordedMoveStaysInItsSubshell(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		"( shopt -u extquote; shopt -p extquote ); shopt -p extquote")
	want := "shopt -u extquote\nshopt -s extquote\n"
	if out != want || st != 0 {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// TestASnapshotOfTheIndistinguishableNamesReadsBackSilently is the capture
// surface again, and the reason recording is worth its cost: a real bash's own
// `shopt -p` sourced into this shell used to write a `not implemented` line
// for each of these ahead of every command that followed.
func TestASnapshotOfTheIndistinguishableNamesReadsBackSilently(t *testing.T) {
	const snapshot = `shopt -s extquote
shopt -s globasciiranges
shopt -u noexpand_translation
shopt -s complete_fullquote
shopt -u progcomp_alias
echo ok
`
	out, st := runBash(t, t.TempDir(), snapshot)
	if out != "ok\n" || st != 0 {
		t.Errorf("out %q status %d, want the three lines to say nothing", out, st)
	}
}

// TestTheHuponexitNameIsListedAndMoves: the option is a capability now, not a
// state — see interp.Runner.SendsHangupToJobsAtExit for the four measured rows
// — and `mailwarn` is recorded beside it on the first ground, an option over a
// facility this shell does not have at all.
func TestTheHuponexitNameIsListedAndMoves(t *testing.T) {
	for _, name := range []string{"huponexit", "mailwarn"} {
		t.Run(name, func(t *testing.T) {
			out, st := answersRun(t, "shopt "+name+"\n"+
				"shopt -s "+name+"; echo \"s=$?\"\nshopt "+name+"\n"+
				"shopt -u "+name+"; echo \"u=$?\"\nshopt "+name)
			pad := name + strings.Repeat(" ", 20-len(name))
			want := pad + "\toff\ns=0\n" + pad + "\ton\nu=0\n" + pad + "\toff\n"
			if out != want || st != 1 {
				t.Errorf("status %d, output %q; want status 1 and %q", st, out, want)
			}
		})
	}
}
