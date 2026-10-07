// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/terminfofixture"
)

// hpTerminal is hp2621's attributes and moves, written into a database the
// test owns: standout and its reset are `\E&dD` and `\E&d@`, which are not
// CSI sequences, and there is no counted move right but there is `hpa`.
func hpTerminal(t *testing.T) []string {
	t.Helper()
	const (
		hpa  = 8
		smso = 35
		sgr0 = 39
		rmso = 43
	)
	db := terminfofixture.Database(t, terminfofixture.Description{
		Name:     "hpterm",
		StrCount: rmso + 1,
		Strs: map[int]string{
			capClear: "\x1bH\x1bJ", capEl: "\x1bK", capEd: "\x1bJ",
			capCud1: "\n", capCub1: "\b", capCuf1: "\x1bC", capCuu1: "\x1bA",
			hpa: "\x1b&a%p1%dC", smso: "\x1b&dD", sgr0: "\x1b&d@", rmso: "\x1b&d@",
		},
	})
	return []string{"TERM=hpterm", "TERMINFO=" + db}
}

// A prompt's attribute sequences take no columns, whatever they look like.
// Measured 2026-10-07 through a pseudo-terminal, zsh 5.9.2 `-f -i` under
// `TERM=hp2621` with `PS1=$'JNROW\n%S>%s jn> '`, typing `abc`: the mark
// `\E&dD%\E&d@\E&d@` is padded to the column before the edge, ^A is three
// backspaces and ^E `\E&a9C` (#6342). Here the sequences were counted as
// columns: the mark was padded six short, and the line was placed two
// columns further along than it is.
func TestAPromptsAttributeSequencesTakeNoColumns(t *testing.T) {
	control, screen, _ := jobNoticeSessionOn(t, hpTerminal(t), "PS1=$'JNROW\\n%S>%s jn> '", "zsh", "-i")
	t.Cleanup(func() { _, _ = control.WriteString("\x15") })
	mark := "\x1b&dD%\x1b&d@\x1b&d@" + strings.Repeat(" ", 98) + "\r \r"
	if !strings.Contains(screen.Text(), mark) {
		t.Fatalf("the mark was not padded to the column before the edge:\n%q", screen.Text())
	}
	for _, k := range []struct{ send, want string }{
		{"a", "a"},
		{"b", "\bab"},
		{"c", "c"},
		{"\x01", "\b\b\b"},
		{"\x05", "\x1b&a9C"},
	} {
		at := len(screen.Text())
		if _, err := control.WriteString(k.send); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(jobNoticeBudget)
		for screen.Text()[at:] != k.want {
			if time.Now().After(deadline) {
				t.Fatalf("after %q: drew %q, want %q", k.send, screen.Text()[at:], k.want)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}
