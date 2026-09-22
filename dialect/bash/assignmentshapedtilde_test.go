// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A word that merely *looks* like an assignment is a tilde context here, so
// `make -k FOO=~/x` hands make the home directory. This shell alone: dash,
// zsh, ksh93, BusyBox ash and this same binary under the `sh` name all keep
// the two characters.
//
// Measured 2026-09-22, script files under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with `HOME=/usr/xyz`, each word passed to `echo`, on 5.3.20 and on the
// 3.2.57 macOS ships — which agree. See
// Semantics.AnAssignmentShapedArgumentIsATildeContextOutsidePosixMode (#4213).
//
// **The boundary rows are the test.** What decides is the *shape* of an
// assignment — a name, then an `=` — and not a word with an `=` in it, so a
// pass built only from the expanding rows would be satisfied by a shell that
// expanded a tilde after any `=` at all, which is nobody's.
func TestAnArgumentShapedLikeAnAssignmentIsATildeContext(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an ordinary argument", `echo make -k FOO=~/mumble`, "make -k FOO=/h/mumble"},
		{"the value being the whole tilde", `echo FOO=~`, "FOO=/h"},
		{"both sides of a colon", `echo foo=~:~`, "foo=/h:/h"},
		{"a tilde after a colon in the value", `echo FOO=x:~/m`, "FOO=x:/h/m"},
		{"a name that starts with a letter", `echo xFOO=~/m`, "xFOO=/h/m"},
		{"a name that starts with an underscore", `echo _f=~/m`, "_f=/h/m"},
		{"an append", `echo FOO+=~/m`, "FOO+=/h/m"},
		{"a subscripted name", `echo a[0]=~/m`, "a[0]=/h/m"},
		{"a function's argument", `f() { echo "$1"; }; f FOO=~/m`, "FOO=/h/m"},
		{"a for list", `for w in FOO=~/m; do echo "$w"; done`, "FOO=/h/m"},
		// The boundary. Every one of these keeps the tilde in every column,
		// this one included.
		{"a left side that is not a name", `echo -- --opt=~/m`, "-- --opt=~/m"},
		{"a left side that opens with a digit", `echo 1abc=~/m`, "1abc=~/m"},
		{"a left side with a dot in it", `echo f.g=~/m`, "f.g=~/m"},
		{"a value that opens with a second equals", `echo FOO==~/m`, "FOO==~/m"},
		{"a tilde after a later equals", `echo FOO=a=~/m`, "FOO=a=~/m"},
		{"a word that arrived quoted", `echo "FOO=~/m"`, "FOO=~/m"},
		{"a head that arrived quoted", `echo ""FOO=~/m`, "FOO=~/m"},
		{"no assignment shape at all", `echo x:~/m`, "x:~/m"},
		// And POSIX mode takes it away, which is the same binary answering
		// both ways rather than two builds: `as sh` in the measurement is
		// this shell under that name.
		{"posix mode", `set -o posix; echo FOO=~/m`, "FOO=~/m"},
		{"posix mode turned back off", "set -o posix\nset +o posix\necho FOO=~/m", "FOO=/h/m"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			sh := bashShell(&out, &errs)
			sh.Env = []string{"HOME=/h", "PATH=/usr/bin:/bin", "LC_ALL=C"}
			code := driver.MainArgs(sh, []string{"bash", "-c", tc.src})
			if got := strings.TrimSpace(out.String()); got != tc.want || errs.Len() != 0 || code != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q",
					tc.src, got, errs.String(), code, tc.want)
			}
		})
	}
}
