// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// The sizes, the trim and the numbering, measured 2026-09-21 against bash
// 5.3.20 at `/opt/homebrew/bin/bash` — the panel's bash, not `/bin/bash`,
// which is 3.2 — with `env -i`, a scratch HOME and no terminal. Every want
// below is the byte-for-byte output of the same script under the real binary,
// and every row was run side by side with it.

// historySizingIgnores keeps the reader's own lines out of the list, for the
// reason historyStifleIgnores gives: the line carrying an assignment is
// itself an entry, and its insert moves the numbers on again before the
// `history` on the next line can look.
const historySizingIgnores = "HISTIGNORE='history*:HISTSIZE*:HISTFILESIZE*:set*:unset*:" +
	"HISTIGNORE*:echo*:printf*:HISTFILE*:local*:fn*'\n"

// HISTFILESIZE and HISTSIZE are one question asked of two things, and bash
// reads their values the same way. The whitespace rows are the ones that were
// wrong: `historySize` had learned to ignore it and `historyFileSize` was
// still on a bare `strconv.Atoi` beside it (#4074).
//
// The figure is how many lines a three-line HISTFILE holds when the shell
// ends, the same measurement the issue carries.
func TestHistfilesizeReadsItsValueTheWayTheListSizeDoes(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, value string
		lines       int
	}{
		{"spaces around the digits", `" 2 "`, 2},
		{"a leading tab", `$'\t2'`, 2},
		{"a sign", "+2", 2},
		{"negative zero is a count of none", "-0", 0},
		{"text after the digits is not a count", "2x", 5},
		{"and neither is a hex spelling", "0x2", 5},
		{"nor a value too large to hold", "99999999999999999999", 5},
		{"a negative keeps everything", "-1", 5},
		{"and so does a word", "abc", 5},
		{"only whitespace is not a count either", `" "`, 5},
		{"and an empty value keeps everything", `""`, 5},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			src := "HISTFILE=$F\nset -o history\nHISTFILESIZE=" + c.value + "\necho one\n"
			_, file := historyFileRun(t, src, "l1", "l2", "l3")
			got := 0
			if trimmed := strings.TrimSuffix(file, "\n"); trimmed != "" {
				got = len(strings.Split(trimmed, "\n"))
			}
			if got != c.lines {
				t.Errorf("the file holds %d lines, want %d:\n%s", got, c.lines, file)
			}
		})
	}
}

// A line `history -r` or `history -n` pushes off a full list moves the
// numbering on exactly as an entry the script made does. The read a script's
// first `set -o history` performs moves nothing, which is the row that makes
// this a flag rather than one answer (#4073).
func TestReadingAFileIntoAFullListMovesTheNumbering(t *testing.T) {
	t.Parallel()
	adds := "history -s a; history -s b; history -s c\n"
	for _, c := range []struct{ name, src, want string }{
		{
			"the issue's own case: four lines into a list held at two",
			"set -o history\nHISTSIZE=2\n" + adds + "history -r $F\nhistory\n",
			"    6  z\n    7  w\n",
		},
		{
			"the other letter that reads counts the same way",
			"set -o history\nHISTSIZE=2\n" + adds + "HISTFILE=$F\nhistory -n\nhistory\n",
			"    6  z\n    7  w\n",
		},
		{
			"a read into an empty list moves it by what the cap drops",
			"set -o history\nHISTSIZE=2\nhistory -r $F\nhistory\n",
			"    3  z\n    4  w\n",
		},
		{
			"and the same read twice moves it twice",
			"set -o history\nHISTSIZE=2\nhistory -r $F\nhistory -r $F\nhistory\n",
			"    7  z\n    8  w\n",
		},
		{
			"an unbounded list drops nothing and numbers from one",
			"set -o history\nhistory -r $F\nhistory\n",
			"    1  x\n    2  y\n    3  z\n    4  w\n",
		},
		{
			"the read at startup moves nothing, however many the cap drops",
			"HISTFILE=$F\nHISTSIZE=2\nset -o history\nhistory\n",
			"    1  z\n    2  w\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := historyFileRun(t, historySizingIgnores+c.src, "x", "y", "z", "w")
			if out != c.want {
				t.Errorf("listing %q, want %q", out, c.want)
			}
		})
	}
}

