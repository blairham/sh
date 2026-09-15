// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/blairham/sh/internal/pty"
	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// $COLUMNS and $LINES against a real terminal.
//
// A pseudo-terminal and not a fake, because the whole of what these two
// parameters say is what the *kernel* says: a test that handed the shell a
// number would be asserting that the number came back. It is also the only way
// to ask the question at all — a window that changes size is an ioctl on the
// other end of a pty, and nothing a snippet can write.
//
// **This terminal's own size is never read.** A test that asked how wide the
// window it is running in happens to be passes here and fails on a runner,
// where there is no window; every size below is set on the pair this test
// opened, which is the same discipline internal/terminfofixture keeps for
// `$terminfo`.

// windowShell is a shell whose standard input is the terminal end of a fresh
// pseudo-terminal, with $COLUMNS and $LINES provided.
//
// The resizer is returned with it, which is the half no snippet can reach. Output goes to a buffer rather than to the terminal: the terminal
// is what the shell *asks*, not what it writes to, and reading a pty back to
// see what a command printed is the part of this that would be flaky.
func windowShell(t *testing.T, rows, cols int) (*Runner, func(rows, cols int)) {
	t.Helper()
	control, terminal, err := pty.Open()
	if err != nil {
		if errors.Is(err, pty.ErrUnsupported) {
			t.Skip("no pseudo-terminals on this platform")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = control.Close()
		_ = terminal.Close()
	})
	resize := func(rows, cols int) {
		t.Helper()
		if err := pty.SetSize(terminal, rows, cols); err != nil {
			t.Fatal(err)
		}
	}
	resize(rows, cols)
	sem := testSemantics()
	d := syntax.Core()
	r := newTestRunner(t, &Runner{
		Stdin: terminal, Stdout: &output{}, Stderr: &output{},
		Semantics: &sem, Env: testPATH(), Dialect: &d,
	})
	r.ProvideWindowSize()
	return r, resize
}

// say runs one line on a shell that is already set up and returns what it
// printed, so a test can put a resize *between* two lines.
func say(t *testing.T, r *Runner, src string) string {
	t.Helper()
	var buf output
	r.Stdout = &buf
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return buf.String()
}

// The numbers are the terminal's, and both of them.
//
// Measured 2026-09-11 on zsh 5.9.2 under a pseudo-terminal opened 37 rows by
// 100 columns: `zsh -f -c 'print -r -- "$COLUMNS $LINES"'` is `100 37` — with
// no `-i`, which is the half that separates this shell's reading from bash's.
func TestWindowSizeFollowsTheTerminal(t *testing.T) {
	r, _ := windowShell(t, 37, 100)
	if got, want := say(t, r, `echo "[$COLUMNS][$LINES]"`), "[100][37]\n"; got != want {
		t.Errorf("got %q, want %q — the terminal is 100x37 and the shell must say so", got, want)
	}
}

// A window that changes size is read at the next read, and it is read whether
// or not anything has told the shell about it.
//
// The rule a value read once at startup passes every static test and fails:
// it is wrong from the moment somebody drags a corner, and wrong in silence,
// because the arithmetic it feeds still produces a number.
//
// Measured through a pseudo-terminal resized from 80x24 to 120x40 while the
// script was in a `sleep`, zsh 5.9.2 under plain `-c`: `80x24` before and
// `120x40` after. The same measurement with `trap "" WINCH` in front of it
// still reports `120x40`, which is why nothing here owns a signal.
func TestTheWindowSizeFollowsAResize(t *testing.T) {
	r, resize := windowShell(t, 24, 80)
	if got, want := say(t, r, `echo "[$COLUMNS][$LINES]"`), "[80][24]\n"; got != want {
		t.Fatalf("before: got %q, want %q", got, want)
	}
	resize(40, 120)
	if got, want := say(t, r, `echo "[$COLUMNS][$LINES]"`), "[120][40]\n"; got != want {
		t.Errorf("after a resize: got %q, want %q — the pair is read, not remembered", got, want)
	}
}

