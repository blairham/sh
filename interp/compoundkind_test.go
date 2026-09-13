// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A name is one kind of array at a time, and a declaration naming the *other*
// kind is a conflict the panel answers four ways: refused, refused fatally,
// converted with the elements carried over, and converted with them gone.
//
// It was unanswered in one direction and wrong in the other — `typeset -a`
// over a table left the table standing and said nothing, so no column agreed,
// and `typeset -A` over an array emptied it in every dialect, which is right
// for one column of three (#1375).
func runKind(t *testing.T, table, array CompoundKindChangePolicy, src string) (string, int) {
	t.Helper()
	return runKindWith(t, src, func(s *Semantics) {
		s.TableUnderAnArrayDeclaration = table
		s.ArrayUnderATableDeclaration = array
	})
}

// runKindLiteral is runKind for the shape whose value is the declaration's own
// array literal, which is the other pair of fields — see
// Semantics.TableUnderAnArrayLiteralDeclaration and #2287.
func runKindLiteral(t *testing.T, table, array CompoundKindChangePolicy, src string) (string, int) {
	t.Helper()
	return runKindWith(t, src, func(s *Semantics) {
		s.TableUnderAnArrayLiteralDeclaration = table
		s.ArrayUnderATableLiteralDeclaration = array
	})
}

func runKindWith(t *testing.T, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ArrayLiteral = true
		d.DeclarationUtilities = map[string]bool{"typeset": true, "local": true}
	}, func(r *Runner) {
		sem := *r.Semantics
		set(&sem)
		r.Semantics = &sem
		d := CoreDiagnostics()
		if r.Diagnostics != nil {
			d = *r.Diagnostics
		}
		d.CannotConvertTableToArray = "%[2]s: %[1]s: cannot convert associative to indexed array"
		d.CannotConvertArrayToTable = "%[2]s: %[1]s: cannot convert indexed to associative array"
		// One verb, which is the measured difference between the two sites.
		d.CannotConvertTableToArrayAtTheAssignment = "%[1]s: cannot convert associative to indexed array"
		d.CannotConvertArrayToTableAtTheAssignment = "%[1]s: cannot convert indexed to associative array"
		r.Diagnostics = &d
	})
}

func TestAnArrayDeclarationOverATableIsAnAxis(t *testing.T) {
	const src = `typeset -A h; h[k]=v; typeset -a h; echo "st=$?"; typeset -p h; echo after`

	t.Run("refused", func(t *testing.T) {
		out, _ := runKind(t, CompoundKindChangeRefused, CompoundKindChangeRefused, src)
		if !strings.Contains(out, "typeset: h: cannot convert associative to indexed array") {
			t.Errorf("= %q, want the refusal naming the builtin and the name", out)
		}
		// The table is untouched, the status is 1, and the next command runs.
		for _, want := range []string{"st=1", "[k]", "after"} {
			if !strings.Contains(out, want) {
				t.Errorf("= %q, want it to contain %q", out, want)
			}
		}
	})

	t.Run("refused and the script ends", func(t *testing.T) {
		out, st := runKind(t, CompoundKindChangeEndsTheScript, CompoundKindChangeRefused, src)
		if !strings.Contains(out, "cannot convert") {
			t.Errorf("= %q, want the refusal", out)
		}
		if strings.Contains(out, "after") || strings.Contains(out, "st=") || st == 0 {
			t.Errorf("= %q (status %d), want the input to end", out, st)
		}
	})

	t.Run("converted and emptied", func(t *testing.T) {
		out, st := runKind(t, CompoundKindChangeEmptiesTheName, CompoundKindChangeEmptiesTheName, src)
		if strings.Contains(out, "cannot convert") || st != 0 {
			t.Errorf("= %q (status %d), want a silent conversion", out, st)
		}
		if !strings.Contains(out, "st=0") || strings.Contains(out, "[k]") {
			t.Errorf("= %q, want the table gone", out)
		}
	})
}

