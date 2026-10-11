// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package startupcost_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// initBudgets is how many bytes a package's initialization may allocate
// before main runs, for the packages #6360 moved tables out of.
//
// Go initializes every linked package whether or not the route that wants it
// is taken, so a table built in a package-level initializer is paid by every
// `-c :`, every `--version` and every subshell a script starts with an exec —
// and by every binary that links the package, which for cmd/sh is all of
// them. GODEBUG=inittrace=1 measured dialect/zsh at 139 KB and 706
// allocations and dialect/bash at 93 KB, almost all of it key bindings and
// emulation plans that only `bindkey`, `bind`, `emulate` and the line editor
// read. Built on first use they are 63–66 KB and 12 KB, on darwin and Linux
// alike.
//
// The figure is bytes rather than time because bytes are deterministic: the
// same tree allocates the same amount at init on every run, while the clock
// beside it in the trace moves with the machine's load. The budgets leave
// room for the package to grow and none for a table of that size to come
// back. If one of these fails, the answer is usually sync.OnceValue on the
// table that was added — see dialect/zsh/bindkey.go — and only otherwise a
// larger budget, with the measurement beside it.
var initBudgets = map[string]int{
	"github.com/blairham/sh/dialect/zsh":  96 << 10,
	"github.com/blairham/sh/dialect/bash": 32 << 10,
}

// initCost is one package's line of an init trace.
type initCost struct{ bytes, allocs int }

// parseInitTrace reads GODEBUG=inittrace=1's lines:
//
//	init github.com/blairham/sh/interp @1.2 ms, 0.24 ms clock, 35720 bytes, 131 allocs
func parseInitTrace(trace string) map[string]initCost {
	out := map[string]initCost{}
	for _, line := range strings.Split(trace, "\n") {
		f := strings.Fields(line)
		if len(f) != 11 || f[0] != "init" || f[8] != "bytes," || f[10] != "allocs" {
			continue
		}
		b, errB := strconv.Atoi(f[7])
		a, errA := strconv.Atoi(f[9])
		if errB != nil || errA != nil {
			continue
		}
		out[f[1]] = initCost{bytes: b, allocs: a}
	}
	return out
}

func TestTheDialectsBuildTheirTablesOnFirstUse(t *testing.T) {
	dir := buildDirFor(t)
	checked := map[string]bool{}
	for _, bin := range []string{"our-sh", "our-bash", "our-zsh"} {
		cmd := exec.Command(filepath.Join(dir, bin), "-c", ":")
		cmd.Env = append(os.Environ(), "GODEBUG=inittrace=1")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s -c : failed: %v\n%s", bin, err, stderr.String())
		}
		inits := parseInitTrace(stderr.String())
		// The positive control: every binary links interp, so a trace that
		// does not name it is a trace this parser could not read — and a
		// parser that reads nothing would pass every budget below.
		if _, ok := inits["github.com/blairham/sh/interp"]; !ok {
			t.Fatalf("%s: no init line for interp in the trace, so the budgets below would check nothing:\n%s", bin, stderr.String())
		}
		for pkg, budget := range initBudgets {
			got, ok := inits[pkg]
			if !ok {
				continue
			}
			checked[pkg] = true
			if got.bytes > budget {
				t.Errorf("%s: %s allocates %d bytes (%d allocations) at init, over its budget of %d; a table built in a package-level initializer is paid by every start whether or not anything reads it — build it on first use",
					bin, pkg, got.bytes, got.allocs, budget)
			}
		}
	}
	for pkg := range initBudgets {
		if !checked[pkg] {
			t.Errorf("no binary measured here initialized %s, so its budget checked nothing", pkg)
		}
	}
}
