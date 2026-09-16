// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// braceProgramDialect is the vector these tests move: the construct on, and
// the axis set to the reading under test. Everything else is the core, so a
// row that changes is this axis changing and not another flag.
func braceProgramDialect(end syntax.BraceProgramBodyEnd) syntax.Dialect {
	d := syntax.Core()
	d.CurrentShellSubstitution = true
	d.BraceProgramBodyEnd = end
	return d
}

// braceProgramBody returns the text of the first current-shell substitution
// in the program, and whether there was one.
func braceProgramBody(t *testing.T, src string, d syntax.Dialect) (string, bool) {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		return "", false
	}
	for _, st := range f.Stmts {
		p, ok := st.Expr.(*syntax.Pipeline)
		if !ok {
			continue
		}
		for _, cmd := range p.Cmds {
			s, ok := cmd.(*syntax.SimpleCmd)
			if !ok {
				continue
			}
			for _, w := range s.Args {
				for _, sp := range w.Spans {
					if sp.Kind == syntax.CommandSubst && sp.CurrentShell {
						return sp.Value, true
					}
				}
			}
		}
	}
	return "", false
}

// A body with no terminator in front of the closing brace is not a closed
// substitution under either reading. Both shells that have the construct
// refuse it — one naming the end of the input and the other the brace it
// never matched — and reading it as closed runs a command the author never
// wrote, which is the direction that turns a refusal into a program (#2711).
func TestABodyWithNoTerminatorBeforeTheBraceNeverCloses(t *testing.T) {
	t.Parallel()
	for _, end := range []struct {
		name string
		end  syntax.BraceProgramBodyEnd
	}{
		{"where a list ends", syntax.BraceProgramBodyEndsWhereAListEnds},
		{"at a token start", syntax.BraceProgramBodyEndsAtATokenStart},
	} {
		t.Run(end.name, func(t *testing.T) {
			for _, src := range []string{
				"echo ${ echo hi}",
				"echo ${ echo $(echo x)}",
				"echo ${ echo hi # cmt }",
			} {
				if _, err := syntax.Parse(src, braceProgramDialect(end.end)); err == nil {
					t.Errorf("%q parsed; want a refusal — the body has no terminator in front of the brace", src)
				}
			}
		})
	}
}

// Where the body ends is the axis, and these are the rows that separate its
// two readings (#2724). A body is reported by its text so that a row which
// stops in the wrong place is visible as text rather than only as an output.
func TestWhereABraceProgramBodyEndsFollowsTheAxis(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		src  string
		// list and token are the body each reading takes, or "" for a
		// reading that refuses the line.
		list, token string
	}{
		{
			name: "a terminator in front of the brace closes it either way",
			src:  "echo ${ echo a;}",
			list: " echo a;", token: " echo a;",
		},
		{
			name: "a brace in the middle of a word is an ordinary character either way",
			src:  "echo ${ echo a}b;}",
			list: " echo a}b;", token: " echo a}b;",
		},
		{
			// The token reading ends the body at the brace written as an
			// argument, which leaves `;}` standing in the line and the whole
			// statement is refused — which is the answer the shell with that
			// reading gives, blaming the brace. The row below is the same
			// end with a remainder that parses.
			name: "a brace in argument position ends only the token reading",
			src:  "echo ${ echo } ;}",
			list: " echo } ;", token: "",
		},
		{
			name: "and so does one with a word after it",
			src:  "echo A${ echo B }C",
			list: "", token: " echo B ",
		},
		{
			name: "a group's brace is mid-word, so the token reading runs out",
			src:  "echo ${ echo {a,b};}",
			list: " echo {a,b};", token: "",
		},
		{
			name: "the closing brace need not be a token of its own",
			src:  "echo X${ echo a;}Y",
			list: " echo a;", token: " echo a;",
		},
		{
			name: "a quoted brace closes nothing under either reading",
			src:  `echo ${ echo "a}b" ;}`,
			list: ` echo "a}b" ;`, token: ` echo "a}b" ;`,
		},
		{
			name: "a nested parameter expansion ends at its own brace",
			src:  "echo ${ echo ${x};}",
			list: " echo ${x};", token: " echo ${x};",
		},
		{
			name: "a subshell's paren ends a token and a substitution's does not",
			src:  "echo ${ (echo q)}",
			list: " (echo q)", token: " (echo q)",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, r := range []struct {
				name string
				end  syntax.BraceProgramBodyEnd
				want string
			}{
				{"where a list ends", syntax.BraceProgramBodyEndsWhereAListEnds, c.list},
				{"at a token start", syntax.BraceProgramBodyEndsAtATokenStart, c.token},
			} {
				t.Run(r.name, func(t *testing.T) {
					got, ok := braceProgramBody(t, c.src, braceProgramDialect(r.end))
					if r.want == "" {
						if ok {
							t.Errorf("%q read a body %q; want the line refused", c.src, got)
						}
						return
					}
					if !ok {
						t.Fatalf("%q was refused; want the body %q", c.src, r.want)
					}
					if got != r.want {
						t.Errorf("%q body = %q, want %q", c.src, got, r.want)
					}
				})
			}
		})
	}
}

// The older substitution shields a brace under the token reading and the
// newer one does not, which is the one place the two spellings of a
// substitution part company there. Without the distinction the body of
// `${ echo $(echo x)}` would close at the brace behind the parenthesis,
// where the shell with this reading refuses the line.
func TestTheTokenReadingLetsOnlyTheOlderSubstitutionShieldABrace(t *testing.T) {
	t.Parallel()
	d := braceProgramDialect(syntax.BraceProgramBodyEndsAtATokenStart)
	got, ok := braceProgramBody(t, "echo ${ echo `echo }x`;}", d)
	if !ok {
		t.Fatalf("the backquoted body was refused; want it kept whole")
	}
	if want := " echo `echo }x`;"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if _, ok := braceProgramBody(t, "echo ${ echo $(echo }x);}", d); ok {
		t.Errorf("the newer spelling shielded the brace; want the body to end inside it")
	}
}

// The reply form's body is the same list with a marker in front of it, so it
// takes the same rule: `${|REPLY=hi}` has no terminator either and is the
// same refusal as the blank form's.
func TestTheReplyBodyEndsWhereTheBlankOnesDoes(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.CurrentShellSubstitution = true
	d.ReplySubstitution = true
	if _, err := syntax.Parse("echo ${|REPLY=hi}", d); err == nil {
		t.Errorf("a reply body with no terminator parsed; want a refusal")
	}
	got, ok := braceProgramBody(t, "echo ${|REPLY=hi;}", d)
	if !ok {
		t.Fatalf("`${|REPLY=hi;}` was refused; want it read")
	}
	if want := "REPLY=hi;"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
