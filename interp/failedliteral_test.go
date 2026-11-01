// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// failedLiteralSem is a vector that reaches every literal store here, with
// the failed subscripted literal's answer chosen.
func failedLiteralSem(keeps Answer) Semantics {
	s := PosixSemantics()
	s.FailedExpansionAbandonsTheLine = Yes
	s.FatalErrorStatusIsOne = Yes
	s.FailedSubscriptedLiteralKeepsWhatItStored = keeps
	return s
}

// An array literal stops expanding at the element that would not expand: the
// elements after it never run. Measured 2026-10-15 in bash 5.3.20, zsh 5.9.2
// and ksh93u+ alike, and the same for an append, a subscripted literal, a
// table's pairs and a declaration's operand; a declaration's operand also
// reports the failure once, where the store used to expand the list again and
// report it twice (#6606).
func TestALiteralStopsAtTheElementThatFails(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"a=(1 2)\n(a=(5 $(echo SIDE >&2) $((1/0)) $(echo AFTER >&2)))\necho \"[${a[*]}]\"\n",
		"a=(1 2)\n(a+=(5 $(echo SIDE >&2) $((1/0)) $(echo AFTER >&2)))\necho \"[${a[*]}]\"\n",
		"a=(1 2)\n(a=([3]=$(echo SIDE >&2) [4]=$((1/0)) [5]=$(echo AFTER >&2)))\necho \"[${a[*]}]\"\n",
		"a=(1 2)\n(typeset -A m\nm=([k]=$(echo SIDE >&2) [j]=$((1/0)) [l]=$(echo AFTER >&2)))\necho \"[${a[*]}]\"\n",
		"a=(1 2)\n(typeset a=(5 $(echo SIDE >&2) $((1/0)) $(echo AFTER >&2)))\necho \"[${a[*]}]\"\n",
		"a=(1 2)\n(a=(5 $(echo SIDE >&2) ${u?boom} $(echo AFTER >&2)))\necho \"[${a[*]}]\"\n",
	} {
		out, _ := run(t, src, withSem(failedLiteralSem(No)))
		if strings.Contains(out, "AFTER") || strings.Count(out, "SIDE\n") != 1 ||
			strings.Count(out, "division by zero")+strings.Count(out, "boom") != 1 ||
			!strings.HasSuffix(out, "[1 2]\n") {
			t.Errorf("%q: said %q, want SIDE, one complaint, no AFTER and the name kept", src, out)
		}
	}
}

// A subscripted literal that fails partway through leaves the pairs it had
// placed, as a table, in the column that builds it as it goes; the other
// answer keeps what the name held. An append onto an indexed array keeps
// nothing under either answer, and a literal opening with a bare word is never
// asked (#6608).
func TestAFailedSubscriptedLiteralKeepsWhatItStoredIsTheAxis(t *testing.T) {
	const show = "s() { echo \"[${a[*]}] [${a[0]-}] [${a[5]-}] [${a[1]-}]\"; }\n"
	for _, c := range []struct{ name, src, keeps, not string }{
		{
			"an assignment", "a=([0]=x [5]=y [1]=$((1/0)))",
			"[x y] [x] [y] []", "[1 2] [1] [] [2]",
		},
		{
			"the first element", "a=([$((1/0))]=v)",
			"[] [] [] []", "[1 2] [1] [] [2]",
		},
		{
			"an indexed append", "a+=([5]=x [6]=$((1/0)))",
			"[1 2] [1] [] [2]", "[1 2] [1] [] [2]",
		},
		{
			"a bare first word", "a=(x [5]=$((1/0)))",
			"[1 2] [1] [] [2]", "[1 2] [1] [] [2]",
		},
	} {
		for _, keeps := range []Answer{Yes, No} {
			src := show + "a=(1 2)\n" + c.src + "\ns\n"
			// The failure ends the script, so an exit trap reads what is
			// left.
			src = "trap s EXIT\n" + src
			out, _ := run(t, src, withSem(failedLiteralSem(keeps)))
			want := c.not
			if keeps == Yes {
				want = c.keeps
			}
			if !strings.HasSuffix(out, want+"\n") || strings.Contains(out, "disagree") {
				t.Errorf("%s, keeps=%v: said %q, want it to end %q", c.name, keeps, out, want)
			}
		}
	}
}

// A table that a failed subscripted append was adding to keeps what it added
// under the answer that streams, and only then.
func TestAFailedSubscriptedTableAppendKeepsItsPairs(t *testing.T) {
	src := "trap 'echo \"[${m[k]}] [${m[j]-unset}]\"' EXIT\ntypeset -A m\nm=([k]=old)\n" +
		"m+=([j]=new [i]=$((1/0)))\n"
	for _, c := range []struct {
		keeps Answer
		want  string
	}{{Yes, "[old] [new]\n"}, {No, "[old] [unset]\n"}} {
		if out, _ := run(t, src, withSem(failedLiteralSem(c.keeps))); !strings.HasSuffix(out, c.want) {
			t.Errorf("keeps=%v: said %q, want it to end %q", c.keeps, out, c.want)
		}
	}
}

// With the answer unchosen, a literal that expands asks nothing, and so does
// one that fails without a subscript at its head.
func TestALiteralThatExpandsAsksNothingAboutAFailedOne(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"a=([0]=x [1]=y); echo \"${a[*]}\"",
		"a=(1 2); (a=(x $((1/0)))); echo \"${a[*]}\"",
	} {
		out, _ := run(t, src, withSem(failedLiteralSem(Unspecified)))
		if strings.Contains(out, "disagree") {
			t.Errorf("%s: said %q, want no question", src, out)
		}
	}
	out, _ := run(t, "a=(1 2); (a=([0]=x [1]=$((1/0))))", withSem(failedLiteralSem(Unspecified)))
	if !strings.Contains(out, "keeping what it had stored: the shells disagree") {
		t.Errorf("said %q, want the question", out)
	}
}
