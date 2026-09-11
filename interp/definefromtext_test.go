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