// The ending appends what no `history -a` has written, except where the list
// has been made shorter than the count still waiting: then it puts the list
// down as the whole file, and lines an earlier `-a` wrote — or another shell
// wrote — are gone (#4059).
//
// `p q r` is seeded into the file and is never this session's, unless the row
// says the list was turned on with HISTFILE already naming it.
func TestAnEndingAfterALostEntryWritesTheListRatherThanAppending(t *testing.T) {
	t.Parallel()
	const on = "set -o history\nHISTFILE=$F\n"
	const read = "HISTFILE=$F\nset -o history\n"
	for _, c := range []struct{ name, src, want string }{
		{
			"the control: no assignment, and the earlier write survives",
			on + "history -s a\nhistory -a\nhistory -s b\nhistory -s c\n",
			"p\nq\nr\na\nb\nc\n",
		},
		{
			"a trim that drops only written entries still appends",
			on + "history -s a\nhistory -a\nhistory -s b\nhistory -s c\nHISTSIZE=2\n",
			"p\nq\nr\na\nb\nc\n",
		},
		{
			"a trim that drops an unwritten one writes the list",
			on + "history -s a\nhistory -a\nhistory -s b\nhistory -s c\nHISTSIZE=1\n",
			"c\n",
		},
		{
			"with nothing written first, the same",
			on + "history -s a\nhistory -s b\nhistory -s c\nHISTSIZE=1\n",
			"c\n",
		},
		{
			"and lines the list read at startup go with them",
			read + "history -s a\nhistory -s b\nhistory -s c\nHISTSIZE=1\n",
			"c\n",
		},
		{
			"a trim above what is waiting leaves the read lines alone",
			read + "history -s a\nhistory -s b\nhistory -s c\nHISTSIZE=4\n",
			"p\nq\nr\na\nb\nc\n",
		},
		{
			"an append straight after the trim leaves the ending nothing",
			on + "history -s a\nhistory -a\nhistory -s b\nhistory -s c\nHISTSIZE=1\nhistory -a\n",
			"p\nq\nr\na\nc\n",
		},
		{
			"and entries made after the trim are the list the ending writes",
			on + "history -s a\nhistory -a\nhistory -s b\nhistory -s c\nHISTSIZE=1\n" +
				"history -s d\nhistory -s e\n",
			"e\n",
		},
		{
			"a size of none empties the file",
			on + "history -s a\nhistory -s b\nHISTSIZE=0\n",
			"",
		},
		{
			"but not where nothing was waiting to be written",
			on + "history -s a\nhistory -a\nHISTSIZE=0\n",
			"p\nq\nr\na\n",
		},
		{
			"nor where the list was never filled at all",
			on + "HISTSIZE=0\n",
			"p\nq\nr\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, file := historyFileRun(t, historySizingIgnores+c.src, "p", "q", "r")
			if file != c.want {
				t.Errorf("file %q, want %q", file, c.want)
			}
		})
	}
}

// Restoring a `local` is an assignment moment, and the list is trimmed at the
// return rather than at the next entry (#4045). Nothing runs between the
// return and the listing, so the trim is the restore.
func TestRestoringALocalListSizeTrimsAtTheReturn(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the issue's own case",
			"set -o history\nHISTSIZE=2\nhistory -s a; history -s b; history -s c\n" +
				"fn() { local HISTSIZE=9; history -s d; history -s e; history -s g; history; }\n" +
				"fn\necho ---\nhistory\n",
			"    2  b\n    3  c\n    4  d\n    5  e\n    6  g\n---\n    3  e\n    4  g\n",
		},
		{
			"a restore to the same value moves nothing",
			"set -o history\nHISTSIZE=2\nhistory -s a; history -s b; history -s c\n" +
				"fn() { local HISTSIZE=2; history; }\nfn\necho ---\nhistory\n",
			"    2  b\n    3  c\n---\n    2  b\n    3  c\n",
		},
		{
			"a declaration with no value lifts the cap and the return puts it back",
			"set -o history\nHISTSIZE=9\nhistory -s a; history -s b; history -s c\n" +
				"history -s d; history -s e\n" +
				"fn() { local HISTSIZE; history -s f; history; }\nfn\necho ---\nhistory\n",
			"    1  a\n    2  b\n    3  c\n    4  d\n    5  e\n    6  f\n---\n" +
				"    1  a\n    2  b\n    3  c\n    4  d\n    5  e\n    6  f\n",
		},
		{
			"a name the restore leaves unset lifts the cap instead",
			"set -o history\nunset HISTSIZE\nhistory -s a; history -s b; history -s c\n" +
				"fn() { local HISTSIZE=2; history -s d; }\nfn\nhistory -s e\nhistory\n",
			"    2  c\n    3  d\n    4  e\n",
		},
		{
			"an inner call's own restore is the inner one",
			"set -o history\nHISTSIZE=3\nhistory -s a; history -s b; history -s c\n" +
				"history -s d; history -s e\n" +
				"fnb() { local HISTSIZE=2; history; }\n" +
				"fn() { local HISTSIZE=9; history -s f; history -s g; fnb; echo ---; history; }\n" +
				"fn\necho ===\nhistory\n",
			"    3  f\n    4  g\n---\n    3  f\n    4  g\n===\n    3  f\n    4  g\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := historyFileRun(t, historySizingIgnores+c.src)
			if out != c.want {
				t.Errorf("listing %q, want %q", out, c.want)
			}
		})
	}
}

// The file half of the same moment: the outer HISTFILESIZE truncates where
// the return puts it back. The file **grows** after the inner assignment, so
// the truncation cannot be the one the local did on the way in.
func TestRestoringALocalFileSizeTruncatesAtTheReturn(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the issue's own case",
			"HISTFILESIZE=2\nfn() { local HISTFILESIZE=9; printf '%s\\n' a b c d e > $F; }\nfn\n",
			"d\ne\n",
		},
		{
			"a file already under the restored size is left alone",
			"HISTFILESIZE=5\nfn() { local HISTFILESIZE=9; printf '%s\\n' a b > $F; }\nfn\n",
			"a\nb\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			src := "HISTIGNORE='*'\nset -o history\nHISTFILE=$F\n" + c.src
			_, file := historyFileRun(t, src)
			if file != c.want {
				t.Errorf("file %q, want %q", file, c.want)
			}
		})
	}
}
