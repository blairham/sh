// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestKshAutoloadIsReadAsTheFunctionLoads pins `kshautoload`: where neither
// `-k` nor `-z` was given, the option as the function is loaded decides
// whether the file is run and the definition it leaves called. Measured
// 2026-10-02 on zsh 5.9.2 under `-f` (#5155).
func TestKshAutoloadIsReadAsTheFunctionLoads(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo"), []byte("echo foo loaded; foo() { echo foo run $*; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ src, want string }{
		{"autoload foo; setopt kshautoload; foo a", "foo loaded\nfoo run a\n"},
		{"setopt kshautoload; autoload foo; unsetopt kshautoload; foo a", "foo loaded\n"},
		{"setopt kshautoload; autoload -z foo; foo a", "foo loaded\n"},
		{"setopt kshautoload; autoload foo; foo a; foo b", "foo loaded\nfoo run a\nfoo run b\n"},
	} {
		got, _ := runZsh(t, dir, "fpath=(.); cd "+dir+"; "+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
