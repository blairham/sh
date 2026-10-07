// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"fmt"
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

// An end of input inside a here-document in the body of a command or process
// substitution ends that document too, and the file carries it beside the
// substitution so that the body, read again when it runs, ends there as well.
// Measured
// 2026-10-07 at a terminal, bash 5.3.20: `v=$(cat <<E1`, `a`, ^D, `echo
// still`, `)` leaves `a` and `still` in v (#6309).
func TestAnEndOfInputMarkInsideASubstitution(t *testing.T) {
	for _, c := range []struct {
		name, src, mark string
		incomplete      bool
		ends            []int // the span's, into its Value
		bodies          string
		refused         bool // the body, read again, refuses a line after the document
	}{
		{
			// The first word holds the body, so it is read when the parser
			// is made — before it is told about the end of input.
			name: "the first word", src: "$(cat <<E1\na\necho still\n)\n", mark: "$(cat <<E1\na\n",
			ends: []int{11}, bodies: "a\n",
		},
		{
			name: "an argument", src: "echo $(cat <<E1\na\necho still\n)\n", mark: "echo $(cat <<E1\na\n",
			ends: []int{11}, bodies: "a\n",
		},
		{
			// A `)` the body's grammar spends is not the closer, which only a
			// read that knows where the document ended can tell: counting the
			// parentheses would close the substitution at the pattern's.
			name: "a case pattern after the document", src: "echo $(cat <<E1\na\ncase x in x) echo y;; esac\n)\n",
			mark: "echo $(cat <<E1\na\n", ends: []int{11}, bodies: "a\n",
		},
		{
			name: "a process substitution", src: "cat <(cat <<E1\na\n)\n", mark: "cat <(cat <<E1\na\n",
			ends: []int{11}, bodies: "a\n",
		},
		{
			// The read of the line refuses `fi` and the parentheses are
			// counted instead, which is a second way to the span: it carries
			// the end all the same, so the body read again refuses the line
			// rather than taking `fi` into the document.
			name: "a body the read of the line refused", src: "echo $(cat <<E1\na\nfi\n)\n", mark: "echo $(cat <<E1\na\n",
			ends: []int{11}, refused: true,
		},
		{
			name: "still open", src: "echo $(cat <<E1\na\n", mark: "echo $(cat <<E1\na\n",
			incomplete: true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser(c.src, syntax.Core())
			p.EndsOfInputAt([]int{len(c.mark)})
			f := p.Parse()
			if got := p.EndsOfInputTaken(); got != 1 {
				t.Errorf("ends taken %d, want 1", got)
			}
			if got := p.Incomplete(); got != c.incomplete {
				t.Fatalf("incomplete %v, want %v (error %v)", got, c.incomplete, p.Err())
			}
			var rk []int
			for _, r := range p.Remarks() {
				if r.Kind == syntax.RemarkHeredocAtEOF {
					rk = append(rk, r.EndOfInput)
				}
			}
			if len(rk) != 1 || rk[0] != 1 {
				t.Errorf("remarks numbered %v, want [1]", rk)
			}
			if c.incomplete {
				return
			}
			sc := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
			var span *syntax.Span
			for _, w := range sc.Args {
				for i := range w.Spans {
					if w.Spans[i].Kind == syntax.CommandSubst || w.Spans[i].Kind == syntax.ProcSubstIn {
						span = &w.Spans[i]
					}
				}
			}
			if span == nil {
				t.Fatal("no substitution")
			}
			var ends []int
			for _, e := range f.EndsOfInput {
				if e.At == span.Pos {
					ends = e.Offsets
				}
			}
			if fmt.Sprint(ends) != fmt.Sprint(c.ends) {
				t.Errorf("the file carries %v for the substitution, want %v", ends, c.ends)
			}
			// And the body read again with them ends its document there.
			body := syntax.NewParser(span.Value, syntax.Core())
			body.EndsOfInputAt(ends)
			bf := body.Parse()
			if c.refused {
				if body.Err() == nil {
					t.Error("the body read again took `fi` into its document")
				}
				return
			}
			if body.Err() != nil {
				t.Fatalf("the body read again: %v", body.Err())
			}
			got := bf.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd).Redirs[0].Heredoc.Literal()
			if got != c.bodies {
				t.Errorf("the body's document is %q, want %q", got, c.bodies)
			}
		})
	}
}
