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

// The two spellings of the escape character are two axes, because the two
// shells that split them split them in opposite directions: ksh93 has `\E`
// and not `\e`, zsh has `\e` and not `\E`. One answer for both letters is
// wrong for half the panel (#908).
func TestEchoEscAndCapitalEscAreTwoQuestions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		esc, capEs Answer
		want       string
	}{
		{"bash has both", Yes, Yes, "a\x1bZ:a\x1bZ\n"},
		{"dash and bash 3.2 have neither", No, No, "a\\eZ:a\\EZ\n"},
		{"ksh93 has the capital alone", No, Yes, "a\\eZ:a\x1bZ\n"},
		{"zsh has the small alone", Yes, No, "a\x1bZ:a\\EZ\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			sem.EchoOptions = "neE"
			sem.EchoExpandsEscEscape = tc.esc
			sem.EchoExpandsCapitalEscEscape = tc.capEs
			out, st := run(t, `echo -e 'a\eZ:a\EZ'`, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// Each letter is asked only where its own spelling appears, so a `\E` alone
// needs no answer about `\e` and the other way round.
func TestEchoEscLettersAreAskedSeparately(t *testing.T) {
	for _, tc := range []struct {
		name       string
		esc, capEs Answer
		src, want  string
	}{
		{
			"a lowercase escape asks nothing about the capital",
			Yes, Unspecified, `echo -e 'a\eZ'`, "a\x1bZ\n",
		},
		{
			"a capital escape asks nothing about the lowercase",
			Unspecified, Yes, `echo -e 'a\EZ'`, "a\x1bZ\n",
		},
		{
			"neither letter asks either",
			Unspecified, Unspecified, `echo -e 'a\tZ'`, "a\tZ\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			sem.EchoOptions = "neE"
			sem.EchoExpandsEscEscape = tc.esc
			sem.EchoExpandsCapitalEscEscape = tc.capEs
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// An unanswered letter is refused by name rather than guessed.
func TestEchoEscUnansweredIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, src, axis, rest string }{
		{"the lowercase", `echo -e 'a\eZ'`, "echo expanding \\e", `a\eZ`},
		{"the capital", `echo -e 'a\EZ'`, "echo expanding \\E", `a\EZ`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			sem.EchoOptions = "neE"
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			// The whole rendered output: the refusal names the letter it
			// could not answer for, and the escape stands as written after
			// it rather than being guessed either way.
			want := "sh: " + tc.axis + ": the shells disagree here and no dialect was chosen\n" +
				tc.rest + "\n"
			if out != want || st != 2 {
				t.Errorf("got %q status %d, want %q and 2", out, st, want)
			}
		})
	}
}
