// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// bash's `read -p` takes a prompt as the option's argument and shows it only
// to a terminal, on standard error. Measured against bash 5.3 and 3.2
// (2026-09-04): piped input reads exactly as though -p were absent, with the
// prompt appearing nowhere.
func TestReadPromptIsWithheldFromAPipe(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `printf 'data\n' | { read -p "PR> " v; echo "st=$? v=[$v]"; }`)
	if !strings.Contains(out, "st=0 v=[data]") {
		t.Errorf("got %q, want the line read as though -p were absent", out)
	}
	if strings.Contains(out, "PR> ") {
		t.Errorf("got %q, want no prompt off a terminal", out)
	}
}

// A -p whose prompt never arrives is refused in bash's own words — the same
// sentence the substrate already used — with the usage line after it and
// status 2, exactly as a bad option is. Measured 2026-09-04.
func TestReadPromptMissingItsArgument(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `read -p </dev/null; echo "st=$?"`)
	if !strings.Contains(out, "read: -p: option requires an argument") {
		t.Errorf("got %q, want bash's missing-argument sentence", out)
	}
	if !strings.Contains(out, "read: usage: read") {
		t.Errorf("got %q, want the usage line after the complaint", out)
	}
	if !strings.Contains(out, "st=2") {
		t.Errorf("got %q, want status 2", out)
	}
}
