// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// A signal held until the shell reads more input, in the front end's own run
// loop — see Semantics.SelfAimedWindowChangeWaitsForInputOrAChild, which
// carries the panel and both arrival points.
//
// It is tested here and not in `interp` because the moment it waits for is
// the front end's: the loop that hands the runner one *unit* of the program
// at a time is the thing that decides where "the next unit of input" falls,
// and a test that ran the whole file through one call could not tell a unit
// from a line.
//
// The axis is named and no shell is. Both answers are run for every row, so
// what the rows show is the difference between them rather than one shell's
// output.
func heldWinchShell(a interp.Answer) driver.Shell {
	sh := shell()
	sh.Semantics.SelfAimedWindowChangeWaitsForInputOrAChild = a
	// The two axes every trap body reaches on its way to running at all,
	// answered so that a test about *when* a handler runs is not stopped by
	// a question about what it sees. Both are unanimous-enough here that
	// either answer would do: nothing below reads `$?` in a body, and every
	// body parses.
	sh.Semantics.SignalHandlerSeesEarlierStatus = interp.No
	sh.Semantics.TrapBodyRunsWhatParsed = interp.No
	// And how `eval` reads its text, for the row that is about `eval`. Yes
	// is what the sourced-file question is already answered with above, and
	// nothing here has a later line that fails to parse, so the answer
	// decides nothing but whether the question is put.
	sh.Semantics.EvalRunsWhatItParsed = interp.Yes
	// And where that text's lines sit, which the `eval` row reaches because
	// its text is more than one line. Nothing below reads a line number, so
	// either answer would do.
	sh.Semantics.EvalTextContinuesTheCallersLines = interp.No
	// A real child needs a real program to be one. The environment is
	// otherwise empty here, so `PATH` is given rather than inherited: a
	// child found on the developer's own path would pass here and fail on a
	// runner.
	sh.Env = []string{"PATH=/usr/bin:/bin"}
	return sh
}

const heldWinchTrap = "trap 'echo W' WINCH\n"

func heldWinchFile(t *testing.T, a interp.Answer, body string) string {
	t.Helper()
	out, errs, code := runArgs(t, heldWinchShell(a), "testsh", writeScript(t, heldWinchTrap+body))
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	return strings.Join(strings.Fields(out), " ")
}

// A `;`-separated line is one unit however many statements stand on it, and
// the shell reads no more of the file until it is done — so the handler waits
// for the read that finds the end.
func TestAHeldSignalWaitsForTheNextUnitOfInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		yes  string
		no   string
	}{
		{
			"one line and then the end of the file",
			"kill -WINCH $$; echo a; echo b\n",
			"a b W", "W a b",
		},
		{
			"the line after the one that sent it",
			"kill -WINCH $$; echo a\necho b\n",
			"a W b", "W a b",
		},
		// The row that separates **the physical line** from **the unit of
		// input**: the `kill` and the `echo a` are on different lines and
		// nothing fires between them, because the whole loop is read in one
		// go. A rule keyed on the line would put `W` between them.
		{
			"a loop spanning lines is still one unit",
			"for i in 1\ndo\nkill -WINCH $$\necho a\ndone\necho b\n",
			"a W b", "W a b",
		},
		// And the same shape written as a brace group, which the reference
		// answers identically — so the rule is the unit and not the loop.
		{
			"and so is a brace group",
			"{\nkill -WINCH $$\necho a\n}\necho b\n",
			"a W b", "W a b",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := heldWinchFile(t, interp.Yes, tc.body); got != tc.yes {
				t.Errorf("held: got %q, want %q", got, tc.yes)
			}
			if got := heldWinchFile(t, interp.No, tc.body); got != tc.no {
				t.Errorf("prompt: got %q, want %q", got, tc.no)
			}
		})
	}
}

// The other arrival point, and it is inside the unit: the handler runs when
// the shell has finished waiting for a child.
//
// Three answers are distinguishable here and all three are written out, which
// is why the row has an `echo one` in front of the child. Waiting for the
// child gives `one W a b`; holding only for input gives `one a W b`; running
// between commands gives `W one a b`. The child writes nothing at all, so the
// order is the shell's own and not a race between two writers on one stream —
// the child's output goes to the null device for exactly that reason.
func TestAHeldSignalAlsoArrivesWhenAChildHasBeenWaitedFor(t *testing.T) {
	const body = "kill -WINCH $$; echo one; /bin/echo x >/dev/null; echo a\necho b\n"
	if got := heldWinchFile(t, interp.Yes, body); got != "one W a b" {
		t.Errorf("held: got %q, want %q", got, "one W a b")
	}
	if got := heldWinchFile(t, interp.No, body); got != "W one a b" {
		t.Errorf("prompt: got %q, want %q", got, "W one a b")
	}
}

// A command string is not input the shell reads. The whole program was in
// hand before the shell started, however many lines it is spread over, so
// neither moment ever comes and the handler runs not at all.
//
// This is the row that says the rule is the *read* rather than the statement
// boundary, and it is the one a fix keyed on "the end of a top-level
// statement" would fail.
func TestAHeldSignalNeverArrivesUnderACommandString(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"on one line", "kill -WINCH $$; echo a; echo b\n"},
		{"spread over lines", "kill -WINCH $$\necho a\necho b\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, code := runArgs(t, heldWinchShell(interp.Yes), "testsh", "-c", heldWinchTrap+tc.src)
			if code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs)
			}
			if got := strings.Join(strings.Fields(out), " "); got != "a b" {
				t.Errorf("got %q, want %q", got, "a b")
			}
			out, _, _ = runArgs(t, heldWinchShell(interp.No), "testsh", "-c", heldWinchTrap+tc.src)
			if got := strings.Join(strings.Fields(out), " "); got != "W a b" {
				t.Errorf("prompt: got %q, want %q", got, "W a b")
			}
		})
	}
}

// A sourced file is input the shell reads, exactly as the program it was
// invoked with is — the same moment, in the other reader of shell input.
// Text handed to `eval` is not: it was in hand before it began, as a command
// string was, so a signal held inside it waits for the caller's next line.
func TestAHeldSignalArrivesAtTheNextLineOfASourcedFileAndNotInsideAnEval(t *testing.T) {
	inc := writeScript(t, "kill -WINCH $$\necho s1\necho s2\n")
	sourced := heldWinchFile(t, interp.Yes, ". "+inc+"\necho b\n")
	if want := "W s1 s2 b"; sourced != want {
		t.Errorf("sourced: got %q, want %q", sourced, want)
	}
	evaled := heldWinchFile(t, interp.Yes, "eval 'kill -WINCH $$\necho e1\necho e2'\necho b\n")
	if want := "e1 e2 W b"; evaled != want {
		t.Errorf("eval: got %q, want %q", evaled, want)
	}
}

// And the control the whole file rests on: every other signal a script aims
// at the shell is unmoved under either answer, so this is one condition and
// not a class.
func TestAnotherSelfAimedSignalIsUnmovedByTheAxis(t *testing.T) {
	for _, a := range []interp.Answer{interp.Yes, interp.No} {
		out, errs, code := runArgs(t, heldWinchShell(a), "testsh",
			writeScript(t, "trap 'echo U' USR1\nkill -USR1 $$; echo a\necho b\n"))
		if code != 0 {
			t.Fatalf("status %d, stderr %q", code, errs)
		}
		if got := strings.Join(strings.Fields(out), " "); got != "U a b" {
			t.Errorf("%v: got %q, want %q", a, got, "U a b")
		}
	}
}
