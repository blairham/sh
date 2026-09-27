// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The bare `zstyle` listing quotes a value that needs it, so a value holding a
// space is not indistinguishable from two values. Measured 2026-09-26 on zsh
// 5.9.2 (aarch64-apple-darwin25.4.0), run `-f` with `env -u FPATH` over a
// script file (#4602).
//
// `zstyle -L` is the control and it is an unusually good one: the same stored
// values, quoted correctly, by the other renderer in the same builtin. So what
// was wrong was one call site and not a missing facility.
func TestTheBareStyleListingQuotesItsValues(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zstyle :a:b st plain value "with spaces" "has'quote"
zstyle
print ...
zstyle -L`)
	// `plain` and `value` are unquoted in the reference too, which is what
	// says it quotes only what needs it rather than everything.
	want := "st\n        :a:b plain value 'with spaces' 'has'\\''quote'\n" +
		"...\n" +
		"zstyle :a:b st plain value 'with spaces' 'has'\\''quote'\n"
	if out != want || st != 0 {
		t.Errorf("the listings = %q (status %d), want %q", out, st, want)
	}
}

// And the *pattern* is not quoted, which is the discriminating pair: a reading
// of "quote what needs it" would quote a pattern holding a space too, and this
// listing is a display where `-L` is a command that has to read back.
func TestTheBareStyleListingLeavesThePatternBare(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zstyle ':a b:*' st 'v v'
zstyle
print ...
zstyle -L`)
	want := "st\n        :a b:* 'v v'\n" +
		"...\n" +
		"zstyle ':a b:*' st 'v v'\n"
	if out != want || st != 0 {
		t.Errorf("a pattern needing quotes = %q (status %d), want %q", out, st, want)
	}
}

// The indent is the `-e` marker's field, eight columns either way — found
// because it is the same line as the quoting.
func TestTheBareStyleListingMarksAnEvaluatedStyle(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zstyle -e ':c:*' ev 'reply=(x y)'
zstyle ':d:*' pl bare
zstyle`)
	want := "ev\n(eval)  :c:* 'reply=(x y)'\n" +
		"pl\n        :d:* bare\n"
	if out != want || st != 0 {
		t.Errorf("an evaluated style's row = %q (status %d), want %q", out, st, want)
	}
}

// A value single quotes cannot carry back out of a listing moves the whole
// word into `$'…'`, with the caret vocabulary this shell spells a control byte
// in. Measured 2026-09-26 on zsh 5.9.2, read back with `od -c` because the
// point of the rule is the bytes.
//
// The pattern and the style's *name* go through the same speller, which the
// values make easy to miss.
func TestTheStyleListingEscapesAByteQuotesCannotHold(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zstyle $'con\x00text' $'ke\x00y' $'val\x00u' e
zstyle p1 s1 $'a\tb' $'c\nd' $'e\x01f' $'g\x7fh'
zstyle p2 s2 $'has \x01 and space'
zstyle p3 s3 plain
zstyle -L`)
	// The `p2` row is the one a "quote everything unsafe" reading gets
	// wrong: inside the form the space stays literal, and only the quote
	// and the backslash need anything.
	want := "zstyle $'con\\C-@text' $'ke\\C-@y' $'val\\C-@u' e\n" +
		"zstyle p1 s1 $'a\\tb' $'c\\nd' $'e\\C-Af' $'g\\C-?h'\n" +
		"zstyle p2 s2 $'has \\C-A and space'\n" +
		"zstyle p3 s3 plain\n"
	if out != want || st != 0 {
		t.Errorf("the escaped listing = %q (status %d), want %q", out, st, want)
	}
}

// A high byte that is part of a rune is written bare; one that is not moves
// the word into the escaped form. The first row is what a byte-wise safe-set
// test gets wrong, and it is the control for the second.
func TestTheStyleListingLeavesARuneAlone(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zstyle p1 s1 $'h\xc3\xa9x' $'m\x80y'
zstyle -L`)
	want := "zstyle p1 s1 héx $'m\\M-\\C-@y'\n"
	if out != want || st != 0 {
		t.Errorf("a rune in a value = %q (status %d), want %q", out, st, want)
	}
}

// `zstyle -m` matches the pattern against **any** of the style's values, not
// the first. Measured 2026-09-26 on zsh 5.9.2 with the four-value style the
// suite sets.
func TestTheStylePatternTestReachesEveryValue(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zstyle ':ztst:context:sub1' array-style array value elements 'with spaces'
zstyle -m :ztst:context:sub1 array-style 'arr*';        print "A=$?"
zstyle -m :ztst:context:sub1 array-style 'value';       print "B=$?"
zstyle -m :ztst:context:sub1 array-style 'w* *s';       print "C=$?"
zstyle -m :ztst:context:sub1 array-style 'with spaces'; print "D=$?"
zstyle -m :ztst:context:sub1 array-style 'v';           print "E=$?"
zstyle -m :ztst:context:sub1 array-style 'nope';        print "F=$?"`)
	// Row A is the control — the *first* value, which a reading of element
	// zero alone also gets right — and B, C and D are the ones it fails.
	// E and F are the other control: a pattern nothing matches is still 1.
	want := "A=0\nB=0\nC=0\nD=0\nE=1\nF=1\n"
	if out != want || st != 0 {
		t.Errorf("zstyle -m = %q (status %d), want %q", out, st, want)
	}
}
