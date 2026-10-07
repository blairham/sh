// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// The completion dump compinit keeps of its own (#6307): read back only when
// it still describes $fpath, and never zsh's `.zcompdump`.
//
// Every test here that says the dump was or was not read proves it by
// planting: the dump compinit wrote is rewritten, under the same key and with
// a valid checksum, holding one extra name `planted`. A start that reads the
// dump has that name in `_comps`; a start that scanned does not. A test that
// only compared tables could not tell a dump that was read from one that was
// ignored, since both are meant to give the same tables.

// compdumpNames are the arrays compinit keeps in the dump, in its order.
var compdumpNames = []string{"autoloads", "autoloadx", "comps", "services", "patcomps", "postpatcomps", "mainc"}

// compdumpTables prints everything a scan leaves, sorted: the four tables, and
// each `_` function with the file its stub will be read from.
const compdumpTables = `
local -a d
local k v
for k v in ${(kv)_comps}; do d+=("c $k=$v"); done
for k v in ${(kv)_services}; do d+=("s $k=$v"); done
for k v in ${(kv)_patcomps}; do d+=("p $k=$v"); done
for k v in ${(kv)_postpatcomps}; do d+=("P $k=$v"); done
for k in ${(k)functions[(I)_*]}; do d+=("f ${$(whence -v -- $k)}"); done
print -rl -- ${(o)d}
`

type compdumpEnv struct {
	home, cache string
}

func newCompdumpEnv(t *testing.T) compdumpEnv {
	t.Helper()
	home := t.TempDir()
	cache := filepath.Join(t.TempDir(), "cache")
	// Never the person's: the runner reads its own variables, and these are
	// set too in case anything reaches the process's.
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("ZDOTDIR", home)
	return compdumpEnv{home: home, cache: cache}
}

func (e compdumpEnv) file() string { return filepath.Join(e.cache, "sh", "compdump") }

// run starts compinit over the shipped functions and dirs with the prelude installed, as a front end
// does, and returns what compdumpTables prints.
func (e compdumpEnv) run(t *testing.T, dirs []string, opts string) string {
	t.Helper()
	src := "fpath=(" + quoteDirs(append([]string{shippedFunctionDir(t)}, dirs...)) + ")\nautoload -Uz compinit; compinit -u " + opts + "\n" + compdumpTables
	out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{
		Dir: e.home,
		Vars: map[string]string{
			"PATH": e.home, "HOME": e.home, "XDG_CACHE_HOME": e.cache, "ZDOTDIR": e.home,
		},
	}, src)
	if err != nil || st != 0 {
		t.Fatalf("compinit %s: status %d, %v\n%s", opts, st, err, out)
	}
	// Nothing but table rows: a diagnostic in the output is a run that went
	// wrong somewhere the comparison would not otherwise say.
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if len(line) < 2 || !strings.Contains("csPpf", line[:1]) || line[1] != ' ' {
			t.Fatalf("compinit %s printed %q:\n%s", opts, line, out)
		}
	}
	return out
}

// quoteDirs writes directories as words a script reads back unchanged: a
// test's temporary directory is named after the test.
func quoteDirs(dirs []string) string {
	q := make([]string, len(dirs))
	for i, d := range dirs {
		q[i] = "'" + strings.ReplaceAll(d, "'", `'\''`) + "'"
	}
	return strings.Join(q, " ")
}

