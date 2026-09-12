// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/internal/dialecttest"
)

// What this shell does to a prompt parameter before drawing it: expands it,
// and has no escape language of its own.
func TestPromptStyleAnswers(t *testing.T) {
	p := ash.PromptStyle()
	if p.Expand == nil {
		t.Error("Expand is nil: measured, an inherited PS1='<$LOGNAME>@ ' draws the name")
	}
	if p.Default != "$ " || p.DefaultContinued != "> " {
		t.Errorf("defaults = %q/%q, want `$ ` and `> `", p.Default, p.DefaultContinued)
	}
	// And they are assigned where a script can read them, which is one of the
	// three answers the panel gives.
	if !p.AssignsWithNobodyToPrompt {
		t.Error("AssignsWithNobodyToPrompt = false, want true")
	}
	// The empty escape table, recorded so that a row appearing in it later is
	// a change somebody made rather than one nobody noticed.
	if p.Escape != 0 {
		t.Errorf("Escape = %q, want none: measured, this shell draws `\\u` as `\\u`", p.Escape)
	}
	if len(p.Codes) != 0 {
		t.Errorf("Codes = %v, want none", p.Codes)
	}
}

// The table reaches a runner, which is the half that was missing when it was
// only a value: a runner draws `\u` as `\u` because this table says so, not
// because nobody told it anything (#1455).
func TestApplyInstallsThePromptStyle(t *testing.T) {
	preset.PromptTableInstalled(t, dialecttest.Base{Dir: t.TempDir()}, ash.PromptStyle())
}
