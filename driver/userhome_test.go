// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package, for the reason promptproviders_test.go is: what is asserted is
// that an unexported builder wires a hook, and a hook that never arrives looks
// exactly like a library with no opinion about `~user`.
package driver

import (
	"path/filepath"
	"testing"

	"github.com/blairham/sh/interp"
)

// The binary answers `~user` and the library does not.
//
// interp leaves `~user` as written with no hook, which is right for a Runner
// embedded in some other program and wrong for a shell — every shell in the
// panel resolves it. So the wiring is the whole of the fix, and a wiring
// dropped on the way across is invisible from inside interp: `~root` simply
// stays `~root`, which is exactly what this shell did before (#2191).
//
// Nothing here asserts anything about *this* machine's users. The name asked
// after is one no system has, so the row says the hook is reachable and says
// nothing about what the database holds — a test that named a real user would
// pass on a laptop and fail on a runner.
func TestTheBinaryAnswersForAUserDatabase(t *testing.T) {
	sh := Shell{Name: "testsh"}.withDefaults([]string{"testsh"})
	r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)
	if r.UserHomeDir == nil {
		t.Fatal("the runner has no user-database hook, so `~user` is left as written")
	}
	if dir, ok := r.UserHomeDir("sh-test-no-such-user-2191"); ok {
		t.Errorf("a name no system has answered %q, want no answer", dir)
	}
}

// And the empty name is the user the process runs as, which is the one
// question about the database that has no name in the script to carry it: a
// bare `~` on a column that reads a password entry when `HOME` is unset.
//
// A hook that looked the empty string up like any other name would answer
// false — no system has a user called that — and the shell would go on
// leaving `~` as written with the database right there. That failure is
// silent from inside interp, which cannot tell a hook that declined from a
// hook that was never wired, so the convention is asserted where it is
// implemented. See interp.Runner.UserHomeDir and
// interp.TildeWithNoHomePolicy.
//
// What is asserted is that an answer arrives and is an absolute path, not
// which path: the home of whoever runs the test is a fact about the machine
// and a test that named one would pass on a laptop and fail on a runner.
func TestTheBinaryAnswersForTheCurrentUser(t *testing.T) {
	sh := Shell{Name: "testsh"}.withDefaults([]string{"testsh"})
	r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)
	dir, ok := r.UserHomeDir("")
	if !ok {
		t.Fatal("the empty name answered nothing, so a bare `~` with no HOME stays written")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("the current user's home is %q, want an absolute path", dir)
	}
}
