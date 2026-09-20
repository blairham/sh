// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package printer_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/fmt/comments"
	"github.com/blairham/sh/internal/fmt/printer"
	"github.com/blairham/sh/syntax"
)

// A here-document fed from the lines after the enclosing command survives
// being laid out.
//
// The formatter's first promise is the tree, and these bodies are the one
// thing in a file that is on **no node of it**: the sub-parse that found the
// operators is thrown away, so the walk in `verbatim` and the queue in
// `redirect` both reach nothing, and the walk is how every other body is
// found. Before this they came back as a blank line — the body and its
// delimiter deleted from a program that then meant something else. See
// syntax.Dialect.HeredocBodyFromAfterTheCommand (#3711).
//
// The control is the last row: the same document closed inside the
// parentheses, which is on a node and always was.
func TestABodyFedFromAfterTheCommandSurvivesTheLayout(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{
			"the newer spelling",
			"echo one\necho $(cat <<EOF)\nbody\nEOF\necho after\n",
		},
		{
			"the process substitution",
			"echo one\ncat <(cat <<EOF)\nbody\nEOF\necho after\n",
		},
		{
			"two documents, in order",
			"echo $(cat <<A; cat <<B)\nbody-one\nA\nbody-two\nB\necho after\n",
		},
		{
			// Two commands hold this one — the `if` and the `echo` — and an
			// extent match is not exclusive, so it was written twice.
			"inside a compound on one line",
			"if true; then echo $(cat <<EOF); fi\nbody\nEOF\necho after\n",
		},
		{
			// And where the compound spans lines, the outer command would
			// have flushed it after the *header* rather than after the line
			// the substitution is on.
			"inside a compound over several lines",
			"if true; then\n  echo $(cat <<EOF)\nbody\nEOF\n  echo inner\nfi\necho after\n",
		},
		{
			// The control: on a node, and reached by the ordinary walk.
			"a document that closes inside the parentheses",
			"v=$(cat <<EOF\ninside\nEOF\n)\necho \"[$v]\"\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, bash.Dialect())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			got := printer.Format(tc.src, f, comments.Recover(tc.src, f), bash.Style())
			if got != tc.src {
				t.Errorf("laid out %q as %q, want it unchanged", tc.src, got)
			}
		})
	}
}

// And the canonicalizer, which is a different printer with the same hole.
func TestABodyFedFromAfterTheCommandSurvivesTheCanonicalizer(t *testing.T) {
	src := "echo one\necho $(cat <<EOF)\nbody\nEOF\necho after\n"
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	got := syntax.Print(f)
	again, err := syntax.Parse(got, bash.Dialect())
	if err != nil {
		t.Fatalf("reparse %q: %v", got, err)
	}
	if len(again.CarriedHeredocs) != 1 {
		t.Fatalf("printed %q, which carries %d documents rather than 1",
			got, len(again.CarriedHeredocs))
	}
	body := again.CarriedHeredocs[0].Redirs[0].Heredoc
	if body == nil || body.Literal() != "body\n" {
		t.Errorf("printed %q, whose body is %v rather than `body`", got, body)
	}
}
