// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWhenceReportsAPathHitAsWritten is #6015: a PATH hit is reported as the
// entry spells it, a slash and the name, with nothing cleaned or trimmed —
// and `-s` draws an arrow to the clean physical path whenever that is not
// what was written, while `-S` lists links alone and a relative path draws
// no arrow at all. Every row measured on zsh 5.9.2 (/opt/homebrew/bin/zsh,
// -f), 2026-10-05, in a directory `<d>` holding an executable `bin/man` and
// a link `sub/lnk -> <d>/bin/man`. Nothing outside `<d>` is named, because
// the host's own `/bin` is a link on some systems.
func TestWhenceReportsAPathHitAsWritten(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"bin", "sub"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "man"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "bin", "man"), filepath.Join(dir, "sub", "lnk")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, src, want string }{
		{"<d>/sub/../bin", `whence -s man`, "<d>/sub/../bin/man -> <d>/bin/man\nst=0"},
		{"<d>/sub/../bin", `whence -S man`, "<d>/sub/../bin/man\nst=0"},
		{"<d>/sub/../bin", `whence -p man`, "<d>/sub/../bin/man\nst=0"},
		{"<d>/sub/../bin", `whence -v man`, "man is <d>/sub/../bin/man\nst=0"},
		{"<d>/sub/../bin", `which man`, "<d>/sub/../bin/man\nst=0"},
		{"<d>/sub/../bin", `whence -sa man`, "<d>/sub/../bin/man -> <d>/bin/man\nst=0"},
		{"<d>/bin/", `whence -a man`, "<d>/bin//man\nst=0"},
		{"<d>/bin/", `whence -s man`, "<d>/bin//man -> <d>/bin/man\nst=0"},
		{"bin", `whence -s man`, "bin/man\nst=0"},
		{"./bin", `whence -va man`, "man is ./bin/man\nst=0"},
		{"bin/:<d>/bin", `whence -a man`, "bin//man\n<d>/bin/man\nst=0"},
		{"<d>/./sub", `whence -S lnk`, "<d>/./sub/lnk -> <d>/bin/man\nst=0"},
		// A relative path draws no arrow, even through a link.
		{"sub", `whence -s lnk`, "sub/lnk\nst=0"},
		{"sub", `whence -S lnk`, "sub/lnk\nst=0"},
		{"sub", `type -s lnk`, "lnk is sub/lnk\nst=0"},
		// Controls: a clean absolute path and a real link answer as before.
		{"<d>/bin", `whence -s man`, "<d>/bin/man\nst=0"},
		{"<d>/bin:<d>/sub", `whence -sa man lnk`, "<d>/bin/man\n<d>/sub/lnk -> <d>/bin/man\nst=0"},
	} {
		path := strings.ReplaceAll(tc.path, "<d>", dir)
		want := strings.ReplaceAll(tc.want, "<d>", dir) + "\n"
		out, _ := runZsh(t, dir, "cd "+dir+"\nPATH="+path+"\n"+tc.src+"; echo st=$?")
		if out != want {
			t.Errorf("PATH=%s %s = %q, want %q", tc.path, tc.src, out, want)
		}
	}
}

// TestWhenceAllIsSilentAboutAMissAfterAHit is #6016: under `-a`, once any
// operand has been found a later miss writes nothing and leaves the status
// alone, and in the shapes that write no line for a miss the status is 1
// only when nothing was found. Measured on zsh 5.9.2 (-f), 2026-10-05.
func TestWhenceAllIsSilentAboutAMissAfterAHit(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`whence -va sh nosuch`, "sh is /bin/sh\nst=0"},
		{`whence -va nosuch sh`, "nosuch not found\nsh is /bin/sh\nst=1"},
		{`whence -va nosuch sh nosuch2`, "nosuch not found\nsh is /bin/sh\nst=1"},
		{`whence -va nosuch nosuch2 sh`, "nosuch not found\nnosuch2 not found\nsh is /bin/sh\nst=1"},
		{`whence -va sh nosuch sh nosuch2`, "sh is /bin/sh\nsh is /bin/sh\nst=0"},
		{`whence -wa nosuch sh nosuch2`, "nosuch: none\nsh: command\nst=1"},
		{`whence -ca sh nosuch nosuch2`, "/bin/sh\nst=0"},
		{`whence -a nosuch sh nosuch2`, "/bin/sh\nst=0"},
		{`whence -pa nosuch sh`, "/bin/sh\nst=0"},
		{`whence -a nosuch nosuch2`, "st=1"},
		{`whence -va cd nosuch`, "cd is a shell builtin\nst=0"},
		{`type -a sh nosuch`, "sh is /bin/sh\nst=0"},
		{`which -a sh nosuch`, "/bin/sh\nst=0"},
		{`where nosuch sh`, "nosuch not found\n/bin/sh\nst=1"},
		// Control: without `-a` every operand is answered.
		{`whence -v sh nosuch`, "sh is /bin/sh\nnosuch not found\nst=1"},
		{`whence nosuch sh`, "/bin/sh\nst=1"},
	} {
		out, _ := runZsh(t, t.TempDir(), "PATH=/bin\n"+tc.src+"; echo st=$?")
		if out != tc.want+"\n" {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want+"\n")
		}
	}
}
