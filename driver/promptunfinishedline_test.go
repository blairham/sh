// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/driver"
)

// A construct the input ends inside names the line it **opened** on, counted
// over the whole session.
//
// The number lives inside the wording rather than in the prefix, and that is
// what the base was getting wrong: it was told to start every construct's text
// at line 1 unless the dialect names a line in its *prefix*, which only dash
// does — so bash reported `on line 1` for a brace opened on line 4. Measured
// 2026-09-24 from a pipe under `-i`, three assignments and then `{ echo a`,
// with a scratch HOME:
//
//	bash 5.3.20   syntax error: unexpected end of file from `{' command on line 4
//	before        the same wording, `on line 1`
//	dash 0.5.12   `5: Syntax error: …` — its prefix, the line the failure was
//	              noticed on, which this shell already matched
//	zsh, ksh93    no line in either wording
//
// Two wordings over one base: bash names where the construct opened and dash's
// prefix names where the failure was noticed. Which of them a dialect writes is
// Diagnostics.PromptLocation's; neither is a reason to hand the parser a
// position that is not where the text is (#4363).
func TestAnUnfinishedConstructAtAPromptNamesWhereItOpened(t *testing.T) {
	bashShell := func() driver.Shell {
		sh := shell()
		sh.Dialect, sh.Semantics, sh.Diagnostics = bash.Dialect(), bash.Semantics(), bash.Diagnostics()
		return sh
	}
	_, errs, _ := runPipedShell(t, bashShell(), "x=1\nx=2\nx=3\n{ echo a\n", "testsh", "-i")
	if want := "on line 4"; !strings.Contains(errs, want) {
		t.Errorf("wrote %q, want it to name %q — where the brace opened", errs, want)
	}
	// The control, so a shell that named *every* line 4 would not pass: a
	// brace opened on the first line is named there.
	_, errs, _ = runPipedShell(t, bashShell(), "{ echo a\n", "testsh", "-i")
	if want := "on line 1"; !strings.Contains(errs, want) {
		t.Errorf("wrote %q, want it to name %q", errs, want)
	}
}