// plant rewrites the dump, checksum and all, with `planted` added to _comps.
func (e compdumpEnv) plant(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(e.file())
	if err != nil {
		t.Fatalf("no dump to plant in: %v", err)
	}
	names, arrays, err := zsh.CompdumpDecode(data)
	if err != nil {
		t.Fatalf("the dump compinit wrote does not decode: %v", err)
	}
	i := slices.Index(names, "comps")
	if i < 0 || !slices.Equal(names[len(names)-len(compdumpNames):], compdumpNames) {
		t.Fatalf("the dump holds %v, want compinit's arrays %v at its end", names, compdumpNames)
	}
	arrays[i] = append(arrays[i], "planted", "_planted")
	if err := os.WriteFile(e.file(), zsh.CompdumpEncode(names, arrays), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTheDump(out string) bool { return hasRow(out, "c planted=_planted") }

func hasRow(out, row string) bool { return strings.Contains("\n"+out, "\n"+row+"\n") }

// compdumpFixture is two directories of completion files, aged so the racy
// window has nothing to say about them.
func compdumpFixture(t *testing.T) (a, b string) {
	t.Helper()
	a, b = t.TempDir(), t.TempDir()
	for dir, files := range map[string]map[string]string{
		a: {
			"_alpha": "#compdef alpha beta\n",
			"_svc":   "#compdef svc=service other\n",
			"_pat":   "#compdef -p 'pat*' after\n",
			"_post":  "#compdef -P 'post*'\n",
			"_help":  "#autoload\n",
			"_flag":  "#autoload +X\nprint flagged\n",
			"_none":  "nothing\n",
		},
		b: {
			"_beta":  "#compdef beta gamma\n",
			"_alpha": "#compdef shadowed\n",
		},
	} {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range files {
			writeAged(t, filepath.Join(dir, name), body)
		}
	}
	return a, b
}

func writeAged(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// One fixed time for every file, so a rewrite that puts it back leaves
	// the change time and the bytes as the only witnesses.
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

// TestCompinitReadsItsDumpBack is the positive control the rest depend on: a
// second start reads what the first wrote, and gives the tables the scan gave.
func TestCompinitReadsItsDumpBack(t *testing.T) {
	zsh.SetCompdumpForTest(t, 0, "")
	e := newCompdumpEnv(t)
	a, b := compdumpFixture(t)
	dirs := []string{a, b}
	scanned := e.run(t, dirs, "")
	if _, err := os.Stat(e.file()); err != nil {
		t.Fatalf("compinit wrote no dump: %v", err)
	}
	if again := e.run(t, dirs, ""); again != scanned {
		t.Errorf("the dump gives other tables than the scan\nscan:\n%s\ndump:\n%s", scanned, again)
	}
	for _, want := range []string{"c alpha=_alpha", "c gamma=_beta", "s svc=service", "p 'pat*'=_pat", "P 'post*'=_post", "c beta=_alpha"} {
		if !hasRow(scanned, want) {
			t.Errorf("%q is not in the tables, so the fixture is not reaching what it tests:\n%s", want, scanned)
		}
	}
	if strings.Contains(scanned, "shadowed") {
		t.Errorf("the second _alpha registered, so the first file of a name did not win:\n%s", scanned)
	}
	e.plant(t)
	if out := e.run(t, dirs, ""); !readTheDump(out) {
		t.Fatalf("an unchanged $fpath did not read the dump back:\n%s", out)
	}
	// -D is zsh's "no dump": the planted file is there and is not read.
	if out := e.run(t, dirs, "-D"); readTheDump(out) {
		t.Errorf("compinit -D read the dump")
	}
	// -C skips compaudit, not the check: the dump is still read when current.
	if out := e.run(t, dirs, "-C"); !readTheDump(out) {
		t.Errorf("compinit -C did not read a current dump")
	}
}

// TestCompinitRescansWhenFpathChanges: every change the key is meant to see
// makes the next start scan, and the scan it makes is the one -D makes.
func TestCompinitRescansWhenFpathChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, a, b string) []string
		want   string
	}{
		{"a file added", func(t *testing.T, a, b string) []string {
			writeAged(t, filepath.Join(b, "_delta"), "#compdef delta\n")
			return []string{a, b}
		}, "c delta=_delta"},
		{"a file removed", func(t *testing.T, a, b string) []string {
			if err := os.Remove(filepath.Join(a, "_svc")); err != nil {
				t.Fatal(err)
			}
			return []string{a, b}
		}, "c alpha=_alpha"},
		// Same bytes, same inode, same count of files: only the name says
		// the function is another one.
		{"a file renamed", func(t *testing.T, a, b string) []string {
			if err := os.Rename(filepath.Join(b, "_beta"), filepath.Join(b, "_betb")); err != nil {
				t.Fatal(err)
			}
			return []string{a, b}
		}, "c gamma=_betb"},
		// The same length and the old modification time put back, so only
		// the change time and the bytes say anything happened.
		{"a #compdef line edited in place", func(t *testing.T, a, b string) []string {
			writeAged(t, filepath.Join(b, "_beta"), "#compdef bet4 gamma\n")
			return []string{a, b}
		}, "c bet4=_beta"},
		{"fpath reordered", func(_ *testing.T, a, b string) []string {
			return []string{b, a}
		}, "c shadowed=_alpha"},
		{"a directory dropped", func(_ *testing.T, a, _ string) []string {
			return []string{a}
		}, "c alpha=_alpha"},
		// The same files, the same stats, through another name: what the
		// dump holds is the paths compinit autoloads by, and those moved.
		{"a directory reached by another name", func(t *testing.T, a, b string) []string {
			link := filepath.Join(t.TempDir(), "via")
			if err := os.Symlink(b, link); err != nil {
				t.Fatal(err)
			}
			return []string{a, link}
		}, "f _beta is an autoload shell function from " + "VIA/_beta"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			zsh.SetCompdumpForTest(t, 0, "")
			e := newCompdumpEnv(t)
			a, b := compdumpFixture(t)
			e.run(t, []string{a, b}, "")
			e.plant(t)
			if out := e.run(t, []string{a, b}, ""); !readTheDump(out) {
				t.Fatalf("the unchanged control did not read the dump:\n%s", out)
			}
			dirs := tc.change(t, a, b)
			// The last directory's own spelling, so the row reached through
			// a link can name it.
			via := func(out string) string { return strings.ReplaceAll(out, dirs[len(dirs)-1]+"/", "VIA/") }
			got := via(e.run(t, dirs, ""))
			if readTheDump(got) {
				t.Fatalf("after %s the stale dump was read:\n%s", tc.name, got)
			}
			if !hasRow(got, tc.want) {
				t.Errorf("after %s, %q is not in the tables:\n%s", tc.name, tc.want, got)
			}
			if scan := via(e.run(t, dirs, "-D")); got != scan {
				t.Errorf("the rescan disagrees with -D\nrescan:\n%s\n-D:\n%s", got, scan)
			}
			// And the rescan wrote a dump the next start reads.
			e.plant(t)
			if out := e.run(t, dirs, ""); !readTheDump(out) {
				t.Errorf("the dump the rescan wrote was not read back")
			}
		})
	}
}

