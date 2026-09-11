// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `function a b` with no body, run rather than only parsed.
//
// Behavioral because what a bodyless declaration *defines* is the question.
// #1686 recorded it as an autoload stub, and it is not one — measured
// 2026-09-10 on zsh 5.9.2, `fpath=(dir); autoload af1; functions af1` prints
// `# undefined` and `builtin autoload -X` and a call reads the file off
// `fpath`, where `fpath=(dir); eval "function af1"; af1` prints nothing at
// status 0 and `functions af1` prints an empty body. So each name is defined
// with an empty body, and a test that only asked "did it parse" could not
// tell the two apart.
func TestABodylessDeclarationDefinesEachNameWithAnEmptyBody(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The declaration is inside an `eval` because at the top level the
		// command after it would be the *body* — which is the other half of
		// the same production and the row below.
		{`eval "function a b"; print -rl -- ${(ko)functions}`, "a\nb"},
		{`eval "function a b"; a; echo "st=$?"`, "st=0"},
		{`eval "function a"; functions a`, "a () {\n\t\n}"},
		// A name list that runs out at a stop word rather than at the end of
		// the input, which is where a real script would write one.
		{`if true; then function a b; fi; print -rl -- ${(ko)functions}`, "a\nb"},
		{`{ function a b; }; print -rl -- ${(ko)functions}`, "a\nb"},
		// And the declaration succeeds, so what follows an `&&` runs.
		{`function a b && echo and`, "and"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// The command after a `;` is the body, not the next statement — which is what
// the bodyless reading would silently turn it into. Both readings run `echo B`
// exactly once, so only *when* it runs says which one happened.
func TestTheCommandAfterTheSeparatorIsTheBody(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Read as a body: nothing prints until `a` is called.
		{`function a; echo B
echo mid
a`, "mid\nB"},
		{`function a b; echo "$0"
a; b`, "a\nb"},
		// A compound after the separator is the body too.
		{`function a; { echo B; }
echo mid
a`, "mid\nB"},
		// And the statement after the body is still a statement.
		{`function a; echo B; echo C
a`, "C\nB"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}
