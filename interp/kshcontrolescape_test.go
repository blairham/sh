// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// A `~(K)` group makes a written `\n` the **newline**, and seven escapes
// beside it likewise: the control characters they name rather than the
// letters they are spelled with.
//
// A family beside the six classes rather than six more of them, because the
// shape is different — a class names a *set* and takes whatever unit is in
// it, and one of these names exactly one character. The rows are ksh93u+
// 2012-08-01's, measured 2026-09-28 from a script file under
// `env -i PATH=/usr/bin:/bin` with a scratch `HOME`; nothing here names a
// shell, because the construct is a grammar flag and the letters are that
// construct's.
//
// **Every letter is a pair**, because a control reading and a letter reading
// agree on half of all subjects: a row asserting only that `~(K)a\nb` fails
// to match `zanb` would pass for a pattern that matched nothing at all.
func TestATildeGlobGroupReadsTheControlEscapes(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The controls. With no group the escape is the shell's, so `\n` is
		// the letter `n` — and that pair is what says the rows below it are
		// the group's doing rather than this shell's quote removal.
		{"no group, the letter", `[[ zanb == za\nb ]] && echo YES || echo NO`, "YES"},
		{"no group, a newline", `[[ $'za\nb' == za\nb ]] && echo YES || echo NO`, "NO"},

		// And the escape is not matching nothing or everything: with no
		// character between the `a` and the `b`, neither reading matches.
		{"nothing between", `[[ zab == z~(K)a\nb ]] && echo YES || echo NO`, "NO"},

		// The eight letters, each as a pair.
		{"a newline", `[[ $'za\nb' == z~(K)a\nb ]] && echo YES || echo NO`, "YES"},
		{"and not an n", `[[ zanb == z~(K)a\nb ]] && echo YES || echo NO`, "NO"},
		{"a tab", `[[ $'za\tb' == z~(K)a\tb ]] && echo YES || echo NO`, "YES"},
		{"and not a t", `[[ zatb == z~(K)a\tb ]] && echo YES || echo NO`, "NO"},
		{"a carriage return", `[[ $'za\rb' == z~(K)a\rb ]] && echo YES || echo NO`, "YES"},
		{"and not an r", `[[ zarb == z~(K)a\rb ]] && echo YES || echo NO`, "NO"},
		{"a form feed", `[[ $'za\fb' == z~(K)a\fb ]] && echo YES || echo NO`, "YES"},
		{"and not an f", `[[ zafb == z~(K)a\fb ]] && echo YES || echo NO`, "NO"},
		{"a vertical tab", `[[ $'za\vb' == z~(K)a\vb ]] && echo YES || echo NO`, "YES"},
		{"and not a v", `[[ zavb == z~(K)a\vb ]] && echo YES || echo NO`, "NO"},
		{"an alert", `[[ $'za\ab' == z~(K)a\ab ]] && echo YES || echo NO`, "YES"},
		{"and not an a", `[[ zaab == z~(K)a\ab ]] && echo YES || echo NO`, "NO"},
		{"an escape", `[[ $'za\eb' == z~(K)a\eb ]] && echo YES || echo NO`, "YES"},
		{"and not an e", `[[ zaeb == z~(K)a\eb ]] && echo YES || echo NO`, "NO"},
		// `\E` is the same character under a second spelling, measured:
		// `$'\e'` and `$'\E'` are both byte 27 there.
		{"the same escape, capitalised", `[[ $'za\eb' == z~(K)a\Eb ]] && echo YES || echo NO`, "YES"},
		{"and not an E", `[[ zaEb == z~(K)a\Eb ]] && echo YES || echo NO`, "NO"},

		// **`\0` is the letter**, so the family is not "every escape a C
		// string has". It is the row that keeps the table measured rather
		// than assumed.
		{"a nought is the letter", `[[ za0b == z~(K)a\0b ]] && echo YES || echo NO`, "YES"},

		// And a letter neither family names is the letter, which is what
		// says the reading is a table and not "any backslash".
		{"an unnamed letter", `[[ zaqb == z~(K)a\qb ]] && echo YES || echo NO`, "YES"},

		// The six classes are untouched beside them.
		{"a class still reads", `[[ za1b == z~(K)a\db ]] && echo YES || echo NO`, "YES"},
		{"and its other half", `[[ zadb == z~(K)a\db ]] && echo YES || echo NO`, "NO"},

		// **The letter decides and not the language**: `p` names the same
		// glob and does not read the escape.
		{"another letter for the glob", `[[ $'za\nb' == z~(p)a\nb ]] && echo YES || echo NO`, "NO"},
		{"which reads the letter instead", `[[ zanb == z~(p)a\nb ]] && echo YES || echo NO`, "YES"},
	} {
		if got := tildeMid(t, tc.src, nil); got != tc.want {
			t.Errorf("%s: %s = %q, want %q", tc.name, tc.src, got, tc.want)
		}
	}
}

// A control escape takes **one byte** where a class takes one unit, which is
// what keeps it from consuming a character it never named.
//
// Every letter in the family is ASCII, so a multi-byte character can equal
// none of them. A reading that took a whole unit would match `é` against
// `\n` whenever the unit happened to start with the right byte — it cannot
// here, but the row is what says so rather than the comment.
//
// The two `é` rows are **locale-independent** and that is on purpose: under
// `C` it is two units and under `en_US.UTF-8` it is one, and neither is a
// newline either way. The class row beside them uses a one-byte subject for
// the same reason.
func TestAControlEscapeTakesOneByteAndNotOneUnit(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a multi-byte character is not a newline", `[[ 'zaéb' == z~(K)a\nb ]] && echo YES || echo NO`, "NO"},
		{"nor a tab", `[[ 'zaéb' == z~(K)a\tb ]] && echo YES || echo NO`, "NO"},
		// And the class beside it does match a character, which is the
		// contrast. Deliberately a **one-byte** non-digit: whether `é` is
		// one unit or two is the locale's answer, not this family's — see
		// matchKshClassEscape, where that pair is measured under `C` and
		// under `en_US.UTF-8` — and a row that turned on it would be
		// measuring the locale the harness happened to run in.
		{"where a class matches a character", `[[ zaXb == z~(K)a\Db ]] && echo YES || echo NO`, "YES"},
	} {
		if got := tildeMid(t, tc.src, nil); got != tc.want {
			t.Errorf("%s: %s = %q, want %q", tc.name, tc.src, got, tc.want)
		}
	}
}