// TestCompinitKeepsTheDumpForAFileRewrittenTheSame: a file renamed over with
// the same first line — what oh-my-zsh's kubectl plugin does to `_kubectl` on
// every start — has a new inode and new times and the same scan, so the dump
// is still read; and a file given a new body under the same first line is
// the same. The control beside them changes the first line and is a scan.
func TestCompinitKeepsTheDumpForAFileRewrittenTheSame(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  string
		reads bool
	}{
		{"renamed over, same bytes", "#compdef beta gamma\n", true},
		{"same first line, another body", "#compdef beta gamma\nprint a body that is new\n", true},
		{"another first line", "#compdef beta gammb\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			zsh.SetCompdumpForTest(t, 0, "")
			e := newCompdumpEnv(t)
			a, b := compdumpFixture(t)
			dirs := []string{a, b}
			e.run(t, dirs, "")
			e.plant(t)
			target := filepath.Join(b, "_beta")
			before, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			fresh := filepath.Join(b, ".fresh")
			if err := os.WriteFile(fresh, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(fresh, target); err != nil {
				t.Fatal(err)
			}
			after, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			if os.SameFile(before, after) {
				t.Fatal("the rename left the same file, so the stat did not move and the row tests nothing")
			}
			got := e.run(t, dirs, "")
			if readTheDump(got) != tc.reads {
				t.Errorf("read the dump: %v, want %v\n%s", readTheDump(got), tc.reads, got)
			}
			if scan := e.run(t, dirs, "-D"); strings.Replace(got, "c planted=_planted\n", "", 1) != scan {
				t.Errorf("the tables disagree with -D\ngot:\n%s\n-D:\n%s", got, scan)
			}
		})
	}
}

