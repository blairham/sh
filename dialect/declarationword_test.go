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

// How a declaration utility has to be *written* for its `name=value` operand
// to be an assignment, across the five dialects — one table, because the panel
// splits three ways over one question and five per-dialect files would each
// state a value and none of them would state the split.
//
// Measured 2026-09-16 from script files, C locale, `b='x y'`, each line inside
// a function and in a subshell of its own, for every declaration utility each
// shell has — export, readonly and local everywhere they exist, plus typeset
// and declare in bash, and typeset in ksh93:
//
//	                        bash 5.3  zsh 5.9.2  ksh93u+  dash    ash
//	export v=$b             [x y]     [x y]      [x y]    [x y]   [x y]
//	cmd=export; $cmd v=$b   [x]       [x]        [x]      [x y]   [x y]
//	e=; $e export v=$b      [x]       [x]        [x]      [x y]   [x y]
//	\export v=$b            [x]       [x]        [x y]    [x y]   [x y]
//	'export' v=$b           [x]       [x]        [x y]    [x y]   [x y]
//	command export v=$b     [x]       [x]        [x y]    [x y]   [x y]
//	command -p export v=$b  [x]       [x]        [x]      [x y]   [x y]
//
// zsh's `command` rows are under `setopt posixbuiltins`, without which
// `command export` is `command not found` there; the rest of its column is
// under `setopt shwordsplit`, without which this shell splits no unquoted
// parameter at all and every row would read `[x y]` whichever rule applied.
//
// The zsh column is exercised in dialect/zsh, where the two options can be
// set; here it is the vector's answers that are asserted for it.
func declarationPresets() []struct {
	dialecttest.Preset
	word   interp.DeclarationCommandWordReading
	prefix interp.Answer
} {
	return []struct {
		dialecttest.Preset
		word   interp.DeclarationCommandWordReading
		prefix interp.Answer
	}{
		{dialecttest.Preset{
			Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
			Diagnostics: bash.Diagnostics, Apply: bash.Apply,
		}, interp.DeclarationByUnquotedLiteralWord, interp.No},
		{dialecttest.Preset{
			Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics,
			Diagnostics: zsh.Diagnostics, Apply: zsh.Apply,
		}, interp.DeclarationByUnquotedLiteralWord, interp.No},
		{dialecttest.Preset{
			Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics,
			Diagnostics: ksh.Diagnostics, Apply: ksh.Apply,
		}, interp.DeclarationByWrittenWord, interp.Yes},
		{dialecttest.Preset{
			Name: "dash", Dialect: dash.Dialect, Semantics: dash.Semantics,
			Diagnostics: dash.Diagnostics, Apply: dash.Apply,
		}, interp.DeclarationByUtilityName, interp.Yes},
		{dialecttest.Preset{
			Name: "ash", Dialect: ash.Dialect, Semantics: ash.Semantics,
			Diagnostics: ash.Diagnostics, Apply: ash.Apply,
		}, interp.DeclarationByUtilityName, interp.Yes},
	}
}

func TestEachDialectAnswersTheDeclarationWordAxes(t *testing.T) {
	for _, p := range declarationPresets() {
		t.Run(p.Name, func(t *testing.T) {
			s := p.Semantics()
			if got := s.DeclarationCommandWord; got != p.word {
				t.Errorf("DeclarationCommandWord = %v, want %v", got, p.word)
			}
			if got := s.CommandPrefixKeepsADeclaration; got != p.prefix {
				t.Errorf("CommandPrefixKeepsADeclaration = %v, want %v", got, p.prefix)
			}
		})
	}
}

// The measured table, run. zsh is not here: without `setopt shwordsplit` it
// splits no unquoted parameter, so every row would read `[x y]` and say
// nothing about the rule — dialect/zsh sets the option and runs the same forms.
func TestADeclarationsOperandSplitsWhereThePanelSplits(t *testing.T) {
	rows := []struct {
		name, line              string
		bashZsh, ksh, dashAndSh string
	}{
		{"the literal word", `export v=$b`, "[x y]", "[x y]", "[x y]"},
		{"an expansion", `cmd=export; $cmd v=$b`, "[x]", "[x]", "[x y]"},
		{"an empty expansion in front", `e=; $e export v=$b`, "[x]", "[x]", "[x y]"},
		{"a backslash", `\export v=$b`, "[x]", "[x y]", "[x y]"},
		{"single quotes", `'export' v=$b`, "[x]", "[x y]", "[x y]"},
		{"double quotes", `"export" v=$b`, "[x]", "[x y]", "[x y]"},
		{"a quoted part", `expor't' v=$b`, "[x]", "[x y]", "[x y]"},
		{"the command prefix", `command export v=$b`, "[x]", "[x y]", "[x y]"},
		{"two command prefixes", `command command export v=$b`, "[x]", "[x y]", "[x y]"},
		{"a quoted command prefix", `\command export v=$b`, "[x]", "[x y]", "[x y]"},
		{"the -p flag on the prefix", `command -p export v=$b`, "[x]", "[x]", "[x y]"},
	}
	for _, p := range declarationPresets() {
		if p.Name == "zsh" {
			continue
		}
		for _, row := range rows {
			t.Run(p.Name+"/"+row.name, func(t *testing.T) {
				want := row.dashAndSh
				switch p.Name {
				case "bash":
					want = row.bashZsh
				case "ksh":
					want = row.ksh
				}
				out, _, err := p.Combined(t, dialecttest.Base{},
					"b='x y'\nf() {\n"+row.line+"\n"+`echo "[$v]"`+"\n}\nf")
				if err != nil {
					t.Fatalf("err %v: %s", err, out)
				}
				if got := strings.TrimSpace(out); got != want {
					t.Errorf("%s wrote %q, want %q", row.line, got, want)
				}
			})
		}
	}
}
