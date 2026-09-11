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

// The trace prefix is a parameter — #1454, where it was the constant `+ ` in
// every case, whatever `PS4` held.
//
// Named for the parameter and the axis rather than for a shell: which dialect
// repeats the first character, and which prompt language each one reads in the
// value, are dialect/'s to assert.

// tracedWith runs a script under a dialect's prompt style and returns what
// went to standard error.
//
// A helper of its own rather than traceOf, because the whole point of the
// prefix is that it is rendered the way *this* dialect renders a prompt, and
// traceOf builds a runner with no table at all.
func tracedWith(t *testing.T, src string, diag Diagnostics, st PromptStyle, user string) string {
	t.Helper()
	d := syntax.Core()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	var out, errOut bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errOut,
		Semantics: &sem, Diagnostics: &diag, Name: "sh", Env: testPATH(),
	})
	r.SetPromptStyle(st)
	if user != "" {
		r.SetPromptUser(user)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return errOut.String()
}

func TestTheTracePrefixIsTheParameter(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a value set in the shell", `PS4="XX "; set -x; :`, "XX :\n"},
		{"an empty value draws nothing before the command", `PS4=; set -x; :`, ":\n"},
		{"no value falls back to this dialect's own prefix", `set -x; :`, "+ :\n"},
		{
			// Each time it is drawn rather than once when it is assigned,
			// which is the whole reason a prompt parameter is expanded.
			"the value is expanded at every trace",
			"PS4='+$n '; set -x; n=1; n=2", "+ n=1\n+1 n=2\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := tracedWith(t, c.src, Diagnostics{}, PromptStyle{Expand: PromptExpandsAlways}, "")
			if got != c.want {
				t.Errorf("%s traced %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// The value goes through the *prompt* language, which is what makes the trace
// prefix the only route to a prompt escape that needs no terminal.
func TestTheTracePrefixReadsTheDialectsPromptLanguage(t *testing.T) {
	style := PromptStyle{
		Escape:  '%',
		Codes:   map[rune]PromptField{'%': FieldEscape, 'n': FieldUser},
		Expand:  PromptExpandsAlways,
		Unknown: DropBoth,
	}
	if got, want := tracedWith(t, `PS4='<%n> '; set -x; :`, Diagnostics{}, style, "someone"), "<someone> :\n"; got != want {
		t.Errorf("traced %q, want %q", got, want)
	}
	// A code in no table is the *drawer's* policy rather than a script's
	// refusal: a prefix has to draw something, so this dialect's answer for
	// an escape it does not have is what appears.
	if got, want := tracedWith(t, `PS4='<%q> '; set -x; :`, Diagnostics{}, style, "someone"), "<> :\n"; got != want {
		t.Errorf("an unknown code traced %q, want %q", got, want)
	}
	// And with no table at all — the substrate's own answer — the value is
	// drawn as it stands rather than having a language invented for it.
	if got, want := tracedWith(t, `PS4='<%n> '; set -x; :`, Diagnostics{}, PromptStyle{}, "someone"), "<%n> :\n"; got != want {
		t.Errorf("with no table traced %q, want %q", got, want)
	}
	// A dialect that does not expand draws the parameter's own text, which
	// is the second half of "the prompt language": one shell in the panel
	// answers no here unless a script has said otherwise.
	if got, want := tracedWith(t, `PS4='+$x '; set -x; x=1`, Diagnostics{}, PromptStyle{}, ""), "+$x x=1\n"; got != want {
		t.Errorf("unexpanded traced %q, want %q", got, want)
	}
}

// TracePrefixRepeatsAtIndirection, both answers.
func TestTheTracePrefixCountsIndirection(t *testing.T) {
	for _, c := range []struct{ name, src, on, off string }{
		{"an eval", `PS4="XY "; set -x; eval :`, "XY eval :\nXXY :\n", "XY eval :\nXY :\n"},
		{
			"an eval inside an eval", `set -x; eval "eval :"`,
			"+ eval 'eval :'\n++ eval :\n+++ :\n", "+ eval 'eval :'\n+ eval :\n+ :\n",
		},
		// In a command word rather than in an assignment: an assignment
		// whose value is a substitution runs the substitution twice under
		// `set -x` on this shell, which is a separate bug (#1915) and not
		// one this test should bake in.
		{"a command substitution", `set -x; echo $(:)`, "++ :\n+ echo\n", "+ :\n+ echo\n"},
		// The count is of text being read again, not of the stack: a call
		// and a subshell add nothing in any column.
		{"a function call, which is not indirection", `set -x; f(){ :; }; f`, "+ f\n+ :\n", "+ f\n+ :\n"},
		{"a subshell, which is not either", `set -x; (:)`, "+ :\n", "+ :\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			style := PromptStyle{Expand: PromptExpandsAlways}
			on := tracedWith(t, c.src, Diagnostics{TraceQuoting: QuoteShell, TracePrefixRepeatsAtIndirection: true}, style, "")
			if on != c.on {
				t.Errorf("counting: traced %q, want %q", on, c.on)
			}
			off := tracedWith(t, c.src, Diagnostics{TraceQuoting: QuoteShell}, style, "")
			if off != c.off {
				t.Errorf("not counting: traced %q, want %q", off, c.off)
			}
		})
	}
}
