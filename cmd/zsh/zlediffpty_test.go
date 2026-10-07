// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
	"time"

	"github.com/blairham/sh/internal/terminfofixture"
)

// A change to the line is drawn cell by cell, the way zsh draws it: runs that
// already hold what they should are stepped over, and room is closed or
// opened with the terminal's own delete and insert sequences. Measured
// 2026-10-07 through a pseudo-terminal, zsh 5.9.2 `-f -i` with this test's
// prompt, `echo aa bbb cccc ddddd eeeeee`, M-b five times, then (#6341):
//
//	         xterm                                      tvi912 (ich1 \EQ, dch1 \EW, el \ET)
//	X        Xaa \e[2Cb \e[3Cc \e[4Cd \e[5Ce\e[24D      X\EQa\t\t\tee\EW\r\t X
//	Bksp     \ba\e[P\e[23C \e[25D                       \ba\EW\t\t\te \r\t
//	^T       \ba                                       \ba
//
// Here every one of them wrote the rest of the line again.
func TestAChangeIsDrawnCellByCell(t *testing.T) {
	const (
		dch1 = 21
		ich1 = 52
		dch  = 105
		ht   = 134
	)
	xterm := map[int]string{
		capClear: "\x1b[H\x1b[2J", capEl: "\x1b[K", capEd: "\x1b[J",
		capCud1: "\n", capCub1: "\b", capCuf1: "\x1b[C", capCuu1: "\x1b[A",
		capCud: "\x1b[%p1%dB", capCub: "\x1b[%p1%dD", capCuf: "\x1b[%p1%dC", capCuu: "\x1b[%p1%dA",
		dch1: "\x1b[P", dch: "\x1b[%p1%dP",
	}
	tvi := map[int]string{
		capClear: "\x1a", capEl: "\x1bT", capCud1: "\n", capCub1: "\b", capCuf1: "\f", capCuu1: "\v",
		dch1: "\x1bW", ich1: "\x1bQ", ht: "\t",
	}
	for _, c := range []struct {
		name       string
		strs       map[int]string
		nums       []int
		x, bs, swp string
	}{
		{
			"xterm-like", xterm, nil,
			"Xaa \x1b[2Cb \x1b[3Cc \x1b[4Cd \x1b[5Ce\x1b[24D", "\ba\x1b[P\x1b[23C \x1b[25D", "\ba ",
		},
		{
			"tvi912-like", tvi,
			[]int{terminfofixture.Absent, 8},
			"X\x1bQa\t\t\tee\x1bW\r\t X", "\ba\x1bW\t\t\te \r\t ", "\ba ",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			top := 0
			for i := range c.strs {
				top = max(top, i)
			}
			db := terminfofixture.Database(t, terminfofixture.Description{
				Name: "diffterm", Nums: c.nums, StrCount: top + 1, Strs: c.strs,
			})
			control, screen, _ := jobNoticeSessionOn(t, []string{"TERM=diffterm", "TERMINFO=" + db}, "", "zsh", "-i")
			t.Cleanup(func() { _, _ = control.WriteString("\x15") })
			press := func(key, want string) {
				t.Helper()
				at := len(screen.Text())
				if _, err := control.WriteString(key); err != nil {
					t.Fatal(err)
				}
				deadline := time.Now().Add(jobNoticeBudget)
				for want != "" && screen.Text()[at:] != want || want == "" && len(screen.Text()) == at {
					if time.Now().After(deadline) {
						t.Fatalf("after %q: drew %q, want %q", key, screen.Text()[at:], want)
					}
					time.Sleep(5 * time.Millisecond)
				}
			}
			for _, r := range "echo aa bbb cccc ddddd eeeeee" {
				press(string(r), "")
			}
			for range 5 {
				press("\x1bb", "")
			}
			press("X", c.x)
			press("\x7f", c.bs)
			press("\x14", c.swp)
		})
	}
}
