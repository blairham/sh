// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"
)

// newTestRunner is the in-package twin of the helper in interp_test.go.
//
// Two of them because there are two test packages in this directory: most of
// the tests are the external `interp_test`, which is the rule for this package
// — a test that asserts what a dialect does has to import one, and a dialect
// imports interp — but a handful reach unexported state and have to be in
// `package interp`. Identifiers do not cross between the two, so the helper
// cannot be shared, and the alternative to a twin is those files being the one
// place the rule does not reach. They are exactly the files a guard is for.
//
// The contract is the same one: a Runner a test builds gets a working
// directory and a temporary directory of its own, both of which the framework
// takes away again, and its CleanUp is registered rather than remembered. See
// the long comment on the other one for why each of those matters.
func newTestRunner(t *testing.T, r *Runner) *Runner {
	t.Helper()
	if r.Dir == "" {
		r.Dir = t.TempDir()
	}
	if !hasTestEnv(r.Env, "TMPDIR") {
		r.Env = append(r.Env, "TMPDIR="+t.TempDir())
	}
	t.Cleanup(r.CleanUp)
	return r
}

func hasTestEnv(env []string, name string) bool {
	for _, kv := range env {
		if strings.HasPrefix(kv, name+"=") {
			return true
		}
	}
	return false
}
