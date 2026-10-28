// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/parsecache"
)

// TestAParseCacheIsUnobservable is zsh's test of the same name in this
// dialect's terms: a sourced file read with the cache off, through an empty
// cache and through the filled one writes the same bytes each time — line
// numbers, the call stack's files and lines, the xtrace prefix, a grammar an
// earlier line changed, aliases an earlier line defined, a here-document, and
// a parse failure after commands that ran.
func TestAParseCacheIsUnobservable(t *testing.T) {
	pad := strings.Repeat("# commentary, to make this file one the parse cache keeps\n", 80)
	cases := []struct{ name, lib, main string }{
		{
			"the call stack",
			"f() {\n  echo \"$LINENO ${BASH_SOURCE[0]##*/} ${BASH_LINENO[0]} ${FUNCNAME[0]}\"\n  caller 0 | sed 's#.*/##'\n}\nf\necho top $LINENO\n",
			". ./lib; f",
		},
		{"xtrace", "PS4='+${BASH_SOURCE##*/}:$LINENO> '\nset -x\nx=1\necho $x\nset +x\n", ". ./lib"},
		{"extglob", "shopt -s extglob\ncase ab in @(ab|cd)) echo matched;; esac\n", ". ./lib"},
		{"aliases", "shopt -s expand_aliases\nalias greet='echo hi:'\ngreet one\n", ". ./lib"},
		{"here-document", "cat <<EOF\nline $LINENO\nEOF\n", ". ./lib"},
		{"diagnostics", "echo before\nno-such-command-here\necho after\n", ". ./lib"},
		{"parse failure", "echo ran\nfi\necho never\n", ". ./lib; echo st=$?"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir, cache := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "lib"), []byte(pad+c.lib), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("XDG_CACHE_HOME", cache)
			run := func(on bool) (string, string, int, int64) {
				if on {
					t.Setenv("SH_NO_PARSE_CACHE", "")
				} else {
					t.Setenv("SH_NO_PARSE_CACHE", "1")
				}
				var out, errs bytes.Buffer
				sh := bashShell(&out, &errs)
				sh.Dir = dir
				served := parsecache.Served.Load()
				code := driver.MainArgs(sh, []string{"bash", "-c", c.main})
				return out.String(), errs.String(), code, parsecache.Served.Load() - served
			}
			out, errs, code, _ := run(false)
			if out == "" {
				t.Fatalf("the case printed nothing: %q", errs)
			}
			for _, name := range []string{"cold", "warm"} {
				gotOut, gotErrs, gotCode, served := run(true)
				if gotOut != out || gotErrs != errs || gotCode != code {
					t.Errorf("%s: %q %q %d, want %q %q %d", name, gotOut, gotErrs, gotCode, out, errs, code)
				}
				if name == "warm" && c.name != "parse failure" && served == 0 {
					t.Error("the warm run was not read from the cache")
				}
			}
		})
	}
}
