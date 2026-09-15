// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// The float letter, which this shell spells `-F[n]` in its own usage line.
// Measured on ksh93u+ 2012-08-01, 2026-09-14, `env -i PATH=/usr/bin:/bin`
// with a scratch HOME, over `-c`, a script file and standard input alike
// (#2419).
func TestTheFloatLetterIsAPrecision(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The number is the letter's argument and not a second name, under
		// both spellings, and the value is an arithmetic expression.
		{`typeset -F 3 a=3.14159; echo "[$a]"`, "[3.142]\n"},
		{`typeset -F3 b=3.14159; echo "[$b]"`, "[3.142]\n"},
		{`typeset -F c=3.14159; echo "[$c]"`, "[3.1415900000]\n"},
		{`typeset -F 3 d=1+2; echo "[$d]"`, "[3.000]\n"},
		{`typeset -F 3 e=abc; echo "[$e]"`, "[0.000]\n"},
		// The precision is *rendering*: the characters are what the name
		// holds, so its length is five and a later append adds numbers.
		{`typeset -F 3 a=1.5; echo "[${#a}]"`, "[5]\n"},
		{`typeset -F 3 a=1.5; a+=2.25; echo "[$a]"`, "[3.750]\n"},
		// The listing writes the precision back as a word of its own, which
		// is this shell's arrangement for the integer base too.
		{`typeset -F 3 a=1.5; typeset -p a`, "typeset -F 3 a=1.500\n"},
		{`typeset -xF 3 a=1.5; typeset -p a`, "typeset -x -F 3 a=1.500\n"},
		// The plus form takes the attribute off and leaves the text it
		// rendered standing, so the name lists as a plain assignment.
		{`typeset -F 3 f=1.5; typeset +F f; echo "[$f]"; typeset -p f`, "[1.500]\nf=1.500\n"},
		// A bare letter over a name that already has a precision resets it
		// to the default here, where zsh keeps it.
		{`typeset -F 3 e=1.5; typeset -F e; echo "[$e]"`, "[1.5000000000]\n"},
		// A word that is not digits was never the argument: `abc` is a
		// second name, and what it holds is nothing here.
		{`typeset -F abc a=1; echo "[$a][$abc]"`, "[1.0000000000][]\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The detached number is read only where the letter already ends its word,
// which is what `-F[n]` means: a letter written behind it leaves the number an
// operand, and the operand is then a name that is not one.
func TestADetachedPrecisionNeedsTheLetterLast(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The refusal, and its wording: the export letter brings `export`'s
		// own sentence with it while the builtin still names itself.
		{`typeset -Fx 3 a=1.5`, "ksh: typeset: 3: is not an identifier\n"},
		{`typeset -Fr 3 c=1.5`, "ksh: typeset: 3: invalid variable name\n"},
		// And one number satisfies the letter, so a second is an operand
		// again — the same refusal from the other side.
		{`typeset -F 3 4 b=1.5`, "ksh: typeset: 4: invalid variable name\n"},
	} {
		out, st := kshOut(t, c.src)
		if out != c.want || st == 0 {
			t.Errorf("%s\n got %q at %d\nwant %q non-zero", c.src, out, st, c.want)
		}
	}
}

// The three numeric type letters have a fixed rank here — `E` over `F` over
// `i` — rather than the order they were written in, and the *number* the
// word's last letter read goes to whichever of them wins. The one pair this
// shell will not take at all is `-i` with `-E`, which is its usage block.
func TestTheNumericLettersHaveARankAndTheNumberFollowsIt(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -iF 3 a=1.5; typeset -p a`, "typeset -F 3 a=1.500\n"},
		{`typeset -Fi 3 a=1.5; typeset -p a`, "typeset -F 3 a=1.500\n"},
		{`typeset -i -F 3 a=1.5; typeset -p a`, "typeset -F 3 a=1.500\n"},
		{`typeset -F -i a=1.5; typeset -p a`, "typeset -F a=1.5000000000\n"},
		// The number goes with the attribute whichever letter read it: here
		// the `i` read the 3 as a base and the `F` renders three places, and
		// below the `F` read the 16 as a precision.
		{`typeset -iF 16 a=255; typeset -p a`, "typeset -F 16 a=255.0000000000000000\n"},
		// And the exponent letter outranks the plain one from either side.
		{`typeset -EF 3 a=1.5; typeset -p a`, "typeset -E 3 a=1.5\n"},
		{`typeset -FE 3 a=1.5; typeset -p a`, "typeset -E 3 a=1.5\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
	// The pair that is refused rather than ranked, both orders, with the
	// usage block and nothing else.
	for _, src := range []string{`typeset -iE 3 a=1.5`, `typeset -Ei 3 a=1.5`} {
		out, st := kshOut(t, src)
		if st == 0 || !strings.HasPrefix(out, "Usage: typeset") {
			t.Errorf("%s\n got %q at %d\nwant the usage block non-zero", src, out, st)
		}
	}
}

// The `-h` letter takes a string this shell records nowhere, attached or
// detached, and it is neither zsh's hide-in-scope nor anything this engine
// does. Measured at the same time.
func TestTheHideStringLetterTakesItsWordAndRecordsNothing(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -h "a string" q=1; typeset -p q`, "q=1\n"},
		{`typeset -h s q=1 r=2; typeset -p q r`, "q=1\nr=2\n"},
		{`typeset -h"s" q=1; typeset -p q`, "q=1\n"},
		{`typeset -hx "s" q=1; typeset -p q`, "q=1\n"},
		// The letter beside `-H` leaves the hiding attribute standing, which
		// is what says the two letters are different questions.
		{`typeset -Hh s q=1; typeset -p q`, "typeset -H q=1\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}
