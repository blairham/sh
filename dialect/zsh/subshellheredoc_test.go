// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A `( … )` is a process of its own here, so the here-document body fed to it
// is expanded there and what it writes does not come back — where the same
// body on a group, a function or a loop leaves the write behind.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/zsh -f` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, `not a Go executable` by `go version -m` —
// over a script file under `env -i PATH=/usr/bin:/bin` with a scratch HOME.
// `n=0` before, a body of `$(( n+=5 ))`, `echo "n=[$n]"` after: `( cat )`
// leaves 0, `{ cat; }` and a function leave 5, and `cat` on its own leaves 0,
// which is the other axis and is unchanged by this one (#4700).
func TestASubshellsHeredocBodyIsExpandedInTheSubshellHere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, cmd, want string }{
		{"a subshell loses it", "( /bin/cat )", "n=[0]"},
		{"a group keeps it", "{ /bin/cat; }", "n=[5]"},
		{"a function keeps it", "f", "n=[5]"},
		{"a command of its own loses it", "/bin/cat", "n=[0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := "f() { /bin/cat; }\nn=0\n" + tc.cmd + " <<END\n$(( n+=5 ))\nEND\necho \"n=[$n]\"\n"
			out, _ := runZsh(t, t.TempDir(), src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("%s: out = %q, want %q", tc.cmd, out, tc.want)
			}
			// The body reached the command, so the row is about where the
			// write landed and not about a body nobody expanded.
			if !strings.Contains(out, "5\n") {
				t.Errorf("%s: out = %q, want the expanded body to have reached the command", tc.cmd, out)
			}
		})
	}
}

// The failure side, and this column is the one where the two readings are
// furthest apart: a failed expansion is **fatal** here whatever it is written
// on, so a body the shell expanded itself ends the script — and a body the
// parentheses expanded costs the parentheses and nothing more. The rest of
// the same line runs and `||` catches it, where the same body on a group ends
// the script at 1.
func TestASubshellsFailedHeredocBodyCostsOnlyTheParenthesesHere(t *testing.T) {
	t.Parallel()
	t.Run("the rest of its own line runs", func(t *testing.T) {
		t.Parallel()
		out, st := runZsh(t, t.TempDir(), "( echo RAN ) <<END ; echo SAME\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("out = %q, want the subshell left unrun", out)
		}
		if !strings.Contains(out, "SAME") || !strings.Contains(out, "after st=0") || st != 0 {
			t.Errorf("out = %q (status %d), want the rest of the line run and the script alive", out, st)
		}
	})
	t.Run("|| catches it", func(t *testing.T) {
		t.Parallel()
		out, _ := runZsh(t, t.TempDir(), "( echo RAN ) <<END || echo CAUGHT\n$(( 1/0 ))\nEND\n")
		if !strings.Contains(out, "CAUGHT") {
			t.Errorf("out = %q, want the failure caught", out)
		}
	})
	t.Run("a group still ends the script", func(t *testing.T) {
		t.Parallel()
		out, st := runZsh(t, t.TempDir(), "{ echo RAN; } <<END ; echo SAME\n$(( 1/0 ))\nEND\necho after\n")
		if strings.Contains(out, "SAME") || strings.Contains(out, "after") {
			t.Errorf("out = %q, want the script ended", out)
		}
		if st != 1 {
			t.Errorf("status = %d, want this shell's fatal 1", st)
		}
	})
}

// A quoted delimiter expands nothing, so there is no write to place and the
// axis is never reached — the control that must not move.
func TestAQuotedDelimiterOnASubshellIsLiteralHere(t *testing.T) {
	t.Parallel()
	out, _ := runZsh(t, t.TempDir(), "n=0\n( /bin/cat ) <<'END'\n$(( n+=5 ))\nEND\necho \"n=[$n]\"\n")
	if !strings.Contains(out, "$(( n+=5 ))") || !strings.Contains(out, "n=[0]") {
		t.Errorf("out = %q, want the body literal and no write", out)
	}
}

func TestZshSaysASubshellsHeredocBodyExpandsInTheSubshell(t *testing.T) {
	t.Parallel()
	if got, want := zsh.Semantics().HeredocBodyOnASubshellExpandsInTheSubshell, interp.Yes; got != want {
		t.Errorf("HeredocBodyOnASubshellExpandsInTheSubshell = %v, want %v", got, want)
	}
}
