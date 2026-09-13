// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// The name this shell gives the directory it starts in. Measured against
// ksh93u+ 2012-08-01 on 2026-09-13; interp/inheritedpwd.go holds the panel.
//
// A directory with two true names is what the probe needs, so each case
// builds a link to the one the shell is started in: without it every answer
// spells the directory the same way and a policy that does nothing reads as
// one that works.
func TestThisShellKeepsTheNameItsParentUsed(t *testing.T) {
	t.Run("a PWD that names the directory another way", func(t *testing.T) {
		base := t.TempDir()
		real := filepath.Join(base, "real")
		via := filepath.Join(base, "via")
		if err := os.Mkdir(real, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(real, via); err != nil {
			t.Fatal(err)
		}
		out, _, err := preset.Combined(t,
			dialecttest.Base{Dir: real, Env: []string{"PWD=" + via}}, `echo "[$PWD][$(pwd)]"`)
		if err != nil {
			t.Fatal(err)
		}
		if want := "[" + via + "][" + via + "]"; strings.TrimSpace(out) != want {
			t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
		}
	})
	// And a `PWD` that names nowhere is still the parameter here, where `pwd`
	// answers the real directory — the one place this shell lets the two
	// disagree, and the row that tells this policy from the ash one.
	t.Run("a PWD that names nowhere", func(t *testing.T) {
		base := t.TempDir()
		real := filepath.Join(base, "real")
		if err := os.Mkdir(real, 0o755); err != nil {
			t.Fatal(err)
		}
		out, _, err := preset.Combined(t,
			dialecttest.Base{Dir: real, Env: []string{"PWD=/nonexistent-zz"}}, `echo "[$PWD][$(pwd)]"`)
		if err != nil {
			t.Fatal(err)
		}
		if want := "[/nonexistent-zz][" + real + "]"; strings.TrimSpace(out) != want {
			t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
		}
	})
}

// With no `PWD` handed over, this shell names the directory under `$HOME`
// where it sits there. It is what the conformance harness hands every case,
// and it is why eight of its rows read `/private` before this was measured.
func TestWithNoPwdTheDirectoryIsNamedUnderHome(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	via := filepath.Join(base, "via")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, via); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, home, want string }{
		{"HOME names it", via, via},
		{"HOME is above it", base, base + "/real"},
		{"HOME is somewhere else", "/usr", real},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := preset.Combined(t,
				dialecttest.Base{Dir: real, Env: []string{"HOME=" + c.home}}, `echo "[$PWD][$(pwd)]"`)
			if err != nil {
				t.Fatal(err)
			}
			if want := "[" + c.want + "][" + c.want + "]"; strings.TrimSpace(out) != want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
			}
		})
	}
}
