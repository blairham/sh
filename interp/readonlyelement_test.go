// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runReadonlyElement runs src with the readonly refusal answered as the one
// shell that reports and carries on, so the line after the refusal can say
// what the table holds. A fatal answer would end the script and leave the
// write itself unobserved — which is exactly how the bug survived: every
// probe that reached it stopped before it could read the array back.
func runReadonlyElement(t *testing.T, src string) (out string, status int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		s := readonlyElementSemantics()
		r.Semantics = &s
	})
}

// readonlyElementSemantics is the shared answer set.
func readonlyElementSemantics() Semantics {
	{
		s := permissive()
		s.ReadonlyReassignmentFatal = No
		s.ReadonlyReassignmentByDeclarationFatal = No
		// Two axes the setup lines below would otherwise leave unanswered:
		// `typeset -A m` declares without a value, and a function body wants
		// `typeset` to declare a local without asking which functions have a
		// scope. Neither is what these tests are about, so both take bash's
		// answer — the dialect every expectation here was measured in.
		s.DeclaredNameWithoutValueIsEmpty = No
		s.TypesetLocalNeedsKeywordFunction = No
		// And what `readonly -a` records about the kind, which is a third
		// question these tests are not about — bash's answer again, and the
		// one the expectations here were measured in.
		s.ReadonlyRecordsTheCompoundAttribute = No
		return s
	}
}

// runReadonlyElementWith is runReadonlyElement with the route and the wording
// under the test's control, for the two questions the shared setup answers
// flat.
func runReadonlyElementWith(t *testing.T, src string, route Route, d Diagnostics) (out string, status int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		s := readonlyElementSemantics()
		r.Semantics, r.Diagnostics, r.Route = &s, &d, route
	})
}

// runReadonlyElementFromCommandString answers the two fatality axes in
// opposite directions and says the program came from `-c`, so which of them
// the refusal consulted is visible in whether the script carried on.
func runReadonlyElementSemantics(t *testing.T, src string, set func(*Semantics)) (out string, status int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		s := readonlyElementSemantics()
		s.FatalErrorStatusIsOne = Yes
		if set != nil {
			set(&s)
		}
		r.Semantics = &s
	})
}

// An element of a readonly associative array cannot be assigned.
//
// Measured 2026-09-06 on the snippet below against bash 5.3.15, zsh 5.9.2 and
// ksh93u+ 2012-08-01 — the three shells in the panel that have the attribute
// at all; bash 3.2.57 has no `-A` and dash has no arrays. All three refuse the
// write, say so and leave the table as it was, so the refusal is the
// intersection rather than any dialect's choice.
//
// Ours stored the element, said nothing and reported 0 (#1012). The value and
// the status are both asserted because the bug is precisely a plausible value
// at status 0: an assertion that only looked for an error would have passed
// against it.
func TestAnElementOfAReadonlyAssociativeArrayIsRefused(t *testing.T) {
	const src = `typeset -A m; m[a]=1; typeset -r m
m[k]=v
echo "st=$? k=[${m[k]-unset}] a=[${m[a]}]"`

	out, st := runReadonlyElement(t, src)
	if want := "sh: m: readonly variable\nst=1 k=[unset] a=[1]\n"; out != want {
		t.Errorf("wrote %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status %d, want the 0 of the `echo` that carried on", st)
	}
}

// The indexed spelling was the same bug wearing a diagnostic.
//
// It *reported* the refusal, because storeArray keeps the scalar view of an
// array in step and that call goes through setVarAs — so the complaint printed
// after the element had already been written. Every probe that showed it
// working ran from a command string, where the refusal is fatal and the script
// stops before anything can read the array back.
//
// bash 5.3.15 leaves `x y`; ours left `z y` behind the same message.
func TestAnElementOfAReadonlyIndexedArrayIsNotWrittenEither(t *testing.T) {
	const src = `typeset -a a=(x y); typeset -r a
a[0]=z
echo "st=$? all=[${a[*]}]"`

	out, st := runReadonlyElement(t, src)
	if want := "sh: a: readonly variable\nst=1 all=[x y]\n"; out != want {
		t.Errorf("wrote %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status %d, want the 0 of the `echo` that carried on", st)
	}
}

