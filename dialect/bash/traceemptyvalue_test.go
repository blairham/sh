// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// An empty assignment *value* is written bare here — the name, the operator
// and nothing — where an empty *argument* is written as two quotes. The
// position is what decides it and not the quoting, which is why it cannot be
// read off the quoting style this shell shares with the columns that write
// two single quotes for both.
//
// Measured 2026-09-17 against bash 5.3.20, a script file, `env -i` with
// LC_ALL=C. Ours wrote an empty pair of quotes for every shape, which is
// ksh93's and zsh's
// answer, so a reader diffing two trace logs saw a difference that was not
// one (#3158).
func TestAnEmptyAssignmentValueIsWrittenBare(t *testing.T) {
	out, st := answersRun(t, "declare -a arr=(x)\nset -x\nA=\nA+=\narr[0]=\nB= C= :\nD=''\nE=\"$NOPE\"\nset +x\n")
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	want := "+ A=\n+ A+=\n+ arr[0]=\n+ B=\n+ C=\n+ :\n+ D=\n+ E=\n"
	if !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
}

// And the contrast that says it is the empty value and not quoting in
// general: an empty *argument* keeps its two quotes, and a value with a space
// in it keeps the quotes the same style gives it.
func TestAnEmptyArgumentKeepsItsQuotes(t *testing.T) {
	out, _ := answersRun(t, "set -x\n: ''\nB='a b'\nset +x\n")
	for _, want := range []string{"+ : ''\n", "+ B='a b'\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want it to contain %q", out, want)
		}
	}
}
