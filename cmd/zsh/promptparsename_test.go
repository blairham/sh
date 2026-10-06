// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// A line typed at the prompt that will not parse is named the way the rest of
// the session's diagnostics are, which in zsh is `zsh` under any name: the
// front end used to be handed the name the shell was started as (#6244).
//
// Measured 2026-10-06 on zsh 5.9.2 on a pipe, `env -i` with a scratch HOME,
// started as `./links/zfoo -f -i` and as `/opt/homebrew/bin/zsh -i`: both
// write `zsh: parse error near `fi'`.
func TestAZshPromptNamesALineThatWillNotParseAsZsh(t *testing.T) {
	home := scratchHome(t)
	t.Chdir(home)
	_, errs, _ := prompt(t, "fi\n", "./weird/myzsh", "-f", "-i")
	if !strings.Contains(errs, "zsh: parse error near `fi'\n") {
		t.Errorf("stderr %q, want `zsh: parse error near `fi'`", errs)
	}
	if strings.Contains(errs, "myzsh:") {
		t.Errorf("stderr %q names the shell by the name it was started as", errs)
	}
}
