// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// The list has a size, and assigning it trims what is already there — with
// the entries keeping the numbers they had.
//
// Measured 2026-09-21 against zsh 5.9.2 with `env -i`, a scratch HOME and no
// startup files, over lists built with `print -s` and read with `fc -l`.
// Every want below is the byte-for-byte output of the same script under the
// real binary, and every row was run side by side with it (#4043).
//
// **The numbering is where this dialect parts company with bash**, which
// renumbers what a trim keeps from the count it dropped: the same three
// entries under `HISTSIZE=2` list as `1 b`, `2 c` there and `2 b`, `3 c`
// here. A fix that took the other shell's rule across would keep the right
// entries under the wrong numbers, so every want here is a whole listing.

// fcAdds is `print -s` over the first n of `a`, `b`, `c`, …
func fcAdds(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "print -s %c\n", 'a'+i)
	}
	return b.String()
}

func TestAssigningTheListSizeTrimsAndLeavesTheNumbersAlone(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the control: no assignment at all",
			fcAdds(3) + "fc -l\n",
			"    1  a\n    2  b\n    3  c\n",
		},
		{
			"the issue's own case",
			fcAdds(3) + "HISTSIZE=2\nfc -l\n",
			"    2  b\n    3  c\n",
		},
		{
			"the cap on the way in keeps the numbers the same way",
			"HISTSIZE=2\n" + fcAdds(3) + "fc -l\n",
			"    2  b\n    3  c\n",
		},
		{
			"a size the list is already under leaves it alone",
			fcAdds(3) + "HISTSIZE=3\nfc -l\n",
			"    1  a\n    2  b\n    3  c\n",
		},
		{
			"a trim and then an entry go on from where the trim left off",
			fcAdds(5) + "HISTSIZE=2\nprint -s f\nfc -l\n",
			"    5  e\n    6  f\n",
		},
		{
			"a larger size afterwards does not bring anything back",
			fcAdds(3) + "HISTSIZE=2\nHISTSIZE=9\nprint -s d\nfc -l\n",
			"    2  b\n    3  c\n    4  d\n",
		},
		{
			"and a removal leaves the size that was in force",
			fcAdds(3) + "HISTSIZE=2\nunset HISTSIZE\nprint -s d\nprint -s e\nprint -s f\nfc -l\n",
			"    5  e\n    6  f\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, st := fcScript(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// A value that is not a count is **not** "keep everything" here, which is the
// other half of the same measurement: `abc`, `-1` and `0` all leave one
// entry, where bash leaves the list alone for the first two and empties it
// for the third. The parameter is arithmetic with a floor of one.
func TestTheListSizeIsArithmeticWithAFloorOfOne(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, value, want string }{
		{"a count", "2", "    2  b\n    3  c\n"},
		{"whitespace around the digits", `" 2 "`, "    2  b\n    3  c\n"},
		{"an expression", "1+1", "    2  b\n    3  c\n"},
		{"a hex spelling, which this shell does read", "0x2", "    2  b\n    3  c\n"},
		{"a word is nothing, and nothing floors at one", "abc", "    3  c\n"},
		{"so does none", "0", "    3  c\n"},
		{"and so does a negative", "-1", "    3  c\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, st := fcScript(t, t.TempDir(), fcAdds(3)+"HISTSIZE="+c.value+"\nfc -l\n")
			if out != c.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// Every route into the list is bounded, not only the one `print -s` takes:
// four lines read into a list held at two move the numbering by four.
func TestReadingAFileIntoTheListIsBoundedToo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const seed = "printf '%s\\n' x y z w > rr\n"
	out, st := fcScript(t, dir, seed+"HISTSIZE=2\n"+fcAdds(3)+"fc -R rr\nfc -l\n")
	if want := "    6  z\n    7  w\n"; out != want || st != 0 {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
	// And an unbounded list numbers the same lines from one, which is the
	// control: the numbers above are the cap's doing and not the read's.
	out, st = fcScript(t, t.TempDir(), seed+"fc -R rr\nfc -l\n")
	if want := "    1  x\n    2  y\n    3  z\n    4  w\n"; out != want || st != 0 {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// The size before anything sets one, which is what bounds a list in a shell
// that never mentions HISTSIZE at all.
func TestTheListHasASizeBeforeAnythingSetsOne(t *testing.T) {
	t.Parallel()
	// Thirty by default: forty entries leave the newest thirty, and the
	// oldest of them is numbered eleven.
	var adds strings.Builder
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&adds, "print -s e%d\n", i)
	}
	out, st := fcScript(t, t.TempDir(), adds.String()+"fc -l 1\n")
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 30 || st != 0 {
		t.Fatalf("%d entries at status %d, want 30 at 0:\n%s", len(lines), st, out)
	}
	if want := "   11  e11"; lines[0] != want {
		t.Errorf("the oldest is %q, want %q", lines[0], want)
	}
}

// A HISTSIZE the shell was **handed** is scanned rather than evaluated, which
// is a different reading of the same parameter and the reason the two are
// separate functions: `1+1` is two when a script assigns it and one when the
// environment carries it in.
func TestAnInheritedListSizeIsScannedRatherThanEvaluated(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ value, want string }{
		{"2x", "    2  b\n    3  c\n"},
		{" 2 ", "    2  b\n    3  c\n"},
		{"0x2", "    2  b\n    3  c\n"},
		{"1+1", "    3  c\n"},
		{"abc", "    3  c\n"},
		{"-1", "    3  c\n"},
		{"0", "    3  c\n"},
		{"", "    3  c\n"},
		{"99", "    1  a\n    2  b\n    3  c\n"},
	} {
		t.Run("["+c.value+"]", func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			out, st, err := preset.Combined(t, dialecttest.Base{
				Dir:  dir,
				Vars: map[string]string{"PATH": dir, "HISTSIZE": c.value},
			}, fcAdds(3)+"fc -l\n")
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != c.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// HISTSIZE is this shell's own parameter rather than a name a script has to
// invent: an integer with a base and a default, whose value is what the
// arithmetic made of the assignment, floored at one (#4093).
//
// The attribute is not decoration. It is what evaluates what is assigned, so
// the rows below and the sizing rows above are one rule and not two.
func TestTheListSizeIsAnIntegerParameterOfItsOwn(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"it is there, and it is thirty",
			`echo "[${HISTSIZE-U}]"` + "\n",
			"[30]\n",
		},
		{
			"and it describes as an integer in base ten",
			"typeset -p HISTSIZE\n",
			"typeset -i10 HISTSIZE=30\n",
		},
		{
			"an assignment is evaluated",
			`HISTSIZE=1+1; echo "[$HISTSIZE]"` + "\n",
			"[2]\n",
		},
		{
			"including a base prefix",
			`HISTSIZE=0x2; echo "[$HISTSIZE]"` + "\n",
			"[2]\n",
		},
		{
			"and whitespace around it",
			`HISTSIZE=" 2 "; echo "[$HISTSIZE]"` + "\n",
			"[2]\n",
		},
		{
			"a word evaluates to nothing, and nothing floors at one",
			`HISTSIZE=abc; echo "[$HISTSIZE]"` + "\n",
			"[1]\n",
		},
		{
			"so does none",
			`HISTSIZE=0; echo "[$HISTSIZE]"` + "\n",
			"[1]\n",
		},
		{
			"and so does a negative",
			`HISTSIZE=-1; echo "[$HISTSIZE]"` + "\n",
			"[1]\n",
		},
		{
			"and a removal takes the name away",
			`unset HISTSIZE; echo "[${HISTSIZE-U}]"` + "\n",
			"[U]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, st := fcScript(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// A value that is not an expression at all is the bad-math error, which ends
// a non-interactive shell — the row that says the attribute is the core's
// reading and not a second one written beside it.
func TestAnUnreadableListSizeIsTheBadMathError(t *testing.T) {
	t.Parallel()
	out, st := fcScript(t, t.TempDir(), `HISTSIZE=2x`+"\necho reached\n")
	if !strings.Contains(out, "bad math expression") {
		t.Errorf("out %q, want the bad-math error", out)
	}
	if strings.Contains(out, "reached") {
		t.Errorf("the shell carried on: %q", out)
	}
	if st == 0 {
		t.Errorf("status %d, want a failure", st)
	}
}

// An inherited HISTSIZE is what the list is bounded at *and* what the
// parameter holds: the scan happens before the attribute goes on, so a value
// the environment carried in is read rather than evaluated and never ends the
// shell.
func TestAnInheritedListSizeIsStoredAsWhatWasScanned(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ value, want string }{
		{"2x", "[2]\n"},
		{" 2 ", "[2]\n"},
		{"0x2", "[2]\n"},
		{"1+1", "[1]\n"},
		{"abc", "[1]\n"},
		{"-1", "[1]\n"},
		{"0", "[1]\n"},
		{"", "[1]\n"},
		{"99", "[99]\n"},
	} {
		t.Run("["+c.value+"]", func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			out, st, err := preset.Combined(t, dialecttest.Base{
				Dir:  dir,
				Vars: map[string]string{"PATH": dir, "HISTSIZE": c.value},
			}, `echo "[$HISTSIZE]"`+"\n")
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != c.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}
