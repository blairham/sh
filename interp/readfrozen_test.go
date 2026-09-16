// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `read` whose name a freeze refuses — see interp/readfrozen.go for the
// panel these rows come from.
//
// The first assertion of every row is that the *line* survives, which is what
// #3208 was: the refusal went through the bare-assignment path and gave up
// the rest of what the shell was running, so the status and the values after
// it were never reached in any dialect.

func frozenReadRun(t *testing.T, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := permissive()
		sem.ReadonlyRefusalInABuiltinIsFatal = No
		sem.ReadRefusedWriteEndsTheBuiltin = Yes
		sem.ReadRefusedWriteIsOneOnTheLastName = No
		if set != nil {
			set(&sem)
		}
		dg := Diagnostics{ReadonlyVariable: "%s: is read only"}
		r.Semantics, r.Diagnostics = &sem, &dg
	})
}

func TestAFrozenReadNameReportsAndLetsTheLineRun(t *testing.T) {
	const src = `a=A; readonly a; printf 'x\n' | { read a; echo "st=$?"; printf '[%s]\n' "$a"; }; echo tail`
	out, st := frozenReadRun(t, src, nil)
	if st != 0 {
		t.Errorf("status %d, want the enclosing script to have finished", st)
	}
	for _, want := range []string{"a: is read only", "st=2", "[A]", "tail"} {
		if !strings.Contains(out, want) {
			t.Errorf("out=%q, want %q in it", out, want)
		}
	}
}

// Whether the builtin stops there, which is the row that says it gave up
// rather than merely complained: the name after the frozen one is filled in
// the column that carries on and left alone in the three that do not.
func TestAFrozenReadNameStopsTheBuiltinWhereTheDialectSaysSo(t *testing.T) {
	const src = `a=A; b=B; readonly a; printf 'x y\n' | { read a b; echo "st=$?"; printf '[%s][%s]\n' "$a" "$b"; }`
	out, _ := frozenReadRun(t, src, nil)
	if !strings.Contains(out, "[A][B]") || !strings.Contains(out, "st=2") {
		t.Errorf("out=%q, want the second name left alone at status 2", out)
	}
	out, _ = frozenReadRun(t, src, func(s *Semantics) {
		s.ReadRefusedWriteEndsTheBuiltin = No
	})
	if !strings.Contains(out, "[A][y]") || !strings.Contains(out, "st=1") {
		t.Errorf("out=%q, want the second name filled at status 1", out)
	}
	// And the column that carries on reports *every* frozen name, where the
	// one that stops reports the first and no more.
	const both = `a=A; b=B; readonly a b; printf 'x y\n' | { read a b; echo "st=$?"; }`
	out, _ = frozenReadRun(t, both, func(s *Semantics) {
		s.ReadRefusedWriteEndsTheBuiltin = No
	})
	if n := strings.Count(out, "is read only"); n != 2 {
		t.Errorf("out=%q says it %d times, want one per frozen name", out, n)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("out=%q, want one status for two refusals", out)
	}
	out, _ = frozenReadRun(t, both, nil)
	if n := strings.Count(out, "is read only"); n != 1 {
		t.Errorf("out=%q says it %d times, want the builtin to have stopped at the first", out, n)
	}
}

// The status, which is a second measurement and not a restatement of the
// stop: the three columns that stop do not agree about it.
func TestAFrozenReadNameStatusCountsWhatWasLeft(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		last      Answer
		want      string
	}{
		// Over three names, the column that parts them answers 2, 2, 1.
		{"the first of three", `a=A;b=B;c=C; readonly a`, Yes, "st=2"},
		{"the middle of three", `a=A;b=B;c=C; readonly b`, Yes, "st=2"},
		{"the last of three", `a=A;b=B;c=C; readonly c`, Yes, "st=1"},
		// And the column that does not answers 2 for all three.
		{"the last of three, unparted", `a=A;b=B;c=C; readonly c`, No, "st=2"},
		{"the first of three, unparted", `a=A;b=B;c=C; readonly a`, No, "st=2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src + `; printf 'x y z\n' | { read a b c; echo "st=$?"; }`
			out, _ := frozenReadRun(t, src, func(s *Semantics) {
				s.ReadRefusedWriteIsOneOnTheLastName = tc.last
			})
			if !strings.Contains(out, tc.want) {
				t.Errorf("out=%q, want %q", out, tc.want)
			}
		})
	}
	// A `read` with no operands has no last name for the question to be
	// about: the name it fills is the shell's own, and the column that would
	// answer 1 for a last operand answers 2 here.
	out, _ := frozenReadRun(t,
		`REPLY=R; readonly REPLY; printf 'x\n' | { read; echo "st=$?"; }`,
		func(s *Semantics) { s.ReadRefusedWriteIsOneOnTheLastName = Yes })
	if !strings.Contains(out, "st=2") {
		t.Errorf("out=%q, want 2 for a name the script did not write", out)
	}
}

