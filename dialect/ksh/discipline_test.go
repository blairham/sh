// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A *discipline* function is this shell's alone, and it was refused outright
// here: every dotted function name met `invalid discipline function` and the
// refusal is fatal, so the definition took the rest of the input with it.
//
// The diagnostic and its fatality were already right — `function ns.thing` is
// that sentence in ksh93 too — and what was wrong was the set of names they
// fired on. Four suffixes name an event on a variable and define, whether or
// not the variable exists yet. Measured against AT&T ksh93u+ 2012-08-01,
// 2026-09-15 (#3033).
func TestTheFourDisciplineSuffixesDefineAndTheRestAreStillRefused(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{`g=raw; function g.get { :; }; echo defined`, "defined\n", 0},
		// And without the variable, which is the row that says the name in
		// front of the dot need not exist yet.
		{`function g.get { :; }; echo defined`, "defined\n", 0},
		{`function g.set { :; }; echo defined`, "defined\n", 0},
		{`function g.append { :; }; echo defined`, "defined\n", 0},
		{`function g.unset { :; }; echo defined`, "defined\n", 0},
		// The POSIX spelling defines one too.
		{`g.get() { :; }; echo defined`, "defined\n", 0},
		// The control, and the row that already agreed: a suffix that is not
		// an event is not a namespace here, it is an invalid discipline.
		{
			`function ns.thing { :; }; echo defined`,
			"sh: ns.thing: invalid discipline function\n", 1,
		},
		// A dash is the other sentence and is untouched by any of this.
		{
			`function f-g { :; }; echo defined`,
			"sh: f-g: invalid function name\n", 1,
		},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s:\n got %q (status %d)\nwant %q at %d",
				tc.src, out, st, tc.want, tc.status)
		}
	}
}

// A hook is stored as an ordinary function under its whole name, which is not
// only the cheap implementation — measured, ksh93 lists it with `typeset -f`,
// runs it when a command names it, and forgets the hook when `unset -f` takes
// the function away.
//
// `unset -f` judging the name is the same builtin's separate question, and it
// judges the *shape*: `unset -f ns.thing` is silent at 0 there — a name no
// definition would have accepted — where `unset -f 1x` and `unset -f f-g`
// are `invalid function name` at 1.
func TestADisciplineIsAnOrdinaryFunctionEverywhereElse(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		// No trailing newline, which is this shell's own one-line rendering
		// and byte-identical to ksh93's.
		{
			`g=raw; function g.get { :; }; typeset -f g.get`,
			"function g.get { :; };", 0,
		},
		{
			`g=raw; function g.get { echo hi; }; g.get; echo called`,
			"hi\ncalled\n", 0,
		},
		{
			`g=raw; function g.get { .sh.value=hooked; }; unset -f g.get; echo "[$g]"`,
			"[raw]\n", 0,
		},
		{`unset -f ns.thing; echo "st=$?"`, "st=0\n", 0},
		{
			`unset -f 1x; echo "st=$?"`,
			"sh: unset: 1x: invalid function name\nst=1\n", 0,
		},
		{
			`unset -f f-g; echo "st=$?"`,
			"sh: unset: f-g: invalid function name\nst=1\n", 0,
		},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s:\n got %q (status %d)\nwant %q at %d",
				tc.src, out, st, tc.want, tc.status)
		}
	}
}

// What each event carries, and the two answers that look like mistakes.
//
// `.get` is entered with `${.sh.value}` **empty** in 93u+ rather than with
// the value the read is about, so a hook written to decorate what is already
// there decorates nothing — and the pair of rows below is what makes that a
// measurement rather than a value comparison: a hook that assigns the empty
// string replaces the read, where one that assigns nothing does not.
//
// `.append` is given only the part being appended, and fires *instead of*
// `.set` rather than beside it.
func TestEachEventCarriesItsOwnValue(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			`g=raw; function g.get { .sh.value="[${.sh.value}]"; }; echo "$g"; echo "$g"`,
			"[]\n[]\n",
		},
		// The discriminator: an explicit empty assignment answers empty,
		// where a hook that assigns nothing lets the store through.
		{`g=raw; function g.get { .sh.value=""; }; echo "[$g]"`, "[]\n"},
		{`g=raw; function g.get { :; }; echo "[$g]"`, "[raw]\n"},
		// And the store is read *after* the hook, so a hook that assigns its
		// own variable is answered by the read that ran it.
		{`g=raw; function g.get { g=written; }; echo "[$g]"`, "[written]\n"},
		{
			`function s.set { .sh.value="<${.sh.value}>"; }; s=first; echo "$s"; s=second; echo "$s"`,
			"<first>\n<second>\n",
		},
		{
			`function n.set { printf 'name=[%s] value=[%s] sub=[%s]\n' "${.sh.name}" "${.sh.value}" "${.sh.subscript}"; }; n=plain; n[3]=indexed`,
			"name=[n] value=[plain] sub=[]\nname=[n] value=[indexed] sub=[3]\n",
		},
		{
			`p=base; function p.append { printf 'given [%s]\n' "${.sh.value}"; }; p+=more; echo "[$p]"`,
			"given [more]\n[basemore]\n",
		},
		{
			`p=base; function p.append { .sh.value="<${.sh.value}>"; }; p+=more; echo "[$p]"`,
			"[base<more>]\n",
		},
		// An append fires only its own event: a `.set` hook with no
		// `.append` beside it never runs for `p+=more`.
		{`p=base; function p.set { echo SET; }; p+=more; echo "[$p]"`, "[basemore]\n"},
		// And a plain assignment does not reach `.append`.
		{`p=base; function p.append { echo APPEND; }; p=plain; echo "[$p]"`, "[plain]\n"},
		{
			`u=here; function u.unset { printf 'still [%s]\n' "$u"; }; unset u; printf 'after [%s]\n' "${u-gone}"`,
			"still [here]\nafter [gone]\n",
		},
		// A name nothing has set does not go away, so nothing fires.
		{`function u.unset { echo UNSET; }; unset u; echo done`, "done\n"},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s:\n got %q (status %d)\nwant %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A hook does not re-enter itself, and the guard is per *event* rather than
