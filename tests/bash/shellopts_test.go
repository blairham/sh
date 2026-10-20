// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// The two startup inputs this dialect has and the others do not, both measured
// on bash 5.3.15 and the 3.2 macOS ships, 2026-09-05, with a scratch HOME.

// TestBashNamesTheOptionRecord. The name is the dialect's and the record is the
// core's; without this line the variable does not exist here at all.
func TestBashNamesTheOptionRecord(t *testing.T) {
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{Semantics: &sem, Diagnostics: &diag, Name: "bash", Dialect: presetDialect()}
	bash.Apply(r)
	value, ok := r.GetVar("SHELLOPTS")
	if !ok {
		t.Fatal("SHELLOPTS is unset, want the record under bash's own name for it")
	}
	if !strings.Contains(":"+value+":", ":braceexpand:") {
		t.Errorf("SHELLOPTS = %q, want the options this shell has on by default", value)
	}
}

// TestBashRefusesAnUnknownNameFromTheEnvironmentInItsOwnWords.
//
// The third of three shapes for the same refusal, and it is the plainest:
// measured, `SHELLOPTS=nosuchoption bash -c …` says
//
//	bash: line 0: nosuchoption: invalid option name
//
// where the same bad name written as `bash -o nosuchoption` names the shell a
// second time where `set` would stand, and a script's own `set -o` names `set`
// there. A run of names is applied left to right and one bad entry costs only
// itself.
func TestBashRefusesAnUnknownNameFromTheEnvironmentInItsOwnWords(t *testing.T) {
	var errs strings.Builder
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Stdout: &strings.Builder{}, Stderr: &errs,
		Semantics: &sem, Diagnostics: &diag, Name: "bash",
		Env:     []string{"SHELLOPTS=nosuchoption:nounset"},
		Dialect: presetDialect(),
	}
	bash.Apply(r)
	r.ApplyInheritedShellOptions()
	if want := "bash: line 0: nosuchoption: invalid option name\n"; errs.String() != want {
		t.Errorf("said %q, want %q", errs.String(), want)
	}
	if value, _ := r.GetVar("SHELLOPTS"); !strings.Contains(":"+value+":", ":nounset:") {
		t.Errorf("SHELLOPTS = %q, want the good name in the same value still applied", value)
	}
}

// TestBashNamesTheNonInteractiveStartupVariable. The file a bash that is not
// going to prompt reads, and the name is all the dialect supplies — the front
// end does the reading, and does it on the script routes only.
func TestBashNamesTheNonInteractiveStartupVariable(t *testing.T) {
	if got := bash.Semantics().NonInteractiveStartupVariable; got != "BASH_ENV" {
		t.Errorf("NonInteractiveStartupVariable = %q, want BASH_ENV", got)
	}
	for _, other := range []struct {
		name string
		sem  interp.Semantics
	}{
		{"the standard's own preset", interp.PosixSemantics()},
		{"the substrate's", interp.CoreSemantics()},
	} {
		if got := other.sem.NonInteractiveStartupVariable; got != "" {
			t.Errorf("%s names %q, want no such file — this is one shell's feature", other.name, got)
		}
	}
}

// And the same binding one namespace along: `$BASHOPTS` is to `shopt` what
// `$SHELLOPTS` is to `set -o`, and it is bound in both directions (#2475).
func TestBashNamesTheShoptOptionRecord(t *testing.T) {
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{Semantics: &sem, Diagnostics: &diag, Name: "bash", Dialect: presetDialect()}
	bash.Apply(r)
	value, ok := r.GetVar("BASHOPTS")
	if !ok {
		t.Fatal("BASHOPTS is unset, want the shopt names this shell has on")
	}
	if !strings.Contains(":"+value+":", ":sourcepath:") {
		t.Errorf("BASHOPTS = %q, want the shopt names on by default", value)
	}
	if strings.Contains(":"+value+":", ":cdspell:") {
		t.Errorf("BASHOPTS = %q, want a name that is off left out of it", value)
	}
}

// Produced rather than stored, which is the whole design: a copy taken at
// startup would answer the same before and after.
func TestTheShoptOptionRecordFollowsTheBuiltin(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a name turned on joins it",
			`shopt -s cdspell; case ":$BASHOPTS:" in *:cdspell:*) echo listed ;; *) echo gone ;; esac`,
			"listed\n",
		},
		{
			"and a name turned off leaves it",
			`shopt -u patsub_replacement; case ":$BASHOPTS:" in *:patsub_replacement:*) echo listed ;; *) echo gone ;; esac`,
			"gone\n",
		},
		{
			"while a default the script did not touch stays",
			`shopt -u patsub_replacement; case ":$BASHOPTS:" in *:promptvars:*) echo listed ;; *) echo gone ;; esac`,
			"listed\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The write direction, and the way it differs from `$SHELLOPTS`: a name this
// namespace does not know is skipped in silence rather than complained about.
// Measured on bash 5.3.15, 2026-09-13 — `BASHOPTS=nosuchopt:cdspell bash -c
// 'shopt -p cdspell'` answers `shopt -s cdspell` at 0 with nothing on standard
// error, where the same shape in `$SHELLOPTS` draws `invalid option name`.
func TestBashReadsTheShoptRecordOutOfTheEnvironment(t *testing.T) {
	var errs strings.Builder
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Stdout: &strings.Builder{}, Stderr: &errs,
		Semantics: &sem, Diagnostics: &diag, Name: "bash",
		Env:     []string{"BASHOPTS=nosuchopt::cdspell"},
		Dialect: presetDialect(),
	}
	bash.Apply(r)
	r.ApplyInheritedShellOptions()
	if errs.String() != "" {
		t.Errorf("said %q, want nothing about a name this namespace does not know", errs.String())
	}
	value, _ := r.GetVar("BASHOPTS")
	if !strings.Contains(":"+value+":", ":cdspell:") {
		t.Errorf("BASHOPTS = %q, want the good name in the same value applied", value)
	}
	// Nothing is turned off: the value says what is on, and a default the
	// value does not mention survives it.
	if !strings.Contains(":"+value+":", ":sourcepath:") {
		t.Errorf("BASHOPTS = %q, want a default the inherited value did not name left alone", value)
	}
}
