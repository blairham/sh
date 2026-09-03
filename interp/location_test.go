// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

func TestLocationStylesAreMeasured(t *testing.T) {
	// All four shells prefix a diagnostic differently, and one of them names
	// the line only once there is one worth naming — ksh93 writes `ksh: msg`
	// on line 1 and `ksh: line 2: msg` after it.
	for _, tc := range []struct {
		name string
		diag Diagnostics
		want string
	}{
		{"dash", dash.Diagnostics(), "mysh: 7: boom"},
		{"bash", bash.Diagnostics(), "mysh: line 7: boom"},
		{"ksh93", ksh.Diagnostics(), "mysh: line 7: boom"},
		{"zsh", zsh.Diagnostics(), "mysh:7: boom"},
		// The zero value is the substrate's own: its name and nothing else.
		{"core", Diagnostics{}, "mysh: boom"},
	} {
		if got := tc.diag.Report("mysh", 7, "boom"); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	// And ksh93 on line 1, which is the case that hid the rule: measuring
	// only `sh -c 'one-liner'` cannot tell it from naming no line at all.
	if got := ksh.Diagnostics().Report("mysh", 1, "boom"); got != "mysh: boom" {
		t.Errorf("ksh93 line 1: %q, want %q", got, "mysh: boom")
	}
	// An empty name still produces something usable.
	if got := (Diagnostics{}).Report("", 1, "boom"); got != "sh: boom" {
		t.Errorf("empty name: %q", got)
	}
}

// TestDiagnosticsCarryTheFailingLine is the reason the Runner tracks a line
// at all: shells report where it went wrong, not where the script began.
func TestDiagnosticsCarryTheFailingLine(t *testing.T) {
	src := "echo one\necho two\nshift 5\n"
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem := dash.Semantics()
	diag := dash.Diagnostics()
	r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag, Name: "mysh"}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	// `shift` is on line 3, and dash names the line.
	if !strings.Contains(buf.String(), "mysh: 3: shift:") {
		t.Errorf("diagnostic did not carry line 3: %q", buf.String())
	}
}

// TestEveryDiagnosticGoesThroughTheDialect guards the rule rather than one
// message: a call that spells its own prefix bypasses the vector, and the
// bypass is invisible until someone measures a dialect that formats
// differently.
func TestEveryDiagnosticGoesThroughTheDialect(t *testing.T) {
	src := "shift 5"
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		diag   Diagnostics
		prefix string
	}{
		{"dash", dash.Diagnostics(), "mysh: 1: "},
		// zsh names the builtin between its own name and the line, so a
		// diagnostic from `shift` reads `mysh:shift:1:`. That is the shape
		// the real shell prints, and this expectation was `mysh:1: ` until
		// it was measured.
		{"zsh", zsh.Diagnostics(), "mysh:shift:1: "},
	} {
		var buf bytes.Buffer
		sem := dash.Semantics() // fatal shift, so there is a diagnostic
		diag := tc.diag
		r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag, Name: "mysh"}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(buf.String(), tc.prefix) {
			t.Errorf("%s: %q does not start with %q", tc.name, buf.String(), tc.prefix)
		}
	}
}