// What a script assigned stands until the window moves, and the window moving
// takes it away.
//
// Neither half is the obvious rule and each is wrong on its own: "the terminal
// always wins" loses the 13 that a prompt theme writes and then reads back,
// and "the assignment always wins" stops following the window for the rest of
// the session. Measured on zsh 5.9.2 under a pseudo-terminal, in one run:
//
//	print $COLUMNS   80        the terminal
//	COLUMNS=13
//	print $COLUMNS   13        the assignment
//	print $COLUMNS   13        and it is still the assignment
//	…resized to 120x40…
//	print $COLUMNS   120       and now it is not
//
// Both names are superseded together, because one resize moves both numbers.
func TestAnAssignedWindowSizeStandsUntilTheWindowMoves(t *testing.T) {
	r, resize := windowShell(t, 24, 80)
	if got, want := say(t, r, `echo "[$COLUMNS]"`), "[80]\n"; got != want {
		t.Fatalf("before: got %q, want %q", got, want)
	}
	if got, want := say(t, r, `COLUMNS=13; echo "[$COLUMNS][$COLUMNS]"`), "[13][13]\n"; got != want {
		t.Errorf("assigned: got %q, want %q — an assignment is what the next read gets", got, want)
	}
	resize(40, 120)
	if got, want := say(t, r, `echo "[$COLUMNS][$LINES]"`), "[120][40]\n"; got != want {
		t.Errorf("after a resize: got %q, want %q — the window moving takes the assignment away, both names", got, want)
	}
}

// The terminal is remembered once it has been seen, so a shell that has
// redirected every stream away still knows how wide its window is.
//
// Measured on zsh 5.9.2 under a 100-column pseudo-terminal:
//
//	zsh -f -c 'print $COLUMNS; v=$( { print -r -- "$COLUMNS" ; } 0</dev/null 2>/dev/null ); print -r -- "$v"'
//
// prints `100` twice, with not one of the three streams a terminal at the
// moment of the second read. A reader that only ever looked at the streams it
// holds says 0 there.
//
// The limit is stated as well: the remembering begins at the first read, so a
// shell that redirects everything away *before* it has ever asked has nothing
// to remember and answers 0, where zsh keeps the terminal it opened at
// startup. That is a narrower claim than zsh's and it is the one this shell
// makes.
func TestTheTerminalIsRememberedOnceItHasBeenSeen(t *testing.T) {
	r, _ := windowShell(t, 24, 80)
	if got, want := say(t, r, `echo "[$COLUMNS]"`), "[80]\n"; got != want {
		t.Fatalf("before: got %q, want %q", got, want)
	}
	// Every stream the shell holds is now something that is not a terminal,
	// which is what the redirections in the measured line amount to.
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = devNull.Close() }()
	r.Stdin, r.Stderr = devNull, &output{}
	if got, want := say(t, r, `echo "[$COLUMNS]"`), "[80]\n"; got != want {
		t.Errorf("with no stream a terminal: got %q, want %q — the window is still the one this shell found", got, want)
	}
}

