// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// Three more answers this dialect never gave, and so gave dash's — the shape
// inheriteddefaults_test.go records and #3248 named.
//
// Every expectation here was measured 2026-09-16 against BusyBox v1.37.0 in
// the digest-pinned Alpine image internal/oracle reaches, under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, a script file with stdin from /dev/null. The
// suite rows are share/suite/ash/arith.tests, builtins-common.tests and
// status.tests; these are the same claims where a `go test` can reach them.

// runInTemp is run() in a directory of the test's own, for the rows that write a
// file. The package's run() leaves the working directory where `go test` put
// it, which is the package's own source directory — a `read < f` row there
// would leave an `f` beside the source (and did, once).
func runInTemp(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "ash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		return out + "unsupported: " + err.Error(), -1
	}
	return out, st
}

// TestAnUnsetNameBelowARecursedLookupIsZero.
//
// ArithNameValueRecurses was answered Yes for this shell and
// ArithRecursedNameMustBeSet was left unanswered beside it, which is worse
// than either answer: the axis is asked only where the recursion happens, so
// every `x=abc; $((x))` with `abc` unset stopped the script with "the shells
// disagree here and no dialect was chosen". BusyBox prints the zero, as bash
// and zsh do; ksh93 is the shell that refuses.
func TestAnUnsetNameBelowARecursedLookupIsZero(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"bare", `x=abc; unset abc; echo "$(( x ))"`, "0\n"},
		{"in a sum", `x=abc; unset abc; echo "$(( x + 1 ))"`, "1\n"},
		{"two names deep", `p=q; q=r; unset r; echo "$(( p ))"`, "0\n"},
		// The control: a name that is set at the bottom is its value, which
		// is what says the rows above recursed at all.
		{"set at the bottom", `x=abc; abc=5; echo "$(( x + 1 ))"`, "6\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// TestReadTakesADelimiterADescriptorAndSilence.
//
// dash's `read` is the POSIX pair; this dialect held dash's list plus `-p`,
// `-t` and `-n`, so `read -d ';' x` was `illegal option -d` at 2 where
// BusyBox answers 0 with the text before the delimiter. `-a`, `-e`, `-i` and
// `-N` are refused in BusyBox and stay refused here, which is the half of
// this that says the list is BusyBox's rather than bash's.
func TestReadTakesADelimiterADescriptorAndSilence(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"-d ends the read at the delimiter",
			"printf 'a b;c\\n' > f; read -r -d ';' x y < f; printf '%s [%s][%s]\\n' \"$?\" \"$x\" \"$y\"",
			"0 [a][b]\n",
		},
		{
			"-d reads past a newline",
			"printf 'one\\ntwo;three\\n' > f; read -r -d ';' x < f; printf '%s\\n' \"$x\"",
			"one\ntwo\n",
		},
		{
			"-d empty reads to the end, at 1",
			"printf 'a;b\\n' > f; read -r -d '' x < f; printf '%s [%s]\\n' \"$?\" \"$x\"",
			"1 [a;b]\n",
		},
		{
			"-u reads the descriptor it is given",
			"printf 'one\\ntwo\\n' > f; exec 3< f; read -r -u 3 x; printf '%s [%s]\\n' \"$?\" \"$x\"",
			"0 [one]\n",
		},
		{
			"-s is accepted",
			"printf 'a\\n' > f; read -r -s x < f; printf '%s [%s]\\n' \"$?\" \"$x\"",
			"0 [a]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runInTemp(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0", out, st, tc.want)
			}
		})
	}
	for _, letter := range []string{"a", "e", "N"} {
		t.Run("-"+letter+" is refused", func(t *testing.T) {
			out, st := runInTemp(t, "printf 'a\\n' > f; read -"+letter+" x < f")
			if st != 2 || !strings.Contains(out, "illegal option -"+letter) {
				t.Errorf("got %q at %d, want `illegal option -%s` at 2", out, st, letter)
			}
		})
	}
}

// TestASpecialBuiltinStopsAtTheRedirectionStatus.
//
// XCU 2.8.1 makes a redirection failure on a special builtin fatal to a
// non-interactive shell, and the status it stops with read as
// FatalErrorStatusIsOne's for as long as nothing could tell the two apart:
// dash is 2 and 2, bash in POSIX mode, ksh93 and zsh emulating sh are 1 and
// 1. BusyBox is the column where the two numbers differ — a redirection
// failure is 1 there and a fatal error is 2 — and it stops at the
// redirection's 1.
func TestASpecialBuiltinStopsAtTheRedirectionStatus(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      int
	}{
		{"exec", `exec 3< /nosuchfile_zz; echo reached`, 1},
		{"colon", `: > /nosuchdir_zz/f; echo reached`, 1},
		{"through eval", `eval ': > /nosuchdir_zz/f'; echo reached`, 1},
		{"in a function", `f() { : > /nosuchdir_zz/f; }; f; echo reached`, 1},
		{"a descriptor that is not open", `: <&9; echo reached`, 1},
		// The controls. A fatal error that is not a redirection is 2, and so
		// is a `<&` word naming no descriptor — that redirection has already
		// stopped the shell on its own before the builtin's rule is asked.
		{"a fatal error", `: "${unset_zz?gone}"; echo reached`, 2},
		{"a word after <& that names nothing", `exec 9<&qq; echo reached`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src)
			if strings.Contains(out, "reached") {
				t.Errorf("the shell carried on: %q", out)
			}
			if st != tc.want {
				t.Errorf("status %d, want %d; output %q", st, tc.want, out)
			}
		})
	}
	// And a regular builtin is not stopped at all, which is what keeps the
	// rows above about *special* builtins.
	out, st := run(t, `true > /nosuchdir_zz/f; echo reached`)
	if st != 0 || !strings.Contains(out, "reached") {
		t.Errorf("a regular builtin stopped the shell: %q at %d", out, st)
	}
}
