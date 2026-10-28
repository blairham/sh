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
	"github.com/blairham/sh/internal/treecodec"
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
		{
			"line numbers",
			"echo top $LINENO\nf() {\n  echo in f $LINENO ${funcfiletrace[1]##*/} ${funcsourcetrace[1]##*/}\n  print -P '%x:%i %N'\n}\nf\necho after $LINENO\n",
			". ./lib; f",
		},
		{"xtrace", "PS4='+%N:%i> '\nset -x\nx=1\necho $x\nset +x\n", ". ./lib"},
		{
			"diagnostics",
			"echo before\nno-such-command-here\nprint ${unset_one:?is not set}\necho after\n",
			". ./lib; echo st=$?",
		},
		{"here-documents", "cat <<EOF\nline $LINENO ${HOME:+home}\nEOF\ncat <<-'X'\n\tkept $LINENO\n\tX\n", ". ./lib"},
		{"aliases", "alias greet='echo hi:'\ngreet one\nalias -g G='| tr a-z A-Z'\necho lower G\n", ". ./lib"},
		{"grammar", "print -r -- 'a''b'\nsetopt rcquotes\nprint -r -- 'a''b'\nunsetopt rcquotes\n", ". ./lib"},
		{"substitutions", "v=$(echo sub $LINENO\necho two)\necho \"$v\"\necho `echo back`\n", ". ./lib"},
		{
			"return",
			"echo one\n[[ -n $stop ]] && return 7\necho two\n",
			"stop=1; . ./lib; echo st=$?; stop=; . ./lib; echo st=$?",
		},
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

// TestTheSwitchCanBeThrownFromAScript is SH_NO_PARSE_CACHE set by a line of
// script rather than by the environment the shell started in: from that line
// on, nothing is read from the cache or recorded into it.
func TestTheSwitchCanBeThrownFromAScript(t *testing.T) {
	dir, cache := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib"), []byte(worthCaching+"echo lib\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	argv := []string{"zsh", "-f", "-c", ". ./lib"}
	runWithCache(t, cache, dir, true, nil, argv...)
	if got := runWithCache(t, cache, dir, true, nil, argv...); got.served == 0 {
		t.Fatal("the control was not served: the row below proves nothing")
	}
	off := runWithCache(t, cache, dir, true, nil, "zsh", "-f", "-c", "SH_NO_PARSE_CACHE=1; . ./lib")
	if off.out != "lib\n" || off.served != 0 || off.recorded != 0 {
		t.Fatalf("printed %q, served %d, recorded %d", off.out, off.served, off.recorded)
	}
}

