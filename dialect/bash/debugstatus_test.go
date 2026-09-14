// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// The third thing `shopt -s extdebug` promises: a DEBUG action's **status**
// decides what runs next. Measured against bash 5.3.15 on 2026-09-14 under
// `env -i PATH=/usr/bin:/bin`, over `-c`, and every row below was run in both
// shells before it was written down.
//
// Three rules, and the reason there are three rather than the two #2476
// describes is that the issue's own probe cannot tell any of them apart. An
// action that refuses *every* command prints nothing whichever rule is in
// force — and prints nothing for a shell with no rule at all, which is what
// made the inert state look like agreement. Every row here refuses **one**
// firing, so a rule that fired at the wrong strength or in the wrong place
// changes the output.
//
// The discriminator is a counter rather than `$BASH_COMMAND`, which would be
// the natural way to name one command: that parameter was empty in this shell
// when these rows were written, and #2779 filled it in afterwards. They are
// left as they are rather than rewritten — a counter is sensitive to firing
// *order* in a way the name is not, so the row above each group pins the
// order the later rows index into, and a count that moved would otherwise
// silently re-aim every refusal.

// countingAction is a DEBUG action that refuses the nth firing with the given
// status and lets every other firing through. Written as shell rather than as
// a helper so that the same text runs in the reference.
func countingAction(nth, status string) string {
	return `shopt -s extdebug; n=0; d(){ n=$((n+1)); [ $n = ` + nth +
		` ] && return ` + status + `; return 0; }; trap d DEBUG; `
}

