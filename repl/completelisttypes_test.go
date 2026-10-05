// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A file listing under a dialect's ListTypesOption: `ls -F`'s marks where the
// option is set, none where it is not — a blank keeping the mark's column —
// and the word inserted the same either way (#6179). The rows are zsh
// 5.9.2's with no completion system loaded, measured 2026-10-05; see
// EditorStyle.ListTypesOption and markedRow.
func TestAFileListingMarksWhatTheOptionSays(t *testing.T) {
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "plain"), nil, 0o644))
	must(os.WriteFile(filepath.Join(dir, "exe"), nil, 0o755))
	must(syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o644))
	must(os.Symlink("plain", filepath.Join(dir, "link")))
	must(os.Symlink("sub", filepath.Join(dir, "dlink")))

	for _, c := range []struct {
		name      string
		listTypes func() bool
		want      string
	}{
		{"set", func() bool { return true }, "dlink@ exe* fifo| link@ plain sub/"},
		{"unset", func() bool { return false }, "dlink_ exe_ fifo_ link_ plain sub_"},
		// A dialect with no such option draws the directory's slash and
		// nothing else, as this completer always has.
		{"no such option", nil, "dlink/ exe fifo link plain sub/"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := shellCompleter{dir: dir, listTypes: c.listTypes}
			_, rows := typeAndTab(t, s, "ls ")
			if got := strings.Join(markBlanks(rows, s), " "); got != c.want {
				t.Errorf("listed %q, want %q", got, c.want)
			}
			// The link to a directory goes in as a directory either way.
			if line, _ := typeAndTab(t, s, "ls dl"); line != "ls dlink/" {
				t.Errorf("ls dl<TAB> is %q, want %q", line, "ls dlink/")
			}
		})
	}
}

// markBlanks is each row as drawn, with a trailing blank written `_` so that
// the kept column shows; a row the completer left empty is drawn as its word,
// which is what the listing does with one.
func markBlanks(rows []string, s shellCompleter) []string {
	words := s.Complete(Completion{Line: "ls ", Point: 3, Start: 3, Dir: s.dir})
	out := make([]string, len(rows))
	for i, row := range rows {
		if row == "" && i < len(words) {
			row = words[i].Word
		}
		if strings.HasSuffix(row, " ") {
			row = strings.TrimSuffix(row, " ") + "_"
		}
		out[i] = row
	}
	return out
}
