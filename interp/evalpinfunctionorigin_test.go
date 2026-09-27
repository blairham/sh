// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A pinned location is measured against the origin that stood when it was
// taken, never against the origin of a body it reaches into.
//
// The switch that says evaluated text is not a place of its own — see
// Runner.EvalTextHasALocationOfItsOwn — pins the caller's location over
// everything the text sets going, a call included. The dialect that has that
// switch also counts a function's lines from the line the function was
// *written* on, and the two together used to subtract the callee's definition
// line from a line of the caller's file: two origins from two different texts,
// measured against each other.
//
// Measured 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go version
// -m` → *not a Go executable*, `-f` over a script file with `unsetopt
// evallineno`, `myfunc` written on line 2 and `eval "myfunc"` on line 7: the
// reference reads `F=7` where this read `F=5`, and the diagnostic from the
// same body was `myfunc:7:` against `myfunc:5:` (#4758).
//
// The direct call is the control in every row below: the function rule itself
// is right, and it is the *pin* that must not be measured against a
// definition line.
func pinnedFunctionSetup(r *Runner) {
	s := *r.Semantics
	s.LinenoCountsFromTheFunction = Yes
	r.Semantics = &s
	var d Diagnostics
	if r.Diagnostics != nil {
		d = *r.Diagnostics
	}
	d.LocationNamesTheFunction = true
	d.Location = LocationTightLine
	r.Diagnostics = &d
	r.SetEvalTextHasALocationOfItsOwn(false)
}

// The `eval` is at the top level, where the origin in force at the pin is
// nothing at all. `myfunc` is written on line 1, so subtracting its
// definition line reads 4 where the pin itself is 5.
const pinnedEvalAtTheTop = "myfunc() {\n" +
	"  echo F=$LINENO\n" +
	"}\n" +
	"echo pad\n" +
	`eval "myfunc"` + "\n" +
	"myfunc\n"

func TestAPinnedLineIsNotMeasuredAgainstTheCalleesDefinitionLine(t *testing.T) {
	out, _ := run(t, pinnedEvalAtTheTop, pinnedFunctionSetup)
	if want := "pad\nF=5\nF=1"; strings.TrimSpace(out) != want {
		t.Errorf("got %q, want %q — the pin, then the direct call's own offset", strings.TrimSpace(out), want)
	}
}

// And where the `eval` is inside a function of its own, the origin is *that*
// function's: the subtraction comes out at the line the pin was taken at,
// counted the way the frame that took it counts.
//
// `outer` is written on line 5 and its `eval` is on line 7, so the answer is
// 2. Subtracting the callee's definition line instead reads 6, and taking the
// pin whole reads 7 — three different numbers, which is what makes this row
// the one that says *which* origin.
const pinnedEvalInsideAFunction = "myfunc() {\n" +
	"  echo F=$LINENO\n" +
	"}\n" +
	"echo pad\n" +
	"outer() {\n" +
	"  echo pad\n" +
	`  eval "myfunc"` + "\n" +
	"}\n" +
	"outer\n"

func TestAPinnedLineKeepsTheOriginInForceWhereItWasTaken(t *testing.T) {
	out, _ := run(t, pinnedEvalInsideAFunction, pinnedFunctionSetup)
	if want := "pad\npad\nF=2"; strings.TrimSpace(out) != want {
		t.Errorf("got %q, want %q — the pin counted from the frame that took it", strings.TrimSpace(out), want)
	}
}

// The diagnostic is the same location read by the other reader, so it moves
// with it. Two readers of one location must not disagree about which origin
// it is measured against.
const pinnedEvalDiagnostic = "myfunc() {\n" +
	"  nosuchcmd-xyz\n" +
	"}\n" +
	"echo pad\n" +
	`eval "myfunc"` + "\n"

func TestAPinnedDiagnosticIsCountedFromTheSameOrigin(t *testing.T) {
	out, _ := run(t, pinnedEvalDiagnostic, pinnedFunctionSetup)
	if !strings.Contains(out, "myfunc:5:") {
		t.Errorf("got %q, want the location `myfunc:5:`", out)
	}
}

// The control the whole row rests on: with the text a place of its own — the
// switch's other state, and the default — nothing here moves. The text
// numbers from its own line one, so the function called from it counts from
// its own definition as it always did.
func TestTheOriginRuleIsUnmovedWhereTheTextIsAPlaceOfItsOwn(t *testing.T) {
	out, _ := run(t, pinnedEvalAtTheTop, func(r *Runner) {
		pinnedFunctionSetup(r)
		r.SetEvalTextHasALocationOfItsOwn(true)
	})
	if want := "pad\nF=1\nF=1"; strings.TrimSpace(out) != want {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
	}
}
