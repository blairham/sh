// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An unquoted `[*]` was joined whatever the dialect, which is bash's answer
// given to all four. It is the same question `[@]` asks — measured, an
// unquoted `${a[*]}` and an unquoted `${a[@]}` are the same fields in every
// shell in the panel — so it asks the same axis rather than a second one.
//
// Measured 2026-09-06 against bash 5.3.15, bash 3.2.57, that build invoked as
// `sh`, dash, ksh93u+ 2012-08-01 and zsh 5.9.2, on the snippets below.

// starRun answers the axes an unquoted list asks and nothing else.
func starRun(t *testing.T, src string, split, join Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = split
		s.GlobExpansionResults = Yes
		s.UnquotedListJoinsOnIFS = join
		s.ArrayBaseIsZero = No
		s.SubscriptCommaIsARange = Yes
		// The tail of the splitting rule as five of the six shells give it —
		// see the note in joinRun.
		s.TrailingSeparatorEndsAField = No
	})
}

// The row #981 left behind: `a=("x y" z); printf "[%s]" ${a[*]}` is `[x y][z]`
// in zsh and `[x][y][z]` in bash and ksh93.
//
// It is not the join that separates them here — it is the *splitting* answer,
// which this path never asked. The join was unconditional and so was the split
// that followed it, so an unquoted `[*]` came back as three fields in every
// dialect, including the one whose splitting is off. Handing the elements to
// the list path is what makes both questions reach it.
//
// With the elements joined there is one boundary left where there were two,
// and no later stage can put it back — which is why zsh's reading cannot be
// produced by joining and then declining to split: that gives one field.
func TestAnUnquotedStarSubscriptAsksTheSplittingAxis(t *testing.T) {
	const src = `a=("x y" z); set -- ${a[*]}; printf "%d" "$#"; printf "[%s]" "$@"`

	if out, st := starRun(t, src, Yes, Yes); out != "3[x][y][z]" || st != 0 {
		t.Errorf("splitting: got %q status %d, want %q at 0", out, st, "3[x][y][z]")
	}
	// zsh's answer, and the one #981 recorded as failing: the elements, whole.
	// The join answer cannot reach it, which is what the pair below says — a
	// shell that joined and did not split would give one field holding all
	// three words, and none does.
	for _, join := range []Answer{Yes, No} {
		if out, st := starRun(t, src, No, join); out != "2[x y][z]" || st != 0 {
			t.Errorf("not splitting, join=%v: got %q status %d, want %q at 0",
				join, out, st, "2[x y][z]")
		}
	}
}

// And where the split *is* on, the join is what separates bash from ksh93 —
// the same axis `${a[@]}` asks, reached by the second spelling.
//
// `IFS=:; a=("x:" y)` is `[x][][y]` in bash, whose join makes `x::y`, and
// `[x][y]` in ksh93 and dash, which split each element and lose the trailing
// separator. Nothing here is empty, so no rule about empty elements reaches it.
func TestAnUnquotedStarSubscriptAsksTheJoinAxis(t *testing.T) {
	const src = `IFS=:; a=("x:" y); set -- ${a[*]}; printf "%d" "$#"; printf "[%s]" "$@"`

	if out, st := starRun(t, src, Yes, Yes); out != "3[x][][y]" || st != 0 {
		t.Errorf("joined: got %q status %d, want %q at 0", out, st, "3[x][][y]")
	}
	if out, st := starRun(t, src, Yes, No); out != "2[x][y]" || st != 0 {
		t.Errorf("not joined: got %q status %d, want %q at 0", out, st, "2[x][y]")
	}
}

// The positional spelling is the same axis wearing a second face, and it had a
// branch of its own: a bare `$*` joined on the scalar path, so `IFS=:; set --
// x y; printf "[%s]" $*` was one field `x:y` in the zsh dialect where the
// shell gives two.
//
// One answer settles both, the way #1009's one fix settled `$@`, `${a[@]}` and
// a bare `$a` at once.
func TestABareUnquotedStarAsksTheSameAxes(t *testing.T) {
	// The issue's row: one field `x:y` in the zsh dialect, where the shell
	// gives two. The join happened on the scalar path and the split that
	// would have undone it never ran, because zsh does not split.
	const src = `IFS=:; set -- x y; set -- $*; printf "%d" "$#"; printf "[%s]" "$@"`
	for _, split := range []Answer{Yes, No} {
		for _, join := range []Answer{Yes, No} {
			if out, st := starRun(t, src, split, join); out != "2[x][y]" || st != 0 {
				t.Errorf("split=%v join=%v: got %q status %d, want %q at 0",
					split, join, out, st, "2[x][y]")
			}
		}
	}

	// The element that cannot survive a join, on the bare spelling.
	const held = `set -- "x y" z; set -- $*; printf "%d" "$#"; printf "[%s]" "$@"`
	if out, st := starRun(t, held, Yes, Yes); out != "3[x][y][z]" || st != 0 {
		t.Errorf("splitting: got %q status %d, want %q at 0", out, st, "3[x][y][z]")
	}
	if out, st := starRun(t, held, No, No); out != "2[x y][z]" || st != 0 {
		t.Errorf("not splitting: got %q status %d, want %q at 0", out, st, "2[x y][z]")
	}

	// And the join, where the split is on to show it.
	const sep = `IFS=:; set -- "x:" y; set -- $*; printf "%d" "$#"; printf "[%s]" "$@"`
	if out, st := starRun(t, sep, Yes, Yes); out != "3[x][][y]" || st != 0 {
		t.Errorf("joined: got %q status %d, want %q at 0", out, st, "3[x][][y]")
	}
	if out, st := starRun(t, sep, Yes, No); out != "2[x][y]" || st != 0 {
		t.Errorf("not joined: got %q status %d, want %q at 0", out, st, "2[x][y]")
	}
}

