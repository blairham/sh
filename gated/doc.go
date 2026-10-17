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
// that happened is recorded. Two front ends share one gate:
//
//   - Main is a whole /bin/sh: `sh -c script [arg0 [arg...]]` or a script on
//     standard input. A consumer's cmd/sh is a few lines; see the example.
//   - Register mounts POST /debug/sh on an http.ServeMux, behind a bearer
//     token the caller supplies. An empty token mounts nothing.
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
