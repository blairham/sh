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
// terminal.
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
			if _, ok := sh.PromptProviders[0].(sandboxMarker); !ok {
				t.Errorf("the provider is %T, want sandboxMarker", sh.PromptProviders[0])
			}
		})
	}
}

// What the mark draws: before a new command and not in the middle of an
// unfinished one.
func TestTheSandboxMarkDrawsOncePerCommand(t *testing.T) {
	if got := (sandboxMarker{}).Prompt(repl.PromptInfo{}); got != "(sandboxed) " {
		t.Errorf("the mark is %q, want %q", got, "(sandboxed) ")
	}
	if got := (sandboxMarker{}).Prompt(repl.PromptInfo{Continued: true}); got != "" {
		t.Errorf("the continuation prompt is marked %q, want nothing", got)
	}
}
