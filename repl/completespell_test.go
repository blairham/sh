// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// Correcting a misspelled directory while completing: bash's `dirspell`, which
// is visible only beside its `direxpand`.
//
// Every want below is what bash 5.3.15 did, driven through a pseudo-terminal
// on 2026-09-08 with the options set from an rc file so readline held them at
// initialization. The panel is in shellCompleter.corrected.

// spellFixture is the tree those measurements were taken in.
func spellFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"documents", "alpha", "alpha/beta", "zzz1", "zzz2", "realdir"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{
		"documents/target-file.txt", "alpha/beta/gamma.txt",
		"zzz1/one.txt", "zzz2/two.txt", "realdir/inside.txt",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "realdir"), filepath.Join(dir, "linkdir")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// spellCompleter is the seam the real session uses, not a hand-built struct:
// the whole question is whether a `shopt` name reaches the completer, and a
// test that filled the fields in itself would answer a different one.
func spellCompleter(dir string, dirspell, direxpand bool) (runnerCompleter, *interp.Runner) {
	r := &interp.Runner{Dir: dir}
	r.SetCorrectsCompletionSpelling(dirspell)
	r.SetExpandsCompletedDirectory(direxpand)
	return runnerCompleter{r: r}, r
}

func completeWord(c Completer, word string) []string {
	return c.Complete(Completion{Line: word, Point: len(word), Word: word})
}

// The pair, which is the finding both #1445 and #1562 turn on: three of the
// four configurations do nothing at all, and a row written against `dirspell`
// alone would have graded a working shell as broken.
func TestAMisspelledDirectoryNeedsBothNames(t *testing.T) {
	dir := spellFixture(t)
	for _, tc := range []struct {
		name                string
		dirspell, direxpand bool
		want                string
	}{
		{"neither", false, false, ""},
		{"dirspell alone", true, false, ""},
		{"direxpand alone", false, true, ""},
		{"both", true, true, "documents/target-file.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := spellCompleter(dir, tc.dirspell, tc.direxpand)
			got := completeWord(c, "documnets/tar")
			switch {
			case tc.want == "":
				if len(got) != 0 {
					t.Errorf("offered %q, want nothing", got)
				}
			case len(got) != 1 || got[0] != filepath.Join(dir, tc.want):
				t.Errorf("offered %q, want %q", got, filepath.Join(dir, tc.want))
			}
		})
	}
}

// What the pair does to the rest of the panel. An absolute path is what goes
// into the line, cleaned, which is bash's answer and not `cdspell`'s.
func TestWhatTheCorrectedCompletionPutsInTheLine(t *testing.T) {
	dir := spellFixture(t)
	c, _ := spellCompleter(dir, true, true)
	for _, tc := range []struct {
		name, typed, want string
	}{
		{"a directory portion one edit out", "documnets/tar", "documents/target-file.txt"},
		{"nothing typed after the slash", "documnets/", "documents/target-file.txt"},
		{"a component in the middle", "alpha/bteta/gam", "alpha/beta/gamma.txt"},
		{"two components at once", "alpah/bteta/gam", "alpha/beta/gamma.txt"},
		{"a `..` that is not the first component", "documents/../documnets/tar", "documents/target-file.txt"},
		{"through a symlink", "linkdir/../documnets/tar", "documents/target-file.txt"},
		// The correction is of the *directory* portion only: with no
		// trailing slash there is no directory to correct, and bash rings the
		// bell in every configuration.
		{"the last component alone", "documnets", ""},
		// Nothing within one edit, so there is nothing to correct to.
		{"a directory that is simply not there", "nowhere/xy", ""},
		// Measured refusals. bash corrects `documents/../documnets/` and
		// refuses `./documnets/`, though the two name the same directory —
		// so the test is on the text as typed and cannot be on the path.
		{"a leading dot component", "./documnets/tar", ""},
		{"a leading dot-dot component", "../documnets/tar", ""},
		{"a dot component further along", "alpha/./bteta/gam", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := completeWord(c, tc.typed)
			if tc.want == "" {
				if len(got) != 0 {
					t.Errorf("%q offered %q, want nothing", tc.typed, got)
				}
				return
			}
			if len(got) != 1 || got[0] != filepath.Join(dir, tc.want) {
				t.Errorf("%q offered %q, want %q", tc.typed, got, filepath.Join(dir, tc.want))
			}
		})
	}
}

// A correction that is not wanted is not made, and a word that needs no
// correction is answered exactly as it was before either name existed: the
// text as typed, with only the last component added.
func TestTheOptionsLeaveAWellSpelledWordAlone(t *testing.T) {
	dir := spellFixture(t)
	for _, on := range []bool{false, true} {
		c, _ := spellCompleter(dir, on, on)
		if got := completeWord(c, "documents/tar"); len(got) != 1 || got[0] != "documents/target-file.txt" {
			t.Errorf("with the names %v, `documents/tar` offered %q, want the typed prefix kept", on, got)
		}
	}
}

// The names are read at the keystroke rather than settled when the session
// started, because both are lines a person types at the prompt.
func TestTheCorrectionFollowsTheRunner(t *testing.T) {
	dir := spellFixture(t)
	c, r := spellCompleter(dir, false, false)
	if got := completeWord(c, "documnets/tar"); got != nil {
		t.Fatalf("offered %q before either name was set", got)
	}
	r.SetCorrectsCompletionSpelling(true)
	r.SetExpandsCompletedDirectory(true)
	if got := completeWord(c, "documnets/tar"); len(got) != 1 {
		t.Fatalf("offered %q after both names were set", got)
	}
	r.SetCorrectsCompletionSpelling(false)
	if got := completeWord(c, "documnets/tar"); got != nil {
		t.Errorf("offered %q after `dirspell` was taken back", got)
	}
}

// A corrected directory goes into the line as a word, so what is in its name
// carries a backslash. Measured: bash escapes a `$` and a space in the path it
// writes back.
//
// The typed dollar is escaped, and since #1574 that is load-bearing rather
// than incidental: a bare `$q` in the directory portion is a *parameter* now
// and an unset one expands to nothing, which would leave the corrector looking
// at `od d` for a directory called `od d$r` and finding it too far away. The
// backslash is how somebody says they meant the character, and it is what
// keeps this row about the escaping of the answer rather than about the
// expansion of the question. See repl.hasLiteralDollar.
func TestACorrectedDirectoryIsEscapedForTheLine(t *testing.T) {
	dir := t.TempDir()
	odd := filepath.Join(dir, "od d$r")
	if err := os.MkdirAll(odd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(odd, "inside.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	c, _ := spellCompleter(dir, true, true)
	got := completeWord(c, `od d\$q/ins`)
	if len(got) != 1 {
		t.Fatalf("offered %q, want one match", got)
	}
	for _, want := range []string{`od\ d\$r/`, "inside.txt"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("offered %q, want it to contain %q", got[0], want)
		}
	}
	// And not the raw name, which would end the word at the space and start
	// an expansion at the dollar.
	if strings.Contains(got[0], "od d$r/") {
		t.Errorf("offered %q with the directory unescaped", got[0])
	}
}

// The correction has to land on a *directory*. A word one edit from a file is
// not a directory to read, and answering with it would rewrite the line and
// then find nothing there.
func TestACorrectionOntoAFileIsNotOne(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	c, _ := spellCompleter(dir, true, true)
	if got := completeWord(c, "notes.tx/any"); got != nil {
		t.Errorf("offered %q for a correction onto a file, want nothing", got)
	}
}
