// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

func TestLocationStylesAreMeasured(t *testing.T) {
	// Each LocationStyle is one measured way of prefixing a diagnostic, and
	// one of them names the line only once there is one worth naming. Which
	// preset uses which style is asserted in the dialect packages.
	for _, tc := range []struct {
		style LocationStyle
		want  string
	}{
		{LocationColonLine, "mysh: 7: boom"},
		{LocationLineWord, "mysh: line 7: boom"},
		{LocationLineWordAfterFirst, "mysh: line 7: boom"},
		{LocationTightLine, "mysh:7: boom"},
		// The zero value is the substrate's own: its name and nothing else.
		{LocationNone, "mysh: boom"},
	} {
		d := Diagnostics{Location: tc.style}
		if got := d.Report("mysh", 7, "boom"); got != tc.want {
			t.Errorf("%v: %q, want %q", tc.style, got, tc.want)
		}
	}
	// The after-first style on line 1, which is the case that hid the rule:
	// measuring only `sh -c 'one-liner'` cannot tell it from naming no line
	// at all.
	d := Diagnostics{Location: LocationLineWordAfterFirst}
	if got := d.Report("mysh", 1, "boom"); got != "mysh: boom" {
		t.Errorf("after-first on line 1: %q, want %q", got, "mysh: boom")
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
	sem := PosixSemantics() // fatal shift, so there is a diagnostic
	diag := Diagnostics{Location: LocationColonLine}
	r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag, Name: "mysh"}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	// `shift` is on line 3, and the style names the line.
	if !strings.Contains(buf.String(), "mysh: 3: ") {
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
		{"colon-line", Diagnostics{Location: LocationColonLine}, "mysh: 1: "},
		// NamesBuiltinInLocation puts the reporting builtin between the
		// shell's name and the line, so a diagnostic from `shift` reads
		// `mysh:shift:1:`. That is the shape one real shell prints, and its
		// expectation was `mysh:1: ` until it was measured.
		{"builtin named", Diagnostics{Location: LocationTightLine, NamesBuiltinInLocation: true}, "mysh:shift:1: "},
	} {
		var buf bytes.Buffer
		sem := PosixSemantics() // fatal shift, so there is a diagnostic
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
