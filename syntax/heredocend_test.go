// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A here-document whose delimiter never arrives is unfinished input and not
// wrong input: every shell in the panel takes the body as everything to the
// end and runs the command.
//
// It is *both* things at once, and that is the point. To a script the input
// has ended, so the body is what there is; at a prompt the input has not
// ended, so there is another line to ask for. The parser reports both and
// each front end reads the one it needs.
func TestAHereDocumentCanRunToTheEndOfInput(t *testing.T) {
	for _, tc := range []struct{ name, src, wantBody string }{
		{
			// The terminator has a leading space, so it is not the
			// delimiter — the shape /usr/local/bin/prlcopy has.
			"a delimiter that never matches",
			"cat <<EOF\na)\n EOF\n", "a)\n EOF\n",
		},
		{
			"nothing after the operator at all",
			"cat <<EOF\n", "",
		},
		{
			"a delimiter that does arrive",
			"cat <<EOF\nbody\nEOF\n", "body\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := syntax.NewParser(tc.src, syntax.Core())
			f := p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			sc := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
			if len(sc.Redirs) != 1 || sc.Redirs[0].Heredoc == nil {
				t.Fatalf("no here-document in %q", tc.src)
			}
			if got := sc.Redirs[0].Heredoc.Literal(); got != tc.wantBody {
				t.Errorf("body = %q, want %q", got, tc.wantBody)
			}
		})
	}
}

// And it says there may be more, which is what a prompt needs: a
// here-document still open when the line ends asks for another line rather
// than running with what it has.
func TestAnOpenHereDocumentIsIncomplete(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      bool
	}{
		{"still open", "cat <<EOF\nbody\n", true},
		{"closed", "cat <<EOF\nbody\nEOF\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := syntax.NewParser(tc.src, syntax.Core())
			p.Parse()
			if got := p.Incomplete(); got != tc.want {
				t.Errorf("Incomplete = %v, want %v", got, tc.want)
			}
			// Unfinished either way, and never *wrong*: a script takes what
			// there is rather than refusing it.
			if err := p.Err(); err != nil {
				t.Errorf("reported as an error: %v", err)
			}
		})
	}
}
