// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// `history -d` takes a **range**, and its three refusals have three wordings.
//
// Measured 2026-09-21 against bash 5.3.20 at `/opt/homebrew/bin/bash` — the
// panel's bash, not `/bin/bash`, which is 3.2 — with `env -i`, no startup
// files, and a nine-entry list built by `history -s` (#4010). Every row below
// was run side by side against the real binary.
//
// This shell read the whole operand as one number, so `2-4` was `numeric
// argument required` **and a usage block**, and every row here cost two lines
// of `history.tests` rather than one.

// historyNine runs `history -d <op>` against a nine-entry list, `cmd1`
// through `cmd9`, and answers what the shell wrote and the status the
// builtin left behind.
//
// The builtin's own status and not the script's: `-d` leaves the line to
// finish, so the script ends on the listing at 0 whatever the operand did,
// and a test reading the script's status would grade every refusal as a
// success.
func historyNine(t *testing.T, op string) (string, int) {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= 9; i++ {
		b.WriteString("history -s cmd" + strconv.Itoa(i) + "\n")
	}
	b.WriteString("history -d " + op + "\necho st=$?\nhistory\n")
	out, _ := runBash(t, t.TempDir(), b.String())
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "st="); ok {
			st, err := strconv.Atoi(rest)
			if err != nil {
				t.Fatalf("-d %s: status line %q", op, line)
			}
			return out, st
		}
	}
	t.Fatalf("-d %s: no status line in %q", op, out)
	return "", 0
}

// A range deletes the whole span, and negative ends count back from the
// newest entry.
func TestHistoryDeleteTakesARange(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ op, want string }{
		{"2-4", "cmd1 cmd5 cmd6 cmd7 cmd8 cmd9"},
		{"6--1", "cmd1 cmd2 cmd3 cmd4 cmd5"},
		{"2--4", "cmd1 cmd7 cmd8 cmd9"},
		{"-1--1", "cmd1 cmd2 cmd3 cmd4 cmd5 cmd6 cmd7 cmd8"},
		{"+2-+4", "cmd1 cmd5 cmd6 cmd7 cmd8 cmd9"},
		{"5-5", "cmd1 cmd2 cmd3 cmd4 cmd6 cmd7 cmd8 cmd9"},
		{"-9-9", ""},
		// A non-negative end below the oldest entry is clamped to it rather
		// than refused, which a lone `0` is not: `history -d 0` is out of
		// range and `0-0` takes the oldest entry alone.
		{"0-3", "cmd4 cmd5 cmd6 cmd7 cmd8 cmd9"},
		{"0-0", "cmd2 cmd3 cmd4 cmd5 cmd6 cmd7 cmd8 cmd9"},
		{"1-0", "cmd2 cmd3 cmd4 cmd5 cmd6 cmd7 cmd8 cmd9"},
		// Whitespace inside the operand is allowed, and the separator is the
		// first `-` after the first character — never the sign in front.
		{"'2 - 4'", "cmd1 cmd5 cmd6 cmd7 cmd8 cmd9"},
		{"'2 -4'", "cmd1 cmd5 cmd6 cmd7 cmd8 cmd9"},
		{"'2- 4'", "cmd1 cmd5 cmd6 cmd7 cmd8 cmd9"},
		// And the newest entry alone, which this shell refused outright.
		{"-1", "cmd1 cmd2 cmd3 cmd4 cmd5 cmd6 cmd7 cmd8"},
	} {
		out, st := historyNine(t, c.op)
		if st != 0 {
			t.Errorf("-d %s: status = %d, want 0", c.op, st)
		}
		if got := historyListed(out); got != c.want {
			t.Errorf("-d %s: list = %q, want %q", c.op, got, c.want)
		}
	}
}

// A range whose start is after its end deletes nothing, says nothing, and
// is 1.
//
// The silence is the discriminator: every other failure here has a sentence,
// so a shell that reported this one would pass a status-only check and fail
// the line count. `3-0` is the second row because its end was clamped
// underneath its start rather than written that way.
func TestHistoryDeleteBackwardsRangeIsSilentAtOne(t *testing.T) {
	t.Parallel()
	for _, op := range []string{"4-2", "3-0"} {
		out, st := historyNine(t, op)
		if st != 1 {
			t.Errorf("-d %s: status = %d, want 1", op, st)
		}
		if strings.Contains(out, "history:") {
			t.Errorf("-d %s: out = %q, want no complaint", op, out)
		}
		if want := "cmd1 cmd2 cmd3 cmd4 cmd5 cmd6 cmd7 cmd8 cmd9"; historyListed(out) != want {
			t.Errorf("-d %s: list = %q, want it untouched", op, historyListed(out))
		}
	}
}

