// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// userStyle is a prompt language with the three identity codes in it, spelled
// the way bash spells them.
func userStyle() PromptStyle {
	return PromptStyle{
		Escape: '\\',
		Codes: map[rune]PromptField{
			'u': FieldUser, 'h': FieldHost, 'H': FieldHostFull,
		},
	}
}

// The drawn user is what the shell was *told*, and not what a variable says.
//
// The row that holds the two apart, and the one #1446 needed. Every other test
// here supplies `USER` in the map newTestRunner turns into SetPromptUser, so
// the variable and the answer agree in all of them and a drawer reading either
// one passes. Here they disagree on purpose.
//
// Which of them is right is measured rather than chosen: bash 5.3.15, bash
// 3.2.57 and zsh 5.9.2 all draw the password database's answer for `\u` and
// `%n` with `USER` and `LOGNAME` injected before the shell starts and assigned
// inside it, and all three draw the system's own host name for `\h` and `\H`
// with `HOSTNAME` set the same two ways. Reading the variable would name the
// wrong person under `env USER=someone-else` — a prompt saying, at the exact
// moment somebody stated which account they meant, that they are somebody
// else.
func TestTheDrawnUserIsWhatTheShellWasTold(t *testing.T) {
	r := newTestRunner(map[string]string{
		"USER":     "impostor",
		"LOGNAME":  "impostor",
		"HOSTNAME": "impostor.example.com",
	})
	r.SetPromptUser("real-person")
	r.SetPromptHost("real-machine.example.com")
	s := Shell{Runner: r, Style: userStyle()}

	if got := s.render(`<\u>`); got != "<real-person>" {
		t.Errorf(`\u drew %q, want "<real-person>"`, got)
	}
	if got := s.render(`<\h>`); got != "<real-machine>" {
		t.Errorf(`\h drew %q, want "<real-machine>"`, got)
	}
	if got := s.render(`<\H>`); got != "<real-machine.example.com>" {
		t.Errorf(`\H drew %q, want "<real-machine.example.com>"`, got)
	}
	if strings.Contains(s.render(`<\u><\h><\H>`), "impostor") {
		t.Error("a code followed the variable, which no shell in the panel does")
	}
}

// A shell nobody told draws nothing rather than reaching for the environment.
//
// The half that made the fault silent: the old resolver's last line was the
// empty string, so a session with no `USER` — `env -i`, `sudo -i`, a
// container, a cron job — drew a prompt with a hole in it and said nothing
// about why. Empty is still what a drawer answers, because a prompt has to
// draw something and there is nothing to draw; what has changed is that the
// four shells that ship from this tree are all told, so nothing reaches this.
//
// It is asserted rather than left implicit so that a future resolver added
// here has to argue with this test instead of quietly reinstating the variable.
func TestAShellNobodyToldDrawsNoUserAtAll(t *testing.T) {
	s := Shell{
		Runner: newTestRunner(map[string]string{"LOGNAME": "impostor"}),
		Style:  userStyle(),
	}
	if got := s.render(`<\u>`); got != "<>" {
		t.Errorf(`\u drew %q, want "<>"`, got)
	}
	// And with no Runner at all, which is the shape a caller assembling a
	// front end by hand can produce.
	bare := Shell{Style: userStyle()}
	if got := bare.render(`<\u><\h>`); got != "<><>" {
		t.Errorf(`drew %q, want "<><>"`, got)
	}
}
