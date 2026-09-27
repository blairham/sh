// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A body arriving as text is parsed with the alias table or without it, and
// that is the caller's to say — an autoloading builtin has a letter for it.
//
// The two methods are the whole of the difference: the same text, the same
// table, and the word is replaced under one and stands under the other. It
// matters that this is asked here rather than left to the parse's own switch,
// which answers about the route the *program* arrived by and not about a file
// read while it runs.
func TestABodyFromTextIsReadWithOrWithoutTheAliasTable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		define  func(*Runner) bool
		wantOut string
	}{
		{"with the table", func(r *Runner) bool {
			return r.DefineFunctionExpandingAliases("f", "hi")
		}, "replaced\n"},
		{"without it", func(r *Runner) bool {
			return r.DefineFunction("f", "hi")
		}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ran bool
			out, _ := run(t, `zzdefine; f`, func(r *Runner) {
				// The table directly rather than through the `alias`
				// builtin, whose option reading is an axis and not this
				// test's subject.
				r.SetAlias("hi", "zzsaw")
				r.Register("zzsaw", func(rr *Runner, _ context.Context, _ []string) int {
					_, _ = rr.Stdout.Write([]byte("replaced\n"))
					return 0
				})
				r.Register("zzdefine", func(rr *Runner, _ context.Context, _ []string) int {
					ran = true
					if !tc.define(rr) {
						t.Error("the text was not read as a body")
					}
					return 0
				})
			})
			if !ran {
				t.Fatal("the definition never happened")
			}
			// Without the table the word is `hi`, which is nothing here.
			if !strings.HasPrefix(out, tc.wantOut) {
				t.Errorf("got %q, want it to start with %q", out, tc.wantOut)
			}
			if tc.wantOut == "" && strings.Contains(out, "replaced") {
				t.Errorf("got %q, want the word left as it was written", out)
			}
		})
	}
}

// The three ways in are one implementation, which is the point of the fold:
// a body that reads one way through `DefineFunction` reads the same way
// through `DefineFunctionFromText`, down to a text the grammar refuses.
func TestTheWaysIntoABodyFromTextAgree(t *testing.T) {
	for _, body := range []string{`echo ok`, `if true; then echo ok; fi`, `; ;`, ``} {
		var byDefine, byText, byAliases bool
		run(t, `zzdefine`, func(r *Runner) {
			r.Register("zzdefine", func(rr *Runner, _ context.Context, _ []string) int {
				byDefine = rr.DefineFunction("a", body)
				byText = rr.DefineFunctionFromText("b", body)
				byAliases = rr.DefineFunctionExpandingAliases("c", body)
				return 0
			})
		})
		if byDefine != byText || byDefine != byAliases {
			t.Errorf("%q: DefineFunction=%v FromText=%v ExpandingAliases=%v, want one answer",
				body, byDefine, byText, byAliases)
		}
	}
}

// The lines of a body that arrived as text are the **text's own**, and that
// is a fact the seam has to correct for rather than one it gets for free: the
// body is read by wrapping it in a declaration, and the wrapper is a line the
// parser counts, so every position inside the body used to be one greater
// than the line the file has.
//
// It stood for as long as it did because every reader in the shell consumed
// the number as a *subtrahend* — `$LINENO` in a body and a diagnostic's
// offset are both `at - funcLine`, and both carried the extra line, so the
// wrapper canceled itself. The first reader to want the line **absolutely**
// is the one that found it (#4471), which is why the row that matters here
// reads a line with no function offset anywhere near it.
//
// Two rows below the first, because the correction is two corrections that
// have to compose: the wrapper's, carried on the outer definition, and the
// body's own offset, carried on a declaration the body holds. A function
// defined inside text-read text needs both at once.
func TestABodyReadFromTextIsNumberedFromItsOwnFirstLine(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{
			// The read is on the body's third line, and nothing here
			// subtracts a definition line from it.
			"the body's own lines", ":\n:\necho L=$LINENO\n", "L=3\n",
		},
		{
			// A declaration the body holds is on the body's second line,
			// and its own body is on the third. Both corrections apply to
			// this one: the wrapper's, on the text around it, and the text's
			// offset, recorded on the declaration as it is read.
			"a declaration the text holds",
			":\ng() {\n  echo L=$LINENO\n}\ng\n", "L=3\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, "zzdefine\nf\n", func(r *Runner) {
				r.Register("zzdefine", func(rr *Runner, _ context.Context, _ []string) int {
					if !rr.DefineFunction("f", tc.body) {
						t.Error("the text was not read as a body")
					}
					return 0
				})
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// And the control the correction must not reach: a declaration the parser
// read out of the program has no wrapper in front of it, so its body's lines
// are the script's and a correction applied to every definition alike would
// take them one too low.
func TestADeclarationTheParserReadIsNotCorrected(t *testing.T) {
	out, _ := run(t, ":\nf() {\n  echo L=$LINENO\n}\nf\n", nil)
	if want := "L=3\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
