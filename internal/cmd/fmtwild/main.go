// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command wild formats every shell script it can find and holds each result
// to the formatter's three promises: the tree survives, the comments survive,
// and a second pass changes nothing. It never runs anything.
//
// Scripts the substrate cannot parse are counted and set aside — they are
// parser facts, not formatter facts. Everything else must verify.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"

	"github.com/blairham/sh/internal/fmt/comments"
	"github.com/blairham/sh/internal/fmt/printer"
	"github.com/blairham/sh/internal/wild"
)

var (
	dirs = flag.String("dirs", strings.Join(wild.DefaultDirs, ","),
		"comma-separated roots to sweep; "+wild.DirsVar+" adds the framework trees a shell sources at startup")
	verbose = flag.Bool("v", false, "list every failing path under its cause")
	depth   = flag.Int("depth", wild.DefaultDepth, "how far below each root to descend")
	explain = flag.String("explain", "", "show one script's failure in detail and exit")
)

// explainOne formats one script and reports exactly how it fails: the
// reparse error with the offending output lines, or which property broke.
func explainOne(path string) {
	src, d, st, ok := read(path)
	if !ok {
		fmt.Println("not recognized as a shell script")
		return
	}
	f, err := syntax.Parse(src, d)
	if err != nil {
		fmt.Println("input does not parse:", err)
		return
	}
	cs := comments.Recover(src, f)
	out := printer.Format(src, f, cs, st)
	fOut, err := syntax.Parse(out, d)
	if err != nil {
		fmt.Println("output does not parse:", err)
		lines := strings.Split(out, "\n")
		var at int
		if e, ok := err.(*syntax.Error); ok {
			at = int(e.Pos.Line)
		}
		for i := max(0, at-4); i < min(len(lines), at+3); i++ {
			fmt.Printf("%5d| %s\n", i+1, lines[i])
		}
		return
	}
	if why, ok := syntax.SameProgram(f, fOut); !ok {
		fmt.Println("the program changed at", why)
		return
	}
	if !sameComments(cs, comments.Recover(out, fOut)) {
		fmt.Println("comments changed")
		return
	}
	if again := printer.Format(out, fOut, comments.Recover(out, fOut), st); again != out {
		reportFirstDiff("not idempotent", out, again)
		return
	}
	fmt.Println("verifies clean")
}

func reportFirstDiff(what, a, b string) {
	fmt.Println(what)
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) || i < len(bl); i++ {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			fmt.Printf("first difference at line %d:\n", i+1)
			for j := max(0, i-2); j < min(len(al), i+4); j++ {
				fmt.Printf("  before| %s\n", al[j])
			}
			for j := max(0, i-2); j < min(len(bl), i+4); j++ {
				fmt.Printf("  after | %s\n", bl[j])
			}
			return
		}
	}
}

// everyDialect is the shebang scope: a formatter is answerable for every
// dialect the parser reads, where the grading sweeps each answer for one.
var everyDialect = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "ksh": true, "ksh93": true, "dash": true,
}

func main() {
	flag.Parse()
	if *explain != "" {
		explainOne(*explain)
		return
	}
	var parsed, unparseable, notAScript, denied int
	fails := map[string][]string{}
	// wild.Find is the shared collector: it follows the links a package
	// manager builds a tree out of, counts each script once by its resolved
	// path, and applies CLEANROOM's denylist. This sweep had its own walk and
	// its own denied() before, and the second copy was the weaker one — it
	// matched a handful of path substrings where wild.Denied knows about
	// package roots, version directories and another project's test data. A
	// formatter sweep that reads a shell's distribution because its private
	// list was shorter is the failure that list exists to prevent.
	scope := wild.Scope{
		Dirs:   append(strings.Split(*dirs, ","), wild.DirsFrom(os.LookupEnv)...),
		Shells: everyDialect,
		Depth:  *depth,
	}
	paths, refused := wild.Find(scope)
	for _, path := range paths {
		src, dialect, layout, ok := read(path)
		if !ok {
			notAScript++
			continue
		}
		f, err := syntax.Parse(src, dialect)
		if err != nil {
			unparseable++
			continue
		}
		parsed++
		if cause := verify(src, f, dialect, layout); cause != "" {
			fails[cause] = append(fails[cause], path)
		}
	}
	for _, n := range refused {
		// Counted, never named: a denied path in a report is an invitation to
		// go and look, which is the one thing that must not happen.
		denied += n
	}
	// Four numbers rather than two, because three different things were not
	// checked and rolling them together reads as coverage. "0 failures" over a
	// population that was mostly set aside is the reading this has to make
	// impossible.
	fmt.Printf("laid out and verified: %d\n", parsed)
	fmt.Printf("  this parser could not read: %d\n", unparseable)
	fmt.Printf("  not a shell script we name:  %d\n", notAScript)
	fmt.Printf("  refused by CLEANROOM:        %d\n", denied)
	if parsed == 0 {
		fmt.Println("nothing was formatted — widen -dirs, or set " + wild.DirsVar)
		os.Exit(1)
	}
	if len(fails) == 0 {
		fmt.Println("failures: none")
		return
	}
	causes := make([]string, 0, len(fails))
	for c := range fails {
		causes = append(causes, c)
	}
	sort.Slice(causes, func(i, j int) bool { return len(fails[causes[i]]) > len(fails[causes[j]]) })
	for _, c := range causes {
		fmt.Printf("FAIL %-22s %d script(s)   e.g. %s\n", c, len(fails[c]), fails[c][0])
		if *verbose {
			for _, p := range fails[c] {
				fmt.Println("   ", p)
			}
		}
	}
	os.Exit(1)
}

