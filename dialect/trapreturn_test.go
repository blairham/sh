// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// A bare `return` written in a trap action's own body —
// Semantics.TrapReturnStatus, and two of the four lines `suite: trap.tests`
// was still differing by (#4157).
//
// The probe puts every shell in front of the *same* number so that delivery
// timing cannot be what splits them: the action's first command prints `$?`
// and every column prints 4. What they then answer to `return` is the axis.
//
// Measured 2026-09-23, `env -i PATH=/usr/bin:/bin <shell> x.sh` over a script
// file, BusyBox ash in a busybox:latest container:
//
//	bash 5.3.20            entry 4    4      the status it was entered at
//	dash 0.5.12            entry 4    4      the same
//	BusyBox ash 1.37       entry 4    4      the same
//	zsh 5.9                entry 4    123    the action's own last command
//	ksh93u+ 2012-08-01     entry 4    0      zero
//
// The interrupted function never resumes in any of them — the `return` ends
// it — so what the caller reads is the number the axis decides.
func trapReturnPresets() []struct {
	dialecttest.Preset
	reading interp.TrapReturnReading
	want    string
} {
	return []struct {
		dialecttest.Preset
		reading interp.TrapReturnReading
		want    string
	}{
		{
			dialecttest.Preset{
				Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
				Diagnostics: bash.Diagnostics, Apply: bash.Apply,
			},
			interp.TrapReturnTakesTheStatusBeforeIt, "entry 4 exit 4\n",
		},
		{
			dialecttest.Preset{
				Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
				Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
			},
			// `entry 0` and not `entry 4`, which is **this shell's** zsh and
			// not zsh's: real zsh 5.9 prints `entry 4` here like every other
			// column. That half is Semantics.SignalHandlerSeesEarlierStatus
			// — what `$?` a handler is entered with — and it is measured to
			// be unchanged by this axis: the zsh binary built before and
			// after this change writes the same bytes for all three probes.
			// Written down rather than smoothed to `entry 4`, so the gap is
			// visible to whoever reaches that axis next.
			interp.TrapReturnTakesTheHandlersLastStatus, "entry 0 exit 123\n",
		},
		{
			dialecttest.Preset{
				Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
				Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
			},
			interp.TrapReturnIsZero, "entry 4 exit 0\n",
		},
		{
			dialecttest.Preset{
				Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
				Diagnostics: dash.Diagnostics, Apply: dash.Apply,
			},
			interp.TrapReturnTakesTheStatusBeforeIt, "entry 4 exit 4\n",
		},
		{
			dialecttest.Preset{
				Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
				Diagnostics: ash.Diagnostics, Apply: ash.Apply,
			},
			interp.TrapReturnTakesTheStatusBeforeIt, "entry 4 exit 4\n",
		},
	}
}

// The entry status is 4 for everybody by construction: the action interrupts a
// subshell that sleeps and then exits 4, and the signal is sent from a
// background job well before it ends. Without that the columns differ about
// *when* the action runs and the axis cannot be read at all — and without it
// bash and ksh93 would both answer 0 and the probe could not tell them apart,
// which is the whole reason this one waits rather than signaling itself.
//
// `/bin/sleep` by path because the harness runs with no PATH of its own.
const trapReturnProbe = "f() { return $1; }\n" +
	"h() { ( /bin/sleep 0.1; kill -USR1 $$ ) & ( /bin/sleep 0.5; exit 4 ); return 7; }\n" +
	"trap 'printf \"entry %s \" \"$?\"; f 123; return' USR1\n" +
	"h\n" +
	"printf 'exit %s\\n' \"$?\"\n" +
	"wait 2>/dev/null\n"

func TestEachDialectAnswersTheTrapReturnAxis(t *testing.T) {
	for _, p := range trapReturnPresets() {
		t.Run(p.Name, func(t *testing.T) {
			if got := p.Semantics().TrapReturnStatus; got != p.reading {
				t.Errorf("TrapReturnStatus = %v, want %v", got, p.reading)
			}
		})
	}
}

// The bytes, which is what the field is for: a preset holding the right value
// and writing the wrong number would pass the test above.
func TestTheTrapReturnStatusInEveryDialect(t *testing.T) {
	for _, p := range trapReturnPresets() {
		t.Run(p.Name, func(t *testing.T) {
			out, _, err := p.Combined(t, dialecttest.Base{}, trapReturnProbe)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if out != p.want {
				t.Errorf("wrote %q, want %q", out, p.want)
			}
		})
	}
}

// How far the reading reaches, which is measured and is not the same for all
// of them. `inner() { false; return; }` called *from* the action:
//
//	bash 5.3.20, dash 0.5.12, zsh 5.9   inner=1   the function's own last
//	ksh93u+ 2012-08-01                  inner=0   the same zero as the action
//
// So ksh93's answer covers the whole extent of the action and the other two
// readings stop at the action's own commands. Outside a trap entirely all four
// answer 1 — the second case here — which is what says this belongs to the
// trap and not to `return`.
func TestHowFarTheTrapReturnReadingReaches(t *testing.T) {
	const called = "inner() { /usr/bin/false; return; }\n" +
		"h() { ( /bin/sleep 0.1; kill -USR1 $$ ) & ( /bin/sleep 0.5; exit 4 ); return 7; }\n" +
		"trap 'inner; printf \"inner=%s \" \"$?\"' USR1\n" +
		"h\n" +
		"printf 'exit %s' \"$?\"\n" +
		"wait 2>/dev/null\n"
	const noTrap = "inner() { /usr/bin/false; return; }\ninner\nprintf 'inner=%s' \"$?\"\n"
	for _, p := range trapReturnPresets() {
		t.Run(p.Name, func(t *testing.T) {
			want := "inner=1 exit 7"
			if p.reading == interp.TrapReturnIsZero {
				want = "inner=0 exit 7"
			}
			out, _, err := p.Combined(t, dialecttest.Base{}, called)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if out != want {
				t.Errorf("called from the action: wrote %q, want %q", out, want)
			}
			// And the same function with no trap anywhere is the ordinary
			// reading in every column, which is the control: without it a
			// shell that always answered the function's last command would
			// pass the row above for four of the five.
			out, _, err = p.Combined(t, dialecttest.Base{}, noTrap)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if out != "inner=1" {
				t.Errorf("with no trap: wrote %q, want %q", out, "inner=1")
			}
		})
	}
}
