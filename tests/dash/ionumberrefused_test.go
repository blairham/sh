// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// A refused IO number is a redirection, and is worded as one rather than
// named `IO number` (#6552). Measured 2026-10-15: `case 2>x in x) esac` is
// `redirection unexpected (expecting word)` on dash 0.5.12 and
// `unexpected redirection (expecting word)` on BusyBox ash 1.37.0.
func TestARefusedIONumberIsARedirection(t *testing.T) {
	_, err := syntax.Parse("case 2>x in x) esac\n", dash.Dialect())
	if err == nil {
		t.Fatal("parsed, want a refusal")
	}
	got := dash.Diagnostics().ParseFailure(err)
	if want := "Syntax error: redirection unexpected (expecting word)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Where a `case` pattern list starts, an IO number is part of the one
// operator this shell lets stand there, so the refusal is of the word after
// it — and the operator alone opens an arm that matches nothing (#6617).
// Measured 2026-10-15 on dash 0.5.12, `-c`; the rows are on
// syntax.Parser.casePatterns.
func TestAnIONumberStartingAPatternIsPartOfTheOperator(t *testing.T) {
	const word = `Syntax error: word unexpected (expecting ")")`
	const redir = `Syntax error: redirection unexpected (expecting ")")`
	for _, c := range []struct{ src, want string }{
		{"case x in 2>x) esac\n", word},
		{"case x in (2>x) esac\n", word},
		{"case x in a|2>x) esac\n", word},
		{"case x in 2>>x) esac\n", word},
		{"case x in 2<&1) esac\n", word},
		// Not where the list starts, and two digits are a word here.
		{"case x in a 2>x) esac\n", redir},
		{"case x in 22>x) esac\n", redir},
	} {
		_, err := syntax.Parse(c.src, dash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", c.src)
			continue
		}
		if got := dash.Diagnostics().ParseFailure(err); got != c.want {
			t.Errorf("%q: got %q, want %q", c.src, got, c.want)
		}
	}
	for _, src := range []string{
		"case a in 2> ) echo hit;; *) echo miss;; esac",
		"case a in 2>&) echo hit;; *) echo miss;; esac",
	} {
		if out, _ := runDash(t, t.TempDir(), src); out != "miss\n" {
			t.Errorf("%q: %q, want the second arm", src, out)
		}
	}
}
