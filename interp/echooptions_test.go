// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// echo's flags are a per-dialect letter set, the last-flag question is asked
// only on the `-e -E` order, and the escape table has two per-dialect
// extensions. These name the vector fields and never a shell.

func echoRun(t *testing.T, src string, set func(*Semantics)) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	if set != nil {
		set(&sem)
	}
	var out bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Semantics: &sem, Name: "testsh"})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String()
}

func TestEchoFlagLettersAreTheDialects(t *testing.T) {
	full := func(s *Semantics) {
		s.EchoOptions = "neE"
		s.EchoLastEscapeFlagWins = Yes
		s.EchoExpandsHexEscapes = Yes
		s.EchoExpandsEscEscape = Yes
	}
	bare := func(s *Semantics) { s.EchoOptions = "n"; s.EchoInterpretsEscapes = Yes }

	for _, tc := range []struct {
		name, src, want string
		set             func(*Semantics)
	}{
		{"-e turns escapes on", `echo -e 'a\tb'`, "a\tb\n", full},
		{"-E turns them off", `echo -E 'c\td'`, `c\td` + "\n", full},
		{"a cluster splits", `echo -ne 'x\n'`, "x\n", full},
		{"an unknown letter makes the word an operand", `echo -nq hi`, "-nq hi\n", full},
		{"a letter outside the set makes the word an operand", `echo -e 'a\tb'`, `-e a` + "\tb\n", bare},
		{"hex expands where admitted", `echo -e 'A\x41B'`, "AAB\n", full},
		{"esc expands where admitted", `echo -e 'e\eE'`, "e\x1bE\n", full},
		{"backslash-c stops the output", `echo -e 'p\cq'; echo done`, "pdone\n", full},
		{"octal is the XSI set", `echo -e 'A\0102C'`, "ABC\n", full},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := echoRun(t, tc.src, tc.set); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

func TestEchoHexStaysLiteralWhereNotAdmitted(t *testing.T) {
	got := echoRun(t, `echo -e 'A\x41B'`, func(s *Semantics) {
		s.EchoOptions = "ne"
		s.EchoExpandsHexEscapes = No
	})
	if got != `A\x41B`+"\n" {
		t.Errorf("got %q, want the hex escape kept as written", got)
	}
}

func TestTheOrderOfEAndCapitalEIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"the last flag wins", Yes, `m\tn` + "\n"},
		{"-e wins whatever the order", No, "m\tn\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := echoRun(t, `echo -e -E 'm\tn'`, func(s *Semantics) {
				s.EchoOptions = "neE"
				s.EchoLastEscapeFlagWins = tc.answer
			})
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
	// The other order agrees everywhere, so it asks nothing: an unanswered
	// axis would fail loudly here if it were consulted.
	got := echoRun(t, `echo -E -e 'x\ty'`, func(s *Semantics) { s.EchoOptions = "neE" })
	if got != "x\ty\n" {
		t.Errorf("-E -e = %q, want the expansion without a question", got)
	}
}
