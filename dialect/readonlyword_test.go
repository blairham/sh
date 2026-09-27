// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// Which of the two things each dialect's `readonly` is — see
// interp.Semantics.ReadonlyWord. One table, because the split is the point:
// a per-dialect file would state five values and none of them would say that
// four are the same value and one is not.
//
// Measured 2026-09-27 from script files under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with a scratch HOME, asking each shell for every letter of the
// alphabet under `readonly` and again under its own declaration word:
//
//	zsh 5.9.2     readonly takes afghilptuxAEFHLRTUZ, which is typeset's
//	              set less k, m, r and z — the word *is* `typeset -r`
//	bash 5.3      readonly takes aAfpn where declare takes aAfFgilnprtux;
//	              `readonly -i v` is `invalid option`
//	ksh93u+       readonly takes p alone, where typeset takes far more
//	dash 0.5.12   readonly takes p alone, and there is no declaration word
//	BusyBox ash   readonly takes p and n, likewise
func readonlyWordPresets() []struct {
	dialecttest.Preset
	word interp.ReadonlyWordReading
} {
	return []struct {
		dialecttest.Preset
		word interp.ReadonlyWordReading
	}{
		{dialecttest.Preset{
			Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
			Diagnostics: bash.Diagnostics, Apply: bash.Apply,
		}, interp.ReadonlyWordIsAnAttribute},
		{dialecttest.Preset{
			Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
			Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
		}, interp.ReadonlyWordIsTheDeclaration},
		{dialecttest.Preset{
			Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
			Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
		}, interp.ReadonlyWordIsAnAttribute},
		{dialecttest.Preset{
			Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
			Diagnostics: dash.Diagnostics, Apply: dash.Apply,
		}, interp.ReadonlyWordIsAnAttribute},
		{dialecttest.Preset{
			Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
			Diagnostics: ash.Diagnostics, Apply: ash.Apply,
		}, interp.ReadonlyWordIsAnAttribute},
	}
}

func TestEachDialectAnswersWhatTheReadonlyWordIs(t *testing.T) {
	for _, p := range readonlyWordPresets() {
		t.Run(p.Name, func(t *testing.T) {
			if got := p.Semantics().ReadonlyWord; got != p.word {
				t.Errorf("ReadonlyWord = %v, want %v", got, p.word)
			}
		})
	}
}

// And the value run, so the answer above is a fact about the shell rather
// than a field nobody reads. The integer letter is the row, because it is the
// one whose two outcomes cannot be confused: under the declaration reading
// the value is arithmetic, and under the attribute reading the letter is not
// this word's to take at all.
func TestTheReadonlyWordTakesTheDeclarationsLettersOnlyWhereItIsOne(t *testing.T) {
	for _, p := range readonlyWordPresets() {
		t.Run(p.Name, func(t *testing.T) {
			out, _, err := p.Combined(t, dialecttest.Base{},
				`readonly -i v=2+3`+"\n"+`echo "[${v-unset}]"`)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if p.word == interp.ReadonlyWordIsTheDeclaration {
				if !strings.Contains(out, "[5]") {
					t.Errorf("`readonly -i v=2+3` wrote %q, want the arithmetic value", out)
				}
				return
			}
			if strings.Contains(out, "[5]") {
				t.Errorf("`readonly -i v=2+3` wrote %q, want the letter refused "+
					"rather than the declaration's reading of it", out)
			}
			if !strings.Contains(out, "-i") {
				t.Errorf("`readonly -i v=2+3` wrote %q, want the letter named in a refusal", out)
			}
		})
	}
}
