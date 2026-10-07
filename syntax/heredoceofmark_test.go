// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// An end of input marked inside the text ends the here-document body that
// reaches it and nothing else: the next document on the line reads on from
// there, and the remark for each is numbered by the end that ended it. The
// `at line N` numbers are bash 5.3.20's at a terminal, measured 2026-10-07
// with ^D where each mark stands (#6287).
func TestAnEndOfInputMarkEndsOneHeredocBody(t *testing.T) {
	type remark struct {
		token      string
		at         int32
		endOfInput int
	}
	for _, c := range []struct {
		name       string
		src        string
		marks      []string // each end of input falls after the text its mark spells
		taken      int
		incomplete bool
		bodies     []string
		remarks    []remark
	}{
		{
			name: "the second document is still open", src: "cat <<E1 <<E2\na\n",
			marks: []string{"cat <<E1 <<E2\na\n"}, taken: 1, incomplete: true,
			bodies:  []string{"a\n", ""},
			remarks: []remark{{"E1", 1, 1}, {"E2", 2, 0}},
		},
		{
			name: "the second document closes", src: "cat <<E1 <<E2\na\nb\nE2\n",
			marks: []string{"cat <<E1 <<E2\na\n"}, taken: 1,
			bodies:  []string{"a\n", "b\n"},
			remarks: []remark{{"E1", 1, 1}},
		},
		{
			name: "two ends at one place", src: "cat <<E1 <<E2\na\n",
			marks: []string{"cat <<E1 <<E2\na\n", "cat <<E1 <<E2\na\n"}, taken: 2,
			bodies:  []string{"a\n", ""},
			remarks: []remark{{"E1", 1, 1}, {"E2", 2, 2}},
		},
		{
			name: "three documents, numbered where each body began", src: "true\ncat <<E1 <<E2 <<E3\na\nx\nb\n",
			marks: []string{"true\ncat <<E1 <<E2 <<E3\na\nx\n", "true\ncat <<E1 <<E2 <<E3\na\nx\nb\n"}, taken: 2, incomplete: true,
			bodies:  []string{"a\nx\n", "b\n", ""},
			remarks: []remark{{"E1", 2, 1}, {"E2", 4, 2}, {"E3", 5, 0}},
		},
		{
			name: "an end no body reaches ends nothing", src: "echo a\n",
			marks: []string{"echo a\n"}, taken: 0,
		},
		{
			name: "a document closed before the end keeps its delimiter", src: "cat <<E1 <<E2\nE1\nb\n",
			marks: []string{"cat <<E1 <<E2\nE1\nb\n"}, taken: 1,
			bodies:  []string{"", "b\n"},
			remarks: []remark{{"E2", 2, 1}},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var ends []int
			for _, m := range c.marks {
				if !strings.HasPrefix(c.src, m) {
					t.Fatalf("mark %q is not a prefix of the text", m)
				}
				ends = append(ends, len(m))
			}
			p := syntax.NewParser(c.src, syntax.Core())
			p.EndsOfInputAt(ends)
			f := p.Parse()
			if got := p.EndsOfInputTaken(); got != c.taken {
				t.Errorf("ends taken %d, want %d", got, c.taken)
			}
			if got := p.Incomplete(); got != c.incomplete {
				t.Errorf("incomplete %v, want %v", got, c.incomplete)
			}
			var got []remark
			for _, r := range p.Remarks() {
				if r.Kind == syntax.RemarkHeredocAtEOF {
					got = append(got, remark{r.Token, r.At.Line, r.EndOfInput})
				}
			}
			if len(got) != len(c.remarks) {
				t.Fatalf("remarks %v, want %v", got, c.remarks)
			}
			for i := range got {
				if got[i] != c.remarks[i] {
					t.Errorf("remark %d is %v, want %v", i, got[i], c.remarks[i])
				}
			}
			if c.bodies == nil {
				return
			}
			var bodies []string
			for _, st := range f.Stmts {
				pl, ok := st.Expr.(*syntax.Pipeline)
				if !ok {
					continue
				}
				for _, cmd := range pl.Cmds {
					if sc, ok := cmd.(*syntax.SimpleCmd); ok {
						for _, rd := range sc.Redirs {
							if rd.Heredoc != nil {
								bodies = append(bodies, rd.Heredoc.Literal())
							}
						}
					}
				}
			}
			if strings.Join(bodies, "|") != strings.Join(c.bodies, "|") {
				t.Errorf("bodies %q, want %q", bodies, c.bodies)
			}
		})
	}
}