// per variable — which is the distinction a single-hook probe cannot make.
//
// A `.get` that reads its own variable reads the store; a `.get` that
// *assigns* its variable does fire that variable's `.set`.
func TestAHookDoesNotReEnterItselfButItsSiblingsStillFire(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			`g=raw; function g.get { printf 'self=[%s]\n' "$g"; }; echo "[$g]"`,
			"self=[raw]\n[raw]\n",
		},
		{
			`g=raw; function g.get { g=written; }; function g.set { printf 'SET [%s]\n' "${.sh.value}"; }; echo "[$g]"`,
			"SET [written]\n[written]\n",
		},
		{`function s.set { s=other; }; s=x; echo "[$s]"`, "[x]\n"},
		{`u=here; function u.unset { unset u; }; unset u; echo "[${u-gone}]"`, "[gone]\n"},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s:\n got %q (status %d)\nwant %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A read is not a command, so `.get` leaves `$?` where it found it. `.set` is
// the opposite half of that measurement in ksh93 and is not modeled yet — see
// the note on the issue.
func TestAReadHookLeavesTheStatusAlone(t *testing.T) {
	out, st := answersRun(t, `true; g=raw; function g.get { false; }; echo "[$g]"; echo "st=$?"`)
	if out != "[raw]\nst=0\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[raw]\nst=0\n")
	}
}

// `${.sh.fun}` is the function running now and `${.sh.level}` its depth, and
// both expanded to nothing here — which made `0` at the top level
// indistinguishable from the shell not having the name at all.
//
// The old measurement that left them out was taken at the top level, where
// they really are empty and `0`; it could not tell the two readings apart.
func TestTheRunningFunctionAndItsDepthAnswer(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			`function outer { printf 'fun=[%s] level=[%s]\n' "${.sh.fun}" "${.sh.level}"; }; outer`,
			"fun=[outer] level=[1]\n",
		},
		{
			`function outer { inner; }; function inner { printf 'fun=[%s] level=[%s]\n' "${.sh.fun}" "${.sh.level}"; }; outer`,
			"fun=[inner] level=[2]\n",
		},
		// The POSIX spelling answers too, and counts the same.
		{
			`posixform() { printf 'fun=[%s] level=[%s]\n' "${.sh.fun}" "${.sh.level}"; }; posixform`,
			"fun=[posixform] level=[1]\n",
		},
		// A `name()` function called from a `function` one is the innermost,
		// which is where this differs from `$0` in this shell.
		{
			`function a { b; }; b() { printf 'fun=[%s] level=[%s]\n' "${.sh.fun}" "${.sh.level}"; }; a`,
			"fun=[b] level=[2]\n",
		},
		{`printf 'fun=[%s] level=[%s]\n' "${.sh.fun}" "${.sh.level}"`, "fun=[] level=[0]\n"},
		// And inside a discipline, which is a function like any other.
		{
			`g=raw; function g.get { printf 'fun=[%s] level=[%s]\n' "${.sh.fun}" "${.sh.level}"; }; echo "[$g]"`,
			"fun=[g.get] level=[1]\n[raw]\n",
		},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s:\n got %q (status %d)\nwant %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The three parameters a hook is entered with exist only while it runs, and
// the shell's own values come back afterwards — which is what lets one hook
// run inside another without the outer one losing its `${.sh.value}`.
//
// They are ordinary dotted variables the rest of the time, which is what the
// first and last lines are: a script may assign `.sh.value` and find it
// where it left it.
func TestTheDisciplineParametersAreRestoredAfterTheHook(t *testing.T) {
	src := `.sh.value=mine
g=raw
function g.get { printf 'in=[%s][%s]\n' "${.sh.value}" "${.sh.name}"; }
echo "[$g]"
printf 'out=[%s][%s]\n' "${.sh.value}" "${.sh.name}"`
	out, st := answersRun(t, src)
	want := "in=[][g]\n[raw]\nout=[mine][]\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}
