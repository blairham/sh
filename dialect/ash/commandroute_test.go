// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// `command` reaches a **builtin** here, so its redirections are this shell's
// exactly as the bare builtin's are — the row zsh moves and this column does
// not.
//
// [interp.Semantics.CommandReachesABuiltin] is Yes here. Measured 2026-09-26
// on BusyBox v1.37.0 in the digest-pinned alpine image
// (alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b),
// over a script file under `env -i PATH=/usr/bin:/bin` with a scratch HOME
// (#4701).
//
// Every row keeps the write, the function and the program behind the word
// included — and that is **not** because the route is the same for them. It
// is [interp.Semantics.RedirectTargetExpandsInTheCommandsProcess], which is
// No here: this column keeps a target's write whoever runs the command. So it
// cannot see the route at all, and is recorded as unmoved rather than as
// agreeing.
func TestACommandPrefixedRedirectionIsThisShellsHere(t *testing.T) {
	t.Parallel()
	if ash.Semantics().CommandReachesABuiltin == interp.No {
		t.Fatal("CommandReachesABuiltin is No here, so these rows are not this column's")
	}
	for _, cmd := range []string{
		`:`, `command :`, `command read x`, `command echo RAN`,
		`command -p echo RAN`, `command -v :`, `command f`, `command /bin/echo RAN`,
	} {
		src := "f() { :; }\nunset u\n" + cmd + " > \"${u:=made}\" 2>/dev/null\nprintf 'u=%s' \"${u-unset}\"\n"
		if out, _ := runIn(t, src); !strings.HasSuffix(out, "u=made") {
			t.Errorf("%s: out = %q, want the write kept", cmd, out)
		}
	}
}

// And the here-document body beside it.
func TestACommandPrefixedHeredocBodyIsThisShellsHere(t *testing.T) {
	t.Parallel()
	for _, cmd := range []string{`:`, `command :`, `command echo`} {
		src := "unset u\n" + cmd + " <<END 2>/dev/null\n[${u:=made}]\nEND\nprintf 'u=%s' \"${u-unset}\"\n"
		if out, _ := runIn(t, src); !strings.HasSuffix(out, "u=made") {
			t.Errorf("%s: out = %q, want the write kept", cmd, out)
		}
	}
}

// The positive row, so the rows above are falsifiable: a `command`-prefixed
// target that expands is opened and its command runs.
func TestACommandPrefixedTargetThatExpandsStillOpensHere(t *testing.T) {
	t.Parallel()
	if out, st := runIn(t, "command echo RAN < /dev/null\necho \"after st=$?\"\n"); out != "RAN\nafter st=0\n" || st != 0 {
		t.Errorf("= %q (status %d), want RAN and after st=0", out, st)
	}
}
