// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/blairham/sh/internal/suite"
)

// printCells is #2291's leg-2 roll-up: the `(column, area)` space, what is
// closed, and the cells closed by measurement rather than by a file.
//
// It starts no shell and reads no reference, which is deliberate. Leg 2 is a
// number the campaign is quoted from, and a number that can only be produced
// by a full sweep is one that gets quoted from memory — which is how "120 of
// 230" came to reconstruct from nothing (#3481). This is a derivation over
// [suite.Areas], [suite.Ours] and the files, so anyone can re-run it in a
// second and disagree with it in public.
//
// It exits nonzero on a stale ledger entry, and only on that. The counts are
// meant to move; a ledger that has stopped describing the tree is a claim
// nobody is checking any more.
func printCells(root string) int {
	roll, err := suite.RollUp(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "suitecheck: the cell roll-up: %v\n", err)
		return 2
	}
	fmt.Println("#2291 leg 2 — the cell space, derived")
	fmt.Println()
	fmt.Print(roll.Report())
	fmt.Println()
	fmt.Println("  A cell is (column, area), where a column is one of our own suite's five")
	fmt.Println("  and is closed only when its reference is **gated** — both shells inside a")
	fmt.Println("  digest-pinned image, so the figure is the same on a runner as on a laptop.")
	fmt.Println("  The alternative reading — closed where the dialect merely runs a file for")
	fmt.Println("  the area — is 125 of 125 today and was measured saturated before this was")
	fmt.Println("  built; suite.TestOptionBIsSaturated is that measurement, committed, so")
	fmt.Println("  the choice can be re-argued if it ever stops being true.")
	fmt.Println()

	stale, err := suite.StaleUnclaimedFiles(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "suitecheck: the unclaimed-file ledger: %v\n", err)
		return 2
	}
	if len(stale) > 0 {
		fmt.Println("  STALE UNCLAIMED-FILE LEDGER")
		for _, s := range stale {
			fmt.Printf("    %s\n", s)
		}
		fmt.Println()
	}
	if len(roll.Stale) > 0 || len(stale) > 0 {
		return 1
	}
	return 0
}
