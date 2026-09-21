// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// The size the list is capped at is **not** a reading of HISTSIZE: a value
// that is not a count leaves the last one that was in force, and only a
// negative, an empty value and `unset` take the cap off.
//
// Measured 2026-09-21 against bash 5.3.20 at `/opt/homebrew/bin/bash` — the
// panel's bash, not `/bin/bash`, which is 3.2 — with `env -i`, a scratch HOME
// and HISTIGNORE keeping the reader's own lines out of the list (#4054).
// Every want below is the byte-for-byte output of the same script under the
// real binary.
//
// The half that hides this is that a non-count assignment does not *trim*
// either, which already agreed here: the list right after `HISTSIZE=abc`
// looks the same whether the cap is still two or has been lifted, and the two
// only part company at the next entry. So every row below inserts after the
// assignment, and a test that only reads passes a shell with no cap at all.

// historyInForceQuiet keeps the reader's own lines out of the list. Its own
// file's copy rather than the one beside it, because the two tables ask
// different questions of it and a shared one would have to answer both.
const historyInForceQuiet = "HISTIGNORE='history*:HISTSIZE*:set*:unset*:" +
	"HISTIGNORE*:echo*:printf*'\n"

// historyInForceRun runs a script under those ignores, with an environment of
// the caller's own, and answers the whole listing, the diagnostics and the
// status.
//
// The environment is explicit — and empty for every row that does not need
// one — because the first size in force is the value the list is turned on
// with, so a HISTSIZE the machine running the test happens to export would be
// the thing under test.
//
// The whole listing, numbers included, because a containment check passes a
// list that grew past its cap: the entries the cap should have dropped are
// still there in front of the ones it kept.
func historyInForceRun(t *testing.T, env []string, src string) (string, string, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.sh")
	if err := os.WriteFile(path, []byte(historyInForceQuiet+src), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	shell := bashShell(&out, &errs)
	shell.Env = env
	code := driver.MainArgs(shell, []string{"bash", path})
	return out.String(), strings.ReplaceAll(errs.String(), path, "S"), code
}

// historyInForceListing is what `history` prints for entries numbered from
// first, in bash's own columns.
func historyInForceListing(first int, entries ...string) string {
	var b strings.Builder
	for i, entry := range entries {
		fmt.Fprintf(&b, "%5d  %s\n", first+i, entry)
	}
	return b.String()
}

// The issue's own grid. A list capped at two, nine entries deep, then a value
// that is not a count and two more entries: the cap is still two.
func TestANonCountHistsizeKeepsTheSizeInForce(t *testing.T) {
	t.Parallel()
	// `HISTSIZE=2` and nine entries leave `8 h`, `9 i`; the two that follow
	// are the tenth and eleventh whatever the cap does with them.
	held := historyInForceListing(10, "j", "k")
	lifted := historyInForceListing(8, "h", "i", "j", "k")
	for _, c := range []struct{ name, value, want string }{
		{"letters are not a count", "abc", held},
		{"nor are digits with text after them", "2x", held},
		{"nor is hexadecimal, which bash does not read here", "0x2", held},
		{"nor a value too large to hold", "99999999999999999999", held},
		{"nor a value that is only whitespace", `" "`, held},
		// And the three that do say something, which are the control: an
		// implementation that lifted the cap for everything would pass these
		// and only these.
		{"a negative lifts the cap", "-1", lifted},
		{"and so does an empty value", "", lifted},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, errs, code := historyInForceRun(t, []string{},
				"set -o history\nHISTSIZE=2\n"+historyStifleAdds(9)+
					"HISTSIZE="+c.value+"\nhistory -s j\nhistory -s k\nhistory\n")
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("HISTSIZE=%s: out %q err %q status %d, want %q, no diagnostic and 0",
					c.value, out, errs, code, c.want)
			}
		})
	}
	t.Run("and unset lifts it", func(t *testing.T) {
		t.Parallel()
		out, errs, code := historyInForceRun(t, []string{},
			"set -o history\nHISTSIZE=2\n"+historyStifleAdds(9)+
				"unset HISTSIZE\nhistory -s j\nhistory -s k\nhistory\n")
		if out != lifted || errs != "" || code != 0 {
			t.Errorf("out %q err %q status %d, want %q, no diagnostic and 0", out, errs, code, lifted)
		}
	})
}

