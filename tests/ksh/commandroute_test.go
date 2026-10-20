// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// `command` reaches a **builtin** here, so its redirections are this shell's
// exactly as the bare builtin's are — the row zsh moves and this column does
// not.
//
// [interp.Semantics.CommandReachesABuiltin] is Yes here. Measured 2026-09-26
// on /bin/ksh — `Version AJM 93u+ 2012-08-01`, AT&T's own 2012 build and a
// different lineage from ksh93u+m, `not a Go executable` by `go version -m` —
// over a script file under `env -i PATH=/usr/bin:/bin` with a scratch HOME
// (#4701).
//
// **This is the column that says "where the redirection is applied" and "what
// its failure costs" are two questions.** `command : > "${u:=made}"` keeps the
// write here, because the builtin is reached and the word was expanded in this
// shell — and `command : > $(( 1/0 ))` still carries this shell on at 1 where
// a bare `:` ends it, because
// [interp.Semantics.RedirectTargetFailureIsTheRedirections] is keyed on a
// *special* builtin and `command` takes the specialness away without taking
// the process away. One fact settles both in zsh and two fields settle them
// here (#4689).
func TestACommandPrefixedRedirectionIsThisShellsHere(t *testing.T) {
	t.Parallel()
	if ksh.Semantics().CommandReachesABuiltin == interp.No {
		t.Fatal("CommandReachesABuiltin is No here, so these rows are not this column's")
	}
	dir := t.TempDir()
	for _, c := range []struct {
		cmd  string
		kept bool
	}{
		{`:`, true},
		{`command :`, true},
		{`command read x`, true},
		{`command echo RAN`, true},
		{`command -p echo RAN`, true},
		{`command -v :`, true},
		{`command f`, false},
		{`command /bin/echo RAN`, false},
	} {
		src := "f() { :; }\nunset u\n" + c.cmd + " > \"${u:=made}\" 2>/dev/null\nprintf 'u=%s' \"${u-unset}\"\n"
		out, _ := runKsh(t, dir, src)
		want := "u=unset"
		if c.kept {
			want = "u=made"
		}
		if !strings.HasSuffix(out, want) {
			t.Errorf("%s: out = %q, want it to end in %s", c.cmd, out, want)
		}
	}
}

// And the here-document body beside it.
func TestACommandPrefixedHeredocBodyIsThisShellsHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, cmd := range []string{`:`, `command :`, `command echo`} {
		src := "unset u\n" + cmd + " <<END 2>/dev/null\n[${u:=made}]\nEND\nprintf 'u=%s' \"${u-unset}\"\n"
		if out, _ := runKsh(t, dir, src); !strings.HasSuffix(out, "u=made") {
			t.Errorf("%s: out = %q, want the write kept", cmd, out)
		}
	}
}

// The positive row, so the rows above are falsifiable: a `command`-prefixed
// target that expands is opened and its command runs.
func TestACommandPrefixedTargetThatExpandsStillOpensHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runKsh(t, dir, "command echo RAN < /dev/null\necho \"after st=$?\"\n"); out != "RAN\nafter st=0\n" || st != 0 {
		t.Errorf("= %q (status %d), want RAN and after st=0", out, st)
	}
}

// The pair that separates the two questions, in the one column that can pose
// it. Same command, same target, same failure: the **write** says the word
// was expanded in this shell — `command :` keeps it, as the bare `:` does —
// and the **failure** says the word took the builtin's specialness away, so
// this shell carries on at 1 where the bare `:` ends it.
//
// Measured 2026-09-26 on /bin/ksh ksh93u+ 2012-08-01, the failing redirection
// on its own line with `echo "SAME st=$?"` beside it and `echo NEXT` after
// (#4689, #4701).
func TestTheWriteAndTheFailureAnswerSeparatelyHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, _ := runKsh(t, dir, "unset u\ncommand : > \"${u:=made}\"\nprintf 'u=%s' \"${u-unset}\"\n"); !strings.HasSuffix(out, "u=made") {
		t.Errorf("the write = %q, want it kept — the builtin is reached here", out)
	}
	out, st := runKsh(t, dir, "command : > $(( 1/0 )) ; echo \"SAME st=$?\"\necho NEXT\n")
	if !strings.Contains(out, "SAME st=1") || !strings.Contains(out, "NEXT") || st != 0 {
		t.Errorf("the failure = %q (status %d), want the line carrying on at 1", out, st)
	}
	bare, bareSt := runKsh(t, dir, ": > $(( 1/0 )) ; echo \"SAME st=$?\"\necho NEXT\n")
	if strings.Contains(bare, "NEXT") || bareSt != 1 {
		t.Errorf("the bare builtin = %q (status %d), want the shell ended at 1", bare, bareSt)
	}
}
