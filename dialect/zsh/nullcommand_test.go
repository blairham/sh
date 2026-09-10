// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// The two parameters are this dialect's, and their defaults are a script's to
// read: `print -r -- $NULLCMD` answers `cat` in real zsh 5.9.2, measured
// 2026-09-10.
func TestTheNullCommandParametersHaveTheirDefaults(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(), `print -r -- "[$NULLCMD][$READNULLCMD]"`)
	if st != 0 {
		t.Fatalf("status %d", st)
	}
	if want := "[cat][more]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

// And the vector names them rather than holding their values, which is what
// lets a script reassign them between two commands.
func TestTheNullCommandAxesNameTheParameters(t *testing.T) {
	s := zsh.Semantics()
	if got, want := s.NullCommandVariable, "NULLCMD"; got != want {
		t.Errorf("NullCommandVariable = %q, want %q", got, want)
	}
	if got, want := s.ReadNullCommandVariable, "READNULLCMD"; got != want {
		t.Errorf("ReadNullCommandVariable = %q, want %q", got, want)
	}
	if got, want := zsh.Diagnostics().RedirectionWithNoCommand,
		"redirection with no command"; got != want {
		t.Errorf("RedirectionWithNoCommand = %q, want %q", got, want)
	}
}

// A command that is only redirections, through the whole dialect. The markers
// are what separate the two parameters — left at `cat` and `more` both routes
// print the file and the probe proves nothing.
func TestARedirectionWithNoCommandRunsTheHook(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the reading parameter", `R(){ print -r -- R; }; READNULLCMD=R; <f`, "R\n"},
		{"the writing parameter", `N(){ print -r -- N; }; NULLCMD=N; READNULLCMD=R; <f 2>/dev/null`, "N\n"},
		{"an emptied reader falls back", `N(){ print -r -- N; }; NULLCMD=N; READNULLCMD=; <f`, "N\n"},
		{"a name that is not there", `READNULLCMD=nosuchcmd; <f`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			out, _ := runZshPrelude(t, dir, "printf 'hello\\n' > f\n"+tc.src+"\n")
			if tc.want == "" {
				return
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// `setopt cshnullcmd` and `setopt shnullcmd` are the two names that take the
// command off the parameters, and they are implemented rather than recorded.
//
// Both, together, because they are not independent: `cshnullcmd` wins while
// it is on in either order, and turning it off hands the shell back to
// `shnullcmd` if that one is still on. Measured on zsh 5.9.2.
func TestTheNullCommandOptions(t *testing.T) {
	for _, tc := range []struct {
		name, src, wantOut string
		wantStatus         int
	}{
		{"csh refuses", `setopt cshnullcmd; <f; print -r -- after`, "", 1},
		{"sh runs nothing", `setopt shnullcmd; R(){ print -r -- R; }; READNULLCMD=R; <f; print -r -- "st=$?"`, "st=0\n", 0},
		{"csh wins over sh", `setopt shnullcmd cshnullcmd; <f`, "", 1},
		{"csh wins in either order", `setopt cshnullcmd shnullcmd; <f`, "", 1},
		{"sh takes over again", `setopt shnullcmd cshnullcmd; unsetopt cshnullcmd; <f; print -r -- "st=$?"`, "st=0\n", 0},
		{"neither leaves the hook", `setopt shnullcmd; unsetopt shnullcmd; R(){ print -r -- R; }; READNULLCMD=R; <f`, "R\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			out, st := runZshPrelude(t, dir, "printf 'hello\\n' > f\n"+tc.src+"\n")
			if st != tc.wantStatus {
				t.Errorf("status %d, want %d (out %q)", st, tc.wantStatus, out)
			}
			if tc.wantStatus == 0 && out != tc.wantOut {
				t.Errorf("out = %q, want %q", out, tc.wantOut)
			}
			if tc.wantStatus != 0 && !strings.Contains(out, "redirection with no command") {
				t.Errorf("out = %q, want the refusal", out)
			}
		})
	}
}

// `multios` is the axis under this shell's own name, in both directions, and
// it moves rather than being remembered.
func TestMultiosIsTheAxisUnderItsOwnName(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"reading, on", `all <f <g`, "<a><b>\n"},
		{"reading, off", `unsetopt multios; all <f <g`, "<b>\n"},
		{"writing, on", `echo x >p >q; print -r -- "[$(<p)][$(<q)]"`, "[x][x]\n"},
		{"writing, off", `unsetopt multios; echo x >p >q; print -r -- "[$(<p)][$(<q)]"`, "[][x]\n"},
		// A subshell's change stays in it, which is what reading the axis
		// rather than a stored bit buys.
		{"a subshell keeps it", `(unsetopt multios; all <f <g); all <f <g`, "<b>\n<a><b>\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			// `all` reads its whole standard input, which is what a
			// concatenation shows and what an external `cat` would show if
			// there were a PATH here to find one on.
			const setup = "printf 'a\\n' > f\nprintf 'b\\n' > g\n" +
				"all(){ local l; while read -r l; do print -rn -- \"<$l>\"; done; print; }\n"
			out, st := runZshPrelude(t, dir, setup+tc.src+"\n")
			if st != 0 {
				t.Fatalf("status %d (out %q)", st, out)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
	if got, want := zsh.Semantics().RedirectsUseEveryTarget, interp.Yes; got != want {
		t.Errorf("RedirectsUseEveryTarget = %v, want %v", got, want)
	}
}
