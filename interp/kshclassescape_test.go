// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// A `~(K)` group makes a written `\d` a **digit class** in the glob that
// letter names, and five escapes beside it likewise.
//
// `K` names the language a pattern with no prefix is already in, so the group
// is consumed and what follows is an ordinary glob — and it is also the one
// letter that makes the backslash a class. The rows are ksh93u+
// 2012-08-01's, measured 2026-09-27; nothing here names a shell, because the
// construct is a grammar flag and the letters are that construct's.
//
// **Every letter is a pair**, because a class reading and a literal reading
// agree on half of all subjects: a row asserting only that `~(K)a\db` matches
// `za1b` would pass for a shell reading `\d` as the letter and handed a
// subject with a `d` in it.
func TestATildeGlobGroupReadsTheClassEscapes(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The controls. With no group at all the escape is the shell's, so
		// `\d` is the letter — and that pair is what says the rows below it
		// are the group's doing.
		{"no group, a digit", `[[ za1b == za\db ]] && echo YES || echo NO`, "NO"},
		{"no group, the letter", `[[ zadb == za\db ]] && echo YES || echo NO`, "YES"},

		// The six letters, each as a pair.
		{"a digit", `[[ za1b == z~(K)a\db ]] && echo YES || echo NO`, "YES"},
		{"and not a letter", `[[ zadb == z~(K)a\db ]] && echo YES || echo NO`, "NO"},
		{"a non-digit", `[[ zadb == z~(K)a\Db ]] && echo YES || echo NO`, "YES"},
		{"and not a digit", `[[ za1b == z~(K)a\Db ]] && echo YES || echo NO`, "NO"},
		{"a word character", `[[ za_b == z~(K)a\wb ]] && echo YES || echo NO`, "YES"},
		{"and not a dash", `[[ 'za-b' == z~(K)a\wb ]] && echo YES || echo NO`, "NO"},
		{"a non-word character", `[[ 'za.b' == z~(K)a\Wb ]] && echo YES || echo NO`, "YES"},
		{"and not a digit either", `[[ za1b == z~(K)a\Wb ]] && echo YES || echo NO`, "NO"},
		{"a space", `[[ 'za b' == z~(K)a\sb ]] && echo YES || echo NO`, "YES"},
		{"and not a digit at all", `[[ za1b == z~(K)a\sb ]] && echo YES || echo NO`, "NO"},
		{"a non-space", `[[ za1b == z~(K)a\Sb ]] && echo YES || echo NO`, "YES"},
		{"and not a space", `[[ 'za b' == z~(K)a\Sb ]] && echo YES || echo NO`, "NO"},

		// **The letter decides and not the language.** `p` and `s` name the
		// same glob, `i` is the fold and `~()` asks nothing, and none of the
		// four reads the escape.
		{"the same glob by another letter", `[[ za1b == z~(p)a\db ]] && echo YES || echo NO`, "NO"},
		{"and by its third spelling", `[[ za1b == z~(s)a\db ]] && echo YES || echo NO`, "NO"},
		{"the fold does not", `[[ za1b == z~(i)a\db ]] && echo YES || echo NO`, "NO"},
		{"nor an empty group", `[[ za1b == z~()a\db ]] && echo YES || echo NO`, "NO"},
		{"and nothing takes it back", `[[ za1b == z~(K)a~(p)\db ]] && echo YES || echo NO`, "YES"},
		{"nor does the sign", `[[ za1b == z~(-K)a\db ]] && echo YES || echo NO`, "YES"},

		// **Positional**, which is the row that says the group is read where
		// it stands rather than applied to the whole pattern.
		{"at the head", `[[ za1b == ~(K)za\db ]] && echo YES || echo NO`, "YES"},
		{"behind the escape it does not", `[[ za1b == za\db~(K) ]] && echo YES || echo NO`, "NO"},
		{"where the letter still matches", `[[ zadb == za\db~(K) ]] && echo YES || echo NO`, "YES"},

		// **One unit and not one byte, and the classes are the
		// character's.** The locale is what separates the three readings,
		// so each of these is run under a UTF-8 locale and again under the
		// one the rest of the file runs in, where a unit is a byte.
		//
		// Matching one *byte* answers the first row NO under both, since
		// two bytes do not fit the one position a class takes. Matching one
		// unit and asking about its first byte answers the *second* row YES
		// under UTF-8, since `0xC3` is no word character. Only a unit whose
		// class is asked of the character answers all four.
		{
			"a two-byte character is one unit",
			`LC_ALL=en_US.UTF-8; [[ 'zaéb' == z~(K)a\Db ]] && echo YES || echo NO`, "YES",
		},
		{
			"and a letter, so not a non-word one",
			`LC_ALL=en_US.UTF-8; [[ 'zaéb' == z~(K)a\Wb ]] && echo YES || echo NO`, "NO",
		},
		{"where a unit is a byte it is two", `[[ 'zaéb' == z~(K)a\Db ]] && echo YES || echo NO`, "NO"},
		{"and still not a non-word one", `[[ 'zaéb' == z~(K)a\Wb ]] && echo YES || echo NO`, "NO"},
		{
			"with the wildcard as the control on the width",
			`LC_ALL=en_US.UTF-8; [[ 'zaé' == z?? ]] && echo YES || echo NO`, "YES",
		},
		{"which is two bytes without it", `[[ 'zaé' == z?? ]] && echo YES || echo NO`, "NO"},

		// Beside the rest of the glob.
		{"two classes in a row", `[[ za12b == z~(K)a\d\db ]] && echo YES || echo NO`, "YES"},
		{"and one is not two", `[[ za1b == z~(K)a\d\db ]] && echo YES || echo NO`, "NO"},
		{"a wildcard behind one", `[[ za1b == z~(K)a\d*b ]] && echo YES || echo NO`, "YES"},
		{"inside a pattern group", `[[ za1b == @(z~(K)a\db) ]] && echo YES || echo NO`, "YES"},
		{"with the fold beside it", `[[ za1b == z~(Ki)A\db ]] && echo YES || echo NO`, "YES"},
		{"and the glob is still a glob", `[[ zab == z~(K)a* ]] && echo YES || echo NO`, "YES"},

		// Quoting decides it, the same way it does for every other letter.
		{"a quoted group is characters", `[[ za1b == z"~(K)"a\db ]] && echo YES || echo NO`, "NO"},
		{"and an unquoted value is read", `p='z~(K)a\db'; [[ za1b == $p ]] && echo YES || echo NO`, "YES"},

		// A flavor letter behind `K` takes the language with it, and the
		// class escapes go with the language: the *last* one decides.
		{"a flavor behind it wins", `[[ abc == ~(KE)a.c ]] && echo YES || echo NO`, "YES"},
		{"and the glob behind a flavor", `[[ abc == ~(EK)a.c ]] && echo YES || echo NO`, "NO"},
		{"which brings the classes back", `[[ za1b == ~(EK)za\db ]] && echo YES || echo NO`, "YES"},
		{"and not the letter", `[[ zadb == ~(EK)za\db ]] && echo YES || echo NO`, "NO"},

		// A `case` arm is the same pattern language.
		{"a case arm reads it", `case za1b in z~(K)a\db) echo YES;; *) echo NO;; esac`, "YES"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tildeMid(t, tc.src, nil); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// A surface that **chooses a span** does not read the group at all, so it
// does not read the classes either.
//
// ksh93u+ does not read a `~(K)` group on three of the four such operators —
// it is the *group* it declines rather than the escape — so the escapes
// follow the group. #4961 read them on the whole-subject surfaces only and
// left this, and #4978 closed it by gating both on the surface. Measured
// 2026-09-27, with the bracket rows as the discriminating ones, since a
// bracket expression is a class either way:
//
//	${v#~(K)x}        xab     where ${v#x} is ab
//	${v/~(K)x/Q}      xab     where ${v/x/Q} is Qab
//	${v%%~(K)[0-9]}   1abc1   where ${v%%[0-9]} is 1abc
//	${v%~(K)[0-9]}    1abc    and `%` alone does read it
//	${v#~(i)[0-9]}    abc1    with another letter as the control
//
// **`${v%…}` was the one row left open and #4978 closed it**, along with the
// group's own divergence that this test used to pin. The rows below are the
// reference's now rather than a mixture of its answers and ours, and three of
// them moved: `${v%~(K)\d}` reads the class because `%` reads the group, and
// the two rows at the bottom stopped trimming because the other operators do
// not read it. See patternOpts.tildeGlobRead.
func TestASpanChoosingSurfaceReadsTheClassEscapesOnlyWhereItReadsTheGroup(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a prefix trim", `v=1abc1; printf "[%s]" "${v#~(K)\d}"`, "[1abc1]"},
		{"the longest prefix trim", `v=1abc1; printf "[%s]" "${v##~(K)\d}"`, "[1abc1]"},
		// `%` is the one that reads the group, so it reads the escape too.
		{"a suffix trim", `v=1abc1; printf "[%s]" "${v%~(K)\d}"`, "[1abc]"},
		{"the longest suffix trim", `v=1abc1; printf "[%s]" "${v%%~(K)\d}"`, "[1abc1]"},
		{"a substitution", `v=1abc1; printf "[%s]" "${v/~(K)\d/X}"`, "[1abc1]"},
		{"a global one", `v=1abc1; printf "[%s]" "${v//~(K)\d/X}"`, "[1abc1]"},

		// The control that says each of those surfaces works at all: with no
		// group the escape is the letter and the trim happens, here and
		// there alike.
		{"a prefix trim with no group", `v=dabc; printf "[%s]" "${v#\d}"`, "[abc]"},
		{"a substitution with no group", `v=adb; printf "[%s]" "${v/\d/X}"`, "[aXb]"},

		// **The group's own divergence, closed.** These two read the group
		// here and did not there, so the trim happened and the reference's
		// did not. Both now leave the value alone, which is the reference's
		// answer — and the controls one line up are what say the operators
		// still work.
		{"a group a prefix trim declines", `v=dabc; printf "[%s]" "${v#~(K)\d}"`, "[dabc]"},
		{"and a substitution too", `v=adb; printf "[%s]" "${v/~(K)\d/X}"`, "[adb]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tildeMid(t, tc.src, nil); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