// An out-of-range **range** names the end that is out of range, as a bare
// number and with no usage block; a side that is not a number at all falls
// back to naming the whole operand at the same wording.
//
// The start is checked first, which `16-40` fixes on its own: both ends are
// out of range there and bash names `16`.
func TestHistoryDeleteRangeNamesTheEndThatIsOutOfRange(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ op, named string }{
		{"16-40", "16"},
		{"1-200", "200"},
		{"1-10", "10"},
		{"10-10", "10"},
		{"-20-50", "-20"},
		// A negative that counts back past the oldest entry is refused even
		// where it lands on zero, which a written `0` is not.
		{"-10-9", "-10"},
		{"-11-9", "-11"},
		// Neither side is a number: the operand itself, same wording.
		{"5-0xaf", "5-0xaf"},
		{"2-@42", "2-@42"},
		{"@42-3", "@42-3"},
		{"2-4x", "2-4x"},
		{"2x-4", "2x-4"},
		{"2-4-6", "2-4-6"},
		{"5-", "5-"},
		{"1-", "1-"},
		{"0-", "0-"},
		{"--1", "--1"},
	} {
		out, st := historyNine(t, c.op)
		if st != 1 {
			t.Errorf("-d %s: status = %d, want 1", c.op, st)
		}
		want := "history: " + c.named + ": history position out of range\n"
		if !strings.Contains(out, want) {
			t.Errorf("-d %s: out = %q, want it to contain %q", c.op, out, want)
		}
		if strings.Contains(out, "usage") {
			t.Errorf("-d %s: out = %q, want no usage block", c.op, out)
		}
	}
}

// A lone operand that is not a number is a third wording again, and a lone
// offset the list does not hold is echoed back **as it was written**.
//
// `numeric argument required` plus a usage block was what this shell said to
// all of these, so both halves are the assertion: the sentence and the
// absence of the block.
func TestHistoryDeleteLoneOperandWordings(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ op, want string }{
		{"@42", "history: @42: invalid number\n"},
		{"9abc", "history: 9abc: invalid number\n"},
		{"0b101", "history: 0b101: invalid number\n"},
		{"0X9", "history: 0X9: invalid number\n"},
		{"+0x9", "history: +0x9: invalid number\n"},
		{"-", "history: -: invalid number\n"},
		// One prefix has a wording of its own, and the rows above are what
		// bound it: a sign or a capital X is not it.
		{"0x9", "history: 0x9: invalid hex number\n"},
		{"0x", "history: 0x: invalid hex number\n"},
		// Out of range, echoed back unchanged — a leading zero is not an
		// octal prefix, so `0777` is seven hundred and seventy-seven.
		{"0", "history: 0: history position out of range\n"},
		{"-0", "history: -0: history position out of range\n"},
		{"10", "history: 10: history position out of range\n"},
		{"010", "history: 010: history position out of range\n"},
		{"+50", "history: +50: history position out of range\n"},
		{"0777", "history: 0777: history position out of range\n"},
		{"-10", "history: -10: history position out of range\n"},
	} {
		out, st := historyNine(t, c.op)
		if st != 1 {
			t.Errorf("-d %s: status = %d, want 1", c.op, st)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("-d %s: out = %q, want it to contain %q", c.op, out, c.want)
		}
		if strings.Contains(out, "usage") {
			t.Errorf("-d %s: out = %q, want no usage block", c.op, out)
		}
	}
}

// A leading zero is a decimal digit and not an octal prefix.
func TestHistoryDeleteLeadingZeroIsDecimal(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ op, gone string }{
		{"007", "cmd7"},
		{"08", "cmd8"},
		{"+3", "cmd3"},
	} {
		out, st := historyNine(t, c.op)
		if st != 0 {
			t.Errorf("-d %s: status = %d, want 0", c.op, st)
		}
		if strings.Contains(historyListed(out), c.gone) {
			t.Errorf("-d %s: list = %q, want %s gone", c.op, historyListed(out), c.gone)
		}
	}
}

// A range takes its whole span off the unwritten count, not one.
//
// The count is what the shell's ending appends, so a `-d 2-4` that took one
// off would write three entries twice. Measured against the real binary with
// `history -a` standing in for the ending, which appends the same count.
func TestHistoryDeleteRangeTakesItsSpanOffTheUnwrittenCount(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := "history -s a\nhistory -s b\nhistory -s c\nhistory -s d\n" +
		"history -d 2-3\nhistory -a f\n"
	if out, st := runBash(t, dir, src); out != "" || st != 0 {
		t.Fatalf("out = %q status = %d, want nothing at 0", out, st)
	}
	data, err := os.ReadFile(filepath.Join(dir, "f"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "a\nd\n"; string(data) != want {
		t.Errorf("file = %q, want %q — the span, not one entry, off the count", data, want)
	}
}

// historyListed is the entries a `history` listing names, space separated.
func historyListed(out string) string {
	var got []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.HasPrefix(fields[1], "cmd") {
			got = append(got, fields[1])
		}
	}
	return strings.Join(got, " ")
}

// An empty list refuses every offset, and that is where a range's clamped
// low end stops applying.
//
// `0-3` is the pair: it takes the first three of a nine-entry list and names
// `0` against no list at all, so a shell that clamped unconditionally would
// delete out of an empty array here.
func TestHistoryDeleteAgainstAnEmptyList(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ op, named string }{
		{"0-0", "0"},
		{"0-3", "0"},
		{"1-2", "1"},
		{"-1--1", "-1"},
	} {
		out, st := runBash(t, t.TempDir(), "history -d "+c.op+"\necho st=$?\n")
		if want := "st=1\n"; !strings.HasSuffix(out, want) {
			t.Errorf("-d %s: out = %q, want it to end with %q", c.op, out, want)
		}
		if st != 0 {
			t.Errorf("-d %s: script status = %d, want the line to finish at 0", c.op, st)
		}
		want := "history: " + c.named + ": history position out of range\n"
		if !strings.Contains(out, want) {
			t.Errorf("-d %s: out = %q, want it to contain %q", c.op, out, want)
		}
	}
}
