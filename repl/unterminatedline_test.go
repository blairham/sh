// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"io"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// An input ending partway through its last line has a prompt drawn once more
// before that line runs: ash's the prompt it was read at, ksh93's the
// continuation prompt, and dash's none. Measured 2026-10-07 on a pipe (#6331).
// See interp.Semantics.PromptAgainForAnUnterminatedLine.
func TestAnUnterminatedLastLineIsPromptedForAgain(t *testing.T) {
	const construct = "if true\nthen echo A; fi"
	for _, c := range []struct {
		name   string
		answer interp.UnterminatedLinePrompt
		in     string
		errs   string
	}{
		{"none", interp.NoPromptForAnUnterminatedLine, "echo A", "P> P> "},
		{"the same prompt", interp.SamePromptForAnUnterminatedLine, "echo A", "P> P> P> "},
		{"the same prompt inside a construct", interp.SamePromptForAnUnterminatedLine, construct, "P> Q> Q> P> "},
		{"the continuation prompt", interp.ContinuationPromptForAnUnterminatedLine, "echo A", "P> Q> P> "},
		{"the continuation prompt inside a construct", interp.ContinuationPromptForAnUnterminatedLine, construct, "P> Q> Q> P> "},
		{"nothing where the line was ended", interp.SamePromptForAnUnterminatedLine, "echo A\n", "P> P> "},
	} {
		t.Run(c.name, func(t *testing.T) {
			var ran, said strings.Builder
			r := newTestRunner(map[string]string{"PS1": "P> ", "PS2": "Q> "})
			r.Stdout = &ran
			s := Shell{Runner: r, In: strings.NewReader(c.in), Out: io.Discard, Err: &said, PromptAgainForAnUnterminatedLine: c.answer}
			if _, err := s.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			if said.String() != c.errs {
				t.Errorf("stderr %q, want %q", said.String(), c.errs)
			}
			if ran.String() != "A\n" {
				t.Errorf("stdout %q, want the line run", ran.String())
			}
		})
	}
}