func TestATableDeclarationOverAnArrayIsAnAxis(t *testing.T) {
	const src = `typeset -a a=(x y); typeset -A a; echo "st=$?"; printf "[%s][%s]" "${a[0]}" "${a[1]}"; echo " after"`

	t.Run("refused", func(t *testing.T) {
		out, _ := runKind(t, CompoundKindChangeRefused, CompoundKindChangeRefused, src)
		if !strings.Contains(out, "typeset: a: cannot convert indexed to associative array") {
			t.Errorf("= %q, want the refusal in the other direction's words", out)
		}
		// Untouched: the elements are still where they were, read by index.
		if !strings.Contains(out, "st=1") || !strings.Contains(out, "[x][y]") {
			t.Errorf("= %q, want the array intact at status 1", out)
		}
	})

	t.Run("the elements are carried over as keys", func(t *testing.T) {
		out, st := runKind(t, CompoundKindChangeRefused, CompoundKindChangeKeepsTheElements, src)
		if strings.Contains(out, "cannot convert") || st != 0 {
			t.Errorf("= %q (status %d), want a silent conversion", out, st)
		}
		// The values survive, and they survive under the keys `0` and `1` —
		// which is what says they were carried and not merely printed.
		if !strings.Contains(out, "[x][y]") {
			t.Errorf("= %q, want the elements under the keys 0 and 1", out)
		}
	})

	t.Run("the elements are gone", func(t *testing.T) {
		out, st := runKind(t, CompoundKindChangeEmptiesTheName, CompoundKindChangeEmptiesTheName, src)
		if strings.Contains(out, "cannot convert") || st != 0 {
			t.Errorf("= %q (status %d), want a silent conversion", out, st)
		}
		if !strings.Contains(out, "[][]") {
			t.Errorf("= %q, want the elements gone", out)
		}
	})
}

// A sparse array carries its own subscripts across rather than a run from
// zero, which is what says the keys are the array's and not the loop's.
func TestTheKeysCarriedOverAreTheSubscriptsTheArrayHeld(t *testing.T) {
	const src = `typeset -a a; a[0]=x; a[5]=q; typeset -A a; printf "[%s][%s][%s]" "${a[0]}" "${a[5]}" "${a[1]}"`
	out, st := runKind(t, CompoundKindChangeRefused, CompoundKindChangeKeepsTheElements, src)
	if out != "[x][q][]" || st != 0 {
		t.Errorf("= %q (status %d), want %q at 0", out, st, "[x][q][]")
	}
}

// The axis is asked where the two kinds actually collide and nowhere else.
func TestTheCompoundKindAxesAreAskedAtTheCollision(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		asked     bool
	}{
		{"an array letter over a table", `typeset -A h; typeset -a h`, true},
		{"a table letter over an array", `typeset -a a=(x); typeset -A a`, true},
		// A redeclaration of the kind the name already is asks nothing.
		{"the same letter twice", `typeset -A h; typeset -A h`, false},
		{"the array letter twice", `typeset -a a=(x); typeset -a a`, false},
		// An unset name and a scalar are other questions.
		{"an unset name", `typeset -a a`, false},
		{"a scalar", `b=1; typeset -a b`, false},
		// A declaration carrying a *scalar* value is a third question again
		// and reaches neither pair of fields.
		{"a declaration with a scalar value", `typeset -A h; h[k]=v; typeset -a h=x`, false},
		// And one carrying its own array literal reaches the *other* pair,
		// so these fields must still say nothing about it — the row that
		// would fail if the literal form were folded into them.
		{"a declaration with a literal", `typeset -A h; h[k]=v; typeset -a h=(x)`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runKind(t, CompoundKindChangeUnspecified, CompoundKindChangeUnspecified, tc.src)
			// The whole phrase, because the literal pair's reads `an array
			// *literal* declaration over a name already declared a table` —
			// a Contains on the tail would call that this axis and pass the
			// row it exists to fail.
			said := strings.Contains(out, "an array declaration over a name already declared a table") ||
				strings.Contains(out, "a table declaration over a name already holding an array")
			if said != tc.asked {
				t.Fatalf("got %q, want the axis %s", out,
					map[bool]string{true: "asked", false: "not asked"}[tc.asked])
			}
		})
	}
}

// The same collision reached by a declaration carrying its own **array
// literal**, which is a second pair of fields because the panel splits
// differently under it — see Semantics.TableUnderAnArrayLiteralDeclaration
// and #2287.

