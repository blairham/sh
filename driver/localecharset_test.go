// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"path/filepath"
	"testing"
)

// bsdLocale writes a BSD-layout locale: an LC_CTYPE whose header names the
// codeset, and the other categories as empty files when whole.
func bsdLocale(t *testing.T, root, name, codeset string, whole bool) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	head := append([]byte("RuneMagB"), codeset...)
	head = append(head, make([]byte, 24)...)
	if err := os.WriteFile(filepath.Join(dir, "LC_CTYPE"), head, 0o644); err != nil {
		t.Fatal(err)
	}
	if whole {
		for _, c := range localeCategories {
			if c == "LC_CTYPE" {
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, c), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestTheBSDLocaleLayoutIsReadOffTheData(t *testing.T) {
	root := t.TempDir()
	bsdLocale(t, root, "en_US", "UTF-8", true)
	bsdLocale(t, root, "fr_FR.ISO8859-1", "NONE:ISO8859-1", true)
	bsdLocale(t, root, "UTF-8", "UTF-8", false)
	bsdLocale(t, root, "C.UTF-8", "UTF-8", false)
	if err := os.MkdirAll(filepath.Join(root, "junk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "junk", "LC_CTYPE"), []byte("not a locale"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		whole   bool
		codeset string
		ok      bool
	}{
		{"en_US", true, "UTF-8", true},
		{"en_US", false, "UTF-8", true},
		{"fr_FR.ISO8859-1", true, "NONE:ISO8859-1", true},
		{"UTF-8", false, "UTF-8", true},
		{"UTF-8", true, "", false},
		// The one name macOS loads whole from an LC_CTYPE alone.
		{"C.UTF-8", true, "UTF-8", true},
		{"C.UTF-8", false, "UTF-8", true},
		{"xx_XX.UTF-8", true, "", false},
		{"junk", false, "", false},
		{"../" + filepath.Base(root) + "/en_US", false, "", false},
	} {
		cs, ok := localeCharsetIn(root, filepath.Join(root, "nowhere"), "darwin", tc.name, tc.whole)
		if cs != tc.codeset || ok != tc.ok {
			t.Errorf("%q whole=%v: got %q %v, want %q %v", tc.name, tc.whole, cs, ok, tc.codeset, tc.ok)
		}
	}
}

func TestTheGlibcLocaleLayoutIsFoundByEitherSpelling(t *testing.T) {
	glibc := t.TempDir()
	if err := os.MkdirAll(filepath.Join(glibc, "de_DE.utf8"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(glibc, "de_DE.utf8", "LC_CTYPE"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	archive := []byte("\x00\x00fr_FR.iso88591\x00ja_JP.eucjp\x00")
	if err := os.WriteFile(filepath.Join(glibc, "locale-archive"), archive, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, codeset string
		ok            bool
	}{
		{"de_DE.UTF-8", "UTF-8", true},
		{"de_DE.utf8", "utf8", true},
		{"fr_FR.ISO-8859-1", "ISO-8859-1", true},
		{"ja_JP.EUC-JP", "EUC-JP", true},
		{"C.UTF-8", "UTF-8", true},
		{"C.utf8", "utf8", true},
		{"en_US.UTF-8", "", false},
		{"fr_FR", "", false},
	} {
		cs, ok := localeCharsetIn(t.TempDir(), glibc, "linux", tc.name, true)
		if cs != tc.codeset || ok != tc.ok {
			t.Errorf("%q: got %q %v, want %q %v", tc.name, cs, ok, tc.codeset, tc.ok)
		}
	}
}

// musl has no locale data to refuse a name with, so every name loads as UTF-8 —
// measured with BusyBox ash on alpine, where even xx_XX.ISO8859-1 is UTF-8.
func TestWithNoGlibcLocaleTreeEveryNameIsUTF8(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "none")
	for _, name := range []string{"xx_XX.UTF-8", "xx_XX.ISO8859-1", "en_US"} {
		if cs, ok := localeCharsetIn(missing, missing, "linux", name, true); cs != "UTF-8" || !ok {
			t.Errorf("%q: got %q %v, want UTF-8 true", name, cs, ok)
		}
	}
}
