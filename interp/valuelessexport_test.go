// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
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
	set := func(s *Semantics) {
		s.DeclaredNameWithoutValueIsEmpty = No
		s.ValuelessDeclarationHidesTheOuterValue = No
		s.LocalInheritsTheExportAttribute = Yes
	}
	out, st := axisRun(t, `export FOO=bar; f() { local FOO; /usr/bin/env | grep -c '^FOO='; }; f`, set)
	if st != 0 || out != "1\n" {
		t.Errorf("got %q status %d, want exactly one entry", out, st)
	}
	// And for an imported name, where the entry the shell was born with is
	// the one that carries it: the record and the inherited entry are the
	// same name, and the list would hold it twice.
	//
	// Read off the list itself rather than out of a child, because the only
	// route that hands the list over *as written* is a process replacement:
	// os/exec collapses a repeated name to its last entry, so a duplicate is
	// invisible to every ordinary command and reaches a child intact through
	// execve.
	var handed []string
	outExec, st := run(t, "f() { local IMPORTED; exec /usr/bin/true; }\nf",
		func(r *Runner) {
			sem := CoreSemantics()
			set(&sem)
			// The one axis the route itself asks, and not the subject here.
			sem.ExecFailureRunsExitTrap = Yes
			r.Semantics = &sem
			r.Env = append(r.Env, "IMPORTED=arrived")
			r.ReplaceProcess = func(_ string, _, env []string, _ []*os.File) error {
				handed = env
				// A real replacement does not return; an error is how a test
				// says the image could not be replaced.
				return os.ErrPermission
			}
		})
	if st != 126 {
		t.Fatalf("got %q status %d, want the replacement reached and refused", outExec, st)
	}
	var carried []string
	for _, kv := range handed {
		if strings.HasPrefix(kv, "IMPORTED=") {
			carried = append(carried, kv)
		}
	}
	if len(carried) != 1 || carried[0] != "IMPORTED=arrived" {
		t.Errorf("the list carries %v, want exactly one IMPORTED=arrived", carried)
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

func TestADeclarationWithoutAValueTellsNoChildAboutTheName(t *testing.T) {
	// The dialect that considers a name declared without a value to be set
	// reads it as empty and still tells no command about it. Every listing
	// spells the two the same way, so a child is the only place the
	// difference shows.
	const probe = `typeset -x FOO; echo "read=[${FOO-UNSET}]"; ` +
		`/usr/bin/env | grep '^FOO=' || echo "(none)"`
	out, st := axisRun(t, probe, func(s *Semantics) {
		s.DeclaredNameWithoutValueIsEmpty = Yes
		s.DeclareOptions = "x"
	})
	if st != 0 || out != "read=[]\n(none)\n" {
		t.Errorf("got %q status %d, want the name read as empty and no entry", out, st)
	}
	// `=` on the same line is an assignment and hands over an empty entry.
	out, st = axisRun(t, `typeset -x FOO=; /usr/bin/env | grep '^FOO=' || echo "(none)"`,
		func(s *Semantics) {
			s.DeclaredNameWithoutValueIsEmpty = Yes
			s.DeclareOptions = "x"
		})
	if st != 0 || out != "FOO=\n" {
		t.Errorf("got %q status %d, want the empty assignment handed over", out, st)
	}
	// And an assignment afterwards is an assignment too.
	out, st = axisRun(t, `typeset -x FOO; FOO=later; /usr/bin/env | grep '^FOO=' || echo "(none)"`,
		func(s *Semantics) {
			s.DeclaredNameWithoutValueIsEmpty = Yes
			s.DeclareOptions = "x"
		})
	if st != 0 || out != "FOO=later\n" {
		t.Errorf("got %q status %d, want the later value handed over", out, st)
	}
	// The declaration's record goes away with the scope that made it, and so
	// does a caller's: a function that declares a local of the name leaves
	// the outer one an empty export where it had been told to nobody. That
	// is measured — the shell that reads a declaration this way forgets on
	// the way out — and not a tidiness rule.
	out, st = axisRun(t,
		`typeset -x FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"; f() { local FOO=v; }; f; `+
			`/usr/bin/env | grep '^FOO=' || echo "(none)"`,
		func(s *Semantics) {
			s.DeclaredNameWithoutValueIsEmpty = Yes
			s.DeclareOptions = "x"
			s.LocalInheritsTheExportAttribute = No
		})
	if st != 0 || out != "(none)\nFOO=\n" {
		t.Errorf("got %q status %d, want the record forgotten with the scope", out, st)
	}
	out, st = axisRun(t,
		`export FOO=out; f() { typeset -x FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f; `+
			`/usr/bin/env | grep '^FOO='`,
		func(s *Semantics) {
			s.DeclaredNameWithoutValueIsEmpty = Yes
			s.DeclareOptions = "x"
			s.LocalInheritsTheExportAttribute = Yes
			s.TypesetLocalNeedsKeywordFunction = No
		})
	if st != 0 || out != "(none)\nFOO=out\n" {
		t.Errorf("got %q status %d, want the outer name exported again on return", out, st)
	}
}

func TestADeclarationWithNoScopeToDeclareIntoRecordsNothing(t *testing.T) {
	// A dialect can give `typeset` a scope only in a function defined with
	// the keyword; in the other kind the declaration is an ordinary
	// assignment that reaches the caller. Nothing was shadowed there, so
	// there is nothing standing in front of the name and nothing for a child
	// to be told about once `unset` takes the value away.
	out, st := axisRun(t,
		`export FOO=bar; f() { typeset FOO; unset FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`,
		func(s *Semantics) {
			s.DeclaredNameWithoutValueIsEmpty = No
			s.ValuelessDeclarationHidesTheOuterValue = Yes
			s.LocalInheritsTheExportAttribute = Yes
			s.TypesetLocalNeedsKeywordFunction = Yes
		})
	if st != 0 || out != "(none)\n" {
		t.Errorf("got %q status %d, want nothing recorded where no scope was taken", out, st)
	}
}

func TestTypesetsOwnExportLetterSaysNothingAboutTheNameItShadows(t *testing.T) {
	// The same reading as `local -x`, through the other builtin, because the
	// two are separate paths and each has to read the name's attribute
	// before applying this declaration's own.
	out, st := axisRun(t,
		`FOO=bar; function f { typeset -x FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`,
		func(s *Semantics) {
			s.DeclaredNameWithoutValueIsEmpty = No
			s.ValuelessDeclarationHidesTheOuterValue = Yes
			s.LocalInheritsTheExportAttribute = Yes
			s.DeclareOptions = "x"
			s.TypesetLocalNeedsKeywordFunction = No
		})
	if st != 0 || out != "(none)\n" {
		t.Errorf("got %q status %d, want no entry for a name nothing exported", out, st)
	}
	// And where the shadowed name *is* exported, the value it held goes to a
	// child whatever this declaration says about the local's own attribute.
	out, st = axisRun(t,
		`export FOO=bar; function f { typeset -x FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`,
		func(s *Semantics) {
			s.DeclaredNameWithoutValueIsEmpty = No
			s.ValuelessDeclarationHidesTheOuterValue = Yes
			s.LocalInheritsTheExportAttribute = Yes
			s.DeclareOptions = "x"
			s.TypesetLocalNeedsKeywordFunction = No
		})
	if st != 0 || out != "FOO=bar\n" {
		t.Errorf("got %q status %d, want the shadowed value handed over", out, st)
	}
}

func TestASecondDeclarationHandsOverWhatTheFirstOneLeft(t *testing.T) {
	// A declaration of a name this scope has already taken hides the value
	// the scope itself put there, not the one it will put back. The two
	// readings differ only where a scope declares the same name twice.
	out, st := valuelessRun(t,
		`export FOO=bar; f() { local FOO=x; local FOO; /usr/bin/env | grep '^FOO='; }; f`)
	if st != 0 || out != "FOO=x\n" {
		t.Errorf("got %q status %d, want the value the second declaration hid", out, st)
	}
}

// The shape the whole of this file was one line short of, and the one a value
// on the declaration reaches. `local +x FOO=z` gives the function a binding of
// its own holding `z` and *not* exported, and the binding behind it is still
// exported and still holds `bar` — so that is what a child is told, while the
// shell itself reads `z`.
//
// It failed in the gap between two lists: the ordinary loop over the tables
// passed the name over because the local is not exported, and the list of
// shadowed exports passed it over because the local has a value of its own.
// Neither claimed it and the child was told nothing at all.
func TestALocalThatDropsTheAttributeStillHandsAChildWhatItShadowed(t *testing.T) {
	out, st := valuelessRun(t,
		`export FOO=bar; f() { local +x FOO=z; /usr/bin/env | grep '^FOO=' || echo "(none)"; `+
			`echo "read=[$FOO]"; }; f; echo "after=[$FOO]"`)
	if want := "FOO=bar\nread=[z]\nafter=[bar]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The control, and what says it is the shadowed binding speaking rather than a
// value the local kept: with nothing exported behind it the same line tells a
// child nothing. A fix that handed the outer value over unconditionally would
// pass the test above and fail this one.
func TestALocalThatDropsTheAttributeOverAnUnexportedNameHandsOverNothing(t *testing.T) {
	out, st := valuelessRun(t,
		`FOO=bar; f() { local +x FOO=z; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`)
	if want := "(none)\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// `export` inside the function puts the attribute on the local, and the child
// is then told the local's own value rather than the one it shadowed — once,
// not twice.
func TestExportingTheLocalAgainSupersedesTheBindingBehindIt(t *testing.T) {
	out, st := valuelessRun(t,
		`export FOO=bar; f() { local +x FOO=z; export FOO; `+
			`/usr/bin/env | grep -c '^FOO=z$'; /usr/bin/env | grep -c '^FOO='; }; f`)
	if want := "1\n1\n"; out != want {
		t.Errorf("out = %q, want %q — exactly one entry and it the local's value", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// Two functions deep, so the value handed over is the nearest shadowed one
// rather than the global behind it — the same rule the valueless form follows,
// reached with a value on the line.
func TestALocalThatDropsTheAttributeHandsOverTheNearestShadowedValue(t *testing.T) {
	out, st := valuelessRun(t,
		`export FOO=bar; g() { local +x FOO=in; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; `+
			`f() { local FOO=mid; g; }; f`)
	if want := "FOO=mid\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The shell whose local carries no export attribute tells a child nothing
// under the name, with a value on the declaration as without one — so this
// half needs no rule of its own and must not gain one.
func TestALocalThatDropsTheAttributeWhereNoLocalInheritsItHandsOverNothing(t *testing.T) {
	out, st := axisRun(t,
		`export FOO=bar; f() { local +x FOO=z; /usr/bin/env | grep '^FOO=' || echo "(none)"; `+
			`echo "read=[$FOO]"; }; f`,
		func(s *Semantics) {
			s.DeclaredNameWithoutValueIsEmpty = No
			s.ValuelessDeclarationHidesTheOuterValue = Yes
			s.LocalInheritsTheExportAttribute = No
			s.LocalOptions = "x"
		})
	if want := "(none)\nread=[z]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}
