// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command corpusguard fails when the oracle corpus has lost a case.
//
// The corpus is a set that git merges as text, so it can shrink without a
// conflict, without a failing test and without anything in a diff to notice —
// a reviewer reads what changed, not what silently left. This compares the
// case IDs in the working tree against the case IDs at the merge base and
// fails on any that are gone.
//
// See internal/corpusguard for why the baseline is history rather than a
// committed count or a committed list.
//
//	corpus-guard                 # compare against the merge base with main
//	go run ./internal/cmd/corpusguard -base <sha>
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/blairham/sh/internal/corpusguard"
	"github.com/blairham/sh/internal/oracle"
)

func main() {
	base := flag.String("base", "", "commit to compare the corpus against; the merge base with main if it does not resolve")
	dir := flag.String("C", ".", "repository to check")
	flag.Parse()

	live := make([]string, 0, len(oracle.Corpus))
	for _, c := range oracle.Corpus {
		live = append(live, c.ID)
	}

	r, err := corpusguard.Check(*dir, *base, live)
	if err != nil {
		fmt.Fprintf(os.Stderr, "corpus-guard: %v\n", err)
		os.Exit(2)
	}
	if r.OK() {
		fmt.Printf("corpus-guard: %d cases, none lost since %s (%d there)\n",
			r.HeadIDs, r.Base[:min(12, len(r.Base))], r.BaseIDs)
		return
	}
	for _, id := range r.Dropped {
		fmt.Fprintf(os.Stderr, "corpus-guard: case %q was present at %s and is gone\n", id, r.Base[:min(12, len(r.Base))])
	}
	for _, id := range r.Duplicate {
		fmt.Fprintf(os.Stderr, "corpus-guard: case %q is written twice; one would overwrite the other in the golden record\n", id)
	}
	if len(r.Dropped) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d case(s) lost: %d at the base, %d now. A merge or a rebase resolved\n"+
			"%s by taking one side. Recover them from the base revision and keep both\n"+
			"sides; if a case is being retired on purpose, say so in corpusguard.Retired.\n",
			len(r.Dropped), r.BaseIDs, r.HeadIDs, corpusguard.CasePath)
	}
	os.Exit(1)
}
