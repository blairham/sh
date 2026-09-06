// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture is the directory the measurements in docs/spec/completion.md
// were taken in: names with a space, a quote, a dollar, brackets, a leading
// dot, a leading tilde and a leading hash, a directory and a file sharing a
// prefix, and a directory that shares its prefix with nothing.
func completionFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{
		"file one.txt", "file two.txt", "quo'te.txt", "a$b.txt",
		"bracket[1].txt", "plain.txt", "dirfile", ".hidden",
		"~lead.txt", "#lead.txt",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "runme"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dir", "onlydir", "sub"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"nested.txt", "nested2.txt", "other.txt"} {
		if err := os.WriteFile(filepath.Join(dir, "sub", name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// typeAndTab presses Tab at the end of a typed line and reports what the line
// became and what would be listed.
func typeAndTab(t *testing.T, c Completer, typed string) (string, []string) {
	t.Helper()
	e := &editor{line: []rune(typed), pos: len([]rune(typed)), out: &strings.Builder{}}
	listed := e.complete(c)
	return string(e.line), listed
}

// What Tab does to an argument, against the fixture the real shells were
// measured in. Every want here is what bash 5.3.15 and zsh 5.9.2 both did.
func TestCompletingAFilename(t *testing.T) {
	dir := completionFixture(t)
	s := shellCompleter{dir: dir}
	for _, tc := range []struct {
		name, typed, want string
	}{
		{"a file gets a space", ": pla", ": plain.txt "},
		{"a directory gets a slash and no space", ": only", ": onlydir/"},
		{"an unescaped space ends the word", ": file o", ": file onlydir/"},
		{"an escaped space does not", `: file\ o`, `: file\ one.txt `},
		{"they agree as far as the space", ": file", `: file\ `},
		{"a quote in a name is escaped", ": quo", `: quo\'te.txt `},
		{"a dollar in a name is escaped", ": a", `: a\$b.txt `},
		{"brackets are escaped", ": brack", `: bracket\[1\].txt `},
		{"a leading hash is escaped", ": #lea", `: \#lead.txt `},
		{"an escaped tilde is a filename", `: \~lea`, `: \~lead.txt `},
		{"the common prefix stops short of the slash", ": di", ": dir"},
		{"into a subdirectory", ": sub/o", ": sub/other.txt "},
		{"and stops where they stop agreeing", ": sub/n", ": sub/nested"},
		{"an empty directory offers nothing", ": onlydir/", ": onlydir/"},
		{"nothing matches", ": zzz", ": zzz"},
	} {
		if got, _ := typeAndTab(t, s, tc.typed); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.name, tc.typed, got, tc.want)
		}
	}
}

// Completion inside quotes: the space is no longer a boundary, the escaping
// changes with the quotation, and the quote is closed only when the word is
// finished.
func TestCompletingInsideQuotes(t *testing.T) {
	dir := completionFixture(t)
	s := shellCompleter{dir: dir}
	for _, tc := range []struct {
		name, typed, want string
	}{
		{"a space needs no escape inside quotes", `: "file o`, `: "file one.txt" `},
		{"and the quote is closed", `: 'file o`, `: 'file one.txt' `},
		{"a dollar is still special inside double quotes", `: "a`, `: "a\$b.txt" `},
		{"a single quote is not", `: "quo`, `: "quo'te.txt" `},
		{"but it is inside single quotes", `: 'quo`, `: 'quo'\''te.txt' `},
		{"a directory leaves the quote open", `: "only`, `: "onlydir/`},
		{"so does an unfinished agreement", `: "file`, `: "file `},
		{"a quoted tilde is a filename", `: "~lea`, `: "~lead.txt" `},
		{"a quoted word still splits on slashes", `: "sub/o`, `: "sub/other.txt" `},
	} {
		if got, _ := typeAndTab(t, s, tc.typed); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.name, tc.typed, got, tc.want)
		}
	}
}

// A name beginning with a dot is offered when the word begins with one, and
// otherwise only where the dialect says so.
func TestHiddenFiles(t *testing.T) {
	dir := completionFixture(t)
	for _, tc := range []struct {
		name   string
		hidden bool
		want   bool
	}{
		{"the core hides them", false, false},
		{"a dialect may not", true, true},
	} {
		s := shellCompleter{dir: dir, hidden: tc.hidden}
		var found bool
		for _, m := range s.files("") {
			if m == `\.hidden` || m == ".hidden" {
				found = true
			}
		}
		if found != tc.want {
			t.Errorf("%s: a bare word offered .hidden = %v, want %v", tc.name, found, tc.want)
		}
		// Asked for by its dot, it is offered either way — and it is then the
		// only match, so it completes outright.
		if got, _ := typeAndTab(t, s, ": .h"); got != ": .hidden " {
			t.Errorf("%s: `: .h` became %q, want the hidden file", tc.name, got)
		}
	}
}

