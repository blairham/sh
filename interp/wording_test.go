// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	. "github.com/blairham/sh/interp"
)

func TestWordingFallsBackToTheSubstrate(t *testing.T) {
	// A dialect states only where it differs, so an unset format must not
	// produce an empty message.
	if got := Wording("", "%s: not found", "x"); got != "x: not found" {
		t.Errorf("fallback: %q", got)
	}
	if got := Wording("%s: command not found", "%s: not found", "x"); got != "x: command not found" {
		t.Errorf("custom: %q", got)
	}
	// A format may ignore what it is given. dash names no count in its
	// `shift` message where ksh93 does, and passing the count to both used
	// to append "%!(EXTRA int=5)".
	if got := Wording("shift: can't shift that many", "unused %d", 5); got != "shift: can't shift that many" {
		t.Errorf("verbless format: %q", got)
	}
	// The shells order the arithmetic verbs differently, so they are
	// positional.
	if got := Wording(`arithmetic expression: %[2]s: "%[1]s"`, "%[2]s", "1/0", "division by zero"); got != `arithmetic expression: division by zero: "1/0"` {
		t.Errorf("positional: %q", got)
	}
	// Only bash names the token an arithmetic failure is blamed on, so every
	// caller passes three arguments and three of the four formats use two.
	// An indexed format ignores what it does not reach, which is what lets a
	// dialect stay silent about a verb rather than having to accept it.
	if got := Wording(`arithmetic expression: %[2]s: "%[1]s"`, "%[2]s", "1/0", "division by zero", "0"); got != `arithmetic expression: division by zero: "1/0"` {
		t.Errorf("unused third verb: %q", got)
	}
	if got := Wording(`%[1]s: %[2]s (error token is "%[3]s")`, "%[2]s", "1/0", "division by 0", "0"); got != `1/0: division by 0 (error token is "0")` {
		t.Errorf("third verb: %q", got)
	}
}

func TestEachDialectWordsItsOwnFailures(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		sem       Semantics
		diag      Diagnostics
		want      string
	}{
		{"dash readonly", `readonly r=1; r=2`, dash.Semantics(), dash.Diagnostics(), "r: is read only"},
		{"bash readonly", `readonly r=1; r=2`, bash.Semantics(), bash.Diagnostics(), "r: readonly variable"},
		{"zsh readonly", `readonly r=1; r=2`, zsh.Semantics(), zsh.Diagnostics(), "read-only variable: r"},
		{"bash not found", `nosuchcommand_xyz`, bash.Semantics(), bash.Diagnostics(), "command not found"},
		{"dash not found", `nosuchcommand_xyz`, dash.Semantics(), dash.Diagnostics(), "nosuchcommand_xyz: not found"},
		{"dash arithmetic", `echo $((1/0))`, dash.Semantics(), dash.Diagnostics(), `arithmetic expression: division by zero: "1/0"`},
		{"ksh93 arithmetic", `echo $((1/0))`, ksh.Semantics(), ksh.Diagnostics(), "1/0: divide by zero"},
		{"dash invalid number", `x=abc; echo $((x+1))`, dash.Semantics(), dash.Diagnostics(), "Illegal number: abc"},
		{"ksh93 shift", `shift 5`, ksh.Semantics(), ksh.Diagnostics(), "shift: 5: bad number"},
		{"dash shift", `shift 5`, dash.Semantics(), dash.Diagnostics(), "shift: can't shift that many"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, func(r *Runner) {
				sem, diag := tc.sem, tc.diag
				r.Semantics, r.Diagnostics = &sem, &diag
			})
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q", out, tc.want)
			}
		})
	}
}

// TestScriptDiagnosticsNameTheScript is behavior rather than wording: a shell
// running a file reports the file, and ksh93 also changes how it names the
// line — under `-c` it names one only after the first, and a script names
// line 1 like any other.
func TestScriptDiagnosticsNameTheScript(t *testing.T) {
	if got := ksh.Diagnostics().Report("s", 2, "m"); got != "s: line 2: m" {
		t.Errorf("ksh -c: %q, want %q", got, "s: line 2: m")
	}
	if got := ksh.Diagnostics().Report("s", 1, "m"); got != "s: m" {
		t.Errorf("ksh -c line 1: %q, want %q", got, "s: m")
	}
	if got := ksh.Diagnostics().ForScript().Report("s", 1, "m"); got != "s: line 1: m" {
		t.Errorf("ksh script line 1: %q, want %q — a script names its first line", got, "s: line 1: m")
	}
	if got := ksh.Diagnostics().ForScript().Report("s", 2, "m"); got != "s: line 2: m" {
		t.Errorf("ksh script: %q, want %q", got, "s: line 2: m")
	}
	// The other three do not change between the two.
	for _, d := range []Diagnostics{dash.Diagnostics(), bash.Diagnostics(), zsh.Diagnostics()} {
		if d.Report("s", 2, "m") != d.ForScript().Report("s", 2, "m") {
			t.Error("only ksh93 differs between -c and a script")
		}
	}
}
