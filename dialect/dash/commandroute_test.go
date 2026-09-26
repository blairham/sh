// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// `command` reaches a **builtin** here, so its redirections are this shell's
// exactly as the bare builtin's are — the row zsh moves and this column does
// not.
//
// [interp.Semantics.CommandReachesABuiltin] is Yes here. Measured 2026-09-26
// on /bin/dash 0.5.12 (`not a Go executable` by `go version -m`), over a
// script file under `env -i PATH=/usr/bin:/bin` with a scratch HOME (#4701).
//
// Every row keeps the write, including the function and the program behind
// the word — and that is **not** because the route is the same for them. It
// is [interp.Semantics.RedirectTargetExpandsInTheCommandsProcess], which is
// No here: this column keeps a target's write whoever runs the command. So
// this column cannot see the route at all, and it is recorded as unmoved
// rather than as agreeing.
func TestACommandPrefixedRedirectionIsThisShellsHere(t *testing.T) {
	t.Parallel()
	if dash.Semantics().CommandReachesABuiltin == interp.No {
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
		{`command f`, true},
		{`command /bin/echo RAN`, true},
	} {
		src := "f() { :; }\nunset u\n" + c.cmd + " > \"${u:=made}\" 2>/dev/null\nprintf 'u=%s' \"${u-unset}\"\n"
		out, _ := runDash(t, dir, src)
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
		if out, _ := runDash(t, dir, src); !strings.HasSuffix(out, "u=made") {
			t.Errorf("%s: out = %q, want the write kept", cmd, out)
		}
	}
}

// The positive row, so the rows above are falsifiable: a `command`-prefixed
// target that expands is opened and its command runs.
func TestACommandPrefixedTargetThatExpandsStillOpensHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, st := runDash(t, dir, "command echo RAN < /dev/null\necho \"after st=$?\"\n"); out != "RAN\nafter st=0\n" || st != 0 {
		t.Errorf("= %q (status %d), want RAN and after st=0", out, st)
	}
}
