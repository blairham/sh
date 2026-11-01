// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Every answer of Semantics.GetoptsOptargOrder, read against the two stores
// it orders. Each row runs `getopts` once with its operand spelled OPTARG and
// once spelled `o`: the second run shows what the builtin's own OPTARG store
// left and what letter the operand got, so the first run must end holding
// one of them, and which one is the answer. Reading the expectation off the
// same vector keeps the rows free of every other OPTARG axis — empty or
// unset, cleared at the end or not — which this one does not decide.
func TestGetoptsOptargOrderPicksTheLastStore(t *testing.T) {
	t.Parallel()
	const show = `; echo "[${OPTARG-unset}]"`
	rows := []struct {
		name, spec, args string
		fault            bool
	}{
		{"an option's argument", "x:", "-x6", false},
		{"an argumentless option", "x", "-x", false},
		{"the end of the options", "x", "a", false},
		{"a silent unknown option", ":x", "-y", true},
		{"an unknown option", "x", "-y", true},
		{"a silent missing argument", ":x:", "-x", true},
		{"a missing argument", "x:", "-x", true},
	}
	for _, policy := range []GetoptsOptargOrderPolicy{
		GetoptsOptargBeforeTheName, GetoptsOptargAfterTheNameOnAnError, GetoptsOptargAfterTheName,
	} {
		for _, row := range rows {
			t.Run(policy.String()+"/"+row.name, func(t *testing.T) {
				sem := permissive()
				sem.GetoptsOptargOrder = policy
				// So the end of the options makes a store of its own to
				// order: where a dialect makes none there is nothing to put
				// after the name, and the `o` run cannot tell "unset by the
				// builtin" from "never set".
				sem.GetoptsUnsetsOptargAtEndOfOptions = Yes
				line := "getopts " + row.spec + " %s " + row.args + " 2>/dev/null"
				own, _ := run(t, strings.Replace(line, "%s", "o", 1)+show+`; echo "[$o]"`, withSem(sem))
				got, _ := run(t, strings.Replace(line, "%s", "OPTARG", 1)+show, withSem(sem))
				ownStore, letter, _ := strings.Cut(own, "\n")
				letter = strings.TrimSuffix(letter, "\n")
				want := letter + "\n"
				if policy == GetoptsOptargAfterTheName || (policy == GetoptsOptargAfterTheNameOnAnError && row.fault) {
					want = ownStore + "\n"
				}
				if got != want {
					t.Errorf("got %q, want %q (own store %s, letter %s)", got, want, ownStore, letter)
				}
			})
		}
	}
}

// The axis is asked only over an operand spelled OPTARG, where it can be
// seen: under CoreSemantics, which answers it neither way, an ordinary
// operand reaches no question — and the same line spelled OPTARG does, which
// is the control that says the first half could have failed.
func TestGetoptsOptargOrderIsAskedOnlyOverAnOperandSpelledOPTARG(t *testing.T) {
	t.Parallel()
	sem := CoreSemantics()
	out, _ := run(t, `getopts x: o -x6; echo "$o $OPTARG"`, withSem(sem))
	if out != "x 6\n" {
		t.Errorf("an ordinary operand: got %q, want the values and no refusal", out)
	}
	out, _ = run(t, `getopts x: OPTARG -x6; echo "$OPTARG"`, withSem(sem))
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("an operand spelled OPTARG: got %q, want the axis reported as unanswered", out)
	}
}

// A freeze on the name operand is the operand's, whatever it is spelled: in
// a dialect whose own OPTARG store writes through a freeze, an operand spelled
// OPTARG is still refused, as an operand spelled `o` is, while the builtin's
// own store beside an `o` goes through (#6498).
func TestAFrozenOperandSpelledOPTARGIsRefusedAsTheOperand(t *testing.T) {
	t.Parallel()
	sem := permissive()
	sem.GetoptsOwnParametersIgnoreAFreeze = Yes
	sem.GetoptsOptargOrder = GetoptsOptargBeforeTheName
	out, st := run(t, `readonly OPTARG; getopts x: OPTARG -x6; echo "st=$?"`, withSem(sem))
	if !strings.Contains(out, "OPTARG") || !strings.Contains(out, "st=2") {
		t.Errorf("operand spelled OPTARG: out=%q st=%d, want the refusal and status 2", out, st)
	}
	out, _ = run(t, `readonly OPTARG; getopts x: o -x6; echo "st=$? $o $OPTARG"`, withSem(sem))
	if out != "st=0 x 6\n" {
		t.Errorf("operand `o`: got %q, want the own store written through the freeze", out)
	}
}

// The order seen through a freeze, over an ordinary operand: where OPTARG is
// written after the name, a refused name ends the builtin before the OPTARG
// store and leaves OPTARG as the call found it; under either other answer the
// store was made first, or is still reached, and stands.
func TestARefusedNameLeavesOptargUnwrittenWhereOptargComesLast(t *testing.T) {
	t.Parallel()
	for _, policy := range []GetoptsOptargOrderPolicy{
		GetoptsOptargBeforeTheName, GetoptsOptargAfterTheNameOnAnError, GetoptsOptargAfterTheName,
	} {
		t.Run(policy.String(), func(t *testing.T) {
			sem := permissive()
			sem.GetoptsOptargOrder = policy
			sem.ReadonlyRefusalInABuiltinIsFatal = No
			sem.GetoptsRefusedWriteEndsTheBuiltin = Yes
			out, _ := run(t, `OPTARG=pre; readonly o; getopts x: o -x6 2>/dev/null; echo "$? ${OPTARG-unset}"`, withSem(sem))
			want := "2 6\n"
			if policy == GetoptsOptargAfterTheName {
				want = "2 pre\n"
			}
			if out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}
