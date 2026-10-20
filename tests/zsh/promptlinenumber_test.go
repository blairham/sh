// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `%i` is the line being read, counted from the start of whatever `%N` names.
// It is a row of this shell's prompt table because its *default* PS4 spells
// it — `+%N:%i> ` — so a prefix drawn from the parameter and the prefix this
// shell drew from a built-in string are the same text only if the code is
// there.
//
// Measured 2026-09-17 against zsh 5.9.2 over a script file, `env -i` with
// LC_ALL=C: `print -P '%i'` on the first line draws `1`, and on the first line
// of a function body draws `0`. Ours refused the escape outright — `the %i
// prompt escape is not implemented` — so an explicit `PS4='+%N:%i> '` already
// drew the wrong prefix before the default was one (#2928).
func TestTheLineNumberEscapeIsDrawn(t *testing.T) {
	out, st := answersRun(t, "print -P '[%i]'\nf() { print -P '[fn %i]'; }\nf\n")
	if st != 0 {
		t.Fatalf("status %d, want 0: %q", st, out)
	}
	if !strings.Contains(out, "[1]") {
		t.Errorf("got %q, want the first line to draw 1", out)
	}
	if !strings.Contains(out, "[fn 0]") {
		t.Errorf("got %q, want a function's first body line to draw 0", out)
	}
}

// And the prefix PS4's default spells is the prefix the trace writes, which is
// the whole reason the escape is here: the two used to be different things
// that happened to agree.
func TestThePS4DefaultDrawsTheTracePrefix(t *testing.T) {
	out, st := answersRun(t, "PS4='+%N:%i> '\nset -x\necho one\nset +x\n")
	if st != 0 {
		t.Fatalf("status %d, want 0: %q", st, out)
	}
	if !strings.Contains(out, ":3> echo one") {
		t.Errorf("got %q, want the prefix to name the unit and the line", out)
	}
}
