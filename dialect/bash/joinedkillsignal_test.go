// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// TestThisPresetReadsASignalJoinedToKillsOption pins the answer this shell
// gives, which the interp tests prove the *effect* of.
//
// It is bash 5's answer and not bash's: bash 3.2.57 refuses both spellings,
// which is exactly why the preset has to say so rather than the engine.
// Measured 2026-09-12 against a background `sleep`: `kill -n9 $!` and
// `kill -sKILL $!` kill it at status 0 in bash 5.3.15 and are
// `kill: n9: invalid signal specification` at status 1 in bash 3.2.57.
func TestThisPresetReadsASignalJoinedToKillsOption(t *testing.T) {
	if got := bash.Semantics().KillReadsASignalJoinedToItsOption; got != interp.Yes {
		t.Errorf("KillReadsASignalJoinedToItsOption is %v, want Yes", got)
	}
	// The existence probe, so the test sends no signal to anything: what is
	// being asked is how the word was read, and `-n0` answers that.
	if out, st := answersRun(t, `kill -n0 $$; echo "st=$?"`); strings.TrimSpace(out) != "st=0" {
		t.Errorf("kill -n0 = %q status %d, want st=0", out, st)
	}
	if out, _ := answersRun(t, `kill -sCONT $$ 2>/dev/null; echo "st=$?"`); strings.TrimSpace(out) != "st=0" {
		t.Errorf("kill -sCONT = %q, want st=0", out)
	}
	// And the guard: a name onto `-n` is nobody's spelling, so it stays the
	// bare form and is refused under the word as written.
	out, _ := answersRun(t, `kill -nCONT $$; echo "st=$?"`)
	if !strings.Contains(out, "nCONT") || !strings.Contains(out, "st=1") {
		t.Errorf("kill -nCONT = %q, want a refusal naming nCONT at st=1", out)
	}
}

// TestThisPresetGivesUpOnAStoppedJob pins the other half of #2227: a `wait`
// for a job this shell has been told stopped does not go on waiting for a
// process that cannot finish on its own.
//
// bash 5's answer again, and again not bash's — bash 3.2.57 and ksh93u+ wait
// it out — so the preset is where it belongs.
func TestThisPresetGivesUpOnAStoppedJob(t *testing.T) {
	if got := bash.Semantics().WaitGivesUpOnAStoppedJob; got != interp.Yes {
		t.Errorf("WaitGivesUpOnAStoppedJob is %v, want Yes", got)
	}
}
