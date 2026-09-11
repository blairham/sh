// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
)

// The file operators, in `[[ ]]` and in `test` alike. Every answer here is
// unanimous across the panel except the two that are axes — a missing file in
// `-nt`/`-ot`, and a non-numeric `-t` operand — and those are asked exactly
// at the disagreement.

// fileCondRun runs src with the runner's directory pinned to dir, so relative
// operands resolve against the runner rather than the process.
func fileCondRun(t *testing.T, dir, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := CoreSemantics()
		if set != nil {
			set(&sem)
		}
		r.Semantics = &sem
		r.Dir = dir
	})
}

func fileCondStatus(t *testing.T, dir, src string) int {
	t.Helper()
	_, st := fileCondRun(t, dir, src, nil)
	return st
}

// TestFileKindOperatorsAskTheFilesystem — `-p`, `-S`, `-b`, `-c` classify a
// file by what it is, and a plain file is none of them. The one portable
// positive is `-c /dev/null`, a character device on every Unix.
func TestFileKindOperatorsAskTheFilesystem(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "reg"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`[[ -p fifo ]]`, 0},
		{`[[ -p reg ]]`, 1},
		{`[[ -S reg ]]`, 1},
		{`[[ -b reg ]]`, 1},
		{`[[ -c reg ]]`, 1},
		{`[[ -c /dev/null ]]`, 0},
		{`[[ -b /dev/null ]]`, 1},
		{`[[ -p missing ]]`, 1},
		// The same questions through the builtin, which shares the code.
		{`test -p fifo`, 0},
		{`test -S reg`, 1},
		{`test -c /dev/null`, 0},
	} {
		if got := fileCondStatus(t, dir, tc.src); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.src, got, tc.want)
		}
	}
}

// TestPermissionBitOperators — `-g`, `-u` and `-k` read the setgid, setuid
// and sticky bits, which an ordinary file does not have.
func TestPermissionBitOperators(t *testing.T) {
	dir := t.TempDir()
	for name, mode := range map[string]os.FileMode{
		"plain":  0o600,
		"setgid": 0o600 | os.ModeSetgid,
		"setuid": 0o600 | os.ModeSetuid,
		"sticky": 0o600 | os.ModeSticky,
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`[[ -g setgid ]]`, 0},
		{`[[ -g plain ]]`, 1},
		{`[[ -u setuid ]]`, 0},
		{`[[ -u plain ]]`, 1},
		{`[[ -k sticky ]]`, 0},
		{`[[ -k plain ]]`, 1},
		{`[[ -g missing ]]`, 1},
	} {
		if got := fileCondStatus(t, dir, tc.src); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.src, got, tc.want)
		}
	}
}

// newerOlderDir lays out the fixtures the comparison tests share: `old` and
// `new` a year apart, `even1` and `even2` with identical times, and `a` with
// a hard link `b` and an unrelated `c`.
func newerOlderDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for name, when := range map[string]time.Time{
		"old":   old,
		"new":   old.AddDate(1, 0, 0),
		"even1": old,
		"even2": old,
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"a", "c"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Link(filepath.Join(dir, "a"), filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestNewerOlderAndSameFile — with both files present the comparisons are
// unanimous, including that equal times are neither newer nor older, and
// `-ef` is identity rather than equality.
func TestNewerOlderAndSameFile(t *testing.T) {
	dir := newerOlderDir(t)
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`[[ new -nt old ]]`, 0},
		{`[[ old -nt new ]]`, 1},
		{`[[ old -ot new ]]`, 0},
		{`[[ new -ot old ]]`, 1},
		{`[[ even1 -nt even2 ]]`, 1},
		{`[[ even1 -ot even2 ]]`, 1},
		{`[[ a -ef b ]]`, 0},
		{`[[ a -ef a ]]`, 0},
		{`[[ a -ef c ]]`, 1},
		{`[[ missing -ef a ]]`, 1},
		// The builtin gives the same answers from the same code.
		{`test new -nt old`, 0},
		{`test old -ot new`, 0},
		{`test a -ef b`, 0},
		{`test a -ef c`, 1},
		// And past four arguments, where the grammar takes over.
		{`test new -nt old -a a -ef b`, 0},
	} {
		if got := fileCondStatus(t, dir, tc.src); got != tc.want {
			t.Errorf("%s = %d, want %d", tc.src, got, tc.want)
		}
	}
}

