// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package gated_test

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/blairham/sh/gated"
)

// The whole of a consumer's cmd/sh. Build it with CGO_ENABLED=0 and install
// the binary at /bin/sh in a distroless or scratch image that has no other
// shell or userland; README.md in this package has the Dockerfile lines.
//
// The process state is read here, in the binary, and handed to Main: the
// arguments, the three streams and the host name. That is the line between a
// program and the library it links.
//
// The work is in a helper (run, in a real cmd/sh; renamed here only because
// this package's tests already have a run) so that the deferred stop runs
// before os.Exit: an os.Exit after a defer skips it, and gocritic's
// exitAfterDefer refuses that shape in every consumer's commit hook.
func ExampleMain() {
	os.Exit(runShell())
}

func runShell() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return gated.Main(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv("HOSTNAME"))
}
