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
