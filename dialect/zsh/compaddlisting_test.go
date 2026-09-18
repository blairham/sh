// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/repl"
)

// What `compadd` says a listing should look like: the row drawn for each
// match, and the block it is drawn in.
//
// Every row here was measured through a pseudo-terminal against zsh 5.9.2 on
// 2026-09-18, from inside a `zle -C probewid .complete-word _probe` bound to a
// key with a two-row prompt, and read off the screen rather than out of a
// parameter — a listing is the one thing a completion function cannot print
// for itself. The measurements are written out in compadd.go and
// compgroups.go; what is here is them, asked of this shell.
//
// The shape of the assertion is `word@row|heading` per candidate, because all
// three have been wrong independently: a `-d` that reached the wrong match, a
// group that lost its heading, and a heading that landed on the words instead
// of beside them.

// drawn is one completion's candidates written out so that a row and a block
// that went missing are both visible in the failure.
func drawn(candidates []repl.Candidate) string {
	var out []string
	for _, c := range candidates {
		row := c.Word + "@" + c.Display
		if c.Group.Heading != "" {
			row += "|" + c.Group.Heading
		}
		if c.Group.Unsorted {
			row += "|unsorted"
		}
		if c.Group.OnePerLine {
			row += "|perline"
		}
		out = append(out, row)
	}
	return strings.Join(out, " ")
}

func TestCompaddSaysHowTheMatchesAreDrawn(t *testing.T) {
	for _, c := range []struct{ name, call, want string }{
		// `-d` replaces the drawn text outright. Measured: `compadd -d '(one
		// two)' -- checkout cherry` with `che` typed lists `one  two` and
		// inserts `checkout` — so the row is not the name with something
		// added to it, it is a different string.
		{
			"-d replaces the row", "compadd -d '(one two)' -- checkout cherry",
			"checkout@one cherry@two",
		},
		// And it takes an array's name as readily as a literal, which is what
		// `compdescribe`'s caller writes.
		{
			"-d from an array", "local -a disp=(DA DB); compadd -d disp -- checkout cherry",
			"checkout@DA cherry@DB",
		},
		// A `-d` list shorter than the candidates leaves the rest drawn as
		// their words, which is the only thing left to draw them as.
		{
			"-d shorter than the candidates", "compadd -d '(one)' -- checkout cherry",
			"checkout@one cherry@",
		},
		// `-J` names a block and `-V` names one that keeps its order. The
		// flag travels with every candidate in it, since that is what makes
		// the block the identity of a listing arrangement.
		{
			"-J names a sorted block", "compadd -J g1 -- checkout cherry",
			"checkout@ cherry@",
		},
		{
			"-V names an unsorted one", "compadd -V g2 -- checkout cherry",
			"checkout@|unsorted cherry@|unsorted",
		},
		// `-l` is one row per line, which is what a row carrying a sentence
		// needs.
		{
			"-l asks for a row each", "compadd -l -- checkout",
			"checkout@|perline",
		},
		// `-X` heads the block.
		{
			"-X heads the block", "compadd -X 'the explanation' -- checkout",
			"checkout@|the explanation",
		},
		// And `-x` heads it too, and wins where a call carries both.
		// Measured: `-X 'with matches' -x 'msg-with-matches'` draws the
		// message.
		{
			"-x wins over -X", "compadd -X 'the explanation' -x 'the message' -- checkout",
			"checkout@|the message",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := drawn(completionCandidatesFor(t, widgetOf(c.call), "git che"))
			if got != c.want {
				t.Errorf("%s drew %q, want %q", c.call, got, c.want)
			}
		})
	}
}

// A message is drawn whether or not its block has anything in it, and an
// explanation is not.
//
// Measured, and it is the pair that says what the two letters are for: `-J g
// -x 'msg-no-matches'` draws the message over an empty block, and `-J g -X
// 'no matches here'` draws nothing at all. So `-x` is a completion system
// saying something and `-X` heads a block that exists.
func TestAMessageSurvivesAnEmptyBlockAndAnExplanationDoesNot(t *testing.T) {
	for _, c := range []struct{ name, call, want string }{
		{"a message with no matches", "compadd -J g -x 'a message'", "@|a message"},
		{"an explanation with no matches", "compadd -J g -X 'a heading'", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := drawn(completionCandidatesFor(t, widgetOf(c.call), "git che"))
			if got != c.want {
				t.Errorf("%s drew %q, want %q", c.call, got, c.want)
			}
		})
	}
}

// Two calls naming one block are one block, and both explanations are drawn.
//
// Measured: `-J gx -X 'first heading'` then `-J gx -X 'second heading'` draws
// two heading rows over one sorted block of `delta  gamma`. So a heading
// cannot be what identifies a block, which is the reason the headings are
// stamped on at the end rather than when each call runs.
func TestTwoCallsNamingOneBlockShareItAndBothAreHeard(t *testing.T) {
	got := drawn(completionCandidatesFor(t, widgetOf(
		"compadd -J gx -X 'first heading' -- checkout\n"+
			"compadd -J gx -X 'second heading' -- cherry\n"), "git che"))
	want := "checkout@|first heading\nsecond heading cherry@|first heading\nsecond heading"
	if got != want {
		t.Errorf("drew %q, want %q", got, want)
	}
}

// Two calls naming no block are one block too, and the first explanation
// heads it.
//
// Measured: `-X 'unnamed heading' -- alpha` then a plain `compadd -- beta`
// draws the heading once over `alpha  beta`.
func TestTwoUnnamedCallsShareOneBlock(t *testing.T) {
	got := drawn(completionCandidatesFor(t, widgetOf(
		"compadd -X 'unnamed heading' -- checkout\ncompadd -- cherry\n"), "git che"))
	want := "checkout@|unnamed heading cherry@|unnamed heading"
	if got != want {
		t.Errorf("drew %q, want %q", got, want)
	}
}

// `compgroups` decides the order the blocks come out in, whatever order they
// were filled in.
//
// Measured on zsh 5.9.2, 2026-09-18: `compgroups second first` followed by a
// `compadd -J first -X FIRST` and a `compadd -J second -X SECOND` draws
// `SECOND` over `beta` and then `FIRST` over `alpha`. It was a no-op here
// until there were blocks for it to order (#3232).
func TestCompgroupsDecidesTheOrderTheBlocksAreDrawnIn(t *testing.T) {
	src := widgetOf(
		"compgroups second first\n" +
			"compadd -J first  -X FIRST  -- checkout\n" +
			"compadd -J second -X SECOND -- cherry\n")
	got := drawn(completionCandidatesFor(t, src, "git che"))
	if want := "cherry@|SECOND checkout@|FIRST"; got != want {
		t.Errorf("drew %q, want %q — the declaration and not the order of the calls", got, want)
	}
	// And without the declaration the calls decide, which is what says the
	// row above is the declaration doing something.
	src = widgetOf(
		"compadd -J first  -X FIRST  -- checkout\n" +
			"compadd -J second -X SECOND -- cherry\n")
	got = drawn(completionCandidatesFor(t, src, "git che"))
	if want := "checkout@|FIRST cherry@|SECOND"; got != want {
		t.Errorf("drew %q, want %q", got, want)
	}
}
