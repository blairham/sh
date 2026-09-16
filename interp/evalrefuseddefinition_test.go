// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Text `eval` could not read binds nothing — #3194.
//
// `eval` reads its text a line at a time where the dialect says the shell runs
// what it has read, and the hole was in the reader rather than in `eval`: a
// logical line whose parse failed *inside* the construct that began it handed
// back the half-built tree, which was then run. A function definition is the
// shape where that is silent rather than loud — the refused definition bound,
// answered 0 and printed nothing, so a script that re-defines a helper wrongly
// loses the helper with no diagnostic at the point of use.
//
// The empty body is the smallest source that does it, and it is deliberately
// *not* the only one here: an empty body is legal in one dialect's grammar, so
// a test written only that way would be measuring which grammar the runner
// had. The second source will not parse in any of them.
func TestTextEvalCouldNotReadBindsNothing(t *testing.T) {
	for _, tc := range []struct{ name, text string }{
		{"an empty body", "f() {\n}"},
		{"a body that will not parse", "f() {\nif; then\n}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "f() { echo orig; }\neval '" + tc.text + "' 2>/dev/null\necho st=$?\nf\n"
			out, _ := run(t, src, func(r *Runner) {
				s := *r.Semantics
				// The dialects that read borrowed text as they run it; the
				// others parse the whole string first and never reach the
				// question.
				s.EvalRunsWhatItParsed = Yes
				s.BuiltinSyntaxErrorFatal = No
				r.Semantics = &s
			})
			if !strings.Contains(out, "orig") {
				t.Errorf("said %q, want the function that was already there to answer", out)
			}
			if strings.Contains(out, "st=0") {
				t.Errorf("said %q, want a failing status for text that did not read", out)
			}
		})
	}
}

// The lines of the text *before* the refusal still ran, which is the half that
// keeps the rule above from being "a text that fails anywhere runs nothing".
func TestTheLinesBeforeARefusalStillRan(t *testing.T) {
	out, _ := run(t, "eval 'echo one\n{ if; then\n}' 2>/dev/null\necho st=$?\n", func(r *Runner) {
		s := *r.Semantics
		s.EvalRunsWhatItParsed = Yes
		s.BuiltinSyntaxErrorFatal = No
		r.Semantics = &s
	})
	if !strings.Contains(out, "one") {
		t.Errorf("said %q, want the line before the refusal to have run", out)
	}
}
