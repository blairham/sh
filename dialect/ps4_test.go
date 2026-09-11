// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"os/user"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/driver"
)

// Every dialect reads `PS4`, and each reads it in *its own* prompt language —
// #1454, where the trace prefix was the constant `+ ` whatever the parameter
// held.
//
// One table across the dialects rather than a test per dialect, because the
// fault is a missing reader and no test of the readers that exist can see one.
// The `\u` row and the `%n` row are the pair that says this: a shell that
// learned one escape language and used it everywhere passes either row alone.
func TestEveryDialectDrawsItsTracePrefixFromPS4(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skipf("no user for this process: %v", err)
	}
	for _, d := range []struct {
		name  string
		shell func() driver.Shell
		// backslashUser is what `PS4='<\u>'` draws, and percentUser what
		// `PS4='<%n>'` draws. Measured 2026-09-11 against the panel.
		backslashUser, percentUser string
		// indirection is what `PS4='XY '; eval :` draws for the eval'd
		// command: one dialect repeats the first character.
		indirection string
	}{
		{
			name: "bash", shell: bashShell,
			backslashUser: "<" + u.Username + ">", percentUser: "<%n>",
			indirection: "XXY",
		},
		{
			name: "zsh", shell: zshShell,
			backslashUser: `<\u>`, percentUser: "<" + u.Username + ">",
			indirection: "XY",
		},
		{
			// This shell has no user escape and drops the backslash of a
			// code it does not know, so `\u` draws `u`.
			name: "ksh", shell: kshShell,
			backslashUser: "<u>", percentUser: "<%n>",
			indirection: "XY",
		},
		{
			// And this one has no escape language at all.
			name: "dash", shell: dashShell,
			backslashUser: `<\u>`, percentUser: "<%n>",
			indirection: "XY",
		},
	} {
		t.Run(d.name, func(t *testing.T) {
			// The parameter decides the prefix in every dialect, which is
			// the unanimous half.
			if got, want := traced(t, d.shell(), `PS4="XX "; set -x; :`), "XX :\n"; got != want {
				t.Errorf("PS4 traced %q, want %q", got, want)
			}
			if got, want := traced(t, d.shell(), `PS4='<\u> '; set -x; :`), d.backslashUser+" :\n"; got != want {
				t.Errorf(`PS4='<\u> ' traced %q, want %q`, got, want)
			}
			if got, want := traced(t, d.shell(), `PS4='<%n> '; set -x; :`), d.percentUser+" :\n"; got != want {
				t.Errorf(`PS4='<%%n> ' traced %q, want %q`, got, want)
			}
			got := traced(t, d.shell(), `PS4="XY "; set -x; eval :`)
			if want := "XY eval :\n" + d.indirection + " :\n"; got != want {
				t.Errorf("eval traced %q, want %q", got, want)
			}
		})
	}
}

// traced runs a snippet under one dialect's whole shell — prelude, builtins
// and prompt style — and returns what went to standard error.
//
// Through driver rather than through a Runner built here, because the prompt
// style is installed by the front end and a runner without one would be
// measuring a shell nobody ships.
func traced(t *testing.T, sh driver.Shell, src string) string {
	t.Helper()
	var out, errs strings.Builder
	sh.Stdout, sh.Stderr = &out, &errs
	driver.MainArgs(sh, []string{sh.Name, "-c", src})
	return errs.String()
}

func bashShell() driver.Shell {
	return driver.Shell{
		Name: "bash", Dialect: bash.Dialect(), Semantics: bash.Semantics(),
		Diagnostics: bash.Diagnostics(), Prelude: bash.Prelude(), Register: bash.Apply,
		PromptStyle: bash.PromptStyle(),
	}
}

func zshShell() driver.Shell {
	return driver.Shell{
		Name: "zsh", Dialect: zsh.Dialect(), Semantics: zsh.Semantics(),
		Diagnostics: zsh.Diagnostics(), Prelude: zsh.Prelude(), Register: zsh.Apply,
		PromptStyle: zsh.PromptStyle(),
	}
}

func kshShell() driver.Shell {
	return driver.Shell{
		Name: "ksh", Dialect: ksh.Dialect(), Semantics: ksh.Semantics(),
		Diagnostics: ksh.Diagnostics(), Prelude: ksh.Prelude(), Register: ksh.Apply,
		PromptStyle: ksh.PromptStyle(),
	}
}

func dashShell() driver.Shell {
	return driver.Shell{
		Name: "dash", Dialect: dash.Dialect(), Semantics: dash.Semantics(),
		Diagnostics: dash.Diagnostics(), Prelude: dash.Prelude(), Register: dash.Apply,
		PromptStyle: dash.PromptStyle(),
	}
}
