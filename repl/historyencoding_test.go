// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// How the history file spells an entry.
//
// Nothing here names a shell: the two facts are fields on HistoryStyle and a
// dialect answers them. What is asserted is what each answer does to a file —
// see historystyle.go for where the answers were measured.

var (
	plainFile   = historyEncoding{}
	continued   = historyEncoding{continuesOnABackslash: true}
	stampedOnly = historyEncoding{mayCarryATimestamp: true}
	zshLikeFile = historyEncoding{continuesOnABackslash: true, mayCarryATimestamp: true}
	hashLines   = historyEncoding{mayCarryAHashLine: true}
)

func TestDecodingAHistoryFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		enc  historyEncoding
		in   []string
		want []string
	}{
		{
			// The default, and the whole of what this used to do: every line
			// is an entry and nothing is looked at.
			"a file of plain lines is unchanged",
			plainFile,
			[]string{"echo one", "echo two"},
			[]string{"echo one", "echo two"},
		},
		{
			// The symptom the issue was filed for.
			"a timestamp header comes off",
			zshLikeFile,
			[]string{": 1789247571:0;go run ./cmd/zsh"},
			[]string{"go run ./cmd/zsh"},
		},
		{
			// Only the first `;` after the numbers ends the header.
			"a semicolon in the command survives",
			zshLikeFile,
			[]string{": 1789253048:0;print 'a;b'"},
			[]string{"print 'a;b'"},
		},
		{
			// The same file holds both kinds, because the option can be turned
			// on part-way through its life. A line that is not a header is the
			// command.
			"a file of both kinds reads as both",
			zshLikeFile,
			[]string{": 100:0;echo stamped", "echo bare"},
			[]string{"echo stamped", "echo bare"},
		},
		{
			// A command that merely looks like a header is not one. Each of
			// these fails a different part of the match, which is why they are
			// one row: no `: ` at all, a non-numeric first field, an empty
			// first field, no `;` to end it, a non-numeric second field, and an
			// empty second field. Both numbers are checked, and a mutation run
			// is what asked — nothing here separated the two until the last
			// two entries were added.
			"a command that resembles a header is left alone",
			zshLikeFile,
			[]string{
				":100:0;x", ": abc:0;x", ": :0;x",
				": 100:0 no semicolon", ": 100:abc;x", ": 100:;x",
			},
			[]string{
				":100:0;x", ": abc:0;x", ": :0;x",
				": 100:0 no semicolon", ": 100:abc;x", ": 100:;x",
			},
		},
		{
			// The fact that is *not* the option's: a multi-line command is
			// stored this way in both files.
			"a backslash joins an entry across lines",
			continued,
			[]string{`for i in 1 2\`, `do\`, `echo $i\`, "done"},
			[]string{"for i in 1 2\ndo\necho $i\ndone"},
		},
		{
			"the two compose, header first and then the join",
			zshLikeFile,
			[]string{`: 200:0;for i in 1 2\`, `do\`, "done", ": 300:0;echo after"},
			[]string{"for i in 1 2\ndo\ndone", "echo after"},
		},
		{
			// The order matters: gathered first, then the header taken off. A
			// continuation line that looks like a header must not lose its
			// text, which is what stripping before joining would do.
			"a continuation line that looks like a header keeps its text",
			zshLikeFile,
			[]string{`: 100:0;echo one\`, ": 200:0;still the same entry"},
			[]string{"echo one\n: 200:0;still the same entry"},
		},
		{
			// A session killed mid-write, or a trim that cut inside an entry.
			// What there is of it is an entry rather than nothing.
			"a file ending mid-entry keeps what there is",
			continued,
			[]string{"echo one", `echo half\`},
			[]string{"echo one", "echo half"},
		},
		{
			// A file that says entries do not continue reads the backslash as
			// part of the command, because there it is one.
			"a backslash is data where entries do not continue",
			plainFile,
			[]string{`echo trailing\`, "echo next"},
			[]string{`echo trailing\`, "echo next"},
		},
		{
			// The third encoding: the time on a line of its own in front of
			// the command. The file opens with one, so the file is one of
			// those, and every one of them goes.
			"a file of hash time lines keeps only the commands",
			hashLines,
			[]string{"#1699999999", "echo one", "#1700000000", "echo two"},
			[]string{"echo one", "echo two"},
		},
		{
			// The decision is the file's and not each line's, which is the
			// half that keeps a typed comment. This file does not open with a
			// time line, so the one in the middle is a command.
			"a hash line in a file that does not open with one is a command",
			hashLines,
			[]string{"echo one", "#1700000000", "echo two"},
			[]string{"echo one", "#1700000000", "echo two"},
		},
		{
			// What counts as one is narrow, and each of these fails a
			// different part of it: the character after the `#` is not a
			// digit, there is nothing after it at all, a space intervenes, and
			// the `#` is not the first character. The file opens with a real
			// one, so the mode is on and these are still commands.
			"a line that resembles a hash time line is left alone",
			hashLines,
			[]string{"#1", "#comment here", "#", "#-5", "# 1700000000", "  #1700000000"},
			[]string{"#comment here", "#", "#-5", "# 1700000000", "  #1700000000"},
		},
		{
			// A time line with no command after it — a file cut between the
			// two halves of an entry — goes with the rest rather than becoming
			// an entry of its own.
			"a dangling time line at the end is dropped",
			hashLines,
			[]string{"#1", "echo one", "#2"},
			[]string{"echo one"},
		},
		{
			// And a file of nothing but time lines holds no entries.
			"a file of only time lines is empty",
			hashLines,
			[]string{"#1"},
			nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeEntries(tc.in, tc.enc)
			if !slices.Equal(got, tc.want) {
				t.Errorf("decoded %q, want %q", got, tc.want)
			}
		})
	}
}

