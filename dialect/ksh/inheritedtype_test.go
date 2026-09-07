// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// This shell discards a value it was started with when a declaration gives
// the name a type, and takes the export attribute with it — the third answer
// to whether an attribute reaches back, and the one that is neither of the
// other two. Measured 2026-09-07 on 93u+ 2012-08-01 (#1121).
func TestADeclaredTypeDiscardsAnInheritedValue(t *testing.T) {
	if got := ksh.Semantics().InheritedValueSurvivesADeclaredType; got != interp.No {
		t.Errorf("InheritedValueSurvivesADeclaredType = %v, want No", got)
	}
	out, st := runKshWithEnv(t, []string{"INHERITED=bar"}, `echo "before=[$INHERITED]"
typeset -i INHERITED
echo "after=[${INHERITED+SET}][$INHERITED]"`)
	want := "before=[bar]\nafter=[][]\n"
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q — the name started over, not emptied", out, st, want)
	}
}

// `-x` or `-r` on the *same* command keeps the value and re-reads it, and the
// same two attributes split across two commands do not. It is this command's
// letters and not the name's standing ones.
func TestTheExportLetterKeepsAnInheritedValueOnItsOwnCommand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"exported on the same command", `typeset -ix G`, "[SET][7]\n"},
		{"frozen on the same command", `typeset -ir G`, "[SET][7]\n"},
		{"exported by the command before", "typeset -x G\ntypeset -i G", "[][]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKshWithEnv(t, []string{"G=3+4"},
				tc.src+"\necho \"[${G+SET}][$G]\"")
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}

// A declaration with nothing to say about a value leaves an inherited name
// alone here as everywhere else, so the discard is about the type letter and
// not about declaring.
func TestADeclarationWithNoTypeLeavesAnInheritedValue(t *testing.T) {
	out, st := runKshWithEnv(t, []string{"P=pv"}, `typeset P
echo "bare=[${P+SET}][$P]"
typeset -x P
echo "exported=[${P+SET}][$P]"`)
	want := "bare=[SET][pv]\nexported=[SET][pv]\n"
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q", out, st, want)
	}
}

// runKshWithEnv is runKsh with an environment the shell was started with,
// which is the whole of what these cases are about.
func runKshWithEnv(t *testing.T, env []string, src string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": dir}, Env: env,
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}
