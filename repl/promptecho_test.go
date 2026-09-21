// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// A terminal echoes a keystroke itself. Nothing does where the input is a
// pipe or a file, so what a person would have seen on the screen is written
// by the shell or is never written at all — and the panel splits on which.
//
// Both answers are here rather than one, because the cost of getting this
// wrong is silent in both directions: a session that echoes where the shell
// does not doubles every line, and one that does not where the shell does
// loses the whole left-hand side of a transcript while every command still
// runs and every answer is still right. The second is what this tree did,
// and the row that found it — a suite file driving `shell -i` from a
// here-document — compared as disagreeing on half its lines for it (#2298).
func TestAPromptEchoesTheLineWhereThereIsNoTerminal(t *testing.T) {
	const text = "echo one\nfor i in 1 2\ndo\necho $i\ndone\n\n"
	for _, c := range []struct {
		name string
		echo bool
		errs string
	}{
		{
			// Every line read, each behind the prompt it was read at, the
			// continuation lines included and the blank one too — a read is
			// being reproduced and not a decision, so a line that runs
			// nothing appears exactly like one that does.
			name: "the dialect that echoes writes every line it read",
			echo: true,
			errs: "$ echo one\n$ for i in 1 2\n> do\n> echo $i\n> done\n$ \n$ ",
		},
		{
			name: "and the one that does not writes only its prompts",
			echo: false,
			errs: "$ $ > > > $ $ ",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var ran, said strings.Builder
			r := newTestRunner(map[string]string{"PS1": "$ ", "PS2": "> "})
			r.Stdout = &ran
			s := Shell{
				Runner:                      r,
				In:                          strings.NewReader(text),
				Out:                         &ran,
				Err:                         &said,
				EchoTheLineWithoutATerminal: c.echo,
			}
			if _, err := s.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			// The commands ran either way. Asserted in both rows, because an
			// echo that replaced the run rather than joining it would make
			// the row above pass while the shell did nothing.
			if want := "one\n1\n2\n"; ran.String() != want {
				t.Errorf("output = %q, want %q", ran.String(), want)
			}
			if said.String() != c.errs {
				t.Errorf("stderr = %q, want %q", said.String(), c.errs)
			}
		})
	}
}

// A last line the input never ended still gave the shell a line, so it is
// written back with the newline the read did not find. Without that the next
// thing the session writes lands on the end of it.
func TestAnEchoedLineGetsTheNewlineTheReadDidNotFind(t *testing.T) {
	var ran, said strings.Builder
	r := newTestRunner(map[string]string{"PS1": "$ ", "PS2": "> "})
	r.Stdout = &ran
	s := Shell{
		Runner:                      r,
		In:                          strings.NewReader("echo one"),
		Out:                         &ran,
		Err:                         &said,
		EchoTheLineWithoutATerminal: true,
		Leaving:                     "bye",
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	// The prompt the session drew before finding the end is part of it,
	// exactly as it is in the shell this was measured against: a prompt is
	// written before the read, and the read is what says there is no more.
	if want := "$ echo one\n$ bye\n"; said.String() != want {
		t.Errorf("stderr = %q, want %q", said.String(), want)
	}
}

// What a session writes as it ends, which one dialect in the panel has a word
// for and the rest do not.
//
// Both loops write it through one function, and this drives the one a test
// can reach. The two ways a session ends are one answer and not two —
// measured, the shell that has a word writes it whether the input ran out or
// a line ran `exit` — so both are asked here.
func TestASessionWritesTheDialectsWordForLeaving(t *testing.T) {
	for _, c := range []struct {
		name, text, leaving, errs string
	}{
		{"the input ran out", "echo one\n", "exit", "$ $ exit\n"},
		{"a line ran exit", "exit 3\n", "exit", "$ exit\n"},
		{"and a dialect with no word writes none", "echo one\n", "", "$ $ "},
	} {
		t.Run(c.name, func(t *testing.T) {
			var ran, said strings.Builder
			r := newTestRunner(map[string]string{"PS1": "$ "})
			r.Stdout = &ran
			s := Shell{
				Runner:  r,
				In:      strings.NewReader(c.text),
				Out:     &ran,
				Err:     &said,
				Leaving: c.leaving,
			}
			if _, err := s.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			if said.String() != c.errs {
				t.Errorf("stderr = %q, want %q", said.String(), c.errs)
			}
		})
	}
}
