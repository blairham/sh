// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// `${(list)}` is one span, a command substitution whose value is the
// parenthesis and everything in it — so what runs it needs no rule of its
// own: it parses the value and finds a subshell there (#2615).
func TestAParenthesizedBodyIsOneCommandSubstitution(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.SubshellSubstitution = true
	for _, c := range []struct{ src, value string }{
		{"echo ${(echo hi)}", "(echo hi)"},
		{"echo ${( echo hi )}", "( echo hi )"},
		{"echo A${(echo hi)}B", "(echo hi)"},
		// The end is where a list stops, so a `)` inside quotes and a `)` of
		// a nested subshell are both stepped over.
		{`echo ${(echo ")" )}`, `(echo ")" )`},
		{"echo ${( (echo a) )}", "( (echo a) )"},
	} {
		f, err := syntax.Parse(c.src+"\n", d)
		if err != nil {
			t.Errorf("%s: parse: %v", c.src, err)
			continue
		}
		var got []syntax.Span
		for _, st := range f.Stmts {
			for _, cmd := range st.Expr.(*syntax.Pipeline).Cmds {
				for _, w := range cmd.(*syntax.SimpleCmd).Args {
					for _, sp := range w.Spans {
						if sp.Kind == syntax.CommandSubst || sp.Kind == syntax.ParamExp {
							got = append(got, sp)
						}
					}
				}
			}
		}
		if len(got) != 1 {
			t.Errorf("%s: %d substitution spans, want 1", c.src, len(got))
			continue
		}
		if got[0].Kind != syntax.CommandSubst || !got[0].CurrentShell {
			t.Errorf("%s: kind %v, CurrentShell %v — want a current-shell command substitution",
				c.src, got[0].Kind, got[0].CurrentShell)
		}
		if got[0].Value != c.value {
			t.Errorf("%s: value %q, want %q", c.src, got[0].Value, c.value)
		}
	}
}

// Written back as it was read, which the parenthesis gives for nothing: the
// value already carries it, so the printer's `${` + value + `}` is the same
// bytes. The blank form's leading blank is body text for the same reason, and
// a rule that had stripped the parens here would print the blank spelling and
// change the construct.
func TestAParenthesizedBodyPrintsBackUnchanged(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.SubshellSubstitution = true
	for _, src := range []string{
		"echo ${(echo hi)}\n",
		"echo \"A${(echo hi)}B\"\n",
		"echo ${( (echo a) )}\n",
		"x=${(echo a)}${(echo b)}\n",
	} {
		f, err := syntax.Parse(src, d)
		if err != nil {
			t.Errorf("%q: parse: %v", src, err)
			continue
		}
		want := strings.TrimSuffix(src, "\n")
		if got := syntax.Print(f); got != want {
			t.Errorf("printed %q, want %q", got, want)
		}
	}
}

// The rule declines rather than refuses, so a word it does not fit is read
// the way it was before the flag. That is ksh93's own order and the reason
// the flag groups written for another shell keep their diagnostic: `ksh -n`
// refuses `${(echo a);}` while reading and passes `${(U)a}` to the run.
func TestAParenBodyThatDoesNotCloseOnItsBraceIsNotTaken(t *testing.T) {
	t.Parallel()
	on, off := syntax.Core(), syntax.Core()
	on.SubshellSubstitution = true
	for _, src := range []string{
		"echo ${(U)a}\n",
		"echo ${(a; b)x}\n",
		"echo ${(echo a);}\n",
		"echo ${(echo a) ;}\n",
		"echo ${(echo a)b}\n",
		// Two adjacent parens are ksh93's braced arithmetic and not this
		// construct, so the flag leaves the spelling exactly where it was.
		"echo ${((1+2))}\n",
	} {
		_, withErr := syntax.Parse(src, on)
		_, withoutErr := syntax.Parse(src, off)
		if (withErr == nil) != (withoutErr == nil) {
			t.Errorf("%q: with the flag %v, without it %v — want the same reading", src, withErr, withoutErr)
			continue
		}
		if withErr != nil && withoutErr != nil && withErr.Error() != withoutErr.Error() {
			t.Errorf("%q: with the flag %q, without it %q", src, withErr, withoutErr)
		}
	}
}

// And the flag is what turns it on. Without it the word is an ordinary
// parameter expansion whose name is `(echo hi)`, which is what carries the
// five columns' answer: none of them refuses it while reading, they all defer
// to the run and call it a bad substitution there.
func TestWithoutTheFlagAParenthesizedBodyIsAParameter(t *testing.T) {
	t.Parallel()
	f, err := syntax.Parse("echo ${(echo hi)}\n", syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, st := range f.Stmts {
		for _, cmd := range st.Expr.(*syntax.Pipeline).Cmds {
			for _, w := range cmd.(*syntax.SimpleCmd).Args {
				for _, sp := range w.Spans {
					if sp.Kind == syntax.CommandSubst {
						t.Errorf("read as a command substitution under the core dialect, want a parameter")
					}
				}
			}
		}
	}
}
