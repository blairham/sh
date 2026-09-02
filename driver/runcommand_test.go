// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// A `-c` command string is not the same as running its text, and a front end
// with options of its own still has to reach the same one.
//
// `cmd/sh` reached past it — it has `-tokens`, `-parse` and `-dialect`, which
// are not shell conventions, so it parses its own arguments and then called
// the generic entry point with the string alone. Three things went with it,
// and the conformance harness could see one of them: `make conformance` ran
// the same corpus through `sh -dialect bash` and scored twelve lower than
// `make conformance-dialects` did through `./bash`, every one of them the
// missing label.
func TestRunCommandIsTheSameMinusThePath(t *testing.T) {
	t.Run("the operands are $0 and the parameters", func(t *testing.T) {
		// Unanimous, and the corpus cannot see it: every case there is `-c`
		// with no operands, so `$#` is legitimately 0 in all of them.
		out := runCommand(t, `echo "0=$0 1=$1 n=$#"`, []string{"zero", "one", "two"})
		if got, want := strings.TrimSpace(out), "0=zero 1=one n=2"; got != want {
			t.Errorf("out = %q, want %q", got, want)
		}
	})

	t.Run("and none of them is $0 when there are none", func(t *testing.T) {
		out := runCommand(t, `echo "0=$0 n=$#"`, nil)
		if !strings.Contains(out, "n=0") {
			t.Errorf("out = %q, want no parameters", out)
		}
		if !strings.Contains(out, "0=testsh") {
			t.Errorf("out = %q, want the shell as $0", out)
		}
	})

	t.Run("the origin is labeled in a parse failure", func(t *testing.T) {
		// One dialect names where the input came from in the location, and
		// `-c` is what it calls this one.
		sh := shell()
		dg := interp.Diagnostics{}
		// The dialect answer this turns on: one of them puts where the input
		// came from into the location and the rest do not.
		dg.NamesTheInputInLocation = true
		sh.Diagnostics = dg
		var o, e bytes.Buffer
		sh.Stdout, sh.Stderr = &o, &e
		driver.RunCommand(sh, "if", nil)
		if !strings.Contains(e.String(), "-c") {
			t.Errorf("err = %q, want the origin named", e.String())
		}
	})
}

func runCommand(t *testing.T, src string, operands []string) string {
	t.Helper()
	sh := shell()
	var o, e bytes.Buffer
	sh.Stdout, sh.Stderr = &o, &e
	if code := driver.RunCommand(sh, src, operands); code != 0 {
		t.Fatalf("status %d: %s", code, e.String())
	}
	return o.String()
}