// TestMissingFileIsOlderIsAnAxis — the one disagreement in the comparisons:
// whether a file that does not exist counts as older than one that does. The
// mirrored cases — a missing file being newer — are false everywhere and must
// not ask.
func TestMissingFileIsOlderIsAnAxis(t *testing.T) {
	dir := newerOlderDir(t)
	for _, src := range []string{
		`[[ new -nt missing ]]`, `[[ missing -ot new ]]`,
		`test new -nt missing`, `test missing -ot new`,
	} {
		for answer, want := range map[Answer]int{Yes: 0, No: 1} {
			out, st := fileCondRun(t, dir, src, func(s *Semantics) {
				s.MissingFileIsOlder = answer
			})
			if st != want || out != "" {
				t.Errorf("%s with %v = %d %q, want %d and silence", src, answer, st, out, want)
			}
		}
		// Unanswered is refused, naming the disagreement.
		out, st := fileCondRun(t, dir, src, nil)
		if st != 2 || !strings.Contains(out, "no dialect was chosen") {
			t.Errorf("%s unanswered = %d %q, want the refusal at 2", src, st, out)
		}
	}
	// The mirrored cases and the both-missing cases are unanimous, so they
	// answer without the axis.
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`[[ missing -nt new ]]`, 1},
		{`[[ new -ot missing ]]`, 1},
		{`[[ no1 -nt no2 ]]`, 1},
		{`[[ no1 -ot no2 ]]`, 1},
	} {
		out, st := fileCondRun(t, dir, tc.src, nil)
		if st != tc.want || out != "" {
			t.Errorf("%s = %d %q, want %d without a question", tc.src, st, out, tc.want)
		}
	}
}

// TestTerminalTestIsFalseForStreamsThatAreNotFiles — a Runner an embedder
// built holds buffers, which are not descriptors the kernel has heard of, so
// `-t` on any of them is false. The same answer the panel gives a shell whose
// streams are pipes, and the answer this shell gave *every* descriptor until
// #1967 — see interp/terminaltest_test.go for the half that says the fixed
// false is gone.
func TestTerminalTestIsFalseForStreamsThatAreNotFiles(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []string{`[[ -t 0 ]]`, `[[ -t 1 ]]`, `[[ -t 99 ]]`, `test -t 0`, `test -t 99`} {
		out, st := fileCondRun(t, dir, src, nil)
		if st != 1 || out != "" {
			t.Errorf("%s = %d %q, want a silent 1 without a question", src, st, out)
		}
	}
}

// TestTerminalTestRequiresANumberIsAnAxis — a `-t` operand that is not a
// number splits the panel between an integer complaint at 2 and a plain
// false at 1.
func TestTerminalTestRequiresANumberIsAnAxis(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []string{`[[ -t x ]]`, `[[ -t "" ]]`, `test -t x`} {
		out, st := fileCondRun(t, dir, src, func(s *Semantics) {
			s.TerminalTestRequiresANumber = Yes
		})
		if st != 2 || !strings.Contains(out, "integer") {
			t.Errorf("%s with Yes = %d %q, want the integer complaint at 2", src, st, out)
		}
		out, st = fileCondRun(t, dir, src, func(s *Semantics) {
			s.TerminalTestRequiresANumber = No
		})
		if st != 1 || out != "" {
			t.Errorf("%s with No = %d %q, want a silent 1", src, st, out)
		}
	}
}
