// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command promptfidelity compares the prompt this shell draws with the
// prompt the program a configuration was imported from draws.
//
// `make prompt-fidelity`. It is in the family of `make oracle`, `make acp`
// and `make sandbox`: drive the real thing, drive ours, compare — and it is
// the instrument docs/spec/prompt-theme.md asks for by name, because "it
// looks the same" is an opinion until something can fail.
//
// **Report-only, and never a gate**, for the reason `make wild` is not one
// and one more. The answer depends on what happens to be installed: a
// comparison against powerlevel10k needs a real zsh and the theme, and a
// comparison against starship needs the program. A source that is not here
// is a **row that says so**, because a table listing two sources where three
// were asked for, with nothing explaining the difference, reads as a source
// that agreed.
//
// It is also not `make check`'s business: it launches real programs on real
// terminals and waits for them to go quiet.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/blairham/sh/internal/promptfidelity"
)

func main() {
	binary := flag.String("bin", "", "the dialect binary to drive")
	config := flag.String("config", "", "this engine's configuration, as `prompt import` wrote it")
	starship := flag.String("starship", "", "the starship binary, or empty to find it on PATH")
	starshipConfig := flag.String("starship-config", "", "the starship configuration the import was made from")
	zsh := flag.String("zsh", "", "the real zsh, or empty to find it on PATH")
	p10k := flag.String("p10k", "", "the directory holding powerlevel10k — no default, and there will not be one")
	p10kConfig := flag.String("p10k-config", "", "the powerlevel10k configuration the import was made from")
	dir := flag.String("dir", "", "the directory to draw the prompt in (default: a scratch one)")
	columns := flag.Int("columns", 80, "the terminal width to compare at")
	status := flag.Int("status", 0, "the exit status to pin")
	jobs := flag.Int("jobs", 0, "the job count to pin")
	duration := flag.Duration("duration", 0, "the command duration to pin")
	flag.Parse()

	ctx := promptfidelity.Context{
		Dir:      *dir,
		Home:     os.Getenv("HOME"),
		Status:   *status,
		Duration: *duration,
		Jobs:     *jobs,
		Columns:  *columns,
	}
	if ctx.Dir == "" {
		scratch, err := os.MkdirTemp("", "promptfidelity-dir")
		if err != nil {
			fmt.Fprintf(os.Stderr, "promptfidelity: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = os.RemoveAll(scratch) }()
		ctx.Dir = scratch
	}
	if abs, err := filepath.Abs(ctx.Dir); err == nil {
		ctx.Dir = abs
	}

	fmt.Printf("prompt fidelity, at %d columns, status %d, %d job(s), duration %s\n",
		ctx.Columns, ctx.Status, ctx.Jobs, ctx.Duration)
	fmt.Printf("in %s\n\n", ctx.Dir)

	ours := promptfidelity.Ours{Binary: *binary, Config: *config}
	// The theme's scratch home is prepared once and removed at the end: it
	// fetches a helper daemon on its first run in a fresh one, so a home per
	// render is a download per render.
	theme := &promptfidelity.Powerlevel10k{Zsh: *zsh, Plugin: *p10k, Config: *p10kConfig}
	defer func() { _ = theme.Close() }()
	rows := []promptfidelity.Row{
		promptfidelity.Compare("starship", ours,
			promptfidelity.Starship{Binary: *starship, Config: *starshipConfig}, ctx),
		promptfidelity.Compare("powerlevel10k", ours, theme, ctx),
	}

	ran, agreed := 0, 0
	for _, row := range rows {
		fmt.Print(row.Report())
		if !row.Ran {
			continue
		}
		ran++
		if row.Agrees() {
			agreed++
		}
	}
	fmt.Printf("\n%s of %s comparisons agree; %s were not run\n",
		count(agreed), count(ran), count(len(rows)-ran))
	// Exit 0 whatever it found. The claim this instrument checks is one this
	// implementation does not yet make — the spec says a preset named after
	// another project is a claim about fidelity and that none is named that
	// way until this can fail — so a nonzero status here would be failing a
	// build over work that is openly unfinished.
}

func count(n int) string { return strconv.Itoa(n) }
