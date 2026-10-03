// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// A `[[ -v ]]` whose subscript will not evaluate ends what a failed
// expansion ends rather than answering false: the complaint, and nothing
// after it. Unanimous across the three shells with arrays, measured
// 2026-10-03 under `-c` on bash 5.3.20, zsh 5.9.2 and ksh93u+ (#5579). The
// `||` row is the control: a `-v` the condition never reaches evaluates
// nothing.
func TestAFailedSubscriptInAnIsSetTestEndsTheLine(t *testing.T) {
	presets := []dialecttest.Preset{
		{Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics, Diagnostics: bash.Diagnostics, Apply: bash.Apply},
		{Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics, Diagnostics: zsh.Diagnostics, Apply: zsh.Apply},
		{Name: "ksh", Dialect: ksh.Dialect, Semantics: ksh.Semantics, Diagnostics: ksh.Diagnostics, Apply: ksh.Apply},
	}
	for _, p := range presets {
		t.Run(p.Name, func(t *testing.T) {
			for _, src := range []string{
				`a=(x y z); [[ -v 'a[1/0]' ]]; echo after`,
				`a=(x y z); [[ -v a[1/0] ]]; echo after`,
				`a=(x y z); if [[ -v 'a[b c]' ]]; then echo y; fi; echo after`,
			} {
				out, st, err := p.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(out, "after") || st == 0 || out == "" {
					t.Errorf("%s: got %q at %d, want the complaint and nothing after", src, out, st)
				}
			}
			out, st, err := p.Combined(t, dialecttest.Base{Dir: t.TempDir()}, `a=(x y z); [[ 1 -eq 1 || -v 'a[1/0]' ]]; echo after`)
			if err != nil {
				t.Fatal(err)
			}
			if out != "after\n" || st != 0 {
				t.Errorf("control: got %q at %d, want after at 0", out, st)
			}
		})
	}
}
