// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/wild"
)

// The whole risk of this instrument is concentrated in the fetch, and it is
// not a wrong number. bash's suite is GPLv3, this tree is Apache-2.0, and
// NOTICE says there is no third-party code here. So these tests are about
// what reaches the disk, not about what the report says.

// archive builds a gzipped tar with the members named, so nothing here needs
// a network.
func archive(t *testing.T, members map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, body := range members {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if body == "" {
			hdr.Typeflag, hdr.Mode = tar.TypeDir, 0o755
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// sample is a suite shaped like bash's: a test directory, expected output
// beside it that must never land, three C helpers kept elsewhere in the
// distribution, and a shell implementation that must not be unpacked at all.
func sample() (Suite, map[string]string) {
	return Suite{
			Name:     "sample",
			Dialect:  "bash",
			Version:  "1.0",
			Root:     "sample-1.0",
			Include:  []string{"tests/", "support/recho.c"},
			Exclude:  []string{".right"},
			TestDir:  "tests",
			Ext:      ".tests",
			ShellVar: "THIS_SH",
		}, map[string]string{
			"sample-1.0/tests/a.tests":   "echo a\n",
			"sample-1.0/tests/a.right":   "a\n",
			"sample-1.0/tests/lib.sub":   "echo sub\n",
			"sample-1.0/support/recho.c": "int main(void){return 0;}\n",
			"sample-1.0/shell.c":         "/* the implementation nobody here may read */\n",
			"sample-1.0/builtins/echo.c": "/* nor this */\n",
		}
}

func TestUnpackWritesTheSuiteAndNothingElse(t *testing.T) {
	s, members := sample()
	dir := t.TempDir()
	if err := unpack(archive(t, members), s, dir); err != nil {
		t.Fatal(err)
	}
	want := []string{"tests/a.tests", "tests/lib.sub", "support/recho.c"}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("the suite needs %s and it was not unpacked: %v", rel, err)
		}
	}
	// The shell's own source is the thing this must never put on the disk,
	// and the expected output is the thing the measurement must never reach
	// for. Both are denials by construction rather than by discipline.
	for _, rel := range []string{"shell.c", "builtins/echo.c", "tests/a.right"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s reached the disk; nothing outside the suite may", rel)
		}
	}
}

func TestWantedRefusesAMemberThatClimbs(t *testing.T) {
	s, _ := sample()
	for _, name := range []string{
		"sample-1.0/tests/../../escape.tests",
		"/etc/passwd",
		"other-1.0/tests/a.tests",
	} {
		if rel, ok := s.Wanted(name); ok {
			t.Errorf("%s was accepted as %s; an archive is somebody else's bytes", name, rel)
		}
	}
}

func TestFetchRefusesAnArchiveThatIsNotThePin(t *testing.T) {
	s, members := sample()
	body := archive(t, members)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	s.URL, s.SHA256 = srv.URL, digest([]byte("a different release entirely"))

	build := t.TempDir()
	if _, err := Fetch(context.Background(), s, build, false); !errors.Is(err, ErrDigest) {
		t.Fatalf("want ErrDigest, got %v", err)
	}
	// And nothing was written. A run that graded whatever arrived would move
	// every number with nothing in the report saying so.
	if _, err := os.Stat(s.Dir(build)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an archive that failed the pin left a tree behind: %v", err)
	}
}

func TestFetchReportsAnAbsentServerAsOffline(t *testing.T) {
	s, _ := sample()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	srv.Close()
	s.URL = srv.URL
	if _, err := Fetch(context.Background(), s, t.TempDir(), false); !errors.Is(err, ErrOffline) {
		t.Fatalf("want ErrOffline, got %v", err)
	}
}

func TestFetchKeepsATreeThatCameFromThePin(t *testing.T) {
	s, members := sample()
	body := archive(t, members)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	s.URL, s.SHA256 = srv.URL, digest(body)

	build := t.TempDir()
	ctx := context.Background()
	if _, err := Fetch(ctx, s, build, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Fetch(ctx, s, build, false); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("fetched %d times; an unpacked tree that matches the pin is the same tree", hits)
	}
	if _, err := Fetch(ctx, s, build, true); err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Errorf("-refetch fetched %d times, want 2", hits)
	}
}

// TestFetchLandsWhereMakeWildRefusesToLook pins the interaction with the one
// guard that stands between a fetched suite and a person reading it.
//
// Gitignoring keeps a suite out of a commit. It does nothing about a grep, an
// editor's index or an agent's search, and `make wild` is the thing in this
// tree that walks directories looking for shell to read. Its denial had a
// carve-out for this project's own working tree until #1039 — which is to say
// the fetch's own destination was the one place on the machine the rule did
// not apply. This asserts the destination is refused, so that a change to
// either side has to face the other.
func TestFetchLandsWhereMakeWildRefusesToLook(t *testing.T) {
	own := filepath.Join(t.TempDir(), "checkout")
	for _, s := range Panel {
		if s.Root == "" {
			continue
		}
		dir := s.Dir(filepath.Join(own, "build"))
		if reason := wild.DeniedDir(dir, own); reason == "" {
			t.Errorf("%s unpacks into %s and the sweep would read it", s.Name, dir)
		}
	}
}
