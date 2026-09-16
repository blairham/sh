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

// `set -k` — POSIX's keyword option — across the five dialects, which is
// where it belongs because the panel splits three ways over one letter.
//
// Measured 2026-09-16, C locale, from a script file, with
// `f() { echo "1=[$1] KV=[${KV-unset}] n=$#"; }` and `f KV=2 a`:
//
//	bash 5.3.20, bash-as-sh, bash 3.2.57   1=[a] KV=[2] n=1
//	ksh93u+ 2012-08-01                     1=[a] KV=[2] n=1
//	zsh 5.9.2                              1=[KV=2] KV=[unset] n=2
//	dash 0.5.12                            set: Illegal option -k, and the file ends
//	BusyBox ash 1.37.0                     set: illegal option -k, and the file ends
//
// zsh's `-k` is `interactivecomments`, a different option under the same
// letter, and its `set -o keyword` is `no such option` — so zsh's positional
// is zsh's answer rather than this shell's gap, and the same is true of dash's
// and BusyBox ash's refusal. One table says that; five per-dialect files would
// each state a value and none of them would state the split (#3095).
func keywordPresets() []struct {
	dialecttest.Preset
	has      interp.Answer
	declares interp.Answer
} {
	return []struct {
		dialecttest.Preset
		has      interp.Answer
		declares interp.Answer
	}{
		{dialecttest.Preset{
			Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
			Diagnostics: bash.Diagnostics, Apply: bash.Apply,
		}, interp.Yes, interp.Yes},
		{dialecttest.Preset{
			Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
			Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
		}, interp.Yes, interp.No},
		{dialecttest.Preset{
			Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
			Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
		}, interp.No, interp.Unspecified},
		{dialecttest.Preset{
			Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
			Diagnostics: dash.Diagnostics, Apply: dash.Apply,
		}, interp.No, interp.Unspecified},
		{dialecttest.Preset{
			Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
			Diagnostics: ash.Diagnostics, Apply: ash.Apply,
		}, interp.No, interp.Unspecified},
	}
}

func TestEachDialectAnswersTheKeywordAxes(t *testing.T) {
	for _, p := range keywordPresets() {
		t.Run(p.Name, func(t *testing.T) {
			s := p.Semantics()
			if got := s.KeywordAssignments; got != p.has {
				t.Errorf("KeywordAssignments = %v, want %v", got, p.has)
			}
			if got := s.KeywordPromotesADeclarationsOperand; got != p.declares {
				t.Errorf("KeywordPromotesADeclarationsOperand = %v, want %v", got, p.declares)
			}
		})
	}
}

// The repro out of #3095, in every column.
//
// The refusal in the two shells that have no such option is part of the
// answer and not an omission: dash and BusyBox ash end the file over the
// letter, which is measured, so what follows it never runs there.
func TestTheKeywordOptionAcrossThePanel(t *testing.T) {
	for _, p := range keywordPresets() {
		t.Run(p.Name, func(t *testing.T) {
			out, _, err := p.Combined(t, dialecttest.Base{},
				`f() { echo "1=[$1] KV=[${KV-unset}] n=$#"; }`+"\n"+
					`set -k`+"\n"+
					`f KV=2 a`)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			want := "1=[a] KV=[2] n=1"
			switch p.Name {
			case "zsh":
				// The letter is this shell's `interactivecomments`, taken
				// silently, and the word stays where it stood.
				want = "1=[KV=2] KV=[unset] n=2"
			case "dash", "ash":
				// Refused, and the file ends there — so the line after it
				// never runs and the only output is the complaint.
				want = ""
			}
			got := strings.TrimSpace(out)
			if p.has == interp.No && p.Name != "zsh" {
				if !strings.Contains(got, "-k") || strings.Contains(got, "KV=") {
					t.Errorf("wrote %q, want a refusal of the letter and nothing after it", got)
				}
				return
			}
			if got != want {
				t.Errorf("wrote %q, want %q", got, want)
			}
		})
	}
}

// The split between the two shells that have the option, in both directions.
//
// Neither row is a wording: a script that writes `export NAME=value` under
// `set -k` exports nothing in bash — the option takes the operand, `export` is
// left with none and lists the environment instead — and exports in ksh93. And
// a promoted assignment outlives the call in ksh93 and does not in bash, which
// is AssignmentPrefixPersistsAfterAFunction answering for it rather than an axis of
// its own: a promoted assignment is an ordinary prefix assignment from the
// moment it is promoted.
func TestWhereTheTwoShellsWithTheOptionPartCompany(t *testing.T) {
	for _, p := range keywordPresets() {
		if p.has != interp.Yes {
			continue
		}
		t.Run(p.Name+"/a declaration's operand", func(t *testing.T) {
			out, _, err := p.Combined(t, dialecttest.Base{},
				"set -k\n"+
					"export E1=e1 > /dev/null\n"+
					`echo "[${E1-unset}]"`)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			want := "[unset]"
			if p.declares == interp.No {
				want = "[e1]"
			}
			if got := strings.TrimSpace(out); got != want {
				t.Errorf("export under -k wrote %q, want %q", got, want)
			}
		})
		t.Run(p.Name+"/outliving the call", func(t *testing.T) {
			out, _, err := p.Combined(t, dialecttest.Base{},
				"f() { :; }\n"+
					"set -k\n"+
					"f KV=2 a\n"+
					`echo "[${KV-unset}]"`)
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			want := "[unset]"
			if p.Semantics().AssignmentPrefixPersistsAfterAFunction == interp.Yes {
				want = "[2]"
			}
			if got := strings.TrimSpace(out); got != want {
				t.Errorf("after the call, KV is %q, want %q", got, want)
			}
		})
	}
}

// The letter and the long name are one state, and `$-` says which.
//
// Two spellings of one question cannot be allowed to answer differently — the
// rule `-h` and `hashall` already follow — so the letter is asked to turn the
// name's state off and the name to turn the letter's back on.
func TestTheLetterAndTheNameAreOneState(t *testing.T) {
	for _, p := range keywordPresets() {
		if p.has != interp.Yes {
			continue
		}
		t.Run(p.Name, func(t *testing.T) {
			out, _, err := p.Combined(t, dialecttest.Base{},
				"g() { echo \"n=$#\"; }\n"+
					"set -o keyword\n"+
					`case "$-" in *k*) echo "letter after the name" ;; *) echo "no letter after the name" ;; esac`+"\n"+
					"g A=1 z\n"+
					"set +k\n"+
					`case "$-" in *k*) echo "letter after +k" ;; *) echo "no letter after +k" ;; esac`+"\n"+
					"g A=2 z")
			if err != nil {
				t.Fatalf("err %v: %s", err, out)
			}
			want := "letter after the name\nn=1\nno letter after +k\nn=2"
			if got := strings.TrimSpace(out); got != want {
				t.Errorf("wrote\n%s\nwant\n%s", got, want)
			}
		})
	}
}