// TestADebugActionsStatusSkipsTheCommandItFiredFor is the first rule: any
// non-zero status, not only the 2 the issue names.
func TestADebugActionsStatusSkipsTheCommandItFiredFor(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		// The order the rows below index into. Four firings for three
		// commands, the fourth being the `trap -` that turns it off, and
		// bash 5.3.15 counts the same four.
		{
			"the firing order these rows index into",
			`n=0; trap 'n=$((n+1))' DEBUG; echo one; echo two; echo three; trap - DEBUG; echo "n=$n"`,
			"one\ntwo\nthree\nn=4\n",
		},
		// 1, 2 and 5 all skip, which is the whole of the first rule and is
		// the half the issue has backwards. A reading where 2 alone skipped
		// would print `one two three` for the first and third rows here.
		{"a status of 1 skips", countingAction("2", "1") + `echo one; echo two; echo three`, "one\nthree\n"},
		{"a status of 2 skips", countingAction("2", "2") + `echo one; echo two; echo three`, "one\nthree\n"},
		{"a status of 5 skips", countingAction("2", "5") + `echo one; echo two; echo three`, "one\nthree\n"},
		// And 0 is not a refusal, which is what keeps every row above from
		// passing for a shell that skips unconditionally.
		{"a status of 0 runs it", countingAction("2", "0") + `echo one; echo two; echo three`, "one\ntwo\nthree\n"},
		// What the skipped command leaves behind is 0 rather than the
		// action's status — measured, and the row that would catch a
		// implementation that let the action's status leak into `$?`.
		{
			"the next command sees 0 and not the action's status",
			countingAction("3", "5") + `false; echo one; echo two; echo "st=$?"`,
			"one\nst=0\n",
		},
		// The rule is the option's, so it is off when the option is.
		{
			"nothing is skipped without extdebug",
			`n=0; d(){ n=$((n+1)); [ $n = 2 ] && return 5; return 0; }; trap d DEBUG; echo one; echo two; echo three`,
			"one\ntwo\nthree\n",
		},
		{
			"and off again when the option goes off",
			countingAction("2", "5") + `shopt -u extdebug; echo one; echo two; echo three`,
			"one\ntwo\nthree\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runTraced(t, c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
}

// TestADebugActionsStatusOfTwoReturnsFromACall is the second rule, and the
// rows are chosen so that no two of the three readings agree on all of them.
func TestADebugActionsStatusOfTwoReturnsFromACall(t *testing.T) {
	const body = `g(){ echo g1; echo g2; echo g3; }; g; echo "g-st=$?"`
	for _, c := range []struct {
		name, src, want string
	}{
		// The order again. Firing 1 is the call, 2 is the frame being
		// entered, and 3, 4 and 5 are the body's own commands — so a
		// refusal at 4 lands on `echo g2`, with a command either side of it
		// inside the same call.
		{
			"the firing order these rows index into",
			`n=0; trap 'n=$((n+1))' DEBUG; shopt -s extdebug; ` + `g(){ echo g1; echo g2; echo g3; }; g; trap - DEBUG; echo "n=$n"`,
			"g1\ng2\ng3\nn=7\n",
		},
		// 2 inside a call returns from it *at 2*, so `echo g3` never runs
		// and the caller reads 2.
		{"2 returns from the call at 2", countingAction("4", "2") + body, "g1\ng-st=2\n"},
		// 1 and 5 at the same firing skip one command and the call runs on
		// to report 0. This is the pair that parts the second rule from the
		// first: a reading where every non-zero returned would print `g1`
		// and `g-st=1` here, and one where nothing returned would print
		// `g1 g3 g-st=0` for the row above.
		{"1 skips one command and the call carries on", countingAction("4", "1") + body, "g1\ng3\ng-st=0\n"},
		{"5 does the same", countingAction("4", "5") + body, "g1\ng3\ng-st=0\n"},
		// At the top level there is no frame to return from, so a 2 is only
		// ever a skip — which is the row that stops the second rule being
		// written as "a 2 unwinds".
		{
			"a 2 at the top level only skips",
			countingAction("2", "2") + `echo one; echo two; echo three; echo "st=$?"`,
			"one\nthree\nst=0\n",
		},
		// Nested, it returns from the frame it fired in and no further.
		// Firing 7 is `echo i2`, the inner body's second command, so `i1`
		// is written before the return — which is what says the call was
		// entered and left rather than never entered.
		{
			"it returns from the inner call alone",
			countingAction("7", "2") +
				`inner(){ echo i1; echo i2; }; outer(){ echo o1; inner; echo "in-st=$?"; echo o2; }; outer; echo "out-st=$?"`,
			"o1\ni1\nin-st=2\no2\nout-st=0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runTraced(t, c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
}

// TestTheEntryFiringReturnsWithTheActionsOwnStatus is the third rule, which
// the checkpoint this work continues had not measured and which is invisible
// to every probe in #2476.
//
// The firing with the frame already pushed answers differently from every
// other firing inside the same call: any non-zero returns, and it carries the
// action's own status out rather than the 2 the rule next door is fixed at.
// Measured on bash 5.3.15, 2026-09-14 — 1, 2, 5 and 7 give `g-st=1`,
// `g-st=2`, `g-st=5` and `g-st=7`.
func TestTheEntryFiringReturnsWithTheActionsOwnStatus(t *testing.T) {
	const body = `g(){ echo in; }; g; echo "g-st=$?"; echo after`
	for _, c := range []struct {
		name, src, want string
	}{
		// Firing 2 is the entry. A non-zero there leaves the body unrun and
		// the call reporting that status — so these three rows differ from
		// each other, which is what says the status is carried rather than
		// replaced by a constant.
		{"1 at the entry firing returns 1", countingAction("2", "1") + body, "g-st=1\nafter\n"},
		{"2 at the entry firing returns 2", countingAction("2", "2") + body, "g-st=2\nafter\n"},
		{"5 at the entry firing returns 5", countingAction("2", "5") + body, "g-st=5\nafter\n"},
		// Firing 1 is the call itself, at the caller's level, and a refusal
		// there is an ordinary skip: the call never happens and `$?` is 0.
		// This is the row that parts the entry rule from the first rule —
		// the two firings are one command apart and answer differently.
		{"1 at the call firing skips and leaves 0", countingAction("1", "1") + body, "g-st=0\nafter\n"},
		// And it returns from the entered call alone. Firing 5 is the
		// inner call's entry; firing 4 is the `inner` command one step
		// earlier, at outer's level, and the pair is what parts the two
		// rules from each other inside one script — 5 there carries out,
		// 4 there is swallowed into an ordinary skip.
		{
			"nested, the inner call alone",
			countingAction("5", "5") +
				`inner(){ echo i1; }; outer(){ echo o1; inner; echo "in-st=$?"; echo o2; }; outer; echo "out-st=$?"`,
			"o1\nin-st=5\no2\nout-st=0\n",
		},
		{
			"the call firing one step earlier is only a skip",
			countingAction("4", "5") +
				`inner(){ echo i1; }; outer(){ echo o1; inner; echo "in-st=$?"; echo o2; }; outer; echo "out-st=$?"`,
			"o1\nin-st=0\no2\nout-st=0\n",
		},
		// The outer call's own entry, which returns from `outer` and takes
		// everything with it.
		{
			"the outer entry returns from outer",
			countingAction("2", "5") +
				`inner(){ echo i1; }; outer(){ echo o1; inner; echo "in-st=$?"; echo o2; }; outer; echo "out-st=$?"`,
			"out-st=5\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runTraced(t, c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
}

// TestHowFarARefusedFiringReaches is the part the rule does not decide: what
// a refusal *costs* is the firing site's answer, and the sites disagree.
//
// A simple command and a compound head that stands for its whole construct
// lose the construct. A loop's per-pass head loses that pass and the loop
// carries on. The arithmetic loop splits again over its own three parts.
// Every row measured against bash 5.3.15 on 2026-09-14.
func TestHowFarARefusedFiringReaches(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		// A list loop's head fires once per pass, so refusing one costs one
		// pass. Three passes rather than two on purpose: with two, the last
		// pass is the one refused and "skip this pass" and "end the loop"
		// print the same thing. That non-discriminating shape is what hid
		// the answer here for an afternoon.
		{
			"the firing order these rows index into",
			`n=0; trap 'n=$((n+1)); echo f$n' DEBUG; for i in 1 2 3; do echo b$i; done; trap - DEBUG`,
			"f1\nf2\nb1\nf3\nf4\nb2\nf5\nf6\nb3\nf7\n",
		},
		{
			"a refused pass head costs that pass alone",
			countingAction("3", "1") + `for i in 1 2 3; do echo b$i; done; echo after`,
			"b1\nb3\nafter\n",
		},
		{
			"and a refused body command costs the same",
			countingAction("4", "1") + `for i in 1 2 3; do echo b$i; done; echo after`,
			"b1\nb3\nafter\n",
		},
		// A `case` head stands for the whole construct, so refusing it
		// costs the branch as well.
		{
			"a refused case head costs the branch",
			countingAction("1", "1") + `case a in a) echo hit;; esac; echo after`,
			"after\n",
		},
		// A `while` writes no head of its own in this reading — the
		// condition is the firing — so refusing it leaves the condition
		// unrun at 0 and the body runs. Counter-intuitive and measured.
		{
			"a refused while condition leaves 0 and runs the body",
			countingAction("1", "1") + `while false; do echo x; done; echo after`,
			"x\nafter\n",
		},
		// The arithmetic loop's three parts, which answer three ways. The
		// order row first: initializer, condition, body, then step,
		// condition, body per pass.
		{
			"the arithmetic firing order",
			`n=0; trap 'n=$((n+1)); echo f$n' DEBUG; for ((i=0;i<3;i++)); do echo b$i; done; trap - DEBUG`,
			"f1\nf2\nf3\nb0\nf4\nf5\nf6\nb1\nf7\nf8\nf9\nb2\nf10\nf11\nf12\n",
		},
		// A refused initializer is simply not evaluated: `i` is never set,
		// so the first pass writes a bare `b`, and the loop runs its three
		// passes regardless.
		{
			"a refused initializer is not evaluated and the loop runs",
			countingAction("1", "1") + `for ((i=0;i<3;i++)); do echo b$i; done; echo after`,
			"b\nb1\nb2\nafter\n",
		},
		// A refused condition ends the loop, which is the one of the three
		// that does.
		{
			"a refused condition ends the loop",
			countingAction("2", "1") + `for ((i=0;i<3;i++)); do echo b$i; done; echo after`,
			"after\n",
		},
		{
			"and ends it wherever it is refused",
			countingAction("5", "1") + `for ((i=0;i<3;i++)); do echo b$i; done; echo after`,
			"b0\nafter\n",
		},
		// A refused step is not evaluated either, so the pass repeats with
		// `i` where it was — `b0` twice — and then carries on normally.
		// This row is what parts "not evaluated" from "skipped": a skip of
		// the whole pass would print `b0` once.
		{
			"a refused step is not evaluated and the pass repeats",
			countingAction("4", "1") + `for ((i=0;i<3;i++)); do echo b$i; done; echo after`,
			"b0\nb0\nb1\nb2\nafter\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runTraced(t, c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
}
