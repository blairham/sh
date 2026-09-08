// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `${x?word}` expands to the value when the parameter is set and ends the
// script when it is not. It did neither: the operator had no case at all, so
// it fell through to the empty string and never reported anything.
//
// Which turns the standard way a script says "this variable is required" into
// a quiet wrong answer, and is what Homebrew's `brew` failed on.
func TestParameterErrorExpandsOrEndsTheScript(t *testing.T) {
	for _, c := range []struct {
		name, src, want, gone string
	}{
		{"set, so it expands", "V=x\necho \"[${V?}]\"\necho after\n", "[x]", "parameter"},
		{"and with a word too", "V=x\necho \"[${V?why}]\"\n", "[x]", "why"},
		{"the colon form as well", "V=x\necho \"[${V:?}]\"\n", "[x]", "parameter"},
		{
			// Unset: reported, and the script stops rather than carrying on
			// with an empty string.
			"unset, so it stops", "unset V\necho \"[${V?}]\"\necho after\n",
			"V: parameter not set", "after",
		},
		{"the word given is what it says", "unset V\necho \"${V?why not}\"\n", "V: why not", ""},
		{
			// The colon extends the test to a parameter that is there and
			// empty, which is the only difference between the two forms.
			"there but empty, with the colon", "V=\necho \"[${V:?}]\"\necho after\n",
			"V: parameter not set", "after",
		},
		{"there but empty, without it", "V=\necho \"[${V?}]\"\necho after\n", "[]", "parameter"},
		{"a positional too", "echo \"[${1?}]\"\necho after\n", "1: parameter not set", "after"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := paramErrRun(t, c.src, Diagnostics{})
			if !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q in it", out, c.want)
			}
			if c.gone != "" && strings.Contains(out, c.gone) {
				t.Errorf("said %q, want %q not in it", out, c.gone)
			}
		})
	}
}

// The default word for the colon form covers two cases at once, and each
// shell says so differently — including one that tells a parameter that is
// there and empty from one that is absent.
func TestTheDefaultWordForTheColonForm(t *testing.T) {
	for _, c := range []struct {
		name, src string
		dg        Diagnostics
		want      string
	}{
		{
			"a shell with one phrase for both", "unset V\necho \"${V:?}\"\n",
			Diagnostics{ParamNullOrNotSet: "parameter null or not set"},
			"V: parameter null or not set",
		},
		{
			"the same phrase when it is there and empty", "V=\necho \"${V:?}\"\n",
			Diagnostics{ParamNullOrNotSet: "parameter null or not set"},
			"V: parameter null or not set",
		},
		{
			// One dialect keeps a word for null alone, and still says "not
			// set" for a parameter that is absent.
			"a shell that tells them apart, null", "V=\necho \"${V:?}\"\n",
			Diagnostics{ParamNull: "parameter null"},
			"V: parameter null",
		},
		{
			"a shell that tells them apart, unset", "unset V\necho \"${V:?}\"\n",
			Diagnostics{ParamNull: "parameter null"},
			"V: parameter not set",
		},
		{
			"and one with neither says not set for both", "V=\necho \"${V:?}\"\n",
			Diagnostics{},
			"V: parameter not set",
		},
		{
			// A word given always wins over any of the defaults.
			"a word given beats them all", "V=\necho \"${V:?mine}\"\n",
			Diagnostics{ParamNullOrNotSet: "phrase", ParamNull: "other"},
			"V: mine",
		},
		{
			// The plain form's default is unanimous, so no dialect answer
			// applies to it.
			"the plain form ignores them", "unset V\necho \"${V?}\"\n",
			Diagnostics{ParamNullOrNotSet: "phrase", ParamNull: "other"},
			"V: parameter not set",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out := paramErrRun(t, c.src, c.dg); !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q in it", out, c.want)
			}
		})
	}
}

// It ends the script the same way an unset parameter under `set -u` does,
// which is the other door to the same room.
func TestTheTwoDoorsToAnUnexpandableParameter(t *testing.T) {
	for _, src := range []string{
		"unset V\necho \"${V?}\"\necho after\n",
		"set -u\nunset V\necho \"$V\"\necho after\n",
	} {
		out, st := paramErr(t, src, Diagnostics{})
		if strings.Contains(out, "after") {
			t.Errorf("%q said %q, want the script stopped", src, out)
		}
		if st == 0 {
			t.Errorf("%q reported success", src)
		}
	}
}

// The word `?` complains with names the word, not the files it would have
// matched.
//
// A diagnostic's text is not a pattern in any of them: with `a.b` present,
// `unset u; echo ${u?a.[a-c]}` says `u: a.[a-c]` on zsh 5.9.2, bash 5.3.15,
// dash and ksh93 alike, measured 2026-09-08. We said `u: a.b` — the operand
// went through the same matching entry point the substituting operators used,
// so the error a script raises to say what is missing named a file instead
// (#1500).
//
// Its own runner rather than a row in the table above, because the assertion
// only means anything in a directory the test owns: paramErr runs where the
// test process happens to be, and a pattern matched there would find whatever
// is in the package directory.
//
// The expansion is written **unquoted**, and that is the whole of what makes
// the case discriminating. Quoted, the operand never reached the match even
// with the fault in place, so `echo "${u?a.[a-c]}"` named the word before
// this change and after it — a probe that cannot tell the two apart.
func TestTheWordAnErrorComplainsWithIsNotAPattern(t *testing.T) {
	dir := globDir(t)
	var buf strings.Builder
	sem := PosixSemantics()
	sem.FatalErrorStatusIsOne = Yes
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh", Dir: dir,
	})
	f, err := syntax.Parse("unset u\necho ${u?a.[a-c]}\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "u: a.[a-c]") {
		t.Errorf("said %q, want %q in it", got, "u: a.[a-c]")
	}
}

func paramErrRun(t *testing.T, src string, dg Diagnostics) string {
	t.Helper()
	out, _ := paramErr(t, src, dg)
	return out
}

func paramErr(t *testing.T, src string, dg Diagnostics) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.FatalErrorStatusIsOne = Yes
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh"})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}
