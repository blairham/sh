// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package, because two of the three subjects are unexported halves of the
// seam: when two spellings of a name are one place, and the table of links the
// operating system itself installs.
package opened

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestPathAnswersForAFileAndNotForAPipe is the platform helper on its
// own, because the two answers it gives are read very differently by its
// caller: a path is checked and a pipe is waved through, and a pipe that
// answered with a path would deny every process substitution a shell makes.
func TestPathAnswersForAFileAndNotForAPipe(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(link)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	got, ok := Path(f)
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		if ok {
			t.Fatalf("Path answered %q on a platform with no way to ask", got)
		}
		return
	}
	if !ok {
		t.Fatal("Path had no answer for an ordinary file")
	}
	if filepath.Base(got) != "file" {
		t.Errorf("Path = %q, want the link's target", got)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	if got, ok := Path(reader); ok {
		t.Errorf("Path = %q for a pipe, want no name at all", got)
	}
}

// TestSamePlaceAbsorbsOnlyThePlatformsOwnSpelling. The suppression is what
// keeps a Mac from consulting the gate twice for every temporary file, and it
// is also the one place a hole could be hidden — so the neighbors are the
// cases worth writing down.
func TestSamePlaceAbsorbsOnlyThePlatformsOwnSpelling(t *testing.T) {
	if len(platformLinks) == 0 {
		t.Skip("no platform links here")
	}
	for _, tc := range []struct {
		name, requested, actual string
		want                    bool
	}{
		{"the link itself", "/tmp", "/private/tmp", true},
		{"a file under it", "/tmp/x", "/private/tmp/x", true},
		{"deeper", "/tmp/a/b/c", "/private/tmp/a/b/c", true},
		{"a link inside it went elsewhere", "/tmp/x", "/private/tmp/elsewhere", false},
		{"a link inside it left the tree", "/tmp/x", "/private/etc/passwd", false},
		{"a neighbor whose name starts the same way", "/tmpfoo/x", "/private/tmpfoo/x", false},
		{"not under any of them", "/srv/x", "/other/x", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := samePlace(tc.requested, tc.actual); got != tc.want {
				t.Errorf("samePlace(%q, %q) = %v, want %v", tc.requested, tc.actual, got, tc.want)
			}
		})
	}
}

// TestPlatformLinksAreWhatThisPlatformInstalls measures the claim rather than
// restating it. Every entry says the operating system ships a symbolic link
// at that path, unconditionally; if one ever stops being true the suppression
// built on it is a hole, and this is what fails.
func TestPlatformLinksAreWhatThisPlatformInstalls(t *testing.T) {
	for _, link := range platformLinks {
		info, err := os.Lstat(link[0])
		if err != nil {
			t.Errorf("%s: %v — the table claims this platform ships it", link[0], err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s is not a symbolic link on this machine", link[0])
			continue
		}
		target, err := os.Readlink(link[0])
		if err != nil {
			t.Fatal(err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(link[0]), target)
		}
		if target != link[1] {
			t.Errorf("%s points at %s, and the table says %s", link[0], target, link[1])
		}
	}
}
