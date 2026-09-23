// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `set -r` is taken here and restricts nothing, which is **not** the answer and
// is pinned as the state it is.
//
// This shell has a restricted mode of its own — measured 2026-09-22 from a
// script file, `set -r; cd /` is `<script>:cd:3: restricted` in zsh 5.9.2 — and
// that mode is not built here: the letter reaches a `setopt` name this dialect
// records, and a recorded name is remembered and acted on by nothing. So the
// letter is silent at 0 and `cd /` moves the shell.
//
// Named by the `unpinned zsh` verdicts on
// Semantics.RestrictedModeIsLeftByTheLetter and its two siblings, and the
// verdicts say what this test says: there is no mode here for the axes to move.
// #4205 built ksh93's mode and this one is filed rather than claimed — a test
// that asserted a refusal would be asserting something this shell does not do,
// and a test that asserted the mode would not pass.
func TestSetTakesTheRestrictedLetterAndDoesNothing(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "set -r\necho \"st=$?\"\ncd / && echo moved\necho tail\n")
	if st != 0 {
		t.Errorf("status %d, want 0 — the letter is taken here", st)
	}
	for _, want := range []string{"st=0", "moved", "tail"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q, want %q in it", out, want)
		}
	}
	if strings.Contains(out, "restricted") {
		t.Errorf("output %q: this shell's restricted mode is not built here, "+
			"so nothing should say the word", out)
	}
}
