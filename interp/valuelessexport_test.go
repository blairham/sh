// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a child is told for an exported name a valueless declaration has taken
// out of view. Two axes put a runner here and neither of them answers it: the
// name is hidden because a valueless declaration hides the outer value, and it
// is still exported because a local inherits the attribute. What the shell
// reads and what a command is told part company at that point, and only a real
// child can see the second.

// valuelessRun answers both axes, so every case here reaches the question
// rather than the refusal a runner with no dialect gives.
func valuelessRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.DeclaredNameWithoutValueIsEmpty = No
		s.ValuelessDeclarationHidesTheOuterValue = Yes
		s.LocalInheritsTheExportAttribute = Yes
		s.LocalOptions = "x"
	})
}

func TestAValuelessLocalStillHandsAChildTheNameItShadowed(t *testing.T) {
	const probe = `export FOO=bar; f() { echo "read=[${FOO-UNSET}]"; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`
	out, st := valuelessRun(t, strings.Replace(probe, "f() {", "f() { local FOO;", 1))
	if st != 0 || out != "read=[UNSET]\nFOO=bar\n" {
		t.Errorf("got %q status %d, want the shell reading it unset and the child told FOO=bar", out, st)
	}
}

func TestAValuelessLocalHandsOverTheNearestShadowedValue(t *testing.T) {
	// Two functions deep: what a child is told is the caller's local, not
	// the global standing behind it, so the record belongs to the scope that
	// took the name rather than to the name.
	out, st := valuelessRun(t,
		`export FOO=bar; g() { local FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; `+
			`f() { local FOO=mid; g; }; f`)
	if st != 0 || out != "FOO=mid\n" {
		t.Errorf("got %q status %d, want the caller's local handed over", out, st)
	}
}

func TestAnAssignmentToTheLocalSupersedesWhatItShadowed(t *testing.T) {
	// One entry, not two: a name handed over twice is settled by whichever
	// of the two the exec keeps, which is not a thing to leave to chance.
	out, st := valuelessRun(t,
		`export FOO=bar; f() { local FOO; FOO=zed; /usr/bin/env | grep -c '^FOO=zed$'; `+
			`/usr/bin/env | grep -c '^FOO='; }; f`)
	if st != 0 || out != "1\n1\n" {
		t.Errorf("got %q status %d, want exactly one entry and it the local's value", out, st)
	}
}

func TestAValuelessLocalOverAnUnexportedNameHandsOverNothing(t *testing.T) {
	// `-x` on the declaration is the local's own attribute and says nothing
	// about the name it shadows. The shadowed name is not exported, so there
	// is nothing behind the local to tell a child about, and the local
	// itself has no value.
	out, st := valuelessRun(t,
		`FOO=bar; f() { local -x FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`)
	if st != 0 || out != "(none)\n" {
		t.Errorf("got %q status %d, want no entry for a name nothing exported", out, st)
	}
}

func TestAValuelessLocalOverAnExportedNameIgnoresTheLocalsOwnAttribute(t *testing.T) {
	// The shadowed binding is what speaks, so `+x` on the local — which
	// takes the attribute off it outright — does not stop the outer value
	// reaching a child.
	for _, letter := range []string{"-x", "+x"} {
		out, st := valuelessRun(t,
			`export FOO=bar; f() { local `+letter+` FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`)
		if st != 0 || out != "FOO=bar\n" {
			t.Errorf("local %s: got %q status %d, want the shadowed value handed over", letter, out, st)
		}
	}
}

func TestAValuelessLocalOverAnImportedNameHandsOverWhatArrived(t *testing.T) {
	// The other half of the question: a name is exported by having arrived
	// in the environment, and it is not in the runner's own table at all.
	// Taking the table's answer alone recorded nothing for the whole of this
	// half, which is the route every inherited name takes.
	out, st := run(t,
		`f() { echo "read=[${IMPORTED-UNSET}]"; /usr/bin/env | grep '^IMPORTED=' || echo "(none)"; }`+
			"\n"+`f`,
		func(r *Runner) {
			sem := CoreSemantics()
			sem.DeclaredNameWithoutValueIsEmpty = No
			sem.ValuelessDeclarationHidesTheOuterValue = Yes
			sem.LocalInheritsTheExportAttribute = Yes
			r.Semantics = &sem
			r.Env = append(r.Env, "IMPORTED=arrived")
		})
	if st != 0 || out != "read=[arrived]\nIMPORTED=arrived\n" {
		t.Fatalf("got %q status %d, want the probe reading the inherited value", out, st)
	}
	out, st = run(t,
		`f() { local IMPORTED; echo "read=[${IMPORTED-UNSET}]"; /usr/bin/env | grep '^IMPORTED=' || echo "(none)"; }`+
			"\n"+`f`,
		func(r *Runner) {
			sem := CoreSemantics()
			sem.DeclaredNameWithoutValueIsEmpty = No
			sem.ValuelessDeclarationHidesTheOuterValue = Yes
			sem.LocalInheritsTheExportAttribute = Yes
			r.Semantics = &sem
			r.Env = append(r.Env, "IMPORTED=arrived")
		})
	if st != 0 || out != "read=[UNSET]\nIMPORTED=arrived\n" {
		t.Errorf("got %q status %d, want the shell reading it unset and the child told what arrived", out, st)
	}
}

func TestTheShadowedValueGoesBackWhenTheFunctionReturns(t *testing.T) {
	out, st := valuelessRun(t,
		`export FOO=bar; f() { local FOO; }; f; echo "read=[$FOO]"; /usr/bin/env | grep '^FOO='`)
	if st != 0 || out != "read=[bar]\nFOO=bar\n" {
		t.Errorf("got %q status %d, want the outer value back and exported once", out, st)
	}
}

func TestALocalThatDoesNotInheritTheAttributeHandsOverNothing(t *testing.T) {
	// The axis answers this shape too rather than leaving it to a rule of
	// its own: a shell whose local carries no export attribute tells a child
	// nothing under the name, with a value or without one.
	out, st := axisRun(t,
		`export FOO=bar; f() { local FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`,
		func(s *Semantics) {
			s.DeclaredNameWithoutValueIsEmpty = No
			s.ValuelessDeclarationHidesTheOuterValue = Yes
			s.LocalInheritsTheExportAttribute = No
		})
	if st != 0 || out != "(none)\n" {
		t.Errorf("got %q status %d, want nothing told under the name", out, st)
	}
}

func TestAValuelessDeclarationThatHidesNothingHandsOverOneEntry(t *testing.T) {
	// Where the outer value shows through, the name was never hidden and the
	// ordinary export list already carries it. Counted rather than read,
	// because the failure this guards against is a second copy.
	out, st := axisRun(t,
		`export FOO=bar; f() { local FOO; /usr/bin/env | grep -c '^FOO='; }; f`,
		func(s *Semantics) {
			s.DeclaredNameWithoutValueIsEmpty = No
			s.ValuelessDeclarationHidesTheOuterValue = No
			s.LocalInheritsTheExportAttribute = Yes
		})
	if st != 0 || out != "1\n" {
		t.Errorf("got %q status %d, want exactly one entry", out, st)
	}
}

func TestADeclarationOutsideAFunctionRecordsNothing(t *testing.T) {
	// No scope was taken, so nothing is standing in front of the name and an
	// exported name with no value reaches no child.
	out, st := valuelessRun(t, `export FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"`)
	if st != 0 || out != "(none)\n" {
		t.Errorf("got %q status %d, want no entry for an exported name with no value", out, st)
	}
}
