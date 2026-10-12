// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/parsecache"
)

// worthCaching is commentary enough to put a file over the size the cache
// starts at; a sourced file shorter than that is parsed every time.
var worthCaching = strings.Repeat("# commentary, to make this file one the parse cache keeps\n", 80)

// run is one start of the shell, with the parse cache on or off, and what it
// wrote, the status it left, and how many reads the cache served.
type cacheRun struct {
	out, errs string
	code      int
	served    int64
	recorded  int64
}

func runWithCache(t *testing.T, cache, dir string, on bool, env map[string]string, argv ...string) cacheRun {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", cache)
	if on {
		t.Setenv("SH_NO_PARSE_CACHE", "")
	} else {
		t.Setenv("SH_NO_PARSE_CACHE", "1")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	var out, errs bytes.Buffer
	sh := zshShell()
	sh.Dir = dir
	sh.Stdout, sh.Stderr = &out, &errs
	served, recorded := parsecache.Served.Load(), parsecache.Recorded.Load()
	code := driver.MainArgs(sh, argv)
	return cacheRun{
		out: out.String(), errs: errs.String(), code: code,
		served:   parsecache.Served.Load() - served,
		recorded: parsecache.Recorded.Load() - recorded,
	}
}

// TestAParseCacheIsUnobservable runs files through `.` and the startup files
// three ways — with the cache off, with it empty, and with it filled by that
// run — and requires all three to write the same bytes and leave the same
// status, the third having had its reads from the cache. The rows are what a
// tree carries that a script can see: line numbers in every place one shows,
// the xtrace prefix, a function's file and line, diagnostics located in the
// file, here-documents, aliases defined by earlier lines, a grammar an earlier
// line changed, `$(…)` read with its line, a `return` partway, and a parse
// failure after commands that ran.
func TestAParseCacheIsUnobservable(t *testing.T) {
	cases := []struct {
		name string
		lib  string
		main string
	}{
		{"line numbers", "echo top $LINENO\nf() {\n  echo in f $LINENO ${funcfiletrace[1]##*/} ${funcsourcetrace[1]##*/}\n  print -P '%x:%i %N'\n}\nf\necho after $LINENO\n", ". ./lib; f"},
		{"xtrace", "PS4='+%N:%i> '\nset -x\nx=1\necho $x\nset +x\n", ". ./lib"},
		{"diagnostics", "echo before\nno-such-command-here\nprint ${unset_one:?is not set}\necho after\n", ". ./lib; echo st=$?"},
		{"here-documents", "cat <<EOF\nline $LINENO ${HOME:+home}\nEOF\ncat <<-'X'\n\tkept $LINENO\n\tX\n", ". ./lib"},
		{"aliases", "alias greet='echo hi:'\ngreet one\nalias -g G='| tr a-z A-Z'\necho lower G\n", ". ./lib"},
		{"grammar", "print -r -- 'a''b'\nsetopt rcquotes\nprint -r -- 'a''b'\nunsetopt rcquotes\n", ". ./lib"},
		{"substitutions", "v=$(echo sub $LINENO\necho two)\necho \"$v\"\necho `echo back`\n", ". ./lib"},
		{"return", "echo one\n[[ -n $stop ]] && return 7\necho two\n", "stop=1; . ./lib; echo st=$?; stop=; . ./lib; echo st=$?"},
		{"parse failure", "echo ran\nif then\necho never\n", ". ./lib; echo st=$?"},
		{"twice", "n=$((n+1))\necho read $n\n", ". ./lib; . ./lib"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir, cache := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "lib"), []byte(worthCaching+c.lib), 0o600); err != nil {
				t.Fatal(err)
			}
			argv := []string{"zsh", "-f", "-c", c.main}
			off := runWithCache(t, cache, dir, false, nil, argv...)
			cold := runWithCache(t, cache, dir, true, nil, argv...)
			warm := runWithCache(t, cache, dir, true, nil, argv...)
			for name, got := range map[string]cacheRun{"cold": cold, "warm": warm} {
				if got.out != off.out || got.errs != off.errs || got.code != off.code {
					t.Errorf("%s: out %q errs %q code %d\nwant out %q errs %q code %d",
						name, got.out, got.errs, got.code, off.out, off.errs, off.code)
				}
			}
			if off.served != 0 || off.recorded != 0 {
				t.Errorf("with the cache off it served %d and recorded %d", off.served, off.recorded)
			}
			if c.name != "parse failure" && (cold.recorded == 0 || warm.served == 0 || warm.recorded != 0) {
				t.Errorf("cold recorded %d, warm served %d and recorded %d: the cache was not exercised",
					cold.recorded, warm.served, warm.recorded)
			}
		})
	}
}

// TestAStartupFileIsCachedUnobservably is the same claim for the startup
// files, which reach the parser by a route of their own.
func TestAStartupFileIsCachedUnobservably(t *testing.T) {
	dir, cache, zdot := t.TempDir(), t.TempDir(), t.TempDir()
	env := worthCaching + "echo env $LINENO\nalias hey='echo hey:'\nhey there\nf() { echo f $LINENO ${funcfiletrace[1]##*/}; }\n"
	if err := os.WriteFile(filepath.Join(zdot, ".zshenv"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	argv := []string{"zsh", "-c", "f; echo main $LINENO"}
	vars := map[string]string{"ZDOTDIR": zdot}
	off := runWithCache(t, cache, dir, false, vars, argv...)
	cold := runWithCache(t, cache, dir, true, vars, argv...)
	warm := runWithCache(t, cache, dir, true, vars, argv...)
	if !strings.Contains(off.out, "env ") {
		t.Fatalf("the startup file did not run: %q %q", off.out, off.errs)
	}
	for name, got := range map[string]cacheRun{"cold": cold, "warm": warm} {
		if got.out != off.out || got.errs != off.errs || got.code != off.code {
			t.Errorf("%s: %q %q %d, want %q %q %d", name, got.out, got.errs, got.code, off.out, off.errs, off.code)
		}
	}
	if warm.served == 0 {
		t.Error("the startup file was not read from the cache")
	}
}

// TestAnEditedFileIsReadAsEdited edits a sourced file in place between two
// starts, keeping its length and putting its time back, which is the edit a
// cache keyed on the file's stat would miss. The edit moves a word's
// boundary rather than only its letters: a tree that kept its places and read
// its words out of the new text would print the new letters from the old
// command, and look right.
func TestAnEditedFileIsReadAsEdited(t *testing.T) {
	dir, cache := t.TempDir(), t.TempDir()
	path := filepath.Join(dir, "lib")
	if err := os.WriteFile(path, []byte(worthCaching+"echo one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	argv := []string{"zsh", "-f", "-c", ". ./lib"}
	if got := runWithCache(t, cache, dir, true, nil, argv...); got.out != "one\n" {
		t.Fatalf("got %q", got.out)
	}
	if err := os.WriteFile(path, []byte(worthCaching+"echo ;on\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	got := runWithCache(t, cache, dir, true, nil, argv...)
	if got.out != "\n" || !strings.Contains(got.errs, "command not found: on") {
		t.Fatalf("the edited file printed %q and said %q", got.out, got.errs)
	}
	// Touching it changes nothing about what it says, and the entry stands:
	// what is checked is the text, and the text is the same.
	later := fi.ModTime().Add(1e9)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if again := runWithCache(t, cache, dir, true, nil, argv...); again.out != got.out || again.served == 0 {
		t.Fatalf("the touched file printed %q from %d served reads", again.out, again.served)
	}
}