// Every other shape an assignment to a frozen array takes is refused with it,
// and each one had a store of its own that the refusal never guarded.
//
// The forms are together in one test because they are one rule: the name is
// frozen, so nothing reaches the table. Measured against bash 5.3.15, which
// gives the same message and the same untouched table for all six.
func TestEveryAssignmentShapeIsRefusedOnAFrozenArray(t *testing.T) {
	// frozen is the name the complaint carries, which is the base and never
	// the subscript: every shell that reaches the check says `m`, not `m[k]`.
	tests := []struct{ name, frozen, src, want string }{
		{
			// `m[a]+=Q` joins the element it names. The append read the old
			// value, joined it and stored the result.
			"an associative element append", "m",
			`typeset -A m; m[a]=1; typeset -r m
m[a]+=Q
echo "st=$? a=[${m[a]}]"`,
			"st=1 a=[1]\n",
		},
		{
			// The whole-name path for an associative array never went
			// through setVarAs at all, so it replaced the table outright:
			// `a` came back unset.
			"an associative literal replacing the table", "m",
			`typeset -A m; m[a]=1; typeset -r m
m=([z]=9)
echo "st=$? a=[${m[a]-unset}] z=[${m[z]-unset}]"`,
			"st=1 a=[1] z=[unset]\n",
		},
		{
			"an associative compound append", "m",
			`typeset -A m; m[a]=1; typeset -r m
m+=([z]=9)
echo "st=$? a=[${m[a]-unset}] z=[${m[z]-unset}]"`,
			"st=1 a=[1] z=[unset]\n",
		},
		{
			// A scalar assignment to a declared associative name lands on
			// the element whose key is `0` — and did so on a frozen one.
			"a scalar assignment to an associative name", "m",
			`typeset -A m; m[a]=1; typeset -r m
m=plain
echo "st=$? zero=[${m[0]-unset}] a=[${m[a]}]"`,
			"st=1 zero=[unset] a=[1]\n",
		},
		{
			// A subscript nothing has been assigned to yet is still a write
			// to the frozen name, not a fresh element beside it.
			"an indexed element at a new subscript", "a",
			`typeset -a a=(x y); typeset -r a
a[5]=n
echo "st=$? all=[${a[*]}] five=[${a[5]-unset}]"`,
			"st=1 all=[x y] five=[unset]\n",
		},
		{
			"an indexed element append", "a",
			`typeset -a a=(x y); typeset -r a
a[0]+=Q
echo "st=$? all=[${a[*]}]"`,
			"st=1 all=[x y]\n",
		},
		{
			"an indexed literal replacing the array", "a",
			`typeset -a a=(x y); typeset -r a
a=(p q)
echo "st=$? all=[${a[*]}]"`,
			"st=1 all=[x y]\n",
		},
		{
			"an indexed compound append", "a",
			`typeset -a a=(x y); typeset -r a
a+=(z)
echo "st=$? all=[${a[*]}]"`,
			"st=1 all=[x y]\n",
		},
		{
			// A scalar assignment replaces any array of the same name, and
			// the `delete` that does it ran after the refusal had already
			// reported: the array was destroyed by an assignment that failed.
			"a scalar assignment over an indexed array", "a",
			`typeset -a a=(x y); typeset -r a
a=p
echo "st=$? all=[${a[*]}]"`,
			"st=1 all=[x y]\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runReadonlyElement(t, tc.src)
			if want := "sh: " + tc.frozen + ": readonly variable\n" + tc.want; out != want {
				t.Errorf("wrote %q, want %q", out, want)
			}
			if st != 0 {
				t.Errorf("status %d, want the 0 of the `echo` that carried on", st)
			}
		})
	}
}