// read decides whether a file is a shell script and in which dialect, by the
// rules the substrate's own wild sweep uses: shebang first, then the name.
// read also decides the layout, because the dialect and its layout are one
// decision: a file read as zsh is laid out as zsh.
func read(path string) (string, syntax.Dialect, syntax.Style, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > 1<<20 {
		return "", syntax.Dialect{}, syntax.Style{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", syntax.Dialect{}, syntax.Style{}, false
	}
	src := string(b)
	first, _, _ := strings.Cut(src, "\n")
	switch {
	case strings.HasPrefix(first, "#!"):
		switch {
		case strings.Contains(first, "bash"):
			return src, bash.Dialect(), bash.Style(), true
		case strings.Contains(first, "zsh"):
			return src, zsh.Dialect(), zsh.Style(), true
		case strings.Contains(first, "dash"):
			return src, dash.Dialect(), dash.Style(), true
		case strings.Contains(first, "ksh"):
			return src, ksh.Dialect(), ksh.Style(), true
		case strings.HasSuffix(first, "/sh"), strings.HasSuffix(first, " sh"):
			return src, syntax.Core(), syntax.CoreStyle(), true
		}
		return "", syntax.Dialect{}, syntax.Style{}, false
	case strings.HasPrefix(first, "#compdef"), strings.HasPrefix(first, "#autoload"):
		return src, zsh.Dialect(), zsh.Style(), true
	}
	switch strings.TrimPrefix(filepath.Ext(path), ".") {
	case "sh":
		return src, syntax.Core(), syntax.CoreStyle(), true
	case "bash":
		return src, bash.Dialect(), bash.Style(), true
	case "zsh", "zsh-theme":
		return src, zsh.Dialect(), zsh.Style(), true
	}
	return "", syntax.Dialect{}, syntax.Style{}, false
}

func verify(src string, f *syntax.File, d syntax.Dialect, st syntax.Style) string {
	cs := comments.Recover(src, f)
	out := printer.Format(src, f, cs, st)
	fOut, err := syntax.Parse(out, d)
	if err != nil {
		return "output-unparseable"
	}
	// SameProgram, not two canonical prints compared: printing both through
	// the canonicalizer erases exactly the differences a formatter is most
	// likely to introduce, so two scripts that differ can print alike.
	if _, ok := syntax.SameProgram(f, fOut); !ok {
		return "program-changed"
	}
	if !sameComments(cs, comments.Recover(out, fOut)) {
		return "comments-changed"
	}
	f2, _ := syntax.Parse(out, d)
	if printer.Format(out, f2, comments.Recover(out, f2), st) != out {
		return "not-idempotent"
	}
	return ""
}

func sameComments(a, b []comments.Comment) bool {
	if len(a) != len(b) {
		return false
	}
	at := make([]string, len(a))
	bt := make([]string, len(b))
	for i := range a {
		at[i], bt[i] = a[i].Text, b[i].Text
	}
	sort.Strings(at)
	sort.Strings(bt)
	for i := range at {
		if at[i] != bt[i] {
			return false
		}
	}
	return true
}
