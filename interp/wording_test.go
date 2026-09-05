// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

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

// What each preset puts in its wording fields — and how the two Report routes
// differ under one of them — is asserted in the dialect packages, next to the
// presets making the claims. This file keeps only the substrate's own rules.

// TestScriptDiagnosticsCanNameTheirFirstLine is behavior rather than wording:
// ScriptLocation lets a dialect name line 1 in a file while leaving it
// unnamed under `-c`, which is what ForScript switches to.
func TestScriptDiagnosticsCanNameTheirFirstLine(t *testing.T) {
	d := Diagnostics{
		Location:       LocationLineWordAfterFirst,
		ScriptLocation: LocationLineWord,
	}
	if got := d.Report("s", 1, "m"); got != "s: m" {
		t.Errorf("-c line 1: %q, want %q", got, "s: m")
	}
	if got := d.ForScript().Report("s", 1, "m"); got != "s: line 1: m" {
		t.Errorf("script line 1: %q, want %q — a script names its first line", got, "s: line 1: m")
	}
	// With no ScriptLocation of its own, a dialect reports both routes the
	// same way.
	same := Diagnostics{Location: LocationLineWord}
	if same.Report("s", 2, "m") != same.ForScript().Report("s", 2, "m") {
		t.Error("an unset ScriptLocation should not change between -c and a script")
	}
}