// TestCompinitRescansForAnotherBuild: a dump another build of the shell wrote
// is not read, since what a scan leaves depends on the interpreter too.
func TestCompinitRescansForAnotherBuild(t *testing.T) {
	e := newCompdumpEnv(t)
	a, b := compdumpFixture(t)
	dirs := []string{a, b}
	func() {
		zsh.SetCompdumpForTest(t, 0, "build one")
		e.run(t, dirs, "")
		e.plant(t)
		if out := e.run(t, dirs, ""); !readTheDump(out) {
			t.Fatalf("the same build did not read its dump")
		}
	}()
	zsh.SetCompdumpForTest(t, 0, "build two")
	if out := e.run(t, dirs, ""); readTheDump(out) {
		t.Errorf("another build read the dump")
	}
}

// TestCompinitFallsBackFromABadDump: a dump cut short, damaged, or not a dump
// at all is a full scan, which then writes a good one.
func TestCompinitFallsBackFromABadDump(t *testing.T) {
	for name, spoil := range map[string]func([]byte) []byte{
		"truncated":      func(d []byte) []byte { return d[:len(d)/2] },
		"one byte wrong": func(d []byte) []byte { d[len(d)/3] ^= 1; return d },
		"empty":          func([]byte) []byte { return nil },
		"not a dump":     func([]byte) []byte { return []byte("#compdef everything\n") },
	} {
		t.Run(name, func(t *testing.T) {
			zsh.SetCompdumpForTest(t, 0, "")
			e := newCompdumpEnv(t)
			a, b := compdumpFixture(t)
			dirs := []string{a, b}
			want := e.run(t, dirs, "-D")
			e.run(t, dirs, "")
			e.plant(t)
			data, err := os.ReadFile(e.file())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(e.file(), spoil(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := e.run(t, dirs, ""); got != want {
				t.Errorf("a %s dump gave\n%s\nwant the scan's\n%s", name, got, want)
			}
			e.plant(t) // fails the test if what was written back does not decode
		})
	}
}

// TestCompinitWritesNoDumpForAFileJustChanged: inside the racy window the
// start scans and leaves no dump, and outside it the same start writes one.
func TestCompinitWritesNoDumpForAFileJustChanged(t *testing.T) {
	e := newCompdumpEnv(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "_fresh"), []byte("#compdef fresh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	zsh.SetCompdumpForTest(t, time.Hour, "")
	if out := e.run(t, []string{dir}, ""); !strings.Contains(out, "c fresh=_fresh") {
		t.Fatalf("the scan did not run:\n%s", out)
	}
	if _, err := os.Stat(e.file()); !os.IsNotExist(err) {
		t.Errorf("a dump was written over a file changed inside the window: %v", err)
	}
	zsh.SetCompdumpForTest(t, 0, "")
	e.run(t, []string{dir}, "")
	if _, err := os.Stat(e.file()); err != nil {
		t.Errorf("no dump outside the window either, so the row above proves nothing: %v", err)
	}
}

// TestCompinitNeverTouchesZshsDump: neither the default `.zcompdump` nor a
// `-d` file is created, read or replaced, and `$_comp_dumpfile` still names
// zsh's.
func TestCompinitNeverTouchesZshsDump(t *testing.T) {
	zsh.SetCompdumpForTest(t, 0, "")
	e := newCompdumpEnv(t)
	a, b := compdumpFixture(t)
	named := filepath.Join(e.home, "named-dump")
	for _, opts := range []string{"", "-d " + named, "-C", "-i"} {
		e.run(t, []string{a, b}, opts)
	}
	for _, p := range []string{filepath.Join(e.home, ".zcompdump"), named} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s exists after compinit: %v", p, err)
		}
	}
	if _, err := os.Stat(e.file()); err != nil {
		t.Errorf("and the dump of our own was not written, so the row above proves nothing: %v", err)
	}
}

// TestCompinitMergesTheDumpOverTablesItFound: a second compinit, or one after
// a compdef, keeps what the tables held exactly as a scan would.
func TestCompinitMergesTheDumpOverTablesItFound(t *testing.T) {
	zsh.SetCompdumpForTest(t, 0, "")
	e := newCompdumpEnv(t)
	a, b := compdumpFixture(t)
	src := "fpath=(" + quoteDirs([]string{shippedFunctionDir(t), a, b}) + ")\nautoload -Uz compinit; compinit -u -D\n" +
		"compdef _mine alpha svc=mysvc mine; _services[other]=held; _patcomps['pat*']=_mypat\n" +
		"autoload -Uz compinit; compinit -u %s\n" + compdumpTables
	run := func(opts string) string {
		out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{
			Dir:  e.home,
			Vars: map[string]string{"PATH": e.home, "HOME": e.home, "XDG_CACHE_HOME": e.cache},
		}, strings.Replace(src, "%s", opts, 1))
		if err != nil || st != 0 {
			t.Fatalf("status %d, %v\n%s", st, err, out)
		}
		return out
	}
	want := run("-D")
	run("")
	got := run("")
	e.plant(t)
	if !readTheDump(run("")) {
		t.Fatal("the second compinit did not read the dump, so the comparison above is scan against scan")
	}
	if got != want {
		t.Errorf("merging the dump\n%s\nwant the scan's\n%s", got, want)
	}
	for _, row := range []string{"c alpha=_mine", "s svc=mysvc", "c other=_svc", "s other=held", "p 'pat*'=_mypat", "c gamma=_beta"} {
		if !hasRow(want, row) {
			t.Errorf("%q is not in the scan's tables, so the merge is not being exercised:\n%s", row, want)
		}
	}
}

