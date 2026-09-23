// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `compgen -A shopt` lists this dialect's option names. The action is the
// dialect's because the names are — interp has no business knowing any of
// them — and it was refused as `compgen: shopt: not implemented` until #4149.
//
// bash's own shopt1.sub loops over `$(compgen -A shopt)` twice, setting every
// name and then unsetting every one, which is why the row could not be worked
// without this.

// TestCompgenListsTheShoptNames: the whole set, sorted, and the same count
// bash has. Measured 2026-09-23 on bash 5.3.15: 59 names, `array_expand_once`
// first.
func TestCompgenListsTheShoptNames(t *testing.T) {
	out, st := answersRun(t, "compgen -A shopt")
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	names := strings.Fields(out)
	if len(names) != 59 {
		t.Errorf("%d names, want 59 — the same count bash 5.3.15 lists", len(names))
	}
	if len(names) > 0 && names[0] != "array_expand_once" {
		t.Errorf("first name %q, want %q — the listing is sorted", names[0], "array_expand_once")
	}
	// Every name the listing knows must be here, or the two are two tables.
	listed, _ := answersRun(t, "shopt")
	for _, line := range strings.Split(strings.TrimSpace(listed), "\n") {
		name := strings.Fields(line)[0]
		if !strings.Contains(out, name+"\n") {
			t.Errorf("`shopt` lists %q and `compgen -A shopt` does not", name)
		}
	}
}

// TestCompgenGeneratesShoptWhereBashDoes pins the **order**, which is the part
// a dialect cannot guess: measured on bash 5.3.15 with a prefix matching one
// of each kind, `shopt` is generated after `builtin` and after `function`, and
// before `variable` and `file`.
//
// Asserted through the builtins alone here because those are the names this
// harness has without arranging a filesystem: `ex` matches `exec`, `exit` and
// `export`, and five shopt names.
func TestCompgenGeneratesShoptWhereBashDoes(t *testing.T) {
	out, st := answersRun(t, "compgen -b -A shopt ex")
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	want := []string{"exec", "exit", "export", "execfail", "expand_aliases", "extdebug", "extglob", "extquote"}
	if got := strings.Fields(out); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v — the builtins first, then the shopt names", got, want)
	}
}

// TestCompgenShoptAnswersOneAtOne: a prefix nothing matches is a failure
// rather than an empty success, which is what a completer asks about.
func TestCompgenShoptAnswersOneAtOne(t *testing.T) {
	out, st := answersRun(t, "compgen -A shopt zzz")
	if st != 1 || out != "" {
		t.Errorf("out %q status %d, want nothing at 1", out, st)
	}
}
