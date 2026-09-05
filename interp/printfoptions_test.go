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

// printfRun keeps the two streams apart, because the claim is about what
// reaches *stdout*: a refusal naming `-v` is right, and printing `-v` as
// though it were the format is the bug.
func printfRun(t *testing.T, tweak func(*Semantics), src string) (out, errs string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	sem := permissive()
	sem.PrintfAssignsWithV = Yes
	sem.PrintfRejectsUnknownOption = Yes
	if tweak != nil {
		tweak(&sem)
	}
	dg := Diagnostics{PrintfBadOption: "printf: %[1]s: invalid option"}
	r := newTestRunner(t, &Runner{Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return o.String(), e.String(), st
}

// `printf -v name` collects the text instead of printing it. It did neither:
// `-v` was taken as the format and printed, and the variable stayed empty.
func TestPrintfAssignsWithV(t *testing.T) {
	out, _, st := printfRun(t, nil, `printf -v out "%05d" 42; echo "[$out]"`)
	if strings.TrimSpace(out) != "[00042]" {
		t.Errorf("got %q status %d, want the text assigned and nothing printed", out, st)
	}
	// Nothing reaches stdout, which is half the point.
	out, _, _ = printfRun(t, nil, `printf -v out "hello"`)
	if out != "" {
		t.Errorf("printed %q, want silence", out)
	}
	// The format is still reused over the operands.
	out, _, _ = printfRun(t, nil, `printf -v out "[%s]" a b c; echo "$out"`)
	if strings.TrimSpace(out) != "[a][b][c]" {
		t.Errorf("got %q, want the format reused", out)
	}
	// Where the dialect has no such option it is refused, not printed.
	out, errs, _ := printfRun(t, func(s *Semantics) { s.PrintfAssignsWithV = No },
		`printf -v out "%05d" 42; echo "[$out]"`)
	if !strings.Contains(errs, "invalid option") {
		t.Errorf("stderr %q, want a refusal", errs)
	}
	if strings.Contains(out, "00042") || strings.Contains(out, "-v") {
		t.Errorf("stdout %q, want nothing formatted and no option echoed", out)
	}
}

// `--` ends the options. Unanimous, and the reason a dash-initial format can
// be written at all.
func TestPrintfDoubleDashEndsTheOptions(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`printf -- "x\n"`, "x"},
		{`printf -- "-v\n"`, "-v"},
		{`printf -- "-%s\n" y`, "-y"},
		// After the format, a dash word is an ordinary argument.
		{`printf "%s\n" -v`, "-v"},
		{`printf "%s\n" --`, "--"},
		// A lone dash is an operand, not an option.
		{`printf "%s\n" -`, "-"},
	} {
		if out, _, _ := printfRun(t, nil, c.src); strings.TrimSpace(out) != c.want {
			t.Errorf("%s = %q, want %q", c.src, strings.TrimSpace(out), c.want)
		}
	}
}

// An unrecognized leading `-` word is an option in three of the four and the
// format in zsh — and where it is refused, it is refused by its first letter.
func TestPrintfUnknownOption(t *testing.T) {
	dg := Diagnostics{PrintfBadOption: "printf: %[1]s: invalid option"}
	f, err := syntax.Parse(`printf "-%s\n" x`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.PrintfAssignsWithV, sem.PrintfRejectsUnknownOption = Yes, Yes
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "-%: invalid option") {
		t.Errorf("said %q, want the first letter named", buf.String())
	}
	// Where they are not refused, the same word is the format.
	out, _, _ := printfRun(t, func(s *Semantics) { s.PrintfRejectsUnknownOption = No }, `printf "-%s\n" x`)
	if strings.TrimSpace(out) != "-x" {
		t.Errorf("got %q, want it taken as the format", out)
	}
	out, _, _ = printfRun(t, func(s *Semantics) { s.PrintfRejectsUnknownOption = No }, `printf -q x`)
	if strings.TrimSpace(out) != "-q" {
		t.Errorf("got %q, want it taken as the format", out)
	}
}

// Unanswered is refused once, and does not print the option as if it were a
// format on the way out.
func TestPrintfUnansweredOptionIsRefused(t *testing.T) {
	out, errs, _ := printfRun(t, func(s *Semantics) { s.PrintfAssignsWithV = Unspecified }, `printf -v o "x"`)
	if !strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("stderr %q, want a refusal", errs)
	}
	if out != "" {
		t.Errorf("stdout %q, want silence — not the option echoed as a format", out)
	}
}

// A lone `-` is an operand, not an option, in all four — which is what keeps
// the leading-dash rule from swallowing it. As a format it prints itself.
func TestPrintfALoneDashIsAFormat(t *testing.T) {
	out, errs, _ := printfRun(t, nil, `printf -`)
	if out != "-" {
		t.Errorf("stdout %q (stderr %q), want the dash printed as the format", out, errs)
	}
	// And with the option rule at its strictest, it is still not an option.
	out, _, _ = printfRun(t, func(s *Semantics) { s.PrintfRejectsUnknownOption = Yes }, `printf -`)
	if out != "-" {
		t.Errorf("stdout %q, want the dash printed", out)
	}
}
