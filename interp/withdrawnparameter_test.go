// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A withdrawn parameter reads as an ordinary unset name, and the producers are
// kept so that putting it back needs nothing to have been remembered.
//
// Each case asserts what an *expansion* does rather than what the accessor
// answers, because a seam that recorded the state and left the tables alone
// would pass an accessor test and is exactly the bug this exists to fix.
func TestAWithdrawnParameterReadsAsAnUnsetName(t *testing.T) {
	install := func(r *Runner) {
		r.SetDynamicAssoc("zzview", func(*Runner) AssocArray {
			return AssocArray{"k": "v"}
		})
		r.SetDynamicAssocElement("zzview", func(_ *Runner, key string) (string, bool) {
			return "v", key == "k"
		})
		r.MarkReadonly("zzview")
		r.MarkHidden("zzview")
	}
	const src = `echo "n=${#zzview[@]} all=[${zzview[@]}] set=${zzview[k]+yes} elem=[${zzview[k]}]"`

	out, _ := run(t, src, install)
	if want := "n=1 all=[v] set=yes elem=[v]\n"; out != want {
		t.Fatalf("registered: got %q, want %q", out, want)
	}

	out, _ = run(t, src, func(r *Runner) {
		install(r)
		r.SetParameterWithdrawn("zzview", true)
	})
	// All three readings go to the unset answer together. One of them saying
	// nothing while another answers is the shape of a seam that gated a read
	// path and missed one — the count, the set test and one element each
	// reach the tables by a different route.
	if want := "n=0 all=[] set= elem=[]\n"; out != want {
		t.Errorf("withdrawn: got %q, want %q", out, want)
	}

	out, _ = run(t, src, func(r *Runner) {
		install(r)
		r.SetParameterWithdrawn("zzview", true)
		r.SetParameterWithdrawn("zzview", false)
	})
	if want := "n=1 all=[v] set=yes elem=[v]\n"; out != want {
		t.Errorf("put back: got %q, want %q", out, want)
	}
}

// And it is an *ordinary* name, not a read-only one with nothing in it.
//
// A produced table is marked readonly so that a write cannot land in a stored
// table and shadow its own producer. With the producer gone there is nothing
// to shadow, and the mark would make an ordinary name refuse an ordinary
// assignment — which is a refusal for a name this shell is claiming not to
// have. The mark comes back with the producer.
func TestAWithdrawnParameterCanBeAssignedAndGetsItsMarkBack(t *testing.T) {
	install := func(r *Runner) {
		r.SetDynamicAssoc("zzview", func(*Runner) AssocArray { return AssocArray{"k": "v"} })
		r.MarkReadonly("zzview")
		r.MarkHidden("zzview")
	}
	out, _ := run(t, `zzview=mine; echo "[$zzview]"`, func(r *Runner) {
		install(r)
		r.SetParameterWithdrawn("zzview", true)
	})
	if want := "[mine]\n"; out != want {
		t.Errorf("withdrawn: got %q, want %q", out, want)
	}
	out, _ = run(t, `zzview=mine; echo "st=$?"`, func(r *Runner) {
		install(r)
		r.SetParameterWithdrawn("zzview", true)
		r.SetParameterWithdrawn("zzview", false)
	})
	if !strings.Contains(out, "readonly variable") {
		t.Errorf("put back: got %q, want the readonly mark restored", out)
	}
}

// It is not SetAbsentParameter, and the difference is what a read *says*. An
// absent parameter refuses by name; a withdrawn one is silent, because the
// shell this models never had anything to complain about — it was asked to
// put the parameter down.
func TestAWithdrawnParameterIsNotAnAbsentOne(t *testing.T) {
	out, st := run(t, `echo "n=${#zzview[@]}"`, func(r *Runner) {
		r.SetDynamicAssoc("zzview", func(*Runner) AssocArray { return AssocArray{"k": "v"} })
		r.SetAbsentParameter("zzother", "parameter not implemented yet")
		r.SetParameterWithdrawn("zzview", true)
	})
	if want := "n=0\n"; out != want || st != 0 {
		t.Errorf("withdrawn: got %q (status %d), want %q at 0", out, st, want)
	}
	out, _ = run(t, `echo "n=${#zzother[@]}"`, func(r *Runner) {
		r.SetAbsentParameter("zzother", "parameter not implemented yet")
	})
	if !strings.Contains(out, "not implemented yet") {
		t.Errorf("absent: got %q, want a refusal by name", out)
	}
}

// An absent parameter can be withdrawn too, and its refusal goes with it —
// which is what makes the seam about the *name* rather than about a producer.
// The sentence comes back when the name does.
func TestWithdrawingAnAbsentParameterSilencesItAndPuttingItBackSpeaksAgain(t *testing.T) {
	absent := func(r *Runner) { r.SetAbsentParameter("zzother", "parameter not implemented yet") }
	out, st := run(t, `echo "n=${#zzother[@]}"`, func(r *Runner) {
		absent(r)
		r.SetParameterWithdrawn("zzother", true)
	})
	if want := "n=0\n"; out != want || st != 0 {
		t.Errorf("withdrawn: got %q (status %d), want %q at 0", out, st, want)
	}
	out, _ = run(t, `echo "n=${#zzother[@]}"`, func(r *Runner) {
		absent(r)
		r.SetParameterWithdrawn("zzother", true)
		r.SetParameterWithdrawn("zzother", false)
	})
	if !strings.Contains(out, "not implemented yet") {
		t.Errorf("put back: got %q, want the refusal restored", out)
	}
}

// The state is per-Runner and goes into a subshell with the tables it took
// names out of, so a withdrawal made inside one does not reach back out.
func TestAParameterWithdrawalDoesNotEscapeASubshell(t *testing.T) {
	out, _ := run(t, `( echo "in=${#zzview[@]}" ); echo "out=${#zzview[@]}"`, func(r *Runner) {
		r.SetDynamicAssoc("zzview", func(*Runner) AssocArray { return AssocArray{"k": "v"} })
	})
	if want := "in=1\nout=1\n"; out != want {
		t.Fatalf("baseline: got %q, want %q", out, want)
	}
	out, _ = run(t, `( echo "in=${#zzview[@]}" ); echo "out=${#zzview[@]}"`, func(r *Runner) {
		r.SetDynamicAssoc("zzview", func(*Runner) AssocArray { return AssocArray{"k": "v"} })
		r.SetParameterWithdrawn("zzview", true)
	})
	if want := "in=0\nout=0\n"; out != want {
		t.Errorf("withdrawn before the subshell: got %q, want %q", out, want)
	}
}
