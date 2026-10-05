// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/smoke"
)

// The pre-redraw widget is called once for every key: for each of two keys
// read together, and for a key that draws nothing.
//
// #5945. Measured 2026-10-04 against zsh 5.9.2 under `zsh -i` on a
// pseudo-terminal with this startup file, `ab` in one write and then ^B, ^F
// and ^F, the last at the end of the line where it moves nothing:
//
//	pre B=[a] C=1 LW=[self-insert]
//	pre B=[ab] C=2 LW=[self-insert]
//	pre B=[ab] C=1 LW=[backward-char]
//	pre B=[ab] C=2 LW=[forward-char]
//	pre B=[ab] C=2 LW=[forward-char]
//
// This shell called it from its redraw, so the pair shared one call and the
// motion that moved nothing had none: the first and last rows were missing.
func TestPreRedrawRunsOncePerKey(t *testing.T) {
	control, screen, home := jobNoticeSessionRC(t, `pre() { print -r -- "pre B=[$BUFFER] C=$CURSOR LW=[$LASTWIDGET]" >> $HOME/out }
zle -N zle-line-pre-redraw pre
`, "zsh", "-i")
	t.Cleanup(func() { _, _ = control.WriteString("\x15") })
	log := func() string {
		data, _ := os.ReadFile(filepath.Join(home, "out"))
		return string(data)
	}
	want := []string{
		"pre B=[a] C=1 LW=[self-insert]",
		"pre B=[ab] C=2 LW=[self-insert]",
		"pre B=[ab] C=1 LW=[backward-char]",
		"pre B=[ab] C=2 LW=[forward-char]",
		"pre B=[ab] C=2 LW=[forward-char]",
	}
	// Each write is sent once the calls the one before owes are logged, so
	// the two keys of the first are the only ones read together.
	for i, keys := range []string{"ab", "\x02", "\x06", "\x06"} {
		if _, err := control.WriteString(keys); err != nil {
			t.Fatal(err)
		}
		lines := i + 2
		for deadline := time.Now().Add(jobNoticeBudget); strings.Count(log(), "\n") < lines; time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("after %q the log is\n%s\nwant %d lines; the screen:\n%s", keys, log(), lines,
					smoke.Readable(smoke.LastLines(screen.Text(), 3)))
			}
		}
	}
	// A quiet moment, so a call too many has time to arrive.
	time.Sleep(200 * time.Millisecond)
	if got := log(); got != strings.Join(want, "\n")+"\n" {
		t.Errorf("the log is\n%s\nwant\n%s", got, strings.Join(want, "\n"))
	}
}