// TestCompinitDumpSurvivesShellsStartingTogether: many shells with one cache
// start at once, each scanning and writing; every one gets the scan's tables
// and the dump left behind is whole.
func TestCompinitDumpSurvivesShellsStartingTogether(t *testing.T) {
	zsh.SetCompdumpForTest(t, 0, "")
	e := newCompdumpEnv(t)
	a, b := compdumpFixture(t)
	dirs := []string{a, b}
	want := e.run(t, dirs, "-D")
	const shells = 12
	outs := make([]string, shells)
	var wg sync.WaitGroup
	for i := range shells {
		wg.Go(func() { outs[i] = e.run(t, dirs, "") })
	}
	wg.Wait()
	for i, out := range outs {
		if out != want {
			t.Errorf("shell %d got\n%s\nwant\n%s", i, out, want)
		}
	}
	e.plant(t) // the dump decodes
	if entries, _ := os.ReadDir(filepath.Dir(e.file())); len(entries) != 1 {
		t.Errorf("the cache directory holds %d entries, want the dump alone: %v", len(entries), entries)
	}
}

// TestCompinitDumpMatchesTheScanOverTheRealFpath: over this machine's own
// completion library — the shipped functions and an installed zsh's, which
// is the default $fpath — the tables and stubs read from the dump are the
// ones the scan leaves, every one.
func TestCompinitDumpMatchesTheScanOverTheRealFpath(t *testing.T) {
	dirs := zsh.SystemFunctionDirectories("/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin")
	zsh.SetCompdumpForTest(t, 0, "")
	e := newCompdumpEnv(t)
	scan := e.run(t, dirs, "-D")
	if n := strings.Count(scan, "\nc "); n < 100 {
		t.Skipf("only %d completions on this machine's $fpath (%v); no installed zsh library to compare over", n, dirs)
	}
	e.run(t, dirs, "")
	data, err := os.ReadFile(e.file())
	if err != nil {
		t.Fatalf("no dump written: %v", err)
	}
	// The dump is read back: a probe name added to it under the same key
	// arrives, and with it taken out the rest is the scan's, line for line.
	e.plant(t)
	got := e.run(t, dirs, "")
	if !readTheDump(got) {
		t.Fatal("the dump was not read back")
	}
	got = strings.Replace(got, "c planted=_planted\n", "", 1)
	if got != scan {
		gl, sl := strings.Split(got, "\n"), strings.Split(scan, "\n")
		t.Errorf("dump and scan disagree over the real $fpath: %d lines against %d", len(gl), len(sl))
		for i := 0; i < len(gl) && i < len(sl); i++ {
			if gl[i] != sl[i] {
				t.Errorf("first difference at line %d:\ndump: %s\nscan: %s", i, gl[i], sl[i])
				break
			}
		}
	}
	t.Logf("%d lines compared over %v; dump %d bytes", strings.Count(scan, "\n"), dirs, len(data))
}

