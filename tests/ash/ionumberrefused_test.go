// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/syntax"
)

// A refused IO number is a redirection, and is worded as one rather than
// named `IO number` (#6552). Measured 2026-10-15: `case 2>x in x) esac` is
// `redirection unexpected (expecting word)` on dash 0.5.12 and
// `unexpected redirection (expecting word)` on BusyBox ash 1.37.0.
func TestARefusedIONumberIsARedirection(t *testing.T) {
	_, err := syntax.Parse("case 2>x in x) esac\n", ash.Dialect())
	if err == nil {
		t.Fatal("parsed, want a refusal")
	}
	got := ash.Diagnostics().ParseFailure(err)
	if want := "syntax error: unexpected redirection (expecting word)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Where a `case` pattern list starts, an IO number is part of the one
// operator this shell lets stand there, so the refusal is of the word after
// it, and the operator alone is an arm (#6617). Measured 2026-10-15 on
// BusyBox ash 1.37.0 in the pinned alpine image; the rows are on
// syntax.Parser.casePatterns. Several digits make one number here, where
// dash reads only one.
func TestAnIONumberStartingAPatternIsPartOfTheOperator(t *testing.T) {
	const word = `syntax error: unexpected word (expecting ")")`
	for _, c := range []struct{ src, want string }{
		{"case x in 2>x) esac\n", word},
		{"case x in (2>x) esac\n", word},
		{"case x in a|2>x) esac\n", word},
		{"case x in 22>x) esac\n", word},
		{"case x in a 2>x) esac\n", `syntax error: unexpected redirection (expecting ")")`},
	} {
		_, err := syntax.Parse(c.src, ash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", c.src)
			continue
		}
		if got := ash.Diagnostics().ParseFailure(err); got != c.want {
			t.Errorf("%q: got %q, want %q", c.src, got, c.want)
		}
	}
	for _, src := range []string{
		"case a in 2> ) echo hit;; *) echo miss;; esac\n",
		"case a in 22> ) echo hit;; *) echo miss;; esac\n",
	} {
		if _, err := syntax.Parse(src, ash.Dialect()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}
