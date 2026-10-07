// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"io"
	"os"
	"strings"
	"testing"
)

// editorOnAPipe runs an editor-on-a-pipe session over typed, written to a real pipe
// in one write, and returns what its commands printed.
func editorOnAPipe(t *testing.T, typed string, vi bool) string {
	t.Helper()
	out, _ := editorSession(t, typed, vi, true)
	return out
}

// editorSession is editorOnAPipe, on a real pipe or on a reader that is not
// a descriptor at all, which the editor reads ahead of; it returns what the
// commands printed and what the editor drew.
func editorSession(t *testing.T, typed string, vi, pipe bool) (string, string) {
	t.Helper()
	if !pipe {
		var ran, drew strings.Builder
		in := strings.NewReader(typed)
		r := newTestRunner(map[string]string{"PS1": "P> ", "PS2": ""})
		r.Stdout = &ran
		r.Stdin = in
		s := Shell{
			Runner: r, In: in, Out: &ran, Err: &drew,
			EditorWithoutATerminal: true,
			ViEditing:              func() bool { return vi },
		}
		if _, err := s.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		return ran.String(), drew.String()
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pr.Close() })
	go func() {
		_, _ = io.WriteString(pw, typed)
		_ = pw.Close()
	}()
	var ran, drew strings.Builder
	r := newTestRunner(map[string]string{"PS1": "P> ", "PS2": ""})
	r.Stdout = &ran
	r.Stdin = pr
	s := Shell{
		Runner: r, In: pr, Out: &ran, Err: &drew,
		EditorWithoutATerminal: true,
		ViEditing:              func() bool { return vi },
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	return ran.String(), drew.String()
}

// The editor a dialect keeps on a pipe takes nothing past the line, so a
// `read` typed at the prompt finds the next one: bash 5.3.20 `-i` on a pipe
// prints `[DATA]`, measured 2026-10-07 (#6334).
func TestTheEditorOnAPipeLeavesTheNextLineForRead(t *testing.T) {
	if got := editorOnAPipe(t, "read x\nDATA\necho \"[$x]\"\n", false); got != "[DATA]\n" {
		t.Errorf("stdout %q, want [DATA]", got)
	}
}

// And it still reads a key sequence that arrived as one burst as one key,
// because it asks the descriptor whether the byte after an Escape is already
// there. bash 5.3.20 on a pipe, measured the same day: `echo abc` ESC `[D` `X`
// prints `abXc`, and with `set -o vi` `echo abcd` ESC `[D` `X` prints `abcXd`.
func TestTheEditorOnAPipeStillReadsABurstAsOneKey(t *testing.T) {
	if got := editorOnAPipe(t, "echo abc\x1b[DX\n", false); got != "abXc\n" {
		t.Errorf("emacs: stdout %q, want abXc", got)
	}
	if got := editorOnAPipe(t, "echo abcd\x1b[DX\n", true); got != "abcXd\n" {
		t.Errorf("vi: stdout %q, want abcXd", got)
	}
}

// And it draws what it drew when it read ahead: input already on the
// descriptor puts the drawing off exactly as input already in hand did, so an
// edit typed in one write is drawn once and not a key at a time.
func TestTheEditorOnAPipeDrawsAsItDidReadingAhead(t *testing.T) {
	for _, typed := range []string{
		"echo abc\b\b\bxyz\n",
		"echo abc\x01X\n",
		"echo abc\x1b[DX\n",
		"echo abc\x1bb\n",
	} {
		_, ahead := editorSession(t, typed, false, false)
		_, piped := editorSession(t, typed, false, true)
		if piped != ahead {
			t.Errorf("%q: drew %q on a pipe, %q reading ahead", typed, piped, ahead)
		}
	}
}
