// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package gated is a shell that cannot run anything, for images that have no
// shell at all.
//
// A distroless or scratch image has no /bin/sh and no coreutils, so
// `kubectl exec` finds nothing to run. The usual answers are to add busybox to
// the image or to attach an ephemeral debug container. Both put an executable
// shell where an attacker who reaches code execution can find it, and neither
// leaves a record of what anyone did.
//
// This does the opposite. The interpreter is this module's own, in the dash
// dialect, and every action it takes passes a deny-by-default gate first:
// exec and signals are refused unconditionally, reads are bounded to a set of
// roots (/proc and /sys/fs/cgroup by default) with the process environment
// refused inside them, nothing may be opened for writing, and every action
// that happened is recorded.
//
// Main is a whole /bin/sh: `sh -c script [arg0 [arg...]]` or a script on
// standard input. A consumer's cmd/sh is a few lines; see the example. Shell
// is the same thing as a call, for a front end of the caller's own; the
// package deliberately ships no network front end, so nothing in it accepts
// input from anyone who cannot already exec into the container.
//
// The gate is policy.go, and it is the whole security argument. README.md in
// this directory is the operator-facing half, including the deployment rules
// the gate cannot enforce on its own: above all, the image must have no other
// shell or userland, because beside busybox a gated /bin/sh protects nothing.
//
// The package is a library and touches no process state: Main is handed its
// streams and its arguments rather than reading os.Args or os.Stdout, and a
// Shell's interpreter gets an empty environment and "/" as its directory.
package gated