// With no terminal anywhere the pair is nought, and an inherited value is kept
// instead.
//
// Nought is a value and not an absence, which is the difference a `-c` probe
// can see: `${COLUMNS-UNSET}` is `0` in zsh 5.9.2 and `UNSET` in the five
// columns without the parameter. And an inherited value is kept rather than
// overwritten — `COLUMNS=55 zsh -f -c 'print $COLUMNS'` is 55 — while a
// terminal, when there is one, wins over it: the same line under a 100-column
// pseudo-terminal is 100.
func TestWithNoTerminalTheWindowSizeIsNoughtOrWhatWasInherited(t *testing.T) {
	for _, tc := range []struct{ name, env, want string }{
		{"nothing inherited", "", "[0][0]\n"},
		{"inherited", "COLUMNS=55", "[55][0]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, `echo "[$COLUMNS][$LINES]"`, func(r *Runner) {
				if tc.env != "" {
					r.Env = append(r.Env, tc.env)
				}
				r.ProvideWindowSize()
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// ScreenSize is the *capability* reading of the window — what
// `$terminfo[cols]` answers — and it is deliberately not windowSizeValue with
// a different caller. Two of its rows say so and neither is shared.
//
// Here: the terminal wins over what a script assigned. Measured against zsh
// 5.9.2 through a pseudo-terminal opened 100 by 37, 2026-09-14 —
// `COLUMNS=77; print "${terminfo[cols]} $COLUMNS"` is `100 77`, so the
// assignment moved one name and not the other. `$COLUMNS` is asserted in the
// same run because without it a reader that ignored the assignment everywhere
// would pass.
func TestTheScreenSizeIsTheTerminalAndNotWhatAScriptAssigned(t *testing.T) {
	r, _ := windowShell(t, 37, 100)
	if rows, cols := r.ScreenSize(); rows != 37 || cols != 100 {
		t.Fatalf("before: %dx%d, want 37x100", rows, cols)
	}
	if got, want := say(t, r, `COLUMNS=77; LINES=9; echo "[$COLUMNS][$LINES]"`), "[77][9]\n"; got != want {
		t.Fatalf("the assignment did not move $COLUMNS: got %q, want %q", got, want)
	}
	if rows, cols := r.ScreenSize(); rows != 37 || cols != 100 {
		t.Errorf("after the assignment: %dx%d, want 37x100 — the window is what a capability answers", rows, cols)
	}
}

// And here: with no terminal at all it is the classic 80 by 24 rather than the
// nought `$COLUMNS` reports there, and an inherited value is taken ahead of
// both. Measured on zsh 5.9.2 with no terminal anywhere: `${terminfo[cols]}`
// is 80 where `$COLUMNS` is 0, and `COLUMNS=100 LINES=40` in the environment
// makes it 100 by 40.
//
// The second row is the control on the first: a reader that answered 80 by 24
// unconditionally passes the first and fails this one.
func TestTheScreenSizeWithNoTerminal(t *testing.T) {
	for _, tc := range []struct {
		name, env  string
		rows, cols int
		columns    string
	}{
		{name: "nothing inherited", rows: 24, cols: 80, columns: "[0][0]\n"},
		{
			name: "inherited", env: "COLUMNS=100",
			rows: 24, cols: 100, columns: "[100][0]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var seen *Runner
			out, _ := run(t, `echo "[$COLUMNS][$LINES]"`, func(r *Runner) {
				if tc.env != "" {
					r.Env = append(r.Env, tc.env)
				}
				r.ProvideWindowSize()
				seen = r
			})
			if out != tc.columns {
				t.Errorf("$COLUMNS and $LINES = %q, want %q", out, tc.columns)
			}
			if rows, cols := seen.ScreenSize(); rows != tc.rows || cols != tc.cols {
				t.Errorf("the screen size = %dx%d, want %dx%d", rows, cols, tc.rows, tc.cols)
			}
		})
	}
}

// A shell that was never given the capability has neither name, which is what
// keeps this off the five panel columns that do not have the parameters.
func TestWithoutTheCapabilityNeitherNameExists(t *testing.T) {
	out, _ := run(t, `echo "[${COLUMNS-UNSET}][${LINES-UNSET}]"`, nil)
	if want := "[UNSET][UNSET]\n"; out != want {
		t.Errorf("got %q, want %q — only the dialect that asks for these has them", out, want)
	}
}

// A terminal that is there and will not say how big it is is the classic
// 80x24, and that is not the same answer as no terminal at all.
//
// A pseudo-terminal created without a window size answers `TIOCGWINSZ` with
// 0x0 — what `pty.fork()` produces, what some CI runners hand a job, and what
// sits in the window between `forkpty` and the parent's `TIOCSWINSZ`.
// Measured 2026-09-13 on zsh 5.9.2 through exactly that: `COLUMNS=80
// LINES=24`, and `typeset -p COLUMNS` writes `typeset -i10 COLUMNS=80`. The
// same shell on a pipe reports 0, which is the row above.
//
// Taking the 0 literally is silent in the worst direction: every wrapping
// calculation still produces a number, and the redraw simply behaves as though
// nothing were known about the terminal (#2489).
func TestATerminalThatWillNotSayItsSizeIsEightyByTwentyFour(t *testing.T) {
	r, resize := windowShell(t, 0, 0)
	if got, want := say(t, r, `echo "[$COLUMNS][$LINES]"`), "[80][24]\n"; got != want {
		t.Errorf("got %q, want %q — a terminal answering 0x0 is not a shell without one", got, want)
	}
	// And it is a fallback rather than a reading, so a size arriving later
	// still wins: this is the `forkpty` window closing.
	resize(37, 100)
	if got, want := say(t, r, `echo "[$COLUMNS][$LINES]"`), "[100][37]\n"; got != want {
		t.Errorf("after the size arrives: got %q, want %q", got, want)
	}
}

// And what was inherited stands in front of the fallback, not behind it.
//
// Measured on the same 0x0 pseudo-terminal: `COLUMNS=55 LINES=9 zsh -f -c
// 'print $COLUMNS $LINES'` answers `55 9` rather than `80 24`. The order is
// what makes the fallback a last resort — a shell launched from one that knew
// its width still knows it.
func TestAnInheritedSizeBeatsTheFallbackForATerminalThatWillNotSay(t *testing.T) {
	r, _ := windowShell(t, 0, 0)
	r.Env = append(r.Env, "COLUMNS=55")
	if got, want := say(t, r, `echo "[$COLUMNS][$LINES]"`), "[55][24]\n"; got != want {
		t.Errorf("got %q, want %q — the environment answers first and the fallback fills the rest", got, want)
	}
}
