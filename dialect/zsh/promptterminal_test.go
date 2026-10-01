// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/terminfofixture"
)

// The string capabilities the attribute codes read, by their index in the
// compiled format: terminfo(5)'s order. `cuu1` is among them because a
// terminal with no way up draws no attribute at all.
const (
	capClrEol            = 6   // el
	capCursorUp          = 19  // cuu1
	capEnterBoldMode     = 27  // bold
	capEnterStandoutMode = 35  // smso
	capEnterUnderline    = 36  // smul
	capExitAttributeMode = 39  // sgr0
	capExitStandoutMode  = 43  // rmso
	capExitUnderline     = 44  // rmul
	capExitAltCharset    = 38  // rmacs
	capSetAttributes     = 131 // sgr
)

// xtermLikeTerminal writes a description holding the attribute strings
// xterm-256color holds — the terminal the prompt rows in this package were
// measured under — and answers the assignments that point a script at it.
// `sgr0`, `sgr` and `rmacs` are xterm's own, so a row reading `%b` exercises
// the termcap `me` they come to, `\e[0m`.
func xtermLikeTerminal(t *testing.T) string {
	t.Helper()
	dir := terminfofixture.Database(t, terminfofixture.Description{
		Name: "zshxterm", StrCount: capSetAttributes + 1,
		Strs: map[int]string{
			capExitAltCharset:    "\x1b(B",
			capSetAttributes:     "%?%p9%t\x1b(0%e\x1b(B%;\x1b[0%?%p6%t;1%;%?%p5%t;2%;%?%p2%t;4%;%?%p1%p3%|%t;7%;%?%p4%t;5%;%?%p7%t;8%;m",
			capClrEol:            "\x1b[K",
			capCursorUp:          "\x1b[A",
			capEnterBoldMode:     "\x1b[1m",
			capEnterStandoutMode: "\x1b[7m",
			capEnterUnderline:    "\x1b[4m",
			capExitAttributeMode: "\x1b(B\x1b[m",
			capExitStandoutMode:  "\x1b[27m",
			capExitUnderline:     "\x1b[24m",
		},
	})
	return "TERM=zshxterm TERMINFO=" + singleQuote(dir) + " TERMINFO_DIRS=; "
}

// **The attribute codes write the terminal's own strings, and nothing where
// it has none** (#5289). Measured 2026-10-01 on zsh 5.9.2 with `print -rnP
// '%B|%b|%U|%u|%S|%s|%E|%F{red}'`: with TERM unset, `dumb` or naming no entry
// every attribute is empty and `%F{red}` is still `\e[31m`; under screen the
// standout pair is that entry's `\e[3m` and `\e[23m`; vt100's padded `bold`
// writes without its `$<2>`; and `%b` writes the termcap `me`, which xterm's
// `\E(B\E[m` comes to as `\e[0m` — the same string `$termcap[me]` reads.
// The fixtures below were also compiled with `tic` and handed to the
// reference, which wrote these rows: the screen-like one carries no `sgr`, so
// its `%b` is its `sgr0` as written.
func TestTheAttributeCodesReadTheTerminal(t *testing.T) {
	const codes = `v='%B|%b|%U|%u|%S|%s|%E|%F{red}'; print -rn -- "${(%%)v}"; print`
	screen := terminfofixture.Database(t, terminfofixture.Description{
		Name: "zshscreen", StrCount: capExitUnderline + 1,
		Strs: map[int]string{
			capClrEol: "\x1b[K", capCursorUp: "\x1bM", capEnterBoldMode: "\x1b[1m$<2>", capEnterStandoutMode: "\x1b[3m",
			capEnterUnderline: "\x1b[4m", capExitAttributeMode: "\x1b[m\x0f",
			capExitStandoutMode: "\x1b[23m", capExitUnderline: "\x1b[24m",
		},
	})
	bare := terminfofixture.Database(t, terminfofixture.Description{
		Name: "zshbare", StrCount: capEnterBoldMode + 1,
		Strs: map[int]string{capCursorUp: "\x1b[A", capEnterBoldMode: "\x1b[1m"},
	})
	// And the same attributes with no way to move up, which zsh draws none
	// of: measured with tic'd descriptions, `cuu1` is the one capability that
	// turned them on.
	noUp := terminfofixture.Database(t, terminfofixture.Description{
		Name: "zshnoup", StrCount: capExitUnderline + 1,
		Strs: map[int]string{capEnterBoldMode: "\x1b[1m", capEnterStandoutMode: "\x1b[7m", capExitAttributeMode: "\x1b[m"},
	})
	for _, c := range []struct{ name, setup, want string }{
		{"no TERM", "unset TERM; ", "|||||||\x1b[31m\n"},
		{"a TERM naming no entry", "TERM=nosuchterminal TERMINFO=" + singleQuote(bare) + " TERMINFO_DIRS=; ", "|||||||\x1b[31m\n"},
		{"xterm's strings", xtermLikeTerminal(t), "\x1b[1m|\x1b[0m|\x1b[4m|\x1b[24m|\x1b[7m|\x1b[27m|\x1b[K|\x1b[31m\n"},
		{"screen's, padding off", "TERM=zshscreen TERMINFO=" + singleQuote(screen) + " TERMINFO_DIRS=; ", "\x1b[1m|\x1b[m\x0f|\x1b[4m|\x1b[24m|\x1b[3m|\x1b[23m|\x1b[K|\x1b[31m\n"},
		{"no way up", "TERM=zshnoup TERMINFO=" + singleQuote(noUp) + " TERMINFO_DIRS=; ", "|||||||\x1b[31m\n"},
		{"an entry with bold alone", "TERM=zshbare TERMINFO=" + singleQuote(bare) + " TERMINFO_DIRS=; ", "\x1b[1m|||||||\x1b[31m\n"},
	} {
		out, st := runZsh(t, t.TempDir(), c.setup+codes)
		if out != c.want || st != 0 {
			t.Errorf("%s: got %q (status %d), want %q", c.name, out, st, c.want)
		}
	}
	out, st := runZsh(t, t.TempDir(), xtermLikeTerminal(t)+`zmodload zsh/termcap; print -r -- "${(V)termcap[me]}"`)
	if want := "^[[0m\n"; out != want || st != 0 {
		t.Errorf("$termcap[me]: got %q (status %d), want %q", out, st, want)
	}
}
