// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// `[[ -o name ]]` reads the option this shell is holding, in both directions
// and through the same field `set` writes.
//
// One name and both states from one run, because asserting only the true side
// asserts that the operator answers *something* rather than that it answers
// the state — a `-o` that returned true unconditionally passes half of this.
func TestTheOptionTestReadsTheLiveState(t *testing.T) {
	const src = `set -e; [[ -o errexit ]]; echo "on=$?"; set +e; [[ -o errexit ]]; echo "off=$?"`
	out, st := run(t, src, nil)
	if want := "on=0\noff=1\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The operand is an ordinary word: expanded, unquoted, and never globbed.
//
// The last of those is the one worth pinning. Nothing inside `[[ ]]` is
// globbed, so `[[ -o err* ]]` asks about an option literally named `err*` and
// finds none — measured, and the same in all three shells that have the
// operator. A `-o` that matched names as patterns would answer true here.
func TestTheOptionTestsOperandIsAnOrdinaryWord(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			"out of a parameter",
			`set -u; v=nounset; [[ -o $v ]]; echo "st=$?"`,
			"st=0\n",
		},
		{
			"with its quotes taken off",
			`set -u; [[ -o 'nounset' ]]; echo "st=$?"`,
			"st=0\n",
		},
		{
			"and not as a pattern",
			`set -u; [[ -o nouns* ]]; echo "st=$?"`,
			"st=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, nil); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A name this shell does not have, with the axis answered both ways on the
// same source.
//
// The No side is bash's and ksh93's: a quiet false, status 1, nothing said.
// The Yes side is zsh's: the complaint, and a status that is neither of the
// two a condition otherwise gives. Both sides carry on to the next command,
// which is what separates this from `set -o nosuchoption` in the same shell.
func TestAnUnknownConditionOptionHasTwoAnswers(t *testing.T) {
	const src = `[[ -o nosuchoption ]]; echo "st=$?"; echo alive`
	for _, tc := range []struct {
		name string
		ans  Answer
		want string
	}{
		{"a quiet false", No, "st=1\nalive\n"},
		{"a status of its own", Yes, "sh: no such option: nosuchoption\nst=3\nalive\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, src, func(r *Runner) {
				s := permissive()
				s.UnknownConditionOptionIsAStatus = tc.ans
				r.Semantics = &s
				d := Diagnostics{UnknownConditionOptionStatus: 3}
				r.Diagnostics = &d
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// What the divergent answer *is*, which the bare status hides: a third value
// rather than a false.
//
// This is the test the implementation was written against and the one a
// plausible shortcut fails. Returning `false` and painting the status on
// afterwards passes the row above and gets every row here wrong — `!` would
// turn the false into a true and answer 0, where the shell answers 3.
//
// Measured across the whole truth table on zsh 5.9.2, both operators and both
// sides of each. The two short-circuit rows are the other half: an operand
// that is never reached is never complained about, so the message's presence
// is evidence about evaluation order and not only about wording.
func TestAnUnknownConditionOptionIsNotAFalse(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			"negation leaves it alone",
			`[[ ! -o nosuchoption ]]; echo "st=$?"`,
			"sh: no such option: nosuchoption\nst=3\n",
		},
		{
			"twice negated, still",
			`[[ ! ! -o nosuchoption ]]; echo "st=$?"`,
			"sh: no such option: nosuchoption\nst=3\n",
		},
		{
			"`||` walks on past it to a true",
			`[[ -o nosuchoption || 1 == 1 ]]; echo "st=$?"`,
			"sh: no such option: nosuchoption\nst=0\n",
		},
		{
			"`||` walks on past it to a false, which is then the answer",
			`[[ -o nosuchoption || 1 == 2 ]]; echo "st=$?"`,
			"sh: no such option: nosuchoption\nst=1\n",
		},
		{
			"`&&` stops on it",
			`[[ 1 == 1 && -o nosuchoption ]]; echo "st=$?"`,
			"sh: no such option: nosuchoption\nst=3\n",
		},
		{
			"`&&` never reaches it after a false",
			`[[ 1 == 2 && -o nosuchoption ]]; echo "st=$?"`,
			"st=1\n",
		},
		{
			"`||` never reaches it after a true",
			`[[ 1 == 1 || -o nosuchoption ]]; echo "st=$?"`,
			"st=0\n",
		},
		{
			"a group does not change it",
			`[[ ( -o nosuchoption ) || 1 == 1 ]]; echo "st=$?"`,
			"sh: no such option: nosuchoption\nst=0\n",
		},
		{
			"two of them, and the right-hand one is the answer",
			`[[ -o nosuchoption || -o alsomissing ]]; echo "st=$?"`,
			"sh: no such option: nosuchoption\nsh: no such option: alsomissing\nst=3\n",
		},
		{
			"a known name after it answers for the whole",
			`set +e; [[ -o nosuchoption || -o errexit ]]; echo "st=$?"`,
			"sh: no such option: nosuchoption\nst=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, func(r *Runner) {
				s := permissive()
				s.UnknownConditionOptionIsAStatus = Yes
				r.Semantics = &s
				d := Diagnostics{UnknownConditionOptionStatus: 3}
				r.Diagnostics = &d
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A condition that is simply *false* is still a false, and `||` sees a false.
//
// The rows are quoted `=~` operands, which every shell in the panel matches
// literally rather than as a regular expression — so `[[ x =~ "[" ]]` is a
// plain 1 and not a compile failure, here and in bash and zsh alike. They pin
// that the status machinery below did not turn ordinary falsehood into
// something else: `!` flips these to 0 and `||` walks past them to the right.
func TestAnOrdinaryFalseIsStillAFalse(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"negated, it is a true", `[[ ! x =~ "[" ]]; echo "st=$?"`, "st=0\n"},
		{"`||` past it reaches the right side", `[[ x =~ "[" || 1 == 1 ]]; echo "st=$?"`, "st=0\n"},
		{"and takes the right side's false", `[[ x =~ "[" || 1 == 2 ]]; echo "st=$?"`, "st=1\n"},
		{"`&&` stops on it at 1", `[[ x =~ "[" && 1 == 1 ]]; echo "st=$?"`, "st=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, nil); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A condition that failed for a reason of its own is *not* carried by the
// rule the option test added: `||` returns it rather than walking on.
//
// This is the guard on that change, and the one row that reaches the other
// error the construct can produce — a `-t` operand that is not a number,
// under the axis that makes it a refusal.
//
// It also records a divergence this change did not introduce and did not
// close. bash walks on past its own refusal exactly as zsh walks on past the
// option test's — measured on bash 5.3.15, `[[ -t x || 1 == 1 ]]` is 0 there
// and `[[ ! -t x ]]` is 0, where this shell answers 2 to both. So the status
// algebra is more general than one operator, and `!` does not treat the two
// alike: bash's turns a refusal into a 0 where zsh's leaves the option test's
// 3 standing. Widening the rule here would have taken a guess at that second
// question, so the rule was left narrow and the difference written down.
func TestAConditionRefusedForItsOwnReasonIsNotCarriedOn(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			"`||` does not walk on past it",
			`[[ -t x || 1 == 1 ]]; echo "st=$?"`,
			"sh: [[: x: integer expected\nst=2\n",
		},
		{
			"and `!` does not flip it",
			`[[ ! -t x ]]; echo "st=$?"`,
			"sh: [[: x: integer expected\nst=2\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, nil); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A dialect may put a wider namespace behind the operator than its `set -o`
// names, and the substrate asks that namespace instead.
//
// Built here as a miniature rather than asserted through a shell, which is
// what keeps this package free of dialect names: the seam takes a function, so
// a test can supply one that folds a spelling the `set -o` table would never
// match and watch the operator find it.
//
// The second result is what the axis turns on, so a namespace that said
// `false, true` for everything would silence the complaint rather than answer
// the question. Both halves are asserted: a name it claims, and one it does
// not.
func TestTheOptionTestAsksTheDialectsNamespace(t *testing.T) {
	install := func(r *Runner) {
		s := permissive()
		s.UnknownConditionOptionIsAStatus = Yes
		r.Semantics = &s
		d := Diagnostics{UnknownConditionOptionStatus: 3}
		r.Diagnostics = &d
		r.SetOptionNamespace(func(name string) (on, known bool) {
			switch name {
			case "one_true_name":
				return true, true
			case "one_false_name":
				return false, true
			}
			return false, false
		})
	}
	const src = `[[ -o one_true_name ]]; echo "t=$?"; ` +
		`[[ -o one_false_name ]]; echo "f=$?"; ` +
		`[[ -o errexit ]]; echo "shadowed=$?"`
	out, _ := run(t, src, install)
	// `errexit` is a `set -o` name and the namespace does not claim it, so it
	// is unknown here — the namespace *replaces* the lookup rather than
	// layering over it, which is what lets a dialect answer for every name it
	// has in one place.
	want := "t=0\nf=1\nsh: no such option: errexit\nshadowed=3\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// With no namespace installed, the operator reads the `set -o` names — which
// is bash's shape and ksh93's, where the two sets are the same set.
func TestTheOptionTestFallsBackToTheSetOptionNames(t *testing.T) {
	const src = `[[ -o braceexpand ]]; echo "declared=$?"; [[ -o pipefail ]]; echo "undeclared=$?"`
	out, _ := run(t, src, func(r *Runner) {
		s := permissive()
		s.UnknownConditionOptionIsAStatus = No
		r.Semantics = &s
		// One of the two names declared, so the row says the operator reads
		// the dialect's declaration rather than the whole extras table.
		r.AddSetOptions("braceexpand")
	})
	if want := "declared=0\nundeclared=1\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// DialectOption is the same lookup a caller outside the language can make, and
// it must reach the namespace rather than the `set -o` names.
//
// The distinction is the whole of why it exists. A front end holding a setting
// a shell spells as an option — zsh's HIST_IGNORE_SPACE — asks this; if it
// read the `set -o` table instead, every name that lives only in the dialect's
// namespace would come back unknown, and the setting would be silently off
// with the shell reporting it on. `errexit` is the case that catches the
// mistake in the other direction too: a namespace that does not claim it must
// shadow the `set -o` name here exactly as it does for the condition.
func TestDialectOptionReadsTheNamespaceAndNotTheSetONames(t *testing.T) {
	var r *Runner
	// A Runner the package's own harness built, so this asks the shell a
	// script would get rather than one assembled here.
	run(t, "true", func(got *Runner) { r = got })
	if r == nil {
		t.Fatal("no runner")
	}
	// With nothing installed the `set -o` names answer, which is the fallback
	// half of the same method and is what a dialect with no namespace gets.
	if on, known := r.DialectOption("errexit"); !known || on {
		t.Errorf("before any namespace: errexit on=%v known=%v, want off and known", on, known)
	}
	r.SetOptionNamespace(func(name string) (on, known bool) {
		switch name {
		case "HIST_IGNORE_SPACE":
			return true, true
		case "hist_ignore_dups":
			return false, true
		}
		return false, false
	})
	for _, tc := range []struct {
		name      string
		on, known bool
		why       string
	}{
		{"HIST_IGNORE_SPACE", true, true, "a name only the namespace has, on"},
		{"hist_ignore_dups", false, true, "a name only the namespace has, off"},
		{"nobody_has_this", false, false, "a name nobody has"},
		// The namespace replaces the lookup rather than layering over it, so
		// a `set -o` name it does not claim is unknown through here — which is
		// what a dialect with its own namespace means by installing one.
		{"errexit", false, false, "a set -o name the namespace does not claim"},
	} {
		on, known := r.DialectOption(tc.name)
		if on != tc.on || known != tc.known {
			t.Errorf("%s: DialectOption(%q) = %v, %v; want %v, %v",
				tc.why, tc.name, on, known, tc.on, tc.known)
		}
	}
}
