// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCompleter answers from a fixed list, so the editor's half can be tested
// without a filesystem or a PATH.
type fakeCompleter struct{ cmds, paths []string }

func (f fakeCompleter) commands(prefix string) []string { return withPrefix(f.cmds, prefix) }
func (f fakeCompleter) files(prefix string) []string    { return withPrefix(f.paths, prefix) }

func withPrefix(all []string, prefix string) []string {
	var out []string
	for _, s := range all {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}

// Which kind of completion a word gets is decided by where it sits, not by
// what is in it — the first word of a command is a command.
func TestCommandPositionDecidesTheKind(t *testing.T) {
	for _, c := range []struct {
		line  string
		start int
		want  bool
	}{
		{"", 0, true},
		{"ec", 0, true},
		{"echo ", 5, false},
		{"echo a", 5, false},
		{"  ec", 2, true},
		{"ls | gr", 5, true},
		{"ls; gr", 4, true},
		{"ls && gr", 6, true},
		{"ls & gr", 5, true},
		{"( gr", 2, true},
		{"if tr", 3, false},
		// A second argument is still an argument.
		{"cp a b", 5, false},
	} {
		if got := commandPosition([]rune(c.line), c.start); got != c.want {
			t.Errorf("%q at %d: got %v, want %v", c.line, c.start, got, c.want)
		}
	}
}

// The word under the cursor ends at whitespace, and an escaped space is not
// whitespace — someone escaped it because it is part of the name.
func TestWordStart(t *testing.T) {
	for _, c := range []struct {
		line string
		pos  int
		want int
	}{
		{"echo abc", 8, 5},
		{"echo", 4, 0},
		{"", 0, 0},
		{"echo  ", 6, 6},
		{`echo a\ b`, 9, 5},
		{`ls /usr/bi`, 10, 3},
	} {
		if got := wordStart([]rune(c.line), c.pos); got != c.want {
			t.Errorf("%q at %d: got %d, want %d", c.line, c.pos, got, c.want)
		}
	}
}

// One match is filled in; several are filled in as far as they agree and then
// returned so the editor can list them.
func TestCompleting(t *testing.T) {
	c := fakeCompleter{
		cmds:  []string{"echo", "export", "exit"},
		paths: []string{"apple.txt", "apples.txt", "apricot.txt", "banana.txt", "sub/"},
	}
	for _, tc := range []struct {
		name, typed, want string
		listed            []string
	}{
		{"a single command", "ech", "echo ", nil},
		{"a single file", "echo ban", "echo banana.txt ", nil},
		{"a directory keeps the cursor on it", "echo sub", "echo sub/", nil},
		{"as far as they agree", "ex", "ex", []string{"exit", "export"}},
		{"nothing matches", "echo zzz", "echo zzz", nil},
		// `ap` is already as far as these agree, so nothing is filled in and
		// the matches come back to be listed.
		{"as far as they agree, which is nowhere", "echo ap", "echo ap", []string{"apple.txt", "apples.txt", "apricot.txt"}},
		// `appl` is not: the three that start with it agree on `apple`, so
		// that much is filled in and no list is needed yet.
		{"the common prefix is filled in", "echo appl", "echo apple", nil},
	} {
		e := &editor{line: []rune(tc.typed), pos: len([]rune(tc.typed)), out: &strings.Builder{}}
		got := e.complete(c)
		if string(e.line) != tc.want {
			t.Errorf("%s: line is %q, want %q", tc.name, string(e.line), tc.want)
		}
		if len(got) != len(tc.listed) {
			t.Errorf("%s: listed %q, want %q", tc.name, got, tc.listed)
		}
	}
}

// The completion goes where the cursor is, not at the end of the line.
func TestCompletingInTheMiddle(t *testing.T) {
	c := fakeCompleter{paths: []string{"banana.txt"}}
	e := &editor{line: []rune("echo ban | wc"), pos: 8, out: &strings.Builder{}}
	e.complete(c)
	if got := string(e.line); got != "echo banana.txt  | wc" {
		t.Errorf("got %q, want the rest of the line kept", got)
	}
}

// The shell completer reads the real filesystem, against the shell's working
// directory rather than the process's.
func TestTheShellCompleterReadsTheShellsDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"apple.txt", "apricot.txt", ".hidden"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := shellCompleter{dir: dir}

	if got := s.files("ap"); len(got) != 2 {
		t.Errorf("ap matched %q, want both apples", got)
	}
	// A directory is offered with the slash that follows it.
	if got := s.files("sub"); len(got) != 1 || got[0] != "subdir/" {
		t.Errorf("sub matched %q, want subdir/", got)
	}
	// A dotfile is not offered unless the dot was typed.
	if got := s.files(""); len(got) != 3 {
		t.Errorf("the bare prefix matched %q, want the three visible ones", got)
	}
	if got := s.files("."); len(got) != 1 || got[0] != ".hidden" {
		t.Errorf(". matched %q, want the hidden one", got)
	}
}

// A command completes against what is executable on PATH, and a name that is
// not executable is not a command.
func TestTheShellCompleterReadsPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "runnable"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readable"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s := shellCompleter{names: []string{"echo", "export"}, path: dir}
	got := s.commands("r")
	if len(got) != 1 || got[0] != "runnable" {
		t.Errorf("matched %q, want only the executable one", got)
	}
	// The names it was given are commands too, and come back sorted with the
	// rest rather than in their own group.
	if got := s.commands("e"); len(got) != 2 || got[0] != "echo" || got[1] != "export" {
		t.Errorf("matched %q, want the builtins", got)
	}
}

func TestCommonPrefix(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{[]string{"apple", "apricot"}, "ap"},
		{[]string{"one"}, "one"},
		{[]string{"a", "b"}, ""},
		{nil, ""},
		{[]string{"日本語", "日本"}, "日本"},
	} {
		if got := commonPrefix(c.in); got != c.want {
			t.Errorf("%q gave %q, want %q", c.in, got, c.want)
		}
	}
}
