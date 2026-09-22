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
	hashLines   = historyEncoding{hashLines: HashTimestampLinesWhenTheFileOpensWithOne}
	spanning    = historyEncoding{hashLines: HashTimestampLinesAndEntriesSpanThem}
	keepEmpties = historyEncoding{emptyIsAnEntry: true}
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
			// A session killed mid-write, or a trim that cut inside an entry:
			// the last line promised another and there is none, so the entry
			// goes and what there was of it goes with it. This used to keep
			// the half, which is the substrate's own answer and is not the
			// one the dialect that reaches this rule gives (#4033).
			"a file ending mid-entry keeps nothing of it",
			continued,
			[]string{"echo one", `echo half\`},
			[]string{"echo one"},
		},
		{
			// And a file that is nothing else holds nothing at all.
			"a file of one dangling continuation is empty",
			continued,
			[]string{`echo half\`},
			nil,
		},
		{
			// The encoder's guard coming back off: an entry whose own last
			// character is a backslash is written with a space after it, so
			// that this newline is not a promise. Exactly one space, and
			// only where a backslash is in front of it — see HistoryText.
			"a backslash and one space is a backslash",
			continued,
			[]string{`echo x\ `, "echo tail"},
			[]string{`echo x\`, "echo tail"},
		},
		{
			"one of two spaces after a backslash comes off",
			continued,
			[]string{`echo x\  `},
			[]string{`echo x\ `},
		},
		{
			// And a trailing space with no backslash in front of it is the
			// command's own, which is what says the rule is the guard's and
			// not a trim.
			"a trailing space with no backslash stays",
			continued,
			[]string{"echo a  "},
			[]string{"echo a  "},
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
		{
			// The third policy, and the row that pays for it: the lines
			// after a command belong to it, so a command somebody typed over
			// several lines comes back as the one entry it was.
			"entries span the lines between two hash time lines",
			spanning,
			[]string{"#1", "cat <<EOF", "x", "y", "EOF", "#2", "echo b"},
			[]string{"cat <<EOF\nx\ny\nEOF", "echo b"},
		},
		{
			// The spanning is still the file's own first line. A file that
			// does not open with a header is read a line at a time — and the
			// headers come out of it anyway, which is the half this policy
			// has that the one before it does not.
			"a file that does not open with a hash time line is still read a line at a time",
			spanning,
			[]string{"lead", "#1", "echo a", "extra", "#2", "echo c"},
			[]string{"lead", "echo a", "extra", "echo c"},
		},
		{
			// The empty lines of an entry are the entry's, except at the
			// front. Measured on the shell that writes these files.
			"an entry keeps its own empty lines but not its leading ones",
			spanning,
			[]string{"#1", "", "", "echo a", "", "", "#2", "echo c"},
			[]string{"echo a\n\n", "echo c"},
		},
		{
			// And a stretch of nothing but empty lines is no entry at all,
			// which is [EmptyLinesAreEntries] reaching the joined entry
			// rather than the physical line.
			"a stretch of only empty lines is no entry",
			spanning,
			[]string{"#1", "", "", "#2", "echo c"},
			[]string{"echo c"},
		},
		{
			// The fourth answer: an empty line is a gap in the file and not
			// a command. bash's, wherever the blank falls — this is the
			// issue's own case with the rest of the positions beside it.
			"an empty line is not an entry",
			plainFile,
			[]string{"", "echo one", "", "", "echo two", ""},
			[]string{"echo one", "echo two"},
		},
		{
			// Empty, not blank. bash lists a line of spaces and a line of
			// one tab, so a TrimSpace test here would drop what it keeps.
			"a line of whitespace is an entry",
			plainFile,
			[]string{"echo one", "   ", "\t", "echo two"},
			[]string{"echo one", "   ", "\t", "echo two"},
		},
		{
			// And it holds inside a file of time lines, which is where the
			// two answers meet: the headers go for their reason and the
			// blank for its own.
			"an empty line among hash time lines goes too",
			hashLines,
			[]string{"#1", "echo one", "", "#2", "echo two"},
			[]string{"echo one", "echo two"},
		},
		{
			// A file of nothing but blanks holds nothing.
			"a file of only empty lines is empty",
			plainFile,
			[]string{"", ""},
			nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeEntries(tc.in, tc.enc, true)
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
	if got := decodeEntries(stamped, plainFile, true); !slices.Equal(got, stamped) {
		t.Errorf("decoded %q with no timestamp answer, want it left alone", got)
	}
	if got, want := decodeEntries(stamped, stampedOnly, true), []string{"echo one"}; !slices.Equal(got, want) {
		t.Errorf("decoded %q, want %q", got, want)
	}
	joined := []string{`echo one\`, "two"}
	if got := decodeEntries(joined, stampedOnly, true); !slices.Equal(got, joined) {
		t.Errorf("decoded %q with no continuation answer, want it left alone", got)
	}
	// The third answer likewise: a file of `#` time lines is a file of
	// commands beginning with `#` to anything that was not told otherwise,
	// and the other two answers do not imply it.
	hashed := []string{"#100", "echo one"}
	if got := decodeEntries(hashed, zshLikeFile, true); !slices.Equal(got, hashed) {
		t.Errorf("decoded %q with no hash answer, want it left alone", got)
	}
	if got, want := decodeEntries(hashed, hashLines, true), []string{"echo one"}; !slices.Equal(got, want) {
		t.Errorf("decoded %q, want %q", got, want)
	}
	// And the fourth answer moves on its own in the other direction: the
	// empty line goes unless the style asks for it to stay, so a shell that
	// has the other two encodings does not thereby keep blanks.
	blanked := []string{"echo one", "", "echo two"}
	if got, want := decodeEntries(blanked, zshLikeFile, true), []string{"echo one", "echo two"}; !slices.Equal(got, want) {
		t.Errorf("decoded %q with no empty-line answer, want %q", got, want)
	}
	if got := decodeEntries(blanked, keepEmpties, true); !slices.Equal(got, blanked) {
		t.Errorf("decoded %q, want the blank kept", got)
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
		EntriesContinueOnABackslash:     true,
		EntriesMayCarryATimestampHeader: true,
		HashTimestampLines:              HashTimestampLinesAndEntriesSpanThem,
		EmptyLinesAreEntries:            true,
	}
	want := historyEncoding{
		continuesOnABackslash: true,
		mayCarryATimestamp:    true,
		hashLines:             HashTimestampLinesAndEntriesSpanThem,
		emptyIsAnEntry:        true,
	}
	if got := historyEncodingFrom(all); got != want {
		t.Errorf("historyEncodingFrom = %+v, want %+v", got, want)
	}
	// And the exported decoder is that construction and the same decode, so a
	// dialect reading a file of its own gets the session's answer.
	lines := []string{"#100", "echo one"}
	got := HistoryEntries(HistoryStyle{HashTimestampLines: HashTimestampLinesWhenTheFileOpensWithOne}, lines)
	if !slices.Equal(got, []string{"echo one"}) {
		t.Errorf("HistoryEntries = %q, want [echo one]", got)
	}
	if got := HistoryEntries(none, lines); !slices.Equal(got, lines) {
		t.Errorf("HistoryEntries with a silent style = %q, want %q", got, lines)
	}
	// The empty-line answer reaches the exported decoder by the same route,
	// which is what stops the session's reader and a script's `history -r`
	// from parting over a blank in one file (#4024).
	blanked := []string{"echo one", "", "echo two"}
	if got := HistoryEntries(none, blanked); !slices.Equal(got, []string{"echo one", "echo two"}) {
		t.Errorf("HistoryEntries = %q, want the blank dropped", got)
	}
	got = HistoryEntries(HistoryStyle{EmptyLinesAreEntries: true}, blanked)
	if !slices.Equal(got, blanked) {
		t.Errorf("HistoryEntries = %q, want the blank kept", got)
	}
}

