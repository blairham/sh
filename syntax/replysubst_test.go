// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// `${| cmd;}` is the current-shell substitution valued from `$REPLY`, and its
// marker is a `|` *adjacent* to the brace — which is not the blank form's rule
// wearing a second character.
//
// Measured 2026-09-13 on bash 5.3.15, the one shell in the panel that has it:
//
//	${|REPLY=hi; }     hi             no blank needed after the pipe
//	${| REPLY=hi; }    hi             a blank after it is ordinary body
//	${ | REPLY=hi; }   syntax error   `|' unexpected, looking for `}'
//	${|}               empty          at status 0
//
// The third row is the one that parts the rules: with a blank first the body has
// already begun, and a `|` opening it is a pipeline with nothing on its left. A
// single flag reading "a blank *or* a pipe after the brace" would take it.
func TestAReplySubstitutionIsToldByTheAdjacentPipe(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.CurrentShellSubstitution = true
	d.ReplySubstitution = true
	for _, c := range []struct {
		name, src string
		reply     bool
		wantBody  string
		wantIsCmd bool
	}{
		{"a pipe makes it the reply form", "echo ${| REPLY=hi;}", true, " REPLY=hi;", true},
		{"with no blank after it", "echo ${|REPLY=hi;}", true, "REPLY=hi;", true},
		{"and with an empty body", "echo ${|}", true, "", true},
		{
			// Still the blank form, whose body is `| REPLY=hi;` — a
			// pipeline with nothing on its left, which is what bash blames.
			"a blank first leaves it the blank form",
			"echo ${ | REPLY=hi;}", false, " | REPLY=hi;", true,
		},
		{"a name is still a parameter", "echo ${x}", false, "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			sp := onlySubstitution(t, c.src, d)
			if isCmd := sp.Kind == syntax.CommandSubst && sp.CurrentShell; isCmd != c.wantIsCmd {
				t.Fatalf("read as a command = %v, want %v", isCmd, c.wantIsCmd)
			}
			if sp.ReplyValue != c.reply {
				t.Errorf("ReplyValue = %v, want %v", sp.ReplyValue, c.reply)
			}
			if c.wantIsCmd && sp.Value != c.wantBody {
				// The `|` is the marker and not body text, where the blank
				// form's opening blank *is* body text.
				t.Errorf("body = %q, want %q", sp.Value, c.wantBody)
			}
		})
	}
}

// Without the flag it is not this construct, and the four dialects that do not
// have it go on saying what they said before.
//
// bash alone has it: measured 2026-09-13, `echo ${| REPLY=hi; }` is `bad
// substitution` in bash 3.2.57, zsh 5.9.2 and dash, and ksh93u+ — which has the
// blank form — answers “ `|' unexpected “. The last of those is why the
// CurrentShellSubstitution half is tested separately below: with that flag on
// and this one off, the body is never extracted and the refusal stays the
// parameter form's.
func TestWithoutTheFlagThePipeIsNotAMarker(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"the core", "a dialect with the blank form"} {
		t.Run(name, func(t *testing.T) {
			d := syntax.Core()
			if name != "the core" {
				d.CurrentShellSubstitution = true
			}
			sp := onlySubstitution(t, "echo ${| REPLY=hi;}", d)
			if sp.Kind != syntax.ParamExp {
				t.Fatalf("kind = %v, want a parameter expansion", sp.Kind)
			}
			if sp.Param == nil || !sp.Param.Bad {
				t.Error("read a clean parameter expansion, want it marked bad")
			}
		})
	}
}

// The spelling is written back as it was read. The `|` is the one byte a round
// trip has to put back rather than reprint, because the lexer took it off the
// body — see Span.ReplyValue.
func TestTheReplyFormRoundTrips(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.CurrentShellSubstitution = true
	d.ReplySubstitution = true
	for _, src := range []string{
		"echo ${| REPLY=hi;}",
		"echo ${|REPLY=hi;}",
		"echo ${|}",
		"echo \"${| REPLY=hi;}\"",
		"echo ${| REPLY=${| REPLY=in;};}",
		"echo ${| REPLY=${ echo in;};}",
		"x=${| REPLY=v;}",
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

// A `#` in the body is a comment here as it is in the blank form, which is the
// half of the flag that is load-bearing rather than tidy: without the construct
// the text is an ordinary expansion and an apostrophe in the comment runs off
// the end. Measured on bash 5.3.15, `echo "${| REPLY=hi # it's fine\n}"` is
// `hi` — the same shape #1397 is about, one spelling over.
func TestTheReplyFormsBodyTakesAComment(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.CurrentShellSubstitution = true
	d.ReplySubstitution = true
	src := "echo ${| REPLY=hi # it's fine }\n}\n"
	sp := onlySubstitution(t, src, d)
	if !sp.ReplyValue {
		t.Fatalf("read as %v with ReplyValue %v, want the reply form", sp.Kind, sp.ReplyValue)
	}
	if !strings.Contains(sp.Value, "\n") {
		t.Errorf("body = %q, want the comment to have carried a brace past the newline", sp.Value)
	}
}

// onlySubstitution parses one command and returns the expansion in it, which is
// the walk every test in this file would otherwise repeat.
func onlySubstitution(t *testing.T, src string, d syntax.Dialect) syntax.Span {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var found syntax.Span
	for _, st := range f.Stmts {
		for _, cmd := range st.Expr.(*syntax.Pipeline).Cmds {
			sc, ok := cmd.(*syntax.SimpleCmd)
			if !ok {
				continue
			}
			words := sc.Args
			for _, a := range sc.Assigns {
				words = append(words, a.Value)
			}
			for _, w := range words {
				if w == nil {
					continue
				}
				for _, sp := range w.Spans {
					if sp.Kind == syntax.CommandSubst || sp.Kind == syntax.ParamExp {
						found = sp
					}
				}
			}
		}
	}
	if found.Kind == syntax.Literal {
		t.Fatalf("no expansion found in %q", src)
	}
	return found
}
