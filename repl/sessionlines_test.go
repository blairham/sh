// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// One dialect numbers a line typed at a prompt by how many the session has
// read, where the numbering otherwise restarts at 1 for every construct.
//
// Measured 2026-09-12, three lines piped into `dash -i` with a scratch HOME:
// `echo one`, `if; then`, `echo three` answers `dash: 2: Syntax error: ";"
// unexpected`, and `nosuchcmd_zz` twice answers line 1 and then line 2. The
// other three panel shells name no line at a prompt at all, for either
// failure, so the number is invisible to them (#2022).

// sessionLines runs text through a session and returns what each refusal was
// told the line was, from both routes at once: the parse one, which reads the
// error's position, and the run-time one, which reads the tree's.
func sessionLines(t *testing.T, text string, counting bool) string {
	t.Helper()
	var ran, said strings.Builder
	r := newTestRunner(map[string]string{"PS1": "", "PS2": ""})
	r.Stdout = &ran
	s := Shell{
		Runner:            r,
		In:                strings.NewReader(text),
		Out:               &ran,
		Err:               &said,
		CountSessionLines: counting,
		Report: func(err error) string {
			var se *syntax.Error
			if !errors.As(err, &se) {
				return "refused at ?\n"
			}
			return fmt.Sprintf("refused at %d\n", se.Pos.Line)
		},
		ParseFailureStatus: func(error) int { return 2 },
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatalf("run %q: %v", text, err)
	}
	return said.String()
}

func TestALineMayBeNumberedByTheSession(t *testing.T) {
	const text = "echo one\nfi\nfi\n"
	if got, want := sessionLines(t, text, false), "refused at 1\nrefused at 1\n"; got != want {
		t.Errorf("without counting = %q, want %q — every construct starts at 1", got, want)
	}
	if got, want := sessionLines(t, text, true), "refused at 2\nrefused at 3\n"; got != want {
		t.Errorf("counting = %q, want %q — the session's own lines", got, want)
	}
}

// A construct typed over several lines is numbered from the line it began on,
// and the lines it took are counted: what follows it is not back at 1.
func TestAMultiLineConstructAdvancesTheCount(t *testing.T) {
	const text = "echo one\nif true\nthen\necho two\nfi\nfi\n"
	if got, want := sessionLines(t, text, true), "refused at 6\n"; got != want {
		t.Errorf("counting = %q, want %q", got, want)
	}
}

// And a blank line is a line: dash counts what was read, not what was run.
func TestABlankLineIsCounted(t *testing.T) {
	const text = "echo one\n\n\nfi\n"
	if got, want := sessionLines(t, text, true), "refused at 4\n"; got != want {
		t.Errorf("counting = %q, want %q", got, want)
	}
}

// And the run-time route agrees with the parse one, which is why the number
// is carried in at the parse rather than written by whatever reports: a
// diagnostic about the text as written comes from the error's position and one
// about running it comes from the tree's, and the two are on the same screen.
func TestTheRunTimeRouteIsNumberedTheSameWay(t *testing.T) {
	var ran, said strings.Builder
	r := newTestRunner(map[string]string{"PS1": "", "PS2": ""})
	r.Stdout = &ran
	r.Stderr = &said
	r.Diagnostics = &interp.Diagnostics{Location: interp.LocationColonLine, SelfName: "testsh"}
	s := Shell{
		Runner:            r,
		In:                strings.NewReader("echo one\nnosuchcmd_zz\nnosuchcmd_zz\n"),
		Out:               &ran,
		Err:               &said,
		CountSessionLines: true,
		Name:              "testsh",
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"testsh: 2:", "testsh: 3:"} {
		if !strings.Contains(said.String(), want) {
			t.Errorf("stderr = %q, want a line naming %q", said.String(), want)
		}
	}
}
