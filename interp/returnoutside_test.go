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

// A `return` with nothing to return from — neither a function nor a sourced
// file — splits the panel over *where the script stops*.
//
// Three of them obey it and end the script with the status given. bash
// reports it, leaves 2 behind and carries on. Found on a script whose last
// statement is that `return`: identical output, and a status of 2 against
// our 127, which only a comparison that reads the status could see.
func TestAReturnWithNothingToReturnFrom(t *testing.T) {
	const src = "echo before\nreturn 7\necho \"after st=$?\"\n"
	for _, c := range []struct {
		name    string
		refused Answer
		want    []string
		gone    string
		status  int
	}{
		{
			"refused, and the script carries on",
			Yes,
			[]string{"before", "can only `return'", "after st=2"},
			"",
			0,
		},
		{
			"obeyed, and the script ends there",
			No,
			[]string{"before"},
			"after",
			7,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := returnRun(t, src, c.refused)
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("said %q, want %q in it", out, w)
				}
			}
			if c.gone != "" && strings.Contains(out, c.gone) {
				t.Errorf("said %q, want %q not in it", out, c.gone)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// Inside a function there is something to return from, so the question is
// never put and both answers behave alike.
func TestAReturnInsideAFunctionIsAlwaysObeyed(t *testing.T) {
	for _, refused := range []Answer{Yes, No, Unspecified} {
		out, st := returnRun(t, "f() { return 7; }\nf\necho \"st=$?\"\n", refused)
		if !strings.Contains(out, "st=7") {
			t.Errorf("refused=%v: said %q, want st=7", refused, out)
		}
		if strings.Contains(out, "can only") {
			t.Errorf("refused=%v: said %q, want nothing refused", refused, out)
		}
		if st != 0 {
			t.Errorf("refused=%v: status = %d, want 0", refused, st)
		}
	}
}

// And inside a sourced file, which is the other place a `return` has
// something to return from. Unanimous across the panel, which is what makes
// the axis about the remaining case alone.
func TestAReturnInsideASourcedFileIsAlwaysObeyed(t *testing.T) {
	for _, refused := range []Answer{Yes, No, Unspecified} {
		dir := t.TempDir()
		src := "printf 'return 7\\n' > " + dir + "/s.sh\n. " + dir + "/s.sh\necho \"st=$?\"\n"
		out, _ := returnRun(t, src, refused)
		if !strings.Contains(out, "st=7") {
			t.Errorf("refused=%v: said %q, want st=7", refused, out)
		}
		if strings.Contains(out, "can only") {
			t.Errorf("refused=%v: said %q, want nothing refused", refused, out)
		}
	}
}

// A shell with no answer refuses rather than guessing, and the refusal does
// not end the script either — which is the safe direction, because obeying
// would stop it somewhere the other answer would not.
func TestAReturnWithNoAnswerRecordedIsRefused(t *testing.T) {
	out, _ := returnRun(t, "echo before\nreturn 7\necho after\n", Unspecified)
	if !strings.Contains(out, "return") || !strings.Contains(out, "disagree") {
		t.Errorf("said %q, want the axis named", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("said %q, want the script to carry on rather than stop somewhere one answer would not", out)
	}
}

func returnRun(t *testing.T, src string, refused Answer) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.ReturnOutsideAFunctionIsRefused = refused
	// The refusal and its fatality are two axes and this helper is about the
	// first. Only one column refuses at all and it does not stop there, so
	// the pairing every other row here means is refused-and-carries-on; the
	// other pairing is the whole of
	// TestARefusedReturnEndsTheScriptWhereAFailedSpecialBuiltinDoes below.
	sem.BadOptionToSpecialBuiltinFatal = No
	dg := Diagnostics{}
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

// The depth is given back when the sourced file finishes, so a `return`
// *after* a source is still one with nothing to return from.
//
// Without that, sourcing anything at all would leave the rest of the script
// looking like the inside of a sourced file for good.
func TestSourcingDoesNotLeaveTheScriptLookingSourced(t *testing.T) {
	dir := t.TempDir()
	src := "printf 'echo sourced\\n' > " + dir + "/s.sh\n" +
		". " + dir + "/s.sh\n" +
		"return 7\n" +
		"echo \"after st=$?\"\n"

	out, st := returnRun(t, src, Yes)
	if !strings.Contains(out, "sourced") {
		t.Fatalf("said %q, want the sourced file to have run", out)
	}
	if !strings.Contains(out, "can only `return'") || !strings.Contains(out, "after st=2") {
		t.Errorf("said %q, want the later return refused as one with nothing to return from", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// A `return` with nowhere to return from is a special builtin declining the
// way it was called, so whether the *script* ends there is the axis that
// already carries that question — see Runner.refusedReturnOperand, which is
// the same builtin's other refusal one branch over.
//
// The two are genuinely separate: bash refuses the place and carries on,
// bash in POSIX mode refuses it and stops, and `set -o posix` moves the same
// binary between the two. So this is the axis' other value over the same
// source, and the sentence is unchanged by it.
func TestARefusedReturnEndsTheScriptWhereAFailedSpecialBuiltinDoes(t *testing.T) {
	const src = "echo before\nreturn 7\necho \"after st=$?\"\n"
	for _, c := range []struct {
		name   string
		fatal  Answer
		gone   bool
		status int
	}{
		{"carries on where a failed special builtin does", No, false, 0},
		{"ends the script where one is fatal", Yes, true, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf strings.Builder
			sem := PosixSemantics()
			sem.ReturnOutsideAFunctionIsRefused = Yes
			sem.BadOptionToSpecialBuiltinFatal = c.fatal
			dg := Diagnostics{}
			r := newTestRunner(t, &Runner{
				Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh",
			})
			f, err := syntax.Parse(src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			st, err := r.Run(context.Background(), f)
			if err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if !strings.Contains(out, "can only `return'") {
				t.Errorf("said %q, want the refusal in it whichever way it ends", out)
			}
			if got := strings.Contains(out, "after st="); got == c.gone {
				t.Errorf("said %q, want the line after the return to have run: %v", out, !c.gone)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}
