// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// What a listing writes *between* two statements of a re-read `$( … )` body —
// #3830, swept out of #3801 once 33 of its 35 probes agreed.
//
// The body is not laid out one statement to a line, so something has to say
// where the break goes, and the two candidates part company wherever a `;`
// and a line break are both written. Measured 2026-09-19 on bash 5.3.20,
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// through `declare -f`: the separator is the **terminator that was written**
// and not the line the statement was left on. `$(a;` newline `b)` comes back
// on one line and `$(a` newline `b)` keeps its two.
//
// Every `want` below is that shell's own output, byte for byte.
func TestASubstitutionsBodyIsSeparatedByItsTerminator(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// **The control**, and the only row here a "join them all"
			// answer would fail: a newline on its own stays a newline. It is
			// what says the rule is the token rather than a normalization.
			name: "a newline alone keeps the line break",
			src:  "f() { echo $(a\nb); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(a\nb)\n}\n",
		},
		{
			// The second control, from the other side: a `;` on its own
			// joins, which is what this shell already did and what a
			// "follow the source's lines" answer also gives.
			name: "a semicolon alone joins",
			src:  "f() { echo $(a;b); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(a; b)\n}\n",
		},
		{
			// The row the issue is named for: both are written, and the
			// terminator wins.
			name: "a semicolon and a newline",
			src:  "f() { echo $(a;\nb); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(a; b)\n}\n",
		},
		{
			// A comment between two statements is the same shape reached a
			// second way, and it is the reason the tree cannot answer this
			// from what it already held: a comment is not in the tree, so
			// what is left is a `;`, a line break, and nothing to tell the
			// printer which of them ended the statement.
			name: "a comment between the two",
			src:  "f() { echo $(a; # c\nb); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(a; b)\n}\n",
		},
		{
			// And a gap of any size is still the `;` that terminated.
			name: "a blank line between the two",
			src:  "f() { echo $(a;\n\nb); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(a; b)\n}\n",
		},
		{
			// `&` is a terminator too, and it terminates wherever the next
			// statement was written — which the line rule got wrong in the
			// same place, since `a &` and a newline is two lines to it.
			name: "an ampersand and a newline",
			src:  "f() { echo $(a &\nb); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(a & b)\n}\n",
		},
		{
			// Both answers in one row, which is what says neither is being
			// applied to the whole list: the first pair keeps its line and
			// the second joins.
			name: "a newline then a semicolon",
			src:  "f() { echo $(a\nb;\nc); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(a\nb; c)\n}\n",
		},
		{
			name: "inside a brace group",
			src:  "f() { echo $({ a;\nb; }); }\ndeclare -f f",
			want: "f () \n{ \n    echo $({ a; b; })\n}\n",
		},
		{
			// The blank after the `$(` is the reprint's, as it is in the
			// neighboring suite: without it this is `$((`, which is
			// arithmetic and a different program.
			name: "inside a subshell",
			src:  "f() { echo $( (a;\nb) ); }\ndeclare -f f",
			want: "f () \n{ \n    echo $( ( a; b ))\n}\n",
		},
		{
			// A `case` arm's own list is the same list, so the rule reaches
			// it — and the arm is laid out around it, which is the
			// arrangement the body already had.
			name: "inside a case arm",
			src:  "f() { echo $(case x in y) a;\nb;; esac); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(case x in y)\n        a; b\n    ;;\nesac)\n}\n",
		},
		{
			// A substitution inside a substitution reprints with the same
			// rule, which is what says it travels with the arrangement.
			name: "inside a nested substitution",
			src:  "f() { echo $(x $(a;\nb)); }\ndeclare -f f",
			want: "f () \n{ \n    echo $(x $(a; b))\n}\n",
		},
		{
			// **The third control**, and the one that keeps this from being
			// read as a rule about listings generally: the body of the
			// function *itself* is laid out one statement to a line, and a
			// `;` written between two of its statements does not join them.
			// Only the arrangement inside the parentheses follows the
			// source, so only it asks this question.
			name: "the listed body itself is unaffected",
			src:  "f() { a;\nb; }\ndeclare -f f",
			want: "f () \n{ \n    a;\n    b\n}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if st != 0 || out != tc.want {
				t.Errorf("out %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}
