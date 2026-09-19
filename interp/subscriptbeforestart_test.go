// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runBeforeStart runs src with the two axes a *read* past the first element
// turns on: what the reach costs, and whether a name with no element has an
// end to count back from.
func runBeforeStart(t *testing.T, p SubscriptBeforeStartPolicy, needs Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.SubscriptBeforeTheFirstElementRead = p
		sem.SubscriptBeforeTheFirstElementNeedsAnElement = needs
		// And the length answered as the read is, which is what two of the
		// three columns with arrays do: the third is pinned in
		// TestTheLengthOfAnElementReachedPastIsItsOwnRefusal, so a row here
		// varies the read's own answer alone.
		sem.SubscriptBeforeTheFirstElementRefusesTheLength = No
		sem.ArrayBaseIsZero = Yes
		// The reach's fatal answer stops the shell at the dialect's generic
		// fatal status, so the status this test reads is that axis and not
		// this one. Pinned, so a change to it is not read as a change here.
		sem.FatalErrorStatusIsOne = Yes
		r.Semantics = &sem
	})
}

// The reach an assignment already refuses, read from the right of `=`. It was
// silently empty under every answer, so a script counting back off the end of
// an array was told nothing by the two shells that say something.
func TestASubscriptBeforeTheFirstElementIsAnsweredWhereTheDialectAnswersIt(t *testing.T) {
	const src = `a=(x y z); echo "[${a[-4]}]"; echo after`
	for _, c := range []struct {
		name   string
		policy SubscriptBeforeStartPolicy
		want   []string
		absent []string
		status int
	}{
		{
			"nothing is said", SubscriptBeforeStartIsNothing,
			[]string{"[]", "after"},
			[]string{"subscript"},
			0,
		},
		{
			"the complaint and the rest of the line", SubscriptBeforeStartIsReported,
			[]string{"subscript", "[]", "after"},
			nil, 0,
		},
		{
			"the script ends", SubscriptBeforeStartEndsTheScript,
			[]string{"subscript"},
			[]string{"after"},
			1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBeforeStart(t, c.policy, No, src)
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("got %q, want it to hold %q", out, w)
				}
			}
			for _, w := range c.absent {
				if strings.Contains(out, w) {
					t.Errorf("got %q, want no %q in it", out, w)
				}
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// Every read route goes through the one door, which is why the rule is under
// the element lookup rather than at each expansion: the length, the
// set-or-not test, the default, an expression and the conditional all reach
// the same subscript and the answering column complains once at each.
func TestEveryReadOfASubscriptBeforeTheFirstElementIsAnswered(t *testing.T) {
	for _, src := range []string{
		`a=(x y z); echo "[${a[-4]}]"`,
		`a=(x y z); echo "[${#a[-4]}]"`,
		`a=(x y z); echo "[${a[-4]+set}]"`,
		`a=(x y z); echo "[${a[-4]:-dflt}]"`,
		`a=(x y z); echo "[$(( a[-4] + 1 ))]"`,
	} {
		out, _ := runBeforeStart(t, SubscriptBeforeStartIsReported, No, src)
		if !strings.Contains(out, "bad array subscript") {
			t.Errorf("%s: got %q, want the reach reported", src, out)
		}
		if out, _ := runBeforeStart(t, SubscriptBeforeStartIsNothing, No, src); strings.Contains(out, "subscript") {
			t.Errorf("%s: got %q, want the silent answer to say nothing", src, out)
		}
	}
}

// A subscript that lands *on* the first element, or past the end, is not this
// question and must stay quiet under every answer — otherwise the complaining
// column would report the ordinary way of reading the last element.
func TestASubscriptInsideTheArrayIsNotTheReach(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x y z); echo "[${a[-3]}]"`, "[x]"},
		{`a=(x y z); echo "[${a[-1]}]"`, "[z]"},
		{`a=(x y z); echo "[${a[9]}]"`, "[]"},
	} {
		out, st := runBeforeStart(t, SubscriptBeforeStartEndsTheScript, No, c.src)
		if !strings.Contains(out, c.want) || strings.Contains(out, "subscript") || st != 0 {
			t.Errorf("%s: got %q (status %d), want %q and nothing said", c.src, out, st, c.want)
		}
	}
}

// Whether a name holding no element has an end to count back from. The two
// columns that complain disagree about it, and the row that shows it is an
// array that has been emptied — a name holding a plain string answers the
// same way, from the one place a string has.
func TestCountingBackFromANameWithNoElementsIsSilent(t *testing.T) {
	for _, src := range []string{
		`a=(); echo "[${a[-1]}]"; echo after`,
		`a=x; echo "[${a[-1]}]"; echo after`,
	} {
		out, st := runBeforeStart(t, SubscriptBeforeStartEndsTheScript, Yes, src)
		if strings.Contains(out, "subscript") || !strings.Contains(out, "after") || st != 0 {
			t.Errorf("%s: needs an element: got %q (status %d), want silence", src, out, st)
		}
		out, _ = runBeforeStart(t, SubscriptBeforeStartIsReported, No, src)
		if !strings.Contains(out, "bad array subscript") {
			t.Errorf("%s: no end needed: got %q, want the reach reported", src, out)
		}
	}
	// The control: with one element high up, the two answers coincide
	// exactly where the boundary is, so the axis is about the empty name and
	// not about how either side counts.
	out, _ := runBeforeStart(t, SubscriptBeforeStartEndsTheScript, Yes, `a[5]=q; echo "[${a[-6]}]"; echo after`)
	if !strings.Contains(out, "after") || strings.Contains(out, "subscript") {
		t.Errorf("in range: got %q, want the unassigned slot read silently", out)
	}
	if out, st := runBeforeStart(t, SubscriptBeforeStartEndsTheScript, Yes, `a[5]=q; echo "[${a[-7]}]"; echo after`); !strings.Contains(out, "subscript") || st != 1 {
		t.Errorf("one further: got %q (status %d), want the reach refused", out, st)
	}
}

// The **length** of the element the same reach was made for is its own
// refusal in one column, with its own subject and its own give-up (#3591).
//
// Measured 2026-09-18 against bash 5.3.20, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with stdin /dev/null, over `a=(x y z)`, `a=()`
// and `a=x` alike:
//
//	echo "[${a[-4]}]"; echo after    a: bad array subscript | [] | after
//	echo "[${#a[-4]}]"; echo after   [-4]: bad array subscript | after
//
// Two differences in one row and they go together: the subject is the
// subscript **as written**, with its brackets and without the name, and the
// refusal abandons the word rather than standing beside an empty value, so
// the `echo` never runs at all.
//
// **How far it gives up is the route's and not this axis's**, and the two
// routes are measured: from a script file the *command* is abandoned and
// `after` runs at 0, and under `-c` — one parse unit — nothing after it runs
// at all. This harness is the second shape, exactly as the neighboring
// TestTheLengthOfAnEmptyKeyMayBeRefused is, and the same pair of routes holds
// for that one.
//
// It is the exact shape
// Semantics.EmptyAssociativeKeyRefusesTheLength and
// Diagnostics.EmptyAssociativeKeyLength already record for `${#m[$w]}` under
// an empty key — same column, same split, same two subjects.
//
// The other two columns with arrays answer No, and for two different reasons:
// zsh is silent on both routes, and ksh93 gives the length the read's own
// sentence and ends the script on it.
func TestTheLengthOfAnElementReachedPastIsItsOwnRefusal(t *testing.T) {
	runLength := func(t *testing.T, refuses Answer, src string) (string, int) {
		t.Helper()
		return runGrammar(t, src, nil, func(r *Runner) {
			sem := *r.Semantics
			sem.SubscriptBeforeTheFirstElementRead = SubscriptBeforeStartIsReported
			sem.SubscriptBeforeTheFirstElementNeedsAnElement = No
			sem.SubscriptBeforeTheFirstElementRefusesTheLength = refuses
			sem.ArrayBaseIsZero = Yes
			sem.FatalErrorStatusIsOne = Yes
			r.Semantics = &sem
			r.Diagnostics = &Diagnostics{
				SubscriptBeforeTheFirstElementRead:   "%[1]s: bad array subscript",
				SubscriptBeforeTheFirstElementLength: "[%[1]s]: bad array subscript",
			}
		})
	}
	for _, src := range []string{
		`a=(x y z)`, `a=()`, `a=x`,
	} {
		full := src + `; echo "[${#a[-4]}]"; echo after`
		out, _ := runLength(t, Yes, full)
		if !strings.Contains(out, "[-4]: bad array subscript") {
			t.Errorf("%s: got %q, want the subscript as written as the subject", src, out)
		}
		// Abandoned rather than reported: the word does not stand beside an
		// empty value, and nothing else in this parse unit runs.
		if strings.Contains(out, "[0]") || strings.Contains(out, "after") {
			t.Errorf("%s: got %q, want the word abandoned", src, out)
		}
		// The read one line up is the control: same reach, the array's name
		// as the subject, and the value stands.
		out, _ = runLength(t, Yes, src+`; echo "[${a[-4]}]"; echo after`)
		if !strings.Contains(out, "a: bad array subscript") || !strings.Contains(out, "[]") {
			t.Errorf("%s: the read got %q, want the name as the subject and an empty value", src, out)
		}
		// And No is the read's own answer on both routes.
		out, _ = runLength(t, No, full)
		if !strings.Contains(out, "a: bad array subscript") || !strings.Contains(out, "[0]") {
			t.Errorf("%s: answered as the read, got %q, want the name and the length", src, out)
		}
	}
}

// A name holding nothing at all reaches the same door — which it did not,
// because the target lookup answers "the name holds nothing" before any
// element is counted and the subscript was then refused as text (#3591).
//
// Measured 2026-09-18 against bash 5.3.20: `unset a; echo "[${a[-1]}]"; echo
// after` is `a: bad array subscript`, then `[]`, then `after` — the same
// three lines `a=()` and `a=x` give in that shell, and this engine was silent
// on the third of them alone.
//
// **The length is not this route**, and that is measured rather than left
// out: `unset a; echo "[${#a[-1]}]"` is `[0]` and silent in the same shell,
// where the length over an existing name is the refusal above. A name that is
// not there has its length answered before the subscript is looked at.
func TestANameHoldingNothingReachesTheSameDoor(t *testing.T) {
	out, _ := runBeforeStart(t, SubscriptBeforeStartIsReported, No,
		`unset a; echo "[${a[-1]}]"; echo after`)
	if !strings.Contains(out, "bad array subscript") {
		t.Errorf("got %q, want the reach reported for a name holding nothing", out)
	}
	if !strings.Contains(out, "[]") || !strings.Contains(out, "after") {
		t.Errorf("got %q, want an empty value and the next command", out)
	}
	// The axis that withholds it is the one the emptied array and the scalar
	// already ask, and it answers this row too.
	out, _ = runBeforeStart(t, SubscriptBeforeStartIsReported, Yes,
		`unset a; echo "[${a[-1]}]"; echo after`)
	if strings.Contains(out, "subscript") {
		t.Errorf("got %q, want nothing said where the dialect needs an element", out)
	}
	// And the length of a name that is not there is answered before the
	// subscript is looked at, in every answer.
	out, _ = runBeforeStart(t, SubscriptBeforeStartIsReported, No,
		`unset a; echo "[${#a[-1]}]"; echo after`)
	if strings.Contains(out, "subscript") {
		t.Errorf("the length got %q, want nothing said", out)
	}
	if !strings.Contains(out, "[0]") {
		t.Errorf("the length got %q, want [0]", out)
	}
	// The silent answer stays silent, and costs no lookup at all.
	out, _ = runBeforeStart(t, SubscriptBeforeStartIsNothing, No,
		`unset a; echo "[${a[-1]}]"; echo after`)
	if strings.Contains(out, "subscript") {
		t.Errorf("silent: got %q, want nothing said", out)
	}
}
