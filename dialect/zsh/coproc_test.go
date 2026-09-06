// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// This shell's coprocess is the same keyword as bash's and a different model
// behind it: no name, no array, and two letters to speak to it with. Measured
// 2026-09-05, zsh 5.9.2 — `coproc cat; print -p hi; read -p l` answers `hi`
// while `${COPROC[0]}` stays empty.
func TestCoprocIsSpokenToByALetter(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`coproc /bin/cat; print -p hi; read -p l; echo "l=$l COPROC=${#COPROC[@]}"`)
	if want := "l=hi COPROC=0\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// The two ends stay open across several exchanges, and a second `coproc`
// replaces the first — measured, the later one is the one the letters reach.
func TestCoprocKeepsItsEndsAndIsReplaced(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`coproc /bin/cat; print -p a; print -p b; read -p x; read -p y; echo "$x/$y"`)
	if want := "a/b\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(),
		`coproc /bin/cat; coproc /bin/cat; print -p z; read -p l; echo "l=$l"`)
	if want := "l=z\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// A compound command needs no name in front of it, which is the half a
// grammar that only reads `coproc NAME <compound>` would get wrong.
func TestCoprocTakesACompoundWithNoName(t *testing.T) {
	for _, src := range []string{
		`coproc { /bin/cat; }; print -p hi; read -p l; echo "l=$l"`,
		`coproc ( /bin/cat ); print -p hi; read -p l; echo "l=$l"`,
	} {
		out, st := runZsh(t, t.TempDir(), src)
		if want := "l=hi\n"; st != 0 || out != want {
			t.Errorf("%s: out %q status %d, want %q at 0", src, out, st, want)
		}
	}
}

// And a name in front of one is a parse error, because there is no name to
// read: `MY {` is a simple command and the `}` closes nothing.
func TestANamedCoprocIsAParseErrorHere(t *testing.T) {
	if _, err := parseZsh(`coproc MY { /bin/cat; }`); err == nil {
		t.Fatal("accepted a named coprocess")
	} else if got, want := err.Error(), `1:23: "}" unexpected`; got != want {
		t.Errorf("refused with %q, want %q", got, want)
	}
}

// With nothing started the two letters are the same refusal in the same
// words, at 1, asserted as whole rendered lines with this shell's location.
func TestTheCoprocessLettersWithNothingStarted(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(),
		`print -p x; echo "p=$?"; read -p y; echo "r=$?"`)
	wantOut := "p=1\nr=1\n"
	wantErr := "zsh:print:1: -p: no coprocess\nzsh:read:1: -p: no coprocess\n"
	if st != 0 || out != wantOut || errs != wantErr {
		t.Errorf("out %q err %q status %d, want %q and %q", out, errs, st, wantOut, wantErr)
	}
}
