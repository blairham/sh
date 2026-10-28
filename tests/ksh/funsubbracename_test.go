// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// A `{name}` in a `${ cmd;}` body opens no level of the body's braces, so the
// descriptor-picking redirection reads through it, where a brace expansion's
// group still opens one that nothing closes (#6559). Measured 2026-10-15 on
// ksh93u+ 2012-08-01 at /bin/ksh with `-c`; the table is on
// syntax.braceNameWordLen.
func TestABraceNameOpensNoLevelInACurrentShellBody(t *testing.T) {
	for _, src := range []string{
		"echo ${ exec {v}</dev/null; echo $v; }\n",
		"echo ${ exec {ab}>>/dev/null; echo $ab; }\n",
		"echo ${ exec {v}<&0; echo $v; }\n",
		"echo ${ exec {v}>&-; echo z; }\n",
		"echo \"${ exec {v}</dev/null; echo $v; }\"\n",
		"echo ${ x=${ exec {v}</dev/null; echo $v; }; echo $x; }\n",
		"echo ${ echo {v}; }\n",
		"echo ${ echo {}; }\n",
		"echo ${ echo {9a}; }\n",
		"echo ${ echo {v}{w}; }\n",
		"echo ${ echo {v}-; }\n",
		"echo ${ echo {v}\"x\"; }\n",
		"echo ${ echo {v}$HOME; }\n",
		"echo ${ echo {v}|cat; }\n",
		"echo ${ echo {v}(x); }\n",
		"echo ${ echo {v}:; }\n",
	} {
		if _, err := syntax.Parse(src, ksh.Dialect()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
	for _, src := range []string{
		"echo ${ echo {a,b}; }\n",
		"echo ${ echo {1..2}; }\n",
		"echo ${ echo {a[1]}; }\n",
		"echo ${ echo {a-b}; }\n",
		"echo ${ echo {v}x; }\n",
		"echo ${ echo {v}1; }\n",
		"echo ${ echo {v}_; }\n",
		"echo ${ echo {v}.; }\n",
		"echo ${ echo {v},; }\n",
		"echo ${ echo {v}/x; }\n",
		"echo ${ echo {v}=; }\n",
		"echo ${ echo {v}#; }\n",
		"echo ${ echo {}x; }\n",
		"echo ${ echo {v}{a,b}; }\n",
	} {
		_, err := syntax.Parse(src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want `{' unmatched", src)
			continue
		}
		if got, want := diagTable(ksh.Diagnostics()).ParseFailure(err), "syntax error at line 1: `{' unmatched"; got != want {
			t.Errorf("%q:\n got %q\nwant %q", src, got, want)
		}
	}
	// And a `}` straight behind one begins a token too, so it closes the
	// body, and what follows is refused on its own.
	for _, src := range []string{
		"echo ${ echo {v}};}\n",
		"echo ${ echo {v}}x; }\n",
	} {
		_, err := syntax.Parse(src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want `}' unexpected", src)
			continue
		}
		if got, want := diagTable(ksh.Diagnostics()).ParseFailure(err), "syntax error at line 1: `}' unexpected"; got != want {
			t.Errorf("%q:\n got %q\nwant %q", src, got, want)
		}
	}
}
