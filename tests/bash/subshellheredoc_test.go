// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// A `( … )` is a process of its own here, so the here-document body fed to it
// is expanded there and what it writes does not come back — where the same
// body on a group, a function, a builtin or a loop leaves the write behind.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/bash` — `GNU bash, version
// 5.3.20(1)-release (aarch64-apple-darwin25.6.0)`, `not a Go executable` by
// `go version -m` — over a script file under `env -i PATH=/usr/bin:/bin` with
// a scratch HOME. `n=0` before, a body of `$(( n+=5 ))`, `echo "n=[$n]"`
// after:
//
//	`<<END` on …                          n afterwards
//	( cat )         a subshell            0
//	{ cat; }        a group               5
//	f               a function            5
//	read x          a builtin             5
//	while … done    a loop                5
//	cat             a command of its own  0
//
// The last row is the other axis and is unchanged by this one (#4700).
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
			out, _ := runBash(t, t.TempDir(), src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("%s: out = %q, want %q", tc.cmd, out, tc.want)
			}
			// The body reached the command, so the row above is about where
			// the write landed and not about a body nobody expanded.
			if !strings.Contains(out, "5\n") {
				t.Errorf("%s: out = %q, want the expanded body to have reached the command", tc.cmd, out)
			}
		})
	}
}

// The write is confined and not refused: the body still reaches the command,
// and a quoted delimiter is not expanded at all. Without the first of these a
// shell that simply declined to expand a subshell's body would pass the row
// above.
func TestASubshellsHeredocBodyStillReachesTheCommandHere(t *testing.T) {
	t.Parallel()
	out, _ := runBash(t, t.TempDir(), "n=0\n( /bin/cat ) <<END\n[$(( n+=5 ))]\nEND\necho \"n=[$n]\"\n")
	if !strings.Contains(out, "[5]") || !strings.Contains(out, "n=[0]") {
		t.Errorf("out = %q, want the body through at 5 and the write lost", out)
	}
	quoted, _ := runBash(t, t.TempDir(), "n=0\n( /bin/cat ) <<'END'\n$(( n+=5 ))\nEND\necho \"n=[$n]\"\n")
	if !strings.Contains(quoted, "$(( n+=5 ))") || !strings.Contains(quoted, "n=[0]") {
		t.Errorf("a quoted delimiter: out = %q, want the body literal", quoted)
	}
}

// And the failure side, which this column shows most sharply: a body that
// will not expand costs the **parentheses** here, so the rest of the same
// line runs and `||` catches it — where the same body on a group gives up the
// line and `||` never sees it. The marker is after a `;` on the
// redirection's own line, because one on the next line is printed by both
// readings.
func TestASubshellsFailedHeredocBodyCostsOnlyTheParenthesesHere(t *testing.T) {
	t.Parallel()
	t.Run("the rest of its own line runs", func(t *testing.T) {
		t.Parallel()
		out, _ := runBash(t, t.TempDir(), "( echo RAN ) <<END ; echo SAME\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("out = %q, want the subshell left unrun", out)
		}
		if !strings.Contains(out, "SAME") || !strings.Contains(out, "after st=0") {
			t.Errorf("out = %q, want the rest of the line run", out)
		}
	})
	t.Run("|| catches it", func(t *testing.T) {
		t.Parallel()
		out, _ := runBash(t, t.TempDir(), "( echo RAN ) <<END || echo CAUGHT\n$(( 1/0 ))\nEND\n")
		if !strings.Contains(out, "CAUGHT") {
			t.Errorf("out = %q, want the failure caught", out)
		}
	})
	t.Run("a group still gives up its line", func(t *testing.T) {
		t.Parallel()
		out, _ := runBash(t, t.TempDir(), "{ echo RAN; } <<END ; echo SAME\n$(( 1/0 ))\nEND\necho \"after st=$?\"\n")
		if strings.Contains(out, "SAME") {
			t.Errorf("out = %q, want the rest of the line given up", out)
		}
		if !strings.Contains(out, "after st=1") {
			t.Errorf("out = %q, want the next line at 1", out)
		}
	})
}

func TestBashSaysASubshellsHeredocBodyExpandsInTheSubshell(t *testing.T) {
	t.Parallel()
	if got, want := bash.Semantics().HeredocBodyOnASubshellExpandsInTheSubshell, interp.Yes; got != want {
		t.Errorf("HeredocBodyOnASubshellExpandsInTheSubshell = %v, want %v", got, want)
	}
}
