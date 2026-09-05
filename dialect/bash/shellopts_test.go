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
	r := &interp.Runner{Semantics: &sem, Diagnostics: &diag, Name: "bash"}
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
		Env: []string{"SHELLOPTS=nosuchoption:nounset"},
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