// The refusal must not reach the declaration that is applying `-r` in the
// first place. `typeset -ar a=(x y)` is one command: the value it carries
// cannot be refused by the attribute it carries beside it.
//
// This is the half a guard on the element path breaks if it is placed without
// thinking about ordering — and it was already broken for the indexed
// spelling before the guard existed, because `readonly` is the only
// declaration that assigns its operands before it locks. bash 5.3.15 and zsh
// 5.9.2 both leave the value in place for all four spellings below; ksh93
// does not cluster `-ar` at all and refuses the flag rather than the value.
func TestADeclarationsOwnArrayValueSurvivesItsReadonlyFlag(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{
			"an indexed literal as a declaration operand",
			`typeset -ar a=(x y); echo "st=$? all=[${a[*]}]"`,
			"st=0 all=[x y]\n",
		},
		{
			"an associative literal as a declaration operand",
			`typeset -Ar m=([k]=v); echo "st=$? k=[${m[k]}]"`,
			"st=0 k=[v]\n",
		},
		{
			// `readonly` reaches the same place by the other order, which is
			// what makes the two spellings one rule rather than two.
			"readonly with an array operand",
			`readonly -a a=(x y); echo "st=$? all=[${a[*]}]"`,
			"st=0 all=[x y]\n",
		},
		{
			// The freeze is real once the value has landed: the next write
			// is refused.
			"and the name is frozen afterwards",
			`typeset -ar a=(x y)
a[0]=z
echo "st=$? all=[${a[*]}]"`,
			"sh: a: readonly variable\nst=1 all=[x y]\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runReadonlyElement(t, tc.src)
			if out != tc.want {
				t.Errorf("wrote %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
		})
	}
}

// A declaration whose operand *is* refused reports 1, and nothing else did.
//
// The operand assignments of a declaration land after the builtin has
// returned, so the 0 the builtin reported for the attributes it did apply
// stood over the value it did not: `readonly a; typeset -a a=(p q)` said
// `a: readonly variable` and reported 0. bash 5.3.15 reports 1.
func TestARefusedDeclarationOperandReportsOne(t *testing.T) {
	const src = `typeset -a a=(x y); typeset -r a
typeset -a a=(p q)
echo "st=$? all=[${a[*]}]"`

	out, st := runReadonlyElement(t, src)
	if want := "sh: a: readonly variable\nst=1 all=[x y]\n"; out != want {
		t.Errorf("wrote %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status %d, want the 0 of the `echo` that carried on", st)
	}
}

