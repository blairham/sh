// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Temporary probe. Not committed.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/blairham/sh/internal/oracle"
)

type probe struct{ id, snippet string }

func main() {
	probes := []probe{
		{"and-assign", `x=0; : $((0 && (x = 9))); echo "and:$x"`},
		{"or-assign", `y=0; : $((1 || (y = 8))); echo "or:$y"`},
		{"and-value", `z=0; echo "val:$((0 && (z = 9)))"`},
		{"or-value", `z=0; echo "val:$((1 || (z = 9)))"`},
		{"tern-true", `w=5; echo "tern:$((0 ? (w = 1) : 2)) w=$w"`},
		{"tern-false", `w=5; echo "tern:$((1 ? 2 : (w = 1))) w=$w"`},
		{"and-incr", `x=0; : $((0 && (x++))); echo "andincr:$x"`},
		{"or-incr", `x=0; : $((1 || (x++))); echo "orincr:$x"`},
		{"and-predecr", `x=5; : $((0 && (--x))); echo "andpredecr:$x"`},
		{"left-operand", `x=0; echo "left:$(((x = 0) && (x = 9))) x=$x"`},
		{"nested-and", `x=0; y=0; : $((0 && (1 && (x = 1)) && (y = 2))); echo "nested:x=$x,y=$y"`},
		{"nested-or", `x=0; y=0; : $((1 || (0 || (x = 1)) || (y = 2))); echo "nestedor:x=$x,y=$y"`},
		{"and-value-differs", `x=0; echo "vd:$((0 && (x = 9))) x=$x"`},
		{"or-value-differs", `x=0; echo "vd:$((7 || (x = 9))) x=$x"`},
		{"comma", `x=0; echo "comma:$((1, (x = 3))) x=$x"`},
		{"and-inside-tern", `x=0; echo "ait:$((0 ? (0 && (x = 9)) : 4)) x=$x"`},
		{"tern-inside-and", `x=0; echo "tia:$((0 && (1 ? (x = 9) : 0))) x=$x"`},
		{"and-div-zero", `echo "dz:$((0 && (1/0)))"; echo "st:$?"`},
		{"or-div-zero", `echo "dz:$((1 || (1/0)))"; echo "st:$?"`},
		{"and-nested-plain", `x=0; : $((0 && x = 9)); echo "np:$x"`},
		{"and-double", `x=0; y=0; : $((0 && (x = 1) && (y = 2))); echo "dbl:x=$x,y=$y"`},
		{"or-double", `x=0; y=0; : $((1 || (x = 1) || (y = 2))); echo "ordbl:x=$x,y=$y"`},
		{"and-true-left", `x=0; : $((1 && (x = 9))); echo "tl:$x"`},
		{"or-false-left", `x=0; : $((0 || (x = 8))); echo "fl:$x"`},
		{"and-unset-var", `unset q; : $((0 && (q = 9))); echo "uv:${q-UNSET}"`},
	}

	ctx := context.Background()
	found, absent := oracle.Resolve(ctx)
	defer oracle.Shutdown()
	for _, a := range absent {
		fmt.Fprintln(os.Stderr, "absent:", a)
	}

	// Build our own binaries.
	ours := map[string]string{}
	dir, _ := os.MkdirTemp("", "ashprobe")
	for _, sh := range []string{"ash", "dash", "bash", "zsh", "ksh", "sh"} {
		bin := dir + "/" + sh
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/"+sh)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintln(os.Stderr, "build", sh, err, string(out))
			continue
		}
		ours[sh] = bin
	}

	names := []string{}
	for _, f := range found {
		names = append(names, f.Shell.Name)
	}
	fmt.Println("panel:", strings.Join(names, " "))

	for _, p := range probes {
		fmt.Println("=== " + p.id + "  " + p.snippet)
		c := oracle.Case{ID: p.id, Snippet: p.snippet}
		for _, f := range found {
			r := oracle.Exec(ctx, f, c)
			fmt.Printf("  %-14s %-40q st=%d %s\n", f.Shell.Name, r.Stdout, r.Status, strings.TrimSpace(r.Stderr))
		}
		for _, sh := range []string{"ash", "dash", "bash", "zsh", "ksh"} {
			bin, ok := ours[sh]
			if !ok {
				continue
			}
			cmd := exec.Command(bin, "-c", p.snippet)
			out, _ := cmd.Output()
			st := cmd.ProcessState.ExitCode()
			fmt.Printf("  %-14s %-40q st=%d\n", "ours/"+sh, strings.TrimRight(string(out), "\n"), st)
		}
	}
}
