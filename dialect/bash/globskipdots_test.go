// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dotDirectory is a directory holding `.a`, `.b`, `vis` and a `sub/` with
// `.x` and `y` in it — the shape every row below is measured in.
func dotDirectory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".a", ".b", "vis", "sub/.x", "sub/y"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// `shopt -u globskipdots` puts `.` and `..` back into a pathname expansion
// whose component begins with a period, which is the one way any shell in the
// panel has of asking for them here.
//
// The default is already right — the two names are absent, which is what
// bash 5.2 onward does — so what was missing was the way back: the name sat in
// shoptStates reading `on`, so `shopt -s` was a silent grant and `shopt -u`
// the refusal, at status 1 (#3386).
//
// Measured 2026-09-17 on bash 5.3.20 under `LC_ALL=C` in the directory above.
func TestGlobSkipDotsOffListsDotAndDotDot(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The default, and the same pattern with the option off.
		{`printf ' [%s]' .*`, " [.a] [.b]"},
		{`shopt -u globskipdots; printf ' [%s]' .*`, " [.] [..] [.a] [.b]"},
		// It is the *component* the pattern wrote, so a `*` never sees them
		// however the option stands.
		{`shopt -u globskipdots; printf ' [%s]' *`, " [sub] [vis]"},
		// And the leading-period rule is a separate question: lifting it and
		// asking for the two names still does not put them in a `*`. That is
		// the row that says this is not the hidden-name rule again.
		{`shopt -u globskipdots; shopt -s dotglob; printf ' [%s]' *`, " [.a] [.b] [sub] [vis]"},
		// One level down, where the period is written on the last component.
		{`shopt -u globskipdots; printf ' [%s]' */.*`, " [sub/.] [sub/..] [sub/.x]"},
		// The trailing-slash form keeps what is a directory, which is what
		// leaves `.a` and `.b` out — and the order is the listing's own.
		{`shopt -u globskipdots; printf ' [%s]' .*/`, " [../] [./]"},
		// A `**` descent does not list them at all, measured rather than
		// assumed: the walk is untouched by the option.
		{`shopt -u globskipdots; shopt -s globstar; printf ' [%s]' **`, " [sub] [sub/y] [vis]"},
		// And the way back.
		{`shopt -u globskipdots; shopt -s globskipdots; printf ' [%s]' .*`, " [.a] [.b]"},
	} {
		out, st := runBash(t, dotDirectory(t), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The name answers the builtin the way bash does in every spelling, and a
// request in either direction is taken at 0 with nothing on standard error —
// which is the half that was a refusal before.
//
// The two rows at status 1 are the *query*, which answers whether every name
// asked about is on: `shopt -p globskipdots` after a `shopt -u` writes the
// line and reports 1. Measured, and it is the same 1 the refusal used to
// carry — which is why the status alone could not tell the two apart and the
// rows below assert what was written with it.
func TestGlobSkipDotsIsAnOptionRatherThanARecordedName(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{`shopt -u globskipdots; echo "st=$?"`, "st=0\n", 0},
		{`shopt -u globskipdots; shopt -p globskipdots`, "shopt -u globskipdots\n", 1},
		{`shopt -u globskipdots; shopt globskipdots`, "globskipdots        \toff\n", 1},
		{`shopt -u globskipdots; shopt -q globskipdots; echo "q=$?"`, "q=1\n", 0},
		{`shopt -s globskipdots; echo "st=$?"`, "st=0\n", 0},
		{`shopt -p globskipdots`, "shopt -s globskipdots\n", 0},
		{`shopt -q globskipdots; echo "q=$?"`, "q=0\n", 0},
		// It is on by default, so `$BASHOPTS` names it — and stops naming it
		// once a script has turned it off.
		{`case ":$BASHOPTS:" in *:globskipdots:*) echo in ;; *) echo out ;; esac`, "in\n", 0},
		{
			`shopt -u globskipdots; case ":$BASHOPTS:" in *:globskipdots:*) echo in ;; *) echo out ;; esac`,
			"out\n", 0,
		},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s = %q status %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}

// A subshell's `shopt` stays in the subshell, which the option gets for free
// by being state on the runner rather than a dialect answer — and is worth an
// assertion because that is what the choice bought.
func TestGlobSkipDotsStaysInASubshell(t *testing.T) {
	src := `(shopt -u globskipdots; printf 'in'; printf ' [%s]' .*; echo); printf 'out'; printf ' [%s]' .*; echo`
	out, st := runBash(t, dotDirectory(t), src)
	want := "in [.] [..] [.a] [.b]\nout [.a] [.b]\n"
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
	if strings.Contains(out, "not implemented") {
		t.Errorf("the option was refused: %q", out)
	}
}
