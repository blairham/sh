// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// arithSentence is what this dialect says about an expression it refused. The
// refusal is found while parsing, so there is no run to observe it from.
func arithSentence(t *testing.T, expr string) string {
	t.Helper()
	_, err := syntax.Parse(`echo "$((`+expr+`))"`, ksh.Dialect())
	if err == nil {
		t.Fatalf("$((%s)) parsed, want a failure", expr)
	}
	d := ksh.Diagnostics()
	return d.ParseFailure(err)
}

// This shell words a missing operand two ways, and which one it says turns on
// whether the expression ran out or found something it could not use.
// Measured against ksh93u+ 2012-08-01 (2026-09-05).
func TestAMissingOperandIsWordedTwoWays(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"1+", "1+: more tokens expected"},
		{"~", "~: more tokens expected"},
		{"1**", "1**: more tokens expected"},
		{"%", "%: arithmetic syntax error"},
		{"@", "@: arithmetic syntax error"},
		{"1+&2", "1+&2: arithmetic syntax error"},
	} {
		if got := arithSentence(t, tc.src); got != tc.want {
			t.Errorf("$((%s)): got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A subscript is an expression, so `unset a[@]` reaches the found wording:
// `@` is text this shell's brackets cannot read, not a range that ran out.
func TestABadSubscriptGetsTheFoundWording(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(), `a=(p q r); unset "a[@]"; echo "n=${#a[@]}"`)
	want := "unset: @: arithmetic syntax error"
	if !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
}

// A substring's offset is blamed together with the rest of the range here,
// and the same shell reads the rest of the range — so `${x:1+:2}` found a
// token where `${x:1+}` ran out. The two namings are one fact, which is why
// one answer decides both.
func TestASubstringRangeIsReadAsFarAsItIsBlamed(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"${x:1+}", "1+: more tokens expected"},
		{"${x:2:1+}", "1+: more tokens expected"},
		{"${x:1+:2}", "1+:2: arithmetic syntax error"},
		{"${x:2:%}", "%: arithmetic syntax error"},
	} {
		out, _ := runKsh(t, t.TempDir(), `x=abcdef; echo "`+tc.src+`"`)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: got %q, want it to contain %q", tc.src, out, tc.want)
		}
	}
}
