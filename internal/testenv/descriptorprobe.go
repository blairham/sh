// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package testenv

import "strconv"

// The two answers a DescriptorProbe child gives, each on a line of its own.
const (
	DescriptorOpen   = "dup=ok"
	DescriptorClosed = "dup=failed"
)

// DescriptorProbe is a command line that starts a child and has it say whether
// descriptor fd was open when it started: DescriptorOpen or DescriptorClosed,
// written through the redirection report — "" for standard output, ">&2" for
// standard error, ">"+path for a file. A probe of standard output has to be
// told to answer somewhere else. report goes inside single quotes, so a path
// in it must not contain one.
//
// It is for the question #1260 was about: whether a shell hands a command a
// *closed* descriptor where the script closed one, rather than an empty one —
// a pipe that has already shut, or the null device. Those two read the same to
// anything that only reads or writes, and the child is asked something else.
//
// # Not a program's complaint
//
// The rows that ask this used to hand the descriptor to /bin/cat or /bin/echo
// and read whether it failed and said so. That is the program's policy rather
// than a fact about the descriptor, and on Ubuntu 26.04 the policy changed:
// /bin/cat and /bin/echo are uutils coreutils there, and both answer a closed
// standard input or output silently at status 0 — the answer an empty stream
// gives — where GNU and BSD fail at 1 and say `Bad file descriptor` (#7199).
// Nine rows failed on that runner with the shell unchanged.
//
// # Not this test binary
//
// The obvious hermetic child is the test binary re-executed, inspecting the
// descriptor itself. It cannot see the thing being asked. The Go runtime opens
// the null device read-write on any of 0, 1 and 2 that a Go program starts
// without, before any code of the program runs, so a Go child handed a closed
// standard input finds it open — on a character device, read-write, which is
// also exactly what `0<>/dev/null` hands it. Measured on macOS and in
// ubuntu:24.04 and ubuntu:26.04 alike.
//
// # Asked by duplicating it
//
// So the child is /bin/sh, and it duplicates the descriptor. Duplicating a
// closed one fails with EBADF, and a failed redirection fails the subshell it
// is on — POSIX requires both. Duplicating an open one succeeds whatever is in
// it, buffered or not, read or not, so neither an empty pipe nor the null
// device can pass for closed. Measured with bash 3.2 as macOS's /bin/sh and
// dash as Ubuntu's, for each of 0, 1 and 2: closed answers DescriptorClosed;
// the null device and an empty pipe answer DescriptorOpen.
//
// The subshell keeps the failed redirection from ending the child before it
// can answer, and the child exits 0 either way, so a status other than 0 next
// to its answer is the shell under test's own. The host shell's diagnostic
// for the failed duplicate, whose wording is the host's, goes to the null
// device — except when the descriptor asked about is standard error, where that
// redirection would put the null device on it and answer "open" every time.
func DescriptorProbe(fd int, report string) string {
	n := strconv.Itoa(fd)
	dup := "( exec 3>&" + n + " ) 2>/dev/null"
	switch fd {
	case 0:
		dup = "( exec 3<&0 ) 2>/dev/null"
	case 2:
		dup = "( exec 3>&2 )"
	}
	if report != "" {
		report = " " + report
	}
	return "/bin/sh -c '" + dup + " && echo " + DescriptorOpen + report +
		" || echo " + DescriptorClosed + report + "'"
}
