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

// runPow evaluates src in a dialect that has `**`, with the float flag and
// the negative-exponent axis given. It names the flag and the axis rather
// than a shell, as this package's rule requires.
func runPow(t *testing.T, src string, float bool, negIsError Answer) (string, int) {
	t.Helper()
	d := syntax.Core()
	d.ArithFloat = float
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	s := PosixSemantics()
	s.ArithNegativeExponentIsError = negIsError
	r := &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return strings.TrimSpace(out.String()), st
}

// The unanimous rows: every shell that parses `**` answers these the same
// way, so no axis is consulted.
func TestExponentEvaluates(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo $((2**10))`, "1024"},
		{`echo $((2**0))`, "1"},
		{`echo $((0**0))`, "1"},
		{`echo $((2**3**2))`, "512"},
		{`echo $((-2**2))`, "4"},
		{`echo $((2*3**2))`, "18"},
		{`echo $((1<<2**2))`, "16"},
		{`echo $((2**62))`, "4611686018427387904"},
		// The result feeds an assignment like any other value.
		{`echo $((y=2**3))`, "8"},
		{`x=3; echo $((x**2))`, "9"},
	} {
		if got, _ := runPow(t, tc.src, false, Unspecified); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A negative exponent has no integer answer, and the axis decides between
// refusing and going float: `2**-1` is an error on one side and 0.5 on the
// other.
func TestNegativeExponentIsTheAxis(t *testing.T) {
	// The status a failed expression carries is a separate diagnostics
	// field; the axis owns only whether the expression fails at all.
	out, status := runPow(t, `echo $((2**-1))`, false, Yes)
	if status == 0 || !strings.Contains(out, "exponent less than 0") {
		t.Errorf("refusing side: got %q with status %d, want the refusal and a failure status", out, status)
	}
	for _, tc := range []struct{ src, want string }{
		{`echo $((2**-1))`, "0.5"},
		{`echo $((2**~1))`, "0.25"},
	} {
		if got, _ := runPow(t, tc.src, true, No); got != tc.want {
			t.Errorf("float side: %s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// Where the dialect has floats, `**` is a float operation like the other
// arithmetic operators — no integer-only refusal arises.
func TestExponentWithFloats(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo $((9**0.5))`, "3"},
		{`echo $((2.5**2))`, "6.25"},
		{`echo $((2**2.0))`, "4"},
	} {
		if got, _ := runPow(t, tc.src, true, No); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}