// The freeze a declaration defers is still that declaration's own, so a
// function-local one stays local: the operand lands in the shadow the
// declaration took, and the attribute lands on the same cell.
//
// Measured in bash 5.3.15 and zsh 5.9.2: `in` reads the array and `out` finds
// nothing, which is what says the value never reached the caller's scope.
func TestADeferredFreezeStaysInsideTheFunctionThatDeclaredIt(t *testing.T) {
	const src = `f() { typeset -ar l=(m n); echo "in=[${l[*]}]"; }
f
echo "out=[${l[*]-unset}]"`

	out, st := runReadonlyElement(t, src)
	if want := "in=[m n]\nout=[unset]\n"; out != want {
		t.Errorf("wrote %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}

// runReadonlyElementFatal answers the refusal as the three shells that end the
// script on it, so what a fatal refusal leaves in the status can be read.
func runReadonlyElementFatal(t *testing.T, src string) (out string, status int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		s := readonlyElementSemantics()
		s.ReadonlyReassignmentFatal = Yes
		s.FatalErrorStatusIsOne = Yes
		r.Semantics = &s
	})
}

// An array operand's refusal is a bare assignment's, not a declaration's —
// which the spelling does not suggest, since it is written after a
// declaration utility's own word.
//
// Measured in bash 5.3.15, where the two are told apart twice over. A scalar
// declaration reports and runs the next command on the same line, and names
// the builtin in the complaint: `declare r=2` against a frozen `r` writes
// `declare: r: readonly variable` and `one` still prints. An array operand
// does neither: `typeset -a a=(p q); echo one` writes `a: readonly variable`
// and `one` never prints, exactly as `a=(p q)` and `a[0]=z` do. The array
// operand goes through the assignment machinery and the builtin's name never
// reaches it.
//
// Both halves are asserted, because either alone would pass against the wrong
// form: the wording is what a dialect that names the builtin would change, and
// the abandoned line is what a dialect that does not would.
func TestARefusedArrayOperandIsABareAssignmentAndNotADeclaration(t *testing.T) {
	names := Diagnostics{
		ReadonlyVariableInDeclaration: "%[2]s: %[1]s: readonly variable",
		ReadonlyRefusalNamesBuiltin:   map[string]bool{"typeset": true},
	}
	const src = `typeset -a a=(x y); typeset -r a
typeset -a a=(p q); echo one
echo two`

	out, st := runReadonlyElementWith(t, src, RouteScriptFile, names)
	if want := "sh: a: readonly variable\ntwo\n"; out != want {
		t.Errorf("wrote %q, want %q — a declaration's wording or a declaration's line", out, want)
	}
	if st != 0 {
		t.Errorf("status %d, want the 0 of the `echo` on the line after", st)
	}
}

// The element path asks the same fatality axis the scalar path does, rather
// than a second one of its own.
//
// The two axes are answered in **opposite directions**, which is what makes
// which one was asked visible: an element write is an assignment standing on
// its own, so it must read ReadonlyReassignmentFatal and not the declaration
// answer. With both answered alike, a write that consulted neither correctly
// would still look right.
//
// This used to be discriminated by the route, through a field that has since
// been removed — it was measured on the diagonal of route-by-separator and
// the route decides nothing here (#1182). The declaration axis is the
// discriminator that survives, and it is a better one: it is a real split in
// the panel rather than an artifact.
func TestTheElementPathAsksTheAssignmentAxisAndNotTheDeclarationOne(t *testing.T) {
	const src = `typeset -A m; m[a]=1; typeset -r m
m[k]=v
echo after`

	// Fatal for a bare assignment, not for a declaration: the element write
	// must stop.
	out, st := runReadonlyElementSemantics(t, src, func(s *Semantics) {
		s.ReadonlyReassignmentFatal = Yes
		s.ReadonlyReassignmentByDeclarationFatal = No
	})
	if want := "sh: m: readonly variable\n"; out != want {
		t.Errorf("wrote %q, want %q — `after` printing means the declaration answer was read", out, want)
	}
	if st != 1 {
		t.Errorf("status %d, want the 1 of a fatal error", st)
	}

	// And the other way round, so neither answer is a default: not fatal for
	// a bare assignment, fatal for a declaration. The write is refused and
	// the shell carries on.
	out, _ = runReadonlyElementSemantics(t, src, func(s *Semantics) {
		s.ReadonlyReassignmentFatal = No
		s.ReadonlyReassignmentByDeclarationFatal = Yes
	})
	if !strings.Contains(out, "after") {
		t.Errorf("wrote %q, want the shell to carry on — the declaration answer was read", out)
	}
	if !strings.Contains(out, "readonly variable") {
		t.Errorf("wrote %q, want the refusal still reported", out)
	}
}

// A fatal refusal of a declaration's operand keeps the status a fatal error
// carries, rather than the 0 the builtin reported for the attributes it did
// apply.
//
// The operand assignments land after the builtin has returned, so the
// builtin's status is written into `$?` last and wrote over the fatal one.
// Measured in zsh 5.9.2 and ksh93u+, the two that end the script here: both
// exit 1 and neither reaches the line after.
//
// The status is what this asserts, and the absent output is what says the
// script really stopped — a shell that carried on and happened to leave 1
// behind would pass on the number alone.
func TestAFatalRefusalOfADeclarationOperandKeepsItsStatus(t *testing.T) {
	const src = `typeset -a a=(x y); typeset -r a
typeset -a a=(p q)
echo after`

	out, st := runReadonlyElementFatal(t, src)
	if want := "sh: a: readonly variable\n"; out != want {
		t.Errorf("wrote %q, want %q — `after` printing means the script did not stop", out, want)
	}
	if st != 1 {
		t.Errorf("status %d, want the 1 of a fatal error rather than the builtin's 0", st)
	}
}

// And the plain element write reaches the same answer, which is what says the
// two are one rule: nothing about the fatality is decided by which shape the
// assignment wore.
func TestAFatalRefusalOfAnElementWriteKeepsItsStatus(t *testing.T) {
	const src = `typeset -A m; m[a]=1; typeset -r m
m[k]=v
echo after`

	out, st := runReadonlyElementFatal(t, src)
	if want := "sh: m: readonly variable\n"; out != want {
		t.Errorf("wrote %q, want %q", out, want)
	}
	if st != 1 {
		t.Errorf("status %d, want the 1 of a fatal error", st)
	}
}
