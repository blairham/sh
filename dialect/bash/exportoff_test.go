// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// bash is the only shell in the panel with `export -n`. Measured 2026-09-05
// on 5.3.15 and 3.2.57 alike: the name keeps its value in the shell and stops
// reaching a child, and `export` reports 0. dash and ksh93 refuse the letter
// fatally and zsh refuses it and carries on (#516).
func TestExportHasTheOffOption(t *testing.T) {
	if got := bash.Semantics().ExportTakesTheAttributeOff; got != interp.Yes {
		t.Errorf("ExportTakesTheAttributeOff = %v, want Yes", got)
	}
	f, err := syntax.Parse(`export -n V; echo "st=$? [$V]"`, bash.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var out, errs strings.Builder
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Stdout: &out, Stderr: &errs,
		Semantics: &sem, Diagnostics: &diag, Name: "bash",
		Env:     []string{"V=first"},
		Dialect: presetDialect(),
	}
	bash.Apply(r)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatal(rerr)
	}
	if got, want := out.String(), "st=0 [first]\n"; got != want {
		t.Errorf("got %q (stderr %q), want %q", got, errs.String(), want)
	}
}
