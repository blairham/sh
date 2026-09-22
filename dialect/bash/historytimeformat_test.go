// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"regexp"
	"strings"
	"testing"
)

// HISTTIMEFORMAT, which is three separate rules and not one switch.
//
// Measured 2026-09-22 on bash 5.3.20 from script files with no terminal,
// `env -i` and a scratch HOME, one shape at a time. Every want here is what
// bash wrote for the same script, with the epoch seconds it chose replaced by
// `#T` on both sides — the second is the machine's clock and is the one thing
// in the file that cannot be asserted.

// anEpoch is the `#<seconds>` line a written file carries, which is compared
// as a shape rather than as a number.
var anEpoch = regexp.MustCompile(`(?m)^#[0-9]+$`)

func withoutTheClock(s string) string { return anEpoch.ReplaceAllString(s, "#T") }

// The read: with the variable set, a file of `#<epoch>` lines holds an entry
// per header rather than an entry per line, so a command typed over several
// lines comes back whole.
func TestATimedFileHoldsAnEntryPerHeader(t *testing.T) {
	seed := []string{"#1", "cat <<EOF", "x", "y", "EOF", "#2", "echo b"}
	for _, c := range []struct{ name, src, want string }{{
		name: "with the variable set the lines after a command are the command's",
		src:  "F=$F\nHISTTIMEFORMAT=''\nset +o history\nhistory -r \"$F\"\nhistory\n",
		want: "    1  cat <<EOF\nx\ny\nEOF\n    2  echo b\n",
	}, {
		// The control, and it is the same file: without the variable the
		// headers still go and every remaining line is an entry.
		name: "without it every line is an entry",
		src:  "F=$F\nset +o history\nhistory -r \"$F\"\nhistory\n",
		want: "    1  cat <<EOF\n    2  x\n    3  y\n    4  EOF\n    5  echo b\n",
	}, {
		// And the spanning is the *file's* first line as well as the
		// variable: one that does not open with a header is read a line at a
		// time, with the headers taken out of it anyway.
		name: "a file that does not open with a header is read a line at a time",
		src:  "F=$F\nHISTTIMEFORMAT=''\nset +o history\nhistory -r \"$F\"\nhistory\n",
		want: "    1  lead\n    2  cat <<EOF\n    3  x\n    4  y\n    5  EOF\n    6  echo b\n",
	}} {
		t.Run(c.name, func(t *testing.T) {
			lines := seed
			if strings.Contains(c.want, "lead") {
				lines = append([]string{"lead"}, seed...)
			}
			out, _ := historyFileRun(t, c.src, lines...)
			if out != c.want {
				t.Errorf("listing %q, want %q", out, c.want)
			}
		})
	}
}

// The empty lines of such an entry are the entry's, except at the front.
func TestATimedEntryKeepsItsOwnEmptyLines(t *testing.T) {
	out, _ := historyFileRun(t,
		"F=$F\nHISTTIMEFORMAT=''\nset +o history\nhistory -r \"$F\"\nhistory\n",
		"#1", "", "", "echo a", "", "", "#2", "echo c")
	if want := "    1  echo a\n\n\n    2  echo c\n"; out != want {
		t.Errorf("listing %q, want %q", out, want)
	}
}

// The write: a `#<epoch>` line goes in front of each entry that has a time,
// and whether any are written at all is the variable as it stands **at the
// write** rather than as it stood when the entries were made.
func TestWhetherAWrittenFileCarriesTimesIsAskedAtTheWrite(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{{
		name: "set at the write, so the times it read are written back",
		src:  "F=$F\nHISTTIMEFORMAT=''\nset +o history\nhistory -r \"$F\"\nhistory -w \"$F\"\n",
		want: "#T\necho a\n#T\necho b\n",
	}, {
		name: "unset at the write, so the same entries go down bare",
		src: "F=$F\nHISTTIMEFORMAT=''\nset +o history\nhistory -r \"$F\"\n" +
			"unset HISTTIMEFORMAT\nhistory -w \"$F\"\n",
		want: "echo a\necho b\n",
	}, {
		// An entry made before the variable was ever set has no time of its
		// own, and is written bare among entries that have one.
		name: "an entry with no time is written with no header",
		src: "F=$F\nset +o history\nhistory -c\nhistory -s alpha\n" +
			"HISTTIMEFORMAT=''\nhistory -s beta\nhistory -w \"$F\"\n",
		want: "alpha\n#T\nbeta\n",
	}, {
		// And the recording outlives the variable: an `unset` does not stop
		// it, which is what makes it a latch rather than a reading.
		name: "an unset does not stop the recording",
		src: "F=$F\nHISTTIMEFORMAT=''\nunset HISTTIMEFORMAT\nset +o history\nhistory -c\n" +
			"history -s alpha\nHISTTIMEFORMAT=''\nhistory -w \"$F\"\n",
		want: "#T\nalpha\n",
	}} {
		t.Run(c.name, func(t *testing.T) {
			_, file := historyFileRun(t, c.src, "#1700000000", "echo a", "#1700000060", "echo b")
			if got := withoutTheClock(file); got != c.want {
				t.Errorf("file %q, want %q", got, c.want)
			}
		})
	}
}

