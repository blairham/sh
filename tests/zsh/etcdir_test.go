// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// The two platforms, each as a directory this test made rather than as the
// machine's own `/etc` — which is the point of the parameter: the answer is
// about a tree, so a test can hand it one and never read the runner's.
func TestSystemStartupDirectory(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		// make is what exists under the scratch `/etc` before the ask.
		make func(t *testing.T, etc string)
		want func(etc string) string
	}{
		{
			// macOS: the files sit in `/etc` itself and there is no
			// `/etc/zsh` at all.
			name: "the files are in etc itself",
			make: func(t *testing.T, etc string) {
				t.Helper()
				write(t, etc, "zshrc", "# marker\n")
				write(t, etc, "zprofile", "# marker\n")
			},
			want: func(etc string) string { return etc },
		},
		{
			// Debian: a directory of zsh's own, and nothing named `zsh*`
			// beside it.
			name: "the files are in a directory of their own",
			make: func(t *testing.T, etc string) {
				t.Helper()
				own := filepath.Join(etc, "zsh")
				if err := os.Mkdir(own, 0o755); err != nil {
					t.Fatal(err)
				}
				write(t, own, "zshenv", "# marker\n")
			},
			want: func(etc string) string { return filepath.Join(etc, "zsh") },
		},
		{
			// An administrator with no zsh files at all still gets a
			// directory to look in, and finds nothing in it.
			name: "nothing there",
			make: func(t *testing.T, etc string) { t.Helper() },
			want: func(etc string) string { return etc },
		},
		{
			// A *file* named `zsh` is not the directory, and a shell that
			// took it would try to source `/etc/zsh/zshenv` through it.
			name: "a file of that name is not the directory",
			make: func(t *testing.T, etc string) {
				t.Helper()
				write(t, etc, "zsh", "# marker\n")
			},
			want: func(etc string) string { return etc },
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			etc := t.TempDir()
			c.make(t, etc)
			if got, want := zsh.SystemStartupDirectory(etc), c.want(etc); got != want {
				t.Errorf("SystemStartupDirectory(%q) = %q, want %q", etc, got, want)
			}
		})
	}
}

// The empty string is the front end saying it reads no system files, and an
// answer of `zsh` or `/zsh` would be a real path on a real machine.
func TestSystemStartupDirectoryOfNothingIsNothing(t *testing.T) {
	t.Parallel()
	if got := zsh.SystemStartupDirectory(""); got != "" {
		t.Errorf("SystemStartupDirectory(%q) = %q, want %q", "", got, "")
	}
}
