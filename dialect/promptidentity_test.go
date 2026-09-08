// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"os"
	"os/user"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A dialect whose prompt language can draw the user must tell the runner who
// that is, and every one that can must answer the same thing.
//
// An invariant across the dialects rather than a test per dialect, because the
// fault it exists for is a *missing* caller and no test of the callers that
// exist can see one. #1446 was exactly that: `%n` was answered and `\u` was
// not, so the drawer fell back to a lookup of its own that read `$USER` — and
// with no `USER` in the environment, which `env -i`, `sudo -i`, a container
// and a cron job all give you, the escape drew nothing at all. Every other
// escape in the default `\u@\h:\w\$ ` was byte-identical to bash 5.3.15, which
// is what made the hole look like a prompt.
//
// Driven off each dialect's own PromptStyle table so that it is the *escape*
// that obliges the dialect: a shell added later with a user code in its table
// and no caller in its Apply fails here on the day it is written, and one with
// no such code — ksh93 drops the backslash and draws `u`, dash has no escape
// language at all — is not asked to install a question it can never ask.
func TestEveryDialectThatCanDrawTheUserWasToldWho(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skipf("no user for this process: %v", err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Skipf("no host name for this machine: %v", err)
	}
	for _, d := range []struct {
		name    string
		style   interp.PromptStyle
		grammar syntax.Dialect
		sem     interp.Semantics
		apply   func(*interp.Runner)
	}{
		{"bash", bash.PromptStyle(), bash.Dialect(), bash.Semantics(), bash.Apply},
		{"dash", dash.PromptStyle(), dash.Dialect(), dash.Semantics(), dash.Apply},
		{"ksh", ksh.PromptStyle(), ksh.Dialect(), ksh.Semantics(), ksh.Apply},
		{"zsh", zsh.PromptStyle(), zsh.Dialect(), zsh.Semantics(), zsh.Apply},
	} {
		t.Run(d.name, func(t *testing.T) {
			for _, want := range []struct {
				field interp.PromptField
				name  string
				value string
			}{
				{interp.FieldUser, "user", u.Username},
				{interp.FieldHost, "host", firstLabel(host)},
				{interp.FieldHostFull, "full host", host},
			} {
				if !draws(d.style, want.field) {
					continue
				}
				sem, grammar := d.sem, d.grammar
				r := &interp.Runner{Semantics: &sem, Dialect: &grammar}
				d.apply(r)
				got, ok := r.PromptField(want.field, "", false)
				if !ok || got != want.value {
					t.Errorf("the %s code answered %q (ok=%v), want %q — the dialect draws it and did not say who",
						want.name, got, ok, want.value)
				}
			}
		})
	}
}

// draws reports whether this prompt language has a code for a field at all.
func draws(style interp.PromptStyle, f interp.PromptField) bool {
	for _, got := range style.Codes {
		if got == f {
			return true
		}
	}
	return false
}

// firstLabel is the host name up to its first dot, which is what the short
// host code draws.
func firstLabel(host string) string {
	for i := range len(host) {
		if host[i] == '.' {
			return host[:i]
		}
	}
	return host
}