// A `~` at the start of a word names a directory to read, and stays a tilde
// in the line: neither shell writes the home directory out.
func TestTilde(t *testing.T) {
	dir := completionFixture(t)
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "Documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "notes.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// An account file of the test's own, so the answer does not depend on
	// who has a login on the machine running it.
	passwd := filepath.Join(t.TempDir(), "passwd")
	if err := os.WriteFile(passwd, []byte(
		"# a comment\nfixture:*:1:1:Fixture:"+home+":/bin/sh\nfixtwo:*:2:2::/nowhere:/bin/sh\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	s := shellCompleter{dir: dir, home: home, passwd: passwd}

	for _, tc := range []struct {
		name, typed, want string
	}{
		{"the tilde is kept", ": ~/no", ": ~/notes.md "},
		{"a directory under it too", ": ~/Docu", ": ~/Documents/"},
		{"a user name gets a slash", ": ~fixture", ": ~fixture/"},
		{"and completes as a prefix", ": ~fixt", ": ~fixt"},
		{"a tilde with no slash is never a filename", ": ~lea", ": ~lea"},
		{"an unknown user completes nothing", ": ~nobodyhere", ": ~nobodyhere"},
	} {
		if got, _ := typeAndTab(t, s, tc.typed); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.name, tc.typed, got, tc.want)
		}
	}
	// `~fixt` agrees as far as it is typed, so the two accounts are listed.
	if _, listed := typeAndTab(t, s, ": ~fixt"); len(listed) != 2 {
		t.Errorf("`: ~fixt` listed %q, want both accounts", listed)
	}
	// Without a home there is nothing for a bare tilde to read, and the line
	// is left alone rather than read against the process's own home.
	bare := shellCompleter{dir: dir, passwd: passwd}
	if got, _ := typeAndTab(t, bare, ": ~/no"); got != ": ~/no" {
		t.Errorf("with no HOME `: ~/no` became %q, want it left alone", got)
	}
}

// A command word is a path as soon as it holds a slash, and then only what
// could run is offered.
func TestCommandWordWithASlash(t *testing.T) {
	dir := completionFixture(t)
	s := shellCompleter{dir: dir, names: []string{"echo"}}
	for _, tc := range []struct {
		name, typed, want string
	}{
		{"a directory could be on the way to one", "./only", "./onlydir/"},
		{"an executable is offered", "./run", "./runme "},
		{"a file that cannot run is not", "./pla", "./pla"},
		{"a bare word is still a command", "ech", "echo "},
		{"and never a file in the directory", "pla", "pla"},
	} {
		if got, _ := typeAndTab(t, s, tc.typed); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.name, tc.typed, got, tc.want)
		}
	}
}

// A listing shows the part of the name below the directory being completed,
// which is what both shells print.
func TestTheListingDropsWhatIsAlreadyTyped(t *testing.T) {
	dir := completionFixture(t)
	s := shellCompleter{dir: dir}
	_, listed := typeAndTab(t, s, ": sub/")
	want := []string{"nested.txt", "nested2.txt", "other.txt"}
	if len(listed) != len(want) {
		t.Fatalf("listed %q, want %q", listed, want)
	}
	for i := range want {
		if listed[i] != want[i] {
			t.Fatalf("listed %q, want %q", listed, want)
		}
	}
	// A directory keeps its slash in the listing.
	_, listed = typeAndTab(t, s, ": dir")
	if len(listed) != 2 || listed[0] != "dir/" || listed[1] != "dirfile" {
		t.Errorf("listed %q, want the directory marked", listed)
	}
	// The opening quote goes the same way as the directory: it is on every
	// row and about none of them.
	_, listed = typeAndTab(t, s, `: "sub/nested`)
	if len(listed) != 2 || listed[0] != "nested.txt" || listed[1] != "nested2.txt" {
		t.Errorf("listed %q, want the names without the quote or the directory", listed)
	}
}

// A symlink to a directory is a directory: it gets the slash, so the next Tab
// can continue through it.
func TestASymlinkToADirectoryIsADirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	s := shellCompleter{dir: dir}
	if got, _ := typeAndTab(t, s, ": li"); got != ": link/" {
		t.Errorf("`: li` became %q, want the slash a directory gets", got)
	}
}
