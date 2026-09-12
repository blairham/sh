// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `unset -n` on a name that is not a name reference.
//
// The letter means "unset the reference itself rather than what it points at",
// and the two shells that have it disagree about a name that is not one: one
// reads it as naming nothing and removes nothing, the other removes the name
// like any other. Both report 0, so the value is the only way to tell them
// apart — which is why the corpus row that records who *takes* the letter
// deliberately does not print it (#932).
func TestUnsetReferenceLetterOnANameThatIsNotOne(t *testing.T) {
	for _, c := range []struct {
		name   string
		answer Answer
		src    string
		want   string
		status int
	}{
		{
			"removes it, like any other name", Yes,
			`x=1; unset -n x; echo "[${x-gone}]"`, "[gone]", 0,
		},
		{
			"removes nothing", No,
			`x=1; unset -n x; echo "[${x-gone}]"`, "[1]", 0,
		},
		{
			"several names, none of them removed", No,
			`x=1; y=2; unset -n x y; echo "[${x-gone}][${y-gone}]"`, "[1][2]", 0,
		},
		{
			// The reading reaches the name check as well: measured, the
			// shell that removes nothing is silent at 0 for a digit-led
			// operand where plain `unset 1x` refuses it.
			"a name that is not an identifier is not refused either", No,
			`unset -n 1x; echo "[done]"`, "[done]", 0,
		},
		{
			// And the one thing it does not excuse. UnsetReadonlyFatal is
			// answered No below so the refusal can be seen from the line
			// after it; whether it ends the script is that axis's question.
			"a readonly name is still refused", No,
			`readonly r=1; unset -n r; echo "[${r-gone}]"`, "[1]", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs := &strings.Builder{}, &strings.Builder{}
			sem := PosixSemantics()
			sem.UnsetOptions = "vfn"
			sem.UnsetReferenceLetterRemovesANonReference = c.answer
			sem.UnsetReadonlyFatal = No
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
				Stdout: out, Stderr: errs,
			})
			runCd(t, r, c.src)
			if got := strings.TrimSpace(out.String()); got != c.want {
				t.Errorf("printed %q, want %q (stderr %q)", got, c.want, errs.String())
			}
		})
	}
}

// A readonly name is refused under the letter, and the refusal is the same one
// `unset` makes without it: the status comes back and the diagnostic is
// written, rather than the letter turning the whole call into a no-op.
func TestUnsetReferenceLetterStillRefusesAReadonly(t *testing.T) {
	out, errs := &strings.Builder{}, &strings.Builder{}
	sem := PosixSemantics()
	sem.UnsetOptions = "vfn"
	sem.UnsetReferenceLetterRemovesANonReference = No
	sem.UnsetReadonlyFatal = No
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: errs,
	})
	runCd(t, r, `readonly r=1; unset -n r; echo "st=$?"`)
	if got := strings.TrimSpace(out.String()); got != "st=1" {
		t.Errorf("status %q, want st=1", got)
	}
	if !strings.Contains(errs.String(), "r") {
		t.Errorf("said %q, want a complaint naming the readonly", errs.String())
	}
}

// An unanswered axis refuses rather than guessing, which is what every axis
// does and what makes leaving one unset a statement rather than an oversight.
func TestUnsetReferenceLetterUnanswered(t *testing.T) {
	out, errs := &strings.Builder{}, &strings.Builder{}
	sem := PosixSemantics()
	sem.UnsetOptions = "vfn"
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: errs,
	})
	runCd(t, r, `x=1; unset -n x; echo "st=$? [${x-gone}]"`)
	if got := strings.TrimSpace(out.String()); got != "st=2 [1]" {
		t.Errorf("printed %q, want the refusal's status and an untouched name", got)
	}
	if !strings.Contains(errs.String(), "`unset -n` on a name that is not a name reference") {
		t.Errorf("said %q, want the axis named", errs.String())
	}
}
