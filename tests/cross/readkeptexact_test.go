// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What a trapped signal does to a `read -N` in ksh93 (#7177). Every row is a
// script run from -c in a directory of its own against ksh93u+ 2012-08-01 on
// 2026-10-31, ten runs with one answer, and the wanted output is what it
// printed. See interp.Semantics.ReadExactCountGivenUpKeepsWhatItTook and
// interp.ReadAbandonedOnlyAtTheStartOfALine.

// exactPartOfALine is partOfALine with the next read written out, since which
// number that read names is what these rows are about: `first` waiting before
// the read, INT 0.3 s in, and `cd` and `end` half a second after that.
func exactPartOfALine(open, first, read, next string) string {
	return `mkfifo p; ` + open + `; trap "echo T" INT; printf '` + first + `' >p; ` +
		`(sleep 0.3; kill -INT $$; sleep 0.5; printf 'cd\nend\n' >p) & ` +
		read + `; echo "st=$? l=[$l]"; ` + next + `; echo "next [$m]"; wait`
}

func TestAGivenUpExactCountKeepsWhatItTookWhereTheShellsDo(t *testing.T) {
	t.Parallel()
	const (
		kept    = "T\nst=258 l=[]\nnext [abcd]\n"
		dropped = "T\nst=258 l=[]\nnext [cd]\n"
		readOn  = "T\nst=0 l=[abcd]\nnext []\n"
		// With nothing waiting, a read that goes on takes `cd`, the newline
		// and `e`, and the next read the rest of `end`.
		readOnEmpty   = "T\nst=0 l=[cd\ne]\nnext [nd]\n"
		copied        = readingOnly + `; exec 5<&3`
		exactByNumber = `read -r -N 4 -u3 l`
	)
	for _, tc := range []struct{ name, src, want string }{
		// Given up with `ab` taken, the read keeps it, and the next read of
		// the same number has it first. Not the stream: `dd` reads what came
		// after it. Not another number: the source of a copy reads on in the
		// stream.
		{"-u3", exactPartOfALine(readingOnly, "ab", exactByNumber, byNumber2), kept},
		{"a copy", exactPartOfALine(copied, "ab", `read -r -N 4 -u5 l`, `read -r -u5 m`), kept},
		{"the copy's source", exactPartOfALine(copied, "ab", `read -r -N 4 -u5 l`, byNumber2), dropped},
		{"a count after it", exactPartOfALine(readingOnly, "ab", exactByNumber, `read -r -N 3 -u3 m`), "T\nst=258 l=[]\nnext [abc]\n"},
		{
			"a command after it", exactPartOfALine(readingOnly, "ab", exactByNumber, `dd bs=1 count=2 <&3 2>/dev/null; echo; `+byNumber2),
			"T\nst=258 l=[]\ncd\nnext [ab]\n",
		},
		{"both ways", exactPartOfALine(bothWays, "ab", exactByNumber, byNumber2), kept},
		{"standard input both ways", exactPartOfALine(`exec 0<>p`, "ab", `read -r -N 4 l`, `read -r m`), kept},
		// The read's own redirection keeps them on the standard input it
		// made, and that goes when the read does.
		{"its own <&3, both ways", exactPartOfALine(bothWays, "ab", `read -r -N 4 l <&3`, redirect2), dropped},
		// A wait for the first byte is not given up from a descriptor open
		// for reading alone; one open both ways is.
		{"nothing in", exactPartOfALine(readingOnly, "", exactByNumber, byNumber2), readOnEmpty},
		{"nothing in, both ways", exactPartOfALine(bothWays, "", exactByNumber, byNumber2), dropped},
		// Standard input is never given up, where `-n` is: `T`, `st=0
		// l=[ab]`, `cd` next (TestATrappedSignalGivesUpAReadWhereTheShellsDo).
		{"standard input", exactPartOfALine(readingOnly, "ab", `read -r -N 4 l <&3`, redirect2), readOn},
		{"its own <p", exactPartOfALine(`exec 4<>p`, "ab", `read -r -N 4 l <p`, `read -r m <p`), readOn},
		{"standard input, nothing in", exactPartOfALine(readingOnly, "", `read -r -N 4 l <&3`, redirect2), readOnEmpty},
	} {
		t.Run("ksh/"+tc.name, func(t *testing.T) {
			t.Parallel()
			if got := runReleasingFifos(t, presets["ksh"], tc.src); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAReadMarkGoesWithTheCommandsOwnStandardInput: a mark a `read` leaves on
// the standard input its own redirection made is gone once that redirection
// is, even where the redirection copied a number that is still open. Measured
// 2026-10-31 on ksh93u+, which prints this; the next `read -u3` finds the end,
// since the copy took `abc` from the file they share (#7177). See
// interp.Semantics.ReadCountAtTheEndSticksTheDescriptor.
func TestAReadMarkGoesWithTheCommandsOwnStandardInput(t *testing.T) {
	t.Parallel()
	out, st, err := presets["ksh"].Combined(t, dialecttest.Base{
		Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
	}, `: >f; exec 3<f; read -r -n 4 l <&3; echo "st=$?"; echo abc >>f; read -r m <&3; echo "st=$? m=[$m]"; read -r -u3 m; echo "st=$? m=[$m]"`)
	if want := "st=1\nst=0 m=[abc]\nst=1 m=[]\n"; err != nil || st != 0 || out != want {
		t.Errorf("got %q (status %d, err %v), want %q", out, st, err, want)
	}
}
