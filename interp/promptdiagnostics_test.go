// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"reflect"
	"testing"
)

// TestThePromptsDiagnosticsAreMadeOnce pins that a line typed at a prompt
// reads the prompt's version of the vector without rebuilding it per question
// (#5873), that the version it reads is the one ForPrompt makes, and that a
// replaced vector is read afresh.
func TestThePromptsDiagnosticsAreMadeOnce(t *testing.T) {
	first := CoreDiagnostics()
	first.PromptLocation = LocationNone
	r := newTestRunner(t, &Runner{Diagnostics: &first, AtPrompt: true})
	if !r.promptLocated() {
		t.Fatal("the runner is not located at a prompt, so this measures nothing")
	}
	got := r.diag()
	if want := first.ForPrompt(); !reflect.DeepEqual(*got, want) {
		t.Error("the prompt's diagnostics are not ForPrompt's")
	}
	if again := r.diag(); again != got {
		t.Error("a second question rebuilt the prompt's diagnostics")
	}
	if n := testing.AllocsPerRun(50, func() { r.diag() }); n > 0 {
		t.Errorf("asking at a prompt allocated %v times, want none", n)
	}

	second := CoreDiagnostics()
	second.SyntaxUnexpected = "a different wording %s"
	second.PromptSyntaxUnexpected = ""
	r.Diagnostics = &second
	if got := r.diag(); got.SyntaxUnexpected != "a different wording %s" {
		t.Errorf("a replaced vector was not read: %q", got.SyntaxUnexpected)
	}
}
