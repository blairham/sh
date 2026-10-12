// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// Reading `${functions[f]}` prints f's body once per definition, not once per
// read. F-Sy-H's lifecycle EXIT trap reads it for each of 44 autoload names
// on every highlight call, and an autoload stub was printed twice a read: once
// to ask whether it was still a stub and once for its value (#6423). See
// interp/bodytext.go.
//
// Allocations rather than time, so the row is deterministic.
func TestReadingAFunctionsTextPrintsItOnce(t *testing.T) {
	const defs = "autoload -Uz fsa\n" +
		"g() { local -a l; l=(1 2 3); if (( $#l )); then print -r -- $l; fi; }\n"
	call := localCostRunner(t, defs, "x=${functions[fsa]}; y=${functions[g]}")
	call()
	// 22 on the change that set this, and 52 before it.
	if got := testing.AllocsPerRun(200, call); got > 30 {
		t.Errorf("two reads allocate %v times; the bodies are being printed again", got)
	}
}