// The listing: the format is strftime's against the entry's own time, and an
// entry with none draws `??` where the time would have gone.
func TestAListingDrawsTheTimeTheFormatAsksFor(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{{
		name: "a format with a conversion in it",
		src:  "F=$F\nHISTTIMEFORMAT='[%s] '\nset +o history\nhistory -r \"$F\"\nhistory\n",
		want: "    1  [1700000000] echo a\n    2  [1700000060] echo b\n",
	}, {
		// An empty format draws nothing, which is how a script asks for the
		// spanning read without changing what a listing looks like.
		name: "an empty format draws nothing",
		src:  "F=$F\nHISTTIMEFORMAT=''\nset +o history\nhistory -r \"$F\"\nhistory\n",
		want: "    1  echo a\n    2  echo b\n",
	}, {
		// A format with no conversion is written as it stands.
		name: "a format with no conversion is literal",
		src:  "F=$F\nHISTTIMEFORMAT='X'\nset +o history\nhistory -r \"$F\"\nhistory\n",
		want: "    1  Xecho a\n    2  Xecho b\n",
	}, {
		name: "an entry with no time of its own draws two question marks",
		src: "F=$F\nset +o history\nhistory -c\nhistory -s alpha\n" +
			"HISTTIMEFORMAT='[%s] ' history\n",
		want: "    1  ??alpha\n",
	}} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := historyFileRun(t, c.src, "#1700000000", "echo a", "#1700000060", "echo b")
			if out != c.want {
				t.Errorf("listing %q, want %q", out, c.want)
			}
		})
	}
}

// HISTFILESIZE counts lines everywhere else and counts **entries** in a file
// of headers, which is visible: cutting by lines leaves a command with no
// header above it.
func TestHistfilesizeCountsEntriesInATimedFile(t *testing.T) {
	seed := []string{"#1", "a", "#2", "b", "#3", "c", "#4", "d", "#5", "e"}
	for _, c := range []struct{ name, src, want string }{{
		name: "with the variable set the newest three entries are kept",
		src:  "HISTFILE=$F\nHISTTIMEFORMAT=''\nHISTFILESIZE=3\n",
		want: "#T\nc\n#T\nd\n#T\ne\n",
	}, {
		// The control: without it the count is of lines, and the cut lands
		// inside an entry.
		name: "without it the newest three lines are kept",
		src:  "HISTFILE=$F\nHISTFILESIZE=3\n",
		want: "d\n#T\ne\n",
	}} {
		t.Run(c.name, func(t *testing.T) {
			_, file := historyFileRun(t, c.src, seed...)
			if got := withoutTheClock(file); got != c.want {
				t.Errorf("file %q, want %q", got, c.want)
			}
		})
	}
}

// Two different letters out of `-anrw` on one call is a refusal, and it
// stands in front of the whole builtin rather than in front of the file work.
//
// Measured 2026-09-22 on bash 5.3.20. Repeating one letter is not the
// combination — `history -a -a` appends once and says nothing — and the
// status is 1 with no usage block under it, where a letter the builtin does
// not have is 2 with one.
func TestTwoOfTheFourFileLettersIsRefused(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{name: "two letters in one word", src: "history -an\n", want: "S: line 2: history: cannot use more than one of -anrw\n", status: 1},
		{name: "two letters in two words", src: "history -a -r\n", want: "S: line 2: history: cannot use more than one of -anrw\n", status: 1},
		{name: "all four", src: "history -anrw\n", want: "S: line 2: history: cannot use more than one of -anrw\n", status: 1},
		{name: "one letter twice is not the combination", src: "history -a -a\n", want: "", status: 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, errs, status := historyRun(t, "HISTFILE=/dev/null\n"+c.src)
			if errs != c.want || status != c.status {
				t.Errorf("stderr %q status %d, want %q and %d", errs, status, c.want, c.status)
			}
		})
	}
	// And the refusal is before any of the builtin's other work: the `-c` of
	// a call that also carries two file letters does not clear the list.
	out, _, _ := historyRun(t, "set +o history\nhistory -s one\nhistory -c -a -r\nhistory\n")
	if want := "    1  one\n"; out != want {
		t.Errorf("listing %q, want %q", out, want)
	}
}