// The unquoted `[*]` also picks up the stage this path never performed: the
// result of an expansion is a pattern only where the dialect says it is, and
// the join returned its fields raw.
//
// zsh 5.9.2 leaves the star alone — `a=("zz*" other); printf "[%s]" ${a[*]}`
// is `[zz*][other]` there and `[zz1][other]` in bash and ksh93 — and ours
// matched the directory in every dialect.
func TestAnUnquotedStarSubscriptAsksTheGlobbingAxis(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zz1"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	src := `cd ` + dir + `; a=("zz*" other); set -- ${a[*]}; printf "%d" "$#"; printf "[%s]" "$@"`

	out, st := axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = Yes
		s.GlobExpansionResults = No
		s.UnquotedListJoinsOnIFS = No
	})
	if out != "2[zz*][other]" || st != 0 {
		t.Errorf("globbing off: got %q status %d, want %q at 0", out, st, "2[zz*][other]")
	}

	out, st = axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = Yes
		s.GlobExpansionResults = Yes
		s.UnquotedListJoinsOnIFS = No
	})
	if out != "2[zz1][other]" || st != 0 {
		t.Errorf("globbing on: got %q status %d, want %q at 0", out, st, "2[zz1][other]")
	}
}

// The quoted spellings are the guard, and they must not move: `"${a[*]}"` and
// `"$*"` are one field holding the elements joined on the first character of
// IFS in every shell measured, whatever this axis answers.
func TestTheQuotedStarAlwaysJoins(t *testing.T) {
	const sub = `IFS=-; a=("x y" z); set -- "${a[*]}"; printf "%d" "$#"; printf "[%s]" "$@"`
	const bare = `IFS=-; set -- "x y" z; set -- "$*"; printf "%d" "$#"; printf "[%s]" "$@"`

	for _, join := range []Answer{Yes, No, Unspecified} {
		if out, st := starRun(t, sub, Yes, join); out != "1[x y-z]" || st != 0 {
			t.Errorf(`join=%v: "${a[*]}" gave %q status %d, want one joined field`, join, out, st)
		}
		if out, st := starRun(t, bare, Yes, join); out != "1[x y-z]" || st != 0 {
			t.Errorf(`join=%v: "$*" gave %q status %d, want one joined field`, join, out, st)
		}
	}
}

// And `"$@"` keeps one field per parameter beside them, which is the half of
// the pair the join must never reach: the two spellings exist to differ inside
// quotes, and this change is about what they do outside.
func TestTheQuotedAtStillKeepsItsFields(t *testing.T) {
	const src = `IFS=-; set -- "x y" z; set -- "$@"; printf "%d" "$#"; printf "[%s]" "$@"`

	for _, join := range []Answer{Yes, No, Unspecified} {
		if out, st := starRun(t, src, No, join); out != "2[x y][z]" || st != 0 {
			t.Errorf(`join=%v: got %q status %d, want %q at 0`, join, out, st, "2[x y][z]")
		}
	}
}

// A subscript range is joined by the same rule and reaches the same place:
// `${a[1,2]}` unquoted is a list, and the elements go through the two stages
// like any other. Quoted, it is one joined field — measured, `"${a[1,2]}"` is
// `x y-z` under `IFS=-` exactly as `"$a"` is.
func TestAnUnquotedRangeReachesTheListPath(t *testing.T) {
	const src = `IFS=-; a=("x y" z w); set -- ${a[1,2]}; printf "%d" "$#"; printf "[%s]" "$@"`

	if out, st := starRun(t, src, No, No); out != "2[x y][z]" || st != 0 {
		t.Errorf("not joined: got %q status %d, want %q at 0", out, st, "2[x y][z]")
	}
	const quoted = `IFS=-; a=("x y" z w); set -- "${a[1,2]}"; printf "%d" "$#"; printf "[%s]" "$@"`
	for _, join := range []Answer{Yes, No, Unspecified} {
		if out, st := starRun(t, quoted, No, join); out != "1[x y-z]" || st != 0 {
			t.Errorf("join=%v quoted: got %q status %d, want one joined field", join, out, st)
		}
	}
}
