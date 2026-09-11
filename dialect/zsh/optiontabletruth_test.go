// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// Eleven names this table wired immovable that real zsh moves in both
// directions, and the measurement that says so (#1739).
//
// Measured one name at a time on zsh 5.9.2, 2026-09-10, with no terminal:
//
//	zsh -f -c 'setopt NAME; print -n "on:$? "; unsetopt NAME; print -n "off:$?"'
//
// answers `on:0 off:0` for every one of them. The four that really are
// immovable there — `interactive`, `shinstdin`, `singlecommand`, `zle` — and
// `monitor`, which moves where the shell has a terminal, are unchanged.
//
// The names split three ways and each was decided on its own rather than
// swept. `emacs` and `vi` are the substrate's own editing mode, which really
// moves, so they became `set -o` backed. The other nine are states this shell
// holds and does not leave: they became **recorded** — recognized, remembered,
// reported and acted on by nothing — which is the kind the table's other 141
// names already use, and the kind the three-way split had no room for.

// zshOptionMove runs one name in both directions and answers what it said.
func zshOptionMove(t *testing.T, name string) (out, errs string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	src := "setopt " + name + `; printf 'on:%s ' "$?"; unsetopt ` + name + `; printf 'off:%s' "$?"`
	code = driver.MainArgs(zshWriting(&o, &e), []string{"zsh", "-c", src})
	return o.String(), e.String(), code
}

// TestTheElevenNamesRealZshMovesNowMoveHere is the differential, written as
// the measurement was taken.
func TestTheElevenNamesRealZshMovesNowMoveHere(t *testing.T) {
	for _, name := range []string{
		"banghist", "chaselinks", "emacs", "functionargzero", "hashdirs",
		"ignorebraces", "ignoreeof", "interactivecomments", "notify",
		"privileged", "shglob", "vi",
	} {
		t.Run(name, func(t *testing.T) {
			out, errs, code := zshOptionMove(t, name)
			if out != "on:0 off:0" || errs != "" || code != 0 {
				t.Errorf("%s = %q / %q status %d, want `on:0 off:0` and nothing said",
					name, out, errs, code)
			}
		})
	}
}

// TestTheImmovableNamesAreStillImmovable: the other half, and the half a
// sweep would have lost. These are the names zsh itself refuses a running
// script, measured the same way and in the same run.
func TestTheImmovableNamesAreStillImmovable(t *testing.T) {
	for _, name := range []string{"interactive", "shinstdin", "singlecommand", "zle"} {
		t.Run(name, func(t *testing.T) {
			var out, errs bytes.Buffer
			code := driver.MainArgs(zshWriting(&out, &errs),
				[]string{"zsh", "-c", "setopt " + name + "\necho NO"})
			want := "zsh:setopt:1: can't change option: " + name + "\n"
			if errs.String() != want {
				t.Errorf("said %q, want %q", errs.String(), want)
			}
			// `setopt` is not a special builtin, so the script carries on
			// past its own refusal — measured, the complaint is followed by
			// the next line and the shell leaves at 0.
			if out.String() != "NO\n" || code != 0 {
				t.Errorf("ran %q status %d, want the script to carry on at 0", out.String(), code)
			}
		})
	}
}

// TestTheEditingModeNamesReachTheSubstrateSwitch: `emacs` and `vi` are not
// recorded, and this is the difference. They move the one editing mode the
// core holds — the same state `set -o vi` writes — so selecting one
// deselects the other, which is what a remembered pair could not do.
func TestTheEditingModeNamesReachTheSubstrateSwitch(t *testing.T) {
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{
		"zsh", "-c",
		`setopt vi; [[ -o vi ]]; echo "vi=$?"; [[ -o emacs ]]; echo "emacs=$?"`,
	})
	if want := "vi=0\nemacs=1\n"; out.String() != want || code != 0 {
		t.Errorf("out %q status %d, want %q — one mode, two names", out.String(), code, want)
	}
}

// TestARecordedNameRemembersWhatItWasTold: the whole of what the recorded
// kind promises. The request is reported back faithfully by every door into
// the namespace, and the shell goes on doing what it did.
func TestARecordedNameRemembersWhatItWasTold(t *testing.T) {
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{
		"zsh", "-c",
		`setopt ignorebraces; [[ -o ignorebraces ]]; echo "cond=$?"; setopt | grep ignorebraces; echo {a,b}`,
	})
	if want := "cond=0\nignorebraces\na b\n"; out.String() != want || code != 0 {
		t.Errorf("out %q status %d, want %q", out.String(), code, want)
	}
	// And a subshell's change stays in the subshell, because the store is in
	// the variable table the clone deep-copies.
	out.Reset()
	code = driver.MainArgs(zshWriting(&out, &errs), []string{
		"zsh", "-c",
		`(setopt ignorebraces); [[ -o ignorebraces ]]; echo "cond=$?"`,
	})
	if want := "cond=1\n"; out.String() != want || code != 0 {
		t.Errorf("out %q status %d, want %q", out.String(), code, want)
	}
}

// `login` is one of zsh's options and it reads on for exactly the invocations
// that made the shell a login shell, which is where zsh keeps the fact —
// Semantics.LoginShowsLInDollarDash is `Yes` here precisely because `$-` does
// not carry it. Ours omitted the name from the listing entirely (#1727).
//
// Measured on zsh 5.9.2, 2026-09-10: `zsh -f -l -c setopt` writes `login` and
// `zsh -f -c setopt` does not, `[[ -o login ]]` follows, and the option is
// *movable* on top of that — `setopt login` in a shell that is not one is 0
// and reports `login` afterwards. That last is not the obvious answer and is
// the opposite of bash's for its own spelling, which is why it was measured
// rather than assumed.
func TestTheLoginOptionReadsTheInvocation(t *testing.T) {
	for _, tc := range []struct{ name, argv, want string }{
		{"a login shell reports it", "-l", "login\n"},
		{"an ordinary one does not", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			argv := []string{"zsh"}
			if tc.argv != "" {
				argv = append(argv, tc.argv)
			}
			argv = append(argv, "-c", "setopt | grep '^login$'; [[ -o login ]]; echo \"cond=$?\"")
			var out, errs bytes.Buffer
			driver.MainArgs(zshWriting(&out, &errs), argv)
			cond := "cond=1\n"
			if tc.want != "" {
				cond = "cond=0\n"
			}
			if want := tc.want + cond; out.String() != want {
				t.Errorf("out %q, want %q (stderr %q)", out.String(), want, errs.String())
			}
		})
	}
}

// TestTheLoginOptionStillMoves: measured, and the half that makes it an
// option rather than an indicator.
func TestTheLoginOptionStillMoves(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{
		"zsh", "-c",
		`setopt login; echo "s=$?"; [[ -o login ]]; echo "cond=$?"; setopt | grep -c '^login$'`,
	})
	if want := "s=0\ncond=0\n1\n"; out.String() != want || code != 0 {
		t.Errorf("out %q status %d, want %q", out.String(), code, want)
	}
	out.Reset()
	code = driver.MainArgs(zshWriting(&out, &errs), []string{
		"zsh", "-l", "-c",
		`unsetopt login; echo "s=$?"; [[ -o login ]]; echo "cond=$?"`,
	})
	if want := "s=0\ncond=1\n"; !strings.HasSuffix(out.String(), want) || code != 0 {
		t.Errorf("out %q status %d, want it to end %q", out.String(), code, want)
	}
}
