// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// A `name=( … )` operand keeps the array reading through *quoting* of the
// command word here, and loses it only to an expansion.
//
// Measured 2026-09-16 on ksh93u+ 2012 from script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, stdin on /dev/null, each probe followed by
// `echo "[${a[1]}]"`:
//
//	typeset a=(x y)           [y]
//	'typeset' a=(x y)         [y]
//	\typeset a=(x y)          [y]
//	type"set" a=(x y)         [y]
//	cmd=typeset; $cmd a=(x y) `(' unexpected, status 3
//
// bash 5.3 refuses all four of the quoted spellings as a syntax error and
// zsh 5.9.2 reads a glob qualifier on the word `a=` in each, which is what
// makes this a dialect field rather than a core rule (#3351).
func TestADeclarationsArrayOperandSurvivesAQuotedCommandWord(t *testing.T) {
	d := ksh.Dialect()
	if got := d.DeclarationArrayFromTheCommandWord; got != syntax.DeclarationArrayFromAWrittenWord {
		t.Fatalf("DeclarationArrayFromTheCommandWord = %v, want the written-word reading", got)
	}
	for _, src := range []string{
		"typeset a=(x y); echo \"[${a[1]}]\"",
		"'typeset' a=(x y); echo \"[${a[1]}]\"",
		"\\typeset a=(x y); echo \"[${a[1]}]\"",
		"type\"set\" a=(x y); echo \"[${a[1]}]\"",
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, src)
		if err != nil || out != "[y]\n" {
			t.Errorf("%q: out = %q, err = %v, want %q", src, out, err, "[y]\n")
		}
	}
	// An expansion takes the reading away, and the `(` is then a parenthesis
	// behind a word with nowhere to go.
	if _, err := syntax.Parse("cmd=typeset; $cmd a=(x y)", d); err == nil {
		t.Error("`$cmd a=(x y)` parsed, want the `(` refused")
	}
}
