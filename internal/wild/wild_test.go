// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/wild"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// What counts as a shell script is the shebang, read the way the kernel reads
// it: the interpreter's base name, or the word after `env`.
func TestFindReadsTheShebang(t *testing.T) {
	dir := t.TempDir()
	want := map[string]bool{}
	for _, tc := range []struct{ name, head string }{
		{"plain-sh", "#!/bin/sh\n"},
		{"plain-bash", "#!/bin/bash\n"},
		{"with-flags", "#!/bin/bash -e\n"},
		{"via-env", "#!/usr/bin/env bash\n"},
		{"via-env-sh", "#!/usr/bin/env sh\n"},
		{"local-bash", "#!/usr/local/bin/bash\n"},
	} {
		want[write(t, dir, tc.name, tc.head+"echo hi\n")] = true
	}
	for _, tc := range []struct{ name, head string }{
		{"python", "#!/usr/bin/python3\n"},
		{"perl-via-env", "#!/usr/bin/env perl\n"},
		{"no-shebang", "echo hi\n"},
		// The shebang marker itself matters, not just the words after it: a
		// file whose first line merely *mentions* a shell is not a script.
		{"names-a-shell-without-a-shebang", "sh -c 'echo hi'\n"},
		{"almost-a-shebang", "# !/bin/sh\n"},
		{"empty", ""},
		{"binary-ish", "\x7fELF\x02\x01\x01\x00"},
	} {
		write(t, dir, tc.name, tc.head)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, _ := wild.Find(wild.Scope{Dirs: []string{dir, "/nonexistent-directory"}})
	if len(got) != len(want) {
		t.Fatalf("found %d scripts, want %d: %v", len(got), len(want), got)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("found %s, which is not a shell script", p)
		}
	}
}

// Zsh function files are read by the shell rather than run by the kernel, so
// they carry no shebang and say what they are zsh's own way. A sweep that
// looked only for `#!` missed nearly every real zsh program on a machine.
func TestFindReadsZshsOwnDeclaration(t *testing.T) {
	dir := t.TempDir()
	want := map[string]bool{}
	for _, tc := range []struct{ name, head string }{
		{"completion", "#compdef mytool\n"},
		{"completion-bare", "#compdef\n"},
		{"loaded-on-use", "#autoload\n"},
		{"shebang-zsh", "#!/bin/zsh\n"},
		{"shebang-env-zsh", "#!/usr/bin/env zsh\n"},
	} {
		want[write(t, dir, tc.name, tc.head+"echo hi\n")] = true
	}
	for _, tc := range []struct{ name, head string }{
		// The marker is a whole word, not a prefix of one.
		{"not-a-marker", "#compdefine\n"},
		{"ordinary-comment", "# compdef mytool\n"},
		{"a-bash-script", "#!/bin/bash\n"},
	} {
		write(t, dir, tc.name, tc.head+"echo hi\n")
	}

	got, _ := wild.Find(wild.Scope{Dirs: []string{dir}, Shells: wild.ZshScope})
	if len(got) != len(want) {
		t.Fatalf("found %d zsh scripts, want %d: %v", len(got), len(want), got)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("found %s, which is not a zsh script", p)
		}
	}
}

// The sweep descends, because the scripts that run on an ordinary working day
// are not all in a bin directory — a package's helpers sit several levels down
// behind a symbolic link, which is where a package manager puts everything.
func TestFindDescendsAndFollowsLinks(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "one", "two", "three")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	buried := write(t, deep, "helper", "#!/bin/sh\necho hi\n")

	// A link to the tree, the way /opt/homebrew/opt links into the Cellar.
	linked := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "one"), linked); err != nil {
		t.Fatal(err)
	}

	// Deep enough to reach it: the link counts as one level, like a directory.
	// It is reported once, by whichever route the walk met first — which of
	// the two is not the point and depends on the order the directory lists.
	got, _ := wild.Find(wild.Scope{Dirs: []string{dir}, Depth: 4})
	if len(got) != 1 {
		t.Fatalf("found %v, want one script — one file reachable two ways is one script", got)
	}
	// Both sides are resolved: a temporary directory is itself reached through
	// a link on some systems, so comparing a resolved path with an unresolved
	// one fails for a reason that has nothing to do with the sweep.
	want, err := filepath.EvalSymlinks(buried)
	if err != nil {
		t.Fatal(err)
	}
	if real, err := filepath.EvalSymlinks(got[0]); err != nil || real != want {
		t.Errorf("found %s, which resolves to %s, want %s", got[0], real, want)
	}

	// Not deep enough: the walk stops above it rather than reporting nothing
	// at all, which is the difference between a bound and a bug.
	got, _ = wild.Find(wild.Scope{Dirs: []string{dir}, Depth: 2})
	if len(got) != 0 {
		t.Errorf("found %v at depth 2, want nothing that deep", got)
	}
}

