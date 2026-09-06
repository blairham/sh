// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os/user"
	"strings"
	"testing"
)

// This dialect tells the runner who it is running as, so `%n` answers.
//
// Compared against the system's own answer rather than against a name, which
// is what makes it a test about the escape and not about the machine — the
// corpus row for the same fact is written the same way, against `id -un`.
func TestThePromptUserEscapeNamesThisProcessUser(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skipf("no user for this process: %v", err)
	}
	out, st := runZsh(t, t.TempDir(), `printf "[%s]" "${(%):-%n}"`)
	if want := "[" + u.Username + "]"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// And it is not read out of a variable, which is measured rather than assumed:
// `%n` ignores `USER`, `LOGNAME` and `USERNAME` in zsh 5.9.2, whether they are
// assigned inside the shell or injected before it starts, and bash's `\u`
// ignores them too. A shell that read one would name the wrong person under
// `env USER=someone-else`.
//
// The variable is read back in the same word, so the row also says the
// assignment really happened and the escape simply did not consult it.
func TestThePromptUserEscapeIgnoresTheEnvironmentNames(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skipf("no user for this process: %v", err)
	}
	if u.Username == "impostor" {
		t.Skip("the probe name is this machine's real one")
	}
	const src = `USER=impostor; LOGNAME=impostor; printf "[%s][%s]" "${(%):-%n}" "$USER"`
	out, _ := runZsh(t, t.TempDir(), src)
	if want := "[" + u.Username + "][impostor]"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if strings.Contains(out, "[impostor][") {
		t.Error("the escape followed the variable, which no shell in the panel does")
	}
}
