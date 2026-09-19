// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// Whether a successful **pattern** match writes the record a `=~` writes —
// Semantics.PatternMatchWritesTheMatchRecord, and #2916.
//
// Two dialects name such a record and they split on this, which is the whole
// reason it is an axis: measured 2026-09-19 over a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, stdin on `/dev/null`, with the record
// seeded by `[[ SEED =~ SEED ]]` on line 1 so that "nothing was written" reads
// back as `SEED` rather than as an empty array:
//
//	after                     	ksh93u+ 2012-08-01, `.sh.match`	bash 5.3.20, `BASH_REMATCH`
//	`[[ abcd == a*d ]]`       	`1:abcd`                       	`1:SEED`
//	`[[ abcd == a?(b)cd ]]`   	`2:abcd b`                     	`1:SEED`
//	`v=hello; ${v#he}`        	`1:he`                         	`1:SEED`
//	`w=abc; ${w/zz/Y}`        	`1:he` — the failure leaves it 	`1:SEED`
//	`case abcd in a*d) ;; esac`	`1:he` — a `case` never writes 	`1:SEED`
//
// bash was run with `-O extglob`, which is what makes `a?(b)cd` the same
// pattern in both columns rather than a comparison against six literal
// characters — and its answer does not move with the option.
//
// The other three dialects name no record at all, so the axis cannot be put to
// them; that is asserted here as an absence rather than left to a reader.
const patternRecordProbe = "[[ SEED =~ SEED ]]\n" +
	"[[ abcd == a*d ]]\n" +
	"v=hello; x=${v#he}\n" +
	"w=abc; x=${w/zz/Y}\n" +
	"case abcd in a*d) ;; esac\n"

func patternRecordPresets() []struct {
	dialecttest.Preset
	writes interp.Answer
	name   string
	want   string
} {
	return []struct {
		dialecttest.Preset
		writes interp.Answer
		name   string
		want   string
	}{
		{
			dialecttest.Preset{
				Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
				Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
			},
			interp.Yes, ".sh.match", "1:he",
		},
		{
			dialecttest.Preset{
				Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
				Diagnostics: bash.Diagnostics, Apply: bash.Apply,
			},
			interp.No, "BASH_REMATCH", "1:SEED",
		},
	}
}

func TestEachDialectAnswersWhetherAPatternWritesTheRecord(t *testing.T) {
	for _, p := range patternRecordPresets() {
		t.Run(p.Name, func(t *testing.T) {
			if got := p.Semantics().PatternMatchWritesTheMatchRecord; got != p.writes {
				t.Errorf("PatternMatchWritesTheMatchRecord = %v, want %v", got, p.writes)
			}
		})
	}
}

// The record itself, which is what the axis is for: a preset holding the right
// value and writing the wrong bytes would pass the test above.
//
// Three commands after the seed rather than one, because a single glob
// comparison cannot tell the columns apart from a `case`: the trim is the
// surface that writes even for a literal pattern, and the `case` is the one
// that never writes — so the ksh column's `1:he` is the trim's answer standing
// after an arm that did match.
func TestAPatternMatchsRecordIsWrittenInEveryDialect(t *testing.T) {
	for _, p := range patternRecordPresets() {
		t.Run(p.Name, func(t *testing.T) {
			src := patternRecordProbe +
				`printf '%s:%s' "${#` + p.name + `[@]}" "${` + p.name + `[*]}"` + "\n"
			out, _, err := p.Combined(t, dialecttest.Base{}, src)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			if out != p.want {
				t.Errorf("record was %q, want %q", out, p.want)
			}
		})
	}
}

// The three dialects that name no record cannot be asked, and say so by
// holding the axis unanswered — which is what keeps a shell keeping its
// captures under another shape, or keeping none, from answering a question
// about a parameter it does not have.
func TestADialectWithNoRecordLeavesTheAxisUnanswered(t *testing.T) {
	for _, p := range []dialecttest.Preset{
		{
			Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
			Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
		},
		{
			Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
			Diagnostics: dash.Diagnostics, Apply: dash.Apply,
		},
		{
			Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
			Diagnostics: ash.Diagnostics, Apply: ash.Apply,
		},
	} {
		t.Run(p.Name, func(t *testing.T) {
			if got := p.Semantics().PatternMatchWritesTheMatchRecord; got != interp.Unspecified {
				t.Errorf("PatternMatchWritesTheMatchRecord = %v, want it unanswered", got)
			}
		})
	}
}
