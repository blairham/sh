// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runNamerefOff is runNameref with the refusal answered as the shell that
// reports and carries on, so a case can assert on the state behind it. The
// fatality itself is ReadonlyReassignmentFatal's question and is not what
// these are about.
func runNamerefOff(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := namerefAimSemantics()
	sem.ReadonlyReassignmentFatal = No
	sem.ReadonlyReassignmentByDeclarationFatal = No
	sem.ReadonlyReassignmentBySpecialBuiltinFatal = No
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamIndirection = true
		d.Herestring = true
	}, func(r *Runner) { r.Semantics = &sem })
}

// `typeset +n r` takes the reference away and leaves **the name it pointed at**
// behind as the value. See Runner.namerefAttributeRemoved, where the panel is.
//
// The letter did nothing at all here: the sign was read as "no `n` on this
// line" rather than as a declaration of its own, so the reference stood and
// every later read still went through it.
func TestTheReferenceLetterUnderAPlusTakesTheReferenceAway(t *testing.T) {
	// The value is the target's *name* and not what the target holds, which
	// is the row that says the reference was not followed on the way out.
	out, st := runNamerefOff(t, `v=bar; typeset -n foo=v
typeset +n foo; echo "st=$?"
typeset -p foo
echo "[$foo]"`)
	if !strings.Contains(out, `declare -- foo='v'`) {
		t.Errorf("got %q at %d, want the reference gone and the target's name left behind", out, st)
	}
	if !strings.Contains(out, "[v]") {
		t.Errorf("got %q, want a read of foo to answer v rather than bar", out)
	}
	if !strings.Contains(out, "st=0") {
		t.Errorf("got %q, want status 0", out)
	}

	// A name that is not a reference is an ordinary declaration, which is
	// what keeps the letter from meaning anything on its own.
	out, _ = runNamerefOff(t, `h=1; typeset +n h; echo "st=$?"; typeset -p h`)
	if !strings.Contains(out, `declare -- h='1'`) || !strings.Contains(out, "st=0") {
		t.Errorf("got %q, want h left exactly as it was", out)
	}

	// A reference with nothing to point at has nothing to leave behind, and
	// the attribute still goes: the next assignment writes the name itself
	// rather than aiming a reference.
	out, _ = runNamerefOff(t, `typeset -n g
typeset +n g
g=plain
echo "[$g]"
typeset -p g`)
	if !strings.Contains(out, "[plain]") || !strings.Contains(out, `declare -- g='plain'`) {
		t.Errorf("got %q, want the write to land on g itself", out)
	}
}

// A **frozen** reference may not be taken apart, which is the freeze the
// reference carries rather than the one its target carries — the same half
// `unset -n` asks, and the opposite of what a write through a reference asks.
func TestAFrozenReferenceRefusesTheLetterUnderAPlus(t *testing.T) {
	out, _ := runNamerefOff(t, `w=2; typeset -rn k=w
typeset +n k; echo "st=$?"
typeset -p k
echo "[$k]"`)
	if !strings.Contains(out, "readonly variable") {
		t.Errorf("got %q, want the frozen reference refused", out)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want status 1", out)
	}
	if !strings.Contains(out, `declare -nr k='w'`) {
		t.Errorf("got %q, want the reference still standing", out)
	}
	if !strings.Contains(out, "[2]") {
		t.Errorf("got %q, want a read of k to still go through to w", out)
	}
}
