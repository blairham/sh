// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// ksh93 takes `set -H` in a script and expands nothing, which is the other
// half of the third history axis.
//
// Measured 2026-09-16 on ksh93u+ 2012-08-01: `set -H` is accepted — the letter
// is in its own usage line — and `echo !!` after `echo one two three` still
// prints the two characters. It refuses `set -o history` (`bad option(s)`),
// and it does not expand at a prompt either, which is the separate axis
// HistoryExpansionAtAPrompt already records.
func TestKshDoesNotExpandHistoryInAScript(t *testing.T) {
	var out, errs bytes.Buffer
	if code := driver.MainArgs(kshShell(&out, &errs), []string{
		"ksh", "-c", "set -H\necho one two three\necho !!\n",
	}); code != 0 {
		t.Fatalf("status %d (stderr %q)", code, errs.String())
	}
	if out.String() != "one two three\n!!\n" {
		t.Errorf("ran %q, want the two characters left alone", out.String())
	}
	if errs.String() != "" {
		t.Errorf("said %q, want nothing said", errs.String())
	}
}