// A blank line inside a multi-line entry is part of the entry, which is why
// the blanks are dropped after the decoding rather than before it.
//
// Dropping them first would join the halves into a line nobody typed.
func TestABlankLineInsideAnEntrySurvives(t *testing.T) {
	got := decodeEntries([]string{`for i in 1 2\`, `\`, "done"}, continued, true)
	if want := []string{"for i in 1 2\n\ndone"}; !slices.Equal(got, want) {
		t.Errorf("decoded %q, want %q", got, want)
	}
	// An entry of nothing at all goes; one of whitespace is a command.
	kept, kepttimes := withoutEmpty([]string{"echo one", "", "   ", "echo two"},
		[]string{"1", "2", "3", "4"})
	if want := []string{"echo one", "   ", "echo two"}; !slices.Equal(kept, want) {
		t.Errorf("withoutEmpty = %q, want %q", kept, want)
	}
	// And the times go with the entries they belong to rather than sliding
	// onto the ones that followed the dropped line.
	if want := []string{"1", "3", "4"}; !slices.Equal(kepttimes, want) {
		t.Errorf("withoutEmpty times = %q, want %q", kepttimes, want)
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
		// Both answers on, because the ordering is only a question for a
		// file whose encoding joins lines *and* whose shell drops empties.
		// With the empty answer off there is nothing to order.
		style: HistoryStyle{EntriesContinueOnABackslash: true},
	}
	got := h.load(t.Context())
	// Three entries. Dropping the blank first gives two, the second of which
	// is `for i in 1 2\necho two` — a line nobody typed.
	want := []string{"echo one", "for i in 1 2\n", "echo two"}
	if !slices.Equal(got, want) {
		t.Errorf("load = %q, want %q", got, want)
	}
}

// Where the file **stops** is part of the decoding, and it is a question only
// the text can answer.
//
// Measured 2026-09-21, zsh 5.9.2 under `env -i` with a scratch `HOME`, one
// file at a time through `fc -R` and `fc -l 1`. The middle two rows are the
// pair that says the rule is about the end of the file rather than about a
// dangling backslash: the same bytes are an entry that goes and a command
// ending in a backslash, and the newline after them is the whole difference.
// The last row is the shortest file there is (#4033).
func TestAFilesEndDecidesWhatADanglingContinuationIs(t *testing.T) {
	style := HistoryStyle{EntriesContinueOnABackslash: true, EmptyLinesAreEntries: true}
	for _, tc := range []struct {
		name string
		text string
		want []string
	}{
		{"a promise with a newline after it drops the entry", "echo a\necho b\\\n", []string{"echo a"}},
		{"a file of nothing else holds nothing", "echo b\\\n", nil},
		{"with no newline the backslash is a character", `echo b\`, []string{`echo b\`}},
		{"a file of one newline is one empty entry", "\n", []string{""}},
		{"a file of no bytes at all holds nothing", "", nil},
		{"an unterminated plain file keeps its last line", "echo a\necho b", []string{"echo a", "echo b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := HistoryEntriesIn(style, tc.text); !slices.Equal(got, tc.want) {
				t.Errorf("decoded %q as %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

// The encoder, which is the mirror of the decoder and was missing entirely:
// every writer in the tree put an entry down followed by a newline, so an
// entry holding a newline became two lines of the file (#4034).
//
// Measured 2026-09-21, zsh 5.9.2 under a pseudo-terminal, entries planted and
// written with `fc -W`. The guard row is the one that is not cosmetic: with
// no space after the backslash the newline ending the entry is read as a
// promise, and the entry after it is swallowed.
func TestEncodingAHistoryFile(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enc     historyEncoding
		entries []string
		times   []string
		want    string
	}{
		{
			"a file that states no continuation writes lines",
			plainFile,
			[]string{"echo one", "a\nb"},
			nil,
			"echo one\na\nb\n",
		},
		{
			"an entry's newline becomes a backslash and a newline",
			continued,
			[]string{"a\nb", "echo tail"},
			nil,
			"a\\\nb\necho tail\n",
		},
		{
			"an entry ending in a backslash is guarded with a space",
			continued,
			[]string{`echo x\`, "echo tail"},
			nil,
			"echo x\\ \necho tail\n",
		},
		{
			"an entry of nothing is a line of nothing",
			continued,
			[]string{""},
			nil,
			"\n",
		},
		{
			"a tab is written as itself",
			continued,
			[]string{"a\tb"},
			nil,
			"a\tb\n",
		},
		{
			"no entries is no text",
			continued,
			nil,
			nil,
			"",
		},
		{
			// The header the spanning policy reads is the header it writes.
			"a timed entry is written under its own hash line",
			spanning,
			[]string{"echo one", "echo two"},
			[]string{"1700000000", "1700000060"},
			"#1700000000\necho one\n#1700000060\necho two\n",
		},
		{
			// An entry with no time of its own goes down bare, among ones
			// that have one. Measured on the shell that writes these.
			"an entry with no time is written with no header",
			spanning,
			[]string{"echo one", "echo two"},
			[]string{"", "1700000060"},
			"echo one\n#1700000060\necho two\n",
		},
		{
			// And the policy is the gate: the same entries written by a
			// shell that was not recording times carry no headers.
			"times are not written where the policy does not read them",
			hashLines,
			[]string{"echo one"},
			[]string{"1700000000"},
			"echo one\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := encodeEntries(tc.entries, tc.times, tc.enc); got != tc.want {
				t.Errorf("encoded %q as %q, want %q", tc.entries, got, tc.want)
			}
		})
	}
}

// And the two close over each other, which is the property the pair exists
// for: what the encoder writes, the decoder reads back unchanged.
//
// Asserted over the shapes that have a rule each rather than over one string,
// because the guard and the join are separate answers and a round trip that
// exercised only one would pass with the other missing.
func TestTheEncoderAndTheDecoderCloseOverEachOther(t *testing.T) {
	style := HistoryStyle{EntriesContinueOnABackslash: true, EmptyLinesAreEntries: true}
	entries := []string{
		"echo plain",
		"for i in 1 2\ndo\necho $i\ndone",
		`echo x\`,
		"a\tb",
		"",
		"echo last",
	}
	text := HistoryText(style, entries)
	if got := HistoryEntriesIn(style, text); !slices.Equal(got, entries) {
		t.Errorf("round trip through %q gave %q, want %q", text, got, entries)
	}
	// The shape where this encoder and the real shell's part: a backslash
	// immediately *before* an embedded newline. The real shell writes one
	// backslash for the two facts and reads the entry back without the
	// character somebody typed; this writes both and keeps it. Measured
	// 2026-09-21, and the choice is safe rather than only nicer — the real
	// shell reads the two-backslash file to the same entry this does, so a
	// file written here is one it understands.
	awkward := []string{"a\\\nb"}
	if got := HistoryEntriesIn(style, HistoryText(style, awkward)); !slices.Equal(got, awkward) {
		t.Errorf("round trip of %q gave %q, want it kept", awkward, got)
	}
	if got, want := HistoryText(style, awkward), "a\\\\\nb\n"; got != want {
		t.Errorf("encoded %q as %q, want %q", awkward, got, want)
	}
}
