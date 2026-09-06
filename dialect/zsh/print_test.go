// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// `print` joins its operands with a separator and ends with a terminator, and
// the two are separate settings: `-l` moves the separator, `-N` moves both and
// `-n` clears the terminator. Measured 2026-09-05, zsh 5.9.2.
func TestPrintSeparatorAndTerminator(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print hello world`, "hello world\n"},
		{`print -n abc`, "abc"},
		{`print`, "\n"},
		{`print -l a b`, "a\nb\n"},
		{`print -ln a b`, "a\nb"},
		{`print -nl a b`, "a\nb"},
		{`print -N a b`, "a\x00b\x00"},
		{`print -Nn a b`, "a\x00b"},
		// `-l` wins the separator whichever order the two are written in,
		// while `-N` keeps the terminator: neither is the later-wins
		// opposite of the other.
		{`print -lN a b`, "a\nb\x00"},
		{`print -Nl a b`, "a\nb\x00"},
		{`print -l`, "\n"},
		{`print -N`, "\x00"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The escape set, which outruns this shell's own `echo` in three places: a
// bare octal escape needs no leading zero, `\M-` and `\C-` are escapes here,
// and one nobody knows loses its backslash instead of keeping it.
func TestPrintEscapes(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print 'a\tb'`, "a\tb\n"},
		{`print -r 'a\tb'`, "a\\tb\n"},
		{`print 'X\1Y'`, "X\x01Y\n"},
		{`print 'X\0101Y'`, "X\b1Y\n"},
		{`print 'X\777Y'`, "X\xffY\n"},
		{`print 'X\400Y'`, "X\x00Y\n"},
		{`print 'X\zY'`, "XzY\n"},
		{`print 'X\eY\EZ'`, "X\x1bY\x1bZ\n"},
		{`print 'X\x41Y'`, "XAY\n"},
		{`print 'X\x4Y'`, "X\x04Y\n"},
		{`print 'X\xY'`, "X\x00Y\n"},
		{`print 'X\u41Y'`, "XAY\n"},
		{`print 'X\U00000041Y'`, "XAY\n"},
		{`print 'X\M-aY'`, "X\xe1Y\n"},
		{`print 'X\C-aY'`, "X\x01Y\n"},
		{`print 'X\C-?Y'`, "X\x7fY\n"},
		{`print 'X\C-@Y'`, "X\x00Y\n"},
		{`print 'X\M-\C-aY'`, "X\x81Y\n"},
		{`print 'X\M-\C-aZQ'`, "X\x81ZQ\n"},
		{`print 'X\\Y'`, "X\\Y\n"},
		// A trailing backslash is a backslash, and the expansion is per
		// operand rather than over the joined text — so the `t` of the next
		// operand does not become this one's tab.
		{`print 'a\' 't'`, "a\\ t\n"},
		// `\c` ends the whole command's output: the rest of the operand,
		// every operand after it, and the terminator.
		{`print 'ab\c def'; print after`, "abafter\n"},
		{`print -l 'a\cX' b`, "a"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// `-R` is raw *and* a change of option parser: the rest of its own bundle is
// still read as this shell's letters, while every later word is read as
// echo's — only `e` and `n` are options there, and `--` is an operand.
func TestPrintCapitalRSwitchesTheOptionParser(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print -Rl a b`, "a\nb\n"},
		{`print -R -l c d`, "-l c d\n"},
		{`print -R -r a`, "-r a\n"},
		{`print -R -- -n a`, "-- -n a\n"},
		{`print -R - -n a`, "-n a\n"},
		{`print -R 'a\tb'`, "a\\tb\n"},
		{`print -R -e 'a\tb'`, "a\tb\n"},
		{`print -R -en 'a\tb'`, "a\tb"},
		{`print -Rn a b`, "a b"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// `-o` sorts, `-O` reverses and `-i` folds case. `-O` is a bit on top of `-o`
// rather than its later-wins opposite: `print -O -o` still sorts backwards.
// The order is byte order, which is what the oracle's LC_ALL=C measures.
func TestPrintSortsAndFilters(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print -o B a C`, "B C a\n"},
		{`print -O B a C`, "a C B\n"},
		{`print -oi B a C`, "a B C\n"},
		{`print -Oi B a C`, "C B a\n"},
		{`print -oO a c b`, "c b a\n"},
		{`print -Oo a c b`, "c b a\n"},
		{`print -m 'a*' abc bcd axy`, "abc axy\n"},
		{`print -m 'z*' a b`, "\n"},
		{`print -m '*' a b`, "a b\n"},
		{`print -o -m 'a*' ab aa`, "aa ab\n"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// `-s`, `-S` and `-z` aim at a history and a line editor a non-interactive
// shell has not got: the operands are consumed and nothing is written.
func TestPrintHistoryAndEditorLettersWriteNowhere(t *testing.T) {
	for _, src := range []string{`print -s a b`, `print -S ab`, `print -S`, `print -z c`} {
		out, st := runZsh(t, t.TempDir(), src)
		if st != 0 || out != "" {
			t.Errorf("%s: out %q status %d, want nothing at 0", src, out, st)
		}
	}
}

// The refusals, asserted as whole rendered lines: this shell names the builtin
// and the line between its own name and the message, and a usage complaint is
// 1 with no usage line after it — where ksh93 prints one and exits 2.
func TestPrintRefusals(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{`print -Y a`, "zsh:print:1: bad option: -Y\n", 1},
		{`print -q a`, "zsh:print:1: bad option: -q\n", 1},
		// `-e` is echo's letter and reaches this builtin only inside `-R`.
		{`print -e zz`, "zsh:print:1: bad option: -e\n", 1},
		{`print -Re a`, "zsh:print:1: bad option: -e\n", 1},
		{`print -eR a`, "zsh:print:1: bad option: -e\n", 1},
		{`print -p x`, "zsh:print:1: -p: no coprocess\n", 1},
		{`print -u9 x`, "zsh:print:1: bad file number: 9\n", 1},
		{`print -u-1 x`, "zsh:print:1: bad file number: -1\n", 1},
		{`print -uq x`, "zsh:print:1: number expected after -u: q\n", 1},
		{`print -u2n hi`, "zsh:print:1: number expected after -u: 2n\n", 1},
		{`print -u`, "zsh:print:1: argument expected: -u\n", 1},
		{`print -f`, "zsh:print:1: argument expected: -f\n", 1},
		{`print -m`, "zsh:print:1: no pattern specified\n", 1},
		{`print -m '[' a`, "zsh:print:1: bad pattern: [\n", 1},
		{`print -S x y`, "zsh:print:1: option -S takes a single argument\n", 1},
		// The letters this shell has that this one has not: refused by name,
		// because a builtin that accepts an option and ignores it reads as
		// one that implements it.
		{`print -a x`, "zsh:print:1: -a is not implemented yet\n", 1},
		{`print -b x`, "zsh:print:1: -b is not implemented yet\n", 1},
		{`print -c x`, "zsh:print:1: -c is not implemented yet\n", 1},
		{`print -C 2 x`, "zsh:print:1: -C is not implemented yet\n", 1},
		{`print -D /tmp`, "zsh:print:1: -D is not implemented yet\n", 1},
		// `-P` has left this list — it is implemented, and what it refuses
		// is a prompt *escape* by name rather than the letter. See
		// printprompt_test.go.
		{`print -P '%d'`, "zsh:print:1: the %d prompt escape is not implemented\n", 1},
		{`print -v foo x`, "zsh:print:1: -v is not implemented yet\n", 1},
		{`print -x2 a`, "zsh:print:1: -x is not implemented yet\n", 1},
		{`print -X2 a`, "zsh:print:1: -X is not implemented yet\n", 1},
		{`print -u2 -f '%s' a`, "zsh:print:1: -f with -u is not implemented yet\n", 1},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != tc.status || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}

// `-u` aims the output at a descriptor and `-f` hands the command to printf.
// The option's argument may be attached to the letter or be the next word, and
// `--` and a lone `-` both end the options.
func TestPrintDescriptorFormatAndEnders(t *testing.T) {
	dir := t.TempDir()
	out, st, errs := runZshSplit(t, dir, `print -u2 to-err; print -nu2 more; print -u 2 again`)
	if st != 0 || out != "" || errs != "to-err\nmoreagain\n" {
		t.Errorf("out %q err %q status %d, want the three forms on the diagnostic stream", out, errs, st)
	}
	for _, tc := range []struct{ src, want string }{
		{`print -f '%s|' a b; echo .`, "a|b|.\n"},
		{`print -f '' a; echo .`, ".\n"},
		{`print -- -n; print - -n`, "-n\n-n\n"},
		{`print --`, "\n"},
		{`print -`, "\n"},
		{`print -rn --`, ""},
		{`print -u1 x`, "x\n"},
	} {
		got, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || got != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 0", tc.src, got, st, tc.want)
		}
	}
}
