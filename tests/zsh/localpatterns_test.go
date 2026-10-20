// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `emulate -L` turns on three local-scoping options and not two. The listing
// is taken *inside* the function, so the scope the letter opens is still
// open; a listing taken after the return reads the caller's table and would
// agree whatever the letter did.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f`, 2026-09-26:
// `f(){ emulate -L zsh; setopt }; f` prints `nohashdirs`, `localoptions`,
// `localpatterns`, `localtraps`, `norcs`. `-L zsh` rather than `-L sh` is the
// control: zsh is the mode that moves no defaults, so every name in that list
// is the letter's own doing (#4530).
func TestEmulateLocalTurnsOnThreeLocalScopingOptions(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f(){ emulate -L zsh; setopt }; f`)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	for _, name := range []string{"localoptions", "localpatterns", "localtraps"} {
		if !strings.Contains(out, name+"\n") {
			t.Errorf("setopt inside emulate -L zsh = %q, want %s among the on names", out, name)
		}
	}
	// The neighboring name that does *not* go on is what says the letter
	// sets a measured three rather than every name beginning `local`.
	if strings.Contains(out, "localloops") {
		t.Errorf("setopt inside emulate -L zsh = %q, want localloops left off", out)
	}
}

// The state is readable through the two surfaces a script uses, not only
// through the listing — and it is the letter that writes it, so a plain
// `emulate zsh` leaves the option where it found it.
func TestEmulateLocalIsWhatTurnsLocalPatternsOn(t *testing.T) {
	const src = `f(){ emulate -L zsh; [[ -o localpatterns ]] && print inL=on || print inL=off }
g(){ emulate zsh; [[ -o localpatterns ]] && print bare=on || print bare=off }
f
g
[[ -o localpatterns ]] && print after=on || print after=off`
	out, st := runZsh(t, t.TempDir(), src)
	if st != 0 || out != "inL=on\nbare=off\nafter=off\n" {
		t.Errorf("out %q status %d, want the letter on, a bare emulation off, and the caller untouched", out, st)
	}
}

// `$options` is the third surface, and the one a framework reads.
func TestLocalPatternsReachesTheOptionsParameter(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f(){ emulate -L zsh; print ${options[localpatterns]} }; f`)
	if st != 0 || out != "on\n" {
		t.Errorf("out %q status %d, want ${options[localpatterns]} on inside emulate -L", out, st)
	}
}
