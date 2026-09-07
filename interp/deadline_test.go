// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"
	"time"
)

// deadline runs body and fails the test if it has not finished in time.
//
// The house rule for any case whose subject can *hang* rather than answer
// wrongly, and it is a rule because of the shape a hang has when it is not
// bounded: the test never returns, `go test` panics at the package timeout,
// and the report names whichever case the binary happened to be sitting in
// with no statement of what it was waiting for. A ten-minute build failure
// with no assertion in it is worse than a red test, which is why #1071 lists
// the hanging tests beside the failing ones.
//
// Bounded so that the failure is *named and quick*, not so that a slow case
// passes. The bound is two orders of magnitude above what any of these cases
// takes when it works — the ones that use it finish in milliseconds — so a run
// that reaches it is not slow, it is stopped. Raising this to make something
// green would be reading the tool backwards.
//
// The body keeps running after the failure, and that is the trade for a case
// written this way rather than a limitation of the package any more:
// [interp.Runner.Run] consults its context at every command since #1075, so a
// case that hands the runner a cancelable context and cancels it on the bound
// gets a round that *stops* as well as one that is named —
// procsubeof_test.go is written that way and could not have been before. This
// form stays for the cases whose body is not a Runner at all, and for those
// the trade is unchanged: a goroutine left spinning until the package ends is
// still far less than a package that ends by timing out, and where the body
// is waiting on something rather than spinning the case releases it in a
// t.Cleanup — see blockingFifo.
func deadline(t *testing.T, what string, body func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		body()
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("%s did not finish: the shell is still running", what)
	}
}
