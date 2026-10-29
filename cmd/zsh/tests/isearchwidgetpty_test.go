// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tests_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/cellgrid"
	"github.com/blairham/sh/internal/smoke"
)

// `C-r` under a wrapper that calls the search by name searches.
//
// zsh-autosuggestions wraps every widget, the incremental search included, and
// its wrapper runs the original as `zle .history-incremental-search-backward`.
// Until #5895 this editor refused that call, so for everyone running the
// plugin `C-r` printed
//
//	…:zle: history-incremental-search-backward: calling a built-in widget is not implemented yet
//
// and searched nothing. The wrapper here is the plugin's shape cut down to the
// call and one line after it, which writes the call's status and the line to a
// file so that what the wrapper saw is read off the disk rather than the
// screen.
//
// Measured 2026-10-04 through a pseudo-terminal against /opt/homebrew/bin/zsh
// (zsh 5.9.2) with this wrapper on `C-r`:
//
//	C-r bra Return   the search row drawn under the line, the match in the
//	                 line; the wrapper sees status 0 and the match in
//	                 $BUFFER, and then the line runs
//	abc C-r bra C-g  the wrapper sees status 3 and `abc`, and `abc` is left
//	                 on the line to go on editing
//	abc C-r bra C-c  the same as C-g, status 3 included
//
// Asserted on a terminal model, because the search's row is the part a person
// sees and the part the refusal never drew.
func TestTheSearchRunsFromAWrapperWidget(t *testing.T) {
	const rc = `w() { zle .history-incremental-search-backward; print -r -- "rc=$? B=$BUFFER" > $HOME/after; }
zle -N history-incremental-search-backward w
`
	control, screen, home := jobNoticeSessionRC(t, rc, "zsh", "-i")
	file := func(name string) (string, bool) {
		b, err := os.ReadFile(filepath.Join(home, name))
		return string(b), err == nil
	}
	await := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(jobNoticeBudget); !ok(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("waited for %s:\n%s\nraw: %q", what, grid(screen), smoke.LastLines(screen.Text(), 6))
			}
		}
	}
	send := func(keys string) {
		t.Helper()
		if _, err := control.WriteString(keys); err != nil {
			t.Fatalf("typing %q: %v", keys, err)
		}
	}
	// wrapperSaw is the line the wrapper wrote, once the whole of it is there.
	// The file being there is not enough: the redirection creates it before
	// print writes to it, so a look between the two finds a file that exists
	// and holds nothing (#6997). The newline is the last thing print writes.
	wrapperSaw := func(after string) string {
		t.Helper()
		for deadline := time.Now().Add(jobNoticeBudget); ; time.Sleep(10 * time.Millisecond) {
			got, there := file("after")
			if there && strings.HasSuffix(got, "\n") {
				return got
			}
			if time.Now().After(deadline) {
				t.Fatalf("after %s the wrapper wrote no whole line (file there %v, holding %q):\n%s",
					after, there, got, grid(screen))
			}
		}
	}

	// The history the search is asked about: the line it should find, then a
	// newer one it should pass over. The first leaves a file behind, which is
	// removed so that the file coming back is the search's line running again.
	const found = ": > ranA"
	jobNoticeType(t, control, screen, found)
	jobNoticeType(t, control, screen, ": > other")
	if err := os.Remove(filepath.Join(home, "ranA")); err != nil {
		t.Fatalf("the seeded line did not run: %v", err)
	}

	// The search, drawn: zsh's shape keeps the prompt and the line, and puts
	// the search on the row under them.
	send("\x12ranA")
	searched := -1
	await("the search row under the line holding the match", func() bool {
		g := grid(screen)
		searched = promptRow(g)
		return searched >= 0 && g.Text(searched) == jobNoticeMark+found && g.Text(searched+1) == "bck-i-search: ranA_"
	})
	if _, ok := file("after"); ok {
		t.Fatal("the wrapper's code after the call ran before the search ended")
	}

	// Return ends the search; the wrapper's code after the call runs with the
	// match in the line, and then the line itself runs.
	send("\r")
	await("the found line to run", func() bool { _, ok := file("ranA"); return ok })
	if got, want := wrapperSaw("Return"), "rc=0 B="+found+"\n"; got != want {
		t.Errorf("after the search the wrapper saw %q, want %q", got, want)
	}
	// And the next prompt, empty and below the search, read off the grid. Not
	// screen.Await on the mark: the search redrew the prompt on every key, so
	// one is already waiting past Await's cursor and it returns at once. Keys
	// sent then can reach the terminal before the editor has taken it back,
	// and the terminal's own line discipline eats the ^R as a reprint.
	await("an empty prompt under the line that ran", func() bool {
		g := grid(screen)
		p := promptRow(g)
		return p > searched && g.Text(p) == strings.TrimSpace(jobNoticeMark)
	})

	// Abandoned, by either key: the line goes back to what was typed and
	// stays on the line, and the call answers 3.
	for _, abandon := range []struct{ name, key string }{{"C-g", "\a"}, {"C-c", "\x03"}} {
		_ = os.Remove(filepath.Join(home, "after"))
		send("abc\x12ranA")
		await("the search for "+abandon.name, func() bool {
			g := grid(screen)
			p := promptRow(g)
			return p >= 0 && g.Text(p) == jobNoticeMark+found && g.Text(p+1) == "bck-i-search: ranA_"
		})
		send(abandon.key)
		if got, want := wrapperSaw(abandon.name), "rc=3 B=abc\n"; got != want {
			t.Errorf("after %s the wrapper saw %q, want %q", abandon.name, got, want)
		}
		await("the typed line back, with no search row, after "+abandon.name, func() bool {
			g := grid(screen)
			p := promptRow(g)
			row, col := g.Cursor()
			return p >= 0 && g.Text(p) == jobNoticeMark+"abc" && strings.TrimSpace(g.Text(p+1)) == "" &&
				row == p && col == len(jobNoticeMark+"abc")
		})
		// Cleared for the next round and for the session's ending.
		send("\x15")
		await("the line cleared", func() bool {
			g := grid(screen)
			p := promptRow(g)
			// Trimmed, because a row's text ends at its last character and
			// the mark ends in a blank.
			return p >= 0 && g.Text(p) == strings.TrimSpace(jobNoticeMark)
		})
	}
}

// grid is the screen so far as a terminal would show it.
func grid(screen *smoke.Screen) *cellgrid.Grid {
	g := cellgrid.New(100)
	_, _ = g.Write([]byte(screen.Text()))
	return g
}

// promptRow is the last row the prompt's own row begins, or -1.
func promptRow(g *cellgrid.Grid) int {
	row := -1
	for r := range g.Rows() {
		// The mark alone is the prompt with nothing typed, and its trailing
		// blank is not in the row's text.
		if text := g.Text(r); strings.HasPrefix(text, jobNoticeMark) || text == strings.TrimSpace(jobNoticeMark) {
			row = r
		}
	}
	return row
}
