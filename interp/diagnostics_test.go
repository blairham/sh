// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

func TestSyntaxErrorStatusIsADialectAnswer(t *testing.T) {
	// Measured across eight distinct syntax errors and stable within each
	// shell. It was hardcoded to 2 under a comment claiming every shell in
	// the panel agreed, which is true of half of them.
	for _, tc := range []struct {
		name string
		diag Diagnostics
		want int
	}{
		{"dash", dash.Diagnostics(), 2},
		{"bash", bash.Diagnostics(), 2},
		{"ksh93", ksh.Diagnostics(), 3},
		{"zsh", zsh.Diagnostics(), 1},
		// The substrate answers for itself rather than refusing: a status is
		// not a claim about another shell, and the process must exit with
		// some number.
		{"core", CoreDiagnostics(), 2},
		{"zero value", Diagnostics{}, 2},
	} {
		if got := tc.diag.SyntaxError(); got != tc.want {
			t.Errorf("%s: SyntaxError() = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestCommandSubstitutionCarriesTheDialectStatus covers the second place a
// script is parsed. A seam that only reached whatever read the file first
// would be wrong inside `$( )`, which re-parses.
func TestCommandSubstitutionCarriesTheDialectStatus(t *testing.T) {
	for _, tc := range []struct {
		name string
		diag Diagnostics
		want int
	}{
		{"ksh93", ksh.Diagnostics(), 3},
		{"zsh", zsh.Diagnostics(), 1},
		{"bash", bash.Diagnostics(), 2},
	} {
		f, err := syntax.Parse("x=$(if true); echo after", bash.Dialect())
		if err != nil {
			t.Fatalf("the outer script must parse: %v", err)
		}
		var buf bytes.Buffer
		sem := bash.Semantics()
		diag := tc.diag
		r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag}
		st, err := r.Run(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		// Fatal in all four shells: none of them reach the next command.
		if bytes.Contains(buf.Bytes(), []byte("after")) {
			t.Errorf("%s: the script continued past a parse error: %q", tc.name, buf.String())
		}
		if st != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, st, tc.want)
		}
	}
}
