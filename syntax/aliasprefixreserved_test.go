// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// parseErrorWith runs src through a dialect and gives back what the parser
// complained about, so a row can say which token was named rather than
// reaching into the error's fields.
func parseErrorWith(t *testing.T, d syntax.Dialect, a syntax.Aliases, src string) string {
	t.Helper()
	p := syntax.NewParser(src, d)
	p.Aliases = a
	p.Parse()
	if err := p.Err(); err != nil {
		return err.Error()
	}
	return ""
}

// A reserved word an alias supplied keeps the reading behind an assignment
// prefix where a written-out one loses it — see
// [syntax.Dialect.AliasedReservedWordStandsBehindAnAssignmentPrefix].
//
// The flag decides *which token the complaint names*, which is why every row
// is a pair: the same construct written out, and the same construct reached
// through an alias. Written out, `{` behind a prefix is an ordinary word and
// the list runs on to the `}` that closes nothing; supplied by an alias it is
// the reserved word and the complaint stops at it.
func TestAReservedWordAnAliasSuppliedBehindAnAssignmentPrefix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name         string
		alias, src   string
		written, via string
	}{
		{"a brace group", "{ :; }", "v=x g", "}", "{"},
		{"a while loop", "while :; do :; done", "v=x g", "do", "while"},
		{"an if clause", "if :; then :; fi", "v=x g", "then", "if"},
		{"a case clause", "case a in a) :;; esac", "v=x g", ")", "case"},
		// The control. A `(` is an *operator* rather than a word, so no
		// reading is ever taken away from it and the two spellings agree
		// whatever the flag says.
		{"a subshell, which is an operator", "( : )", "v=x g", "(", "("},
		// And the second control: the reserved word is not the first word
		// of the value, so the prefix is long spent by the time it stands
		// and the flag has nothing to say about it.
		{"a value whose reserved word is not first", "x { :; }", "v=y g", "}", "}"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			on, off := syntax.Core(), syntax.Core()
			on.AliasedReservedWordStandsBehindAnAssignmentPrefix = true
			a := table("g", c.alias)
			// Written out, both answers name the same token: the flag is
			// about the expansion and not about the construct.
			for _, d := range []syntax.Dialect{on, off} {
				got := parseErrorWith(t, d, nil, strings.Replace(c.src, "g", c.alias, 1))
				if !strings.Contains(got, strconv.Quote(c.written)) {
					t.Errorf("written out = %q, want the complaint to name %q", got, c.written)
				}
			}
			if got := parseErrorWith(t, on, a, c.src); !strings.Contains(got, strconv.Quote(c.via)) {
				t.Errorf("through an alias, flag on = %q, want the complaint to name %q", got, c.via)
			}
			// And with the flag off the alias route answers what the
			// written-out spelling does, which is what makes the flag the
			// whole of the difference.
			if got := parseErrorWith(t, off, a, c.src); !strings.Contains(got, strconv.Quote(c.written)) {
				t.Errorf("through an alias, flag off = %q, want the complaint to name %q", got, c.written)
			}
		})
	}
}

// The flag reaches an alias-supplied reserved word and nothing else. A value
// that is an ordinary command still runs behind the prefix, and a reserved
// word with no assignment in front of it is the construct it always was.
func TestTheFlagReachesOnlyAPrefixedAliasedReservedWord(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.AliasedReservedWordStandsBehindAnAssignmentPrefix = true
	for _, c := range []struct{ name, alias, src, want string }{
		{"an ordinary value behind a prefix", "echo hi", "v=x g", "v=x echo hi"},
		{"the same value with no prefix", "{ :; }", "g", "{ :; }"},
		{"a reserved word written out with no prefix", "echo hi", "{ :; }", "{ :; }"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := parsed(t, table("g", c.alias), c.src); got != c.want {
				t.Errorf("%s = %q, want %q", c.src, got, c.want)
			}
		})
	}
}
