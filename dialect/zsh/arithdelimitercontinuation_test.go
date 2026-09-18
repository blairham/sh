// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// A line continuation between the two `)` of an arithmetic expansion parts
// them here, so the construct is a command substitution holding a subshell —
// and the same pair between the two `(` of the *opener* does not.
//
// Measured 2026-09-16 on zsh 5.9.2 from script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, stdin on /dev/null:
//
//	echo "[$(\⏎( 1 + 2 ))]"   [3]                      — arithmetic
//	echo "[$(( 1 + 2 )\⏎)]"   command not found: 1, [] — a substitution
//
// This is the row that makes the two ends two fields: bash 5.3, bash 3.2 and
// dash join both, ksh93u+ parts both, and this shell does one of each, so a
// single yes/no could not be given a value here.
//
// A line continuation behind the `((` of an arithmetic *command* is a third
// question and is not this one: `((\⏎x = 5 ))` is 5 here as it is in bash.
func TestAContinuationPartsTheArithmeticCloserAndNotTheOpener(t *testing.T) {
	d := zsh.Dialect()
	if d.ContinuationPartsTheArithmeticOpener {
		t.Error("zsh reads over a continuation between the two `(` of an opener")
	}
	if !d.ContinuationPartsTheArithmeticCloser {
		t.Error("zsh parts the two `)` of a closer across a continuation")
	}
	if d.ContinuationEndsTheArithmeticCommandOpener {
		t.Error("zsh opens an ordinary arithmetic command behind `((`")
	}

	if out, _, err := preset.Combined(t, dialecttest.Base{}, "echo \"[$(\\\n( 1 + 2 ))]\""); err != nil || out != "[3]\n" {
		t.Errorf("the opener: out = %q, err = %v, want %q", out, err, "[3]\n")
	}
	out, _, _ := preset.Combined(t, dialecttest.Base{}, "echo \"[$(( 1 + 2 )\\\n)]\"")
	if !strings.Contains(out, "[]") || !strings.Contains(out, "command not found: 1") {
		t.Errorf("the closer: out = %q, want an empty expansion and a command named `1`", out)
	}

	if out, _, err := preset.Combined(t, dialecttest.Base{}, "((\\\nx = 5 )); echo \"[$x]\""); err != nil || out != "[5]\n" {
		t.Errorf("the arithmetic command: out = %q, err = %v, want %q", out, err, "[5]\n")
	}
}
