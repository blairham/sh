// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// `${ cmd;}` is a command substitution and `${x}` is a parameter, and the
// space after the brace is the whole of the difference.
func TestABracedCommandSubstitutionIsToldByTheSpace(t *testing.T) {
	d := syntax.Core()
	d.CurrentShellSubstitution = true
	for _, c := range []struct {
		name, src string
		command   bool
	}{
		{"a space makes it a command", "echo ${ echo one;}", true},
		{"a tab too", "echo ${\techo one;}", true},
		{"and a newline", "echo ${\necho one;}", true},
		{"a name makes it a parameter", "echo ${x}", false},
		{"and so does an operator", "echo ${x:-d}", false},
		{"even one that looks like a command", "echo ${x-echo hi}", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, d)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var got syntax.SpanKind
			var current bool
			for _, st := range f.Stmts {
				for _, cmd := range st.Expr.(*syntax.Pipeline).Cmds {
					for _, w := range cmd.(*syntax.SimpleCmd).Args {
						for _, sp := range w.Spans {
							if sp.Kind == syntax.CommandSubst || sp.Kind == syntax.ParamExp {
								got, current = sp.Kind, sp.CurrentShell
							}
						}
					}
				}
			}
			if c.command && got != syntax.CommandSubst {
				t.Errorf("kind = %v, want a command substitution", got)
			}
			if isCmd := got == syntax.CommandSubst && current; isCmd != c.command {
				t.Errorf("read as a command = %v, want %v", isCmd, c.command)
			}
		})
	}
}

// Without the dialect flag it is not a substitution at all: the parser looks
// for a parameter name, does not find one, and refuses — which is what the
// two shells without this construct then word as a bad substitution.
func TestWithoutTheFlagItIsRefused(t *testing.T) {
	_, err := syntax.Parse("echo ${ echo one;}", syntax.Core())
	if err == nil {
		t.Fatal("parsed, want a parameter name to be expected")
	}
	if !strings.Contains(err.Error(), "parameter name") {
		t.Errorf("said %q, want it to be about a parameter name", err)
	}
}

// The spelling is written back as it was read, so a round trip keeps it.
func TestTheBracedFormRoundTrips(t *testing.T) {
	d := syntax.Core()
	d.CurrentShellSubstitution = true
	for _, src := range []string{
		"echo ${ echo one;}",
		"echo \"${ echo one;}\"",
		"echo ${ echo ${ echo in;};}",
		"echo ${ printf \"%s\" \"a}b\";}",
		"x=${ echo v;}",
	} {
		f, err := syntax.Parse(src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if got := strings.TrimRight(syntax.Print(f), "\n"); got != src {
			t.Errorf("printing %q gave %q", src, got)
		}
	}
}
