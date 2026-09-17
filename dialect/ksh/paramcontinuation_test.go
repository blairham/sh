// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// A line continuation inside `${ }` is removed here only once a name has
// begun. Measured 2026-09-16 on ksh93u+ 2012 from script files under `env -i`,
// stdin on /dev/null:
//
//	x=5; echo "[${x\⏎}]"          [5]
//	x=; echo "[${x:\⏎-d}]"        [d]
//	set --; echo "[${@:\⏎-d}]"    [d]
//	x=5; echo "[${\⏎x}]"          syntax error at line 1: `\' unexpected, status 3
//	x=abc; echo "[${#\⏎x}]"       the same
//	set -- a b; echo "[${1\⏎}]"   the same, and for ${@\⏎}, ${?\⏎}, ${#\⏎}
//
// bash 5.3, bash 3.2, zsh 5.9.2 and dash read every one of them. In an
// unquoted here-document body this shell reads them too: `cat <<E⏎[${\⏎x}]⏎E`
// prints the value (#3452).
func TestALineContinuationInAnExpansionWaitsForAName(t *testing.T) {
	if !ksh.Dialect().ParamContinuationNeedsAName {
		t.Fatal("ksh keeps a continuation in `${ }` until a name has begun")
	}
	for _, src := range []string{
		"x=5; echo \"[${\\\nx}]\"",
		"x=abc; echo \"[${#\\\nx}]\"",
		"set -- a b; echo \"[${1\\\n}]\"",
		"set -- a b; echo \"[${@\\\n}]\"",
		"echo \"[${#\\\n}]\"",
	} {
		if _, err := syntax.Parse(src, ksh.Dialect()); err == nil {
			t.Errorf("%q parsed, want a syntax error", src)
		}
	}
	for _, tc := range []struct{ src, want string }{
		{"x=5; echo \"[${x\\\n}]\"", "[5]\n"},
		{"xy=5; echo \"[${x\\\ny}]\"", "[5]\n"},
		{"x=; echo \"[${x:\\\n-d}]\"", "[d]\n"},
		{"set --; echo \"[${@:\\\n-d}]\"", "[d]\n"},
		{"x=abc; echo \"[${#x\\\n}]\"", "[3]\n"},
		{"x=a.b.c; read -r l <<E\n[${\\\nx}] [${#\\\nx}]\nE\necho \"$l\"", "[a.b.c] [5]\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil || out != tc.want {
			t.Errorf("%q: out = %q, err = %v, want %q", tc.src, out, err, strings.TrimSpace(tc.want))
		}
	}
}
