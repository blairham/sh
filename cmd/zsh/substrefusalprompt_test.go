// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// zsh asks again after a refused token at the top of a line, and still
// refuses a token in an open substitution's body at once. Measured 2026-10-07
// on zsh 5.9.2 through a pseudo-terminal: `echo $(` / `esac` and `echo $(` /
// `| x` write `parse error near …` and `parse error near `$('` at once, and
// `echo $(` / `if; then` — which its grammar takes — draws `> ` and goes on
// (#6319).
func TestAZshPromptRefusesAStopWordInASubstitutionBody(t *testing.T) {
	out, errs, _ := prompt(t, "echo $(\nesac\necho af''ter\n", "zsh", "-f", "-i")
	if !strings.Contains(errs, "parse error near `esac'\n") || !strings.Contains(out, "after\n") {
		t.Errorf("stdout %q stderr %q, want `esac' refused and the next line run", out, errs)
	}
	out, errs, _ = prompt(t, "echo $(\n| x\necho af''ter\n", "zsh", "-f", "-i")
	if !strings.Contains(errs, "parse error near `|'\n") || !strings.Contains(out, "after\n") {
		t.Errorf("stdout %q stderr %q, want `|' refused and the next line run", out, errs)
	}
	out, _, _ = prompt(t, "echo $(\nif; then\necho af''ter\n", "zsh", "-f", "-i")
	if strings.Contains(out, "after") {
		t.Errorf("stdout %q, want the substitution still being read", out)
	}
}
