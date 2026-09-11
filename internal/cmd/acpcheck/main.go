// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command acpcheck drives the shell's Agent Client Protocol front end the way
// an editor would — as a subprocess, over JSON-RPC on a pipe — and prints what
// works, what it cost, and what an ordinary pipe would have seen instead.
//
// Run it with `make acp`. It answers three questions in one table each:
//
//	does it work        every property docs/design/acp.md claims, graded
//	what does it cost   the handshake, and the time a turn adds to a command
//	why use it          the same script under a pipe and under the protocol
//
// It is not a gate, for the reason `make smoke` is not one: it launches real
// binaries and real children and times them. `go test ./internal/acpcheck`
// covers the instrument's own machinery.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blairham/sh/internal/acpcheck"
)

func main() {
	var (
		bin  = flag.String("bin", "", "the shell binary to drive")
		root = flag.String("dir", "", "where to make the scratch directories; a temporary one by default")
		only = flag.String("run", "", "grade only the rows whose name contains this")
		verb = flag.Bool("v", false, "print the detail for every row, not only for the ones that did not pass")
		wire = flag.Bool("wire", false, "print a real annotated transcript of one session, message by message")
		// The client rows need an agent to drive, and this binary is one:
		// re-executed with this flag it speaks ACP as an agent instead of
		// grading one. Not a mode anybody runs by hand.
		asAgent = flag.String("as-agent", "", "internal: behave as a scripted ACP agent, not as the grader")
	)
	flag.Parse()

	if *asAgent != "" {
		os.Exit(acpcheck.RunAgent(*asAgent))
	}

	if *bin == "" {
		fmt.Fprintln(os.Stderr, "acpcheck: name the binary to drive with -bin")
		os.Exit(2)
	}
	// Resolved here, because every row runs the binary from a scratch
	// directory of its own: a relative path that worked on the command line
	// names nothing from there, and the failure reads as an agent that would
	// not start.
	shell, err := filepath.Abs(*bin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "acpcheck:", err)
		os.Exit(2)
	}
	dir := *root
	if dir == "" {
		d, err := os.MkdirTemp("", "acpcheck")
		if err != nil {
			fmt.Fprintln(os.Stderr, "acpcheck:", err)
			os.Exit(2)
		}
		defer func() { _ = os.RemoveAll(d) }()
		dir = d
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "acpcheck:", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if *wire {
		t, err := acpcheck.Transcript(ctx, shell, filepath.Join(dir, "transcript"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "acpcheck:", err)
			os.Exit(1)
		}
		fmt.Print(t)
		return
	}

	// The path to this binary is how the client rows get an agent to drive.
	// A build that cannot name itself grades the agent direction alone.
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "acpcheck: naming this binary:", err)
	}
	res := acpcheck.Run(ctx, shell, dir, self, *only)
	fmt.Print(table(res, *verb))

	if *only == "" {
		cmp, err := acpcheck.Compare(ctx, shell, filepath.Join(dir, "compare"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "acpcheck: comparison:", err)
		} else {
			fmt.Print(cmp.Report())
		}
	}
	if res.Failed() {
		os.Exit(1)
	}
}

// table renders the graded rows.
func table(res acpcheck.Result, verbose bool) string {
	var b strings.Builder
	width := 0
	for _, r := range res.Rows {
		if len(r.Name) > width {
			width = len(r.Name)
		}
	}
	fmt.Fprintf(&b, "\nACP, graded against the binary that ships\n\n")
	pass := 0
	for _, r := range res.Rows {
		mark := "FAIL"
		if r.Pass {
			mark = "ok"
			pass++
		} else if r.Known != "" {
			mark = "known"
		}
		fmt.Fprintf(&b, "  %-6s %-*s  %s\n", mark, width, r.Name, r.Claim)
		if (!r.Pass || verbose) && r.Detail != "" {
			fmt.Fprintf(&b, "  %-6s %-*s  └─ %s\n", "", width, "", r.Detail)
		}
		if !r.Pass && r.Known != "" {
			fmt.Fprintf(&b, "  %-6s %-*s  └─ owned by %s\n", "", width, "", r.Known)
		}
	}
	fmt.Fprintf(&b, "\n  %d of %d properties hold\n", pass, len(res.Rows))
	return b.String()
}
