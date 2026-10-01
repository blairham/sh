// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// **A builtin run under another's name complains under that name**, its own
// name at the front of the sentence rewritten to match — and with no name to
// rewrite from, the running name is all that changes. See
// Runner.RunBuiltinAs.
func TestARenamedBuiltinComplainsUnderItsName(t *testing.T) {
	run := func(from string) string {
		out, _ := runGrammar(t, "chdir /nonexistent-dir-for-this-test", nil, func(r *Runner) {
			r.Register("chdir", func(rr *Runner, ctx context.Context, args []string) int {
				cd, _ := rr.Builtin("cd")
				return rr.RunBuiltinAs("chdir", from, cd, ctx, args)
			})
		})
		return out
	}
	if out := run("cd"); !strings.Contains(out, "chdir: ") || strings.Contains(out, "cd: ") {
		t.Errorf("renamed from cd: %q, want the complaint opening with chdir and never cd", out)
	}
	if out := run(""); !strings.Contains(out, "cd: ") {
		t.Errorf("with nothing to rename from: %q, want cd's own wording kept", out)
	}
}