// The fatal column, which is asked before either axis above and so reaches
// neither.
func TestAFrozenReadNameEndsTheScriptWhereTheDialectSaysSo(t *testing.T) {
	out, st := frozenReadRun(t,
		`a=A; readonly a; read a <<'EOT'
x
EOT
echo reached`,
		func(s *Semantics) { s.ReadonlyRefusalInABuiltinIsFatal = Yes })
	if strings.Contains(out, "reached") {
		t.Errorf("out=%q, want the script ended", out)
	}
	if st == 0 {
		t.Error("status 0, want the fatal refusal reported")
	}
}

// The wording, and the one column that calls it a warning.
func TestAFrozenReadNameCanBeWordedAsAWarning(t *testing.T) {
	const src = `a=A; readonly a; printf 'x\n' | { read a; echo "st=$?"; }`
	out, _ := run(t, src, func(r *Runner) {
		sem := permissive()
		sem.ReadonlyRefusalInABuiltinIsFatal = No
		sem.ReadRefusedWriteEndsTheBuiltin = No
		dg := Diagnostics{
			ReadonlyVariable:              "%s: is read only",
			ReadonlyVariableInDeclaration: "%[2]s: %[1]s: is read only",
			ReadonlyVariableInRead:        "%[2]s: warning: %[1]s: is read only",
			ReadonlyRefusalNamesBuiltin:   map[string]bool{"read": true},
		}
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	if !strings.Contains(out, "read: warning: a: is read only") {
		t.Errorf("out=%q, want the builtin named and the refusal called a warning", out)
	}
	// Empty falls back to the declaration wording, which is what the two
	// columns that name `read` and do not call it a warning want.
	out, _ = run(t, src, func(r *Runner) {
		sem := permissive()
		sem.ReadonlyRefusalInABuiltinIsFatal = No
		sem.ReadRefusedWriteEndsTheBuiltin = Yes
		sem.ReadRefusedWriteIsOneOnTheLastName = No
		dg := Diagnostics{
			ReadonlyVariable:              "%s: is read only",
			ReadonlyVariableInDeclaration: "%[2]s: %[1]s: is read only",
			ReadonlyRefusalNamesBuiltin:   map[string]bool{"read": true},
		}
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	if !strings.Contains(out, "read: a: is read only") || strings.Contains(out, "warning") {
		t.Errorf("out=%q, want the declaration wording with no warning in it", out)
	}
}

// A subscripted operand is refused by its array's freeze and named by it,
// which is the name the store would have landed on.
func TestAFrozenArrayRefusesASubscriptedReadOperand(t *testing.T) {
	out, _ := frozenReadRun(t,
		`a=1; readonly a; printf 'v\n' | { read 'a[0]'; echo "st=$?"; }; echo tail`,
		nil)
	if !strings.Contains(out, "a: is read only") || strings.Contains(out, "a[0]: is read only") {
		t.Errorf("out=%q, want the array named rather than the operand as written", out)
	}
	// The status is what says the *builtin* refused rather than the store
	// underneath it: a resolution that missed the array would fall through
	// to the bare-assignment path, which reports the same sentence and then
	// gives up the line, so `st=` is the only line that parts them.
	if !strings.Contains(out, "st=2") {
		t.Errorf("out=%q, want the builtin's own refusal and status", out)
	}
	if !strings.Contains(out, "tail") {
		t.Errorf("out=%q, want the line to have run on", out)
	}
}

// An axis nothing answered refuses rather than taking a side.
func TestAFrozenReadNameWithNoAnswerRefuses(t *testing.T) {
	out, _ := run(t, `a=A; readonly a; printf 'x\n' | { read a; echo "st=$?"; }`, func(r *Runner) {
		sem := permissive()
		sem.ReadonlyRefusalInABuiltinIsFatal = No
		dg := Diagnostics{ReadonlyVariable: "%s: is read only"}
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	if !strings.Contains(out, "ending the builtin rather than only being reported") {
		t.Errorf("out=%q, want the axis named", out)
	}
}
