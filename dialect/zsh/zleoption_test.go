// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `zle` follows **interactive** and not the terminal.
//
// That is the measurement, and it is not what the manual's wording suggests:
// `zsh -i -c …` with no terminal anywhere reports it on, and `zsh -c …` on a
// pseudo-terminal reports it off. Both real builds on this machine agree —
// 5.9 and 5.9.2 — so the name is about the kind of shell, not the fds.
//
// It was a constant `off` here, which is right for the `-c` shell it was
// measured in and wrong for every interactive one. powerlevel10k's instant
// prompt is guarded on `[[ … -o zle … ]]`, so the constant meant the cached
// prompt was never printed and a real configuration took 400ms to a prompt
// where real zsh takes 33ms (#2121).
func TestZleIsOnAtAnInteractivePromptAndOffInAScript(t *testing.T) {
	for _, tc := range []struct {
		name        string
		interactive bool
		want        string
	}{
		{"an interactive shell has the line editor", true, "on\n"},
		{"a script does not", false, "off\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := dialecttest.Base{Dir: t.TempDir(), Interactive: tc.interactive}
			out, st, err := preset.Combined(t, base, `[[ -o zle ]] && print -r on || print -r off`)
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// At an interactive prompt it is the shape `monitor` has and not the shape
// `shinstdin` has: freely movable in both directions, granted at 0, and it
// really moves. Measured in an interactive zsh, both builds alike.
func TestZleMovesBothWaysAtAnInteractivePrompt(t *testing.T) {
	const src = `unsetopt zle; s1=$?
[[ -o zle ]] && a=on || a=off
setopt zle; s2=$?
[[ -o zle ]] && b=on || b=off
print -r -- "$s1 $a $s2 $b"`
	base := dialecttest.Base{Dir: t.TempDir(), Interactive: true}
	out, st, err := preset.Combined(t, base, src)
	if err != nil {
		t.Fatal(err)
	}
	if want := "0 off 0 on\n"; out != want || st != 0 {
		t.Errorf("out %q status %d, want %q at 0 — both directions granted, and the state follows", out, st, want)
	}
}

// And in a script it is still held still, which is the half the table already
// had and the half every existing test pins: asking for the state it is
// already in is granted, and asking it to move is zsh's own sentence at 1.
//
// This is the pair that says the new entry did not simply become movable
// everywhere — a `set` that always ran would answer `0` here and take the
// refusal with it.
func TestZleIsStillImmovableInAScript(t *testing.T) {
	base := dialecttest.Base{Dir: t.TempDir()}
	out, _, err := preset.Combined(t, base, `setopt zle; print -r -- "status $?"`)
	if err != nil {
		t.Fatal(err)
	}
	want := "zsh:setopt:1: can't change option: zle\nstatus 1\n"
	if out != want {
		t.Errorf("out %q, want %q", out, want)
	}
	// Off is where it already is, so asking for that is granted in silence.
	out, _, err = preset.Combined(t, base, `unsetopt zle; print -r -- "status $?"`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "status 0\n"; out != want {
		t.Errorf("unsetopt in a script: out %q, want %q", out, want)
	}
}

// A subshell keeps its own answer, which is what the recorded store buys and
// what a bit hung off the Runner would not: `(unsetopt zle)` does not reach
// the shell that started it.
func TestZleMovedInASubshellStaysThere(t *testing.T) {
	base := dialecttest.Base{Dir: t.TempDir(), Interactive: true}
	out, st, err := preset.Combined(t, base,
		`(unsetopt zle; [[ -o zle ]] && print -rn in=on || print -rn in=off); `+
			`[[ -o zle ]] && print -r " out=on" || print -r " out=off"`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "in=off out=on\n"; out != want || st != 0 {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}
