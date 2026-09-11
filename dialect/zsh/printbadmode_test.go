// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `print -u` to a descriptor that was opened for *reading* is refused by name.
//
// Measured 2026-09-10 on zsh 5.9.2 with no startup files, where `sysopen -u ro
// f` with no direction letter opens read-only:
//
//	$ printf 'abcdef\n' > f
//	$ zsh -c 'zmodload zsh/system; sysopen -u ro f; print -u $ro -- nope; echo $?'
//	zsh:print:1: bad mode on fd 3
//	1
//
// It is a different complaint from the one a number nothing is open at draws —
// `bad file number: 9` — and the difference is the whole point: one says the
// descriptor is not there, the other says it is there and will not take a
// write.
//
// Before this the write was attempted, the error discarded, and the answer was
// 0 with nothing written (#1751): a script writing to the wrong one of two
// descriptors it holds was told nothing and its bytes went nowhere.
//
// The file's contents are asserted afterwards as well as the sentence. The
// descriptor is read-only either way, so a `print` that reported the refusal
// *and* wrote would be a different bug that the status alone cannot see.
func TestPrintRefusesADescriptorOpenForReading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ro.txt")
	if err := os.WriteFile(path, []byte("abcdef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st, errs := runZshSplit(t, dir, `zmodload zsh/system
sysopen -u ro ro.txt
print -u $ro -- nope
print "st=$?"
read -u $ro line
print "read=[$line]"`)
	if st != 0 {
		t.Fatalf("the snippet itself failed: status %d, %q / %q", st, out, errs)
	}
	if want := "st=1\nread=[abcdef]\n"; out != want {
		t.Errorf("print -u on a read-only descriptor = %q, want %q", out, want)
	}
	if !strings.Contains(errs, "bad mode on fd ") {
		t.Errorf("said %q, want the mode named", errs)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != "abcdef\n" {
		t.Errorf("the file holds %q afterwards, want it untouched", after)
	}
}

// The neighboring refusal, which must not move: a number nothing is open at
// is `bad file number` and not a mode complaint. Measured as `zsh:print:1: bad
// file number: 9` at 1 in that shell.
func TestPrintStillRefusesADescriptorNothingIsOpenAt(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(), "print -u 9 x\nprint \"st=$?\"")
	if out != "st=1\n" || st != 0 {
		t.Errorf("print -u 9 = %q (status %d), want st=1", out, st)
	}
	if !strings.Contains(errs, "bad file number: 9") {
		t.Errorf("said %q, want the number named", errs)
	}
}
