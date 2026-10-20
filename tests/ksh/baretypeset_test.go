// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// A bare `typeset` here is a listing, and a third one — see
// interp.BareLocalListsAttributedNames for the measurement and
// interp/attributephrase.go for the words.

// The preset holds the value. Asserted on its own because the listing below
// runs through a scratch environment, and a preset that had quietly gone back
// to unanswered would show there as a refusal that reads like a bad option.
func TestABareDeclarationListsTheAttributedNames(t *testing.T) {
	if got := ksh.Semantics().BareTypesetListing; got != interp.BareLocalListsAttributedNames {
		t.Errorf("BareTypesetListing = %v, want BareLocalListsAttributedNames", got)
	}
}

// The listing itself, and the control is in the same run: `plain` is assigned
// and never declared, so nothing is written about it. Measured 2026-09-13
// against ksh93u+ 2012-08-01 with `env -i`.
func TestABareDeclarationWritesTheAttributeWordsAndNoValue(t *testing.T) {
	dir := t.TempDir()
	src := strings.Join([]string{
		"plain=1",
		"export ex=2",
		// The letters spelled out rather than through `integer`, which is
		// this dialect's own alias for `typeset -li` and lives in the
		// prelude a Combined run does not install.
		"typeset -li n=3",
		"typeset -ui uns=7",
		"typeset -u up=q",
		"typeset -r ro=4",
		"typeset -A m",
		"typeset -a arr=(1 2)",
		"typeset -i16 h=255",
		"typeset -xr both=1",
		"typeset",
	}, "\n")
	out, st := runKsh(t, dir, src)
	if st != 0 {
		t.Fatalf("status = %d, want 0 — output %q", st, out)
	}
	for _, want := range []string{
		"export ex", "long integer n", "unsigned integer uns",
		"toupper up", "readonly ro",
		"associative array m", "indexed array arr", "integer base 16 h",
		"export readonly both",
	} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("the listing has no %q line:\n%s", want, out)
		}
	}
	// The name with no attribute, and the values of the ones that have some.
	// Both absences are the form rather than an accident, and a listing that
	// wrote every parameter or wrote assignments would fail here.
	for _, unwanted := range []string{"plain", "=2", "=3", "=4", "=7", "=255", "ex=", "n="} {
		if strings.Contains(out, unwanted) {
			t.Errorf("the listing wrote %q, which this form does not:\n%s", unwanted, out)
		}
	}
}
