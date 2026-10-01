// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// **A fill puts what PATH holds into the table once, keeps what was there,
// and is undone by emptying the table.** See Runner.FillCommandHashFromPath.
func TestFillingTheCommandTableFromPath(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"zexe", "zkept"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "znoexec"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(src string) string {
		out, _ := runGrammar(t, "PATH="+dir+"\n"+src, nil, func(r *Runner) {
			r.Register("fill", func(rr *Runner, _ context.Context, args []string) int {
				rr.FillCommandHashFromPath(len(args) > 0)
				return 0
			})
			r.Register("table", func(rr *Runner, _ context.Context, _ []string) int {
				var parts []string
				names := rr.HashedCommandNames()
				slices.Sort(names)
				for _, n := range names {
					p, _ := rr.HashedCommandPath(n)
					parts = append(parts, n+"="+strings.TrimPrefix(p, dir+"/"))
				}
				_, _ = fmt.Fprintln(rr.Out(), strings.Join(parts, " "))
				return 0
			})
			r.Register("keep", func(rr *Runner, _ context.Context, _ []string) int {
				rr.HashCommand("zkept", "/elsewhere")
				return 0
			})
		})
		return out
	}
	if got, want := run("keep; fill; table"), "zexe=zexe zkept=/elsewhere znoexec=znoexec\n"; got != want {
		t.Errorf("a fill: %q, want %q", got, want)
	}
	if got, want := run("keep; fill x; table"), "zexe=zexe zkept=/elsewhere\n"; got != want {
		t.Errorf("executables only: %q, want %q", got, want)
	}
	if got, want := run("keep; fill x; fill; table"), "zexe=zexe zkept=/elsewhere\n"; got != want {
		t.Errorf("a second fill: %q, want the first one standing, %q", got, want)
	}
	if got, want := run("keep; fill x; PATH=$PATH; fill; table"), "zexe=zexe zkept=zkept znoexec=znoexec\n"; got != want {
		t.Errorf("after PATH empties the table: %q, want a fill of its own, %q", got, want)
	}
}
