// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// TestEveryKillComplaintIsTheAppletsOwn is #3165.
//
// Measured 2026-09-17 against BusyBox v1.37.0 in the pinned Alpine image
// (alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b),
// under a matching `argv[0]`, one probe per line:
//
//	kill                 ash: you need to specify whom to kill        1
//	kill notapid         ash: invalid number 'notapid'                1
//	kill -l nope         ash: unknown signal 'nope'                   1
//	kill -s              ash: bad signal name 's'                     1
//	kill -n 99 <pid>     ash: bad signal name 'n'                     1
//	kill -0 999999       ash: can't kill pid 999999: No such process  1
//	kill -9x <pid>       ash: bad signal name '9x'                    1
//	kill -Q <pid>        ash: bad signal name 'Q'                     1
//
// **Every `kill` failure in this applet is 1.** There is no route to 2 in it
// at all, and this shell answered 2 for the bare `kill`. Four wordings were
// wrong beside that, and each is its own question rather than a typo:
//
//   - the usage sentence is a sentence and not a usage line;
//   - the not-a-pid sentence is `invalid number` where the ash family's other
//     builtins write `Illegal number` — `wait abc` still does;
//   - `kill -l` has its own refusal, distinct from the signal spec's, which
//     is what Diagnostics.KillListBadNumber is for and it was unset here;
//   - `-n` is not an option at all, so `kill -n 99` is the *letter* refused
//     and the 99 is a target.
//
// And the messages carry the shell's name and nothing else — no file, no line
// — where this shell's own `not found` carries `<file>: line N:`. That is
// Diagnostics.BuiltinNamesTheShellAlone rather than the unprefixed flags,
// which write no name at all.
//
// A failed send was also followed by a **blank line**: the wording ended in a
// newline and killFailed adds another. dash's entry has the same trailing
// newline and dash really does print the blank line, measured — so the fix is
// this string and not the caller.
func TestEveryKillComplaintIsTheAppletsOwn(t *testing.T) {
	s := ash.Semantics()
	if got := s.KillReadsTheNumberOption; got != interp.No {
		t.Errorf("KillReadsTheNumberOption is %v, want No", got)
	}
	if got := s.KillOptionWithNoArgumentIsASignalName; got != interp.Yes {
		t.Errorf("KillOptionWithNoArgumentIsASignalName is %v, want Yes", got)
	}
	for _, c := range []struct{ src, want string }{
		{`kill`, "you need to specify whom to kill"},
		{`kill notapid`, "invalid number 'notapid'"},
		{`kill -l nope`, "unknown signal 'nope'"},
		{`kill -s`, "bad signal name 's'"},
		{`kill -n 99 999999`, "bad signal name 'n'"},
		{`kill -9x 999999`, "bad signal name '9x'"},
		{`kill -Q 999999`, "bad signal name 'Q'"},
		{`kill -0 999999`, "can't kill pid 999999: No such process"},
	} {
		out, _ := run(t, c.src+` 2>&1 >/dev/null; echo "st=$?"`)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s said %q, want %q in it", c.src, out, c.want)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%s said %q, want status 1 — this applet has no route to 2", c.src, out)
		}
		// The shell's name and nothing else: no file and no line.
		if strings.Contains(out, "line ") {
			t.Errorf("%s said %q, want no location in it", c.src, out)
		}
	}
	// And no blank line after a failed send.
	if out, _ := run(t, `kill -0 999999 2>&1 >/dev/null | wc -l`); strings.TrimSpace(out) != "1" {
		t.Errorf("a failed send wrote %q lines, want one", strings.TrimSpace(out))
	}
	// The other builtins keep the number reader they had: this sentence is
	// `kill`'s own and not the family's.
	if out, _ := run(t, `wait abc 2>&1 >/dev/null`); !strings.Contains(out, "Illegal number") {
		t.Errorf("wait said %q, want the family's own number sentence", out)
	}
}
