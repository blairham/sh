// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A `read` from a descriptor a redirection closed is refused out loud, not
// read as an empty input. Measured 2026-10-04 on bash 5.3.20 (#5722). See
// interp.Diagnostics.ReadFromAClosedDescriptor.
func TestReadFromAClosedDescriptorSaysSo(t *testing.T) {
	for _, c := range []struct{ src, errs string }{
		{"read a <&-; echo st=$?", "bash: line 1: read: 0: read error: Bad file descriptor\n"},
		{"exec 3<&-; read -u 3 a 3<&-; echo st=$?", "bash: line 1: read: 3: invalid file descriptor: Bad file descriptor\n"},
	} {
		var o, e strings.Builder
		sh := shell()
		sh.SystemStartupDirectory = t.TempDir()
		sh.Stdout, sh.Stderr = &o, &e
		sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
		driver.MainArgs(sh, []string{"bash", "-c", c.src})
		if o.String() != "st=1\n" || e.String() != c.errs {
			t.Errorf("%s:\n got %q, %q\nwant %q, %q", c.src, o.String(), e.String(), "st=1\n", c.errs)
		}
	}
}
