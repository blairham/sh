// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// A shell is asked its version two ways — the parameter its own scripts read
// and the `--version` its installers read — and it must not answer them
// differently. #1713 is the second of the two missing entirely: the parameter
// was right in every dialect and the option was an unknown option in every
// dialect, so anything probing a shell the way an installer does concluded it
// was broken.
//
// The invariant is across the dialects rather than per dialect, so that a
// shell added later either names no version option or has both halves. The
// token is what the two claims share: the tag that says whose shell this is,
// which is in the parameter and in the line, and the version itself.
func TestAVersionOptionAndTheVersionParameterMakeOneClaim(t *testing.T) {
	for _, d := range []struct {
		name  string
		shell driver.Shell
		// parameter is the name a script reads this shell's version from.
		parameter string
		// shared is what the parameter's value and the version line must
		// both carry: the version this dialect implements, and the tag.
		shared []string
	}{
		{
			name: "bash", parameter: "BASH_VERSION", shared: []string{"5.3.15", "blairham"},
			shell: driver.Shell{
				Name: "bash", Dialect: bash.Dialect(), Semantics: bash.Semantics(),
				Diagnostics: bash.Diagnostics(), Prelude: bash.Prelude(), Register: bash.Apply,
			},
		},
		{
			name: "zsh", parameter: "ZSH_VERSION", shared: []string{"5.9.2", "blairham"},
			shell: driver.Shell{
				Name: "zsh", Dialect: zsh.Dialect(), Semantics: zsh.Semantics(),
				Diagnostics: zsh.Diagnostics(), Prelude: zsh.Prelude(), Register: zsh.Apply,
			},
		},
		{
			name: "ksh", parameter: "KSH_VERSION", shared: []string{"93u+ 2026-09-02", "blairham"},
			shell: driver.Shell{
				Name: "ksh", Dialect: ksh.Dialect(), Semantics: ksh.Semantics(),
				Diagnostics: ksh.Diagnostics(), Prelude: ksh.Prelude(), Register: ksh.Apply,
			},
		},
	} {
		t.Run(d.name, func(t *testing.T) {
			v := d.shell.Semantics.VersionOption
			if v.Spellings == "" {
				t.Fatalf("names no version option, and this dialect's shell answers one")
			}
			var out, errs strings.Builder
			sh := d.shell
			sh.Stdout, sh.Stderr = &out, &errs
			if got := driver.MainArgs(sh, []string{d.name, "--version"}); got != v.Status {
				t.Errorf("status %d, want the dialect's %d", got, v.Status)
			}
			line := out.String()
			if v.ToStandardError {
				line = errs.String()
				if out.String() != "" {
					t.Errorf("standard output %q, want the answer on standard error alone", out.String())
				}
			}
			if strings.Count(line, "\n") != 1 {
				t.Errorf("wrote %q, want exactly one line", line)
			}
			var param, perr strings.Builder
			sh = d.shell
			sh.Stdout, sh.Stderr = &param, &perr
			if got := driver.MainArgs(sh, []string{d.name, "-c", "printf %s \"$" + d.parameter + "\""}); got != 0 {
				t.Fatalf("reading $%s exited %d (%q)", d.parameter, got, perr.String())
			}
			for _, want := range d.shared {
				if !strings.Contains(line, want) {
					t.Errorf("the version line %q does not carry %q", line, want)
				}
				if !strings.Contains(param.String(), want) {
					t.Errorf("$%s is %q and does not carry %q", d.parameter, param.String(), want)
				}
			}
		})
	}
}

// dash is the panel's holdout and the reason the zero value has to mean
// something: measured 2026-09-11, `dash --version` is `Illegal option --` at
// status 2. A dialect that names no spelling refuses the word, which is what
// the front end did for every dialect before #1713.
func TestADialectThatNamesNoVersionOptionRefusesTheWord(t *testing.T) {
	if got := dash.Semantics().VersionOption.Spellings; got != "" {
		t.Fatalf("names %q, want no version option: this shell refuses the word", got)
	}
	var out, errs strings.Builder
	sh := driver.Shell{
		Name: "dash", Dialect: dash.Dialect(), Semantics: dash.Semantics(),
		Diagnostics: dash.Diagnostics(), Prelude: dash.Prelude(), Register: dash.Apply,
		Stdout: &out, Stderr: &errs,
	}
	if got := driver.MainArgs(sh, []string{"dash", "--version"}); got == 0 {
		t.Errorf("exited 0, want the word refused")
	}
	if out.String() != "" {
		t.Errorf("wrote %q, want nothing on standard output", out.String())
	}
	if !strings.Contains(errs.String(), "--version") {
		t.Errorf("said %q, want the refused word named", errs.String())
	}
}

// The substrate's own vector names none either, which is what makes the
// binary's answer the binary's: interp holds no build's version. See
// cmd/sh's coreSemantics.
func TestTheCoreNamesNoVersionOption(t *testing.T) {
	for _, s := range []struct {
		name string
		sem  interp.Semantics
	}{
		{"core", interp.CoreSemantics()},
		{"posix", interp.PosixSemantics()},
		{"zero", interp.Semantics{}},
	} {
		if got := s.sem.VersionOption.Spellings; got != "" {
			t.Errorf("%s names %q, want no version option", s.name, got)
		}
	}
}
