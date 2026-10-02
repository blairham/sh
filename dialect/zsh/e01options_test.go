// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestABraceClassReadsADollarSingleSpanAsItsValue: under `braceccl` a `$'…'`
// span in the body is the characters it stands for, so `{$'\0'-$'\5'}` is the
// six characters NUL to 5. Measured 2026-10-02 on zsh 5.9.2, E01options'
// `BRACE_CCL option starting from NUL` (#5155). It was the backslash, the
// digits and everything between `0` and `\`.
func TestABraceClassReadsADollarSingleSpanAsItsValue(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`setopt braceccl; for c in {$'\0'-$'\5'}; do print -n $(( #c )),; done`, "0,1,2,3,4,5,"},
		{`setopt braceccl; print -r -- {$'\t'b}`, "\t b\n"},
		{`setopt braceccl; print -r -- {a$'\x2d'c}`, "a b c\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestBsdechoTakesEchosEscapesAway: with `bsdecho` on, `echo` interprets its
// escapes only behind `-e`, and the option is scoped as the axis it reads is.
// Measured 2026-10-02 on zsh 5.9.2, E01options' `BSD_ECHO option` (#5155).
func TestBsdechoTakesEchosEscapesAway(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `setopt bsdecho; echo "a\nb"; echo -e "c\nd"; [[ -o bsdecho ]] && echo on
unsetopt bsdecho; echo "e\nf"; (setopt bsdecho); echo "g\nh"`)
	if want := "a\\nb\nc\nd\non\ne\nf\ng\nh\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestCdablevarsReadsTheOperandAsANamedDirectory: with `cdablevars` on, a `cd`
// operand that names no directory is a `~name` without its tilde — the name
// is the first component and only an absolute value counts — and the move is
// not announced. Measured 2026-10-02 on zsh 5.9.2, E01options' `CDABLE_VARS
// option` (#5155); bash's `cdable_vars` takes a relative value and announces
// it, and has its own tests.
func TestCdablevarsReadsTheOperandAsANamedDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "tmpcd"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _ := runZsh(t, dir, `setopt cdablevars
v1=tmpcd; (cd v1)
v2=$PWD/tmpcd; (cd v2 && print -r -- ${PWD:t})
v3=$PWD/tmpcd/; (cd v3 && print -r -- ${PWD:t})
(cd v2/.. && [[ $PWD == $v2:h ]] && print up)
unsetopt cdablevars; (cd v2)`)
	want := "zsh:cd:2: no such file or directory: v1\ntmpcd\ntmpcd\nup\nzsh:cd:6: no such file or directory: v2\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestCshjunkieloopsClosesALoopBodyWithEnd: with `cshjunkieloops` on, a loop's
// body that is neither `do … done` nor a brace group is a list closed by
// `end`, and the one-command body is gone. Measured 2026-10-02 on zsh 5.9.2
// under `eval`, E01options' `CSH_JUNKIE_LOOPS option (for loop)` (#5155).
func TestCshjunkieloopsClosesALoopBodyWithEnd(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`for f in a b; print $f; end`, "a\nb\n"},
		{`for f (a b) print $f; print x; end`, "a\nx\nb\nx\n"},
		{`for ((i=0;i<2;i++)); print $i; end`, "0\n1\n"},
		{`repeat 2; print r; end`, "r\nr\n"},
		{`i=0; while ((i++ < 2)) print w$i; end`, "w1\nw2\n"},
		{`for f in a b; do print $f; done`, "a\nb\n"},
		{`for f in a b; { print $f }`, "a\nb\n"},
		{`repeat 1 print r`, "(eval):1: parse error near `r'\n"},
		{`if true; then print t; end`, "(eval):1: parse error near `end'\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), "setopt cshjunkieloops; eval '"+tc.src+"'")
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
	out, _ := runZsh(t, t.TempDir(), "eval 'for f in a; print $f; end'")
	if want := "(eval):1: parse error near `end'\n"; out != want {
		t.Errorf("without the option = %q, want %q", out, want)
	}
}