// The variable keeps the text while the list is held at the older size, which
// is what says the size is state beside the list rather than a reading of
// HISTSIZE. Both halves in one test, because either alone is satisfied by a
// shell that stores the size *into* the variable.
func TestTheSizeInForceIsNotReadableFromHistsize(t *testing.T) {
	t.Parallel()
	out, errs, code := historyInForceRun(t, []string{},
		"set -o history\nHISTSIZE=2\n"+historyStifleAdds(9)+
			"HISTSIZE=abc\nhistory -s j\nhistory -s k\nhistory\necho \"[$HISTSIZE]\"\n")
	want := historyInForceListing(10, "j", "k") + "[abc]\n"
	if out != want || errs != "" || code != 0 {
		t.Errorf("out %q err %q status %d, want %q, no diagnostic and 0", out, errs, code, want)
	}
}

// One non-count does not spend it: a second and a third leave the same cap.
func TestTheSizeInForceSurvivesEveryNonCountAfterIt(t *testing.T) {
	t.Parallel()
	out, errs, code := historyInForceRun(t, []string{},
		"set -o history\nHISTSIZE=2\n"+historyStifleAdds(9)+
			"HISTSIZE=abc\nHISTSIZE=2x\nHISTSIZE=0x2\nhistory -s j\nhistory -s k\nhistory\n")
	want := historyInForceListing(10, "j", "k")
	if out != want || errs != "" || code != 0 {
		t.Errorf("out %q err %q status %d, want %q, no diagnostic and 0", out, errs, code, want)
	}
}

// A lift is remembered exactly as a count is, which is the row that says this
// is not "fall back to the last number anybody wrote". After `-1`, an empty
// value or `unset`, a later non-count leaves the list growing.
func TestALiftIsWhatTheNextNonCountFindsInForce(t *testing.T) {
	t.Parallel()
	want := historyInForceListing(2, "b", "c", "d", "e", "p", "q")
	for _, c := range []struct{ name, line string }{
		{"a negative", "HISTSIZE=-1"},
		{"an empty value", "HISTSIZE="},
		{"and unset", "unset HISTSIZE"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, errs, code := historyInForceRun(t, []string{},
				"set -o history\nHISTSIZE=2\n"+historyStifleAdds(3)+c.line+
					"\nhistory -s d\nhistory -s e\nHISTSIZE=abc\n"+
					"history -s p\nhistory -s q\nhistory\n")
			if out != want || errs != "" || code != 0 {
				t.Errorf("%s: out %q err %q status %d, want %q, no diagnostic and 0",
					c.line, out, errs, code, want)
			}
		})
	}
	// And with nothing at all between the lift and the non-count, which is
	// the shape a size kept anywhere but the assignment cannot see: no entry
	// is inserted in between for a lazy reading to notice.
	out, errs, code := historyInForceRun(t, []string{},
		"set -o history\nHISTSIZE=2\nunset HISTSIZE\nHISTSIZE=abc\n"+
			historyStifleAdds(4)+"history\n")
	if w := historyInForceListing(1, "a", "b", "c", "d"); out != w || errs != "" || code != 0 {
		t.Errorf("unset then abc with nothing between: out %q err %q status %d, want %q",
			out, errs, code, w)
	}
}

// Raising the size again after a non-count is an ordinary assignment: the new
// count is in force and it trims where it stands.
func TestACountAfterANonCountIsInForceAgain(t *testing.T) {
	t.Parallel()
	out, errs, code := historyInForceRun(t, []string{},
		"set -o history\nHISTSIZE=4\n"+historyStifleAdds(6)+
			"HISTSIZE=abc\nHISTSIZE=2\nhistory\n")
	// Six entries under a cap of four leave `3 c` … `6 f`; the trim to two
	// numbers what it keeps from the count it dropped.
	want := historyInForceListing(2, "e", "f")
	if out != want || errs != "" || code != 0 {
		t.Errorf("out %q err %q status %d, want %q, no diagnostic and 0", out, errs, code, want)
	}
}

