// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A bare `emulate` names the current mode, and the mode is what the last
// emulation set — measured, `zsh` before anything and the word after.
func TestBareEmulateNamesTheMode(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `emulate; emulate sh; emulate; emulate ksh; emulate`)
	if st != 0 || out != "zsh\nsh\nksh\n" {
		t.Errorf("out %q status %d, want the three modes in turn", out, st)
	}
}

// `emulate sh` moves the three measured axes: unquoted expansions split, a
// failed glob passes through, and arrays base at zero; `emulate zsh` puts all
// three back.
func TestEmulateShMovesTheMeasuredAxes(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`x="a b"; set -- $x; echo n=$#; emulate sh; set -- $x; echo n=$#; echo x*; `+
			`a=(p q); echo ${a[1]}; emulate zsh; set -- $x; echo n=$#`)
	if st != 0 || out != "n=1\nn=2\nx*\nq\nn=1\n" {
		t.Errorf("out %q status %d, want split, passthrough, zero base, and back", out, st)
	}
}

// A plain emulation resets options to its defaults — measured, no -R needed:
// `setopt err_exit; emulate zsh` turns errexit back off.
func TestEmulateResetsOptions(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt err_exit; emulate zsh; false; echo reached`)
	if st != 0 || !strings.Contains(out, "reached") {
		t.Errorf("out %q status %d, want errexit gone", out, st)
	}
}

// A word naming no emulation is passed over in silence, the mode unchanged —
// measured on `fish` and on `SH`, whose case does not match.
func TestEmulateIgnoresAnUnknownMode(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `emulate fish; echo st=$?; emulate SH; emulate`)
	if st != 0 || out != "st=0\nzsh\n" {
		t.Errorf("out %q status %d, want silence and the mode still zsh", out, st)
	}
}

// `-c` runs the string under the emulation and restores everything after,
// options included — measured, a `no_glob` set before it comes back.
func TestEmulateDashCRestores(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`setopt no_glob; emulate sh -c 'x="a b"; set -- $x; echo n=$#'; echo x*; `+
			`x="c d"; set -- $x; echo n=$#; emulate`)
	if st != 0 || out != "n=2\nx*\nn=1\nzsh\n" {
		t.Errorf("out %q status %d, want the emulation inside and everything back after", out, st)
	}
}

// The refusals, each measured: a bad option letter, a `-c` whose string never
// arrives, a second operand, flags with no mode — and `-L`, which is not
// zsh's refusal but this shell's own: the function-local form needs a restore
// on return that no seam provides, so it is refused rather than silently made
// global.
func TestEmulateRefusals(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{`emulate -x sh`, "bad option: -x", 1},
		{`emulate sh -c`, "string expected after -c", 1},
		{`emulate zsh sh`, "unknown argument sh", 1},
		{`emulate -R`, "not enough arguments", 1},
		{`emulate -L sh`, "-L is not implemented yet", 2},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != tc.status || !strings.Contains(out, tc.want) {
			t.Errorf("%s: out %q status %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}

// An emulation in a subshell stays there: the semantics swap is
// copy-on-write and the mode lives in the variable table a clone copies.
func TestEmulateStaysInItsSubshell(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `(emulate sh); emulate; x="a b"; set -- $x; echo n=$#`)
	if st != 0 || out != "zsh\nn=1\n" {
		t.Errorf("out %q status %d, want the parent untouched", out, st)
	}
}