// TestAnAutoloadedFileIsCachedUnobservably is the same claim for function
// files `autoload` reads, which are parsed four ways each — to see whether a
// substitution swallows its closer, whether the file is one lone definition,
// whether it parses at all, and as the body — and every one of those
// readings goes through the cache. The rows are a plain body, a file that is
// one definition of its own name, a body using an alias defined before it was
// loaded (read with the tables, and with -U without them), a body that will
// not parse, and the listing and the line numbers a body reports.
func TestAnAutoloadedFileIsCachedUnobservably(t *testing.T) {
	files := map[string]string{
		"plain":    "echo plain $LINENO ${funcfiletrace[1]##*/}\nprint -r -- ${(j:,:)@}\n",
		"lone":     "lone() {\n  echo lone body $LINENO\n}\n",
		"aliased":  "greet from-a-file\n",
		"refused":  "echo before\nif then\n",
		"subst":    "v=$(echo inner $LINENO)\necho $v\n",
		"listed":   "local x=1\nfor i in a b; do echo $i$x; done\n",
		"withargs": "echo args $# $1\n",
	}
	cases := []struct{ name, main string }{
		{"a plain body", "autoload -Uz plain; plain a b; whence -v plain | sed 's#/.*/##'"},
		{"one lone definition", "autoload -Uz lone; lone; functions lone"},
		{"an alias read with the tables", "alias greet='echo hi:'; autoload -z aliased; aliased"},
		{"an alias not read under -U", "alias greet='echo hi:'; autoload -Uz aliased; aliased; echo st=$?"},
		{"a body that will not parse", "autoload -Uz refused; refused; echo st=$?"},
		{"a substitution", "autoload -Uz subst; subst"},
		{"the listing", "autoload -Uz listed; autoload +X listed; functions listed; listed"},
		{"arguments", "autoload -Uz withargs; withargs one two"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir, cache, fns := t.TempDir(), t.TempDir(), t.TempDir()
			for name, body := range files {
				if err := os.WriteFile(filepath.Join(fns, name), []byte(worthCaching+body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			argv := []string{"zsh", "-f", "-c", "fpath=(" + fns + " $fpath); " + c.main}
			off := runWithCache(t, cache, dir, false, nil, argv...)
			cold := runWithCache(t, cache, dir, true, nil, argv...)
			warm := runWithCache(t, cache, dir, true, nil, argv...)
			if off.out == "" && off.errs == "" {
				t.Fatal("the case wrote nothing")
			}
			for name, got := range map[string]cacheRun{"cold": cold, "warm": warm} {
				if got.out != off.out || got.errs != off.errs || got.code != off.code {
					t.Errorf("%s: out %q errs %q code %d\nwant out %q errs %q code %d",
						name, got.out, got.errs, got.code, off.out, off.errs, off.code)
				}
			}
			if c.name != "a body that will not parse" && warm.served == 0 {
				t.Errorf("the warm run served nothing (cold recorded %d)", cold.recorded)
			}
		})
	}
}

// TestAnAutoloadedAliasChangeIsSeen loads a function file whose body expands
// an alias, then changes what the alias says between two starts: the second
// start's body says the new thing.
func TestAnAutoloadedAliasChangeIsSeen(t *testing.T) {
	dir, cache, fns := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(fns, "aliased"), []byte(worthCaching+"greet there\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(value string) cacheRun {
		return runWithCache(t, cache, dir, true, nil, "zsh", "-f", "-c",
			"fpath=("+fns+" $fpath); alias greet='"+value+"'; autoload -z aliased; aliased")
	}
	run("echo one:")
	if got := run("echo one:"); got.out != "one: there\n" || got.served == 0 {
		t.Fatalf("the control printed %q from %d served reads", got.out, got.served)
	}
	if got := run("echo two:"); got.out != "two: there\n" {
		t.Fatalf("the body read with the old alias: %q", got.out)
	}
}

// deferredLib defines functions long enough for the parse cache to keep their
// bodies encoded until they are asked for (#6483): one that is listed, called
// and fails partway; one defined inside it; and one nobody touches.
var deferredLib = worthCaching + `big() {
  local n=$1
  print -r -- "big $n at $LINENO in ${funcfiletrace[1]##*/}"
  inner() {
    print -r -- "inner $1 at $LINENO"
    print -r -- "inner again $(( $1 * 2 )) ${(j:,:)@}"
  }
  inner $n
  if [[ $n == fail ]]; then
    print -r -- ${missing_one:?is not set in big}
  fi
  print -r -- "big done $n ${#n} ${(U)n}"
}
untouched() {
  print -r -- "untouched $LINENO $1 $2 $3"
  print -r -- "more text so the body is long enough to defer $(( 1 + 2 ))"
  print -r -- "and more, still untouched, ${HOME:+home} ${PATH:+path}"
  print -r -- "and a last line that nothing ever runs during a start"
}
`

// TestADeferredBodyIsUnobservable: a body the cache kept encoded must behave
// exactly as a parsed one wherever a script can see it — called, listed by
// `functions`, `typeset -f`, `whence -c` and `$functions`, failing at the
// line it fails on, redefined, removed — and a definition alone must not
// decode it.
func TestADeferredBodyIsUnobservable(t *testing.T) {
	dir, cache := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib"), []byte(deferredLib), 0o600); err != nil {
		t.Fatal(err)
	}
	script := strings.Join([]string{
		". ./lib",
		"functions big",
		"typeset -f inner; echo inner-before=$?",
		"big one",
		"typeset -f inner",
		`print -r -- "$functions[big]"`,
		"whence -c untouched",
		"(big fail); echo st=$?",
		"big() { print redefined $1; }",
		"big two",
		"unfunction big",
		"big three; echo st=$?",
		"functions untouched | wc -l",
	}, "\n")
	argv := []string{"zsh", "-f", "-c", script}
	off := runWithCache(t, cache, dir, false, nil, argv...)
	cold := runWithCache(t, cache, dir, true, nil, argv...)
	deferred, read := treecodec.BodiesDeferred.Load(), treecodec.BodiesRead.Load()
	warm := runWithCache(t, cache, dir, true, nil, argv...)
	deferred, read = treecodec.BodiesDeferred.Load()-deferred, treecodec.BodiesRead.Load()-read
	if !strings.Contains(off.out, "inner one at") || !strings.Contains(off.errs, "is not set in big") {
		t.Fatalf("the script did not run as written: %q %q", off.out, off.errs)
	}
	for name, got := range map[string]cacheRun{"cold": cold, "warm": warm} {
		if got.out != off.out || got.errs != off.errs || got.code != off.code {
			t.Errorf("%s: out %q errs %q code %d\nwant out %q errs %q code %d",
				name, got.out, got.errs, got.code, off.out, off.errs, off.code)
		}
	}
	if warm.served == 0 || deferred < 2 || read < 2 {
		t.Errorf("warm served %d reads, deferred %d bodies and read %d: deferring was not exercised",
			warm.served, deferred, read)
	}

	// Defining is not reading: a start that only sources the file leaves
	// every body it deferred encoded — and so does asking for the names
	// alone, nested in another expansion the way a plugin manager diffs the
	// function table around each load, which once rendered every body to
	// learn that the inner value was a list.
	deferred, read = treecodec.BodiesDeferred.Load(), treecodec.BodiesRead.Load()
	only := runWithCache(t, cache, dir, true, nil, "zsh", "-f", "-c",
		". ./lib; x=( ${${(k)functions}:#_*} ); print ${#x} ${(o)x}")
	deferred, read = treecodec.BodiesDeferred.Load()-deferred, treecodec.BodiesRead.Load()-read
	if only.served == 0 || deferred < 2 || read != 0 || only.out != "2 big untouched\n" {
		t.Errorf("sourcing alone wrote %q, served %d, deferred %d bodies and read %d of them",
			only.out, only.served, deferred, read)
	}
}
