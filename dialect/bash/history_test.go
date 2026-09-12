// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `history`, measured 2026-09-12 against bash 5.3.15 under plain `bash -c`.
//
// The gate is asserted from outside, against the shipped binary, in
// `cmd/sh/sandboxhistory_test.go` — a policy is something a person passes to a
// program, so a test that built one in-process would grade the wiring it
// wrote. What is here is what the builtin does when nothing is watching,
// which is the half that decides whether the sandbox rows measure anything.
//
// Every expectation below was run side by side against the real binary. Where
// a case asserts a diagnostic, the text is bash's own with the `bash: line N:`
// location in front of it left to the substrate, which is why the assertions
// are on the trailing sentence rather than on the whole line.

// The list is empty in a script, and `-w` writes the file anyway.
//
// This is the measurement the whole row turns on: bash keeps a history list
// with no terminal anywhere, so `history -w` is an ordinary script creating a
// file at a path it chose. zsh's `fc -W` writes nothing at all unless the
// shell is interactive, which is why the sibling sandbox row is still inert
// and this one is not.
func TestHistoryWritesAFileFromAnEmptyListInAScript(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runBash(t, dir, "history\nhistory -w saved\n")
	if out != "" || st != 0 {
		t.Errorf("out = %q, status = %d, want the empty list and 0", out, st)
	}
	info, err := os.Stat(filepath.Join(dir, "saved"))
	if err != nil {
		t.Fatalf("the file was not created: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("size = %d, want an empty file", info.Size())
	}
	// `0600` and not the umask's answer. A history file holds what somebody
	// typed, so the shell that writes it narrows the mode itself — measured,
	// and a test that accepted 0644 would be asserting the umask.
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %#o, want 0600", got)
	}
}