// TestCompinitAsksTheGateBeforeItsDump: the dump is a file the shell reads and
// writes on the script's behalf, at a path the script's own `$XDG_CACHE_HOME`
// chose, so a policy refusing the path refuses the dump — the start scans and
// nothing is written — and the same start under a gate that allows it writes
// one, so the refusal is not the dump failing for some other reason.
func TestCompinitAsksTheGateBeforeItsDump(t *testing.T) {
	zsh.SetCompdumpForTest(t, 0, "")
	a, b := compdumpFixture(t)
	for _, deny := range []bool{true, false} {
		e := newCompdumpEnv(t)
		var mu sync.Mutex
		var asked []string
		gate := interp.GateFunc(func(_ context.Context, act interp.Action) interp.Decision {
			if strings.HasPrefix(act.Path, e.cache) {
				mu.Lock()
				asked = append(asked, act.Path)
				mu.Unlock()
				if deny {
					return interp.Deny
				}
			}
			return interp.Allow
		})
		var buf bytes.Buffer
		r := preset.Runner(dialecttest.Base{
			Dir: e.home, Stdout: &buf, Stderr: &buf,
			Vars: map[string]string{"PATH": e.home, "HOME": e.home, "XDG_CACHE_HOME": e.cache},
		})
		r.Gate = gate
		r.SourcingPrelude(true)
		if _, err := r.Run(context.Background(), preset.Parse(t, zsh.Prelude())); err != nil {
			t.Fatal(err)
		}
		r.SourcingPrelude(false)
		src := "fpath=(" + quoteDirs([]string{shippedFunctionDir(t), a, b}) + ")\nautoload -Uz compinit; compinit -u\nprint -r -- $_comps[gamma]"
		if st, err := r.Run(context.Background(), preset.Parse(t, src)); err != nil || st != 0 || buf.String() != "_beta\n" {
			t.Fatalf("deny=%v: status %d, %v\n%s", deny, st, err, buf.String())
		}
		_, statErr := os.Stat(e.file())
		switch {
		case len(asked) == 0:
			t.Errorf("deny=%v: the gate was never asked about the dump", deny)
		case deny && !os.IsNotExist(statErr):
			t.Errorf("a refused dump was written anyway: %v (asked about %v)", statErr, asked)
		case !deny && statErr != nil:
			t.Errorf("an allowed dump was not written, so the refusal above proves nothing: %v", statErr)
		}
	}
}

// TestCompdumpIsNotWrittenOverAFileThatMovedDuringTheScan: the stat snapshot
// is taken before the scan and checked again at the write, so a file edited
// in between — what the scan read is then unknown — leaves no dump. The
// control is the same calls with nothing edited, which writes one.
func TestCompdumpIsNotWrittenOverAFileThatMovedDuringTheScan(t *testing.T) {
	for _, edit := range []bool{false, true} {
		zsh.SetCompdumpForTest(t, 0, "")
		e := newCompdumpEnv(t)
		a, _ := compdumpFixture(t)
		src := "local -a autoloads=(x) autoloadx comps services patcomps postpatcomps mainc=(0)\n" +
			"__compinit_dump key k 1 " + quoteDirs([]string{a}) + "\n"
		if edit {
			src += "print '#compdef moved' > " + quoteDirs([]string{filepath.Join(a, "_alpha")}) + "\n"
		}
		src += "__compinit_dump save $k 1 autoloads autoloadx comps services patcomps postpatcomps mainc -- " +
			quoteDirs([]string{a}) + "; print st=$? ${#k}"
		out, _, err := preset.CombinedWithPrelude(t, dialecttest.Base{
			Dir:  e.home,
			Vars: map[string]string{"PATH": e.home, "HOME": e.home, "XDG_CACHE_HOME": e.cache},
		}, src)
		if err != nil {
			t.Fatal(err)
		}
		_, statErr := os.Stat(e.file())
		if want := map[bool]string{false: "st=0 64\n", true: "st=1 64\n"}[edit]; out != want {
			t.Errorf("edit=%v: %q, want %q", edit, out, want)
		}
		if edit != os.IsNotExist(statErr) {
			t.Errorf("edit=%v: the dump's presence is %v", edit, statErr)
		}
	}
}
