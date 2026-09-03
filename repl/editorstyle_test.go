// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// What the dialect says reaches the editor.
//
// The editor is only built where there is a terminal, so nothing else here
// can look at this — and a mutation that dropped the dialect's answer on the
// way survived every other test, because an answer thrown away looks exactly
// like a dialect that did not answer.
func TestTheDialectsMarkReachesTheEditor(t *testing.T) {
	for _, want := range []string{"^C", "", "<interrupted>"} {
		s := Shell{Editor: EditorStyle{Interrupt: want}}
		if got := s.newEditor().interrupt; got != want {
			t.Errorf("editor marks with %q, want %q", got, want)
		}
	}
}