// The listing is `%5d  %s`, oldest first, numbered from one — and a count
// operand does not renumber.
//
// `history 1` on a two-entry list writes `    2  b`, so the number is the
// entry's place in the whole list rather than in what was printed. A shell
// that numbered the printed lines would write `    1  b` and look right.
func TestHistoryListsWithBashsOwnNumberingAndWidth(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct{ src, want string }{
		{"history -s 'echo one'\nhistory\n", "    1  echo one\n"},
		{"history -s echo two args\nhistory\n", "    1  echo two args\n"},
		{"history -s a\nhistory -s b\nhistory\n", "    1  a\n    2  b\n"},
		{"history -s a\nhistory -s b\nhistory 1\n", "    2  b\n"},
	} {
		if out, st := runBash(t, dir, c.src); out != c.want || st != 0 {
			t.Errorf("%q: out = %q status = %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// `-r` re-reads the whole file every time and `-n` reads only what it has not
// read yet, which is the entire difference between the two letters.
//
// Both halves are measured and both are needed: `-r` twice on a one-line file
// leaves two copies, `-n` twice leaves one, and `-n` after the file grows
// picks up only the new line. A shell that tracked an offset for `-r` would
// pass the second and third and fail the first.
func TestHistoryReadLettersDifferOnWhetherTheyTrackWhatTheyHaveRead(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{{
		name: "-r does not track",
		src:  "printf 'a\\n' > f\nhistory -r f\nhistory -r f\nhistory\n",
		want: "    1  a\n    2  a\n",
	}, {
		name: "-n tracks",
		src:  "printf 'a\\nb\\n' > f\nhistory -n f\nhistory -n f\nhistory\n",
		want: "    1  a\n    2  b\n",
	}, {
		name: "-n reads only what arrived since",
		src:  "printf 'a\\n' > f\nhistory -n f\nprintf 'b\\n' >> f\nhistory -n f\nhistory\n",
		want: "    1  a\n    2  b\n",
	}} {
		if out, st := runBash(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s: out = %q status = %d, want %q at 0", c.name, out, st, c.want)
		}
	}
}

// The history file is read back with `read` in a loop rather than with `cat`.
// The dialect harness gives a runner a PATH of its scratch directory and
// nothing else, so there is no external command on it at all — which is the
// right shape for a test about a builtin, and is why these cases say what
// they say.

// `-a` appends what has arrived since the last *append*, and `-w` does not
// reset that mark.
//
// The three-line file is the measurement and it looks like a bug until the
// rule is stated: the write puts the whole list down, then the append adds
// everything since the last append — which was none — so the first entry
// lands twice.
func TestHistoryAppendCountsFromTheLastAppendAndNotFromTheLastWrite(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{{
		name: "two appends write each entry once",
		src:  ": > f\nhistory -s one\nhistory -a f\nhistory -s two\nhistory -a f\nwhile IFS= read -r l; do echo \"$l\"; done < f\n",
		want: "one\ntwo\n",
	}, {
		name: "an append with nothing new writes nothing",
		src:  ": > f\nhistory -s one\nhistory -a f\nhistory -a f\nwhile IFS= read -r l; do echo \"$l\"; done < f\n",
		want: "one\n",
	}, {
		name: "a write does not move the append mark",
		src:  "history -s one\nhistory -w f\nhistory -s two\nhistory -a f\nwhile IFS= read -r l; do echo \"$l\"; done < f\n",
		want: "one\none\ntwo\n",
	}} {
		if out, st := runBash(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s: out = %q status = %d, want %q at 0", c.name, out, st, c.want)
		}
	}
}

// `-c` empties the list and the numbering starts again from one.
func TestHistoryClearRestartsTheNumbering(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runBash(t, dir, "history -s a\nhistory -c\nhistory\n"); out != "" || st != 0 {
		t.Errorf("out = %q status = %d, want nothing at 0", out, st)
	}
	if out, _ := runBash(t, dir, "history -s a\nhistory -c\nhistory -s b\nhistory\n"); out != "    1  b\n" {
		t.Errorf("out = %q, want the numbering to start again", out)
	}
}

// `-d` counts from one at both ends, and zero is out of range.
//
// Zero is the case worth having: a shell that treated the list as zero-based
// would delete the first entry there and pass every other row here.
func TestHistoryDeleteIsOneBasedAtBothEnds(t *testing.T) {
	t.Parallel()
	if out, st := runBash(t, t.TempDir(), "history -s a\nhistory -s b\nhistory -d 1\nhistory\n"); out != "    1  b\n" || st != 0 {
		t.Errorf("out = %q status = %d, want the first entry gone at 0", out, st)
	}
	for _, offset := range []string{"0", "9"} {
		out, st := runBash(t, t.TempDir(), "history -s a\nhistory -d "+offset+"\n")
		if st != 1 {
			t.Errorf("-d %s: status = %d, want 1", offset, st)
		}
		if want := "history: " + offset + ": history position out of range\n"; !strings.HasSuffix(out, want) {
			t.Errorf("-d %s: out = %q, want it to end with %q", offset, out, want)
		}
	}
}

// A letter bash does not have is the complaint, the usage line, and **2** —
// where nearly every other refusal in this builtin is 1.
func TestHistoryRefusesAnUnknownLetterWithTheUsageLineAndTwo(t *testing.T) {
	t.Parallel()
	out, st := runBash(t, t.TempDir(), "history -Z\n")
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
	for _, want := range []string{
		"history: -Z: invalid option\n",
		"history: usage: history [-c] [-d offset] [n] or history -anrw [filename] or history -ps arg [arg...]\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("out = %q, want it to contain %q", out, want)
		}
	}
}

// With no operand the path is `$HISTFILE`, and with neither bash names the
// *variable* rather than saying an operand is missing.
//
// That is the right way round and worth pinning: the letter has a default,
// and what is wrong is that the default is unset.
func TestHistoryFallsBackToHistfileAndNamesItWhenItIsUnset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runBash(t, dir, "HISTFILE=h\nhistory -s z\nhistory -w\nwhile IFS= read -r l; do echo \"$l\"; done < h\n"); out != "z\n" || st != 0 {
		t.Errorf("out = %q status = %d, want the entry written through HISTFILE", out, st)
	}
	out, st := runBash(t, t.TempDir(), "unset HISTFILE\nhistory -s z\nhistory -w\n")
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
	if want := "history: HISTFILE: parameter null or not set\n"; !strings.HasSuffix(out, want) {
		t.Errorf("out = %q, want it to end with %q", out, want)
	}
}

// `-p` writes each operand back, and an operand carrying a `!` fails.
//
// This shell has no history expansion, and that is the answer rather than a
// gap in it: bash's own `-p` in a shell that cannot expand `!!` is
// `history expansion failed` at 1, which is the same sentence for the same
// reason. A word with no `!` expands to itself in both.
func TestHistoryPrintWritesOperandsBackAndFailsOnAnExpansion(t *testing.T) {
	t.Parallel()
	if out, st := runBash(t, t.TempDir(), "history -p foo bar\n"); out != "foo\nbar\n" || st != 0 {
		t.Errorf("out = %q status = %d, want both words at 0", out, st)
	}
	out, st := runBash(t, t.TempDir(), "history -p '!!'\n")
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
	if want := "history: !!: history expansion failed\n"; !strings.HasSuffix(out, want) {
		t.Errorf("out = %q, want it to end with %q", out, want)
	}
}

// A subshell gets its own list, which the store being a shell variable gives
// for free and which bash does too.
//
// Worth a case because it is a property of *where* the list is kept rather
// than of any letter: a package-level table would have leaked the child's
// entry back to the parent, and nothing else here would have noticed.
func TestASubshellsHistoryDoesNotReachTheParent(t *testing.T) {
	t.Parallel()
	out, st := runBash(t, t.TempDir(), "history -s outer\n(history -s inner)\nhistory\n")
	if want := "    1  outer\n"; out != want || st != 0 {
		t.Errorf("out = %q status = %d, want %q at 0", out, st, want)
	}
}
