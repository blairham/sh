// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A session with no editor keeps a history too.
//
// There are two loops here — one that owns a terminal and draws a line, and
// one that reads a pipe or a file — and the list, the rules and the file
// belonged to the first. So `shell -i` with its input redirected recalled
// nothing, recorded nothing and wrote nothing, at every size and with
// HISTFILE set to anywhere at all, and said nothing about it.
//
// It is the shape a test suite drives a shell with — `shell -i` on a
// here-document whose HISTFILE the next command reads — and bash's own suite
// has a file that does exactly that seven times over, every one of which read
// an empty file here (#2298).
//
// This is the second-helper failure this tree keeps finding, so what the test
// holds is that there is one implementation and not two: the rules exercised
// below are the editor loop's rules, reached from the loop that has no editor.
func TestAnEditorLessSessionKeepsItsHistory(t *testing.T) {
	for _, c := range []struct {
		name, control, text string
		want                []string
	}{
		{
			// The whole construct and once, which is what the shared
			// recorder does: a `for` typed over four lines is one entry
			// rather than four, and the blank line between commands is
			// none at all. It reaches the file as its own lines, which is
			// what the editor loop writes for the same entry — the two
			// loops agreeing is the whole of what this case is for.
			name: "every line it read, a construct entire",
			text: "echo one\nfor i in 1 2\ndo\necho $i\ndone\n\necho two\n",
			want: []string{"earlier", "echo one", "for i in 1 2", "do", "echo $i", "done", "echo two"},
		},
		{
			// And the rules apply, which is the half that says these are the
			// editor loop's rules rather than a second copy of them.
			name:    "and the rules it was given",
			control: "ignoredups:ignorespace",
			text:    "echo one\necho one\n echo hidden\necho two\n",
			want:    []string{"earlier", "echo one", "echo two"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hist")
			if err := os.WriteFile(path, []byte("earlier\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			var ran, said strings.Builder
			vars := map[string]string{"PS1": "", "PS2": "", "HISTFILE": path}
			if c.control != "" {
				vars["HISTCONTROL"] = c.control
			}
			r := newTestRunner(vars)
			r.Stdout = &ran
			s := Shell{
				Runner:  r,
				In:      strings.NewReader(c.text),
				Out:     &ran,
				Err:     &said,
				History: HistoryStyle{Control: "HISTCONTROL"},
			}
			if _, err := s.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
			if strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Errorf("file holds %q, want %q", got, c.want)
			}
		})
	}
}
