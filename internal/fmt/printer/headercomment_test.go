// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package printer_test

import (
	"strings"
	"testing"
)

// A comment written between a declaration's header and its body is kept in
// front of the declaration, not moved past the whole of it.
//
// The header is squeezed onto one line, so the comment cannot stay where it
// stood. It used to be left queued and the next thing to flush queued
// comments is whatever follows the *declaration*, so a comment that described
// a function came out describing the line after it — with a blank line where
// it had been (#3759). Nothing runs either way, so `SameProgram` held and the
// output was a fixed point throughout: what was lost is the one thing a
// formatter promises about a comment besides keeping it, which is where the
// author put it.
func TestAHeaderCommentStaysInFrontOfItsDeclaration(t *testing.T) {
	for _, tc := range []struct {
		name string
		zsh  bool
		src  string
		want string
	}{
		{
			name: "before a brace body",
			src:  "foo()\n# what it does\n{ echo hi; }\necho after\n",
			want: "# what it does\nfoo() { echo hi; }\necho after\n",
		},
		{
			name: "trailing on the header's own line",
			zsh:  true,
			src:  "function foo # trailing\n{ echo hi; }\n",
			want: "# trailing\nfunction foo { echo hi; }\n",
		},
		{
			name: "two of them, in the order they were written",
			src:  "foo() # one\n# two\n{ echo hi; }\necho after\n",
			want: "# one\n# two\nfoo() { echo hi; }\necho after\n",
		},
		{
			// A body already on several lines took the comment in at the
			// top and gave it a blank line of its own, because the gap was
			// measured across the header line the squeeze had consumed.
			name: "a body already spread over lines",
			src:  "foo() # trail\n{\n  echo hi\n}\necho after\n",
			want: "# trail\nfoo() {\n  echo hi\n}\necho after\n",
		},
		{
			// A body with no braces to put a comment inside, which is why
			// the comment goes in front of the declaration rather than at
			// the top of the body the way a loop header's does.
			name: "a body that is not a group",
			zsh:  true,
			src:  "function foo\n# why\nfor i in a; do echo $i; done\necho after\n",
			want: "# why\nfunction foo\nfor i in a; do echo $i; done\necho after\n",
		},
		{
			name: "a subshell body",
			src:  "foo()\n# why\n( echo hi )\necho after\n",
			want: "# why\nfoo() (echo hi)\necho after\n",
		},
		{
			name: "indented inside another construct",
			src:  "if true; then\n  foo()\n  # nested\n  { echo hi; }\nfi\n",
			want: "if true; then\n  # nested\n  foo() { echo hi; }\nfi\n",
		},
		{
			// The control: a comment inside the *body* was never the
			// problem and must not move.
			name: "a comment inside the body stays inside it",
			src:  "foo() {\n# inside\necho hi\n}\n",
			want: "foo() {\n  # inside\n  echo hi\n}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, st := dialect(tc.zsh), style(tc.zsh)
			out := format(t, tc.src, d, st)
			if out != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", out, tc.want)
			}
			// And it is a fixed point, which is what says the comment has
			// been put somewhere the next pass reads as outside the header.
			if again := format(t, out, d, st); again != out {
				t.Errorf("not a fixed point:\n%s\nbecame:\n%s", out, again)
			}
			if strings.Contains(out, "\n\n") {
				t.Errorf("a blank line appeared where the source had none:\n%s", out)
			}
		})
	}
}

// A declaration written after a `;` on a line with other statements keeps the
// old placement, because a comment emitted mid-line would comment out the
// rest of it.
//
// Its own test rather than a row above: the point is that the fix declines
// rather than that it applies, and a row that asserted the same output as the
// others would be asserting the opposite thing.
func TestAMidLineDeclarationLeavesItsHeaderCommentQueued(t *testing.T) {
	d, st := dialect(false), style(false)
	src := "true; foo()\n# mid\n{ echo hi; }\necho after\n"
	out := format(t, src, d, st)
	if !strings.HasPrefix(out, "true; foo() { echo hi; }\n") {
		t.Fatalf("the run was broken up:\n%s", out)
	}
	if !strings.Contains(out, "# mid") {
		t.Errorf("the comment was lost:\n%s", out)
	}
	if again := format(t, out, d, st); again != out {
		t.Errorf("not a fixed point:\n%s\nbecame:\n%s", out, again)
	}
}
