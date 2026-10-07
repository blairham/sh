// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/terminfofixture"
)

// On a terminal with the magic-cookie glitch a standout sequence takes a
// column of its own. Measured 2026-10-07 through a pseudo-terminal, zsh 5.9.2
// under `TERM=tvi912` (`xmc#1`, standout `\Ej` and `\Ek`) with
// `PROMPT_EOL_MARK='%S>%s'`: the mark is padded as three columns wide, 76
// spaces on an 80-column terminal with no `xn`, where `>` alone is padded
// 78. Here the two sequences took none.
func TestAStandoutCookieTakesAColumn(t *testing.T) {
	const (
		xmc  = 4
		smso = 35
		rmso = 43
	)
	db := terminfofixture.Database(t, terminfofixture.Description{
		Name:     "cookieterm",
		Nums:     []int{terminfofixture.Absent, terminfofixture.Absent, terminfofixture.Absent, terminfofixture.Absent, 1},
		StrCount: rmso + 1,
		Strs: map[int]string{
			capClear: "\x1a", capEl: "\x1bT", capCud1: "\n", capCub1: "\b", capCuf1: "\f", capCuu1: "\v",
			smso: "\x1bj", rmso: "\x1bk",
		},
	})
	_, screen, _ := jobNoticeSessionOn(t, []string{"TERM=cookieterm", "TERMINFO=" + db}, "PROMPT_EOL_MARK='%S>%s'", "zsh", "-i")
	// The session is 100 columns, so 100 less the mark's 3 less 1.
	want := "\x1bj>\x1bk" + strings.Repeat(" ", 96) + "\r   \r"
	deadline := time.Now().Add(jobNoticeBudget)
	for !strings.Contains(screen.Text(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("the mark was not padded as three columns:\n%q", screen.Text())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// `%G` counts a column whatever surrounds it, and on a terminal zsh draws no
// attributes on it is drawn as `?`. Measured 2026-10-07, zsh 5.9.2 under
// `TERM=dumb` with `PS1=$'JNROW\nx%3Gjn> '`: the prompt is drawn
// `x???jn> `, and ^A on an eleven-character line is `\r` and eight spaces.
// Here `%G` drew nothing and counted nothing.
func TestACountedColumnIsCounted(t *testing.T) {
	control, screen, _ := jobNoticeSessionOn(t, []string{"TERM=dumb"}, "PS1=$'JNROW\\nx%3Gjn> '", "zsh", "-i")
	t.Cleanup(func() { _, _ = control.WriteString("\x15") })
	if !strings.Contains(screen.Text(), "JNROW\r\nx???jn> ") {
		t.Fatalf("the prompt was not drawn with its counted columns:\n%q", screen.Text())
	}
	for _, r := range "abcdefghijk" {
		at := len(screen.Text())
		if _, err := control.WriteString(string(r)); err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(jobNoticeBudget); len(screen.Text()) == at; time.Sleep(5 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%q drew nothing", r)
			}
		}
	}
	at := len(screen.Text())
	if _, err := control.WriteString("\x01"); err != nil {
		t.Fatal(err)
	}
	want := "\r" + strings.Repeat(" ", 8)
	for deadline := time.Now().Add(jobNoticeBudget); screen.Text()[at:] != want; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("^A drew %q, want %q", screen.Text()[at:], want)
		}
	}
}

// Hidden inside `%{ %}`, `%G` still counts its column, and the prompt is
// written again to move over it only where that is fewer bytes, weighing two
// for each hidden region and one for each counted column. Measured
// 2026-10-07, zsh 5.9.2 under vt52 with an eleven-character line: under
// `PS1='%{ab%G%}jn> '` ^A is `\r` and the prompt written again (6 bytes,
// weighed at 9, against 10 for five steps), under `PS1='%{abc%G%}jn> '` it is
// `\r` and five `\eC` (weighed at 10, a tie, which is the steps), and ^E is
// two tabs from column 5 to 16 under both.
func TestAHiddenCountedColumnIsCounted(t *testing.T) {
	for _, c := range []struct{ ps1, home string }{
		{"%{ab%G%}jn> ", "\rabjn> "},
		{"%{abc%G%}jn> ", "\r" + strings.Repeat("\x1bC", 5)},
	} {
		t.Run(c.ps1, func(t *testing.T) { hiddenCountedColumn(t, c.ps1, c.home) })
	}
}

func hiddenCountedColumn(t *testing.T, ps1, home string) {
	control, screen, _ := jobNoticeSessionOn(t, vt52Terminal(t), "PS1='"+ps1+"'", "zsh", "-i")
	t.Cleanup(func() { _, _ = control.WriteString("\x15") })
	for _, r := range "abcdefghijk" {
		at := len(screen.Text())
		if _, err := control.WriteString(string(r)); err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(jobNoticeBudget); len(screen.Text()) == at; time.Sleep(5 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%q drew nothing", r)
			}
		}
	}
	for _, k := range []struct{ send, want string }{
		{"\x01", home},
		{"\x05", "\t\t"},
	} {
		at := len(screen.Text())
		if _, err := control.WriteString(k.send); err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(jobNoticeBudget); screen.Text()[at:] != k.want; time.Sleep(5 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("after %q: drew %q, want %q", k.send, screen.Text()[at:], k.want)
			}
		}
	}
}
