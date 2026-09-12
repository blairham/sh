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
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ArrayLiteral = true
		d.DeclarationUtilities = map[string]bool{"typeset": true, "local": true}
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.TableUnderAnArrayDeclaration = table
		sem.ArrayUnderATableDeclaration = array
		r.Semantics = &sem
		d := CoreDiagnostics()
		if r.Diagnostics != nil {
			d = *r.Diagnostics
		}
		d.CannotConvertTableToArray = "%[2]s: %[1]s: cannot convert associative to indexed array"
		d.CannotConvertArrayToTable = "%[2]s: %[1]s: cannot convert indexed to associative array"
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
		// And a declaration carrying its own value is a second question the
		// panel splits differently: it is left where it was.
		{"a declaration with a value", `typeset -A h; h[k]=v; typeset -a h=(x)`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runKind(t, CompoundKindChangeUnspecified, CompoundKindChangeUnspecified, tc.src)
			said := strings.Contains(out, "already declared a table") ||
				strings.Contains(out, "already holding an array")
			if said != tc.asked {
				t.Fatalf("got %q, want the axis %s", out,
					map[bool]string{true: "asked", false: "not asked"}[tc.asked])
			}
		})
	}
}
