// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// TestAnEmulationThatSearchesFindsTheScriptOnPath pins which emulations
// start with `pathscript` on, so that a slash-less script operand is looked
// for along PATH. Measured 2026-10-02 on zsh 5.9.2 with the script on PATH
// and nowhere else (#5383).
func TestAnEmulationThatSearchesFindsTheScriptOnPath(t *testing.T) {
	onPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(onPath, "myscr"), []byte("echo ran-from-path\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	for _, tc := range []struct {
		argv  []string
		found bool
	}{
		{[]string{"zsh", "--emulate", "sh", "-f", "myscr"}, true},
		{[]string{"zsh", "--emulate", "ksh", "-f", "myscr"}, true},
		{[]string{"sh", "-f", "myscr"}, true},
		{[]string{"/some/where/ksh", "-f", "myscr"}, true},
		// The option written after the mode wins over it.
		{[]string{"zsh", "--emulate", "sh", "-f", "+o", "pathscript", "myscr"}, false},
		{[]string{"sh", "-f", "+o", "pathscript", "myscr"}, false},
		// And the modes that do not have it.
		{[]string{"zsh", "--emulate", "zsh", "-f", "myscr"}, false},
		{[]string{"zsh", "--emulate", "csh", "-f", "myscr"}, false},
		{[]string{"zsh", "-f", "myscr"}, false},
	} {
		var out, errs bytes.Buffer
		sh := zshWriting(&out, &errs)
		sh.Env = []string{"PATH=" + onPath}
		code := driver.MainArgs(sh, tc.argv)
		got := code == 0 && out.String() == "ran-from-path\n"
		if got != tc.found {
			t.Errorf("%s: ran %q said %q status %d, want found=%v",
				strings.Join(tc.argv, " "), out.String(), errs.String(), code, tc.found)
		}
	}
}
