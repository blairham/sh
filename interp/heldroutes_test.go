// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"
)

// Every route a relative path takes to the operating system answers from the
// directory the shell holds, not from whatever its old name leads to now.
//
// Measured 2026-10-15 with a script that enters `a` and has `a` removed and
// made again holding `f`, `x` (a sourceable file) and `p` (an executable):
// zsh 5.9.2, bash 5.3, bash 3.2, ksh93 and dash all glob the deleted directory
// (bash, ksh93 and dash print `[*]`, zsh has no matches), fail `: > g` and
// leave nothing in the new `a`, fail `< f`, `. ./x` and `./p`, and start an
// external command in the deleted directory; with `a` removed and nothing in
// its place an external command still runs there. This shell answered every
// one of those from the new directory, or failed the start with `chdir …: no
// such file or directory` (#6532). The panel is unanimous, so the rows name no
// shell; the glob row is written for the shells that keep an unmatched
// pattern, which is the posix preset's reading.

// heldRouteRows prints one field per route. An external /bin/sh globbing its
// own working directory is what says where the child started: `*` in the
// deleted directory, `f p x` in the new one.
const heldRouteRows = `ok() { if [ "$1" = 0 ]; then printf ok; else printf fail; fi; }
printf 'glob=[%s] ' "$(printf '%s' *)"
( : > g ) 2>/dev/null; s=$?; printf 'redir='; ok $s
( read l < f ) 2>/dev/null; s=$?; printf ' in='; ok $s
( . ./x ) >/dev/null 2>&1; s=$?; printf ' source='; ok $s
( ./p ) >/dev/null 2>&1; s=$?; printf ' exec='; ok $s
printf ' child=[%s]\n' "$(/bin/sh -c 'echo *' 2>/dev/null)"
`

func recreateWithFiles(t *testing.T, d string) {
	t.Helper()
	if err := os.RemoveAll(d); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(d, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"f": "hi\n", "x": "echo SOURCED\n", "p": "#!/bin/sh\necho RAN\n"} {
		if err := os.WriteFile(filepath.Join(d, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEveryRelativeRouteAsksTheDirectoryHeld(t *testing.T) {
	t.Run("control: the files are where the shell is", func(t *testing.T) {
		// The positive the rows below need: in a directory nothing has
		// happened to, every route finds what is there.
		d := filepath.Join(resolvedTempDir(t), "a")
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		recreateWithFiles(t, d)
		r, out := relativeProbeRunner(t, d)
		runPart(t, r, heldRouteRows)
		want := "glob=[fpx] redir=ok in=ok source=ok exec=ok child=[f g p x]\n"
		if got := out.String(); got != want {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("deleted and made again under the same name", func(t *testing.T) {
		d := filepath.Join(resolvedTempDir(t), "a")
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		r, out := relativeProbeRunner(t, d)
		recreateWithFiles(t, d)
		runPart(t, r, heldRouteRows)
		want := "glob=[*] redir=fail in=fail source=fail exec=fail child=[*]\n"
		if got := out.String(); got != want {
			t.Errorf("got  %q\nwant %q: a route answered from the new directory "+
				"rather than the one the shell is in", got, want)
		}
		if _, err := os.Stat(filepath.Join(d, "g")); err == nil {
			t.Error("`: > g` wrote into the new directory")
		}
	})

	t.Run("deleted and nothing in its place", func(t *testing.T) {
		d := filepath.Join(resolvedTempDir(t), "a")
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		r, out := relativeProbeRunner(t, d)
		if err := os.Remove(d); err != nil {
			t.Fatal(err)
		}
		runPart(t, r, "/bin/sh -c 'echo started' 2>/dev/null; echo status=$?\n")
		if got, want := out.String(), "started\nstatus=0\n"; got != want {
			t.Errorf("got %q, want %q: every shell in the panel starts it in the deleted directory", got, want)
		}
	})
}
