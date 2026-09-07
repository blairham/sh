// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// declareElementSemantics is a shell that takes a subscripted operand and does
// everything a declaration of one can do, so the writes below are observable
// at all.
func declareElementSemantics() Semantics {
	s := permissive()
	s.TypesetTakesASubscript = Yes
	s.DeclarationTakesASubscript = Yes
	s.SubscriptedOperandTakesTheIntegerAttribute = Yes
	s.SubscriptedOperandTakesALocalDeclaration = Yes
	s.ReadonlyElement = ReadonlyElementWritten
	// Not what these tests are about: a declaration with no value, and which
	// functions have a scope. Both are answered flat so an unanswered axis
	// cannot stand in for the refusal a test is looking for.
	s.DeclaredNameWithoutValueIsEmpty = No
	s.TypesetLocalNeedsKeywordFunction = No
	// Nor are these: how an array counts a gap, and whether an element goes
	// through the name's attribute. Both are read by the lines that make a
	// write visible rather than by the write, and both take the answer of
	// the shell every expectation below was measured in.
	s.ArraysAreSparse = Yes
	s.CompoundElementsGoThroughTheAttribute = Yes
	s.IntegerBaseComesFromTheValueAssigned = No
	// The letters the declarations below write. The core names none, so a
	// test asking what `-g` or `-i` does has to say the word exists first.
	s.DeclareOptions = "aAfFgilprux"
	return s
}

func runDeclareElement(t *testing.T, src string, set func(*Semantics)) (out string, status int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		s := declareElementSemantics()
		if set != nil {
			set(&s)
		}
		r.Semantics = &s
	})
}

// A declaration's subscripted operand names an element, and the element is
// what it writes.
//
// It used to split the operand at its `=` and hand `a[1]` to the variable
// store as though the brackets were part of the name: status 0, nothing
// created, and every read of the array empty afterwards (#1203).
func TestADeclarationWritesTheElementItsOperandNames(t *testing.T) {
	out, status := runDeclareElement(t, `typeset a[1]=v; echo "[${a[1]}] n=${#a[@]} st=$?"`, nil)
	if want := "[v] n=1 st=0\n"; out != want || status != 0 {
		t.Errorf("typeset a[1]=v = %q (status %d), want %q at 0", out, status, want)
	}
}

// The element it names and not the first one: an array standing before the
// declaration keeps its other elements.
func TestADeclaredElementLeavesTheRestOfTheArray(t *testing.T) {
	out, status := runDeclareElement(t,
		`a=(x y z); typeset a[1]=v; echo "[${a[0]}][${a[1]}][${a[2]}] n=${#a[@]}"`, nil)
	if want := "[x][v][z] n=3\n"; out != want || status != 0 {
		t.Errorf("a=(x y z); typeset a[1]=v = %q (status %d), want %q at 0", out, status, want)
	}
}

// The subscript is an expression, as it is everywhere else a subscript is
// written — so the operand is not a name with brackets in it.
func TestADeclaredElementsSubscriptIsAnExpression(t *testing.T) {
	out, status := runDeclareElement(t, `typeset a[1+1]=v; echo "[${a[2]}] n=${#a[@]}"`, nil)
	if want := "[v] n=1\n"; out != want || status != 0 {
		t.Errorf("typeset a[1+1]=v = %q (status %d), want %q at 0", out, status, want)
	}
}

// A declared table takes its subscript as a key rather than as an expression,
// which is the same switch the attribute throws for a bare `m[k]=v`.
func TestADeclaredElementOfATableIsKeyed(t *testing.T) {
	out, status := runDeclareElement(t, `typeset -A m; typeset m[k]=v; echo "[${m[k]}]"`, nil)
	if want := "[v]\n"; out != want || status != 0 {
		t.Errorf("typeset m[k]=v = %q (status %d), want %q at 0", out, status, want)
	}
}

