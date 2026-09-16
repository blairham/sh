// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A word behind the count of `break`, `continue`, `return`, `exit` or
// `shift`, which bash refuses and which this shell took as silence (#2298).
//
// Measured 2026-09-16 against GNU bash 5.3.20 in a script file, each line
// followed by `echo "A=$?"` on a line of its own. The refusal costs the rest
// of the *statement* — a loop around it stops where it stands and a `; echo`
// behind it never runs — and the script carries on at the next line at 2.
// bash 3.2.57 writes the same sentence at status 1.
func TestAWordBehindANumericOperandIsTooManyArguments(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"shift", "set -- a b c\nshift 1 2\necho \"A=$? n=$#\"\n",
			"bash: line 2: shift: too many arguments\nA=2 n=3\n",
		},
		{
			"the shift did not happen and neither did the rest of the line",
			"set -- a b c\nshift 1 2; echo SAME\necho TAIL\n",
			"bash: line 2: shift: too many arguments\nTAIL\n",
		},
		{
			"a loop around it stops",
			"set -- a b c\nfor i in 1 2; do shift 1 2; echo IN; done\necho TAIL\n",
			"bash: line 2: shift: too many arguments\nTAIL\n",
		},
		{
			"exit", "exit 1 2\necho \"A=$?\"\n",
			"bash: line 1: exit: too many arguments\nA=2\n",
		},
		{
			"three operands are two too many", "exit 1 2 3\necho \"A=$?\"\n",
			"bash: line 1: exit: too many arguments\nA=2\n",
		},
		{
			"return", "f() { return 1 2; echo BODY; }\nf\necho \"A=$?\"\n",
			"bash: line 1: return: too many arguments\nA=2\n",
		},
		{
			// The count of words is judged before the place, which is the
			// opposite order from `break` below — measured, and the reason
			// the two are separate rows.
			"return with nowhere to return to", "return 1 2\necho \"A=$?\"\n",
			"bash: line 1: return: too many arguments\nA=2\n",
		},
		{
			"break", "for i in 1 2; do break 1 2; echo IN; done\necho \"A=$?\"\n",
			"bash: line 1: break: too many arguments\nA=2\n",
		},
		{
			"continue", "for i in 1 2; do continue 1 2; echo IN; done\necho \"A=$?\"\n",
			"bash: line 1: continue: too many arguments\nA=2\n",
		},
		{
			// bash judges a `break`'s place first, so outside a loop the
			// second word is never counted.
			"break outside a loop is the place's complaint", "break 1 2\necho \"A=$?\"\n",
			"bash: line 1: break: only meaningful in a `for', `while', or `until' loop\nA=0\n",
		},
		{
			// The marker is taken first, so what is behind it is a count and
			// a word too many.
			"past the end-of-options marker", "for i in 1 2; do break -- 1 2; done\necho \"A=$?\"\n",
			"bash: line 1: break: too many arguments\nA=2\n",
		},
		{
			// A count that will not read speaks first: the operand is read
			// before the words behind it are counted.
			"an unreadable count speaks first", "set -- a b\nshift abc def\necho \"A=$?\"\n",
			"bash: line 2: shift: abc: numeric argument required\nA=2\n",
		},
		{
			// And it wins over both ends of the range, which a reading that
			// counted the words last would get wrong.
			"a count past the end is still one word too many",
			"set -- a b c\nshift 5 2\necho \"A=$? n=$#\"\n",
			"bash: line 2: shift: too many arguments\nA=2 n=3\n",
		},
		{
			// The marker is consumed, so what is behind it is one operand
			// and not two — the row the first writing of this got wrong.
			"a taken marker is not an operand", "for i in 1 2; do break -- 1; echo IN; done\necho \"A=$?\"\n",
			"A=0\n",
		},
		{
			"one operand is the ordinary reading", "set -- a b c\nshift 2\necho \"A=$? n=$#\"\n",
			"A=0 n=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
