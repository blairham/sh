// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/driver"
)

// A run of slashes behind the first pattern component comes back as one, and
// the literal text ahead of it is written back as written. Measured
// 2026-10-03 on bash 5.3.20 in a tree holding `cx/dx/ax`.
func TestASlashRunBehindAPatternIsOneSlash(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cx", "dx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cx", "dx", "ax"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ pattern, want string }{
		{`c*//`, "[cx/]"},
		{`c*///`, "[cx/]"},
		{`c*//dx`, "[cx/dx]"},
		{`c*/dx//a*`, "[cx/dx/ax]"},
		{`cx//d*//ax`, "[cx//dx/ax]"},
		{`.//c*//dx`, "[.//cx/dx]"},
		// The control: no pattern ahead of the run, so it stays as written.
		{`cx//d*`, "[cx//dx]"},
	} {
		var out, errs bytes.Buffer
		src := "cd " + dir + " && printf '[%s]' " + tc.pattern
		driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", src})
		if out.String() != tc.want {
			t.Errorf("%s: got %q (err %q), want %q", tc.pattern, out.String(), errs.String(), tc.want)
		}
	}
}
