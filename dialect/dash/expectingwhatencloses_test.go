// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// Which closer a refusal says it was expecting is the enclosing construct's,
// and some refusals expect nothing. Every row measured 2026-10-04 on dash
// 0.5.12.
func TestARefusalExpectsWhatTheEnclosingConstructWanted(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// A `(` after a word ends the command and the list refuses it.
		{`{ a=(x); }`, `"(" unexpected (expecting "}")`},
		{`if a=(x); then :; fi`, `"(" unexpected (expecting "then")`},
		{`{ echo x(a); }`, `"(" unexpected (expecting "}")`},
		{`a=(x)`, `"(" unexpected`},
		// An arm's body ended by something that is neither `;;` nor `esac`.
		{`case x in x) echo ) ;; esac`, `")" unexpected (expecting ";;")`},
		// Control: a separator where a command would begin is refused as
		// itself.
		{`case x in x) ; echo two;; esac`, `";" unexpected`},
		{`case x in x) echo; & esac`, `"&" unexpected`},
		{`case x in x) | ;; esac`, `"|" unexpected`},
		{`case x in x) && ;; esac`, `"&&" unexpected`},
		// The input running out where an arm's pattern would begin.
		{`case x in`, `end of file unexpected (expecting ")")`},
		{`case x in x) : ;;`, `end of file unexpected (expecting ")")`},
		{`case x in (`, `end of file unexpected (expecting ")")`},
		// A token ending a `for` word list expects nothing.
		{`for x in a (b); do :; done`, `"(" unexpected`},
		{`for i in a >f; do :; done`, `redirection unexpected`},
		// Control: the separator came, and then the wrong word.
		{`for i in a; x`, `word unexpected (expecting "do")`},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		if err == nil {
			t.Errorf("%s: parsed, want a refusal", tc.src)
			continue
		}
		got := dash.Diagnostics().ParseFailure(err)
		if !strings.HasSuffix(got, tc.want) {
			t.Errorf("%s: %q, want it to end %q", tc.src, got, tc.want)
		}
	}
}
