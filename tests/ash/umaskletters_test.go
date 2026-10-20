// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// runUmask runs a snippet on a runner holding a mask of its own, since a
// [interp.Runner] with no SetUmask hook refuses every `umask` before a clause
// is ever read. The hook is a variable rather than the process's mask: a test
// that moved the real one would be touching state the whole package shares.
func runUmask(t *testing.T, src string) (string, int) {
	t.Helper()
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{
		Name: "ash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
		Stdout: &buf, Stderr: &buf,
	})
	held := 0o022
	r.SetUmask = func(mask int) (int, error) { old := held; held = mask; return old, nil }
	status, err := r.Run(context.Background(), preset.Parse(t, src))
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return buf.String(), status
}

// The two letters a umask has no bit for split this column the opposite way
// from dash, and both values were inherited from the group a doc comment put
// this dialect in rather than from a run (#3237).
//
// Measured 2026-09-17 inside the pinned image
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0, a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`:
//
//	umask 022; umask u+r     0  mask 0022
//	umask 022; umask u+s     2  umask: illegal mode: u+s
//	umask 022; umask u+rs    2  umask: illegal mode: u+rs
//	umask 022; umask g+s     2  umask: illegal mode: g+s
//	umask 022; umask u+t     0  mask 0022
//	umask 022; umask o+t     0  mask 0022
//	umask 022; umask u+X     0  mask 0022
//	umask 022; umask u=rwXs  2  umask: illegal mode: u=rwXs
//
// `u+r` is the control that says the shape of a clause is fine, so what is
// refused is the letter; `u+X` beside `u=rwXs` is the control that says the
// refusal is the `s` and not the conditional-execute letter next to it.
func TestSymbolicMaskRefusesTheSetuidLetterAndTakesTheSticky(t *testing.T) {
	for _, tc := range []struct {
		clause string
		status int
		why    string
	}{
		{"u+r", 0, "the control: an ordinary clause is taken"},
		{"u+s", 2, "the setuid letter is refused"},
		{"u+rs", 2, "and a clause carrying it is refused whole"},
		{"g+s", 2, "whichever group it is written for"},
		{"u+t", 0, "the sticky letter is taken, where dash refuses it"},
		{"o+t", 0, "likewise for the group that has a sticky bit"},
		{"u+X", 0, "conditional execute is taken"},
		{"u=rwXs", 2, "and a clause holding both is refused for the `s`"},
	} {
		out, _ := runUmask(t, "umask 022; umask "+tc.clause+
			"; echo \"st=$? mask=$(umask)\"")
		if want := fmt.Sprintf("st=%d ", tc.status); !strings.Contains(out, want) {
			t.Errorf("umask %s said %q, want %q — %s", tc.clause, out, want, tc.why)
		}
		if !strings.Contains(out, "mask=0022") {
			t.Errorf("umask %s left %q, want the mask unmoved: neither letter "+
				"names a bit a umask holds", tc.clause, out)
		}
		if tc.status == 0 {
			continue
		}
		// The whole clause is named and not the letter, which is what parts
		// this refusal from zsh's `bad symbolic mode permission: s`.
		if want := "illegal mode: " + tc.clause; !strings.Contains(out, want) {
			t.Errorf("umask %s said %q, want %q", tc.clause, out, want)
		}
	}
}