// The operand is an assignment, so its name half is never a pattern.
//
// The discriminating shape: a file on disk that the operand *matches*. Without
// it the word is a pattern with no match and stays as written, which is the
// same text either way — and is why this went unnoticed until a shell that
// refuses an unmatched pattern ran the same line.
func TestADeclarationsSubscriptedOperandIsNotAPattern(t *testing.T) {
	out, status := runDeclareElement(t,
		`: > 'a1=v'; typeset a[1]=v; echo "[${a[1]}] a1=[$a1]"`, nil)
	if want := "[v] a1=[]\n"; out != want || status != 0 {
		t.Errorf("typeset a[1]=v beside a matching file = %q (status %d), want %q at 0", out, status, want)
	}
}

// And its value half expands as an assignment's, so it is neither split nor
// matched against the filesystem.
func TestADeclaredElementsValueIsNotSplit(t *testing.T) {
	out, status := runDeclareElement(t,
		`x="p q"; typeset a[1]=$x; echo "[${a[1]}] n=${#a[@]}"`, nil)
	if want := "[p q] n=1\n"; out != want || status != 0 {
		t.Errorf("typeset a[1]=$x = %q (status %d), want %q at 0", out, status, want)
	}
}

// The subscript may hold an expansion, which reaches the word as spans of its
// own — so the `=` that ends the name is in the last of them rather than the
// first. Reading only the first span made this an ordinary word again.
func TestADeclaredElementsSubscriptMayExpand(t *testing.T) {
	out, status := runDeclareElement(t, `i=1; typeset a[$i]=v; echo "[${a[1]}]"`, nil)
	if want := "[v]\n"; out != want || status != 0 {
		t.Errorf("typeset a[$i]=v = %q (status %d), want %q at 0", out, status, want)
	}
}

// A word that only looks like one is still an ordinary operand: the name half
// has to be a name for any of this to apply.
func TestAnOperandThatIsNotANameWithASubscriptIsNotAnAssignment(t *testing.T) {
	out, status := runDeclareElement(t, `typeset 1x[0]=v`, nil)
	if !strings.Contains(out, "1x[0]") || status == 0 {
		t.Errorf("typeset 1x[0]=v = %q (status %d), want the operand reported and a failure", out, status)
	}
}

// The attributes land on the array, which is what they are about, and the
// value on the element.
func TestADeclaredElementTakesTheDeclarationsAttribute(t *testing.T) {
	out, status := runDeclareElement(t, `typeset -i a[1]=0x10; echo "[${a[1]}]"`, nil)
	if want := "[16]\n"; out != want || status != 0 {
		t.Errorf("typeset -i a[1]=0x10 = %q (status %d), want %q at 0", out, status, want)
	}
}

// A shell that will not put an attribute on an element refuses the operand
// rather than doing half of it, and ends the script.
func TestAnElementRefusedByTheIntegerAttributeEndsTheScript(t *testing.T) {
	out, status := runDeclareElement(t, `typeset -i a[1]=5; echo after`, func(s *Semantics) {
		s.SubscriptedOperandTakesTheIntegerAttribute = No
	})
	if want := "sh: a[1]: cannot declare an array element\n"; out != want || status == 0 {
		t.Errorf("typeset -i a[1]=5 refused = %q (status %d), want %q and a failure", out, status, want)
	}
}

// The same for a declaration that would make the array local, and it is a
// separate axis: a dialect may take one and refuse the other.
func TestAnElementRefusedByALocalDeclarationEndsTheScript(t *testing.T) {
	out, status := runDeclareElement(t, `f() { typeset a[1]=v; }; f; echo after`, func(s *Semantics) {
		s.SubscriptedOperandTakesALocalDeclaration = No
	})
	if want := "sh: a[1]: cannot declare an array element\n"; out != want || status == 0 {
		t.Errorf("local declaration of an element refused = %q (status %d), want %q and a failure", out, status, want)
	}
}

// And only where there is a scope to take: the same axis answered No leaves a
// declaration at the top level alone.
func TestTheLocalElementAxisIsNotAskedOutsideAFunction(t *testing.T) {
	out, status := runDeclareElement(t, `typeset a[1]=v; echo "[${a[1]}]"`, func(s *Semantics) {
		s.SubscriptedOperandTakesALocalDeclaration = No
	})
	if want := "[v]\n"; out != want || status != 0 {
		t.Errorf("typeset a[1]=v at the top level = %q (status %d), want %q at 0", out, status, want)
	}
}

