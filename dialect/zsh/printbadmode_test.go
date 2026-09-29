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
//
// **And the sentence is the whole of what is said.** It used to be graded with
// a Contains, which cannot see a line added beside the one it looks for — a
// `print` that named the mode *and* went on to record the failed write, so
// that the shell added `write error: bad file descriptor` underneath, passed
// it unchanged. The number the descriptor was opened at is asked for rather
// than assumed, because the number this shell hands out is not the reference's
// and is residue on #4436 in its own right.
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
print "read=[$line]"
print "fd=$ro"`)
	if st != 0 {
		t.Fatalf("the snippet itself failed: status %d, %q / %q", st, out, errs)
	}
	fd, ok := strings.CutPrefix(strings.TrimSuffix(out, "\n"), "st=1\nread=[abcdef]\nfd=")
	if !ok || fd == "" {
		t.Fatalf("print -u on a read-only descriptor = %q, want st=1, the text unread and the descriptor named", out)
	}
	if want := "bad mode on fd " + fd + "\n"; !strings.HasSuffix(errs, want) ||
		strings.Count(errs, "\n") != 1 {
		t.Errorf("said %q, want one line ending %q", errs, want)
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
