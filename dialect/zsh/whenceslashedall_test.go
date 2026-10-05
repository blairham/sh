// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWhenceAllLooksASlashedNameUpAlongPath is #5956: under `-a`, a relative
// name with a slash in it is each PATH entry with the name appended as
// written, and is not the file in the working directory. Every row measured
// on zsh 5.9.2 (/opt/homebrew/bin/zsh, -f), 2026-10-05, in a directory `<d>`
// holding an executable `bin/man` and a plain file `plain`.
func TestWhenceAllLooksASlashedNameUpAlongPath(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "man"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, src, want string }{
		{"/usr/bin:/bin", `whence -va ./bin/man`, "./bin/man not found\nst=1"},
		{"/usr/bin:/bin", `whence -a ./bin/man`, "st=1"},
		{"/usr/bin:/bin", `whence -wa ./bin/man`, "./bin/man: none\nst=1"},
		{"/usr/bin:/bin", `type -a ./bin/man`, "./bin/man not found\nst=1"},
		{"/usr/bin:/bin", `which -a ./bin/man`, "./bin/man not found\nst=1"},
		{"<d>:/usr/bin:/bin", `whence -va ./bin/man`, "./bin/man is <d>/./bin/man\nst=0"},
		{"<d>:/usr/bin:/bin", `whence -pa bin/man`, "<d>/bin/man\nst=0"},
		{"<d>:/usr/bin:/bin", `type -a bin/man`, "bin/man is <d>/bin/man\nst=0"},
		{"<d>/:/bin", `whence -va bin/man`, "bin/man is <d>//bin/man\nst=0"},
		{".:/usr/bin", `whence -va bin/man`, "bin/man is ./bin/man\nst=0"},
		{":/usr/bin", `whence -va ./bin/man`, "./bin/man is ./bin/man\nst=0"},
		{"<d>:<d>", `whence -a bin/man`, "<d>/bin/man\n<d>/bin/man\nst=0"},
		{"<d>", `whence -va plain`, "plain not found\nst=1"},
		// Controls: without `-a` the working directory answers, and an
		// absolute name is only itself.
		{"/usr/bin:/bin", `whence -v ./bin/man`, "./bin/man is ./bin/man\nst=0"},
		{"/usr/bin:/bin", `whence -va /bin/sh`, "/bin/sh is /bin/sh\nst=0"},
	} {
		path := strings.ReplaceAll(tc.path, "<d>", dir)
		want := strings.ReplaceAll(tc.want, "<d>", dir) + "\n"
		out, _ := runZsh(t, dir, "cd "+dir+"\nPATH="+path+"\n"+tc.src+"; echo st=$?")
		if out != want {
			t.Errorf("PATH=%s %s = %q, want %q", tc.path, tc.src, out, want)
		}
	}
}
