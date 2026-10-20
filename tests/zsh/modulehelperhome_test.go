// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestTheModuleHelperLeavesNoHomeBehind is #5454: the helper re-entered with
// the small environment a shell under test hands it must not make a scratch
// home it then exits without removing.
//
// The child gets the environment the wrappers give it, minus anything that
// would point it at a home, with TMPDIR at a directory of this test's own,
// which must be empty afterwards. Before the fix this left one
// `sh-dialect-zsh-home-*` directory per run (measured: five over the package's
// module cases).
func TestTheModuleHelperLeavesNoHomeBehind(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	cmd := exec.Command(exe, "-test.run=^TestModuleHelperProcess$")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "TMPDIR=" + tmp, helperMode + "=delay", "SH_TEST_MODULE_HELPER_MS=0"}
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != "x" {
		t.Fatalf("the helper wrote %q (%v), want x — the case is about a helper that ran", out, err)
	}
	left, _ := filepath.Glob(filepath.Join(tmp, "*"))
	if len(left) != 0 {
		t.Errorf("the helper left %v in its temporary directory, want nothing", left)
	}
}
