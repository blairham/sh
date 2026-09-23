// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
)

// The order this shell publishes the letters of `$-` in: the lowercase ones
// sorted, then the uppercase ones, then the letter naming the route it was
// invoked by.
//
// Measured on bash 5.3.15, 2026-09-12, and bash 3.2.57 and bash-as-`sh` agree
// on every row:
//
//	set -f; set -u; set -e            efhuBc
//	set -C                            hBCc
//	set -C -e                         ehBCc
//	set -a                            ahBc
//	set -e -C, program on stdin       ehBCs
//	-i -c, at a pseudo-terminal       himBHc
//	set -aefhkmuvxBC (2026-09-16)     aefhkmuvxBCc
//	set -aefhkmuvxBC -o privileged    aefhkmpuvxBCc
//	-o physical -o privileged -o histexpand -o errtrace -o functrace -C
//	                                  hpBCEHPT
//
// The last of those is where `k` sits, measured when `set -k` was built
// (#3095): behind `h` and in front of `m`, which is the lowercase sort.
//
// The two rows after it are where `p` and `P` sit, measured when the letters
// were declared (#4163). `p` is behind `m` and in front of `u`, and behind
// `r` as well — `set -o privileged; set -r` is `hprB`. Between `m` and `r`
// it could only be `n`, and that one cannot be measured from either side:
// `noexec` stops the `echo` that would read `$-`, which is the gap
// interp.orderedOptionLetters already records for `n` itself. `P` is between
// `H` and `T`, measured with all five uppercase letters on at once.
//
// The last two are what say the trailing letter belongs to the *route* and
// not to `c` in particular: `i` and `m` sort in with the lowercase letters
// where `s` does not. Which of `c` and `s` leads is not measurable here,
// because this shell never shows both — see CommandStringShowsSInDollarDash.
func TestDollarDashLetterOrder(t *testing.T) {
	if got, want := bash.Semantics().DollarDashLetterOrder, "aefhkilmnprtuvxBCEHPTcs"; got != want {
		t.Errorf("DollarDashLetterOrder = %q, want %q", got, want)
	}
}

// And the order as this shell writes it, rather than as a string in a field.
// The startup letters are `hB`, so each row is a letter the script set
// finding its place among letters the shell already held.
func TestDollarDashIsWrittenInThatOrder(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set -f; set -u; set -e; echo "[$-]"`, "[efhuB]"},
		{`set -C; echo "[$-]"`, "[hBC]"},
		{`set -C; set -e; echo "[$-]"`, "[ehBC]"},
		{`set -a; echo "[$-]"`, "[ahB]"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil {
			t.Fatalf("run %q: %v", tc.src, err)
		}
		if out != tc.want+"\n" {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want+"\n")
		}
	}
}

// TestASetOptionThisShellOnlyRecordsStillShowsItsLetter is the half of the
// pairing that had nothing asserting it.
//
// `physical`, `privileged` and `histexpand` are names this shell's `set -o`
// table grants — the first two it records and runs nothing for — and a script
// that asks for one and then reads `$-` was told nothing had happened. The
// letter is not decoration: `case $- in *P*)` is how a script asks a shell
// what it is, and a name granted without its letter is a request taken and
// then denied.
//
// Each row is run by the long name *and* by the letter, because that is what
// says they are one request rather than two: the refusal table used to answer
// `set -P` while the `set -o` table answered `set -o physical`, and the two
// gave different sentences for the same question (#4163).
func TestASetOptionThisShellOnlyRecordsStillShowsItsLetter(t *testing.T) {
	for _, tc := range []struct{ name, letter, want string }{
		{"physical", "P", "[hBP]"},
		{"privileged", "p", "[hpB]"},
		{"histexpand", "H", "[hBH]"},
	} {
		for _, src := range []string{
			`set -o ` + tc.name + `; echo "[$-]"`,
			`set -` + tc.letter + `; echo "[$-]"`,
		} {
			out, status, err := preset.Combined(t, dialecttest.Base{}, src)
			if err != nil {
				t.Fatalf("run %q: %v", src, err)
			}
			if out != tc.want+"\n" {
				t.Errorf("%s = %q at %d, want %q", src, out, status, tc.want+"\n")
			}
		}
		// And back off again, by both spellings, so the row cannot pass
		// against a letter that is simply always there.
		for _, src := range []string{
			`set -o ` + tc.name + `; set +o ` + tc.name + `; echo "[$-]"`,
			`set -` + tc.letter + `; set +` + tc.letter + `; echo "[$-]"`,
		} {
			out, status, err := preset.Combined(t, dialecttest.Base{}, src)
			if err != nil {
				t.Fatalf("run %q: %v", src, err)
			}
			if out != "[hB]\n" {
				t.Errorf("%s = %q at %d, want %q", src, out, status, "[hB]\n")
			}
		}
	}
}