// The declaration that freezes the array has three answers rather than two,
// so it is a policy and not an Answer. Written is one of them.
func TestAReadonlyElementIsWrittenAndTheArrayFrozenOverIt(t *testing.T) {
	out, status := runDeclareElement(t,
		`readonly a[1]=v; echo "[${a[1]}]"; a[2]=q; echo after`, func(s *Semantics) {
			s.ReadonlyReassignmentFatal = No
		})
	if !strings.HasPrefix(out, "[v]\n") || !strings.Contains(out, "readonly") {
		t.Errorf("readonly a[1]=v = %q (status %d), want the element written and `a` frozen", out, status)
	}
}

// Refused is the other, and it is refused without a value as well: the
// refusal is about the attribute rather than about the assignment.
func TestARefusedReadonlyElementIsRefusedWithoutAValueToo(t *testing.T) {
	for _, src := range []string{`readonly a[1]=v; echo after`, `readonly "a[1]"; echo after`} {
		out, status := runDeclareElement(t, src, func(s *Semantics) {
			s.ReadonlyElement = ReadonlyElementRefused
		})
		if want := "sh: a[1]: cannot declare an array element\n"; out != want || status == 0 {
			t.Errorf("%s = %q (status %d), want %q and a failure", src, out, status, want)
		}
	}
}

// An unanswered policy is refused by name rather than guessed at, which is
// what one column's third answer is left as.
func TestAnUnansweredReadonlyElementPolicyRefusesTheDeclaration(t *testing.T) {
	out, status := runDeclareElement(t, `typeset -r a[1]=v; echo "[${a[1]}]"`, func(s *Semantics) {
		s.ReadonlyElement = ReadonlyElementUnspecified
	})
	if want := "sh: a declaration of one array element freezing the whole array: " +
		"the shells disagree here and no dialect was chosen\n[]\n"; out != want {
		t.Errorf("typeset -r a[1]=v unanswered = %q (status %d), want %q", out, status, want)
	}
}

// A name already frozen refuses the element by the *base* name, which is the
// rule `unset a[0]` follows: the subscript is never reached.
func TestADeclaredElementOfAFrozenArrayIsRefusedByItsBase(t *testing.T) {
	out, status := runDeclareElement(t,
		`readonly a=z; typeset a[1]=v; echo "st=$? [$a][${a[1]}]"`, func(s *Semantics) {
			s.ReadonlyReassignmentFatal = No
			s.ReadonlyReassignmentByDeclarationFatal = No
		})
	// The refusal names `a`, the value it was holding is still there, and the
	// element was not written — the last of the three is what says the guard
	// stopped the store rather than only reporting on the way past it.
	if want := "sh: a: readonly variable\nst=1 [z][]\n"; out != want || status != 0 {
		t.Errorf("typeset a[1]=v on a frozen name = %q (status %d), want %q at 0", out, status, want)
	}
}

// `local` reads the declaration's subscript question and not `export`'s,
// which is a per-builtin split rather than a per-dialect one.
func TestLocalReadsTheDeclarationsSubscriptQuestion(t *testing.T) {
	out, status := runDeclareElement(t, `f() { local a[1]=v; echo "[${a[1]}]"; }; f`, func(s *Semantics) {
		s.TypesetTakesASubscript = Yes
		s.DeclarationTakesASubscript = No
	})
	if want := "[v]\n"; out != want || status != 0 {
		t.Errorf("local a[1]=v = %q (status %d), want %q at 0", out, status, want)
	}
}

// `export` and `readonly` read theirs, and a dialect that refuses one there
// still refuses it after this.
func TestExportKeepsItsOwnSubscriptQuestion(t *testing.T) {
	out, status := runDeclareElement(t, `export a[1]=v`, func(s *Semantics) {
		s.DeclarationTakesASubscript = No
	})
	if !strings.Contains(out, "export") || !strings.Contains(out, "a[1]") || status == 0 {
		t.Errorf("export a[1]=v refused = %q (status %d), want the name reported and a failure", out, status)
	}
}

