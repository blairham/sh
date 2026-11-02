// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tests_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// An interactive bash puts back the entries a failed prefix had already
// assigned ahead of the failure, as it does in a script the failure ends.
// Measured 2026-11-02 on bash 5.3.20 under `-i` with the lines on standard
// input, `env -i PATH=/usr/bin:/bin HOME=<scratch> PS1= ENV=`: `v=old`, where
// an interactive ksh93 keeps `v=new` (#7098). See
// interp.Semantics.PrefixEntriesStayWhenAnInteractiveShellCarriesOn.
func TestAnInteractiveBashPutsAFailedPrefixsEntriesBack(t *testing.T) {
	t.Parallel()
	var o, e bytes.Buffer
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &o, &e
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "PS1=", "ENV="}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString("f() { :; }\nv=old\nv=new w=${q?boom} :\necho \"v=$v\"\n")
		_ = w.Close()
	}()
	t.Cleanup(func() { _ = r.Close() })
	sh.Stdin = r
	driver.MainArgs(sh, []string{"bash", "-i"})
	// The complaint is what says the prefix failed at all.
	if out, errs := o.String(), e.String(); out != "v=old\n" || !strings.Contains(errs, "q: boom") {
		t.Errorf("stdout %q, want %q after `q: boom` (stderr %q)", out, "v=old\n", errs)
	}
}
