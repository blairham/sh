// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// hostLocaleCharset is interp.Runner.LocaleCharset for a binary that is a
// shell: whether this machine's C library would load a locale name, read off
// the data it would load, and the codeset that data declares. See
// interp/localeloaded.go for the measurements.
//
// Three layouts, decided by what is on the machine rather than by a build tag,
// so that a test can point it at a fixture:
//
//   - BSD and macOS: `/usr/share/locale/<name>/LC_<category>`. The LC_CTYPE
//     file begins `RuneMag` and a version byte, and the codeset is the
//     NUL-terminated name after them (`UTF-8`, `NONE:ISO8859-1`, `EUC-JP`).
//   - glibc: `/usr/lib/locale/<name>/LC_<category>`, under the name as written
//     or its normalized spelling (`en_US.utf8`), or the same normalized name
//     in `/usr/lib/locale/locale-archive`. The codeset is the name's. `C.UTF-8`
//     is built in.
//   - musl, which has no data: every name loads, as UTF-8.
var localeRoots = struct{ bsd, glibc string }{"/usr/share/locale", "/usr/lib/locale"}

func hostLocaleCharset(name string, whole bool) (string, bool) {
	return localeCharsetIn(localeRoots.bsd, localeRoots.glibc, runtime.GOOS, name, whole)
}

// localeCategories are the files a whole locale has to have.
var localeCategories = []string{"LC_COLLATE", "LC_CTYPE", "LC_MONETARY", "LC_NUMERIC", "LC_TIME"}

func localeCharsetIn(bsd, glibc, goos, name string, whole bool) (string, bool) {
	if strings.ContainsRune(name, '/') || name == "." || name == ".." {
		return "", false
	}
	if goos != "linux" {
		dir := filepath.Join(bsd, name)
		if whole {
			for _, c := range localeCategories {
				if _, err := os.Stat(filepath.Join(dir, c)); err != nil {
					return "", false
				}
			}
		}
		b, err := os.ReadFile(filepath.Join(dir, "LC_CTYPE"))
		if err != nil || len(b) < 8 || !bytes.HasPrefix(b, []byte("RuneMag")) {
			return "", false
		}
		cs := b[8:]
		if i := bytes.IndexByte(cs, 0); i >= 0 {
			cs = cs[:i]
		}
		return string(cs), true
	}
	codeset := localeCodesetOf(name)
	if isCUTF8(name) {
		return codeset, true
	}
	if _, err := os.Stat(glibc); err != nil {
		// No glibc locale tree at all: musl, which loads any name as UTF-8.
		return "UTF-8", true
	}
	for _, n := range []string{name, normalizedLocaleName(name)} {
		if _, err := os.Stat(filepath.Join(glibc, n, "LC_CTYPE")); err == nil {
			return codeset, true
		}
	}
	if archive, err := os.ReadFile(filepath.Join(glibc, "locale-archive")); err == nil {
		if bytes.Contains(archive, []byte("\x00"+normalizedLocaleName(name)+"\x00")) {
			return codeset, true
		}
	}
	return "", false
}

func localeCodesetOf(name string) string {
	if i := strings.IndexByte(name, '@'); i >= 0 {
		name = name[:i]
	}
	if i := strings.IndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return ""
}

func isCUTF8(name string) bool {
	n := normalizedLocaleName(name)
	return n == "C.utf8"
}

// normalizedLocaleName is glibc's spelling of a name: the codeset lowercased
// with its punctuation taken out, so `en_US.UTF-8` is `en_US.utf8`.
func normalizedLocaleName(name string) string {
	mod := ""
	if i := strings.IndexByte(name, '@'); i >= 0 {
		name, mod = name[:i], name[i:]
	}
	i := strings.IndexByte(name, '.')
	if i < 0 {
		return name + mod
	}
	var b strings.Builder
	for _, c := range []byte(name[i+1:]) {
		switch {
		case c >= 'A' && c <= 'Z':
			b.WriteByte(c + 'a' - 'A')
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteByte(c)
		}
	}
	return name[:i+1] + b.String() + mod
}