// The first size in force is the one the list is turned on with, and there
// are three ways for it to arrive. None of them is an assignment this shell
// heard after the list existed, which is what makes them worth pinning apart.
func TestTheFirstSizeInForceIsTheOneTheListWasTurnedOnWith(t *testing.T) {
	t.Parallel()

	t.Run("a value assigned before the list was turned on", func(t *testing.T) {
		t.Parallel()
		out, errs, code := historyInForceRun(t, []string{},
			"HISTSIZE=2\nHISTSIZE=abc\nset -o history\n"+historyStifleAdds(3)+"history\n")
		if w := historyInForceListing(2, "b", "c"); out != w || errs != "" || code != 0 {
			t.Errorf("out %q err %q status %d, want %q", out, errs, code, w)
		}
	})

	t.Run("a value inherited from the environment", func(t *testing.T) {
		t.Parallel()
		out, errs, code := historyInForceRun(t, []string{"HISTSIZE=2"},
			"set -o history\nHISTSIZE=abc\n"+historyStifleAdds(3)+"history\n")
		if w := historyInForceListing(2, "b", "c"); out != w || errs != "" || code != 0 {
			t.Errorf("out %q err %q status %d, want %q", out, errs, code, w)
		}
	})

	t.Run("and a value that was never a count leaves no cap", func(t *testing.T) {
		t.Parallel()
		out, errs, code := historyInForceRun(t, []string{"HISTSIZE=abc"},
			"set -o history\n"+historyStifleAdds(4)+"history\n")
		if w := historyInForceListing(1, "a", "b", "c", "d"); out != w || errs != "" || code != 0 {
			t.Errorf("out %q err %q status %d, want %q", out, errs, code, w)
		}
	})
}

// The default the list is turned on with is a size like any other, and a
// non-count afterwards holds the list at it.
//
// Five hundred entries rather than a paraphrase, because 500 and "no cap at
// all" are the same listing for every shorter list — which is exactly the
// reading this rules out. `unset` first is the control: the same script
// without a size in force keeps all 520.
func TestTheDefaultFiveHundredStaysInForceAfterANonCount(t *testing.T) {
	t.Parallel()
	var adds strings.Builder
	entries := make([]string, 0, 520)
	for i := range 520 {
		fmt.Fprintf(&adds, "history -s e%d\n", i)
		entries = append(entries, fmt.Sprintf("e%d", i))
	}

	out, errs, code := historyInForceRun(t, []string{},
		"set -o history\nHISTSIZE=abc\n"+adds.String()+"history\n")
	// Twenty of the 520 fell off the front, so the oldest kept is numbered
	// 21 and the newest 520.
	want := historyInForceListing(21, entries[20:]...)
	if out != want || errs != "" || code != 0 {
		t.Errorf("the default held: %d lines, err %q status %d, want %d lines starting %q",
			strings.Count(out, "\n"), errs, code, strings.Count(want, "\n"),
			strings.SplitN(want, "\n", 2)[0])
	}

	out, errs, code = historyInForceRun(t, []string{},
		"set -o history\nunset HISTSIZE\nHISTSIZE=abc\n"+adds.String()+"history\n")
	want = historyInForceListing(1, entries...)
	if out != want || errs != "" || code != 0 {
		t.Errorf("unset first: %d lines, err %q status %d, want %d lines starting %q",
			strings.Count(out, "\n"), errs, code, strings.Count(want, "\n"),
			strings.SplitN(want, "\n", 2)[0])
	}
}

// A `local HISTSIZE` is the one route back to a size nothing assigned: the
// restore is not an assignment, so the size in force after the call is read
// off the variable the frame put back rather than remembered from the local.
//
// bash trims at the restore where this shell trims at the next entry (#4045);
// what is pinned here is the *cap*, which agrees.
func TestASizeRestoredWithAFrameIsInForceAgain(t *testing.T) {
	t.Parallel()
	out, errs, code := historyInForceRun(t, []string{},
		"set -o history\nHISTSIZE=9\n"+historyStifleAdds(3)+
			// The function's own two lines out of the way as well, so that
			// the listing is the entries and not the reader.
			strings.TrimSuffix(historyInForceQuiet, "'\n")+":fn*'\n"+
			"fn() { local HISTSIZE=2; history -s x; }\nfn\n"+
			"history -s d\nhistory -s e\nhistory -s g\nhistory\n")
	// The local's trim leaves `2 c`, `3 x`; the restored nine then holds
	// everything that follows.
	want := historyInForceListing(2, "c", "x", "d", "e", "g")
	if out != want || errs != "" || code != 0 {
		t.Errorf("out %q err %q status %d, want %q, no diagnostic and 0", out, errs, code, want)
	}
}
