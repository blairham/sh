// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestAReferenceAimedAtAnElementOfItsOwnNameIsAimedAtItself — a reference whose
// target is an element of the name being declared is a self reference, and it
// takes the same top-level-against-function split a bare self reference does:
// refused at the top level, warned about twice inside a function.
//
// Measured 2026-09-23 against bash 5.3.15 in the pinned debian:sid-slim and
// bash 5.3.20 on macOS, which agree. Every row was 0 and silent here (#4178).
func TestAReferenceAimedAtAnElementOfItsOwnNameIsAimedAtItself(t *testing.T) {
	const self = "nameref variable self references not allowed"
	// At the top level it is refused, and the name is never brought into
	// being — the listing after it says so.
	for _, src := range []string{
		"declare -n g='g[0]'\ndeclare -p g\n",
		"declare -n m='m[2]'\ndeclare -p m\n",
	} {
		out, errs := runBashSplitFatal(t, src)
		if !strings.Contains(errs, self) {
			t.Errorf("%q: stderr = %q, want the self-reference sentence", src, errs)
		}
		if !strings.Contains(errs, "not found") {
			t.Errorf("%q: stderr = %q, want the name never declared", src, errs)
		}
		if out != "" {
			t.Errorf("%q: got %q on stdout, want nothing", src, out)
		}
	}
	// Inside a function there is somewhere to refer out to, so it is the
	// circular warning twice and the reference stands — the same split a bare
	// self reference already takes.
	out, errs := runBashSplitFatal(t, "f() { local -n j='j[0]'; declare -p j; }\nf\n")
	if n := strings.Count(errs, "circular name reference"); n != 2 {
		t.Errorf("stderr = %q, want the warning twice, got %d", errs, n)
	}
	if want := "declare -n j=\"j[0]\"\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	// The control, and it is what says this is about the name's *own*
	// element: an element of another name is an ordinary target.
	out, errs = runBashSplitFatal(t, "declare -n h='k[0]'\ndeclare -p h\n")
	if want := "declare -n h=\"k[0]\"\n"; out != want {
		t.Errorf("control: got %q, want %q", out, want)
	}
	if errs != "" {
		t.Errorf("control: stderr = %q, want nothing", errs)
	}
}
