// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `zsh/mapfile`, measured 2026-09-12 against zsh 5.9.2 under `zsh -f`.
//
// The gate is asserted from outside, against the shipped binary, in
// `cmd/sh/sandboxmapfile_test.go` — a policy is a thing a person passes to a
// program, so a test that built one in-process would grade the wiring it
// wrote rather than the wiring that ships. What is here is the other half:
// what the module *does* when nothing is watching, which is the half that
// decides whether the rows in `make sandbox` are measuring anything at all.

// mapfileIn runs src in a scratch directory and returns everything it wrote,
// stdout and stderr together, so that a diagnostic is asserted as the line it
// is rather than as a substring.
func mapfileIn(t *testing.T, dir, src string) string {
	t.Helper()
	out, _, errs := runZshSplit(t, dir, "zmodload zsh/mapfile\n"+src)
	return out + errs
}

// A key is a path and the value is that file's bytes, exactly.
//
// The trailing newline is the case worth pinning: a file holding `one\ntwo\n`
// reads back with its final newline intact, which is what makes
// `${(f)mapfile[f]}` split into *three* fields with an empty last one. A
// module that trimmed would give two, and every caller counting lines would
// be off by one in the direction that looks correct.
func TestAMapfileKeyReadsTheWholeFileVerbatim(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "two"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := mapfileIn(t, dir, `printf "[%s]" "${mapfile[two]}"`), "[one\ntwo\n]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := mapfileIn(t, dir, `a=("${(f)mapfile[two]}"); echo $#a`), "3\n"; got != want {
		t.Errorf("field count = %q, want %q", got, want)
	}
}

// A key that is not a readable file is empty and **not present**, which is
// one answer covering three states: nothing there, a directory, and a name
// the process may not read.
//
// `${+mapfile[k]}` is what separates empty-and-present from absent, and it is
// the reason this is asserted rather than assumed — a module that reported
// every key as present would give `${mapfile[nosuch]:-fallback}` the empty
// string where zsh gives the fallback.
func TestAMapfileKeyThatIsNotAReadableFileIsAbsent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"nosuch", "sub"} {
		if got, want := mapfileIn(t, dir, `echo "[${mapfile[`+key+`]}]" ${+mapfile[`+key+`]}`), "[] 0\n"; got != want {
			t.Errorf("%s: got %q, want %q", key, got, want)
		}
		if got, want := mapfileIn(t, dir, `echo ${mapfile[`+key+`]:-fallback}`), "fallback\n"; got != want {
			t.Errorf("%s: default = %q, want %q", key, got, want)
		}
	}
}