// A link that points into a tree nobody may read is still that tree. The rule
// is applied to where the walk arrived and to where it points, because a bin
// directory is full of innocent-looking names pointing into shell packages.
func TestFindRefusesALinkIntoADeniedTree(t *testing.T) {
	dir := t.TempDir()
	cellar := filepath.Join(dir, "Cellar", "zsh", "5.9", "bin")
	bin := filepath.Join(dir, "bin")
	for _, d := range []string{cellar, bin} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write(t, cellar, "zshbug", "#!/bin/sh\necho hi\n")
	if err := os.Symlink(filepath.Join(cellar, "zshbug"), filepath.Join(bin, "zshbug")); err != nil {
		t.Fatal(err)
	}

	got, skipped := wild.Find(wild.Scope{Dirs: []string{bin}, Depth: 2})
	if len(got) != 0 {
		t.Errorf("found %v, want nothing: the link points into a shell's own distribution", got)
	}
	if skipped[wild.ReasonShellSource] != 1 {
		t.Errorf("skipped[%q] = %d, want 1", wild.ReasonShellSource, skipped[wild.ReasonShellSource])
	}
}

// A file whose shebang says shell is not necessarily shell. The reference is
// what tells them apart, and a file it also refuses must not be counted
// against this parser — a Tcl program that execs its real interpreter from
// the first line is the usual case.
func TestTheReferenceDecidesWhatIsNotAScript(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "good", "#!/bin/sh\necho hi\n")
	write(t, dir, "bad", "#!/bin/sh\n{ fi; }\n")

	ctx := context.Background()
	// A reference that accepts everything makes the refusal ours.
	rep := wild.Sweep(ctx, wild.Scope{Dirs: []string{dir}}, bash.Dialect(), "/usr/bin/true")
	if rep.Scanned != 2 || rep.Parsed != 1 || rep.NotShell != 0 || len(rep.Failures) != 1 {
		t.Errorf("accepting reference: %+v", rep)
	}
	// One that refuses everything makes it the file's.
	rep = wild.Sweep(ctx, wild.Scope{Dirs: []string{dir}}, bash.Dialect(), "/usr/bin/false")
	if rep.Scanned != 2 || rep.Parsed != 1 || rep.NotShell != 1 || len(rep.Failures) != 0 {
		t.Errorf("refusing reference: %+v", rep)
	}
	// And no reference at all trusts the shebang.
	rep = wild.Sweep(ctx, wild.Scope{Dirs: []string{dir}}, bash.Dialect(), "")
	if len(rep.Failures) != 1 {
		t.Errorf("no reference: %+v", rep)
	}
}

