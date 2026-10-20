// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/repl"
)

// `compadd -E`, `-2`, a repeated explanation and `packed` in
// `$compstate[list]`: the four things the grid compdescribe builds needs from
// `compadd` (#6178). Each row was measured on zsh 5.9.2, 2026-10-05, through
// a pseudo-terminal from inside a `.list-choices` widget; compadd.go has the
// tables.

// cellsDrawn writes each candidate as `word@row` with the block it is in, so
// that a filler in the wrong block and a block that lost `packed` both show.
func cellsDrawn(candidates []repl.Candidate) string {
	var out []string
	for _, c := range candidates {
		row := c.Word + "@" + c.Display + "[" + strings.ReplaceAll(c.Group.Name, "\x00", "~")
		if c.Filler {
			row += " filler"
		}
		if c.Group.Packed {
			row += " packed"
		}
		if c.Group.Heading != "" {
			row += " " + strings.ReplaceAll(c.Group.Heading, "\n", "/")
		}
		out = append(out, row+"]")
	}
	return strings.Join(out, " ")
}

func TestCompaddFillersAndTheBlockTheyJoin(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{
		{
			// Under `-2`, a filler joins the names whatever its own letter.
			"a filler joins a -2 block of its name",
			`compadd -2V ej -- -b -a; compadd -E1 -J ej -d '(DESC)'`,
			"-b@[ej] -a@[ej] @DESC[ej filler]",
		},
		{
			"and under -V too", `compadd -2V ej -- -b -a; compadd -E1 -V ej -d '(DESC)'`,
			"-b@[ej] -a@[ej] @DESC[ej filler]",
		},
		{
			// Without one it is a block of its own, even under the names' -J.
			"without -2 it is its own block", `compadd -J ej -- -b -a; compadd -E1 -J ej -d '(DESC)'`,
			"-b@[ej] -a@[ej] @DESC[ej~fillers filler]",
		},
		{
			"nor does another name join", `compadd -2V ej -- -b; compadd -E1 -J other -d '(DESC)'`,
			"-b@[ej] @DESC[other~fillers filler]",
		},
		{
			// A blank filler is still a cell, and a heading is drawn over
			// fillers alone.
			"blank fillers under a heading", `compadd -E2 -J g -X HEAD`,
			"@[g~fillers filler HEAD] @[g~fillers filler HEAD]",
		},
		{
			// The same heading twice is drawn once; a different one is not
			// merged into it.
			"one heading for one explanation", `compadd -J g -X H1 -- alpha; compadd -J g -X H2 -- beta; compadd -J g -X H1 -- gamma`,
			"alpha@[g H1/H2] beta@[g H1/H2] gamma@[g H1/H2]",
		},
		{
			// `packed` counts where it stood when the call was made.
			"packed before the call", `compstate[list]="$compstate[list] packed"; compadd -J g -- a1 a2`,
			"a1@[g packed] a2@[g packed]",
		},
		{
			"packed after it", `compadd -J g -- a1 a2; compstate[list]="$compstate[list] packed"`,
			"a1@[g] a2@[g]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := cellsDrawn(completionCandidatesFor(t, widgetOf(c.body), "x "))
			if got != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.body, got, c.want)
			}
		})
	}
}

// TestFillersAreCountedAndAnswerZero — `compadd -E2 -J g` answers 0 and
// leaves `$compstate[nmatches]` at 2; `-E-1` is refused.
func TestFillersAreCountedAndAnswerZero(t *testing.T) {
	got := reported(t, `local out
		compadd -E2 -J g; out+="st=$? n=$compstate[nmatches] "
		compadd -E1 -J g -- aa; out+="st=$? n=$compstate[nmatches] "
		compadd -E-1 -J g 2>/dev/null; out+="neg=$?"
		say "$out"`, "x ")
	if want := "st=0 n=2 st=0 n=4 neg=1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
