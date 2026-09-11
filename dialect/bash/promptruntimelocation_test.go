// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// A **run-time** diagnostic at a prompt names no line here either, which is
// the other half of the seam the parse failure next door records.
//
// Measured 2026-09-11 with `printf '<line>\n' | env -i HOME=<scratch>
// PS1='P1> ' bash -i`, bash 5.3.15, the shell's path normalized:
//
//	nosuchcmd_zz              bash: nosuchcmd_zz: command not found
//	cd /nope                  bash: cd: /nope: No such file or directory
//	echo ${undef?boom}        bash: undef: boom
//	echo $((1/0))             bash: 1/0: division by 0 (error token is "0")
//	echo hi > /nope/x         bash: /nope/x: No such file or directory
//	eval "cd /nope"           bash: cd: /nope: No such file or directory
//
// — where every one of those names `line 1` in a script, and where this shell
// wrote `bash: line 1: …` for all of them until #2024. A builtin's complaint
// is the row that needs a second field rather than the same one: the location
// a builtin is reported at is `BuiltinLocation`, so a prompt answer for it has
// to be said separately.
func TestARunTimeDiagnosticAtAPromptNamesNoLine(t *testing.T) {
	d := bash.Diagnostics()
	if got := d.ForPrompt().Report("bash", 1, "nosuchcmd_zz: command not found"); got !=
		"bash: nosuchcmd_zz: command not found" {
		t.Errorf("a shell's own complaint at a prompt: got %q", got)
	}
	// The same failure in a script keeps its line, which is what says the
	// answer belongs to the route.
	if got := d.ForScript().Report("s.sh", 3, "nosuchcmd_zz: command not found"); got !=
		"s.sh: line 3: nosuchcmd_zz: command not found" {
		t.Errorf("in a script: got %q", got)
	}
	if d.PromptBuiltinLocation != interp.LocationNameOnly {
		t.Errorf("a builtin's complaint at a prompt is %v, want the name alone",
			d.PromptBuiltinLocation)
	}
}
