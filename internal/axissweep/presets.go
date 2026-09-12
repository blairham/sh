// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// Preset is one shell under dialect/ and the vector it ships.
//
// This is the roster the whole package counts from, and it is one list rather
// than several on purpose. What it replaced was a hardcoded four-entry slice
// with a comment promising that "a dialect gained tomorrow is a compile error
// here rather than a column quietly missing from the sweep". The promise was
// false twice over — the thing it named was a `switch` on a string with a
// `panic` default, so a new dialect produced no compile error anywhere, and
// its only failure mode was a run-time panic on a name nothing ever passed —
// and a column quietly missing from the sweep is exactly what happened: ash
// was written, was in nobody's list, and shipped `read -t 1 x` refusing at
// run time with `go test ./...` green (#2272, #2340).
//
// So the list is asserted against the packages on disk instead of promised
// about. See TestPresetsAreEveryDialectPackage: a new directory under
// dialect/ fails that test until it is named here, which is a check that
// exists rather than a comment that claims one does.
type Preset struct {
	// Name is the dialect's package name and its -dialect value.
	Name string
	// Semantics is the vector that dialect ships.
	Semantics interp.Semantics
	// Against is the oracle panel column this dialect's answers are graded
	// against, and empty when the shell it imitates cannot be run here.
	Against string
	// Ungraded is why there is no column, when there is none. It is the
	// reason a coverage check that costs no processes matters more for that
	// dialect than for the others, so it is carried rather than implied.
	Ungraded string
}

// Presets are every dialect this module ships, graded or not.
func Presets() []Preset {
	return []Preset{
		{Name: "bash", Semantics: bash.Semantics(), Against: "bash"},
		{Name: "zsh", Semantics: zsh.Semantics(), Against: "zsh"},
		{Name: "ksh", Semantics: ksh.Semantics(), Against: "ksh93"},
		{Name: "dash", Semantics: dash.Semantics(), Against: "dash"},
		{
			Name: "ash", Semantics: ash.Semantics(),
			Ungraded: "the oracle panel locates its shells with exec.LookPath and " +
				"there is no BusyBox ash on a macOS machine, so this dialect has no " +
				"column in the golden record (#2263)",
		},
	}
}

// Target is one dialect graded against the shell it claims to be.
//
// The unit is a dialect and not the struct, because an axis is answered per
// dialect: the two that were caught by hand had the same field vacuous in zsh
// *and* in bash, and one report for the pair would have hidden which.
type Target struct {
	// Dialect is the -dialect value, and Against is the panel column its
	// answers are graded against.
	Dialect string
	Against string
	// Semantics is the vector that dialect ships, which is what says what
	// each axis currently holds and therefore which values are a flip.
	Semantics interp.Semantics
}

// Targets are the dialects the flip sweep can grade: those with a panel
// column to grade against.
//
// It is derived from Presets rather than listed, so the difference between
// the two lists is exactly "which shells are installed" and a dialect can
// only fall out of the sweep for that stated reason. core and posix are in
// neither: neither imitates a shell, so there is no column to grade either
// against and no disagreement for a row to record.
func Targets() []Target {
	var out []Target
	for _, p := range Presets() {
		if p.Against == "" {
			continue
		}
		out = append(out, Target{Dialect: p.Name, Against: p.Against, Semantics: p.Semantics})
	}
	return out
}
