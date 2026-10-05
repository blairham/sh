// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// onATerminal names a terminal whose description has the attribute strings,
// since `%B`, `%U` and `%S` write the terminal's own and a runner with no
// TERM has none — measured, they write nothing at all under `TERM=dumb`.
func onATerminal(src string) string { return "TERM=xterm-256color\n" + src }

// What a `compadd -X` explanation and a `-x` message draw: the part of the
// prompt language the manual gives them, and `%n` for a count.
//
// Every row was measured on zsh 5.9.2, 2026-10-05, through a pseudo-terminal
// with a `.list-choices` widget calling the builtin, and is the bytes zsh
// wrote for the heading row. They were drawn literally until #6146, so a
// format style's `%F{yellow}` stood over every block of every listing.
func TestAnExplanationDrawsItsEscapes(t *testing.T) {
	for _, c := range []struct{ name, call, want string }{
		{
			// The issue's own row: a format-style heading, `%n` as the count
			// of the two matches, `%d` as nothing, `%%`, and a color left on.
			"a format style", `compadd -J g -X ' %F{yellow}-- %n %d x %% --%f%B b%b%{Q%}%1F' -- checkout cherry`,
			"checkout@| \x1b[33m-- 2  x % --\x1b[39m\x1b[1m b\x1b[0mQ\x1b[31m cherry@| \x1b[33m-- 2  x % --\x1b[39m\x1b[1m b\x1b[0mQ\x1b[31m",
		},
		{
			// A count in front of `%n` changes nothing, and every other
			// letter is nothing at all.
			"other letters", `compadd -J g -X '[n=%n][d=%d][3n=%3n][q=%q][N=%N]' -- checkout cherry`,
			"checkout@|[n=2][d=][3n=2][q=][N=] cherry@|[n=2][d=][3n=2][q=][N=]",
		},
		{
			// The rest of the prompt language is not here: a conditional, a
			// format, a truncation and a clear are unknown codes, dropped
			// with their letter, and a trailing `%` is dropped.
			"no conditional, format or truncation", `compadd -J g -X 'E[%E]G[%G]C[%(?.t.f)]D[%D{%Y}]T[%3<..<abc]P[%' -- checkout`,
			"checkout@|E[]G[]C[?.t.f)]D[{}]T[..<abc]P[",
		},
		{
			// `%n` counts what this call added, duplicates included: three
			// of the four match `che`.
			"%n counts duplicates", `compadd -J g -X '%n' -- checkout zzz cherry checkout`,
			"checkout@|3 cherry@|3",
		},
		{
			// And a message's `%n` is -1, matches or not.
			"a message counts -1", `compadd -J g -x 'm%n' -- checkout`,
			"checkout@|m-1",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := drawn(completionCandidatesFor(t, onATerminal(widgetOf(c.call)), "git che"))
			if got != c.want {
				t.Errorf("%s drew %q, want %q", c.call, got, c.want)
			}
		})
	}
}

// A restore inside an explanation writes back boldface, standout and
// underline, and never a color — where the same text as a prompt writes the
// color back too. Measured row for row against `compadd -X`; the prompt's
// answers are in the comment on interp.PromptStyle.RestoreLeavesColors.
func TestAnExplanationRestoresNoColor(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"%F{red}a%bz", "\x1b[31ma\x1b[0mz"},
		{"%F{red}%By%f", "\x1b[31m\x1b[1my\x1b[39m"},
		{"%F{red}%Ua%ub", "\x1b[31m\x1b[4ma\x1b[24mb"},
		{"%B%F{red}%Sb%sc", "\x1b[1m\x1b[31m\x1b[7mb\x1b[27m\x1b[1mc"},
		{"%U%S%Ba%bc", "\x1b[4m\x1b[7m\x1b[1ma\x1b[0m\x1b[7m\x1b[4mc"},
		{"%K{blue}%Ba%b.%Uu%u", "\x1b[44m\x1b[1ma\x1b[0m.\x1b[4mu\x1b[24m"},
		{"%Bx%Uy%u", "\x1b[1mx\x1b[4my\x1b[24m\x1b[1m"},
		{"%2Fx%30Fy%f", "\x1b[32mx\x1b[38;5;30my\x1b[39m"},
	} {
		t.Run(c.text, func(t *testing.T) {
			got := drawn(completionCandidatesFor(t, onATerminal(widgetOf("compadd -J g -X '"+c.text+"' -- checkout")), "git che"))
			if want := "checkout@|" + c.want; got != want {
				t.Errorf("%s drew %q, want %q", c.text, got, want)
			}
		})
	}
}

// A `-X` explanation is drawn only where its own call added a match, even
// when a later call fills the block. Measured: `compadd -X H -J g nomatch`
// then `compadd -J g alfa` draws `alfa` with no heading over it.
func TestAnExplanationWhoseCallAddedNothingIsNotDrawn(t *testing.T) {
	got := drawn(completionCandidatesFor(t, widgetOf(
		"compadd -J g -X 'unheard' -- zzz\ncompadd -J g -X 'heard' -- checkout\n"), "git che"))
	if want := "checkout@|heard"; got != want {
		t.Errorf("drew %q, want %q", got, want)
	}
}
