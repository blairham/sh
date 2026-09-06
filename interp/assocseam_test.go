// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Three things a registered builtin needs and could not reach: a table of
// *named* values, the same read back, and the value of an expression it did
// not parse. Each is here because a dialect builtin found it missing —
// `zparseopts` writes an association and `zformat` compares an expression
// with a number — and each is about something this package owns: the
// associative attribute decides how a subscript is read, and the arithmetic
// is the shell's own.

func seamRunner(t *testing.T, out, errs *strings.Builder) *Runner {
	t.Helper()
	dg := Diagnostics{Location: LocationTightLine, BuiltinLocation: LocationTightLine}
	// One axis answered, because one of these tests reaches a *fatal* error
	// and the status a fatal error carries is a thing the shells disagree
	// about — an unanswered axis reports itself, which would be a second
	// diagnostic on a test asserting on the first.
	sem := Semantics{FatalErrorStatusIsOne: Yes}
	return newTestRunner(t, &Runner{
		Stdout: out, Stderr: errs, Diagnostics: &dg, Semantics: &sem, Name: "testsh",
	})
}

func runSeam(t *testing.T, r *Runner, src string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
}

// An **empty** table still declares the name, which is the case a "nothing to
// write, so write nothing" shortcut gets wrong. An association that exists and
// is empty and one that was never created are different things — the second is
// not an array at all — and a builtin whose parse matched nothing has to leave
// the first.
func TestSetAssocDeclaresTheNameEvenWithNothingInIt(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("fill", func(rr *Runner, _ context.Context, args []string) int {
		rr.SetAssoc(args[0], nil)
		return 0
	})
	runSeam(t, r, "fill empty\n")
	got, ok := r.GetAssoc("empty")
	if !ok {
		t.Fatal("GetAssoc(empty) = not there, want a declared empty association")
	}
	if len(got) != 0 {
		t.Errorf("GetAssoc(empty) = %v, want no elements", got)
	}
	if _, ok := r.GetAssoc("never"); ok {
		t.Error("GetAssoc(never) = there, want not there")
	}
}

// The subscript's meaning is what the attribute decides, so a table written
// through this seam has to read back by *key* and not by index: `m[1+1]` is
// the three characters here and would be the element at 2 in an indexed array.
// This is the assertion that a seam reaching into the map directly would fail.
func TestSetAssocGivesTheNameTheAssociativeAttribute(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("fill", func(rr *Runner, _ context.Context, _ []string) int {
		rr.SetAssoc("m", map[string]string{"1+1": "key", "2": "index"})
		return 0
	})
	runSeam(t, r, "fill\nprintf '[%s]' \"${m[1+1]}\"\n")
	if out.String() != "[key]" {
		t.Errorf("${m[1+1]} = %q, want %q", out.String(), "[key]")
	}
}

// GetAssoc hands back a copy. A caller that reads a table, adds to it and
// writes it back — which is exactly what a builtin preserving existing
// elements does — must not change the shell's state halfway through deciding
// what to write, because a failure after that point would leave half a write
// behind.
func TestGetAssocIsACopyAndNotTheShellsOwnTable(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("meddle", func(rr *Runner, _ context.Context, _ []string) int {
		table, _ := rr.GetAssoc("m")
		table["added"] = "no"
		delete(table, "kept")
		return 0
	})
	runSeam(t, r, "typeset -A m\nm[kept]=yes\nmeddle\nprintf '[%s][%s]' \"${m[kept]}\" \"${m[added]}\"\n")
	if out.String() != "[yes][]" {
		t.Errorf("the shell's table after meddling = %q, want %q", out.String(), "[yes][]")
	}
}

// ArithValue is the shell's own arithmetic and not a number parse: `1+1` is 2
// and a bare name is the variable's value, an unset one being zero.
func TestArithValueIsTheShellsOwnArithmetic(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("value", func(rr *Runner, _ context.Context, args []string) int {
		n, ok := rr.ArithValue(args[0])
		_, _ = out.WriteString(strconv.Itoa(n) + ":" + boolText(ok) + " ")
		return 0
	})
	runSeam(t, r, "x=7\nvalue 1+1\nvalue x\nvalue nosuchname\nvalue '(1+2)*2'\n")
	want := "2:ok 7:ok 0:ok 6:ok "
	if out.String() != want {
		t.Errorf("ArithValue = %q, want %q", out.String(), want)
	}
}

// And an expression that will not *evaluate* is not a zero. It is reported the
// way `$(( ))` reports one — located as the shell rather than as the builtin,
// because the expression failed and not the command holding it — and it stops
// the script, so nothing after it runs and the caller is told before it writes
// anything.
func TestArithValueReportsAndStopsWhenAnExpressionWillNotEvaluate(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("value", func(rr *Runner, _ context.Context, args []string) int {
		if _, ok := rr.ArithValue(args[0]); !ok {
			_, _ = out.WriteString("refused ")
			return 1
		}
		_, _ = out.WriteString("took ")
		return 0
	})
	runSeam(t, r, "value 1/0\nvalue 1+1\n")
	if out.String() != "refused " {
		t.Errorf("what ran = %q, want %q — the second line must not run", out.String(), "refused ")
	}
	if errs.String() != "testsh:1: division by zero\n" {
		t.Errorf("diagnostic = %q, want %q", errs.String(), "testsh:1: division by zero\n")
	}
}

func boolText(b bool) string {
	if b {
		return "ok"
	}
	return "no"
}
