// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package gated

import (
	"context"
	"fmt"
	"io"
)

// Main is the gated shell as a /bin/sh, and returns the process's exit status.
//
// argv is the command line after the program name: `-c script [arg0 [arg...]]`
// runs script with arg0 as $0 and the rest as $1, $2 and so on, exactly as real
// sh binds them; no operands reads the script from stdin. Anything else is a
// usage error and returns 2. session names this host in the audit trail; a
// pod's HOSTNAME is the natural value.
//
// The script's output goes to stdout and stderr as it would from any shell. A
// write to either that fails (a closed pipe, a dropped exec session) returns
// 1, so a truncated answer never exits 0. When the gate refused anything, a
// summary of the refusals follows on stderr: a glob the gate stopped and a
// glob that matched nothing look identical on stdout, and an operator reading
// "no output" would conclude the opposite of what happened.
//
// It takes its streams rather than reaching for the process's, which is what
// keeps this package a library. Main runs with DefaultPolicy; a front end that
// needs other roots builds a Shell itself.
//
// There is deliberately no interactive mode. Each run is a fresh interpreter
// so that one operator's leftover variable cannot change what the next one's
// script means, and a terminal session is exactly the state that rules out.
// `kubectl exec -i <pod> -- /bin/sh < script` covers the same ground.
func Main(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer, session string) int {
	src, args, code := source(argv, stdin, stderr)
	if code >= 0 {
		return code
	}

	sh := &Shell{Session: session}
	res, err := sh.Run(ctx, src, args...)

	if _, werr := io.WriteString(stdout, res.Stdout); werr != nil {
		fmt.Fprintf(stderr, "sh: write stdout: %v\n", werr)
		return 1
	}
	if _, werr := io.WriteString(stderr, res.Stderr); werr != nil {
		return 1
	}

	if res.Denied > 0 {
		fmt.Fprintf(stderr, "\nsh: the gate refused %d action(s):\n", res.Denied)
		for _, r := range res.Trail {
			if r.Kind == "denied" {
				fmt.Fprintf(stderr, "  line %d: %s %s\n", r.Line, r.Action, r.Path)
			}
		}
		fmt.Fprintln(stderr,
			"  (this shell cannot exec, and reads are bounded; see github.com/blairham/sh/gated)")
	}

	if err != nil {
		fmt.Fprintf(stderr, "sh: %v\n", err)
		return 1
	}
	return res.Status
}

// source reads the script from -c or from stdin, and returns what `sh -c`
// binds after the script: the first word is $0 and the rest are $1, $2 and so
// on. They are never joined into the script. code is -1 to carry on, or the
// status to exit with.
func source(argv []string, stdin io.Reader, stderr io.Writer) (src string, args []string, code int) {
	switch {
	case len(argv) >= 2 && argv[0] == "-c":
		return argv[1], argv[2:], -1
	case len(argv) == 0:
		if stdin == nil {
			return "", nil, -1
		}
		// One byte past the cap, so an oversized script reaches Run and is
		// refused there with the cap in the message, rather than being read
		// whole into memory first.
		b, err := io.ReadAll(io.LimitReader(stdin, maxScript+1))
		if err != nil {
			fmt.Fprintf(stderr, "sh: read stdin: %v\n", err)
			return "", nil, 1
		}
		return string(b), nil, -1
	default:
		fmt.Fprintln(stderr, "usage: sh -c <script> [arg0 [arg...]]   |   sh < script")
		fmt.Fprintln(stderr, "a gated shell: no exec, reads bounded to /proc and /sys/fs/cgroup")
		return "", nil, 2
	}
}