// The array letter with a literal over a table, all three answers.
func TestAnArrayLiteralDeclarationOverATableIsAnAxis(t *testing.T) {
	const src = `typeset -A h; h[k]=v
typeset -a h=(x)
echo "st=$?"; typeset -p h; echo after`

	t.Run("converted", func(t *testing.T) {
		out, st := runKindLiteral(t, CompoundKindChangeEmptiesTheName, CompoundKindChangeEmptiesTheName, src)
		if strings.Contains(out, "cannot convert") || st != 0 {
			t.Errorf("= %q (status %d), want a silent conversion", out, st)
		}
		// The literal's element is there and the table's key is not: the
		// literal replaces what it lands on rather than joining it, and the
		// key is the half this engine used to keep.
		if !strings.Contains(out, "[0]") || strings.Contains(out, "[k]") {
			t.Errorf("= %q, want the literal's element and no key", out)
		}
	})

	t.Run("refused, and the command list abandoned", func(t *testing.T) {
		// The follow-ups on the *same* line as the refusal, which is what
		// the abandon is about: the row above has them on the next line and
		// they run there, so a test written that way would pass for a policy
		// that abandoned nothing at all.
		const sameLine = `typeset -A h; h[k]=v
typeset -a h=(x); echo "st=$?"; typeset -p h; echo after`
		out, _ := runKindLiteral(t, CompoundKindChangeAbandonsTheLine, CompoundKindChangeAbandonsTheLine, sameLine)
		// One verb: the complaint comes from the assignment, so it names no
		// builtin. A shared wording with the valueless form would print an
		// empty name and a stray colon here.
		if !strings.Contains(out, "h: cannot convert associative to indexed array") {
			t.Errorf("= %q, want the one-verb refusal", out)
		}
		if strings.Contains(out, "typeset: h: cannot") {
			t.Errorf("= %q, want no builtin in the sentence", out)
		}
		// The rest of the *line* is gone and the table is untouched.
		for _, unwanted := range []string{"st=", "after"} {
			if strings.Contains(out, unwanted) {
				t.Errorf("= %q, want nothing after the refusal on that line", out)
			}
		}
	})
}

// The abandon costs the line and not the input, which is the square #1182
// records and the one #2287 was filed on the wrong cell of.
func TestTheAbandonedLiteralConversionRunsTheNextLine(t *testing.T) {
	const src = `typeset -A h; h[k]=v
typeset -a h=(x); echo "st=$?"; echo same-line
echo next-line`
	out, _ := runKindLiteral(t, CompoundKindChangeAbandonsTheLine, CompoundKindChangeAbandonsTheLine, src)
	if strings.Contains(out, "same-line") {
		t.Errorf("= %q, want the rest of the list gone", out)
	}
	if !strings.Contains(out, "next-line") {
		t.Errorf("= %q, want the next line to run", out)
	}
}

// The other direction, and the row that says the converting answer *empties*:
// the array's elements do not come across as keys the way the valueless table
// letter carries them in one column.
func TestATableLiteralDeclarationOverAnArrayIsAnAxis(t *testing.T) {
	const src = `typeset -a a=(x y)
typeset -A a=([k]=v)
echo "st=$?"; printf "[%s][%s]" "${a[k]}" "${a[0]}"; echo; echo after`

	out, st := runKindLiteral(t, CompoundKindChangeEmptiesTheName, CompoundKindChangeEmptiesTheName, src)
	if st != 0 || strings.Contains(out, "cannot convert") {
		t.Errorf("= %q (status %d), want a silent conversion", out, st)
	}
	if !strings.Contains(out, "[v][]") {
		t.Errorf("= %q, want the literal's key and the old elements gone", out)
	}

	const sameLine = `typeset -a a=(x y)
typeset -A a=([k]=v); echo "st=$?"; echo after`
	out, _ = runKindLiteral(t, CompoundKindChangeAbandonsTheLine, CompoundKindChangeAbandonsTheLine, sameLine)
	if !strings.Contains(out, "a: cannot convert indexed to associative array") {
		t.Errorf("= %q, want the one-verb refusal for this direction", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("= %q, want the rest of the line abandoned", out)
	}
}

// An unanswered literal axis refuses rather than picking a shell.
func TestAnUnansweredLiteralConversionRefuses(t *testing.T) {
	out, st := runKindLiteral(t, CompoundKindChangeUnspecified, CompoundKindChangeUnspecified,
		`typeset -A h; h[k]=v; typeset -a h=(x)`)
	if st == 0 || !strings.Contains(out, "an array literal declaration over a name already declared a table") {
		t.Errorf("= %q (status %d), want the unanswered refusal", out, st)
	}
}

// And a literal over a name that is *not* the other kind asks nothing, which
// is what keeps the axis off every array literal in every script.
func TestALiteralOverTheSameKindAsksNothing(t *testing.T) {
	for _, src := range []string{
		`typeset -a a=(p); typeset -a a=(x); typeset -p a`,
		`typeset -A m=([j]=1); typeset -A m=([k]=v); typeset -p m`,
		`typeset -a a=(x); typeset -p a`,
	} {
		out, st := runKindLiteral(t, CompoundKindChangeUnspecified, CompoundKindChangeUnspecified, src)
		if st != 0 || strings.Contains(out, "disagree here") {
			t.Errorf("%s: = %q (status %d), want no question at all", src, out, st)
		}
	}
}
