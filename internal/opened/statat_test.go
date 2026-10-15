// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/blairham/sh/internal/opened"
)

// StatAt's FileInfo is decomposed by hand, so it is held against the os
// package's own decomposition of the same entries — a regular file with the
// special bits, a directory, a symbolic link asked about both ways, and a
// named pipe — field by field.
func TestStatAtAnswersWhatOsStatAnswers(t *testing.T) {
	if !opened.StatAtSupported {
		t.Skip("no fstatat on this platform")
	}
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "f"), []byte("hello"), 0o640))
	must(os.Chmod(filepath.Join(dir, "f"), 0o640|os.ModeSetgid|os.ModeSticky))
	must(os.Mkdir(filepath.Join(dir, "sub"), 0o750))
	must(os.Symlink("f", filepath.Join(dir, "l")))
	must(syscall.Mkfifo(filepath.Join(dir, "p"), 0o600))

	held, _, err := opened.HoldChecked(dir)
	must(err)
	defer func() { _ = held.Close() }()

	for _, name := range []string{"f", "sub", "l", "p", "."} {
		for _, follow := range []bool{true, false} {
			want, err := os.Stat(filepath.Join(dir, name))
			if !follow {
				want, err = os.Lstat(filepath.Join(dir, name))
			}
			must(err)
			got, err := opened.StatAt(held, name, follow)
			if err != nil {
				t.Errorf("StatAt(%q, %v): %v", name, follow, err)
				continue
			}
			if got.Mode() != want.Mode() || got.Size() != want.Size() ||
				!got.ModTime().Equal(want.ModTime()) || got.IsDir() != want.IsDir() {
				t.Errorf("StatAt(%q, follow=%v) = %v %d %v, os says %v %d %v", name, follow,
					got.Mode(), got.Size(), got.ModTime(), want.Mode(), want.Size(), want.ModTime())
			}
			gs, ok := got.Sys().(*syscall.Stat_t)
			ws, _ := want.Sys().(*syscall.Stat_t)
			if !ok || gs.Ino != ws.Ino || gs.Uid != ws.Uid {
				t.Errorf("StatAt(%q, follow=%v).Sys() is not the same stat", name, follow)
			}
		}
	}
	// The control: a name that is not there is an error, not a zero stat.
	if _, err := opened.StatAt(held, "nosuch", true); err == nil {
		t.Error("StatAt found a file that is not there")
	}
	if err := opened.AccessAt(held, "f", 4); err != nil {
		t.Errorf("AccessAt read on a 0640 file of ours: %v", err)
	}
	if err := opened.AccessAt(held, "f", 1); err == nil {
		t.Error("AccessAt execute on a 0640 file said yes")
	}
}