// The sweep reads and never runs, which is what makes it safe to point at
// /usr/bin. The reference is asked to *parse* the file and nothing more.
//
// This asserts on how the reference is invoked rather than on a side effect,
// because a side effect needs a script this parser refuses and the reference
// accepts — which is to say a current gap, and a test that stops testing
// anything the day the gap is closed.
func TestTheSweepOnlyAsksTheReferenceToParse(t *testing.T) {
	dir := t.TempDir()
	// A file this parser refuses, so the reference is consulted at all.
	write(t, dir, "broken", "#!/bin/sh\n{ fi; }\n")

	// A reference that records how it was called and accepts everything.
	log := filepath.Join(dir, "argv")
	ref := filepath.Join(dir, "ref")
	if err := os.WriteFile(ref, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >> "+log+"\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	rep := wild.Sweep(context.Background(), wild.Scope{Dirs: []string{dir}}, bash.Dialect(), ref)
	if len(rep.Failures) != 1 {
		t.Fatalf("want the broken file counted against us, got %+v", rep)
	}
	argv, err := os.ReadFile(log)
	if err != nil {
		t.Fatal("the reference was never consulted")
	}
	if !strings.Contains(string(argv), "-n") {
		t.Errorf("the reference was called with %q, want -n so that it parses rather than runs", argv)
	}
}

// A framework file is sourced rather than run, so it has no shebang and
// declares nothing: the name is all there is. Pointing the sweep at a real
// plugin tree without this found 41 files in a tree holding 123 `.zsh`, and
// the parse failures that prompted the root were in the ones with no first
// line to read.
func TestFindIdentifiesAFrameworkFileByName(t *testing.T) {
	dir := t.TempDir()
	zsh := map[string]bool{}
	bash := map[string]bool{}
	for _, tc := range []struct {
		name, body string
		want       *map[string]bool
	}{
		{"prompt.zsh", "typeset -g x=1\n", &zsh},
		{"git.plugin.zsh", "alias g=git\n", &zsh},
		{"agnoster.zsh-theme", "PROMPT='%~ '\n", &zsh},
		{"UPPER.ZSH", "typeset -g y=1\n", &zsh},
		{"lib.sh", "x=1\n", &bash},
		{"helpers.bash", "x=1\n", &bash},
		// A first line still wins where there is one: the marker says zsh
		// whatever the name is, and the sweep already knew that.
		{"_git", "#compdef git\n", &zsh},
	} {
		(*tc.want)[write(t, dir, tc.name, tc.body)] = true
	}
	for _, tc := range []struct{ name, body string }{
		// The kernel obeys the shebang, so a name is a leftover beside one.
		{"tool.sh", "#!/usr/bin/perl\nprint 1;\n"},
		// A bats file is a suite in a language that is bash with a @test
		// header — neither ours to read nor ours to parse.
		{"cases.bats", "@test \"works\" {\n  true\n}\n"},
		{"README.md", "# notes\n"},
		{"data.json", "{}\n"},
		{"noextension", "x=1\n"},
	} {
		write(t, dir, tc.name, tc.body)
	}

	for _, tc := range []struct {
		name   string
		shells map[string]bool
		want   map[string]bool
	}{
		{"zsh", wild.ZshScope, zsh},
		{"bash", wild.BashScope, bash},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := wild.Find(wild.Scope{Dirs: []string{dir}, Shells: tc.shells})
			if len(got) != len(tc.want) {
				t.Fatalf("found %d, want %d: %v", len(got), len(tc.want), got)
			}
			for _, p := range got {
				if !tc.want[p] {
					t.Errorf("found %s, which this scope does not claim", p)
				}
			}
		})
	}
}

// FrameworkDepth is the constant that makes a configured root reach a plugin's
// own code. A bin directory is flat; a framework tree is a checkout per
// plugin, and the file that matters sits several levels inside one.
func TestFrameworkDepthReachesInsideACheckout(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "plugins", "author---name", "internal", "lib", "src")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	want := write(t, deep, "prompt.zsh", "typeset -g x=1\n")

	got, _ := wild.Find(wild.Scope{Dirs: []string{dir}, Shells: wild.ZshScope, Depth: wild.FrameworkDepth})
	if len(got) != 1 || got[0] != want {
		t.Fatalf("at FrameworkDepth found %v, want %s", got, want)
	}
	// The control, and the reason the constant exists: the depth a bin
	// directory needs does not reach this.
	if got, _ := wild.Find(wild.Scope{Dirs: []string{dir}, Shells: wild.ZshScope}); len(got) != 0 {
		t.Errorf("at DefaultDepth found %v, want none — the constant would be doing nothing", got)
	}
}

// Where the extra roots come from, and why they come from the environment: no
// default could be right, because the path is one machine's.
func TestDirsFromReadsTheEnvironment(t *testing.T) {
	sep := string(os.PathListSeparator)
	for _, tc := range []struct {
		name  string
		value string
		set   bool
		want  []string
	}{
		{"unset is no roots, which is what CI has", "", false, nil},
		{"empty is no roots", "", true, nil},
		{"one root", "/a", true, []string{"/a"}},
		{"several, separated like PATH", "/a" + sep + "/b", true, []string{"/a", "/b"}},
		{"an empty entry is not a root", "/a" + sep + sep + "/b", true, []string{"/a", "/b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			get := func(name string) (string, bool) {
				if name != wild.DirsVar {
					t.Errorf("asked for %q, want %q", name, wild.DirsVar)
				}
				return tc.value, tc.set
			}
			got := wild.DirsFrom(get)
			if len(got) != len(tc.want) {
				t.Fatalf("DirsFrom = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("DirsFrom[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// A missing root is skipped rather than fatal, so one variable serves a laptop
// and a CI runner — and it is still worth naming, because what a silent skip
// hides is a typo, and a typo here reads as a clean sweep.
func TestAbsentNamesTheRootsThatAreNotDirectories(t *testing.T) {
	dir := t.TempDir()
	file := write(t, dir, "notadir.zsh", "x=1\n")
	got := wild.Absent([]string{dir, file, filepath.Join(dir, "nope")})
	want := []string{file, filepath.Join(dir, "nope")}
	if len(got) != len(want) {
		t.Fatalf("Absent = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("Absent[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
