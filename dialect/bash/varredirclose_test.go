// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `shopt -s varredir_close` takes back the descriptor a `{name}>file`
// redirection picked when the command carrying it ends, instead of leaving it
// open for the rest of the shell.
//
// Measured 2026-09-22 on bash 5.3.20, `env -i PATH=/usr/bin:/bin LC_ALL=C`,
// from a script file. With the option on, `echo hi {a}>/dev/null` leaves
// `$a` holding 11 and a later `cat <&$a` is `$a: Bad file descriptor` at 1,
// and the next redirection to pick a number gets 11 back. `exec` is outside
// it — `exec {e}</dev/null` then `cat <&$e` still reads — because there is no
// command there for the descriptor to outlive.
//
// It sat in shoptStates refusing the write, and the refusal was the honest
// kind while nothing implemented it: a shell that granted the name and went
// on leaving descriptors open would be the silent wrong answer, since the
// whole observable is whether a later `<&$fd` finds anything.
func TestVarredirCloseTakesThePickedDescriptorBack(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The default: the descriptor outlives its command, so the second
		// redirection has to pick a number above the first.
		{`echo one {a}>/dev/null; echo two {b}>/dev/null; echo "$a $b"`, "one\ntwo\n10 11\n"},
		// And with the option, the number comes straight back.
		{
			`shopt -s varredir_close
echo one {a}>/dev/null
echo two {b}>/dev/null
echo "$a $b"`,
			"one\ntwo\n10 10\n",
		},
		// The variable still holds the number it was given; what is gone is
		// the descriptor.
		{
			`shopt -s varredir_close; echo one {a}>/dev/null; echo "held=$a"`,
			"one\nheld=10\n",
		},
		// `exec` is outside it: nothing ends, so nothing is taken back.
		{
			`shopt -s varredir_close
exec {e}</dev/null
exec {f}</dev/null
echo "$e $f"`,
			"10 11\n",
		},
		// And the way back.
		{
			`shopt -s varredir_close; shopt -u varredir_close
echo one {a}>/dev/null
echo two {b}>/dev/null
echo "$a $b"`,
			"one\ntwo\n10 11\n",
		},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The name is listed with the rest, on and off, which is the half a script
// reads back — and the half that was wrong before: a name refused by the
// builtin still listed as `off`, so the listing and the write disagreed.
func TestVarredirCloseIsListed(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`shopt varredir_close`, "varredir_close      \toff\n"},
		{`shopt -s varredir_close; shopt varredir_close`, "varredir_close      \ton\n"},
		{`shopt -q varredir_close; echo "st=$?"`, "st=1\n"},
		{`shopt -s varredir_close; shopt -q varredir_close; echo "st=$?"`, "st=0\n"},
	} {
		out, _ := runBash(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}
