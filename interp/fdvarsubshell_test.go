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

// A `{name}` redirection on a `( … )` is the subshell's where
// Semantics.FdVariableOnASubshellIsTheSubshells is Yes: the parentheses see
// the number, and the shell around them keeps neither the name nor the
// descriptor (#6520). No keeps the number in the shell. Every row answers the
// outliving axis Yes, which is the answer that would keep the descriptor if
// anything did, so a descriptor given back on the Yes side is given back
// because the store was the subshell's.
func TestAnFdVariableOnASubshell(t *testing.T) {
	for _, c := range []struct {
		name, src string
		subshells Answer
		want      string
	}{
		{"yes: the name stays unset", `( : ) {v}</dev/null; echo "[${v-unset}]"`, Yes, "[unset]\n"},
		{"yes: the parentheses see the number", `( [ -n "$v" ] && echo in ) {v}</dev/null`, Yes, "in\n"},
		{"yes: the name keeps its value", `v=keep; ( : ) {v}</dev/null; echo "[$v]"`, Yes, "[keep]\n"},
		{"yes: the status is the body's", `( exit 3 ) {v}</dev/null; echo "st=$? [${v-unset}]"`, Yes, "st=3 [unset]\n"},
		{
			"yes: the descriptor is not kept",
			`exec {x}</dev/null; exec {x}<&-
( : ) {v}</dev/null
exec {y}</dev/null
[ "$x" = "$y" ] && echo reused`,
			Yes, "reused\n",
		},
		{"no: the name is set", `( : ) {v}</dev/null; [ -n "$v" ] && echo set`, No, "set\n"},
		{"no: the parentheses see the number", `( [ -n "$v" ] && echo in ) {v}</dev/null`, No, "in\n"},
		{"no: an old value is replaced", `v=keep; ( : ) {v}</dev/null; [ "$v" != keep ] && echo moved`, No, "moved\n"},
		// The control on the Yes side: a group is this shell's, whatever the
		// subshell axis says.
		{"yes: a group keeps the name", `{ :; } {v}</dev/null; [ -n "$v" ] && echo set`, Yes, "set\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := runFdVar(t, t.TempDir(), func(s *Semantics) {
				s.FdVariableOutlivesTheCommand = Yes
				s.FdVariableOnASubshellIsTheSubshells = c.subshells
			}, c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The axis is asked only where parentheses carry a `{name}` redirection, so
// an unanswered vector still runs every other subshell, and every other
// `{name}`, in silence — and refuses by name the one input it is about, once:
// the outliving axis is left unanswered as well, as it is in the bare vector,
// and must not add a second refusal for the same redirection.
func TestAnFdVariableOnASubshellAsksOnlyThere(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		// refused rows run the bare vector, with the outliving axis
		// unanswered as well; the others answer it, so what they show is that
		// the subshell axis was not asked.
		refused bool
	}{
		{"a numbered redirection", `( echo in ) 3</dev/null; echo "st=$?"`, "in\nst=0\n", false},
		{"a plain subshell", `( echo in ); echo "st=$?"`, "in\nst=0\n", false},
		{"a group", `{ :; } {v}</dev/null; [ -n "$v" ] && echo set`, "set\n", false},
		{"a program", `cat {v}</dev/null </dev/null; echo "[${v-unset}]"`, "[unset]\n", false},
		{"the subshell's own", `( echo in ) {v}</dev/null; echo "st=$?"`, "st=2\n", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			sem := PosixSemantics()
			sem.RedirectErrorOnSpecialBuiltinFatal = No
			sem.FdVariableOutlivesTheCommand = Yes
			if c.refused {
				sem.FdVariableOutlivesTheCommand = Unspecified
			}
			sem.FdVariableOnASubshellIsTheSubshells = Unspecified
			var out, errs strings.Builder
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: t.TempDir(),
				Stdout: &out, Stderr: &errs,
			}).SeedVars(map[string]string{"PATH": lookBinPath(t)})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if out.String() != c.want {
				t.Errorf("out %q, want %q", out.String(), c.want)
			}
			want := ""
			if c.refused {
				want = "a variable-named descriptor on a subshell"
			}
			if got := errs.String(); (want == "") != (got == "") || !strings.Contains(got, want) ||
				strings.Count(got, "\n") > 1 {
				t.Errorf("stderr %q, want one refusal naming %q", got, want)
			}
		})
	}
}
