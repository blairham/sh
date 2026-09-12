// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/repl"
)

// A gated session carries the mark, and an ungated one carries nothing.
//
// The wiring rather than the drawing, because the wiring is what is invisible:
// a provider dropped between installSeams and driver.Shell looks exactly like
// a binary that never had one, and the prompt is drawn only where there is a
// terminal. What it draws is driver's to grade, since the mark travels with
// the gate now that every binary can install one — see driver's own test.
func TestOnlyAGatedSessionCarriesTheSandboxMark(t *testing.T) {
	for _, c := range []struct {
		name string
		own  ownFlags
		want bool
	}{
		{"a plain shell", ownFlags{}, false},
		{"-deny", ownFlags{deny: []string{"/etc"}}, true},
		// Watching a shell is not constraining it, so a sink alone is not a
		// sandbox and does not say it is one.
		{"-trace-events alone", ownFlags{traceEvents: true}, false},
		// Both halves of the debug surface at once still mark exactly once.
		// Two gates composed is one sandboxed session, not two.
		{"-deny with -trace-events", ownFlags{deny: []string{"/etc", "/var"}, traceEvents: true}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			sh, _, err := installSeams(driver.Shell{}, c.own, &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			if got := len(sh.PromptProviders) > 0; got != c.want {
				t.Errorf("providers=%v, want %v (gate=%v)", got, c.want, sh.Gate != nil)
			}
			if !c.want {
				return
			}
			if n := len(sh.PromptProviders); n != 1 {
				t.Fatalf("%d providers, want exactly one", n)
			}
			if got := sh.PromptProviders[0].Prompt(repl.PromptInfo{}); got != "(sandboxed) " {
				t.Errorf("the provider draws %q, want %q", got, "(sandboxed) ")
			}
		})
	}
}
