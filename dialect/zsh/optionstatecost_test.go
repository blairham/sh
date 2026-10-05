// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// optionCostRC is the option state the cost rows run in: the `setopt` lines
// of a real rc file — history, completion and globbing options, recorded and
// acted-on names both — so that an emulation has deviations to drop and a
// restore has something to put back, and three functions that each turn the
// table around in one of the ways plugins do.
const optionCostRC = `setopt histexpiredupsfirst histignorealldups histignorespace histreduceblanks
setopt histverify sharehistory autocd nocorrect alwaystoend hashlistall completeinword
setopt listambiguous autolist automenu nolisttypes listpacked extendedglob promptsubst
plain() { : }
emulating() { emulate -L zsh }
strict() { emulate -LR zsh }
`

// optionCostRunner is a zsh runner with optionCostRC run, and a parsed call
// to name.
func optionCostRunner(tb testing.TB, name string) (*interp.Runner, func()) {
	tb.Helper()
	r := optionStateRunner(tb)
	rc, err := syntax.Parse(optionCostRC, Dialect())
	if err != nil {
		tb.Fatal(err)
	}
	if _, err := r.Run(context.Background(), rc); err != nil {
		tb.Fatal(err)
	}
	call, err := syntax.Parse(name, Dialect())
	if err != nil {
		tb.Fatal(err)
	}
	return r, func() {
		if _, err := r.Run(context.Background(), call); err != nil {
			tb.Fatal(err)
		}
	}
}

// callAllocs is the allocations one call to name makes, less those of a call
// to a function that does nothing — so the row measures what the option
// table costs and not what a function call costs.
func callAllocs(t *testing.T, name string) float64 {
	t.Helper()
	_, base := optionCostRunner(t, "plain")
	_, call := optionCostRunner(t, name)
	base()
	call()
	return testing.AllocsPerRun(200, call) - testing.AllocsPerRun(200, base)
}

// `emulate -L zsh` opens nearly every function in a zsh plugin — 6,204 calls
// in one interactive start on the maintainer's real configuration
// (2026-10-05) — so what it costs is paid thousands of times before the first
// prompt is usable.
//
// It cost about 150 allocations a call over a function that does nothing:
// the recorded options were an association in the variable store, so the
// save on the way in copied it, the emulation sorted its names into a list
// and wrote a new table, and the restore on the way out built another; and
// every option the emulation set that reaches the grammar copied the whole
// dialect to find out whether it was already where it was being put. Now the
// recorded options are bits on the runner, an emulation applies a plan
// compiled once per mode, and a grammar setter copies only when it moves
// something (#6100).
//
// Allocations rather than time, so the row is deterministic. The bound has
// room above what a call costs now and is far below what it cost before; the
// strict form is the second row because it resets twice the names.
func TestEmulateLocalCostsFewAllocations(t *testing.T) {
	for _, name := range []string{"emulating", "strict"} {
		if got := callAllocs(t, name); got > 40 {
			t.Errorf("%s: an `emulate` call allocates %v times more than an empty function; want at most 40", name, got)
		}
	}
}

// A function's return under LOCAL_OPTIONS puts the whole table back, and
// with the option on globally every call does it. The save took a copy of
// the recorded options' association at every call and the restore built a
// fresh one; both are a copy of four words now.
func TestALocalOptionsReturnCostsFewAllocations(t *testing.T) {
	_, base := optionCostRunner(t, "plain")
	r, call := optionCostRunner(t, "plain")
	setLocalOptions(r, true)
	base()
	call()
	if got := testing.AllocsPerRun(200, call) - testing.AllocsPerRun(200, base); got > 4 {
		t.Errorf("a call that restores the option table allocates %v times more than one that does not; want at most 4", got)
	}
}

func BenchmarkEmulateLocal(b *testing.B) {
	_, call := optionCostRunner(b, "emulating")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		call()
	}
}

func BenchmarkLocalOptionsReturn(b *testing.B) {
	r, call := optionCostRunner(b, "plain")
	setLocalOptions(r, true)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		call()
	}
}