// And where it is taken, the element is written.
func TestExportWritesTheElementItsOperandNames(t *testing.T) {
	out, status := runDeclareElement(t, `export a[1]=v; echo "[${a[1]}] st=$?"`, nil)
	if want := "[v] st=0\n"; out != want || status != 0 {
		t.Errorf("export a[1]=v = %q (status %d), want %q at 0", out, status, want)
	}
}

// `-g` never takes a shadow, so the element lands on the global array and
// survives the function's return — which is the whole of what the letter
// means, and it is also why the local axis is not asked when it is written.
func TestAGlobalElementDeclarationTakesNoShadow(t *testing.T) {
	for _, local := range []Answer{Yes, No} {
		out, status := runDeclareElement(t,
			`a=(x y); f() { typeset -g a[1]=v; }; f; echo "[${a[1]}]"`, func(s *Semantics) {
				s.SubscriptedOperandTakesALocalDeclaration = local
			})
		if want := "[v]\n"; out != want || status != 0 {
			t.Errorf("typeset -g a[1]=v with the local axis %v = %q (status %d), want %q at 0",
				local, out, status, want)
		}
	}
}

// A plus form is the letter being taken *off*, so neither refusal is asked:
// `typeset +r a[1]=v` and `typeset +i a[1]=5` write the element in every
// column, including the one that refuses both minus forms.
func TestAPlusFormAsksNeitherElementRefusal(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset +r a[1]=v; echo "[${a[1]}]"`, "[v]\n"},
		{`typeset +i a[1]=5; echo "[${a[1]}]"`, "[5]\n"},
	} {
		out, status := runDeclareElement(t, tc.src, func(s *Semantics) {
			s.ReadonlyElement = ReadonlyElementRefused
			s.SubscriptedOperandTakesTheIntegerAttribute = No
		})
		if out != tc.want || status != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, status, tc.want)
		}
	}
}

// The attribute on the same declaration decides how the subscript is read:
// `typeset -A m[k]=v` places a key rather than an element numbered by
// whatever `k` evaluates to.
func TestAnAssociativeAttributeOnTheSameDeclarationKeysTheSubscript(t *testing.T) {
	out, status := runDeclareElement(t, `typeset -A m[k]=v; echo "[${m[k]}]"`, nil)
	if want := "[v]\n"; out != want || status != 0 {
		t.Errorf("typeset -A m[k]=v = %q (status %d), want %q at 0", out, status, want)
	}
}

// A word whose name half is not a name is an ordinary operand, so its value
// is split and matched like any other word — which is what every column does
// with `typeset 1x=$y`: the complaint names `1x=p`, not `1x=p q`.
func TestAnOperandWithABadNameIsSplitLikeAnyOtherWord(t *testing.T) {
	// Two bad names in the value rather than one, so the *number* of
	// complaints says whether the word was split: unsplit it is one operand
	// and one line, split it is two of each.
	out, status := runDeclareElement(t, `y="p 2z"; typeset 1x=$y`, func(s *Semantics) {
		s.BadNameToDeclarationFatal = No
	})
	want := "sh: typeset: `1x': not a valid identifier\n" +
		"sh: typeset: `2z': not a valid identifier\n"
	if out != want || status == 0 {
		t.Errorf("typeset 1x=$y = %q (status %d), want %q and a failure", out, status, want)
	}
}

// And so is one whose name half holds an expansion: `a$n=$y` is not an
// assignment in any column, so the value splits and only the first field
// reaches the name.
func TestAnOperandWhoseNameHoldsAnExpansionIsSplitToo(t *testing.T) {
	out, status := runDeclareElement(t, `n=x; y="p q"; typeset a$n=$y; echo "[$ax]"`, nil)
	if want := "[p]\n"; out != want || status != 0 {
		t.Errorf("typeset a$n=$y = %q (status %d), want %q at 0", out, status, want)
	}
}