// A header is read only where the file may carry one, and a join only where
// entries may continue. Each answer moves on its own.
func TestEachEncodingAnswerMovesOnItsOwn(t *testing.T) {
	stamped := []string{": 100:0;echo one"}
	if got := decodeEntries(stamped, plainFile); !slices.Equal(got, stamped) {
		t.Errorf("decoded %q with no timestamp answer, want it left alone", got)
	}
	if got, want := decodeEntries(stamped, stampedOnly), []string{"echo one"}; !slices.Equal(got, want) {
		t.Errorf("decoded %q, want %q", got, want)
	}
	joined := []string{`echo one\`, "two"}
	if got := decodeEntries(joined, stampedOnly); !slices.Equal(got, joined) {
		t.Errorf("decoded %q with no continuation answer, want it left alone", got)
	}
	// The third answer likewise: a file of `#` time lines is a file of
	// commands beginning with `#` to anything that was not told otherwise,
	// and the other two answers do not imply it.
	hashed := []string{"#100", "echo one"}
	if got := decodeEntries(hashed, zshLikeFile); !slices.Equal(got, hashed) {
		t.Errorf("decoded %q with no hash answer, want it left alone", got)
	}
	if got, want := decodeEntries(hashed, hashLines), []string{"echo one"}; !slices.Equal(got, want) {
		t.Errorf("decoded %q, want %q", got, want)
	}
}

// The encoding a reader uses comes off the style the dialect stated, and
// every reader asks the same question of the same value.
//
// This is the whole of what keeps a script's `history -r` and the session's
// own load from drifting: a rule added to one of them and not the other is
// the shape this repository keeps rediscovering, so there is one decoder and
// the fact lives on HistoryStyle rather than inside either caller.
func TestTheEncodingComesOffTheStatedStyle(t *testing.T) {
	var none HistoryStyle
	if got := historyEncodingFrom(none); got != (historyEncoding{}) {
		t.Errorf("a style that said nothing gave %+v, want the zero encoding", got)
	}
	all := HistoryStyle{
		EntriesContinueOnABackslash:       true,
		EntriesMayCarryATimestampHeader:   true,
		EntriesMayCarryAHashTimestampLine: true,
	}
	want := historyEncoding{continuesOnABackslash: true, mayCarryATimestamp: true, mayCarryAHashLine: true}
	if got := historyEncodingFrom(all); got != want {
		t.Errorf("historyEncodingFrom = %+v, want %+v", got, want)
	}
	// And the exported decoder is that construction and the same decode, so a
	// dialect reading a file of its own gets the session's answer.
	lines := []string{"#100", "echo one"}
	got := HistoryEntries(HistoryStyle{EntriesMayCarryAHashTimestampLine: true}, lines)
	if !slices.Equal(got, []string{"echo one"}) {
		t.Errorf("HistoryEntries = %q, want [echo one]", got)
	}
	if got := HistoryEntries(none, lines); !slices.Equal(got, lines) {
		t.Errorf("HistoryEntries with a silent style = %q, want %q", got, lines)
	}
}

// A blank line inside a multi-line entry is part of the entry, which is why
// the blanks are dropped after the decoding rather than before it.
//
// Dropping them first would join the halves into a line nobody typed.
func TestABlankLineInsideAnEntrySurvives(t *testing.T) {
	got := decodeEntries([]string{`for i in 1 2\`, `\`, "done"}, continued)
	if want := []string{"for i in 1 2\n\ndone"}; !slices.Equal(got, want) {
		t.Errorf("decoded %q, want %q", got, want)
	}
	// And a blank line that is an entry of its own is still dropped.
	if got, want := nonBlank([]string{"echo one", "", "   ", "echo two"}),
		[]string{"echo one", "echo two"}; !slices.Equal(got, want) {
		t.Errorf("nonBlank = %q, want %q", got, want)
	}
}

// The blanks are dropped after the decoding, and that ordering is a property
// of load rather than of decodeEntries — so it is asserted through a file.
//
// A mutation run asked for this one twice. Swapping the two in load broke
// nothing, because every other test here calls the decoder directly; and the
// first version of *this* test did not catch it either, because the "blank"
// line inside its entry was a lone backslash, which is not blank. A blank line
// inside a multi-line command is never bare in the file — it is written as `\`
// like every other continued line.
//
// Where a genuinely blank line appears is at the *end* of an entry: a command
// whose last line is empty. Dropping it first leaves the backslash before it
// dangling, and the entry swallows the next one.
func TestLoadDropsBlanksAfterDecodingAndNotBefore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hist")
	if err := os.WriteFile(path, []byte("echo one\nfor i in 1 2\\\n\necho two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := historyFile{
		path: path, size: 50, file: 50,
		encoding: historyEncoding{continuesOnABackslash: true},
	}
	got := h.load(t.Context())
	// Three entries. Dropping the blank first gives two, the second of which
	// is `for i in 1 2\necho two` — a line nobody typed.
	want := []string{"echo one", "for i in 1 2\n", "echo two"}
	if !slices.Equal(got, want) {
		t.Errorf("load = %q, want %q", got, want)
	}
}
