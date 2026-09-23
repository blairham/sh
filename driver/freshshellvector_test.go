// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// The fresh shell a shebang-less file runs in gets the vector **this front
// end composed**, not the one the calling shell is holding.
//
// interp builds that child from the caller's exported fields, and two of them
// carry state a script can move: the extended-pattern flag lives in
// syntax.Dialect, and several shell options in interp.Semantics. Copied
// across, they made a shebang-less script inherit option changes a real fresh
// shell has no way to know about.
//
// Measured 2026-09-23 on bash 5.3.15 with an executable file holding
// `shopt extglob` and no shebang, run from a shell that had just set it: bash
// answers `off`, this shell answered `on`. The leak was exactly that narrow —
// variables, functions and `set -e` were all correctly fresh in the same run,
// because none of those rides on a field.
//
// The grammar half of that vector is reached *before* execution, so it leaks
// by a second road: the file was read with the caller's parser while the
// child ran with its own. `@(a)b` then parsed, because the caller had the
// flag, and matched literally, because the child did not — an empty answer
// that looks like success. Both roads are closed now, and this row would
// answer MATCHED if either reopened.
//
// This front end has no `shopt`, so the caller moves the flag through a
// builtin registered for the test. That is the whole reason this test exists
// here rather than only in interp: a mutation deleting the reset in
// setUpFreshShell survived every assertion that did not have a caller able to
// move the flag first (#4149).

// movesTheFlag is a dialect whose only addition is a builtin that turns the
// extended patterns on, standing in for the `shopt -s extglob` a real dialect
// would have.
func movesTheFlag(r *interp.Runner) {
	r.Register("move-the-flag", func(r *interp.Runner, _ context.Context, _ []string) int {
		// The exported road a real dialect's `shopt -s extglob` takes, which
		// copies the dialect rather than writing through the shared pointer.
		r.SetMatchOption(interp.QuantifiedGroupsEverywhere, true)
		return 0
	})
}

func noShebang(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "noshebang")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// `@(a)b` needs the extended patterns to *parse*, so the child's answer says
// which vector it was handed: MATCHED where it has the flag, and a refusal
// where it does not. The refusal is the observable rather than a nuisance.
const flagProbe = "case ab in @(a)b) echo MATCHED;; esac\n"

func TestAShebangLessScriptDoesNotInheritAFlagTheCallerMoved(t *testing.T) {
	path := noShebang(t, flagProbe)
	sh := shell()
	sh.Register = movesTheFlag
	sh.Dialect.ExtendedPattern = false

	out, errs, _ := runArgs(t, sh, "testsh", "-c", "move-the-flag; "+path)
	if strings.Contains(out, "MATCHED") {
		t.Errorf("the child inherited the flag the caller moved: %q", out)
	}
	if !strings.Contains(errs, "unexpected") {
		t.Errorf("want the child to refuse the pattern it has no flag for, got %q / %q", out, errs)
	}
}

// And the front end's own answer does reach the child, by the same route, so
// the row above cannot pass against a child that simply never has the flag —
// or against a route that never reaches a shebang-less file at all.
func TestAShebangLessScriptGetsTheFrontEndsOwnVector(t *testing.T) {
	path := noShebang(t, flagProbe)
	sh := shell()
	sh.Dialect.ExtendedPattern = true

	out, errs, _ := runArgs(t, sh, "testsh", "-c", path)
	if !strings.Contains(out, "MATCHED") {
		t.Errorf("the front end's own vector did not reach the child: %q / %q", out, errs)
	}
	if !sh.Dialect.ExtendedPattern {
		t.Error("composing a child mutated the front end's vector")
	}
}
