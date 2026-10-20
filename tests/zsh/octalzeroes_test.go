// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `setopt octal_zeroes` turns the rest of the panel's arithmetic back on: a
// leading zero is a base again, so `$(( 010 ))` is eight.
//
// The default is this shell's own divergence and is already right — every
// other column reads `010` as eight and this one reads it as ten — so what
// the option buys is the way *back*, for a script written against another
// shell's numbers. Measured 2026-09-17 on zsh 5.9.2, from a script file
// under `env -i PATH=/usr/bin:/bin LC_ALL=C`, with the option set and again
// without it.
//
// It was accepted and inert until #2884, which is the shape that reads as
// working: no diagnostic, and the other shells' number wrong in a place
// where both answers are plausible and file modes are written this way.
func TestOctalZeroesMakesALeadingZeroABase(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The default, and the option asked for and taken back.
		{`echo "$(( 010 )) $(( 0100 ))"`, "10 100\n"},
		{`setopt octal_zeroes; echo "$(( 010 )) $(( 0100 ))"`, "8 64\n"},
		{`setopt octal_zeroes; unsetopt octal_zeroes; echo "$(( 010 ))"`, "10\n"},
		// The underscore is a digit separator here, so the zero it uncovers
		// is a prefix and not a digit of its own.
		{`setopt octal_zeroes; echo "$(( 0_10 ))"`, "8\n"},
		// A prefix is untouched in both directions, which is what says this
		// is the bare leading zero and not radix reading generally.
		{`echo "$(( 0x10 ))"`, "16\n"},
		{`setopt octal_zeroes; echo "$(( 0x10 ))"`, "16\n"},
		// The value a *name* holds goes through the same reader as the
		// literal, so the option reaches text no expression ever held.
		{`setopt octal_zeroes; k=010; echo "$(( k ))"`, "8\n"},
		{`k=010; echo "$(( k ))"`, "10\n"},
		// And so does an assignment to an integer name, and `let` — which
		// renders the eight in the **base it was read in**, because a
		// leading zero is a radix here as much as `0x` is and an integer
		// name carries the base its value named. These three rows read `8`
		// until #3520: the number was right and the rendering was short, and
		// the expectation written here was this shell's answer rather than
		// the measurement. Re-measured 2026-09-17 on zsh 5.9.2 from a script
		// file under `env -i PATH=/usr/bin:/bin LC_ALL=C` — `typeset -p d`
		// lists `typeset -i8 d=8` for the first of them.
		{`setopt octal_zeroes; typeset -i d=010; echo "$d"`, "8#10\n"},
		{`setopt octal_zeroes; let "x=010"; echo "$x"`, "8#10\n"},
		{`setopt octal_zeroes; e=010; typeset -i e; echo "$e"`, "8#10\n"},
		// And the option is what makes it one: with the zero decimal there
		// is no radix to carry, which is the control the three rows above
		// need and did not have.
		{`typeset -i d=010; echo "$d"`, "10\n"},
		// The underscored spelling and the run-together one are one name, as
		// every name in this namespace is.
		{`setopt octalzeroes; echo "$(( 010 ))"`, "8\n"},
		{`setopt octal_zeroes; unsetopt octalzeroes; echo "$(( 010 ))"`, "10\n"},
		// `no` in front of it is the other direction, which is how this
		// namespace spells an unset.
		{`setopt octal_zeroes; setopt nooctalzeroes; echo "$(( 010 ))"`, "10\n"},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The option is read back out of the state it moved rather than out of a bit
// beside it, so the listing and the condition cannot drift from the
// arithmetic — and a subshell's `setopt` stays in the subshell, which is what
// reading an axis buys over storing a flag.
func TestOctalZeroesIsReadBackOffTheArithmetic(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[[ -o octalzeroes ]] && echo on || echo off`, "off\n"},
		{`setopt octal_zeroes; [[ -o octal_zeroes ]] && echo on || echo off`, "on\n"},
		{`setopt octal_zeroes; case "$(setopt)" in *octalzeroes*) echo listed ;; *) echo missing ;; esac`, "listed\n"},
		{`case "$(setopt)" in *octalzeroes*) echo listed ;; *) echo missing ;; esac`, "missing\n"},
		{`(setopt octal_zeroes; echo "in $(( 010 ))"); echo "out $(( 010 ))"`, "in 8\nout 10\n"},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}
