// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// At a prompt this shell writes no line, and a **builtin's** complaint drops
// the shell's name with it — two different answers in one session, which is
// what `PromptBuiltinLocation` exists for.
//
// Measured 2026-09-11 with `printf '<line>\n' | env -i HOME=<scratch>
// PS1='P1> ' zsh -i`, zsh 5.9.2:
//
//	nosuchcmd_zz         zsh: command not found: nosuchcmd_zz
//	echo ${undef?boom}   zsh: undef: boom
//	echo $((1/0))        zsh: division by zero
//	cd /nope             cd: no such file or directory: /nope
//	f() { cd /nope; }; f f:cd: no such file or directory: /nope
//
// The fourth row is the one that needs the second field: no `zsh:` in front of
// it and no line, where the first three keep the shell's name. The fifth says
// the function's own name still reaches it, so this is the *location* and not
// a rule about who speaks. We wrote `zsh:1: …` and `zsh:cd:1: …` for all five
// until #2024.
func TestARunTimeDiagnosticAtAPromptNamesNoLine(t *testing.T) {
	d := zsh.Diagnostics()
	if got := d.ForPrompt().Report("zsh", 1, "command not found: nosuchcmd_zz"); got !=
		"zsh: command not found: nosuchcmd_zz" {
		t.Errorf("a shell's own complaint at a prompt: got %q", got)
	}
	if got := d.Report("zsh", 1, "command not found: nosuchcmd_zz"); got !=
		"zsh:1: command not found: nosuchcmd_zz" {
		t.Errorf("off the prompt route: got %q, want the line kept", got)
	}
	if d.PromptBuiltinLocation != interp.LocationBuiltinNameOnly {
		t.Errorf("a builtin's complaint at a prompt is %v, want the builtin's name alone",
			d.PromptBuiltinLocation)
	}
}
