// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An assignment written in front of `alias` or `hash` is kept here, and in
// front of nothing else — **including** the POSIX special builtins, which is
// what says this is not that axis.
//
// Measured 2026-09-16 and again 2026-09-18 on zsh 5.9.2 with `-f`, script
// files under `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME and
// stdin `/dev/null`, and again over `-c`, inside a function, inside `( … )`
// and `$( … )`, and at both ends of a pipeline: the value is there wherever
// the builtin ran in this shell, and absent wherever it ran in a subshell —
// which is the subshell and not the builtin.
//
// `:` and `shift` are POSIX's own and `export` is the one every other column
// keeps, so the three of them together are the control:
// `AssignmentPrefixPersistsOnSpecialBuiltin` is No in this shell and these
// two are kept anyway (#3313).
func TestAnAssignmentInFrontOfAliasAndHashIsKept(t *testing.T) {
	const src = `V1=1 alias >/dev/null;    print "alias=[${V1-UNSET}]"
V2=1 hash >/dev/null;     print "hash=[${V2-UNSET}]"
V3=1 : ;                  print "colon=[${V3-UNSET}]"
V4=1 shift 0;             print "shift=[${V4-UNSET}]"
V5=1 export >/dev/null;   print "export=[${V5-UNSET}]"
V6=1 typeset >/dev/null;  print "typeset=[${V6-UNSET}]"
V7=1 unalias -a;          print "unalias=[${V7-UNSET}]"
V8=1 true;                print "true=[${V8-UNSET}]"
case "$(export -p)" in *V1=*) print exported;; *) print plain;; esac`
	const want = "alias=[1]\nhash=[1]\ncolon=[UNSET]\nshift=[UNSET]\nexport=[UNSET]\n" +
		"typeset=[UNSET]\nunalias=[UNSET]\ntrue=[UNSET]\nplain\n"
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if out != want || st != 0 {
		t.Errorf("out =\n%q at %d\nwant\n%q at 0", out, st, want)
	}
}

// And a subshell is still a subshell: the value is there inside the
// parentheses and gone outside them, which is what keeps the row above a
// statement about the builtin rather than about where it stood.
func TestAKeptPrefixDoesNotEscapeASubshell(t *testing.T) {
	const src = `f(){ B=1 alias >/dev/null; print "infunc=[${B-UNSET}]"; }
f; print "afterfunc=[${B-UNSET}]"
( C=1 alias >/dev/null; print "insub=[${C-UNSET}]" ); print "aftersub=[${C-UNSET}]"`
	const want = "infunc=[1]\nafterfunc=[1]\ninsub=[1]\naftersub=[UNSET]\n"
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if out != want || st != 0 {
		t.Errorf("out =\n%q at %d\nwant\n%q at 0", out, st, want)
	}
}
