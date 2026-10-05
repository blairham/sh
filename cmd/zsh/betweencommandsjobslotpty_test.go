// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"regexp"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A function the shell calls between commands — a prompt hook, a widget a key
// ran — holds no job number, so a job its body starts is [1] in an empty table
// (#5891). A function called *by a command* does hold one, which is why the
// same body typed as `f` at the prompt is [2] in both shells.
//
// Measured 2026-10-04 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), through a
// pseudo-terminal from a scratch `ZDOTDIR`, each body `sleep 1 & print -r --
// "X=${(k)jobstates}"`: `precmd`, a `precmd_functions` member, `preexec` and a
// widget bound to a key all say `X=1`; `precmd() { { … } }` says `X=2`,
// because the brace group in the body holds the number the call did not. This
// shell said `X=2` for all of them, numbering the hook like a typed call.
//
// The brace-group row is the control: it is a `2` from the same hook in the
// same session shape, so a `1` below is the call holding nothing and not an
// instrument that cannot see a second number.
func TestAFunctionCalledBetweenCommandsHoldsNoJobNumber(t *testing.T) {
	const body = `sleep 1 & print -r -- "X=${(k)jobstates}"`
	cases := []struct {
		name, rc, typed, want string
	}{
		{
			name: "precmd",
			rc:   "precmd() { " + body + "; }\n",
			want: "1",
		},
		{
			name: "a precmd_functions member",
			rc:   "p() { " + body + "; }; precmd_functions=(p)\n",
			want: "1",
		},
		{
			name: "a brace group in precmd holds the number the call did not",
			rc:   "precmd() { { " + body + "; } }\n",
			want: "2",
		},
		{
			name:  "preexec",
			rc:    "preexec() { " + body + "; }\n",
			typed: ":",
			want:  "1",
		},
		{
			name:  "a widget a key ran",
			rc:    "w() { " + body + "; }; zle -N w; bindkey '^T' w\n",
			typed: "\x14",
			want:  "1",
		},
	}
	// The value is read off its own row, whole: `X=12` is not `X=1`, and
	// nothing typed contains the row, because the body is in the startup file.
	row := regexp.MustCompile(`(?m)^X=(\S*)\r?$`)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			control, screen, _ := jobNoticeSessionRC(t, "zmodload zsh/parameter\n"+c.rc, "zsh", "-i")
			if c.typed != "" {
				jobNoticeType(t, control, screen, c.typed)
			}
			got := row.FindStringSubmatch(screen.Text())
			if got == nil {
				t.Fatalf("the body never printed its row:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 12)))
			}
			if got[1] != c.want {
				t.Errorf("the job is numbered %s, want %s:\n%s", got[1], c.want,
					smoke.Readable(smoke.LastLines(screen.Text(), 12)))
			}
		})
	}
}