// An assignment writes the value as given: no newline is appended, and an
// existing file is truncated rather than extended.
//
// The mode is `0666` before the umask, which is what zsh leaves behind — the
// assertion is on the bits the process's own umask permits rather than on
// `0644`, since a test that hardcoded the usual answer would fail for anyone
// whose umask is not `022` and would be asserting the umask rather than the
// module.
func TestAMapfileAssignmentWritesTheValueExactly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mapfileIn(t, dir, `mapfile[made]=abc`)
	data, err := os.ReadFile(filepath.Join(dir, "made"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "abc"; got != want {
		t.Errorf("contents = %q, want %q — a newline was added", got, want)
	}
	info, err := os.Stat(filepath.Join(dir, "made"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got&^os.FileMode(0o666) != 0 {
		t.Errorf("mode = %#o, want no bit outside 0666", got)
	}

	// And the same name written again is replaced, not extended.
	mapfileIn(t, dir, `mapfile[made]="longer than before"`+"\n"+`mapfile[made]=hi`)
	data, err = os.ReadFile(filepath.Join(dir, "made"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "hi"; got != want {
		t.Errorf("contents = %q, want %q — the file was not truncated", got, want)
	}
}

// `mapfile[p]+=v` joins what the file already holds.
//
// This is the case that found the core bug #2260 fixes. An element append
// read the *stored* table to find the value it was joining onto, and a
// produced association has no stored table — so every append on one joined
// onto nothing and silently replaced what it meant to extend. It looked
// exactly like a correct append of an empty original, which is why nothing
// noticed: `functions[f]+=…` had been discarding the function body it was
// extending for as long as the seam has existed.
func TestAMapfileAppendJoinsWhatTheFileHolds(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got := mapfileIn(t, dir, `mapfile[f]=a`+"\n"+`mapfile[f]+=b`+"\n"+`printf "[%s]" "${mapfile[f]}"`)
	if want := "[ab]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// `unset "mapfile[p]"` unlinks the file, and `unset mapfile` unlinks nothing.
//
// The pair is the whole assertion. An unset of one element is a write, so it
// removes what it names; an unset of the parameter is an ordinary parameter
// unset, and a module that treated it as "unset every element" would empty
// the working directory on a line that reads like housekeeping.
func TestUnsettingAMapfileElementUnlinksAndUnsettingTheParameterDoesNot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mapfileIn(t, dir, `unset "mapfile[a]"`)
	if _, err := os.Lstat(filepath.Join(dir, "a")); err == nil {
		t.Error("unset of an element left the file in place")
	}
	mapfileIn(t, dir, `unset mapfile`)
	if _, err := os.Lstat(filepath.Join(dir, "b")); err != nil {
		t.Errorf("unset of the parameter removed a file: %v", err)
	}
}

// The roster is the working directory's names, and it carries **no values**.
//
// Both halves are measured. `${(k)mapfile}` names what is in the directory,
// dotfiles included, and follows `cd`; and `for k v in "${(@kv)mapfile}"`
// prints an empty value for every one of them, which is why a read of the
// roster is a readdir rather than a readdir plus a read of everything it
// found. A view that filled the values in would answer a question zsh does
// not ask, at a read and a gate decision per file.
func TestTheMapfileRosterIsTheDirectoryAndCarriesNoValues(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", ".dot"} {
		if err := os.WriteFile(filepath.Join(sub, name), []byte("CONTENTS"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Sorted, because the order of an association is this shell's own
	// decision and not zsh's — see interp.AssocArray.keys, which sorts on the
	// grounds that no shell promises an order and a deterministic one is
	// worth having. `(o)` asks for the sort explicitly so the case is about
	// the membership.
	if got, want := mapfileIn(t, sub, `echo "[${(ok)mapfile}]"`), "[.dot a b]\n"; got != want {
		t.Errorf("roster = %q, want %q", got, want)
	}
	if got, want := mapfileIn(t, sub, `echo ${#mapfile}`), "3\n"; got != want {
		t.Errorf("count = %q, want %q", got, want)
	}
	if got := mapfileIn(t, sub, `for k v in "${(@kv)mapfile}"; do printf "<%s>" "$v"; done`); strings.Contains(got, "CONTENTS") {
		t.Errorf("values = %q: the roster read the files, which zsh does not", got)
	}
	// And it is the *shell's* directory rather than the process's, so `cd`
	// moves it. The process never changes directory — the core may not — so
	// a roster read off `os.Getwd` would answer with this test binary's own.
	if got, want := mapfileIn(t, sub, `cd ..`+"\n"+`echo "[${(ok)mapfile}]"`), "[sub]\n"; got != want {
		t.Errorf("after cd = %q, want %q", got, want)
	}
}

// The parameter describes as zsh's does, and that is two attributes rather
// than one.
//
// `hideval` keeps the value out of a listing and `hide` is what makes `local
// mapfile` inside a function an ordinary parameter. Recording them as one bit
// told a script switching on `${(t)…}` about the wrong letter once already
// (#2042), so the whole word is asserted rather than a substring.
func TestTheMapfileParameterDescribesTheWayZshDescribesIt(t *testing.T) {
	t.Parallel()
	got := mapfileIn(t, t.TempDir(), `echo ${(t)mapfile}`)
	if want := "association-hide-hideval-special\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
